package build

import (
	"fmt"
	"slices"
	"sync"

	llssa "github.com/xgo-dev/llgo/ssa"
	gllvm "github.com/xgo-dev/llvm"
)

// mllvmParsed — опції, вже розібрані LLVM у цьому процесі: розбір змінює
// глобальний стан LLVM і не скасовується.
var mllvmParsed struct {
	sync.Mutex
	done bool
	opts []string
}

// applyMLLVM передає опції `-mllvm` LLVM у процесі llgo, де йдуть проходи
// і кодогенерація без LTO. Інший набір опцій у тому самому процесі — помилка.
func applyMLLVM(opts []string) error {
	if len(opts) == 0 {
		return nil
	}
	mllvmParsed.Lock()
	defer mllvmParsed.Unlock()
	if mllvmParsed.done {
		if slices.Equal(mllvmParsed.opts, opts) {
			return nil
		}
		return fmt.Errorf("-mllvm %q: this process already set LLVM options %q", opts, mllvmParsed.opts)
	}
	if err := gllvm.ParseCommandLineOptionsChecked(append([]string{"llgo"}, opts...)); err != nil {
		return fmt.Errorf("-mllvm: %w", err)
	}
	mllvmParsed.done = true
	mllvmParsed.opts = slices.Clone(opts)
	return nil
}

// ltoTargetLinkerArgs передає lld CPU і ознаки цілі, які без LTO отримує
// кодогенерація в процесі llgo. Біткод llgo не несе атрибутів target-cpu,
// тож без них lld генерує код під базовий CPU і GOAMD64 чи GOARM64 губляться.
// Іменовану ціль crosscompile передає компонувальнику сам.
func ltoTargetLinkerArgs(conf *Config, spec llssa.TargetSpec) []string {
	if !conf.ltoEnabled() || conf.Target != "" || spec.CPU == "" {
		return nil
	}
	args := []string{"-Xlinker", "-mllvm", "-Xlinker", "-mcpu=" + spec.CPU}
	if spec.Features != "" {
		args = append(args, "-Xlinker", "-mllvm", "-Xlinker", "-mattr="+spec.Features)
	}
	return args
}

// mllvmLinkerArgs передає опції `-mllvm` lld: з LTO проходи після
// компонування і кодогенерацію виконує компонувальник.
func mllvmLinkerArgs(conf *Config) []string {
	if !conf.ltoEnabled() {
		return nil
	}
	var args []string
	for _, opt := range conf.MLLVM {
		args = append(args, "-Xlinker", "-mllvm", "-Xlinker", opt)
	}
	return args
}
