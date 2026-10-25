package bridge

import (
	"runtime"
	"testing"
)

func TestAppInfo(t *testing.T) {
	got := New(Info{Name: "EinCast", Version: "1.2.3"}).AppInfo()

	want := AppInfoDTO{Name: "EinCast", Version: "1.2.3", Platform: runtime.GOOS}
	if got != want {
		t.Fatalf("AppInfo() = %+v, want %+v", got, want)
	}
}
