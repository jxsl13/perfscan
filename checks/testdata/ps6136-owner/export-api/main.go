// Command export-api prints type-only model scaffolding from the pinned owner
// checkout. It is not backend execution or detector coverage evidence.
package main

import (
	"fmt"
	"go/types"
	"os"
	"sort"

	"golang.org/x/tools/go/packages"
)

func main() {
	pkgs, err := packages.Load(&packages.Config{Dir: os.Args[1], Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}, "github.com/jxsl13/goai/nlp")
	if err != nil || packages.PrintErrors(pkgs) != 0 || len(pkgs) != 1 {
		panic("cannot load pinned model metadata")
	}
	pkg := pkgs[0].Types
	var selected *types.Package
	selected = pkg
	if len(os.Args) > 2 && os.Args[2] == "nn" {
		for _, imported := range pkg.Imports() {
			if imported.Path() == "github.com/jxsl13/goai/nn" {
				selected = imported
			}
		}
	}
	seen := map[string]*types.Named{}
	var visit func(types.Type)
	visit = func(typ types.Type) {
		switch value := typ.(type) {
		case *types.Named:
			if value.Obj().Pkg() != selected || seen[value.Obj().Name()] != nil {
				return
			}
			seen[value.Obj().Name()] = value
			visit(value.Underlying())
		case *types.Struct:
			for index := 0; index < value.NumFields(); index++ {
				visit(value.Field(index).Type())
			}
		case *types.Pointer:
			visit(value.Elem())
		case *types.Slice:
			visit(value.Elem())
		case *types.Array:
			visit(value.Elem())
		case *types.Map:
			visit(value.Key())
			visit(value.Elem())
		}
	}
	namesToVisit := []string{"GPT", "Llama", "StableLM", "Cohere", "Nemotron", "Gemma", "DeepSeekV2", "Mamba", "Jamba", "Mamba2", "RWKV", "Falcon", "OLMo2", "Gemma2", "MPT", "Mixtral", "Qwen2MoE", "GraniteMoE", "OLMoE", "StarCoder2", "Phi", "GPTNeoX", "QuantLlama"}
	if selected != pkg {
		namesToVisit = []string{"RMSNorm", "LayerNorm", "SwiGLU", "GLU", "DeepSeekMoE", "LoRALinear", "MambaBlock", "SparseMoE", "QuantLinear", "QuantSwiGLU", "RWKVBlock"}
	}
	for _, name := range namesToVisit {
		visit(selected.Scope().Lookup(name).Type())
	}
	var names []string
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("type %s %s\n", name, types.TypeString(seen[name].Underlying(), func(other *types.Package) string {
			if other == selected {
				return ""
			}
			return other.Name()
		}))
	}
}
