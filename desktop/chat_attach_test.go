package main

import "testing"

func TestMentionPath(t *testing.T) {
	for _, tc := range []struct{ project, file, want string }{
		{"/work/app", "/work/app/src/main.go", "src/main.go"},
		{"/work/app", "/work/app/notes with spaces.md", "notes with spaces.md"},
		{"/work/app", "/work/other/file.txt", "/work/other/file.txt"},
		{"/work/app", "/work/app-two/file.txt", "/work/app-two/file.txt"},
		{"/work/app", "/work/app", "/work/app"},
		{"", "/tmp/file.txt", "/tmp/file.txt"},
	} {
		if got := mentionPath(tc.project, tc.file); got != tc.want {
			t.Errorf("mentionPath(%q, %q) = %q, want %q", tc.project, tc.file, got, tc.want)
		}
	}
}
