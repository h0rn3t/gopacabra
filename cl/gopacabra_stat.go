package cl

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"github.com/xgo-dev/llgo/gopacabra/census"
	llssa "github.com/xgo-dev/llgo/ssa"
	"github.com/xgo-dev/llvm"
	"golang.org/x/tools/go/ssa"
)

// gopacabraSiteTable — рядки таблиці gopacabra site пакета. Рядок — лічильник і
// текст «file:line категорія причина операція»; рядки з тим самим текстом
// ділять лічильник. Розкладка глобальної таблиці збігається з
// runtime.siteTable.
type gopacabraSiteTable struct {
	counters map[string]llssa.Global
	rows     []llssa.Expr
}

// countGopacabraSite додає одиницю лічильнику рядка (site, op) під GopacabraStat.
func (p *context) countGopacabraSite(b llssa.Builder, site ssa.Value, op string) {
	if !p.options.GopacabraStat {
		return
	}
	text := p.gopacabraSiteText(site, op)
	t := &p.gopacabraSites
	counter, ok := t.counters[text]
	if !ok {
		if t.counters == nil {
			t.counters = map[string]llssa.Global{}
		}
		name := fmt.Sprintf("%s.__gopacabra_site$%d", p.pkg.Path(), len(t.counters))
		counter = p.pkg.NewVarEx(name, p.prog.Pointer(p.prog.Uint64()))
		counter.InitNil()
		t.counters[text] = counter
		t.rows = append(t.rows, p.prog.ConstStruct(p.gopacabraSiteRowType(), []llssa.Expr{
			counter.Expr,
			p.pkg.ConstString(text),
		}))
	}
	b.Atomic(llssa.OpAdd, counter.Expr, p.prog.IntVal(1, p.prog.Uint64()))
}

// gopacabraSiteText — «file:line категорія причина операція». Місце без
// позначки census — unplaced; позначка без причини дає «-».
func (p *context) gopacabraSiteText(site ssa.Value, op string) string {
	pos := site.Pos()
	if closure, ok := site.(*ssa.MakeClosure); ok && !pos.IsValid() {
		if fn, ok := closure.Fn.(*ssa.Function); ok {
			pos = fn.Pos()
		}
	}
	cat, reason := "unplaced", "-"
	if instr, ok := site.(ssa.Instruction); ok {
		if label, ok := p.gopacabraLabel[instr]; ok {
			head, tail, found := strings.Cut(label, ": ")
			cat = head
			if found {
				reason = strings.ReplaceAll(tail, " ", "_")
			}
		}
	}
	position := p.fset.Position(pos)
	return fmt.Sprintf("%s:%d %s %s %s", position.Filename, position.Line, cat, reason, op)
}

// countGopacabraAlloc рахує купову алокацію місця: alloc для AllocZ, AllocU і
// виклику рантайму, що виділяє, alloc_counted для AllocCounted. append без
// власного буфера виділяє лише при рості, тож не рахується.
func (p *context) countGopacabraAlloc(b llssa.Builder, instr ssa.Instruction) {
	if !p.options.GopacabraStat || !census.IsSite(instr) || p.gopacabraLabel[instr] == "stack" || p.gopacabraLabel[instr] == gopacabraConstLabel {
		return
	}
	val := instr.(ssa.Value)
	if expr, ok := p.bvals[val]; !ok || expr.IsConstant() {
		return
	}
	op := "alloc"
	switch instr := instr.(type) {
	case *ssa.Alloc:
		if !instr.Heap {
			return
		}
		if n, _ := p.gopacabraSiteSize(val); n > 0 && p.isGopacabraCounted(instr) {
			op = "alloc_counted"
		}
		if p.gopacabraInRegionClone() {
			op = "alloc_region"
		}
	case *ssa.Call:
		name, ok := p.gopacabraAppendAlloc(instr)
		if !ok {
			return
		}
		if name == "AllocCounted" {
			op = "alloc_counted"
		}
	default:
		if p.isGopacabraCounted(instr) {
			op = "alloc_counted"
		}
	}
	p.countGopacabraSite(b, val, op)
}

