package plan9asm

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/types"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"

	archcfg "github.com/xgo-dev/llgo/internal/goarch"
	"github.com/xgo-dev/llgo/internal/packages"
	intllvm "github.com/xgo-dev/llgo/internal/xtool/llvm"
	llssaabi "github.com/xgo-dev/llgo/ssa/abi"
	gllvm "github.com/xgo-dev/llvm"
	extplan9asm "github.com/xgo-dev/plan9asm"
)

type FunctionInfo struct {
	TextSymbol     string `json:"text_symbol"`
	ResolvedSymbol string `json:"resolved_symbol"`
}

type FileTranslation struct {
	LLVMIR     string                         `json:"-"`
	Signatures map[string]extplan9asm.FuncSig `json:"signatures"`
	Functions  []FunctionInfo                 `json:"functions"`
}

type ModuleTranslation struct {
	Module     gllvm.Module
	Signatures map[string]extplan9asm.FuncSig
	Functions  []FunctionInfo
	// Untranslated містить функції, які plan9asm не зміг перекласти
	// (розв'язане ім'я → причина). Їх немає в Module, тож посилання на них
	// падає на компоновці.
	Untranslated map[string]string
}

type TranslateOptions struct {
	AnnotateSource bool
	GOARM          string
	GOAMD64        string
	// X87Mode controls explicit 386 x87 assembly lowering. The zero value uses
	// the Go-compatible hardware lowering.
	X87Mode extplan9asm.X87Mode
}

func TranslateFileForPkg(pkg *packages.Package, sfile string, goos string, goarch string, overlay map[string][]byte) (*FileTranslation, error) {
	return TranslateFileForPkgWithOptions(pkg, sfile, goos, goarch, overlay, TranslateOptions{})
}

func TranslateFileForPkgWithOptions(pkg *packages.Package, sfile string, goos string, goarch string, overlay map[string][]byte, opt TranslateOptions) (*FileTranslation, error) {
	if pkg == nil {
		return nil, fmt.Errorf("nil package")
	}
	src, err := ReadFileWithOverlay(overlay, sfile)
	if err != nil {
		return nil, err
	}
	return TranslateSourceForPkgWithOptions(pkg, sfile, src, goos, goarch, opt)
}

func TranslateSourceForPkg(pkg *packages.Package, sfile string, src []byte, goos string, goarch string) (*FileTranslation, error) {
	return TranslateSourceForPkgWithOptions(pkg, sfile, src, goos, goarch, TranslateOptions{})
}

func TranslateSourceForPkgWithOptions(pkg *packages.Package, sfile string, src []byte, goos string, goarch string, opt TranslateOptions) (*FileTranslation, error) {
	tr, err := TranslateSourceModuleForPkgWithOptions(pkg, sfile, src, goos, goarch, opt)
	if err != nil {
		return nil, err
	}
	defer tr.Module.Dispose()
	return &FileTranslation{LLVMIR: tr.Module.String(), Signatures: tr.Signatures, Functions: tr.Functions}, nil
}

func TranslateSourceModuleForPkg(pkg *packages.Package, sfile string, src []byte, goos string, goarch string) (*ModuleTranslation, error) {
	return TranslateSourceModuleForPkgWithOptions(pkg, sfile, src, goos, goarch, TranslateOptions{})
}

