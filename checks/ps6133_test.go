package checks

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

const ps6133Types = `package owner
import "fmt"
import "unsafe"
import "C"
type DeviceBuffer struct {n int;handle unsafe.Pointer}
type Recorder struct {handle unsafe.Pointer}
var rope2Spirv []byte
var _=unsafe.Pointer(nil)
`

var ps6133OwnerDigests = map[string]string{
	"ps6133_owner_metal.go.txt":      "e9e3e10317f002fc8311547393775083e2ae2740cbf3563b264f09160ca3aa33",
	"ps6133_owner_metal_native.txt":  "c99595a768fdb9cda070a4b710ddc0b09fef1073391ac7006657ab2a37c293b8",
	"ps6133_owner_vulkan.go.txt":     "e25e7907d66c4dd21f0bd6f10bd615e4c4d8ffb2f87511214c8dbae4ef8a982e",
	"ps6133_owner_vulkan_bridge.txt": "b48a0a3ea79e970c52490b8b1778768d4ee95cd60786ff857ccdce6bf2898f86",
	"ps6133_owner_vulkan_shader.txt": "043ace3ff2ac4ea23d843d12eca8d619ee500612cfa7908124c674ae7fc687b2",
}

func ps6133ReadOwner(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != ps6133OwnerDigests[name] {
		t.Fatalf("owner %s digest=%s", name, got)
	}
	return data
}

// This importer supplies C ABI types only; no native function executes. The
// original Go function, shader and bridge excerpts remain unchanged.
type ps6133Importer struct {
	fallback types.Importer
	c        *types.Package
}

func (i ps6133Importer) Import(path string) (*types.Package, error) {
	if path == "C" {
		return i.c, nil
	}
	return i.fallback.Import(path)
}

func ps6133C() *types.Package {
	p := types.NewPackage("C", "C")
	named := func(name string, basic types.BasicKind) *types.Named {
		name = strings.ToUpper(name[:1]) + name[1:]
		n := types.NewNamed(types.NewTypeName(token.NoPos, p, name, nil), types.Typ[basic], nil)
		p.Scope().Insert(n.Obj())
		return n
	}
	i, f, u := named("int", types.Int32), named("float", types.Float32), named("uint32_t", types.Uint32)
	for _, name := range []string{"mtl_recorder_rope2", "vk_recorder_rope2"} {
		params := []*types.Var{types.NewVar(token.NoPos, p, "rec", types.Typ[types.UnsafePointer])}
		if strings.HasPrefix(name, "vk_") {
			params = append(params, types.NewVar(token.NoPos, p, "spv", types.NewPointer(u)), types.NewVar(token.NoPos, p, "spvLen", i))
		}
		for _, name := range []string{"q", "inv"} {
			params = append(params, types.NewVar(token.NoPos, p, name, types.Typ[types.UnsafePointer]))
		}
		for j := range 9 {
			params = append(params, types.NewVar(token.NoPos, p, fmt.Sprintf("v%d", j), i))
		}
		params = append(params, types.NewVar(token.NoPos, p, "div", f))
		name = strings.ToUpper(name[:1]) + name[1:]
		p.Scope().Insert(types.NewFunc(token.NoPos, p, name, types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), types.NewTuple(types.NewVar(token.NoPos, p, "rc", i)), false)))
	}
	p.MarkComplete()
	return p
}

func ps6133OwnerContract(t *testing.T, backend string) (config.RowLocalStridedGuardContract, map[string][]byte) {
	t.Helper()
	c := config.RowLocalStridedGuardContract{WrapperMethod: "owner.Recorder.RoPEPair", NativeCallable: "C.mtl_recorder_rope2", ElementCountField: "n", HandleField: "handle", NativeArguments: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, RowLocalBandIndexingReviewed: true, HeadAndHalfWidthRelationReviewed: true, UnshiftedBackingHandleReviewed: true, NativeCodeGenerationAndBuildPathsReviewed: true, CheckedArithmeticAndShapeBehaviorReviewed: true, ErrorsFallbackAndSynchronizationReviewed: true, ElementCountAndFloat32StorageReviewed: true}
	files := map[string][]byte{}
	add := func(fixture, file, start, end string) {
		data := ps6133ReadOwner(t, fixture)
		files[file] = data
		s := string(data)
		a, b := strings.Index(s, start), strings.Index(s, end)
		if a < 0 || b <= a {
			t.Fatal("missing native markers")
		}
		c.Evidence = append(c.Evidence, config.NativeLayoutEvidence{File: file, Start: start, End: end, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(s[a:b+len(end)])))})
	}
	if backend == "metal" {
		add("ps6133_owner_metal_native.txt", "metal_bridge.m", "// rope2 (SPEC T613)", "static id<MTLComputePipelineState> gRoPE2 = nil;")
		add("ps6133_owner_metal_native.txt", "metal_bridge.m", "int mtl_recorder_rope2(", "int mtl_recorder_rope2_split(")
	} else {
		c.NativeCallable = "C.vk_recorder_rope2"
		c.ShaderBinding = "owner.rope2Spirv"
		c.NativeArguments = []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
		add("ps6133_owner_vulkan_bridge.txt", "vk_bridge.c", "int vk_recorder_rope2(", "// vk_recorder_mha_decode")
		add("ps6133_owner_vulkan_shader.txt", "shaders/rope2.comp", "#version 450", "q[base + uint(d.halfd)] = qih * c + qi * s;\n}")
	}
	return c, files
}

