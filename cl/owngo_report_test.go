package cl_test

import (
	"strings"
	"testing"

	"github.com/xgo-dev/llgo/cl/cltest"
	"github.com/xgo-dev/llgo/internal/build"
)

// TestOwnGoReportOptIn — P11.5.0: owngo — єдина збірка, тож звіт owngo freed і
// owngo live на виході програми друкується лише з тегом owngoreport.
func TestOwnGoReportOptIn(t *testing.T) {
	for _, tt := range []struct {
		tags string
		want bool
	}{
		{tags: "", want: false},
		{tags: "owngoreport", want: true},
	} {
		t.Run("tags="+tt.tags, func(t *testing.T) {
			conf := build.NewDefaultConf(build.ModeRun)
			conf.Tags = tt.tags
			out, err := cltest.RunAndCaptureWithConf("../owngo/bytes/pinned", "", conf)
			if err != nil {
				t.Fatalf("llgo run -tags %q ./owngo/bytes/pinned error = %v\n%s", tt.tags, err, out)
			}
			if got := strings.Contains(string(out), "owngo freed"); got != tt.want {
				t.Errorf("llgo run -tags %q ./owngo/bytes/pinned prints owngo freed = %v, want %v\n%s", tt.tags, got, tt.want, out)
			}
		})
	}
}