func TranslateSourceModuleForPkgWithOptions(pkg *packages.Package, sfile string, src []byte, goos string, goarch string, opt TranslateOptions) (*ModuleTranslation, error) {
	if pkg == nil {
		return nil, fmt.Errorf("nil package")
	}
	if pkg.PkgPath == "" {
		return nil, fmt.Errorf("empty package path")
	}
	if pkg.Types == nil || pkg.Types.Scope() == nil {
		return nil, fmt.Errorf("%s: missing types (needed for asm signatures)", pkg.PkgPath)
	}

	// Match the symbol identity used by LLGo's frontend. In particular, an
	// executable package named main is linked as "main" even when go list
	// reports its module-qualified import path.
	symbolPkgPath := llssaabi.PathOf(pkg.Types)
	resolve := resolveSymFuncForTarget(symbolPkgPath, goos, goarch)
	keep := func(textSym, resolved string) bool {
		return shouldKeepResolvedFunc(pkg.PkgPath, goos, goarch, resolved)
	}
	manualSig := func(resolved string) (extplan9asm.FuncSig, bool) {
		fs, ok := extraAsmSigsAndDeclMap(pkg.PkgPath, goarch)[resolved]
		if ok && fs.Name == "" {
			fs.Name = resolved
		}
		return fs, ok
	}

	imports := make(map[string]*types.Package, len(pkg.Imports))
	for path, imp := range pkg.Imports {
		if imp != nil && imp.Types != nil {
			imports[path] = imp.Types
		}
	}
	if goarch == "amd64" {
		level, err := archcfg.ResolveAMD64(opt.GOAMD64)
		if err != nil {
			return nil, err
		}
		// Match cmd/asm's feature macro, including getGOAMD64level used by
		// internal/cpu to decide which baseline features GODEBUG may disable.
		src = append([]byte("#define GOAMD64_"+level+"\n"), src...)
	}
	// Препроцесор plan9asm ігнорує #include, тож макроси funcdata.h, які
	// лише описують карти вказівників для GC і не генерують коду,
	// визначені порожніми для кожного файлу.
	src = append([]byte("#define GO_ARGS\n#define GO_RESULTS_INITIALIZED\n#define NO_LOCAL_POINTERS\n"), src...)

	gopkg := extplan9asm.GoPackage{
		Path:    symbolPkgPath,
		Types:   pkg.Types,
		Imports: imports,
		Syntax:  pkg.Syntax,
	}
	gopt := extplan9asm.GoModuleOptions{
		FileName:       sfile,
		GOOS:           goos,
		GOARCH:         goarch,
		Sizes:          pkg.TypesSizes,
		FrameSizes:     types.SizesFor("gc", goarch),
		TargetTriple:   intllvm.GetTargetTripleWithGOARM(goos, goarch, opt.GOARM),
		X87Mode:        opt.X87Mode,
		AnnotateSource: opt.AnnotateSource,
		ResolveSym:     resolve,
		KeepFunc:       keep,
		ManualSig:      manualSig,
	}
	tr, err := extplan9asm.TranslateGoModule(gopkg, src, gopt)
	var untranslated map[string]string
	if err != nil {
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%+v\x00%x", symbolPkgPath, sfile, goos, goarch, opt, sha256.Sum256(src))
		v, ok := untranslatedCache.Load(key)
		if !ok {
			v, _ = untranslatedCache.LoadOrStore(key, untranslatableFuncs(gopkg, src, gopt))
		}
		untranslated = v.(map[string]string)
		if untranslated == nil {
			return nil, err
		}
		gopt.KeepFunc = func(textSym, resolved string) bool {
			_, bad := untranslated[resolved]
			return !bad && keep(textSym, resolved)
		}
		tr, err = extplan9asm.TranslateGoModule(gopkg, src, gopt)
		if err != nil && isEmptyFileErr(err) {
			// Жодна функція файлу не перекладається: лишається порожній модуль.
			mod := gllvm.GlobalContext().NewModule(sfile)
			mod.SetTarget(gopt.TargetTriple)
			tr, err = &extplan9asm.GoModuleTranslation{Module: mod}, nil
		}
		if err != nil {
			return nil, err
		}
	}

	funcs := make([]FunctionInfo, 0, len(tr.Functions))
	for _, fn := range tr.Functions {
		funcs = append(funcs, FunctionInfo{TextSymbol: fn.TextSymbol, ResolvedSymbol: fn.ResolvedSymbol})
	}
	return &ModuleTranslation{Module: tr.Module, Signatures: tr.Signatures, Functions: funcs, Untranslated: untranslated}, nil
}

// untranslatedCache пам'ятає результат untranslatableFuncs: той самий файл
// перекладається кілька разів за збірку (компіляція, сигнатури, аналіз
// ініціалізаторів, пояснення помилки компоновки).
var untranslatedCache sync.Map

