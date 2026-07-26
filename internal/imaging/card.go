package imaging

import (
	"fmt"
	"image/color"
	"math"

	"github.com/fogleman/gg"
	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// parsedFont is the embedded Go Regular typeface, parsed once. Using an embedded
// font keeps card generation self-contained (no font files to ship).
var parsedFont = mustParse()

func mustParse() *opentype.Font {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic(fmt.Sprintf("parse embedded font: %v", err))
	}
	return f
}

func face(px float64) xfont.Face {
	f, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: xfont.HintingFull})
	if err != nil {
		return nil
	}
	return f
}

// GeneratedCard renders a spoiler-free replacement thumbnail at w×h: a dark
// gradient with the show/movie title, an episode tag (e.g. "S02E05"), and a
// small "spoiler hidden" footer. Deterministic and self-contained.
func GeneratedCard(w, h int, title, tag string) ([]byte, error) {
	if w <= 0 || h <= 0 {
		w, h = 680, 1000
	}
	dc := gg.NewContext(w, h)
	fw, fh := float64(w), float64(h)

	grad := gg.NewLinearGradient(0, 0, 0, fh)
	grad.AddColorStop(0, color.RGBA{30, 33, 40, 255})
	grad.AddColorStop(1, color.RGBA{14, 16, 20, 255})
	dc.SetFillStyle(grad)
	dc.DrawRectangle(0, 0, fw, fh)
	dc.Fill()

	// Font sizes are bounded by height too, so wide (16:9) episode thumbs don't
	// get oversized text that collides with the rule the way a 2:3 poster won't.
	titlePx := math.Min(fw/11, fh/6.5)
	tagPx := math.Min(fw/15, fh/10)
	footPx := math.Min(fw/26, fh/22)

	// Accent rule below the title.
	dc.SetColor(color.RGBA{240, 160, 32, 255})
	dc.DrawRectangle(fw*0.5-fw*0.14, fh*0.55, fw*0.28, math.Max(2, fh/360))
	dc.Fill()

	// Title (wrapped, centered above the rule).
	if f := face(titlePx); f != nil {
		dc.SetFontFace(f)
	}
	dc.SetColor(color.RGBA{233, 236, 242, 255})
	dc.DrawStringWrapped(title, fw/2, fh*0.4, 0.5, 0.5, fw*0.84, 1.25, gg.AlignCenter)

	// Episode tag (accent, below the rule).
	if tag != "" {
		if f := face(tagPx); f != nil {
			dc.SetFontFace(f)
		}
		dc.SetColor(color.RGBA{240, 160, 32, 255})
		dc.DrawStringAnchored(tag, fw/2, fh*0.66, 0.5, 0.5)
	}

	// Footer.
	if f := face(footPx); f != nil {
		dc.SetFontFace(f)
	}
	dc.SetColor(color.RGBA{151, 160, 176, 255})
	dc.DrawStringAnchored("spoiler hidden", fw/2, fh*0.9, 0.5, 0.5)

	return encode(dc.Image())
}
