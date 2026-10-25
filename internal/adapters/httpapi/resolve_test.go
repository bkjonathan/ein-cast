package httpapi

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInside(t *testing.T) {
	root := t.TempDir()
	inside := writeFile(t, root, "a/b.mkv", []byte("x"))
	outsideDir := t.TempDir()
	outside := writeFile(t, outsideDir, "c.mkv", []byte("x"))

	tests := []struct {
		name    string
		root    string
		file    string
		wantErr error // nil means success; errOutsideRoot is matched with errors.Is
		anyErr  bool
	}{
		{name: "inside", root: root, file: inside},
		{name: "outside", root: root, file: outside, wantErr: errOutsideRoot},
		{name: "dot-dot escape", root: root, file: filepath.Join(root, "a", "..", "..", filepath.Base(outsideDir), "c.mkv"), anyErr: true},
		{name: "sibling with shared prefix", root: root, file: writeFile(t, root+"-sibling", "d.mkv", []byte("x")), wantErr: errOutsideRoot},
		{name: "no library", root: "", file: inside, anyErr: true},
		{name: "missing file", root: root, file: filepath.Join(root, "missing.mkv"), anyErr: true},
	}
	t.Cleanup(func() { _ = os.RemoveAll(root + "-sibling") })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveInside(tt.root, tt.file)
			switch {
			case tt.anyErr:
				if err == nil {
					t.Fatal("error = nil, want an error")
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
			default:
				if err != nil {
					t.Fatalf("error = %v", err)
				}
			}
		})
	}
}
