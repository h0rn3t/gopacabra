//go:build !darwin && !linux && !wasm

package runtime

import c "github.com/xgo-dev/llgo/runtime/internal/clite"

const SIGBUS = c.Int(0)
