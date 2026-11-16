//go:build darwin

package main

// The banner delegate in attention_darwin.go calls this when a banner is
// clicked. A file with //export may only declare C, so it lives apart.

import "C"

//export aisNotificationOpened
func aisNotificationOpened(chat *C.char) {
	notificationOpened(C.GoString(chat))
}
