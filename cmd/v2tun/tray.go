//go:build windows || linux

package main

import (
	goruntime "runtime"
	"time"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// startTray runs the notification-area icon on its own OS thread (the tray
// window must be served by the thread that created it).
func (a *App) startTray() {
	go func() {
		defer recoverPanic("tray")
		goruntime.LockOSThread()
		systray.Run(a.trayReady, nil)
	}()
}

// hideWindow sends the window to the tray, or minimises it when this desktop
// has no tray (GNOME without the AppIndicator extension), so v2tun stays
// reachable.
func (a *App) hideWindow() {
	if a.ctx == nil {
		return
	}
	if trayAvailable() {
		runtime.WindowHide(a.ctx)
	} else {
		runtime.WindowMinimise(a.ctx)
	}
}

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

// quit really exits: stops the tunnel (restoring the network) and the tray.
func (a *App) quit() {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	a.hotspot.Stop(a.engine)
	a.engine.Stop()
	a.stopProxyMode()
	systray.Quit()
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

func (a *App) trayReady() {
	defer recoverPanic("tray")
	systray.SetIcon(trayIconOff)
	systray.SetTooltip(T("v2tun — offline"))
	systray.SetOnTapped(a.showWindow)

	mShow := systray.AddMenuItem(T("Show v2tun"), "")
	mToggle := systray.AddMenuItem(T("Connect"), "")
	systray.AddSeparator()
	mAutostart := systray.AddMenuItemCheckbox(T(pick("Start with Windows", "Start at login")), "", a.GetAutostart())
	mAutoConnect := systray.AddMenuItemCheckbox(T("Auto-connect on start"), "", a.GetSettings().AutoConnect)
	systray.AddSeparator()
	mQuit := systray.AddMenuItem(T("Quit (disconnect tunnel)"), "")

	var lastOn, lastBusy *bool
	lastLang := getLang()
	refresh := func() {
		st := a.Status()
		if l := getLang(); l != lastLang { // language changed in the window
			lastLang, lastOn = l, nil
			mShow.SetTitle(T("Show v2tun"))
			mAutostart.SetTitle(T(pick("Start with Windows", "Start at login")))
			mAutoConnect.SetTitle(T("Auto-connect on start"))
			mQuit.SetTitle(T("Quit (disconnect tunnel)"))
		}
		if lastOn == nil || *lastOn != st.Running || *lastBusy != st.Busy {
			on, busy := st.Running, st.Busy
			lastOn, lastBusy = &on, &busy
			switch {
			case busy:
				mToggle.SetTitle(T("Please wait…"))
				mToggle.Disable()
			case on:
				mToggle.SetTitle(T("Disconnect"))
				mToggle.Enable()
				systray.SetIcon(trayIconOn)
			default:
				mToggle.SetTitle(T("Connect"))
				mToggle.Enable()
				systray.SetIcon(trayIconOff)
			}
		}
		tip := T("v2tun — offline")
		if st.Running {
			tip = T("v2tun — connected")
			if st.Exit.IP != "" {
				tip += " (" + st.Exit.IP + " " + st.Exit.Country + ")"
			}
		}
		systray.SetTooltip(tip)
		setChecked(mAutostart, a.GetAutostart())
		setChecked(mAutoConnect, a.GetSettings().AutoConnect)
	}
	refresh()
	tick := time.NewTicker(time.Second)
	for {
		select {
		case <-tick.C:
			refresh()
		case <-mShow.ClickedCh:
			a.showWindow()
		case <-mToggle.ClickedCh:
			go func() {
				defer recoverPanic("tray")
				if a.connected() {
					a.Disconnect()
				} else if err := a.Connect(); err != nil {
					a.showWindow()
				}
			}()
		case <-mAutostart.ClickedCh:
			a.SetAutostart(!mAutostart.Checked())
		case <-mAutoConnect.ClickedCh:
			s := a.GetSettings()
			s.AutoConnect = !s.AutoConnect
			a.SaveSettings(s)
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "settings", s)
			}
		case <-mQuit.ClickedCh:
			a.quit()
			return
		}
	}
}

func setChecked(m *systray.MenuItem, v bool) {
	if m.Checked() != v {
		if v {
			m.Check()
		} else {
			m.Uncheck()
		}
	}
}
