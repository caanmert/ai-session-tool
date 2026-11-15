package main

import (
	"image/color"
	"strings"
	"testing"
)

// Both agents must actually see an attached image: each is sent a plain red
// square and asked for its color.
func TestLiveImageAttachment(t *testing.T) {
	requireLiveAgents(t)
	red := solidPNG(t, color.RGBA{R: 255, A: 255})
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			s := createChat(t, m, t.TempDir(), provider, "")
			prompt := "The attached image is filled with a single color. Reply with only the name of that color as one lowercase English word. Do not use any tools."
			if err := m.sendWith(s.ID, prompt, []ChatImage{{Name: "color.png", Data: red}}); err != nil {
				t.Fatal(err)
			}
			done := finishLiveChat(t, m, s.ID, nil)
			reply := strings.ToLower(done.Messages[len(done.Messages)-1].Text)
			t.Logf("%s saw: %q", provider, shorten(reply, 120))
			if !strings.Contains(reply, "red") {
				t.Fatalf("%s did not describe the attached image: %q", provider, reply)
			}
		})
	}
}
