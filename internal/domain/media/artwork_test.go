package media

import "testing"

func TestMatchArtwork(t *testing.T) {
	tests := []struct {
		name        string
		video       string
		folder      []string
		folderLevel bool
		want        string
	}{
		{"the video's own poster wins", "Heat.mkv", []string{"Heat.mkv", "poster.jpg", "Heat-poster.jpg"}, true, "Heat-poster.jpg"},
		{"a picture named like the video", "Heat.mkv", []string{"Heat.mkv", "heat.JPG"}, false, "heat.JPG"},
		{"the folder's poster, when it is this video's", "Heat.mkv", []string{"Heat.mkv", "poster.jpg"}, true, "poster.jpg"},
		{"the folder's poster is nobody's in a folder of many", "Heat.mkv", []string{"Heat.mkv", "Ronin.mkv", "poster.jpg"}, false, ""},
		{"folder, then cover", "a.mkv", []string{"cover.png", "folder.jpg"}, true, "folder.jpg"},
		{"poster before folder before cover", "a.mkv", []string{"cover.jpg", "folder.jpg", "poster.jpg"}, true, "poster.jpg"},
		{"jpg before png", "a.mkv", []string{"poster.png", "poster.jpg"}, true, "poster.jpg"},
		{"case is ignored", "a.mkv", []string{"POSTER.PNG"}, true, "POSTER.PNG"},
		{"another video's poster is not ours", "Heat.mkv", []string{"Ronin-poster.jpg"}, true, ""},
		{"a photo is not a poster", "Heat.mkv", []string{"IMG_0001.jpg", "holiday.png"}, true, ""},
		{"an unsupported picture type", "Heat.mkv", []string{"poster.gif", "poster.webp"}, true, ""},
		{"nothing at all", "Heat.mkv", nil, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MatchArtwork(tt.video, tt.folder, tt.folderLevel)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("MatchArtwork(%q, %v, %v) = %q, %v; want %q", tt.video, tt.folder, tt.folderLevel, got, ok, tt.want)
			}
		})
	}
}

func TestIsArtworkName(t *testing.T) {
	for name, want := range map[string]bool{
		"poster.jpg": true, "Folder.PNG": true, "cover.jpeg": true, "Heat-poster.jpg": true,
		"IMG_1.jpg": false, "poster.gif": false, "poster": false, "poster.jpg.txt": false, "movie.jpg": false,
	} {
		if got := IsArtworkName(name); got != want {
			t.Errorf("IsArtworkName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsSeasonFolder(t *testing.T) {
	for name, want := range map[string]bool{
		"Season 1": true, "season.02": true, "S03": true, "Series 4": true, "Staffel 1": true,
		"Seasonal": false, "Specials": false, "Show": false, "S": false, "Sea": false,
	} {
		if got := IsSeasonFolder(name); got != want {
			t.Errorf("IsSeasonFolder(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestItem_DisplayName(t *testing.T) {
	if got := (Item{Name: "a.mkv"}).DisplayName(); got != "a.mkv" {
		t.Errorf("DisplayName() = %q, want the file name when there is no title", got)
	}
	if got := (Item{Name: "a.mkv", Meta: Meta{Title: "A"}}).DisplayName(); got != "A" {
		t.Errorf("DisplayName() = %q", got)
	}
}
