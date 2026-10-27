package tray

import (
	"context"
	"log/slog"

	"fyne.io/systray"
)

// Run puts EinCast in the notification area until ctx ends or Quit is chosen.
// show is called on a click or "Open EinCast"; quit on "Quit EinCast". Both run on
// the tray's own goroutine, so they must not block for long.
//
// It starts its own message loop on a thread of its own (systray.Run owns that
// thread until it returns), which is why this is a goroutine of the caller's
// choosing: Run blocks.
func Run(ctx context.Context, log *slog.Logger, icon []byte, tooltip string, show, quit func()) {
	log = log.With(slog.String("source", "tray"))
	start, end := systray.RunWithExternalLoop(func() {
		systray.SetIcon(icon)
		systray.SetTooltip(tooltip)
		systray.SetOnTapped(show)

		open := systray.AddMenuItem("Open EinCast", "Show the window")
		systray.AddSeparator()
		exit := systray.AddMenuItem("Quit EinCast", "Stop sharing and quit")
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-open.ClickedCh:
					show()
				case <-exit.ClickedCh:
					quit()
					return
				}
			}
		}()
		log.Debug("tray icon shown")
	}, func() {})
	start()

	go func() {
		<-ctx.Done()
		end()
	}()
}
