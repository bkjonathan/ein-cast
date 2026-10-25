package media

// Stats summarizes a set of items for display.
type Stats struct {
	Total     int
	TotalSize int64
	// Counts holds the number of items per kind. Kinds with no items are
	// absent, so read it with the zero value in mind.
	Counts map[Kind]int
}

// Summarize totals up items.
func Summarize(items []Item) Stats {
	s := Stats{Counts: make(map[Kind]int)}
	for _, it := range items {
		s.Total++
		s.TotalSize += it.Size
		s.Counts[it.Kind]++
	}
	return s
}
