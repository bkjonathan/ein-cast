// Package reset is the use case behind "Reset EinCast": forget everything this
// install knows (settings, libraries, profiles, resume positions, tokens, caches,
// the open-at-login entry) and open again as a first run (ADR 0034). The person's
// media files are never touched.
package reset

import (
	"errors"
	"fmt"
	"log/slog"

	"eincast/internal/app"
)

// Service erases an install and starts it over.
type Service struct {
	data     app.DataEraser
	secrets  app.SecretEraser
	login    app.LoginItem // nil: the platform has no login entry to remove
	relaunch app.Relauncher
	log      *slog.Logger
}

// NewService returns a Service. login may be nil.
func NewService(data app.DataEraser, secrets app.SecretEraser, login app.LoginItem, relaunch app.Relauncher, log *slog.Logger) *Service {
	return &Service{data: data, secrets: secrets, login: login, relaunch: relaunch, log: log.With(slog.String("source", "reset"))}
}

// Erase removes everything EinCast keeps. Every step runs even when an earlier
// one fails, because a half-erased install is worse than one that reports what
// is left; the failures come back together. Nothing may be writing meanwhile:
// the caller stops the server and flushes what it holds first.
func (s *Service) Erase() error {
	var errs []error
	if s.login != nil {
		if err := s.login.Set(false); err != nil {
			errs = append(errs, fmt.Errorf("remove the open-at-login entry: %w", err))
		}
	}
	if err := s.secrets.EraseAll(); err != nil {
		errs = append(errs, fmt.Errorf("erase saved tokens and PINs: %w", err))
	}
	if err := s.data.Erase(); err != nil {
		errs = append(errs, fmt.Errorf("erase settings and caches: %w", err))
	}
	if err := errors.Join(errs...); err != nil {
		s.log.Error("reset left something behind", slog.String("error", err.Error()))
		return err
	}
	s.log.Info("erased all saved data")
	return nil
}

// Relaunch arranges for EinCast to open again once this process has exited.
func (s *Service) Relaunch() error {
	if err := s.relaunch.Relaunch(); err != nil {
		return fmt.Errorf("reopen EinCast: %w", err)
	}
	return nil
}
