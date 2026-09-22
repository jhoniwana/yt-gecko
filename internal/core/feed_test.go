package core

import (
	"regexp"
	"strings"
	"testing"
)

func TestParseLockupFeed(t *testing.T) {
	raw := []byte(`{
  "contents": {
    "richGridRenderer": {
      "contents": [
        {"richItemRenderer":{"content":{
          "lockupViewModel": {
            "contentId": "abcDEF12345",
            "contentImage": {"thumbnailViewModel":{"overlays":[
              {"thumbnailBottomOverlayViewModel":{"badges":[
                {"thumbnailBadgeViewModel":{"text":"12:03"}}
              ]}}
            ]}},
            "metadata": {"lockupMetadataViewModel": {
              "title": {"content": "Biggest Hits of 2026 - Official"},
              "image": {"decoratedAvatarViewModel":{"a11yLabel":"Go to channel RealArtist"}}
            }}
          }
        }}},
        {"richItemRenderer":{"content":{
          "lockupViewModel": {
            "contentId": "nope", "contentImage": {}, "metadata": {}
          }
        }}},
        {"richItemRenderer":{"content":{
          "lockupViewModel": {
            "contentId": "zzzYYY00011",
            "contentImage": {"thumbnailViewModel":{"overlays":[
              {"thumbnailBottomOverlayViewModel":{"badges":[
                {"thumbnailBadgeViewModel":{"text":"1:02:59"}}
              ]}}
            ]}},
            "metadata": {"lockupMetadataViewModel": {
              "title": {"content": "Long Video"},
              "image": {"decoratedAvatarViewModel":{"a11yLabel":"Go to channel Chan"}}
            }}
          }
        }}}
      ]
    }
  }
}`)
	got, err := parseLockupFeed(raw, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2 (the one without contentId must be skipped): %v", len(got), got)
	}
	wants := []SearchResult{
		{ID: "abcDEF12345", Title: "Biggest Hits of 2026 - Official", URL: "https://www.youtube.com/watch?v=abcDEF12345", Duration: 723, Uploader: "RealArtist"},
		{ID: "zzzYYY00011", Title: "Long Video", URL: "https://www.youtube.com/watch?v=zzzYYY00011", Duration: 3779, Uploader: "Chan"},
	}
	for i, w := range wants {
		if got[i] != w {
			t.Errorf("result %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestParseLockupFeedLimit(t *testing.T) {
	var raw strings.Builder
	raw.WriteString(`{"contents":{"x":[`)
	for i := 0; i < 5; i++ {
		if i > 0 {
			raw.WriteString(",")
		}
		raw.WriteString(`{"lockupViewModel":{"contentId":"v` + string(rune('A'+i)) + `", "metadata":{"lockupMetadataViewModel":{"title":{"content":"t"},"image":{"decoratedAvatarViewModel":{"a11yLabel":"Go to channel c"}}}}}}`)
	}
	raw.WriteString(`]}}`)
	got, err := parseLockupFeed([]byte(raw.String()), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("limit not honored: got %d", len(got))
	}
}

func TestParseRelatedItems(t *testing.T) {
	raw := []byte(`{
  "onResponseReceivedEndpoints": [
    {"reloadContinuationItemsCommand": {"continuationItems": [
      {"lockupViewModel": {
        "contentId": "aaabbb11122",
        "contentImage": {"thumbnailViewModel":{"overlays":[
          {"thumbnailBottomOverlayViewModel":{"badges":[
            {"thumbnailBadgeViewModel":{"text":"4:15"}}
          ]}}
        ]}},
        "metadata": {"lockupMetadataViewModel": {
          "title": {"content": "Up Next Song"},
          "image": {"decoratedAvatarViewModel":{"a11yLabel":"Go to channel Artist"}}
        }}
      }},
      {"lockupViewModel": {
        "contentId": "cccddd33344",
        "contentImage": {"thumbnailViewModel":{"overlays":[
          {"thumbnailBottomOverlayViewModel":{"badges":[
            {"thumbnailBadgeViewModel":{"text":"9:41"}}
          ]}}
        ]}},
        "metadata": {"lockupMetadataViewModel": {
          "title": {"content": "Another Related"},
          "image": {"decoratedAvatarViewModel":{"a11yLabel":"Go to channel Channel2"}}
        }}
      }},
      {"nonVideoRenderer": {"whatever": "skip me"}}
    ]}}
  ]
}`)
	got, err := parseRelatedItems(raw, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(got), got)
	}
	if got[0].ID != "aaabbb11122" || got[0].Title != "Up Next Song" || got[0].Duration != 255 {
		t.Errorf("result 0 = %+v, want aaabbb11122 Up Next Song 4:15", got[0])
	}
	if got[1].Uploader != "Channel2" {
		t.Errorf("result 1 = %+v, want Channel2", got[1])
	}
}

func TestFindToken(t *testing.T) {
	node := map[string]any{
		"continuation": map[string]any{
			"continuationItemRenderer": map[string]any{
				"continuationEndpoint": map[string]any{
					"continuationCommand": map[string]any{"token": "t0k3n123", "command": "watch"},
				},
			},
		},
	}
	if got := findToken(node); got != "t0k3n123" {
		t.Fatalf("findToken = %q, want t0k3n123", got)
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]int{"12:03": 723, "1:02:59": 3779, "5:99": 399, "0:30": 30, "garbage": 0, "9": 9} {
		if got := parseClock(in); got != want {
			t.Errorf("parseClock(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSapisidHashFormat(t *testing.T) {
	h := sapisidHash("SOME_SAPISID")
	re := regexp.MustCompile(`^SAPISIDHASH \d+_[0-9a-f]{40}$`)
	if !re.MatchString(h) {
		t.Errorf("unexpected hash format %q", h)
	}
}
