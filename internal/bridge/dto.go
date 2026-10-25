package bridge

import (
	"log/slog"

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

// ServerStateDTO is the HTTP server's lifecycle state. State is one of "idle",
// "starting", "running", "stopping" or "failed".
type ServerStateDTO struct {
	State string `json:"state"`
	Name  string `json:"name"`
	// Port is the port in use while running, otherwise 0.
	Port int `json:"port"`
	// LocalURL reaches the server from this computer; empty unless running.
	LocalURL string `json:"localUrl"`
	// LANURLs reach it from other devices, most likely first; empty if the
	// machine has no usable network.
	LANURLs []string `json:"lanUrls"`
	Error   string   `json:"error"`
}

func toServerStateDTO(s app.ServerStatus) ServerStateDTO {
	lan := s.LANURLs
	if lan == nil {
		lan = []string{} // JSON [] rather than null
	}
	return ServerStateDTO{
		State:    s.State.String(),
		Name:     s.Name,
		Port:     s.Port,
		LocalURL: s.LocalURL,
		LANURLs:  lan,
		Error:    s.Error,
	}
}

// LogAttrDTO is one key/value pair of a log line.
type LogAttrDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// LogLineDTO is one log record. Time is Unix milliseconds (cheap to turn into a
// JS Date); Level is "debug", "info", "warn" or "error".
type LogLineDTO struct {
	Seq     uint64       `json:"seq"`
	Time    int64        `json:"time"`
	Level   string       `json:"level"`
	Source  string       `json:"source"`
	Message string       `json:"message"`
	Attrs   []LogAttrDTO `json:"attrs"`
}

func toLogLineDTO(e app.LogEntry) LogLineDTO {
	attrs := make([]LogAttrDTO, len(e.Attrs)) // non-nil: JSON [] rather than null
	for i, a := range e.Attrs {
		attrs[i] = LogAttrDTO{Key: a.Key, Value: a.Value}
	}
	return LogLineDTO{
		Seq:     e.Seq,
		Time:    e.Time.UnixMilli(),
		Level:   levelName(e.Level),
		Source:  e.Source,
		Message: e.Message,
		Attrs:   attrs,
	}
}

// levelName buckets any slog level into the four the UI filters on.
func levelName(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}
