// Package testpolicy checks, by machine, the testing rules of this module:
// every Test function is table-driven (it ranges over a table and calls t.Run
// inside the loop). A rule that only lives in a README decays; this one fails
// the build.
package testpolicy_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// tableDriven reports whether fn ranges over something and calls <t>.Run in
// the loop body, where <t> is the *testing.T parameter.
func tableDriven(fn *ast.FuncDecl) bool { return runLoop(fn) != nil }

// runLoop returns the range statement whose body calls <t>.Run, or nil.
func runLoop(fn *ast.FuncDecl) *ast.RangeStmt {
	if len(fn.Type.Params.List) != 1 || len(fn.Type.Params.List[0].Names) != 1 {
		return nil
	}
	t := fn.Type.Params.List[0].Names[0].Name
	var found *ast.RangeStmt
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		loop, ok := n.(*ast.RangeStmt)
		if !ok || found != nil {
			return found == nil
		}
		ast.Inspect(loop.Body, func(m ast.Node) bool {
			if call, ok := m.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == t {
						found = loop
					}
				}
			}
			return found == nil
		})
		return found == nil
	})
	return found
}

// tableSize returns the number of cases of the table the Run loop ranges
// over, when the table is a composite literal written in the function
// (directly, or assigned to the ranged variable). A table built any other way
// (a function call, a loop) is not counted: known is false.
func tableSize(fn *ast.FuncDecl) (n int, known bool) {
	loop := runLoop(fn)
	if loop == nil {
		return 0, false
	}
	if lit, ok := loop.X.(*ast.CompositeLit); ok {
		return len(lit.Elts), true
	}
	id, ok := loop.X.(*ast.Ident)
	if !ok {
		return 0, false
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				if l, ok := lhs.(*ast.Ident); ok && l.Name == id.Name && i < len(x.Rhs) {
					if lit, ok := x.Rhs[i].(*ast.CompositeLit); ok {
						n, known = len(lit.Elts), true
					}
				}
			}
		case *ast.ValueSpec:
			for i, name := range x.Names {
				if name.Name == id.Name && i < len(x.Values) {
					if lit, ok := x.Values[i].(*ast.CompositeLit); ok {
						n, known = len(lit.Elts), true
					}
				}
			}
		}
		return true
	})
	return n, known
}

// minCases is the smallest table worth the name: one case is a plain test
// in a loop, and a boundary needs a case on each side.
const minCases = 2

// violations lists "file:TestName" for every Test function that is not table-driven.
func violations(t *testing.T, root string) []string {
	t.Helper()
	return scan(t, root, func(fn *ast.FuncDecl) bool { return !tableDriven(fn) })
}

// smallTables lists "file:TestName" for every Test function whose table has
// fewer than minCases cases.
func smallTables(t *testing.T, root string) []string {
	t.Helper()
	return scan(t, root, func(fn *ast.FuncDecl) bool {
		n, known := tableSize(fn)
		return known && n < minCases
	})
}

// scan lists "file:TestName" for every Test function flag reports.
func scan(t *testing.T, root string, flag func(*ast.FuncDecl) bool) []string {
	t.Helper()
	var out []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == "tools" || d.Name() == "dist" || strings.HasPrefix(d.Name(), ".")) && path != root:
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(path, "_test.go"):
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" {
				continue
			}
			if flag(fn) {
				rel, _ := filepath.Rel(root, path)
				out = append(out, rel+":"+fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func TestEveryTestIsTableDriven(t *testing.T) {
	good := `package x
import "testing"
func TestGood(t *testing.T) {
	tests := []struct{ name string }{{name: "a"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}`
	bad := `package x
import "testing"
func TestBad(t *testing.T) { if 1 != 1 { t.Fatal() } }
func TestLoopWithoutRun(t *testing.T) { for range 3 {} }
func TestMain(m *testing.M) {}`
	fixture := func(t *testing.T, src string) string {
		t.Helper()
		dir := t.TempDir()
		if err := writeFile(filepath.Join(dir, "x_test.go"), src); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	tests := []struct {
		name string
		root func(t *testing.T) string
		want []string
	}{
		// Controls: the detector passes a table-driven test and flags the others.
		{name: "control: table-driven passes", root: func(t *testing.T) string { t.Helper(); return fixture(t, good) }, want: nil},
		{
			name: "control: plain tests are flagged", root: func(t *testing.T) string { t.Helper(); return fixture(t, bad) },
			want: []string{"x_test.go:TestBad", "x_test.go:TestLoopWithoutRun"},
		},
		// The module itself.
		{name: "module", root: func(*testing.T) string { return filepath.Join("..", "..") }, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, violations(t, tt.root(t))); diff != "" {
				t.Errorf("non-table-driven tests (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEveryTableHasTwoCases(t *testing.T) {
	src := `package x
import "testing"
func TestTwo(t *testing.T) {
	tests := []struct{ name string }{{name: "a"}, {name: "b"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}
func TestOneAssigned(t *testing.T) {
	tests := []struct{ name string }{{name: "a"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}
func TestOneDeclared(t *testing.T) {
	var tests = []string{"a"}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {})
	}
}
func TestOneInline(t *testing.T) {
	for _, tt := range []string{"a"} {
		t.Run(tt, func(t *testing.T) {})
	}
}
func TestBuilt(t *testing.T) {
	for _, tt := range cases() {
		t.Run(tt, func(t *testing.T) {})
	}
}
func TestNoTable(t *testing.T) {}
func cases() []string { return nil }`
	tests := []struct {
		name string
		root func(t *testing.T) string
		want []string
	}{
		// Control: two cases pass, a one-case literal is flagged however it
		// is written, and a table the check cannot count is left alone.
		{
			name: "control: one-case tables are flagged",
			root: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				if err := writeFile(filepath.Join(dir, "x_test.go"), src); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			want: []string{"x_test.go:TestOneAssigned", "x_test.go:TestOneDeclared", "x_test.go:TestOneInline"},
		},
		{name: "module", root: func(*testing.T) string { return filepath.Join("..", "..") }, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, smallTables(t, tt.root(t))); diff != "" {
				t.Errorf("tables with fewer than %d cases (-want +got):\n%s", minCases, diff)
			}
		})
	}
}
