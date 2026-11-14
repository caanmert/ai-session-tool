package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	if err := wails.Run(&options.App{
		Title: "ais — your coding workspace", Width: 1360, Height: 900,
		MinWidth: 960, MinHeight: 640,
		BackgroundColour: &options.RGBA{R: 250, G: 250, B: 249, A: 255}, // --canvas, light
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup, OnShutdown: app.shutdown, OnBeforeClose: app.beforeClose,
		Bind: []interface{}{app},
	}); err != nil {
		log.Fatal(err)
	}
}
