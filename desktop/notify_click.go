package main

import "sync"

// What happens when a notification banner is clicked: set at startup to
// bring the window forward and open the banner's conversation.
var (
	clickMu      sync.Mutex
	onBannerOpen func(chat string)
)

func setBannerOpen(handler func(chat string)) {
	clickMu.Lock()
	defer clickMu.Unlock()
	onBannerOpen = handler
}

func notificationOpened(chat string) {
	clickMu.Lock()
	handler := onBannerOpen
	clickMu.Unlock()
	if handler != nil && chat != "" {
		handler(chat)
	}
}
