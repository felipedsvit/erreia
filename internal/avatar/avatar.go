package avatar

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/image/draw"

	"github.com/felipedsvit/erreia/internal/storage"
)

const (
	MaxSize     = 5 << 20 // 5 MB
	OutputSize  = 256
	JPEGQuality = 85
)

type Uploader struct {
	store  storage.ObjectStore
	prefix string
}

func NewUploader(store storage.ObjectStore) *Uploader {
	return &Uploader{store: store, prefix: "avatars"}
}

// Upload reads an uploaded image, validates it, resizes to OutputSize and
// stores it under avatars/{userID}/{uuid}.jpg. It returns the storage key.
func (u *Uploader) Upload(ctx context.Context, userID, contentType string, body io.Reader) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("user id required")
	}
	buf, err := io.ReadAll(io.LimitReader(body, MaxSize+1))
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if len(buf) > MaxSize {
		return "", fmt.Errorf("file too large (max %d bytes)", MaxSize)
	}
	ct := strings.ToLower(contentType)
	if !isAllowedMIME(ct) {
		return "", fmt.Errorf("unsupported content type %q (allowed: jpeg, png, webp)", contentType)
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}
	resized := resize(img, OutputSize)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, resized, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return "", fmt.Errorf("encode jpeg: %w", err)
	}
	key := fmt.Sprintf("%s/%s/%s.jpg", u.prefix, userID, uuid.NewString())
	if err := u.store.Put(ctx, key, "image/jpeg", &out); err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}
	return key, nil
}

func isAllowedMIME(ct string) bool {
	switch ct {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return true
	}
	return false
}

func resize(src image.Image, size int) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == h && w <= size {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

// Avoid unused import of png if it's not used; we keep it for completeness
// in case we want to handle PNG decoding specially later.
var _ = png.Decode
