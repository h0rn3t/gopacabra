package cl_test

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xgo-dev/llgo/cl/cltest"
	"github.com/xgo-dev/llgo/internal/build"
)

// TestGopacabraInteriorBytes — P8.6: фікстура сама звіряє зміну gopacabra freed і
// gopacabra live rc-local за P3.5 і виходить з кодом 0, лише якщо обидва збіглися.
func TestGopacabraInteriorBytes(t *testing.T) {
	for _, fixture := range []string{
		"interior",      // локальний &s.f (P8.4): 16 байтів звільнено
		"freshinterior", // свіжий внутрішній результат (P8.5): звільняє викликач
		"interiorheld",  // невідомий виклик тримає &s.f: 16 байтів живі
	} {
		t.Run(fixture, func(t *testing.T) {
			conf := build.NewDefaultConf(build.ModeRun)
			conf.Tags = "gopacabra"
			out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/"+fixture, "", conf)
			if err != nil {
				t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/%s error = %v, want exit 0\n%s", fixture, err, out)
			}
		})
	}
}

// TestGopacabraSharedBytes — P9.4: фікстури самі звіряють Freed і Live місць
// спільної втечі через go.
func TestGopacabraSharedBytes(t *testing.T) {
	for _, fixture := range []string{
		"sharedgo",     // змінна в замиканні go: звільнена після Wait і кінця функції
		"sharedtree",   // горутина пише NewTree у змінну: звільнені комірка і дерево
		"sharedglobal", // те саме місце ще в глобальній змінній: живе
	} {
		t.Run(fixture, func(t *testing.T) {
			conf := build.NewDefaultConf(build.ModeRun)
			conf.Tags = "gopacabra"
			out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/"+fixture, "", conf)
			if err != nil {
				t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/%s error = %v, want exit 0\n%s", fixture, err, out)
			}
		})
	}
}

// TestGopacabraPinnedBytes — P10.4: закріплене місце в межах функції, через межу
// виклику і в полі звільненого батька лишається серед живих; фікстура сама
// звіряє Freed, Live(pinned) і суму живих.
func TestGopacabraPinnedBytes(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/pinned", "", conf)
	if err != nil {
		t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/pinned error = %v, want exit 0\n%s", err, out)
	}
}

// TestGopacabraPinnedMapKey — P11.0: присвоєння, читання і видалення в мапі з
// рядковим ключем і ключем-вказівником не додають байтів до live pinned.
func TestGopacabraPinnedMapKey(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/mapkey", "", conf)
	if err != nil {
		t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/mapkey error = %v, want exit 0\n%s", err, out)
	}
}

// TestGopacabraPinnedTypePun — P11.0.1: type-pun у межах функції (math.Float64bits,
// strconv, код модуля) не додає байтів до live pinned; адреса, що втікає в
// глобальну змінну, лишається pinned.
func TestGopacabraPinnedTypePun(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/typepun", "", conf)
	if err != nil {
		t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/typepun error = %v, want exit 0\n%s", err, out)
	}
}

// TestGopacabraPinnedCcgoAddr — фаза 12: форми адреси локальної змінної з коду
// ccgo (коробка виклику через слово, P12.2; аргумент параметра unsafe.Pointer,
// P12.3) не додають байтів до live pinned;
// адреса, що втікає в глобальну змінну, лишається pinned.
func TestGopacabraPinnedCcgoAddr(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/ccgoaddr", "", conf)
	if err != nil {
		t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/ccgoaddr error = %v, want exit 0\n%s", err, out)
	}
}

// TestGopacabraDeferClosureCell — P12.5.1: комірка, яку захоплює
// defer-замикання (форма ccgo `if __ccgo_strace { defer ... }`), звільняється
// після запуску відкладених викликів; на шляху panic і в циклі значення
// правильні.
func TestGopacabraDeferClosureCell(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/deferclosure", "", conf)
	if err != nil {
		t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/deferclosure error = %v, want exit 0\n%s", err, out)
	}
}

// TestGopacabraClosureFieldCell — P12.5.2: комірка параметра, який захоплює
// замикання в полі чи в return (форма database/sql.convertAssignRows),
// звільняється на шляхах без замикання.
func TestGopacabraClosureFieldCell(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/closurefield", "", conf)
	if err != nil {
		t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/closurefield error = %v, want exit 0\n%s", err, out)
	}
}

// TestGopacabraWithLockCell — P12.5.5.1: комірка змінної-вказівника на
// циклічний тип, яку захоплює замикання, передане у виклик (форми
// database/sql QueryContext і queryDC), звільняється при виході з функції.
func TestGopacabraWithLockCell(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/withlock", "", conf)
	if err != nil {
		t.Errorf("llgo run -tags gopacabra ./gopacabra/bytes/withlock error = %v, want exit 0\n%s", err, out)
	}
}