func ps6133Run(t *testing.T, source string, c []config.RowLocalStridedGuardContract, files map[string][]byte) int {
	return ps6133RunWithC(t, source, c, files, ps6133C())
}

func ps6133RunWithC(t *testing.T, source string, c []config.RowLocalStridedGuardContract, files map[string][]byte, cPackage *types.Package) int {
	t.Helper()
	fset := token.NewFileSet()
	filename := filepath.Join(t.TempDir(), "owner.go")
	file, err := parser.ParseFile(fset, filename, source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	cfg := types.Config{Importer: ps6133Importer{importer.Default(), cPackage}}
	// Ordinary go/types imports cannot expose cgo's lowercase pseudo-package
	// names. Adapt spelling for type scaffolding, then restore every source AST
	// selector before analysis; ABI types/objects and original bodies are kept.
	original := map[*ast.Ident]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		if s, ok := node.(*ast.SelectorExpr); ok {
			if root, ok := s.X.(*ast.Ident); ok && root.Name == "C" {
				original[s.Sel] = s.Sel.Name
				s.Sel.Name = strings.ToUpper(s.Sel.Name[:1]) + s.Sel.Name[1:]
			}
		}
		return true
	})
	pkg, err := cfg.Check("owner", fset, []*ast.File{file}, info)
	for id, name := range original {
		id.Name = name
	}
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, ReadFile: func(path string) ([]byte, error) {
		relative, err := filepath.Rel(filepath.Dir(filename), path)
		if err != nil {
			return nil, err
		}
		if data, ok := files[filepath.ToSlash(relative)]; ok {
			return data, nil
		}
		return nil, os.ErrNotExist
	}, Report: func(d analysis.Diagnostic) {
		n++
		if len(d.SuggestedFixes) != 0 || !strings.Contains(d.Message, "conditional") || !strings.Contains(d.Message, "band") {
			t.Error("unsafe or unconditional diagnostic")
		}
	}}
	if _, err := runPS6133WithContracts(pass, c); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPS6133NativeABIBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, backend string
		change        func(*types.Package) *types.Package
	}{
		{"wide C int", "metal", func(p *types.Package) *types.Package {
			p.Scope().Lookup("Int").Type().(*types.Named).SetUnderlying(types.Typ[types.Int64])
			return p
		}},
		{"unsigned C int", "vulkan", func(p *types.Package) *types.Package {
			p.Scope().Lookup("Int").Type().(*types.Named).SetUnderlying(types.Typ[types.Uint32])
			return p
		}},
		{"wide shader words", "vulkan", func(p *types.Package) *types.Package {
			p.Scope().Lookup("Uint32_t").Type().(*types.Named).SetUnderlying(types.Typ[types.Uint64])
			return p
		}},
		{"opaque receiver formal", "metal", func(p *types.Package) *types.Package {
			q := types.NewPackage("C", "C")
			for _, name := range p.Scope().Names() {
				object := p.Scope().Lookup(name)
				if name == "Mtl_recorder_rope2" {
					sig := object.Type().(*types.Signature)
					vars := make([]*types.Var, sig.Params().Len())
					for j := range vars {
						v := sig.Params().At(j)
						typ := v.Type()
						if j == 0 {
							typ = types.NewInterfaceType(nil, nil).Complete()
						}
						vars[j] = types.NewVar(token.NoPos, q, v.Name(), typ)
					}
					object = types.NewFunc(token.NoPos, q, name, types.NewSignatureType(nil, nil, nil, types.NewTuple(vars...), sig.Results(), false))
				}
				q.Scope().Insert(object)
			}
			q.MarkComplete()
			return q
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, files := ps6133OwnerContract(t, tc.backend)
			source := ps6133Types + string(ps6133ReadOwner(t, "ps6133_owner_"+tc.backend+".go.txt"))
			if n := ps6133RunWithC(t, source, []config.RowLocalStridedGuardContract{c}, files, tc.change(ps6133C())); n != 0 {
				t.Fatalf("ABI change emitted %d", n)
			}
		})
	}
	t.Run("shader size only wide C type", func(t *testing.T) {
		t.Parallel()
		c, files := ps6133OwnerContract(t, "vulkan")
		p, q := ps6133C(), types.NewPackage("C", "C")
		long := types.NewNamed(types.NewTypeName(token.NoPos, q, "Long", nil), types.Typ[types.Int64], nil)
		q.Scope().Insert(long.Obj())
		for _, name := range p.Scope().Names() {
			object := p.Scope().Lookup(name)
			if name == "Vk_recorder_rope2" {
				sig := object.Type().(*types.Signature)
				vars := make([]*types.Var, sig.Params().Len())
				for j := range vars {
					v := sig.Params().At(j)
					typ := v.Type()
					if j == 2 {
						typ = long
					}
					vars[j] = types.NewVar(token.NoPos, q, v.Name(), typ)
				}
				object = types.NewFunc(token.NoPos, q, name, types.NewSignatureType(nil, nil, nil, types.NewTuple(vars...), sig.Results(), false))
			}
			q.Scope().Insert(object)
		}
		q.MarkComplete()
		source := strings.Replace(ps6133Types+string(ps6133ReadOwner(t, "ps6133_owner_vulkan.go.txt")), "C.int(len(rope2Spirv))", "C.long(len(rope2Spirv))", 1)
		if n := ps6133RunWithC(t, source, []config.RowLocalStridedGuardContract{c}, files, q); n != 0 {
			t.Fatalf("wide shader size emitted %d", n)
		}
	})
}

