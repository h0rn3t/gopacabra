package census

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// TestOverlaySummary — P12.5.7.1: виклик функції std, яку збірка компілює з
// тіла оверлея (patch.Alt), бере зведення і свіжість оверлея. Тут std-двійник
// позичає параметр і повертає свіжий об'єкт, а оверлей утримує параметр і
// повертає глобальний: за зведенням std census звільнив би живий аргумент, а
// викликач Get зменшив би слово, якого New не писав.
func TestOverlaySummary(t *testing.T) {
	t.Cleanup(func() { PublishProgram(nil, nil) })
	std, alt, app := buildOverlayProgram(t)
	useKeep := app.Func("UseKeep")
	useMethod := app.Func("UseMethod")
	get := app.Func("Get")
	funcs := []*ssa.Function{useKeep, useMethod}
	allocs := map[*ssa.Function]ssa.Instruction{
		useKeep:   heapAlloc(t, useKeep),
		useMethod: heapAlloc(t, useMethod),
	}

	// Без оверлеїв діє тіло std: тест розрізняє два зведення.
	PublishProgram(app.Prog, nil)
	cats, staticUses, decUses, _, _ := Place(funcs)
	for _, fn := range funcs {
		if freed, label := freedAlloc(cats, staticUses, decUses, allocs[fn]); !freed {
			t.Fatalf("без оверлея Place(%s) = %s, want звільнення: std-двійник позичає", fn.Name(), label)
		}
	}
	if !packageSummaries(nil, []*ssa.Function{get})[get].Fresh {
		t.Fatal("без оверлея packageSummaries(Get).Fresh = false, want true: std New свіжий")
	}

	PublishProgram(app.Prog, map[string]*ssa.Package{std.Pkg.Path(): alt})
	cats, staticUses, decUses, _, _ = Place(funcs)
	for _, fn := range funcs {
		if freed, label := freedAlloc(cats, staticUses, decUses, allocs[fn]); freed {
			t.Errorf("Place(%s) = %s, want живий аргумент: оверлей його утримує", fn.Name(), label)
		}
	}
	if packageSummaries(nil, []*ssa.Function{get})[get].Fresh {
		t.Error("packageSummaries(Get).Fresh = true, want false: New оверлея повертає глобальний об'єкт")
	}
}

func buildOverlayProgram(t *testing.T) (std, alt, app *ssa.Package) {
	t.Helper()
	const stdSrc = `package lib

type T struct{}

func Keep(p *int) { _ = *p }

func (T) Keep(p *int) { _ = *p }

func New() *int { return new(int) }
`
	const altSrc = `package lib

var held *int

type T struct{}

func Keep(p *int) { held = p }

func (T) Keep(p *int) { held = p }

func New() *int { return held }
`
	const appSrc = `package app

import "example.com/lib"

func UseKeep() int {
	p := new(int)
	*p = 1
	lib.Keep(p)
	return *p
}

func UseMethod() int {
	p := new(int)
	*p = 1
	lib.T{}.Keep(p)
	return *p
}

func Get() *int { return lib.New() }
`
	fset := token.NewFileSet()
	prog := ssa.NewProgram(fset, ssa.SanityCheckFunctions)
	create := func(path, src string, imp types.Importer) *ssa.Package {
		t.Helper()
		file, err := parser.ParseFile(fset, path+"/x.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		info := &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Implicits:  map[ast.Node]types.Object{},
			Scopes:     map[ast.Node]*types.Scope{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
		pkg := types.NewPackage(path, file.Name.Name)
		if err := types.NewChecker(&types.Config{Importer: imp}, fset, pkg, info).Files([]*ast.File{file}); err != nil {
			t.Fatal(err)
		}
		return prog.CreatePackage(pkg, []*ast.File{file}, info, true)
	}
	std = create("example.com/lib", stdSrc, nil)
	alt = create("example.com/alt/lib", altSrc, nil)
	app = create("example.com/app", appSrc, importerFunc(func(path string) (*types.Package, error) {
		if path == "example.com/lib" {
			return std.Pkg, nil
		}
		return nil, fmt.Errorf("немає пакета %s", path)
	}))
	prog.Build()
	return std, alt, app
}
