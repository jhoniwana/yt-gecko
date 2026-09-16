package tui

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestKittyTransmitChunks(t *testing.T) {
	data := make([]byte, 3*gfxChunk+10)
	for i := range data {
		data[i] = byte(i % 251)
	}
	s := kittyTransmit(7, data)
	if !strings.HasPrefix(s, "\x1b_Ga=t,i=7,f=100,q=2,m=1;") {
		t.Fatalf("bad first chunk: %.40q", s)
	}
	if !strings.HasSuffix(s, "\x1b\\") {
		t.Fatalf("missing terminator")
	}
	chunks := strings.Split(s, "\x1b\\")
	if len(chunks) != 5 { // 3 full chunks + tail, plus trailing ST
		t.Fatalf("expected 4 payload chunks, got %d", len(chunks)-1)
	}
	var buf []byte
	for i, c := range chunks {
		if c == "" {
			continue
		}
		body := c
		if strings.HasPrefix(c, "\x1b_Gm=1;") {
			body = strings.TrimPrefix(c, "\x1b_Gm=1;")
		} else if strings.HasPrefix(c, "\x1b_Gm=0;") {
			body = strings.TrimPrefix(c, "\x1b_Gm=0;")
		} else if strings.HasPrefix(c, "\x1b_Ga=t,i=7,f=100,q=2,m=") {
			body = c[strings.Index(c, ";")+1:]
		} else {
			t.Fatalf("unexpected chunk %d: %.40q", i, c)
		}
		dec, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			t.Fatalf("bad base64 in chunk %d: %v", i, err)
		}
		buf = append(buf, dec...)
	}
	if string(buf) != string(data) {
		t.Fatal("roundtrip mismatch")
	}
}

func TestKittyTransmitSingleChunk(t *testing.T) {
	s := kittyTransmit(1, []byte("hi"))
	if s != "\x1b_Ga=t,i=1,f=100,q=2,m=0;aGk=\x1b\\" {
		t.Fatalf("got %q", s)
	}
}

func TestKittyVirtualHasCursorGuard(t *testing.T) {
	s := kittyVirtual(9, 12, 7)
	for _, want := range []string{"a=p", "U=1", "i=9", "c=12", "r=7", "q=2", "C=1"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %q", want, s)
		}
	}
}

func TestKittyPlaceholderGrid(t *testing.T) {
	s := kittyPlaceholder(42, 12, 7)
	lines := strings.Split(s, "\n")
	if len(lines) != 7 {
		t.Fatalf("got %d rows", len(lines))
	}
	for row, ln := range lines {
		if !strings.HasPrefix(ln, "\x1b[38;5;42m") {
			t.Fatalf("row %d missing fg color", row)
		}
		if !strings.HasSuffix(ln, "\x1b[39m") {
			t.Fatalf("row %d missing color reset", row)
		}
		body := strings.TrimPrefix(strings.TrimSuffix(ln, "\x1b[39m"), "\x1b[38;5;42m")
		cells := strings.Split(body, "\U0010EEEE")
		if len(cells)-1 != 12 {
			t.Fatalf("row %d has %d cells", row, len(cells)-1)
		}
		for col := 0; col < 12; col++ {
			if cells[col+1] != string(phDia[row])+string(phDia[col]) {
				t.Fatalf("row %d col %d diacritics wrong: %q", row, col, cells[col+1])
			}
		}
	}
}

func TestKittyClear(t *testing.T) {
	if s := kittyClear(); s != "\x1b_Ga=d,d=A;\x1b\\" {
		t.Fatalf("got %q", s)
	}
}

func TestPadCells(t *testing.T) {
	if got := padCells("abc", 5); got != "abc  " {
		t.Fatalf("got %q", got)
	}
	if got := padCells("abcdef", 5); got != "abcdef" {
		t.Fatalf("got %q", got)
	}
}
