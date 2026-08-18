package files

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ObjectStore is the narrow surface Service needs from object storage -
// implemented against MinIO locally and real S3 in production via the
// same adapter (internal/files/s3), matching the roadmap's own framing
// of "S3, MinIO locally" as one storage adapter, not two.
type ObjectStore interface {
	PresignPut(ctx context.Context, objectKey, contentType string, expires time.Duration) (url string, err error)
	PresignGet(ctx context.Context, objectKey string, expires time.Duration) (url string, err error)
	// HeadObject reads the object's real size and checksum (ETag) from
	// storage - ConfirmUpload trusts this, never the client's declared
	// values, the same "verify, don't trust the caller" principle as
	// Phase 10's realtime-gateway resolving identity server-side.
	HeadObject(ctx context.Context, objectKey string) (sizeBytes int64, checksum string, err error)
	DeleteObject(ctx context.Context, objectKey string) error
}

// AdminChecker mirrors notifications.AdminChecker - satisfied directly by
// *authz.Service, no adapter needed.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

type Config struct {
	MaxSizeBytes int64
	// AllowedMimePrefixes is matched as a prefix (e.g. "image/" matches
	// "image/png") so a whole media family can be allowed without
	// enumerating every subtype. Empty means "allow anything" - a
	// deliberately permissive default, since this is a generic platform
	// capability, not a product policy; a product wanting a strict
	// allowlist sets one.
	AllowedMimePrefixes []string
	UploadURLTTL        time.Duration
	DownloadURLTTL      time.Duration
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Service struct {
	repo  Repository
	store ObjectStore
	admin AdminChecker
	cfg   Config
}

func NewService(repo Repository, store ObjectStore, admin AdminChecker, cfg Config) *Service {
	if cfg.MaxSizeBytes <= 0 {
		cfg.MaxSizeBytes = 100 << 20 // 100MB
	}
	if cfg.UploadURLTTL <= 0 {
		cfg.UploadURLTTL = 15 * time.Minute
	}
	if cfg.DownloadURLTTL <= 0 {
		cfg.DownloadURLTTL = 15 * time.Minute
	}
	return &Service{repo: repo, store: store, admin: admin, cfg: cfg}
}

// RequestUpload validates the declared metadata, creates a Pending File
// row, and returns a presigned PUT URL the client uploads bytes to
// directly - this service never receives the bytes.
func (s *Service) RequestUpload(ctx context.Context, callerID string, in RequestUploadInput) (File, string, time.Time, error) {
	if err := in.Validate(); err != nil {
		return File{}, "", time.Time{}, err
	}
	if in.SizeBytes > s.cfg.MaxSizeBytes {
		return File{}, "", time.Time{}, ErrSizeLimitExceeded
	}
	if !s.mimeAllowed(in.MimeType) {
		return File{}, "", time.Time{}, ErrMimeTypeNotAllowed
	}
	if in.Visibility == "" {
		in.Visibility = VisibilityPrivate
	}

	objectKey := buildObjectKey(in.AppID, callerID, in.FileName)
	f, err := s.repo.Create(ctx, callerID, objectKey, in)
	if err != nil {
		return File{}, "", time.Time{}, err
	}
	url, err := s.store.PresignPut(ctx, objectKey, in.MimeType, s.cfg.UploadURLTTL)
	if err != nil {
		return File{}, "", time.Time{}, err
	}
	return f, url, time.Now().Add(s.cfg.UploadURLTTL), nil
}

func (s *Service) mimeAllowed(mimeType string) bool {
	if len(s.cfg.AllowedMimePrefixes) == 0 {
		return true
	}
	for _, prefix := range s.cfg.AllowedMimePrefixes {
		if strings.HasPrefix(mimeType, prefix) {
			return true
		}
	}
	return false
}

// buildObjectKey namespaces every object by app and owner so two users
// (or two apps sharing the platform) can never collide on the same key,
// and appends a UUID so re-uploading a file with the same name never
// overwrites a still-referenced object.
func buildObjectKey(appID, ownerUserID, fileName string) string {
	return appID + "/" + ownerUserID + "/" + uuid.NewString() + "-" + sanitizeFileName(fileName)
}

func sanitizeFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "file"
	}
	return b.String()
}

