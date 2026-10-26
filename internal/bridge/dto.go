package bridge

import (
	"log/slog"

	"eincast/internal/app"
	"eincast/internal/app/portfree"
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

// LibraryStateDTO is one library as the UI shows it: what it is, what is in it,
// and what is happening to it. Root is the folder the person picked (shown in
// the UI); individual file paths never cross the bridge.
type LibraryStateDTO struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Root      string        `json:"root"`
	Total     int           `json:"total"`
	TotalSize int64         `json:"totalSize"`
	Counts    KindCountsDTO `json:"counts"`
	// Scanning is true while a scan the person asked for is running.
	Scanning bool `json:"scanning"`
	// Error is a short, user-facing reason the last scan failed; empty if it did not.
	Error string `json:"error"`
	// Watching is true while the folder is watched for changes.
	Watching bool `json:"watching"`
}

func toLibraryStateDTO(s app.LibraryState) LibraryStateDTO {
	return LibraryStateDTO{
		ID:        string(s.Library.ID),
		Name:      s.Library.Name,
		Root:      s.Library.Root,
		Total:     s.Stats.Total,
		TotalSize: s.Stats.TotalSize,
		Counts: KindCountsDTO{
			Video: s.Stats.Counts[media.KindVideo],
			Audio: s.Stats.Counts[media.KindAudio],
			Image: s.Stats.Counts[media.KindImage],
		},
		Scanning: s.Scanning,
		Error:    s.Error,
		Watching: s.Watching,
	}
}

// toLibraryStateDTOs maps every library, as a non-nil slice (JSON [] not null).
func toLibraryStateDTOs(in []app.LibraryState) []LibraryStateDTO {
	out := make([]LibraryStateDTO, len(in))
	for i, s := range in {
		out[i] = toLibraryStateDTO(s)
	}
	return out
}

// LibraryItemDTO is one file in the library. It has no absolute path (see
// ADR 0005); Kind is "video", "audio" or "image", ModTime is Unix milliseconds.
type LibraryItemDTO struct {
	ID string `json:"id"`
	// LibraryID is the library the file is in.
	LibraryID string `json:"libraryId"`
	Name      string `json:"name"`
	RelPath   string `json:"relPath"`
	Kind      string `json:"kind"`
	Size      int64  `json:"size"`
	ModTime   int64  `json:"modTime"`
}

func toLibraryItemDTOs(items []media.Item) []LibraryItemDTO {
	out := make([]LibraryItemDTO, len(items)) // non-nil: JSON [] rather than null
	for i, it := range items {
		out[i] = LibraryItemDTO{
			ID:        string(it.ID),
			LibraryID: string(it.LibraryID),
			Name:      it.Name,
			RelPath:   it.RelPath,
			Kind:      string(it.Kind),
			Size:      it.Size,
			ModTime:   it.ModTime.UnixMilli(),
		}
	}
	return out
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
	// NameURLs are friendlier addresses announced on the network, such as
	// http://eincast.local:8765; empty when none is.
	NameURLs []string `json:"nameUrls"`
	// Discovery lists what is announcing the server: "mdns", "dlna".
	Discovery []string `json:"discovery"`
	Error     string   `json:"error"`
}

func toServerStateDTO(s app.ServerStatus) ServerStateDTO {
	lan := s.LANURLs
	if lan == nil {
		lan = []string{} // JSON [] rather than null
	}
	return ServerStateDTO{
		State:     s.State.String(),
		Name:      s.Name,
		Port:      s.Port,
		LocalURL:  s.LocalURL,
		LANURLs:   lan,
		NameURLs:  nonNil(s.NameURLs),
		Discovery: nonNil(s.Discovery),
		Error:     s.Error,
	}
}

// RemoteStatusDTO is remote access (a tunnel) as the UI shows it. State is one
// of "idle", "starting", "running", "stopping" or "failed", like the server's.
type RemoteStatusDTO struct {
	State string `json:"state"`
	// Provider is the provider in use; empty when none is.
	Provider string `json:"provider"`
	// Public is true while anyone on the Internet might reach the library. It
	// drives the "Public access ON" badge, so it stays true until a stop is
	// confirmed.
	Public bool `json:"public"`
	// URL is the remote address while running.
	URL   string `json:"url"`
	Error string `json:"error"`
	// Live is the provider's own state: "running" while connected, anything
	// else while it reconnects.
	Live string `json:"live"`
	// PinSet says whether an access PIN exists.
	PinSet bool `json:"pinSet"`
	// Sessions counts remote browsers currently signed in.
	Sessions int `json:"sessions"`
	// NeedsSetup lists the providers that cannot be turned on until the person
	// has supplied what they need.
	NeedsSetup []string `json:"needsSetup"`
}

