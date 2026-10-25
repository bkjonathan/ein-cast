package bridge

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
