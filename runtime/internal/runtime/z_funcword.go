package runtime

import (
	"unsafe"

	c "github.com/xgo-dev/llgo/runtime/internal/clite"
	"github.com/xgo-dev/llgo/runtime/internal/sync/atomic"
)

// funcWordNode починається з пари {fn, ctx}, тож вказівник на вузол і є
// словом значення функції.
type funcWordNode struct {
	fn, ctx unsafe.Pointer
	next    unsafe.Pointer // *funcWordNode
}

var funcWords [256]unsafe.Pointer // *funcWordNode

// FuncWord повертає слово значення функції {fn, ctx}, як у gc: вказівник на
// пару, той самий для тієї самої пари. Так слово читає виклик через нього
// (`*(*func())(unsafe.Pointer(&w))`), а cFuncPointer з modernc.org/sqlite
// порівнює слова. Пара живе до кінця програми: слово зберігають як uintptr
// поза купою. Замикання тримає ctx лише поки живе саме, як і в gc.
func FuncWord(fn, ctx unsafe.Pointer) unsafe.Pointer {
	head := &funcWords[(uintptr(fn)^uintptr(ctx))>>4%uintptr(len(funcWords))]
	for {
		first := atomic.Load(head)
		for n := first; n != nil; n = (*funcWordNode)(n).next {
			if node := (*funcWordNode)(n); node.fn == fn && node.ctx == ctx {
				return n
			}
		}
		node := (*funcWordNode)(c.Malloc(unsafe.Sizeof(funcWordNode{})))
		if node == nil {
			panic("out of memory")
		}
		node.fn, node.ctx, node.next = fn, ctx, first
		if _, ok := atomic.CompareAndExchange(head, first, unsafe.Pointer(node)); ok {
			return unsafe.Pointer(node)
		}
		c.Free(unsafe.Pointer(node))
	}
}
