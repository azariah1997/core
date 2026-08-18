package files_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/files"
	"github.com/example/core-platform/backend/core-api/internal/files/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

// fakeStore behaves like a real object store for test purposes: PresignPut
// "uploads" nothing (the test simulates the client's PUT separately via
// put()), but HeadObject/DeleteObject only see what's actually been put,
// so a confirm against an object that was never uploaded genuinely fails -
// the same property the real S3 adapter has.
type fakeStore struct {
	objects map[string]fakeObject
	deleted []string
}

type fakeObject struct {
	size     int64
	checksum string
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string]fakeObject{}} }

func (s *fakeStore) put(objectKey string, size int64, checksum string) {
	s.objects[objectKey] = fakeObject{size: size, checksum: checksum}
}

func (s *fakeStore) PresignPut(ctx context.Context, objectKey, contentType string, expires time.Duration) (string, error) {
	return "https://fake-storage.local/put/" + objectKey, nil
}

func (s *fakeStore) PresignGet(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	return "https://fake-storage.local/get/" + objectKey, nil
}

func (s *fakeStore) HeadObject(ctx context.Context, objectKey string) (int64, string, error) {
	obj, ok := s.objects[objectKey]
	if !ok {
		return 0, "", errors.New("object not found in storage")
	}
	return obj.size, obj.checksum, nil
}

func (s *fakeStore) DeleteObject(ctx context.Context, objectKey string) error {
	if _, ok := s.objects[objectKey]; !ok {
		return errors.New("object not found in storage")
	}
	delete(s.objects, objectKey)
	s.deleted = append(s.deleted, objectKey)
	return nil
}

func newService(store *fakeStore, admins map[string]bool) *files.Service {
	return files.NewService(memory.New(), store, fakeAdmin{admins: admins}, files.Config{})
}

func TestRequestUploadRejectsOversizedFile(t *testing.T) {
	svc := files.NewService(memory.New(), newFakeStore(), fakeAdmin{}, files.Config{MaxSizeBytes: 100})
	_, _, _, err := svc.RequestUpload(context.Background(), "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "big.zip", MimeType: "application/zip", SizeBytes: 200,
	})
	if !errors.Is(err, files.ErrSizeLimitExceeded) {
		t.Fatalf("expected ErrSizeLimitExceeded, got %v", err)
	}
}

func TestRequestUploadRejectsDisallowedMimeType(t *testing.T) {
	svc := files.NewService(memory.New(), newFakeStore(), fakeAdmin{}, files.Config{AllowedMimePrefixes: []string{"image/"}})
	_, _, _, err := svc.RequestUpload(context.Background(), "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "script.exe", MimeType: "application/x-msdownload", SizeBytes: 100,
	})
	if !errors.Is(err, files.ErrMimeTypeNotAllowed) {
		t.Fatalf("expected ErrMimeTypeNotAllowed, got %v", err)
	}
}

func TestRequestUploadAllowsMatchingMimePrefix(t *testing.T) {
	svc := files.NewService(memory.New(), newFakeStore(), fakeAdmin{}, files.Config{AllowedMimePrefixes: []string{"image/"}})
	f, uploadURL, _, err := svc.RequestUpload(context.Background(), "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "photo.png", MimeType: "image/png", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	if f.Status != files.StatusPending || uploadURL == "" {
		t.Fatalf("unexpected result: %+v url=%q", f, uploadURL)
	}
}

func TestConfirmUploadVerifiesRealSizeNotClientClaim(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()

	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 999, // client's claim
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	// The real object in storage is a different size than the client claimed.
	store.put(f.ObjectKey, 42, "abc123")

	confirmed, err := svc.ConfirmUpload(ctx, "u1", f.ID)
	if err != nil {
		t.Fatalf("confirm upload: %v", err)
	}
	if confirmed.ByteSize != 42 {
		t.Fatalf("expected confirmed size to come from storage (42), got %d", confirmed.ByteSize)
	}
	if confirmed.Status != files.StatusActive {
		t.Fatalf("expected status active, got %v", confirmed.Status)
	}
}

