package census

import (
	"go/constant"
	"go/types"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/ssa"
)

// fmtPrintf — функції fmt, у яких перший параметр — рядок формату, а другий —
// варіадичні аргументи. Результат — новий рядок чи нова помилка з ним.
var fmtPrintf = map[string]bool{"fmt.Sprintf": true, "fmt.Errorf": true}

// fmtCalled розбирає виклик fmt.Sprintf чи fmt.Errorf з константним форматом
// і зрізом аргументів, який SSA збирає на місці виклику. Повертає
// аргументи, друк яких може викликати метод значення (String, Error, Format)
// чи утримати його в результаті (%w). Решту fmt лише читає: %T друкує тип,
// базовий тип без методів друкується без коду програми. ok=false — форму не
// розібрано, і виклик іде загальним правилом.
func fmtCalled(call *ssa.CallCommon) (called []ssa.Value, ok bool) {
	fn := call.StaticCallee()
	if fn == nil || !fmtPrintf[fn.String()] || len(call.Args) != 2 {
		return nil, false
	}
	format, isConst := call.Args[0].(*ssa.Const)
	if !isConst || format.Value == nil || format.Value.Kind() != constant.String {
		return nil, false
	}
	verbs, ok := fmtVerbs(constant.StringVal(format.Value))
	if !ok {
		return nil, false
	}
	args, ok := varargs(call.Args[1])
	if !ok {
		return nil, false
	}
	for i, arg := range args {
		if arg == nil {
			continue
		}
		// Зайвий аргумент fmt друкує як %v (%!(EXTRA …)).
		verb := 'v'
		if i < len(verbs) {
			verb = verbs[i]
		}
		if verb == 'T' || verb == '*' || plainPrinted(arg) {
			continue
		}
		called = append(called, arg)
	}
	return called, true
}

// fmtVerbs повертає дієслова формату в порядку аргументів; '*' — ширина чи
// точність з аргументу, яку fmt читає як ціле без виклику методів. Явний
// індекс аргументу ([n]) не розбирається.
func fmtVerbs(format string) ([]rune, bool) {
	var verbs []rune
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		for i < len(format) && strings.IndexByte("+-# 0", format[i]) >= 0 {
			i++
		}
	width:
		for ; i < len(format); i++ {
			switch c := format[i]; {
			case c == '[':
				return nil, false
			case c == '*':
				verbs = append(verbs, '*')
			case c != '.' && (c < '0' || c > '9'):
				break width
			}
		}
		if i == len(format) {
			break
		}
		verb, size := utf8.DecodeRuneInString(format[i:])
		i += size - 1
		if verb != '%' {
			verbs = append(verbs, verb)
		}
	}
	return verbs, true
}

// varargs повертає значення, покладені в масив варіадичного зрізу, за
// індексами. ok=false, якщо масив пишуть чи читають ще десь, крім цього
// виклику: тоді його вміст на момент виклику невідомий.
func varargs(v ssa.Value) ([]ssa.Value, bool) {
	if c, isConst := v.(*ssa.Const); isConst && c.IsNil() {
		return nil, true
	}
	slice, ok := v.(*ssa.Slice)
	if !ok || len(*slice.Referrers()) != 1 {
		return nil, false
	}
	alloc, ok := slice.X.(*ssa.Alloc)
	if !ok {
		return nil, false
	}
	arr, ok := types.Unalias(alloc.Type()).(*types.Pointer).Elem().Underlying().(*types.Array)
	if !ok {
		return nil, false
	}
	vals := make([]ssa.Value, arr.Len())
	for _, ref := range *alloc.Referrers() {
		switch ref := ref.(type) {
		case *ssa.Slice:
			if ref != slice {
				return nil, false
			}
		case *ssa.DebugRef:
		case *ssa.IndexAddr:
			idx, isConst := ref.Index.(*ssa.Const)
			if !isConst {
				return nil, false
			}
			i := idx.Int64()
			for _, use := range *ref.Referrers() {
				st, isStore := use.(*ssa.Store)
				if !isStore || st.Addr != ref || vals[i] != nil {
					return nil, false
				}
				vals[i] = st.Val
			}
		default:
			return nil, false
		}
	}
	return vals, true
}

// plainPrinted: v — значення базового типу без методів, перетворене в
// інтерфейс на місці виклику. fmt друкує його без коду програми за будь-якого
// дієслова.
func plainPrinted(v ssa.Value) bool {
	mi, ok := v.(*ssa.MakeInterface)
	if !ok {
		return false
	}
	t := mi.X.Type()
	if _, basic := t.Underlying().(*types.Basic); !basic {
		return false
	}
	return types.NewMethodSet(t).Len() == 0 && types.NewMethodSet(types.NewPointer(t)).Len() == 0
}