func TestPS6133PinnedOwners(t *testing.T) {
	t.Parallel()
	for _, backend := range []string{"metal", "vulkan"} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			data := ps6133ReadOwner(t, "ps6133_owner_"+backend+".go.txt")
			c, files := ps6133OwnerContract(t, backend)
			if n := ps6133Run(t, ps6133Types+string(data), []config.RowLocalStridedGuardContract{c}, files); n != 1 {
				t.Fatalf("findings=%d want1", n)
			}
		})
	}
}

func TestPS6133AdversarialOwnerFlow(t *testing.T) {
	t.Parallel()
	for _, backend := range []string{"metal", "vulkan"} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			owner := string(ps6133ReadOwner(t, "ps6133_owner_"+backend+".go.txt"))
			for _, tc := range []struct {
				name   string
				source func(string) string
				change func(*config.RowLocalStridedGuardContract, map[string][]byte)
				want   int
			}{
				{"commuted canonical extent", func(s string) string { return strings.Replace(s, "maxOff+seq*stride", "stride*seq+maxOff", 1) }, nil, 1},
				{"trimmed exact-row guard", func(s string) string { return strings.Replace(s, "maxOff+seq*stride", "seq*stride", 1) }, nil, 0},
				{"already independently checked bands", func(s string) string {
					return strings.Replace(s, "qkv.n < maxOff+seq*stride", "qkv.n < seq*stride || offQ+headsQ*hd > stride || offK+headsK*hd > stride", 1)
				}, nil, 0},
				{"legitimate flat subview extent", func(s string) string { return strings.Replace(s, "maxOff+seq*stride", "offQ+seq*hd", 1) }, nil, 0},
				{"altered row bound", func(s string) string { return strings.Replace(s, "maxOff+seq*stride", "maxOff+(seq+1)*stride", 1) }, nil, 0},
				{"mixed geometry rebind", func(s string) string { return strings.Replace(s, "maxOff :=", "seq,z:=1,0;_=z;maxOff :=", 1) }, nil, 0},
				{"range geometry assignment", func(s string) string { return strings.Replace(s, "maxOff :=", "for stride=range 3{};maxOff :=", 1) }, nil, 0},
				{"receiver alias", func(s string) string { return strings.Replace(s, "maxOff :=", "alias:=r;_=alias;maxOff :=", 1) }, nil, 0},
				{"buffer alias", func(s string) string { return strings.Replace(s, "maxOff :=", "alias:=qkv;_=alias;maxOff :=", 1) }, nil, 0},
				{"whole receiver reset", func(s string) string { return strings.Replace(s, "maxOff :=", "*r=Recorder{};maxOff :=", 1) }, nil, 0},
				{"shifted backing handle", func(s string) string {
					return strings.Replace(s, "qkv.handle, inv.handle", "unsafe.Add(qkv.handle,4), inv.handle", 1)
				}, nil, 0},
				{"native alternate input", func(s string) string {
					return strings.Replace(s, "qkv.handle, inv.handle", "inv.handle, inv.handle", 1)
				}, nil, 0},
				{"native changed geometry", func(s string) string { return strings.Replace(s, "C.int(seq)", "C.int(seq+1)", 1) }, nil, 0},
				{"opaque error operand", func(s string) string {
					return strings.Replace(s, "qkv.n, seq*stride, maxOff", "change(qkv), seq*stride, maxOff", 1) + "func change(b *DeviceBuffer)int{b.n=1;return 0}"
				}, nil, 0},
				{"shadowed max", func(s string) string { return s + "func max(a,b int)int{return a+b}" }, nil, 0},
				{"wrong native role", nil, func(c *config.RowLocalStridedGuardContract, _ map[string][]byte) {
					c.NativeArguments[5], c.NativeArguments[7] = c.NativeArguments[7], c.NativeArguments[5]
				}, 0},
				{"unreviewed flat-frame layout", nil, func(c *config.RowLocalStridedGuardContract, _ map[string][]byte) {
					c.RowLocalBandIndexingReviewed = false
				}, 0},
				{"unreviewed subview handle", nil, func(c *config.RowLocalStridedGuardContract, _ map[string][]byte) {
					c.UnshiftedBackingHandleReviewed = false
				}, 0},
				{"unreviewed C conversion range", nil, func(c *config.RowLocalStridedGuardContract, _ map[string][]byte) {
					c.CheckedArithmeticAndShapeBehaviorReviewed = false
				}, 0},
				{"wrong configured native callee", nil, func(c *config.RowLocalStridedGuardContract, _ map[string][]byte) { c.NativeCallable = "C.other_native" }, 0},
				{"unreviewed count units and dtype", nil, func(c *config.RowLocalStridedGuardContract, _ map[string][]byte) {
					c.ElementCountAndFloat32StorageReviewed = false
				}, 0},
				{"changed full-frame offset indexing", nil, func(_ *config.RowLocalStridedGuardContract, files map[string][]byte) {
					for name, data := range files {
						s := string(data)
						s = strings.ReplaceAll(s, "offQ + p*stride", "offQ + seq*stride + p*stride")
						s = strings.ReplaceAll(s, "uint(d.offQ) + p * uint(d.stride)", "uint(d.offQ) + uint(d.seq) * uint(d.stride) + p * uint(d.stride)")
						files[name] = []byte(s)
					}
				}, 0},
				{"changed bridge ABI", nil, func(_ *config.RowLocalStridedGuardContract, files map[string][]byte) {
					for name, data := range files {
						files[name] = []byte(strings.Replace(string(data), "int seq, int stride", "int stride, int seq", 1))
					}
				}, 0},
				{"missing native artifact", nil, func(c *config.RowLocalStridedGuardContract, files map[string][]byte) {
					delete(files, c.Evidence[0].File)
				}, 0},
				{"duplicate native marker", nil, func(c *config.RowLocalStridedGuardContract, files map[string][]byte) {
					e := c.Evidence[0]
					files[e.File] = append(files[e.File], []byte(e.Start)...)
				}, 0},
				{"Vulkan alternate shader", func(s string) string {
					if backend == "vulkan" {
						return strings.ReplaceAll(s, "rope2Spirv", "otherShader") + "var otherShader []byte"
					}
					return s
				}, nil, map[bool]int{true: 0, false: 1}[backend == "vulkan"]},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					c, files := ps6133OwnerContract(t, backend)
					s := owner
					if tc.source != nil {
						s = tc.source(s)
					}
					if tc.change != nil {
						tc.change(&c, files)
					}
					if n := ps6133Run(t, ps6133Types+s, []config.RowLocalStridedGuardContract{c}, files); n != tc.want {
						t.Fatalf("findings=%d want%d", n, tc.want)
					}
				})
			}
		})
	}
}

