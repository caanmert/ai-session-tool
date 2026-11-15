package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// solidPNG is a small single-color PNG, base64 encoded.
func solidPNG(t testing.TB, fill color.Color) string {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			picture.Set(x, y, fill)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(out.Bytes())
}

func TestSendWithImages(t *testing.T) {
	red := solidPNG(t, color.RGBA{R: 255, A: 255})
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := fakeChats(t, "image")
			state := createChat(t, m, t.TempDir(), provider, "")
			if err := m.sendWith(state.ID, "What is in this image?", []ChatImage{{Name: "../../shot one.png", Data: red}}); err != nil {
				t.Fatal(err)
			}
			done := finishFake(t, m, state.ID, provider)
			// The message lists the attachment by a safe name.
			if got := done.Messages[0].Images; len(got) != 1 || got[0] != "shot one.png" {
				t.Fatalf("attachment names: %v", got)
			}
			// It is stored with the conversation, private to the user.
			files, _ := filepath.Glob(filepath.Join(m.dataDir, "attachments", state.ID, "*.png"))
			if len(files) != 1 {
				t.Fatalf("expected one stored image, got %v", files)
			}
			if info, err := os.Stat(files[0]); err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("stored image permissions: %v %v", info, err)
			}
		})
	}
}

func TestImagesAreChecked(t *testing.T) {
	m := fakeChats(t, "normal")
	state := createChat(t, m, t.TempDir(), "codex", "")
	red := solidPNG(t, color.RGBA{R: 255, A: 255})
	text := base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\necho not an image\n"))
	html := base64.StdEncoding.EncodeToString([]byte("<html><script>alert(1)</script></html>"))
	big := base64.StdEncoding.EncodeToString(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, maxImageBytes)...))
	many := make([]ChatImage, maxMessageImages+1)
	for i := range many {
		many[i] = ChatImage{Name: "a.png", Data: red}
	}
	for name, tc := range map[string]struct {
		images []ChatImage
		want   string
	}{
		"a script named .png": {[]ChatImage{{Name: "run.png", Data: text}}, "not a PNG, JPEG, GIF or WebP"},
		"html":                {[]ChatImage{{Name: "page.png", Data: html}}, "not a PNG, JPEG, GIF or WebP"},
		"not base64":          {[]ChatImage{{Name: "x.png", Data: "!!!"}}, "could not be read"},
		"empty":               {[]ChatImage{{Name: "x.png", Data: ""}}, "could not be read"},
		"too large":           {[]ChatImage{{Name: "big.png", Data: big}}, "larger than 5 MB"},
		"too many":            {many, "at most 6 images"},
		"one bad among good":  {[]ChatImage{{Name: "ok.png", Data: red}, {Name: "bad.png", Data: text}}, "bad.png is not"},
	} {
		err := m.sendWith(state.ID, "Look", tc.images)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
	// A rejected message starts nothing and leaves no files behind.
	current, _ := m.read(state.ID, -1)
	if current.State.Status != "idle" || len(current.State.Messages) != 0 {
		t.Fatalf("a rejected message must not start a turn: %+v", current.State)
	}
	left, _ := filepath.Glob(filepath.Join(m.dataDir, "attachments", state.ID, "*"))
	if len(left) != 0 {
		t.Fatalf("rejected images were left on disk: %v", left)
	}
}