func TestConfirmUploadRejectsChecksumMismatch(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()

	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 100, Checksum: "expected-checksum",
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	store.put(f.ObjectKey, 100, "actually-different-checksum")

	_, err = svc.ConfirmUpload(ctx, "u1", f.ID)
	if !errors.Is(err, files.ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	}
}

func TestConfirmUploadFailsIfObjectNeverUploaded(t *testing.T) {
	svc := newService(newFakeStore(), nil)
	ctx := context.Background()
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	// Nothing was ever put into the fake store for this object key.
	if _, err := svc.ConfirmUpload(ctx, "u1", f.ID); err == nil {
		t.Fatal("expected confirm to fail when the object was never actually uploaded")
	}
}

func TestConfirmUploadIsIdempotent(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	store.put(f.ObjectKey, 100, "checksum")
	if _, err := svc.ConfirmUpload(ctx, "u1", f.ID); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	if _, err := svc.ConfirmUpload(ctx, "u1", f.ID); err != nil {
		t.Fatalf("expected re-confirming an already-active file to be a no-op, got %v", err)
	}
}

func TestPrivateFileNotVisibleToStranger(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "secret.pdf", MimeType: "application/pdf", SizeBytes: 100, Visibility: files.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	if _, err := svc.Get(ctx, "stranger", f.ID); !errors.Is(err, files.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestPublicFileVisibleToAnyone(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "public.pdf", MimeType: "application/pdf", SizeBytes: 100, Visibility: files.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	if _, err := svc.Get(ctx, "stranger", f.ID); err != nil {
		t.Fatalf("expected a public file to be visible to a stranger, got %v", err)
	}
}

func TestDownloadURLRequiresActiveFile(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	// Still pending - never confirmed.
	if _, _, err := svc.GetDownloadURL(ctx, "u1", f.ID); !errors.Is(err, files.ErrNotActive) {
		t.Fatalf("expected ErrNotActive for a pending file, got %v", err)
	}
}

func TestDeleteRemovesFromStorageAndSoftDeletesRow(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	store.put(f.ObjectKey, 100, "checksum")
	if _, err := svc.ConfirmUpload(ctx, "u1", f.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if err := svc.Delete(ctx, "u1", f.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != f.ObjectKey {
		t.Fatalf("expected the object to actually be removed from storage, got %v", store.deleted)
	}
	deleted, err := svc.Get(ctx, "u1", f.ID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if deleted.Status != files.StatusDeleted {
		t.Fatalf("expected status deleted, got %v", deleted.Status)
	}
}

func TestNonOwnerCannotDelete(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	ctx := context.Background()
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	if err := svc.Delete(ctx, "stranger", f.ID); !errors.Is(err, files.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestPurgeExpiredRequiresAdmin(t *testing.T) {
	svc := newService(newFakeStore(), nil)
	if _, err := svc.PurgeExpired(context.Background(), "u1"); !errors.Is(err, files.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}
}

// TestPurgeExpiredLeavesNotYetExpiredFilesAlone covers the "not expired
// yet" half of retention. The "actually deletes an expired file" half
// needs a past ExpiresAt, which RequestUpload's RetentionDays (always
// future-relative-to-now) can't produce - that path is exercised in this
// phase's live validation instead, by setting expires_at directly via SQL
// against real Postgres.
func TestPurgeExpiredLeavesNotYetExpiredFilesAlone(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, map[string]bool{"admin": true})
	ctx := context.Background()

	retention := 1
	f, _, _, err := svc.RequestUpload(ctx, "u1", files.RequestUploadInput{
		AppID: "app-1", FileName: "doc.pdf", MimeType: "application/pdf", SizeBytes: 100, RetentionDays: &retention,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	store.put(f.ObjectKey, 100, "checksum")
	if _, err := svc.ConfirmUpload(ctx, "u1", f.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	count, err := svc.PurgeExpired(ctx, "admin")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected nothing expired yet, purged %d", count)
	}
}
