// Package files is the File / Media Platform: presigned upload/download
// against S3-compatible object storage (MinIO locally, real S3 in
// production - the same adapter either way), metadata/ownership/
// permissions in Postgres. Binary bytes are never stored in Postgres and
// never proxied through this service - the client uploads/downloads
// directly against a presigned URL, and this service never sees the
// bytes at all.
package files

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

func (v Visibility) valid() bool {
	return v == VisibilityPrivate || v == VisibilityPublic
}

// Status tracks the presigned-upload lifecycle: Pending (URL issued,
// bytes not yet confirmed in storage) -> Active (confirmed present, with
// size/checksum verified against the real object, not the client's
// unverified claim) -> Deleted (soft-deleted; the object is actually
// removed from storage, the row is kept for audit).
type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusDeleted Status = "deleted"
)

type File struct {
	ID          string
	AppID       string
	OwnerUserID string
	ObjectKey   string
	FileName    string
	MimeType    string
	ByteSize    int64
	Checksum    string // hex MD5, verified against storage at confirm time
	Visibility  Visibility
	Status      Status
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound           = errors.New("file not found")
	ErrForbidden          = errors.New("not permitted to access this file")
	ErrMimeTypeNotAllowed = errors.New("mime type is not allowed")
	ErrSizeLimitExceeded  = errors.New("file exceeds the maximum allowed size")
	ErrChecksumMismatch   = errors.New("uploaded object's checksum does not match the declared checksum")
	ErrNotPending         = errors.New("file is not awaiting upload confirmation")
	ErrNotActive          = errors.New("file is not available for download")
)

type RequestUploadInput struct {
	AppID         string
	FileName      string
	MimeType      string
	SizeBytes     int64
	Checksum      string // optional, hex MD5 the client declares up front
	Visibility    Visibility
	RetentionDays *int
}

func (in RequestUploadInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.FileName) == "":
		return &ValidationError{"fileName is required"}
	case strings.TrimSpace(in.MimeType) == "":
		return &ValidationError{"mimeType is required"}
	case in.SizeBytes <= 0:
		return &ValidationError{"sizeBytes must be positive"}
	case in.Visibility != "" && !in.Visibility.valid():
		return &ValidationError{"visibility must be one of private, public"}
	case in.RetentionDays != nil && *in.RetentionDays <= 0:
		return &ValidationError{"retentionDays must be positive"}
	}
	return nil
}

type ListParams struct {
	Limit  int
	Cursor string
}

type ListResult struct {
	Items      []File
	NextCursor string
}

// Repository is the storage-agnostic boundary for File metadata - never
// binary bytes, which live in ObjectStore instead. Implementations own
// emitting file.uploaded atomically with ConfirmUpload's write, the same
// transactional-outbox pattern as every other domain module.
type Repository interface {
	Create(ctx context.Context, ownerUserID, objectKey string, in RequestUploadInput) (File, error)
	Get(ctx context.Context, id string) (File, error)
	ListForOwner(ctx context.Context, ownerUserID string, params ListParams) (ListResult, error)
	ConfirmUpload(ctx context.Context, id string, actualSize int64, actualChecksum string) (File, error)
	SoftDelete(ctx context.Context, id string) (File, error)
	ListExpired(ctx context.Context, before time.Time) ([]File, error)
}
