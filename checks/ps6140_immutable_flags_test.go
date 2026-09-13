package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6140ImmutableConstructorFlags(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body, extra string
		want              bool
	}{
		{"fresh-zero", "", "", true},
		{"true-initializer", "d.moe=true", "", true},
		{"read-only-call-chain", "", "func read(d *owner)bool{return d.moe};func(d *owner)Read()bool{return read(d)}", true},
		{"other-constructor", "", "func another()*owner{d:=common();d.moe=true;return d}", true},
		{"exported-setter", "", "func(d *owner)Set(){d.moe=true}", false},
		{"source-helper-setter", "", "func set(d *owner){d.moe=true};func(d *owner)Set(){set(d)}", false},
		{"constructor-helper-is-not-private-lifetime", "set(d)", "func set(d *owner){d.moe=true};func(d *owner)Set(){set(d)}", false},
		{"flag-address", "", "func expose(d *owner){opaqueBool(&d.moe)};func opaqueBool(*bool)", false},
		{"whole-owner-replacement", "", "func reset(d *owner){*d=owner{}}", false},
		{"global-retention", "", "var saved *owner;func save(d *owner){saved=d}", false},
		{"boxed-owner", "", "func expose(d *owner){opaque(d)};func opaque(any)", false},
		{"wrapped-owner", "", "type holder struct{d *owner};func expose(d *owner){opaque(holder{d})};func opaque(holder)", false},
		{"captured-mutation", "", "var saved func();func capture(d *owner){saved=func(){d.moe=true}}", false},
		{"linkage", "", "\n//go:linkname common elsewhere.common\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pass, pkg := ps6136CellTestPackage(t, `package cells
type owner struct{moe bool}
func common()*owner{return &owner{}}
func build()*owner{d:=common();`+test.body+`;return d}
`+test.extra)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			context := ps6125NewSSAContext(pkg.Func("build"), nil, nil, 16384)
			root := ps6140TestConstructorOwner(t, context, owner)
			field := ps6136FieldVar(owner, "moe")
			snapshot := ps6140ConstructorFlags(context, root, owner, map[*types.Var]bool{field: true}, 65536)
			if snapshot == nil {
				t.Fatal("selected constructor snapshot prerequisite missing")
			}
			proof := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 65536)
			if (proof != nil) != test.want {
				stage := ""
				ps6140ImmutableConstructorFlagsCheck(pass, pkg, snapshot, owner, 65536, func(reason string) { stage = reason })
				t.Fatalf("immutable=%v want=%v stage=%s", proof != nil, test.want, stage)
			}
			if proof != nil && proof.snapshot != snapshot {
				t.Fatal("selected snapshot association lost")
			}
			if ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 0) != nil {
				t.Fatal("exhausted budget accepted")
			}
		})
	}
}

func TestPS6140AuthenticImmutableFlags(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"newDecoder", "", true},
		{"newOLMo2Decoder", "", true},
		{"newGemma2Decoder", "", true},
		{"newMixtralDecoder", "", true},
		{"newDecoder", "func(d *Decoder)SetFlag(){d.moe=true}", false},
		{"newDecoder", "func setFlag(d *Decoder){d.moe=true};func(d *Decoder)SetFlag(){setFlag(d)}", false},
		{"newDecoder", "func(d *Decoder)ExposeFlag(){opaqueFlag(&d.moe)};func opaqueFlag(*bool)", false},
		{"newDecoder", "func(d *Decoder)Replace(){*d=Decoder{}}", false},
		{"newDecoder", "func(d *Decoder)Expose(){opaqueOwner(d)};func opaqueOwner(any)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetalRewrite(t, "before", func(fs *token.FileSet, files []*ast.File) []*ast.File {
				if test.extra == "" {
					return files
				}
				injected, err := parser.ParseFile(fs, "injected.go", "package llamagpu;"+test.extra, parser.ParseComments|parser.SkipObjectResolution)
				if err != nil {
					t.Fatal(err)
				}
				return append(files, injected)
			})
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			context := ps6125NewSSAContext(pkg.Func(test.name), nil, nil, 16384)
			root := ps6140TestConstructorOwner(t, context, owner)
			fields := map[*types.Var]bool{ps6136FieldVar(owner, "moe"): true, ps6136FieldVar(owner, "postNorm"): true, ps6136FieldVar(owner, "sandwich"): true}
			snapshot := ps6140ConstructorFlags(context, root, owner, fields, 65536)
			if snapshot == nil {
				t.Fatal("authentic selected snapshot prerequisite missing")
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			proof := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 65536)
			if (proof != nil) != test.want {
				stage := ""
				ps6140ImmutableConstructorFlagsCheck(pass, pkg, snapshot, owner, 65536, func(reason string) { stage = reason })
				t.Fatalf("immutable=%v want=%v stage=%s", proof != nil, test.want, stage)
			}
		})
	}
}
