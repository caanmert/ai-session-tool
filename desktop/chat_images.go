package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	maxMessageImages = 6
	maxImageBytes    = 5 << 20
)

// ChatImage is an image attached to a message, as sent by the window.
type ChatImage struct {
	Name string `json:"name"`
	// Data is the file's bytes in standard base64.
	Data string `json:"data"`
}

// savedImage is an attachment after it was checked and written to disk.
type savedImage struct {
	Name      string
	MediaType string
	Path      string
	Data      string // base64, for agents that take the bytes inline
}

var imageExtensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// saveImages checks attachments and stores them with the conversation. The
// type is taken from the bytes, never from the name or what the sender says,
// and only common image formats are accepted. Files are private to the user.
func (m *chatManager) saveImages(id string, images []ChatImage) ([]savedImage, error) {
	if len(images) == 0 {
		return nil, nil
	}
	if len(images) > maxMessageImages {
		return nil, fmt.Errorf("attach at most %d images to one message", maxMessageImages)
	}
	dir := filepath.Join(m.dataDir, "attachments", id)
	if m.dataDir == "" {
		temp, err := os.MkdirTemp("", "ais-attachments-")
		if err != nil {
			return nil, err
		}
		dir = temp
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	saved := []savedImage{}
	fail := func(err error) ([]savedImage, error) {
		for _, image := range saved {
			_ = os.Remove(image.Path)
		}
		return nil, err
	}
	for _, image := range images {
		name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(image.Name, "\\", "/")))
		if name == "" || name == "." || name == "/" {
			name = "image"
		}
		name = shorten(name, 120)
		if base64.StdEncoding.DecodedLen(len(image.Data)) > maxImageBytes+3 {
			return fail(fmt.Errorf("%s is larger than 5 MB", name))
		}
		data, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil || len(data) == 0 {
			return fail(fmt.Errorf("%s could not be read as an image", name))
		}
		if len(data) > maxImageBytes {
			return fail(fmt.Errorf("%s is larger than 5 MB", name))
		}
		mediaType := http.DetectContentType(data)
		extension, ok := imageExtensions[mediaType]
		if !ok {
			return fail(fmt.Errorf("%s is not a PNG, JPEG, GIF or WebP image", name))
		}
		path := filepath.Join(dir, uuid.NewString()+extension)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return fail(err)
		}
		saved = append(saved, savedImage{Name: name, MediaType: mediaType, Path: path, Data: image.Data})
	}
	return saved, nil
}

// SendChatWithImages sends a message with images attached. The images are
// stored with the conversation on this Mac and given to the agent with the
// message.
func (a *App) SendChatWithImages(id, text string, images []ChatImage) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	if len(images) == 0 {
		return a.chats.send(id, text)
	}
	return a.chats.sendWith(id, text, images)
}

var errImagesInReview = errors.New("a review reads a fixed snapshot; images cannot be attached")
