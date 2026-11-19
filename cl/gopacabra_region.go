package cl

import (
	"go/token"
	"go/types"

	llssa "github.com/xgo-dev/llgo/ssa"
	"golang.org/x/tools/go/ssa"
)

// gopacabraInRegionClone: зараз компілюється тіло клону F$region будівника.
func (p *context) gopacabraInRegionClone() bool {
	return !p.gopacabraRegionParam.IsNil()
}

// gopacabraRegionFunc повертає клон F$region будівника f (P-NEW-05.4): регіон —
// перший параметр, далі отримувач і параметри f. Пакет будівника клон
// визначає, решта лише оголошує й кличе за іменем.
func (p *context) gopacabraRegionFunc(f *ssa.Function) llssa.Function {
	_, name, ftype := p.funcName(f)
	name += "$region"
	if fn := p.pkg.FuncOf(name); fn != nil {
		return fn
	}
	oldGoFn := p.goFn
	p.goFn = f
	sig := p.patchType(f.Signature).(*types.Signature)
	p.goFn = oldGoFn
	params := []*types.Var{types.NewParam(token.NoPos, nil, "$region", types.Typ[types.UnsafePointer])}
	if recv := sig.Recv(); recv != nil {
		params = append(params, recv)
	}
	for param := range sig.Params().Variables() {
		params = append(params, param)
	}
	cloneSig := types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), sig.Results(), sig.Variadic())
	return p.pkg.NewFuncEx(name, cloneSig, llssa.CallConv(ftype), false, p.needsLinkOnce(f))
}

// gopacabraRegionCall повертає регіон для виклику: корінь (P-NEW-05.2) створює
// новий, а виклик будівника в клоні передає регіон клону далі.
func (p *context) gopacabraRegionCall(b llssa.Builder, call *ssa.Call) (llssa.Expr, bool) {
	if p.gopacabraInRegionClone() {
		return p.gopacabraRegionParam, p.gopacabraBuilders[call.Call.StaticCallee()]
	}
	if _, root := p.gopacabraRegions[call]; !root {
		return llssa.Nil, false
	}
	// Без точки смерті (корінь читає defer) FreeRegion нікуди поставити, а
	// незвільнений регіон тримав би цілий шматок замість кількох вузлів.
	if _, dies := p.gopacabraDecUses[call]; !dies {
		return llssa.Nil, false
	}
	p.countGopacabraSite(b, call, "region_create")
	sig := types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "region", types.Typ[types.UnsafePointer])), false)
	region := b.Call(p.runtimeFunc("RegionCreate", sig))
	if p.gopacabraRegionOf == nil {
		p.gopacabraRegionOf = map[ssa.Value]llssa.Expr{}
	}
	p.gopacabraRegionOf[call] = region
	return region, true
}

// callGopacabraRegion кличе клон будівника з регіоном першим аргументом.
func (p *context) callGopacabraRegion(b llssa.Builder, call *ssa.Call, region llssa.Expr) llssa.Expr {
	clone := p.gopacabraRegionFunc(call.Call.StaticCallee())
	args := append([]llssa.Expr{region}, p.compileValues(b, call.Call.Args, fnNormal)...)
	return b.Call(clone.Expr, args...)
}

// emitRegionAlloc виділяє вузол у регіоні клону: пам'ять уже обнулена, слова
// лічильника немає.
func (p *context) emitRegionAlloc(b llssa.Builder, elem llssa.Type) llssa.Expr {
	unsafePtr := types.Typ[types.UnsafePointer]
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(
			types.NewVar(token.NoPos, nil, "region", unsafePtr),
			types.NewVar(token.NoPos, nil, "size", types.Typ[types.Uintptr])),
		types.NewTuple(types.NewVar(token.NoPos, nil, "ptr", unsafePtr)),
		false)
	raw := b.Call(p.runtimeFunc("RegionAlloc", sig), p.gopacabraRegionParam, p.prog.IntVal(p.prog.SizeOf(elem), p.prog.Uintptr()))
	return b.PtrCast(p.prog.Pointer(elem), raw)
}

func (p *context) callFreeRegion(b llssa.Builder, region llssa.Expr) {
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "region", types.Typ[types.UnsafePointer])), nil, false)
	b.Call(p.runtimeFunc("FreeRegion", sig), region)
}
