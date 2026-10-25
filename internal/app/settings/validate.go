package settings

import (
	"fmt"
	"strings"

	"eincast/internal/app/streaming"
)

func trim(s string) string { return strings.TrimSpace(s) }

func validatePort(p int) error {
	if p < minPort || p > maxPort {
		return fmt.Errorf("port must be between %d and %d", minPort, maxPort)
	}
	return nil
}

// validateName reuses the server's own name rules, so what settings accepts is
// exactly what the server will start with.
func validateName(name string) error {
	return streaming.Config{Port: minPort, Name: name}.Validate()
}

func validate(port int, name string) error {
	if err := validatePort(port); err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return fmt.Errorf("server name: %w", err)
	}
	return nil
}
