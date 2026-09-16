package core

import "testing"

const musicFixture = `{
  "contents": {
    "singleColumnBrowseResultsRenderer": {
      "tabs": [{
        "tabRenderer": {
          "content": {
            "sectionListRenderer": {
              "contents": [
                {"musicCarouselShelfRenderer": {"contents": [
                  {"musicTwoRowItemRenderer": {
                    "title": {"runs": [{"text": "Redbone"}]},
                    "subtitle": {"runs": [{"text": "Song • Childish Gambino"}]},
                    "navigationEndpoint": {"watchEndpoint": {"videoId": "Kp7eSUU9oy8"}}
                  }},
                  {"musicTwoRowItemRenderer": {
                    "title": {"runs": [{"text": "Archive Mix"}]},
                    "subtitle": {"runs": [{"text": "Parson James, Ariana Grande"}]},
                    "navigationEndpoint": {"watchEndpoint": {"videoId": "abc12345678", "playlistId": "RDCLAK5uy_k"}}
                  }},
                  {"musicTwoRowItemRenderer": {
                    "title": {"runs": [{"text": "Toxicity"}]},
                    "subtitle": {"runs": [{"text": "Album • System Of A Down"}]},
                    "navigationEndpoint": {"browseEndpoint": {"browseId": "VLMPREb_abc"}}
                  }}
                ]}},
                {"musicShelfRenderer": {"contents": [
                  {"musicResponsiveListItemRenderer": {
                    "flexColumns": [
                      {"musicResponsiveListItemFlexColumnRenderer": {"text": {"runs": [{"text": "Everlong"}]}}},
                      {"musicResponsiveListItemFlexColumnRenderer": {"text": {"runs": [{"text": "Foo Fighters • 294M plays"}]}}}
                    ],
                    "playlistItemData": {"videoId": "eBG7P-K-r1Y"},
                    "fixedColumns": [
                      {"musicResponsiveListItemFixedColumnRenderer": {"text": {"runs": [{"text": "4:11"}]}}}
                    ]
                  }}
                ]}}
              ]
            }
          }
        }
      }]
    }
  }
}`

func TestParseMusicShelves(t *testing.T) {
	items, err := parseMusicShelves([]byte(musicFixture), 10)
	if err != nil {
		t.Fatalf("parseMusicShelves: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d items, want 4", len(items))
	}

	first := items[0]
	if first.ID != "Kp7eSUU9oy8" || first.Title != "Redbone" || first.Uploader != "Childish Gambino" {
		t.Errorf("song = %+v", first)
	}
	if first.PlaylistID != "" {
		t.Errorf("plain song should have no playlist id, got %q", first.PlaylistID)
	}

	mix := items[1]
	if mix.PlaylistID != "RDCLAK5uy_k" {
		t.Errorf("mix playlist id = %q, want RDCLAK5uy_k", mix.PlaylistID)
	}
	if mix.Title != "Archive Mix" || mix.Uploader != "Parson James, Ariana Grande" {
		t.Errorf("mix = %+v", mix)
	}

	album := items[2]
	if album.PlaylistID != "MPREb_abc" {
		t.Errorf("album playlist id = %q, want MPREb_abc", album.PlaylistID)
	}

	liked := items[3]
	if liked.ID != "eBG7P-K-r1Y" || liked.Uploader != "Foo Fighters" {
		t.Errorf("responsive item = %+v", liked)
	}
	if liked.Duration != 251 {
		t.Errorf("duration = %d, want 251", liked.Duration)
	}
	if got := liked.DurationLabel(); got != "4:11" {
		t.Errorf("DurationLabel = %q, want 4:11", got)
	}
}

func TestParseMusicShelvesEmpty(t *testing.T) {
	if _, err := parseMusicShelves([]byte(`{"contents":{}}`), 5); err == nil {
		t.Fatal("expected an error for a response with no music items")
	}
}

func TestDurationLabelUnknown(t *testing.T) {
	r := SearchResult{Duration: 0}
	if got := r.DurationLabel(); got != "" {
		t.Errorf("DurationLabel for unknown duration = %q, want empty", got)
	}
}
