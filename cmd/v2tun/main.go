//go:build windows || linux

// v2tun: system-wide tunnel with a kill switch (Windows and Linux).
package main

import (
	"embed"
	"io/fs"
	"os"
	"slices"
	"strconv"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/xjasonlyu/tun2socks/v2/log"
	"go.uber.org/zap"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--watchdog" {
		if pid, err := strconv.Atoi(os.Args[2]); err == nil {
			openLogFile()
			watchdogMain(pid)
		}
		return
	}
	openLogFile()
	log.SetLogger(zap.NewNop())
	ui, _ := fs.Sub(assets, "frontend")
	app := NewApp()
	// Started by Windows at logon: stay in the tray, no window.
	app.launched = slices.Contains(os.Args[1:], "--autostart")

	err := wails.Run(&options.App{
		Title:       "v2tun",
		Width:       440,
		Height:      760,
		MinWidth:    400,
		MinHeight:   620,
		StartHidden: app.launched,
		// No Windows title bar: the app draws its own (frontend .titlebar), so the
		// whole window is one dark surface. Windows 11 still adds the rounded
		// corners, shadow and snap behaviour.
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 11, G: 15, B: 25, A: 255},
		AssetServer:      &assetserver.Options{Assets: ui},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "v2tun-4f1c2b8e-1d3a-4c55-9a0e-7b2f6c1d9e10",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.showWindow() },
		},
		Windows: &windows.Options{Theme: windows.Dark},
		Linux:   &linux.Options{ProgramName: "v2tun", Icon: appIcon(), WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand},
	})
	systray.Quit()
	if err != nil {
		writeLogFile(LogLine{Time: "", Level: "error", Msg: "wails: " + err.Error()})
	}
}

// appIcon is the window / taskbar icon on Linux.
func appIcon() []byte {
	b, _ := assets.ReadFile("frontend/icon.png")
	return b
}
