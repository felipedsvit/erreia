package avatar

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// BenchmarkUploadResize measures the cost of decoding a large PNG,
// resizing it, and writing a JPEG. This is the hot path of the avatar
// upload pipeline.
func BenchmarkUploadResize(b *testing.B) {
	// 1024x1024 image: 4 MB raw RGBA, encodes to a few hundred KB PNG.
	img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		b.Fatal(err)
	}
	payload := buf.Bytes()

	u := NewUploader(storeWithPresign{store: newFakeStore()})
	ctx := context.Background()

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := u.Upload(ctx, "user-1", "image/png", bytes.NewReader(payload)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJPEGEncode measures the cost of encoding a resized image.
func BenchmarkJPEGEncode(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, OutputSize, OutputSize))
	for y := 0; y < OutputSize; y++ {
		for x := 0; x < OutputSize; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
			b.Fatal(err)
		}
	}
}