func TestPS6133ContractCloneAndAmbiguity(t *testing.T) {
	t.Parallel()
	c, files := ps6133OwnerContract(t, "metal")
	cfg := config.Config{RowLocalStridedGuardContracts: []config.RowLocalStridedGuardContract{c}}
	compiled := cfg.Compile()
	cfg.RowLocalStridedGuardContracts[0].NativeArguments[0] = 9
	cfg.RowLocalStridedGuardContracts[0].Evidence[0].File = "changed"
	if compiled.RowLocalStridedGuardContracts[0].NativeArguments[0] != 1 || compiled.RowLocalStridedGuardContracts[0].Evidence[0].File != "metal_bridge.m" {
		t.Fatal("compiled contract aliases input")
	}
	c, _ = ps6133OwnerContract(t, "metal")
	if config.UsableRowLocalStridedGuardContractCount([]config.RowLocalStridedGuardContract{c}) != 1 || config.UsableRowLocalStridedGuardContractCount([]config.RowLocalStridedGuardContract{c, c}) != 0 {
		t.Fatal("usable contract count")
	}
	s := ps6133Types + string(ps6133ReadOwner(t, "ps6133_owner_metal.go.txt"))
	if n := ps6133Run(t, s, []config.RowLocalStridedGuardContract{c, c}, files); n != 0 {
		t.Fatal("ambiguous contract emitted")
	}
	bad := c
	bad.UnshiftedBackingHandleReviewed = false
	if n := ps6133Run(t, s, []config.RowLocalStridedGuardContract{c, bad}, files); n != 0 {
		t.Fatal("contradictory contract emitted")
	}
	if PS6133.AutoFix || !PS6133.NeedsConfig || PS6133.Level != lint.LevelAggressive {
		t.Fatal("unsafe metadata")
	}
}

