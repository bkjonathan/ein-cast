package cloudflared

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"eincast/internal/app"
	"eincast/internal/app/providerconfig"
)

// ErrNotInstalled means no cloudflared was found. The message is shown to the
// person, so it says what to do.
var ErrNotInstalled = errors.New("cloudflared was not found. Install it (for example `brew install cloudflared`, or download it from https://github.com/cloudflare/cloudflared/releases) and try again, or choose where it is in the settings below")

// sharedBinary is the namespace of the setting both Cloudflare tunnels use for
// where cloudflared is: the person is asked once.
const sharedBinary = "cloudflared"

// binaryField is the optional "where is cloudflared" setting.
func binaryField() app.ConfigField {
	return app.ConfigField{
		Key:         "binary",
		Label:       "cloudflared location",
		Help:        "Only needed if EinCast cannot find cloudflared by itself.",
		Placeholder: "/opt/homebrew/bin/cloudflared",
		Kind:        app.FieldFile,
		Shared:      sharedBinary,
		Clean:       cleanBinaryPath,
	}
}

// cleanBinaryPath accepts the full path of an executable file.
func cleanBinaryPath(raw string) (string, error) {
	p := filepath.Clean(raw)
	if !filepath.IsAbs(p) {
		return "", errors.New("enter the full path, starting from the root folder")
	}
	return p, checkExecutable(p)
}

// checkExecutable reports why path cannot be run, in words for the person.
func checkExecutable(path string) error {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("there is no file at %s", path)
	case err != nil:
		return fmt.Errorf("cannot look at %s: %w", path, err)
	case info.IsDir():
		return fmt.Errorf("%s is a folder, not the program", path)
	case runtime.GOOS != "windows" && info.Mode()&0o111 == 0:
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}

// locator finds cloudflared for one provider: the place the person chose, if
// they chose one, otherwise the usual ones.
func locator(cfg *providerconfig.Store, provider app.ProviderID) func() (string, error) {
	return func() (string, error) {
		if cfg != nil {
			chosen, err := cfg.Get(provider, binaryField())
			if err != nil {
				return "", err
			}
			if chosen != "" {
				// They said where it is, so do not quietly use another one. It may
				// have been moved or uninstalled since.
				if err := checkExecutable(chosen); err != nil {
					return "", fmt.Errorf("the saved location of cloudflared no longer works: %w", err)
				}
				return chosen, nil
			}
		}
		return locateCLI()
	}
}

// locateCLI finds cloudflared: on PATH first, then where installers put it.
func locateCLI() (string, error) {
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}
	for _, p := range installLocations(runtime.GOOS) {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotInstalled
}

func installLocations(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/opt/homebrew/bin/cloudflared", "/usr/local/bin/cloudflared"}
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if dir := os.Getenv(env); dir != "" {
				out = append(out, filepath.Join(dir, "cloudflared", "cloudflared.exe"))
			}
		}
		return out
	default:
		return nil
	}
}