// untranslatableFuncs знаходить функції файлу, через які падає переклад, і
// повертає їх (розв'язане ім'я → причина). nil означає, що помилку не
// вдається звести до окремих функцій (розбір файлу, взаємодія функцій), і
// збірка має впасти, як раніше.
func untranslatableFuncs(gopkg extplan9asm.GoPackage, src []byte, gopt extplan9asm.GoModuleOptions) map[string]string {
	keep := gopt.KeepFunc
	if keep == nil {
		keep = func(string, string) bool { return true }
	}
	try := func(set []string) error {
		opt := gopt
		opt.KeepFunc = func(textSym, resolved string) bool {
			return slices.Contains(set, resolved) && keep(textSym, resolved)
		}
		tr, err := extplan9asm.TranslateGoModule(gopkg, src, opt)
		if err == nil {
			tr.Module.Dispose()
		}
		return err
	}

	// Порожній набір перевіряє, що файл розбирається, і збирає імена функцій.
	var remaining []string
	probe := gopt
	probe.KeepFunc = func(textSym, resolved string) bool {
		if keep(textSym, resolved) && !slices.Contains(remaining, resolved) {
			remaining = append(remaining, resolved)
		}
		return false
	}
	tr, err := extplan9asm.TranslateGoModule(gopkg, src, probe)
	if err == nil {
		tr.Module.Dispose()
	} else if !isEmptyFileErr(err) {
		return nil
	}

	// isolate звужує набір, що не перекладається, до однієї функції, яка не
	// перекладається й сама.
	var isolate func(set []string) (string, error)
	isolate = func(set []string) (string, error) {
		if len(set) == 1 {
			return set[0], try(set)
		}
		half := len(set) / 2
		for _, part := range [][]string{set[:half], set[half:]} {
			if err := try(part); err != nil {
				return isolate(part)
			}
		}
		return "", nil
	}

	bad := make(map[string]string)
	// Більшість часу спроби займає розбір усього файлу (50 мс на 585 КБ
	// modernc.org/libc, 1875 функцій), а помилка називає одну функцію. Тому
	// спершу кожна функція перекладається з власного короткого фрагмента;
	// цикл нижче перевіряє решту разом і доловлює те, що фрагменти пропустили.
	common, bodies := splitFuncs(src)
	for _, body := range bodies {
		name := ""
		opt := gopt
		opt.KeepFunc = func(textSym, resolved string) bool {
			name = resolved
			return keep(textSym, resolved)
		}
		tr, err := extplan9asm.TranslateGoModule(gopkg, append(slices.Clip(common), body...), opt)
		if err == nil {
			tr.Module.Dispose()
			continue
		}
		if _, reason, _ := strings.Cut(err.Error(), name+": "); name != "" && reason != "" {
			bad[name] = reason
		}
	}
	remaining = slices.DeleteFunc(remaining, func(n string) bool {
		_, ok := bad[n]
		return ok
	})

	for len(remaining) != 0 {
		err := try(remaining)
		if err == nil {
			return bad
		}
		// Помилка зазвичай називає функцію; перевіряємо її окремо, бо помилка
		// сигнатури виклику називає не функцію, а того, кого вона викликає.
		name := ""
		for _, n := range remaining {
			if strings.Contains(err.Error(), n+": ") && len(n) > len(name) {
				name = n
			}
		}
		var soloErr error
		if name != "" {
			soloErr = try([]string{name})
		}
		if soloErr == nil {
			name, soloErr = isolate(remaining)
		}
		// Помилка, що не називає функцію (наприклад, недійсний режим x87),
		// стосується всього файлу.
		reason := ""
		if soloErr != nil {
			_, reason, _ = strings.Cut(soloErr.Error(), name+": ")
		}
		if reason == "" {
			return nil
		}
		bad[name] = reason
		remaining = slices.DeleteFunc(remaining, func(n string) bool { return n == name })
	}
	return bad
}