func (p *context) isGopacabraCounted(instr ssa.Instruction) bool {
	_, ok := p.gopacabraCounted[instr]
	return ok
}

// finishGopacabraSites, коли всі функції пакета вже зібрані, дає таблиці рядки
// і конструктор модуля, що кличе runtime.RegisterSites. Не init пакета:
// пакети LLGoPackage = "link" init не мають, а лічильники мають.
func (p *context) finishGopacabraSites() {
	t := &p.gopacabraSites
	if len(t.rows) == 0 {
		return
	}
	path := p.pkg.Path()
	tableT := p.gopacabraSiteTableType()
	table := p.pkg.NewVarEx(path+".__gopacabra_sites", p.prog.Pointer(tableT))
	table.Init(p.prog.ConstStruct(tableT, []llssa.Expr{
		p.prog.Nil(p.type_(types.Typ[types.UnsafePointer], llssa.GoDecl)),
		p.pkg.ConstSlice(path+".__gopacabra_sites$rows", p.prog.Slice(p.gopacabraSiteRowType()), t.rows),
	}))

	ctorName := path + ".__gopacabra_sites$ctor"
	b := p.pkg.NewFunc(ctorName, llssa.NoArgsNoRet, llssa.CDecl).MakeBody(1)
	unsafePtr := types.Typ[types.UnsafePointer]
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "table", unsafePtr)), nil, false)
	b.Call(p.runtimeFunc("RegisterSites", sig), b.PtrCast(p.type_(unsafePtr, llssa.GoDecl), table.Expr))
	b.Return()

	mod := p.pkg.Module()
	ctor := mod.NamedFunction(ctorName)
	ctor.SetLinkage(llvm.InternalLinkage)
	if p.prog.Target().GOOS == "linux" {
		// Як defineLibraryRuntimeInit у internal/build: llvm.global_ctors
		// цільова машина на Linux опускає в застарілу секцію .ctors.
		entry := llvm.AddGlobal(mod, ctor.Type(), path+".__gopacabra_sites$init")
		entry.SetInitializer(ctor)
		entry.SetGlobalConstant(true)
		entry.SetSection(".init_array")
		entry.SetVisibility(llvm.HiddenVisibility)
		// Запис ніхто не читає: без compiler.used його викидає GlobalDCE під -lto.
		p.pkg.MarkLLVMUsed(entry)
		return
	}
	ctx := mod.Context()
	var entries []llvm.Value
	if old := mod.NamedGlobal("llvm.global_ctors"); !old.IsNil() {
		// llvm.global_ctors у модулі один: старі записи переходять у новий.
		init := old.Initializer()
		for i := range init.OperandsCount() {
			entries = append(entries, init.Operand(i))
		}
		old.EraseFromParentAsGlobal()
	}
	entries = append(entries, ctx.ConstStruct([]llvm.Value{
		llvm.ConstInt(ctx.Int32Type(), 65535, false),
		ctor,
		llvm.ConstNull(ctor.Type()),
	}, false))
	array := llvm.ConstArray(entries[0].Type(), entries)
	ctors := llvm.AddGlobal(mod, array.Type(), "llvm.global_ctors")
	ctors.SetInitializer(array)
	ctors.SetLinkage(llvm.AppendingLinkage)
}

// gopacabraSiteRowType — runtime.siteRow: лічильник і текст рядка.
func (p *context) gopacabraSiteRowType() llssa.Type {
	return p.type_(types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "N", types.NewPointer(types.Typ[types.Uint64]), false),
		types.NewField(token.NoPos, nil, "Text", types.Typ[types.String], false),
	}, nil), llssa.GoDecl)
}

// gopacabraSiteTableType — runtime.siteTable: наступна таблиця і рядки.
func (p *context) gopacabraSiteTableType() llssa.Type {
	row := p.gopacabraSiteRowType().RawType()
	return p.type_(types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "Next", types.Typ[types.UnsafePointer], false),
		types.NewField(token.NoPos, nil, "Rows", types.NewSlice(row), false),
	}, nil), llssa.GoDecl)
}
