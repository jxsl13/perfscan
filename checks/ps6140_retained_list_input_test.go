package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
)

// Every input mutation retains the full authentic source certificate, including
// allocation, public uses, callbacks, storage, retention and lifecycle. The
// generic API receives actual source contexts, never synthetic ScenarioUses.
func TestPS6140AuthenticRetainedListInputs(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ao", "mo", "missing-required", "wrong-error-cell", "foreign-error-context", "foreign-factory", "missing-participation", "foreign-constructor", "foreign-publication", "foreign-owner", "missing-storage", "foreign-storage", "foreign-storage-native", "foreign-public-context", "foreign-public-context-copy", "foreign-public-owner", "budget"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, "before")
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			member := "ao"
			if name == "mo" {
				member = "mo"
			}
			contract := ps6140AuthenticSourceContract("before", member)
			assembled := ps6140UnusedProjectionSource(pass, pkg, &contract, 262144)
			if assembled == nil || assembled.retained == nil || assembled.retained.public == nil {
				t.Fatal("authentic complete source/lifetime prerequisites")
			}
			public := assembled.retained.public
			selection := assembled.selection.allocation
			input := ps6140RetainedListPublicInput(public)
			if input == nil || len(input.publicContexts) == 0 || public.storage == nil || !ps6140RetainedLifetimeSource(selection, assembled.retained, 262144) {
				t.Fatal("actual context/storage/lifetime premises")
			}
			positive := ps6140RetainedListReceiptSource(pass, pkg, selection, input, 262144)
			wrapper := ps6140RetainedListSource(pass, pkg, selection, public, 262144)
			if positive == nil || positive.public != nil || positive.input != input || positive.retentions[public.allocation.factory] == nil || wrapper == nil || wrapper.public != public || len(positive.retentions) != len(wrapper.retentions) || positive.release != wrapper.release || positive.elementClose != wrapper.elementClose {
				t.Fatal("generic source receipt differs from existing wrapper")
			}
			for factory, retention := range positive.retentions {
				if wrapper.retentions[factory] == nil || wrapper.retentions[factory].context != retention.context || wrapper.retentions[factory].append != retention.append || wrapper.retentions[factory].store != retention.store || wrapper.retentions[factory].load != retention.load {
					t.Fatal("wrapper changed an actual factory retention identity")
				}
			}
			// A generic list receipt cannot masquerade as complete lifetime.
			if ps6140RetainedLifetimeSource(selection, positive, 262144) {
				t.Fatal("list-only receipt silently promoted into lifetime safety")
			}
			factory := public.allocation.factory
			cell := public.allocation.errorCell
			budget := 262144
			want := name == "ao" || name == "mo"
			switch name {
			case "missing-required":
				input.requiredFactories = nil
			case "wrong-error-cell":
				input.requiredFactories[factory] = selection.owner
			case "foreign-error-context":
				foreign := *cell.context
				input.requiredFactories[factory] = ps6125SSAReference{context: &foreign, value: cell.value}
			case "foreign-factory":
				input.requiredFactories = map[*ps6125SSAContext]ps6125SSAReference{input.publication: cell}
			case "missing-participation":
				foreign := *factory
				foreign.calls, foreign.resolved, foreign.resolving, foreign.cells, foreign.structs = nil, nil, nil, nil, nil
				backend := ps6136FactoryResult(&foreign, foreign.reference(foreign.flow.function.Params[0]), selection.allocator, selection.slot)
				foreignCell := ps6136FactoryErrorCell(&foreign, backend)
				if backend == nil || foreignCell.context == nil || foreignCell.value == nil {
					t.Fatal("independently analyzed actual factory/error-cell prerequisites")
				}
				input.requiredFactories[&foreign] = foreignCell
				remaining := budget
				if !ps6140RetainedListInputs(pkg, selection, input, &remaining) {
					t.Fatal("disconnected factory control failed before actual participation gate")
				}
			case "foreign-constructor":
				foreign := *input.constructor
				input.constructor = &foreign
			case "foreign-publication":
				foreign := *input.publication
				input.publication = &foreign
			case "foreign-owner":
				foreign := *input.owner.context
				input.owner.context = &foreign
			case "missing-storage":
				input.storage = nil
			case "foreign-storage":
				foreign := *input.storage
				foreign.factory = input.publication
				input.storage = &foreign
			case "foreign-storage-native":
				foreign := *input.storage
				foreign.native = foreign.backend
				input.storage = &foreign
			case "foreign-public-context":
				input.publicContexts = append(input.publicContexts, ps6140RetainedListContext{input.publication, input.publicContexts[0].owner})
			case "foreign-public-context-copy":
				foreign := *input.publicContexts[0].context
				foreign.resolved = nil
				input.publicContexts[0].context = &foreign
			case "foreign-public-owner":
				input.publicContexts[0].owner = selection.owner
			case "budget":
				budget = 1
			}
			actual := ps6140RetainedListReceiptSource(pass, pkg, selection, input, budget)
			if name == "missing-participation" {
				stage := ""
				ps6140RetainedListReceiptSourceCheck(pass, pkg, selection, input, budget, func(rejected string) { stage = rejected })
				if stage != "required allocation participation" {
					t.Fatal("disconnected factory rejected at the wrong prerequisite", stage)
				}
			}
			if (actual != nil) != want {
				ps6140RetainedListReceiptSourceCheck(pass, pkg, selection, input, budget, func(stage string) { t.Log(stage) })
				t.Fatalf("generic receipt=%v want=%v", actual != nil, want)
			}
		})
	}
}