// ConfirmUpload verifies the object actually landed in storage and that
// its real size/checksum match what was expected, before marking the
// file Active. Confirming an already-Active file is a no-op (idempotent
// double-confirm), not an error.
func (s *Service) ConfirmUpload(ctx context.Context, callerID, id string) (File, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return File{}, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, f.OwnerUserID); err != nil {
		return File{}, err
	}
	if f.Status == StatusActive {
		return f, nil
	}
	if f.Status != StatusPending {
		return File{}, ErrNotPending
	}

	actualSize, actualChecksum, err := s.store.HeadObject(ctx, f.ObjectKey)
	if err != nil {
		return File{}, err
	}
	if f.Checksum != "" && !strings.EqualFold(f.Checksum, actualChecksum) {
		return File{}, ErrChecksumMismatch
	}
	return s.repo.ConfirmUpload(ctx, id, actualSize, actualChecksum)
}

func (s *Service) Get(ctx context.Context, callerID, id string) (File, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return File{}, err
	}
	if err := s.requireVisible(ctx, callerID, f); err != nil {
		return File{}, err
	}
	return f, nil
}

// GetDownloadURL requires the file be Active - a pending or deleted file
// has no bytes to download, presigned URL or not.
func (s *Service) GetDownloadURL(ctx context.Context, callerID, id string) (string, time.Time, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := s.requireVisible(ctx, callerID, f); err != nil {
		return "", time.Time{}, err
	}
	if f.Status != StatusActive {
		return "", time.Time{}, ErrNotActive
	}
	url, err := s.store.PresignGet(ctx, f.ObjectKey, s.cfg.DownloadURLTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	return url, time.Now().Add(s.cfg.DownloadURLTTL), nil
}

func (s *Service) ListMine(ctx context.Context, callerID string, params ListParams) (ListResult, error) {
	if params.Limit <= 0 || params.Limit > maxListLimit {
		params.Limit = defaultListLimit
	}
	return s.repo.ListForOwner(ctx, callerID, params)
}

// Delete removes the object from storage first, then soft-deletes the
// row - if the storage delete fails, the row stays Active rather than
// claiming a file is gone when its bytes still exist.
func (s *Service) Delete(ctx context.Context, callerID, id string) error {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, f.OwnerUserID); err != nil {
		return err
	}
	if f.Status == StatusDeleted {
		return nil
	}
	if err := s.store.DeleteObject(ctx, f.ObjectKey); err != nil {
		return err
	}
	_, err = s.repo.SoftDelete(ctx, id)
	return err
}

// PurgeExpired deletes every Active file past its retention ExpiresAt.
// Not wired to a scheduler in this phase - callable directly (and via an
// admin-only endpoint) rather than run automatically, the same
// documented gap as this platform's outbox-to-Kafka relay and
// notifications' retry mechanism.
func (s *Service) PurgeExpired(ctx context.Context, callerID string) (int, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return 0, err
	}
	expired, err := s.repo.ListExpired(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, f := range expired {
		if err := s.store.DeleteObject(ctx, f.ObjectKey); err != nil {
			continue
		}
		if _, err := s.repo.SoftDelete(ctx, f.ID); err != nil {
			continue
		}
		purged++
	}
	return purged, nil
}

func (s *Service) requireVisible(ctx context.Context, callerID string, f File) error {
	if f.Visibility == VisibilityPublic {
		return nil
	}
	return s.requireOwnerOrAdmin(ctx, callerID, f.OwnerUserID)
}

func (s *Service) requireOwnerOrAdmin(ctx context.Context, callerID, ownerID string) error {
	if callerID == ownerID {
		return nil
	}
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}

func (s *Service) requireAdmin(ctx context.Context, callerID string) error {
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
