package streaming

import (
	"strings"
	"testing"
)

func TestNewConfig_DefaultsAndOptions(t *testing.T) {
	if got := NewConfig(); got != (Config{Port: DefaultPort, Name: DefaultName}) {
		t.Fatalf("NewConfig() = %+v", got)
	}
	got := NewConfig(WithPort(9000), WithName("  Living Room  "))
	if got != (Config{Port: 9000, Name: "Living Room"}) {
		t.Fatalf("NewConfig(options) = %+v", got)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"defaults", NewConfig(), false},
		{"port zero means any free port", NewConfig(WithPort(0)), false},
		{"max port", NewConfig(WithPort(65535)), false},
		{"port too high", NewConfig(WithPort(65536)), true},
		{"negative port", NewConfig(WithPort(-1)), true},
		{"empty name", NewConfig(WithName("   ")), true},
		{"name with newline", Config{Port: 1, Name: "a\nb"}, true},
		{"name at limit", NewConfig(WithName(strings.Repeat("é", maxNameLen))), false},
		{"name over limit", NewConfig(WithName(strings.Repeat("é", maxNameLen+1))), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
