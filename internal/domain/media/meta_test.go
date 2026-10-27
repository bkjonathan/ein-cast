package media

import "testing"

func TestParseMeta(t *testing.T) {
	tests := []struct {
		name string
		path string
		want Meta
	}{
		// Movies
		{"dotted release name", "Movie.Name.2019.1080p.BluRay.x264-GROUP.mkv", Meta{Title: "Movie Name", Year: 2019}},
		{"spaces and parentheses", "The Matrix (1999).mp4", Meta{Title: "The Matrix", Year: 1999}},
		{"underscores", "Some_Film_2007_720p.avi", Meta{Title: "Some Film", Year: 2007}},
		{"no year", "Home Video.mp4", Meta{Title: "Home Video"}},
		{"year in the title, then the release year", "Blade.Runner.2049.2017.1080p.WEB-DL.mkv", Meta{Title: "Blade Runner 2049", Year: 2017}},
		{"a title that is a number", "1917.2019.1080p.mkv", Meta{Title: "1917", Year: 2019}},
		{"only a year is a title", "2012.mkv", Meta{Title: "2012"}},
		{"a year-first title with noise", "2012.1080p.BluRay.mkv", Meta{Title: "2012"}},
		{"hyphen kept inside a title", "Spider-Man.Homecoming.2017.2160p.mkv", Meta{Title: "Spider-Man Homecoming", Year: 2017}},
		{"bracketed tags dropped", "[YTS] Inception [2010] [1080p].mp4", Meta{Title: "Inception", Year: 2010}},
		{"noise words in any case", "Film.2020.BLURAY.X265.HEVC.mkv", Meta{Title: "Film", Year: 2020}},
		{"a name that is only noise keeps the name", "1080p.mkv", Meta{Title: "1080p"}},
		{"unicode title", "အိမ်.2021.mp4", Meta{Title: "အိမ်", Year: 2021}},
		{"accents", "Amélie (2001).mkv", Meta{Title: "Amélie", Year: 2001}},
		{"burmese title with release noise", "အိမ်.2021.1080p.WEB-DL.mkv", Meta{Title: "အိမ်", Year: 2021}},
		{"burmese digits are words, not a year", "အိမ်.၂၀၂၁.mp4", Meta{Title: "အိမ် ၂၀၂၁"}},
		{"folders do not change a movie", "Films/Action/Heat.1995.mkv", Meta{Title: "Heat", Year: 1995}},
		{"a year outside the range is not a year", "Film.1850.mkv", Meta{Title: "Film 1850"}},

		// Episodes by marker
		{"SxxExx dotted", "Show.Name.S02E05.720p.HDTV.mkv", Meta{Title: "S02E05", Show: "Show Name", Season: 2, Episode: 5}},
		{"SxxExx with the episode's name", "Show Name - S01E03 - The Pilot.mkv", Meta{Title: "The Pilot", Show: "Show Name", Season: 1, Episode: 3}},
		{"lowercase", "show.s1e2.mkv", Meta{Title: "S01E02", Show: "show", Season: 1, Episode: 2}},
		{"NxNN", "Show - 2x05.mkv", Meta{Title: "S02E05", Show: "Show", Season: 2, Episode: 5}},
		{"NxNN with a title", "Show - 2x05 - Mandala.avi", Meta{Title: "Mandala", Show: "Show", Season: 2, Episode: 5}},
		{"words", "Show Season 3 Episode 12.mp4", Meta{Title: "S03E12", Show: "Show", Season: 3, Episode: 12}},
		{"a resolution is not an episode", "Film.1920x1080.mkv", Meta{Title: "Film 1920x1080"}},
		{"year in the show name", "Show (2019) S01E01.mkv", Meta{Title: "S01E01", Show: "Show", Year: 2019, Season: 1, Episode: 1}},
		{"two episodes in one file counts the first", "Show.S01E01E02.mkv", Meta{Title: "S01E01", Show: "Show", Season: 1, Episode: 1}},

		// Episodes by folder
		{"season folder and a bare number", "Breaking Bad/Season 2/05 - Mandala.mkv", Meta{Title: "Mandala", Show: "Breaking Bad", Season: 2, Episode: 5}},
		{"season folder and Episode N", "Breaking Bad/Season 2/Episode 5.mkv", Meta{Title: "S02E05", Show: "Breaking Bad", Season: 2, Episode: 5}},
		{"short season folder", "Show/S03/E07.mkv", Meta{Title: "S03E07", Show: "Show", Season: 3, Episode: 7}},
		{"show from the folder when the file has none", "Fargo (2014)/Season 1/S01E02.mkv", Meta{Title: "S01E02", Show: "Fargo", Year: 2014, Season: 1, Episode: 2}},
		{"the file's show wins over the folder's", "Downloads/Show.Name.S01E01.mkv", Meta{Title: "S01E01", Show: "Show Name", Season: 1, Episode: 1}},
		{"no seasons at all", "Anime/Anime Name Episode 12.mkv", Meta{Title: "E12", Show: "Anime Name", Episode: 12}},
		{"a bare number outside a season folder is not an episode", "Films/12 Angry Men.1957.mkv", Meta{Title: "12 Angry Men", Year: 1957}},
		{"burmese show before an episode marker", "ဇာတ်လမ်း.S02E05.mkv", Meta{Title: "S02E05", Show: "ဇာတ်လမ်း", Season: 2, Episode: 5}},
		{"burmese show and episode title in folders", "ဇာတ်လမ်း/Season 1/02 - အစ.mkv", Meta{Title: "အစ", Show: "ဇာတ်လမ်း", Season: 1, Episode: 2}},
		{"season folder, but a name with no number", "Show/Season 1/Behind the Scenes.mkv", Meta{Title: "Behind the Scenes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseMeta(tt.path); got != tt.want {
				t.Errorf("ParseMeta(%q) =\n %+v\nwant\n %+v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseMeta_AnyNameWithLettersOrDigitsKeepsATitle(t *testing.T) {
	for _, p := range []string{"a.mkv", "1080p.BluRay.x264.mkv", "x/y/z.mkv", "S01E01.mkv", "2019.mkv", "[tag] a.mkv"} {
		if ParseMeta(p).Title == "" {
			t.Errorf("ParseMeta(%q) has no title: the list would show a blank row", p)
		}
	}
}

func TestMeta_IsEpisode(t *testing.T) {
	for _, tt := range []struct {
		m    Meta
		want bool
	}{
		{Meta{Show: "S", Episode: 1}, true},
		{Meta{Show: "S", Season: 1, Episode: 1}, true},
		{Meta{Show: "", Episode: 1}, false}, // numbered, but no series to put it under
		{Meta{Show: "S"}, false},
		{Meta{Title: "Movie", Year: 2000}, false},
	} {
		if got := tt.m.IsEpisode(); got != tt.want {
			t.Errorf("%+v.IsEpisode() = %v, want %v", tt.m, got, tt.want)
		}
	}
}