func TestPS6133GeneratedCgoFixture(t *testing.T) {
	t.Parallel()
	if os.Getenv("CGO_ENABLED") == "0" {
		t.Skip("genuine cgo compilation fixture requires cgo")
	}
	c, _ := ps6133OwnerContract(t, "metal")
	c.WrapperMethod = "ps6133.Recorder.RoPEPair"
	for j := range c.Evidence {
		c.Evidence[j].File = "metal_layout.h"
	}
	fixed := c
	fixed.WrapperMethod = "ps6133.Recorder.RoPEPairFixed"
	vulkan, _ := ps6133OwnerContract(t, "vulkan")
	vulkan.WrapperMethod = "ps6133.Recorder.RoPEPairVulkan"
	vulkan.ShaderBinding = "ps6133.rope2Spirv"
	for j := range vulkan.Evidence {
		vulkan.Evidence[j].File = "vulkan_layout.h"
	}
	a := *PS6133.Analyzer
	a.Run = func(pass *analysis.Pass) (any, error) {
		result, err := runPS6133WithContracts(pass, []config.RowLocalStridedGuardContract{c, fixed, vulkan})
		// Preserve the positive fixture assertions, then independently challenge
		// the real compiler wrapper with source-visible extra dispatch/rebind/check.
		m := ps6107Methods(pass)[c.WrapperMethod]
		outer := m.declaration.Body.List[2].(*ast.AssignStmt).Rhs[0].(*ast.CallExpr)
		literal, wrapped := ps2110Unparen(outer.Fun).(*ast.FuncLit)
		if !wrapped {
			t.Fatal("fixture did not exercise genuine cgo pointer wrapper")
		}
		original := literal.Body.List
		native := ps6135Return(original[len(original)-1])
		report := pass.Report
		defer func() { literal.Body.List = original; pass.Report = report }()
		for _, name := range []string{"extra native dispatch", "alias rebind", "duplicate pointer check"} {
			statements := append([]ast.Stmt(nil), original[:len(original)-1]...)
			switch name {
			case "extra native dispatch":
				statements = append(statements, &ast.ExprStmt{X: native})
			case "alias rebind":
				alias := original[0].(*ast.AssignStmt)
				copy := *alias
				copy.Tok = token.ASSIGN
				statements = append(statements, &copy)
			case "duplicate pointer check":
				for _, statement := range original {
					if check, ok := statement.(*ast.ExprStmt); ok {
						statements = append(statements, check)
						break
					}
				}
			}
			literal.Body.List = append(statements, original[len(original)-1])
			n := 0
			pass.Report = func(analysis.Diagnostic) { n++ }
			if _, err := runPS6133WithContracts(pass, []config.RowLocalStridedGuardContract{c}); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("generated wrapper %s emitted %d", name, n)
			}
		}
		return result, err
	}
	analysistest.Run(t, analysistest.TestData(), &a, "ps6133")
}
