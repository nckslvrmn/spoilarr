// Package imaging transforms artwork for spoiler protection: blurring a poster,
// filling it with a solid color, or generating a replacement card. Everything is
// encoded back to JPEG, which is what Plex serves for posters/thumbs and what its
// upload endpoint expects.
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // register JPEG decoder for DecodeConfig
	_ "image/png"  // register PNG decoder for DecodeConfig

	"github.com/disintegration/imaging"
)

const JPEGQuality = 90

// Blur returns a Gaussian-blurred JPEG. sigma is blur strength; around 15-30
// obscures detail while keeping the overall shape so the tile still looks
// intentional.
func Blur(src []byte, sigma float64) ([]byte, error) {
	img, err := imaging.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	// Downscaling first is cheaper and gives a smoother frosted look than
	// blurring at full poster resolution.
	small := imaging.Resize(img, 300, 0, imaging.Linear)
	blurred := imaging.Blur(small, sigma)
	return encode(blurred)
}

// SolidColor keeps src's dimensions so Plex keeps the right aspect ratio.
func SolidColor(src []byte, c color.Color) ([]byte, error) {
	img, err := imaging.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	b := img.Bounds()
	filled := imaging.New(b.Dx(), b.Dy(), c)
	return encode(filled)
}

// DimensionsOf reads the size without a full decode, falling back to a 2:3
// poster shape on error.
func DimensionsOf(src []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil || cfg.Width == 0 {
		return 680, 1000
	}
	return cfg.Width, cfg.Height
}

func encode(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(JPEGQuality)); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), nil
}
