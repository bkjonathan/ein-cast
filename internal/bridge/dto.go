package bridge

import (
	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// AppInfoDTO is what the frontend sees about the running app.
//
// DTOs are the anti-corruption layer between Go and JS: they are flat,
// JSON-safe, and change only when the UI needs them to, never because a
// domain type was refactored.
type AppInfoDTO struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

// KindCountsDTO is a fixed shape (not a map) so TypeScript gets exact keys.
type KindCountsDTO struct {
	Video int `json:"video"`
	Audio int `json:"audio"`
	Image int `json:"image"`
}

// LibrarySummaryDTO describes the loaded library. Root is the folder the user
// picked (shown in the UI); individual file paths never cross the bridge.
type LibrarySummaryDTO struct {
	Root      string        `json:"root"`
	Total     int           `json:"total"`
	TotalSize int64         `json:"totalSize"`
	Counts    KindCountsDTO `json:"counts"`
}

func toLibrarySummaryDTO(s app.LibrarySummary) LibrarySummaryDTO {
	return LibrarySummaryDTO{
		Root:      s.Root,
		Total:     s.Stats.Total,
		TotalSize: s.Stats.TotalSize,
		Counts: KindCountsDTO{
			Video: s.Stats.Counts[media.KindVideo],
			Audio: s.Stats.Counts[media.KindAudio],
			Image: s.Stats.Counts[media.KindImage],
		},
	}
}
