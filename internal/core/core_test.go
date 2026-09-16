package core

import "testing"

func TestParseResults(t *testing.T) {
	out := []byte("abc123||Some Video||245||Channel Name\n" +
		"def456||Second Video||-1||Other Channel\n" +
		"broken-line\n" +
		"ghi789||Third Video||60||Third Channel\n")

	results, err := parseResults(out)
	if err != nil {
		t.Fatalf("parseResults returned error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	first := results[0]
	if first.ID != "abc123" {
		t.Errorf("ID = %q, want %q", first.ID, "abc123")
	}
	if first.Title != "Some Video" {
		t.Errorf("Title = %q, want %q", first.Title, "Some Video")
	}
	if first.URL != "https://www.youtube.com/watch?v=abc123" {
		t.Errorf("URL = %q", first.URL)
	}
	if first.Duration != 245 {
		t.Errorf("Duration = %d, want 245", first.Duration)
	}
	if first.Uploader != "Channel Name" {
		t.Errorf("Uploader = %q, want %q", first.Uploader, "Channel Name")
	}

	if results[1].Duration != -1 {
		t.Errorf("unparseable duration should fall back to -1, got %d", results[1].Duration)
	}
}

func TestDurationString(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{0, "0:00"},
		{59, "0:59"},
		{60, "1:00"},
		{125, "2:05"},
	}
	for _, tt := range tests {
		r := SearchResult{Duration: tt.seconds}
		if got := r.DurationString(); got != tt.want {
			t.Errorf("DurationString(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestSanitizeTitle(t *testing.T) {
	cases := map[string]string{
		"Lofi hip hop":                       "Lofi hip hop",
		"cafe (crying)" + "\U0001F62D":       "cafe (crying)",
		"vibes " + "\U00002614":              "vibes",
		"\U0001F9D1\U0000200D\U0001F4BB dev": "dev",
		"trail \U0000FE0F token":             "trail  token",
		"  spaced  \t \n":                    "spaced",
	}
	for in, want := range cases {
		if got := sanitizeTitle(in); got != want {
			t.Errorf("sanitizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQualityFormats(t *testing.T) {
	cases := []struct {
		quality   string
		audioOnly bool
		wantFirst string
	}{
		{"auto", false, "b"},
		{"720p", false, "b[height<=720]/b"},
		{"1080p", false, "b[height<=1080]/b"},
		{"auto", true, "bestaudio"},
	}
	for _, c := range cases {
		got := qualityFormats(c.quality, c.audioOnly)
		if len(got) == 0 || got[0] != c.wantFirst {
			t.Fatalf("qualityFormats(%q, %v)[0] = %v, want %q", c.quality, c.audioOnly, got, c.wantFirst)
		}
	}
}

func TestSetQuality(t *testing.T) {
	g := &GeckoCore{}
	g.SetQuality("480p")
	if g.Quality() != "480p" {
		t.Fatalf("Quality() = %q, want 480p", g.Quality())
	}
	g.SetQuality("nonsense")
	if g.Quality() != "auto" {
		t.Fatalf("Quality() = %q, want auto fallback", g.Quality())
	}
}