// TestGopacabraNetpollEvents — P12.5.6.1: буфер подій netpoll, адреса якого
// йде в C.epoll_wait, не виділяється в купі на кожен виклик. Фікстура робить
// 1000 обмінів через loopback; рядок gopacabra site з файлу netpoll з
// лічильником понад 100 означає виділення на виклик.
func TestGopacabraNetpollEvents(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("netpoll на epoll — лише linux")
	}
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra,gopacabrastat"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/netpoll", "", conf)
	if err != nil {
		t.Fatalf("llgo run -tags gopacabra,gopacabrastat ./gopacabra/bytes/netpoll error = %v, want exit 0\n%s", err, out)
	}
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) != 7 || f[0] != "gopacabra" || f[1] != "site" || !strings.Contains(f[2], "/netpoll_") {
			continue
		}
		if n, _ := strconv.Atoi(f[6]); n > 100 {
			t.Errorf("site %s %s: %d heap allocations, want none per netpoll call", f[2], f[5], n)
		}
	}
}

// TestGopacabraOverlaySites — P12.5.6.2: census розставляє місця функцій
// оверлея. Фікстура кличе reflect.ValueOf 4000 разів; unplaced-рядок
// gopacabra site з файлу runtime/internal/lib з лічильником понад 100 означає,
// що виділення в оверлеї census не бачив. Оверлей runtime census свідомо не
// розставляє (newPackageEx), тож його рядки тест пропускає.
func TestGopacabraOverlaySites(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra,gopacabrastat"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/overlaysite", "", conf)
	if err != nil {
		t.Fatalf("llgo run -tags gopacabra,gopacabrastat ./gopacabra/bytes/overlaysite error = %v, want exit 0\n%s", err, out)
	}
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) != 7 || f[0] != "gopacabra" || f[1] != "site" || f[3] != "unplaced" ||
			!strings.Contains(f[2], "/runtime/internal/lib/") || strings.Contains(f[2], "/runtime/internal/lib/runtime/") {
			continue
		}
		if n, _ := strconv.Atoi(f[6]); n > 100 {
			t.Errorf("site %s %s: unplaced with %d allocations, want placed by census", f[2], f[5], n)
		}
	}
}

// TestGopacabraJSONQuote — P12.5.4: копія рядка []byte(s), яку
// makeStringArshaler передає в jsonwire.AppendQuote, звільняється на кожному
// виклику json.Marshal. Фікстура кличе json.Marshal 1000 разів; тест звіряє
// виділення й звільнення місця arshal_default.go:231 за рядками gopacabra site.
func TestGopacabraJSONQuote(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra,gopacabrastat"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/jsonquote", "", conf)
	if err != nil {
		t.Fatalf("llgo run -tags gopacabra,gopacabrastat ./gopacabra/bytes/jsonquote error = %v, want exit 0\n%s", err, out)
	}
	var allocs, frees int
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) != 7 || f[0] != "gopacabra" || f[1] != "site" || !strings.HasSuffix(f[2], "/encoding/json/v2/arshal_default.go:231") {
			continue
		}
		n, _ := strconv.Atoi(f[6])
		switch f[5] {
		case "alloc", "alloc_counted":
			allocs += n
		case "dec_accounted", "free_accounted":
			frees += n
		}
	}
	if allocs < 1000 || frees != allocs {
		t.Errorf("site arshal_default.go:231: %d allocations, %d freed; want at least 1000, all freed", allocs, frees)
	}
}

// TestGopacabraFmtErrorf: змінна, адресу якої позичає функція форми
// convertAssignRows (fmt.Errorf лише з %T), не лишається живою (P12.5.7.3).
func TestGopacabraFmtErrorf(t *testing.T) {
	conf := build.NewDefaultConf(build.ModeRun)
	conf.Tags = "gopacabra,gopacabrastat"
	out, err := cltest.RunAndCaptureWithConf("../gopacabra/bytes/fmterrorf", "", conf)
	if err != nil {
		t.Fatalf("llgo run -tags gopacabra,gopacabrastat ./gopacabra/bytes/fmterrorf error = %v, want exit 0\n%s", err, out)
	}
	var allocs, frees int
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) != 7 || f[0] != "gopacabra" || f[1] != "site" || !strings.HasSuffix(f[2], "/fmterrorf/main.go:32") {
			continue
		}
		n, _ := strconv.Atoi(f[6])
		switch f[5] {
		case "alloc", "alloc_counted":
			allocs += n
		case "dec_accounted", "free_accounted":
			frees += n
		}
	}
	// Позичена змінна може піти на стек: тоді виділень немає зовсім.
	if frees != allocs {
		t.Errorf("site main.go:32: %d allocations, %d freed; want all freed\n%s", allocs, frees, out)
	}
}