// splitFuncs ділить файл на спільні рядки (усе до першого TEXT, #include,
// #define, DATA, GLOBL) і тіла функцій від TEXT до наступного TEXT.
// Умовна компіляція і #undef залежать від місця в файлі, тож для такого
// файлу поділу немає.
func splitFuncs(src []byte) (common []byte, bodies [][]byte) {
	inDefine := false
	for line := range bytes.Lines(src) {
		stmt := string(bytes.TrimSpace(line))
		if inDefine || strings.HasPrefix(stmt, "#") {
			if !inDefine && !strings.HasPrefix(stmt, "#include") && !strings.HasPrefix(stmt, "#define") {
				return nil, nil
			}
			inDefine = strings.HasSuffix(stmt, `\`)
			common = append(common, line...)
			continue
		}
		op, _, _ := strings.Cut(strings.ReplaceAll(stmt, "\t", " "), " ")
		switch {
		case op == "TEXT":
			bodies = append(bodies, slices.Clone(line))
		case op == "DATA" || op == "GLOBL" || len(bodies) == 0:
			common = append(common, line...)
		default:
			bodies[len(bodies)-1] = append(bodies[len(bodies)-1], line...)
		}
	}
	return common, bodies
}

// isEmptyFileErr повідомляє, що перекладачу не лишилося жодної функції:
// TranslateModule відмовляється створювати порожній модуль.
func isEmptyFileErr(err error) bool {
	return strings.HasSuffix(err.Error(), ": empty file")
}

func shouldKeepResolvedFunc(pkgPath, goos, goarch, resolved string) bool {
	if pkgPath == "syscall" && goos == "linux" && (goarch == "arm64" || goarch == "amd64") && strings.HasSuffix(resolved, "rawVforkSyscall") {
		return false
	}
	return true
}

func FilterFuncs(pkgPath, goos, goarch string, funcs []extplan9asm.Func, resolve func(sym string) string) []extplan9asm.Func {
	if len(funcs) == 0 {
		return funcs
	}
	keep := funcs[:0]
	for _, fn := range funcs {
		resolved := resolve(StripABISuffix(fn.Sym))
		if shouldKeepResolvedFunc(pkgPath, goos, goarch, resolved) {
			keep = append(keep, fn)
		}
	}
	return keep
}

func ResolveSymFunc(pkgPath string) func(sym string) string {
	return resolveSymFuncForTarget(pkgPath, "", "")
}

func resolveSymFuncForTarget(pkgPath, goos, goarch string) func(sym string) string {
	return func(sym string) string {
		sym = StripABISuffix(sym)
		if strings.HasPrefix(sym, "·") {
			return pkgPath + "." + strings.TrimPrefix(sym, "·")
		}
		if !strings.Contains(sym, "·") && !strings.Contains(sym, ".") && !strings.Contains(sym, "/") {
			return pkgPath + "." + sym
		}
		sym = strings.ReplaceAll(sym, "∕", "/")
		return strings.ReplaceAll(sym, "·", ".")
	}
}

func ReadFileWithOverlay(overlay map[string][]byte, path string) ([]byte, error) {
	if overlay != nil {
		if b, ok := overlay[path]; ok {
			return b, nil
		}
	}
	return os.ReadFile(path)
}

func HasAnyTextAsm(overlay map[string][]byte, files []string) (bool, error) {
	for _, f := range files {
		src, err := ReadFileWithOverlay(overlay, f)
		if err != nil {
			return false, err
		}
		if reTextDirective.Match(src) {
			return true, nil
		}
	}
	return false, nil
}

var abiSuffixRe = regexp.MustCompile(`<ABI[^>]*>$`)
var reTextDirective = regexp.MustCompile(`(?m)^\s*TEXT\b`)

func StripABISuffix(sym string) string {
	sym = abiSuffixRe.ReplaceAllString(sym, "")
	return strings.TrimSuffix(sym, "<>")
}

func extraAsmSigsAndDeclMap(pkgPath string, goarch string) map[string]extplan9asm.FuncSig {
	manual := map[string]extplan9asm.FuncSig{
		"runtime.memmove": {
			Name: "memmove",
			Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.Ptr, extplan9asm.I64},
			Ret:  extplan9asm.Ptr,
		},
	}
	if pkgPath == "internal/bytealg" {
		switch goarch {
		case "386":
			manual["internal/bytealg.cmpbody"] = extplan9asm.FuncSig{
				Args:    []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.I32, extplan9asm.Ptr, extplan9asm.I32, extplan9asm.Ptr},
				Ret:     extplan9asm.Void,
				ArgRegs: []extplan9asm.Reg{extplan9asm.SI, extplan9asm.BX, extplan9asm.DI, extplan9asm.DX, extplan9asm.AX},
			}
			manual["internal/bytealg.memeqbody"] = extplan9asm.FuncSig{
				Args:    []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.Ptr, extplan9asm.I32, extplan9asm.Ptr},
				Ret:     extplan9asm.Void,
				ArgRegs: []extplan9asm.Reg{extplan9asm.SI, extplan9asm.DI, extplan9asm.BX, extplan9asm.AX},
			}
		case "arm":
			manual["internal/bytealg.cmpbody"] = extplan9asm.FuncSig{
				Args:    []extplan9asm.LLVMType{extplan9asm.Ptr, "i32", extplan9asm.Ptr, "i32", extplan9asm.Ptr},
				Ret:     extplan9asm.Void,
				ArgRegs: []extplan9asm.Reg{"R2", "R0", "R3", "R1", "R7"},
			}
			manual["internal/bytealg.memeqbody"] = extplan9asm.FuncSig{
				Args:    []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.Ptr, "i32", extplan9asm.Ptr},
				Ret:     extplan9asm.Void,
				ArgRegs: []extplan9asm.Reg{"R0", "R2", "R1", "R7"},
			}
			manual["internal/bytealg.countbytebody"] = extplan9asm.FuncSig{
				Args:    []extplan9asm.LLVMType{extplan9asm.Ptr, "i32", "i8", extplan9asm.Ptr},
				Ret:     extplan9asm.Void,
				ArgRegs: []extplan9asm.Reg{"R0", "R1", "R2", "R7"},
			}
			manual["internal/bytealg.indexbytebody"] = extplan9asm.FuncSig{
				Args:    []extplan9asm.LLVMType{extplan9asm.Ptr, "i32", "i8", extplan9asm.Ptr},
				Ret:     extplan9asm.Void,
				ArgRegs: []extplan9asm.Reg{"R0", "R1", "R2", "R5"},
			}
		case "arm64":
			manual["internal/bytealg.cmpbody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.I64, extplan9asm.Ptr, extplan9asm.I64}, Ret: extplan9asm.I64}
			manual["internal/bytealg.memeqbody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.Ptr, extplan9asm.I64}, Ret: extplan9asm.I1}
			manual["internal/bytealg.countbytebody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.I64, extplan9asm.LLVMType("i8"), extplan9asm.Ptr}, Ret: extplan9asm.Void, ArgRegs: []extplan9asm.Reg{"R0", "R2", "R1", "R8"}}
			manual["internal/bytealg.indexbody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.I64, extplan9asm.Ptr, extplan9asm.I64, extplan9asm.Ptr}, Ret: extplan9asm.Void, ArgRegs: []extplan9asm.Reg{"R0", "R1", "R2", "R3", "R9"}}
			manual["internal/bytealg.indexbytebody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.LLVMType("i8"), extplan9asm.I64, extplan9asm.Ptr}, Ret: extplan9asm.Void, ArgRegs: []extplan9asm.Reg{"R0", "R1", "R2", "R8"}}
		case "amd64":
			manual["internal/bytealg.cmpbody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.Ptr, extplan9asm.I64, extplan9asm.I64}, Ret: extplan9asm.I64, ArgRegs: []extplan9asm.Reg{extplan9asm.SI, extplan9asm.DI, extplan9asm.BX, extplan9asm.DX}}
			manual["internal/bytealg.countbody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.I64, extplan9asm.LLVMType("i8"), extplan9asm.Ptr}, Ret: extplan9asm.Void, ArgRegs: []extplan9asm.Reg{extplan9asm.SI, extplan9asm.BX, extplan9asm.AX, extplan9asm.Reg("R8")}}
			manual["internal/bytealg.indexbody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.I64, extplan9asm.Ptr, extplan9asm.I64, extplan9asm.Ptr, extplan9asm.Ptr}, Ret: extplan9asm.Void, ArgRegs: []extplan9asm.Reg{extplan9asm.DI, extplan9asm.DX, extplan9asm.Reg("R8"), extplan9asm.AX, extplan9asm.Reg("R10"), extplan9asm.Reg("R11")}}
			manual["internal/bytealg.indexbytebody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.I64, extplan9asm.LLVMType("i8"), extplan9asm.Ptr}, Ret: extplan9asm.Void, ArgRegs: []extplan9asm.Reg{extplan9asm.SI, extplan9asm.BX, extplan9asm.AX, extplan9asm.Reg("R8")}}
			manual["internal/bytealg.memeqbody"] = extplan9asm.FuncSig{Args: []extplan9asm.LLVMType{extplan9asm.Ptr, extplan9asm.Ptr, extplan9asm.I64}, Ret: extplan9asm.I1, ArgRegs: []extplan9asm.Reg{extplan9asm.SI, extplan9asm.DI, extplan9asm.BX}}
		}
	}
	if pkgPath == "internal/runtime/gc/scan" && goarch == "amd64" {
		// Go 1.26 emits local AVX512 scan expanders with no Go declarations.
		// They still need signatures so the translator can keep the asm entry
		// points and wire the packed mask argument through AX.
		for _, n := range []int{
			1, 2, 3, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32,
			36, 40, 44, 48, 52, 56, 60, 64,
		} {
			name := fmt.Sprintf("internal/runtime/gc/scan.expandAVX512_%d", n)
			manual[name] = extplan9asm.FuncSig{
				Args:    []extplan9asm.LLVMType{extplan9asm.Ptr},
				Ret:     extplan9asm.Void,
				ArgRegs: []extplan9asm.Reg{extplan9asm.AX},
			}
		}
	}
	return manual
}
