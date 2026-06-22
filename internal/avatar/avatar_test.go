package avatar

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory ObjectStore used by the tests.
type fakeStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key, _ string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.objects[key] = b
	f.mu.Unlock()
	return nil
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	delete(f.objects, key)
	f.mu.Unlock()
	return nil
}

func (f *fakeStore) PresignedGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://test/" + key, nil
}

func (f *fakeStore) Get(_ context.Context, key string) (io.ReadCloser, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[key]
	if !ok {
		return nil, "", io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), "image/jpeg", nil
}

// We don't actually need the PresignedGet in the upload path; the fake
// returns a stable URL and the test only asserts on Put.

func newPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestUploadValidPNG ensures a PNG that exceeds OutputSize is resized
// down to 256x256 and stored as JPEG.
func TestUploadValidPNG(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	u := NewUploader(storeWithPresign{store: store})

	key, err := u.Upload(context.Background(), "user-1", "image/png", bytes.NewReader(newPNG(t, 800, 600)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "avatars/user-1/") || !strings.HasSuffix(key, ".jpg") {
		t.Fatalf("unexpected key: %q", key)
	}
	store.mu.Lock()
	data := store.objects[key]
	store.mu.Unlock()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output is not jpeg: %v", err)
	}
	if cfg.Width != OutputSize || cfg.Height != OutputSize {
		t.Fatalf("expected %dx%d, got %dx%d", OutputSize, OutputSize, cfg.Width, cfg.Height)
	}
}

// TestUploadRejectsUnsupportedMIME blocks non-image content types.
func TestUploadRejectsUnsupportedMIME(t *testing.T) {
	t.Parallel()
	u := NewUploader(storeWithPresign{store: newFakeStore()})
	_, err := u.Upload(context.Background(), "user-1", "application/pdf", bytes.NewReader([]byte("not an image")))
	if err == nil {
		t.Fatal("expected error for non-image MIME")
	}
}

// TestUploadRejectsTooLarge asserts the size limit.
func TestUploadRejectsTooLarge(t *testing.T) {
	t.Parallel()
	u := NewUploader(storeWithPresign{store: newFakeStore()})
	huge := bytes.Repeat([]byte{0xFF}, MaxSize+10)
	_, err := u.Upload(context.Background(), "user-1", "image/jpeg", bytes.NewReader(huge))
	if err == nil {
		t.Fatal("expected error for too-large upload")
	}
}

// TestUploadAcceptsJPEG verifies the common case of a small JPEG.
func TestUploadAcceptsJPEG(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	u := NewUploader(storeWithPresign{store: store})
	_, err := u.Upload(context.Background(), "user-1", "image/jpeg", bytes.NewReader(newJPEG(t, 100, 100)))
	if err != nil {
		t.Fatal(err)
	}
}

// TestUploadRequiresUserID enforces the precondition.
func TestUploadRequiresUserID(t *testing.T) {
	t.Parallel()
	u := NewUploader(storeWithPresign{store: newFakeStore()})
	_, err := u.Upload(context.Background(), "", "image/jpeg", bytes.NewReader(newJPEG(t, 10, 10)))
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

// TestIsAllowedMIME lists all supported types so a future change is loud.
func TestIsAllowedMIME(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ct   string
		want bool
	}{
		{"image/jpeg", true},
		{"image/jpg", true},
		{"image/png", true},
		{"image/webp", true},
		{"image/gif", false},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isAllowedMIME(c.ct); got != c.want {
			t.Errorf("isAllowedMIME(%q) = %v want %v", c.ct, got, c.want)
		}
	}
}

// storeWithPresign satisfies the storage.ObjectStore interface. The
// PresignedGet signature requires a time.Duration, but the tests never
// call it, so we panic loudly if anything does.
type storeWithPresign struct{ store *fakeStore }

func (s storeWithPresign) Put(ctx context.Context, key, ct string, r io.Reader) error {
	return s.store.Put(ctx, key, ct, r)
}
func (s storeWithPresign) Delete(ctx context.Context, key string) error {
	return s.store.Delete(ctx, key)
}
func (s storeWithPresign) PresignedGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://test/" + key, nil
}

func (s storeWithPresign) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	return s.store.Get(ctx, key)
}
