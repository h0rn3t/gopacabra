package cl_test

import (
	"strings"
	"testing"

	"github.com/xgo-dev/llgo/cl/cltest"
	"github.com/xgo-dev/llgo/internal/build"
)

// TestGopacabraReportOptIn — P11.5.0: gopacabra — єдина збірка, тож звіт gopacabra freed і
// gopacabra live на виході програми друкується лише з тегом gopacabrareport.
func TestGopacabraReportOptIn(t *testing.T) {
	for _, tt := range []struct {
		tags string
		want bool
	}{
		{tags: "", want: false},
		{tags: "gopacabrareport", want: true},
	} {
		t.Run("tags="+tt.tags, func(t *testing.T) {
			conf := build.NewDefaultConf(build.ModeRun)
			conf.Tags = tt.tags
			out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/pinned", "", conf)
			if err != nil {
				t.Fatalf("llgo run -tags %q ./gopacabra/bytes/pinned error = %v\n%s", tt.tags, err, out)
			}
			if got := strings.Contains(string(out), "gopacabra freed"); got != tt.want {
				t.Errorf("llgo run -tags %q ./gopacabra/bytes/pinned prints gopacabra freed = %v, want %v\n%s", tt.tags, got, tt.want, out)
			}
		})
	}
}
