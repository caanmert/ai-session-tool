//go:build darwin

package main

// Wails 2.15's native file dialogs use UTType. Link its framework explicitly
// for current macOS SDKs, alongside the frameworks linked by Wails itself.

// #cgo LDFLAGS: -framework UniformTypeIdentifiers
import "C"