func toRemoteStatusDTO(s app.RemoteStatus) RemoteStatusDTO {
	return RemoteStatusDTO{
		State:    s.State.String(),
		Provider: string(s.Provider),
		Public:   s.Public,
		URL:      string(s.URL),
		Error:    s.Error,
		Live:     s.Live.State.String(),
		PinSet:   s.PINSet,
		Sessions: s.Sessions,

		NeedsSetup: providerIDs(s.NeedsSetup),
	}
}

func providerIDs(in []app.ProviderID) []string {
	out := make([]string, len(in)) // non-nil: JSON [] rather than null
	for i, id := range in {
		out[i] = string(id)
	}
	return out
}

// RemoteProviderDTO is one way of reaching the library from outside. Names and
// explanations are the UI's to write; Go only says what each provider is.
type RemoteProviderDTO struct {
	ID     string `json:"id"`
	Public bool   `json:"public"`
	// Fields are what the provider needs from the person; empty if nothing.
	Fields []RemoteFieldDTO `json:"fields"`
}

// RemoteFieldDTO describes one setting a provider needs. Kind is "text",
// "secret" or "file".
type RemoteFieldDTO struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Help        string `json:"help"`
	Placeholder string `json:"placeholder"`
	Kind        string `json:"kind"`
	Required    bool   `json:"required"`
}

// RemoteFieldStateDTO says whether a setting is saved. Value is empty for
// secrets, always: they are not sent back out.
type RemoteFieldStateDTO struct {
	Key   string `json:"key"`
	Set   bool   `json:"set"`
	Value string `json:"value"`
}

func toRemoteProviderDTOs(in []app.ProviderInfo) []RemoteProviderDTO {
	out := make([]RemoteProviderDTO, len(in)) // non-nil: JSON [] rather than null
	for i, p := range in {
		fields := make([]RemoteFieldDTO, len(p.Fields))
		for j, f := range p.Fields {
			fields[j] = RemoteFieldDTO{
				Key: f.Key, Label: f.Label, Help: f.Help, Placeholder: f.Placeholder,
				Kind: string(f.Kind), Required: f.Required,
			}
		}
		out[i] = RemoteProviderDTO{ID: string(p.ID), Public: p.Public, Fields: fields}
	}
	return out
}

func toRemoteFieldStateDTOs(in []app.FieldState) []RemoteFieldStateDTO {
	out := make([]RemoteFieldStateDTO, len(in))
	for i, st := range in {
		out[i] = RemoteFieldStateDTO{Key: st.Field.Key, Set: st.Set, Value: st.Value}
		if st.Field.Kind == app.FieldSecret {
			out[i].Value = "" // belt and braces: a secret never crosses the bridge
		}
	}
	return out
}

// SettingsDTO is what the Settings screen shows. AccessToken is empty unless the
// token is switched on; TokenPersisted is false when it could not be saved to
// the system credential store.
type SettingsDTO struct {
	Port           int    `json:"port"`
	Name           string `json:"name"`
	RequireToken   bool   `json:"requireToken"`
	DLNA           bool   `json:"dlna"`
	AccessToken    string `json:"accessToken"`
	TokenPersisted bool   `json:"tokenPersisted"`
}

func toSettingsDTO(v app.SettingsView) SettingsDTO {
	return SettingsDTO{
		Port:           v.Port,
		Name:           v.Name,
		RequireToken:   v.RequireToken,
		DLNA:           v.DLNA,
		AccessToken:    v.Token,
		TokenPersisted: v.TokenPersisted,
	}
}

// SettingsInputDTO is what the Settings form sends back. The library folder and
// the token itself are not editable here.
type SettingsInputDTO struct {
	Port         int    `json:"port"`
	Name         string `json:"name"`
	RequireToken bool   `json:"requireToken"`
	DLNA         bool   `json:"dlna"`
}

// PortHolderDTO is a program using a port. Self marks EinCast's own process,
// which the UI shows but offers no way to end.
type PortHolderDTO struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	Self bool   `json:"self"`
}

func toPortHolderDTOs(in []portfree.Holder) []PortHolderDTO {
	out := make([]PortHolderDTO, len(in)) // non-nil: JSON [] rather than null
	for i, h := range in {
		out[i] = PortHolderDTO{PID: h.PID, Name: h.Name, Self: h.Self}
	}
	return out
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

// nonNil makes a nil slice an empty one, so JSON carries [] rather than null.
func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
