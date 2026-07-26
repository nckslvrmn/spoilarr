// Package protect is spoilarr's engine: it applies a show's spoiler profile to
// its episodes/seasons (blur art, clear/rewrite descriptions, mask titles, and
// lock the fields) and reverses it exactly on reveal.
package protect

import (
	"encoding/json"
	"fmt"
	"strings"
)

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

// Profile is how spoilers are hidden. The zero value isn't meaningful; start
// from DefaultProfile.
type Profile struct {
	Thumbnail    string  `json:"thumbnail"`
	BlurSigma    float64 `json:"blur_sigma"`
	Description  string  `json:"description"`
	DescTemplate string  `json:"desc_template"`
	TitleMode    string  `json:"title_mode"`
	SeasonPoster bool    `json:"season_poster"`
}

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

// ParseProfile starts from DefaultProfile, so an empty blob or any unset field
// falls back to the defaults.
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

// MaskTitle's index is the episode number within its season.
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
	default:
		return ""
	}
}

// letters maps 1->A, 2->B, ... 26->Z, 27->AA, spreadsheet-style.
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
