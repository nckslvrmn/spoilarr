// Package protect is spoilarr's engine: it applies a show's spoiler profile to
// its episodes/seasons (blur art, clear/rewrite descriptions, mask titles, and
// lock the fields) and reverses it exactly on reveal.
package protect

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Thumbnail / description / title modes.
const (
	ThumbBlur      = "blur"
	ThumbSolid     = "solid"
	ThumbGenerated = "generated"
	ThumbLeave     = "leave"

	DescClear    = "clear"
	DescTemplate = "template"
	DescLeave    = "leave"

	TitleKeep    = "keep"
	TitleNumber  = "episode_number"
	TitleLetters = "letters"
	TitleGeneric = "generic"
)

// Profile is the per-show configuration for how spoilers are hidden. The zero
// value is not meaningful; use DefaultProfile then override.
type Profile struct {
	Thumbnail    string  `json:"thumbnail"`
	BlurSigma    float64 `json:"blur_sigma"`
	Description  string  `json:"description"`
	DescTemplate string  `json:"desc_template"`
	TitleMode    string  `json:"title_mode"`
	SeasonPoster bool    `json:"season_poster"`
}

// DefaultProfile is what a freshly-protected show gets: blurred art, cleared
// descriptions, episode titles reduced to their number, season posters blurred.
func DefaultProfile() Profile {
	return Profile{
		Thumbnail:    ThumbBlur,
		BlurSigma:    20,
		Description:  DescClear,
		DescTemplate: "Hidden to avoid spoilers.",
		TitleMode:    TitleNumber,
		SeasonPoster: true,
	}
}

// ParseProfile decodes a stored profile JSON blob, falling back to defaults for
// an empty/missing value and for any field left unset.
func ParseProfile(blob string) Profile {
	p := DefaultProfile()
	if strings.TrimSpace(blob) == "" || blob == "{}" {
		return p
	}
	_ = json.Unmarshal([]byte(blob), &p)
	if p.BlurSigma <= 0 {
		p.BlurSigma = 20
	}
	return p
}

// MaskTitle produces the replacement episode title for the configured mode.
// index is the episode number within its season.
func (p Profile) MaskTitle(original string, index int) string {
	switch p.TitleMode {
	case TitleNumber:
		return fmt.Sprintf("Episode %d", index)
	case TitleLetters:
		return "Episode " + letters(index)
	case TitleGeneric:
		return "Episode"
	default: // TitleKeep
		return original
	}
}

// MaskDescription produces the replacement summary for the configured mode.
func (p Profile) MaskDescription(show string, season, episode int) string {
	switch p.Description {
	case DescClear:
		return ""
	case DescTemplate:
		r := strings.NewReplacer(
			"{show}", show,
			"{season}", fmt.Sprintf("%d", season),
			"{episode}", fmt.Sprintf("%d", episode),
		)
		return r.Replace(p.DescTemplate)
	default: // DescLeave
		return "" // caller checks WantsDescriptionChange first
	}
}

// letters maps 1->A, 2->B, ... 26->Z, 27->AA (spreadsheet-style).
func letters(n int) string {
	if n <= 0 {
		return ""
	}
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('A' + n%26)}, b...)
		n /= 26
	}
	return string(b)
}
