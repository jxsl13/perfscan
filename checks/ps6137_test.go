package checks

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6137PinnedFixtureIntegrity(t *testing.T) {
	t.Parallel()
	fixtures := map[string]string{
		"profile_before.go": "9f564cdd882c28dc11d497e7fe1c58a5f608903fbae8f335155092c294eaa4fb",
		"profile_after.go":  "91a20c9c43d8ac15164721b9831cfc8cd73456b5fe83173d7854e9f30f9fb68d",
		"header_before.h":   "73f4f3279d32e77fafc722c3230eecdb762d487ccfa48e546de708fc4391c6a0",
		"header_after.h":    "b7fe8e71c4b9c59bc66a9721bce4da181178c1ec1703dd1cdf8dfae653a1d992",
		"bridge_before.m":   "0014b85b5a44ba42f5afe426e1401754961f3ebb020a76ec5d266c953a688343",
		"bridge_after.m":    "6d8d61b53ab3e663a859ddd55b205cd6bec63d595da6784fb4c02f7efef4ab6d",
	}
	for name, want := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("testdata/ps6137_owner_" + name + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsRune(data, '\r') {
				t.Fatal("pinned fixture must retain LF line endings")
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
				t.Fatalf("pinned fixture digest=%s, want %s", got, want)
			}
		})
	}
}

// Native declarations are a type/ABI scaffold only. They do not record a GPU
// workload or certify actual recorder storage/lifetime. Owner bodies below are
// frozen verbatim and pass through the genuine installed cgo compiler.
func ps6137CgoTypes(t *testing.T, after bool) string {
	t.Helper()
	name := "before"
	if after {
		name = "after"
	}
	data, err := os.ReadFile("testdata/ps6137_owner_header_" + name + ".h.txt")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[bool]string{false: "73f4f3279d32e77fafc722c3230eecdb762d487ccfa48e546de708fc4391c6a0", true: "b7fe8e71c4b9c59bc66a9721bce4da181178c1ec1703dd1cdf8dfae653a1d992"}[after]
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != expected {
		t.Fatalf("native header digest=%s", got)
	}
	native := string(data) + `
int mtl_recorder_profile_snapshot(void *r,mtl_recorder_profile_event_snapshot **e,int *n,int *a,int *b,int *c,unsigned long long *f,unsigned long long *d) { return -8; }
int mtl_recorder_profile_label_tokens(void *r,uintptr_t **t) { return -8; }
void mtl_recorder_free(void *r) {}
static int direct_snapshot(void *r,mtl_recorder_profile_event_snapshot **e,int *n) { return -8; }
`
	if after {
		native += `mtl_recorder_profile_snapshot_view mtl_recorder_profile_view(void *r) { mtl_recorder_profile_snapshot_view v={0};v.status=-8;return v; }`
	}
	return "package owner\n/*\n#include <stdint.h>\n" + native + `
*/
import "C"
import("fmt";"strings";"time";"unsafe")
var _=strings.Clone
type ResidentQGroup struct{}
`
}

func ps6137Contract() config.NativeSnapshotReuseContract {
	return config.NativeSnapshotReuseContract{
		CandidateMethod: "owner.Recorder.Profile", AcquireCallable: "C.mtl_recorder_profile_snapshot", TokensCallable: "C.mtl_recorder_profile_label_tokens", LifecycleMethod: "owner.Recorder.Free",
		ReceiverHandleField: "handle", ResultSliceField: "Events", ResultStringField: "Label", NativeStringField: "label", FillCallable: "owner.fillRecorderProfileEvents", OwnMethod: "owner.recorderProfileLabels.own", IntoMethod: "owner.Recorder.ProfileInto",
		HandleArgument: 0, PointerOutArgument: 1, CountOutArgument: 2, ScalarOutArguments: []int{3, 4, 5, 6, 7}, ScalarResultFields: []string{"OmittedMPS", "OmittedOverflow", "OmittedUnsupported", "TimestampFrequency", "CommandDuration"},
		EventNumericFields: []config.NativeSnapshotNumericField{{NativeField: "startOffsetNS", ResultField: "StartOffset"}, {NativeField: "ticks", ResultField: "Ticks"}, {NativeField: "durationNS", ResultField: "Duration"}}, EventSpanField: "EventSpan", EventStartField: "StartOffset", EventDurationField: "Duration",
		NativeStorageLifetimeReviewed: true, NativeRecordShapeAndTerminationReviewed: true, ReadonlySynchronousExtractionReviewed: true, TokenIdentityAndContentReviewed: true, RepeatedExtractionPolicyReviewed: true, OwnedOutputAndErrorSemanticsReviewed: true,
	}
}

func ps6137Owner(t *testing.T, after bool) string {
	t.Helper()
	name := "before"
	if after {
		name = "after"
	}
	data, err := os.ReadFile("testdata/ps6137_owner_profile_" + name + ".go.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := map[bool]string{false: "9f564cdd882c28dc11d497e7fe1c58a5f608903fbae8f335155092c294eaa4fb", true: "91a20c9c43d8ac15164721b9831cfc8cd73456b5fe83173d7854e9f30f9fb68d"}[after]
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
		t.Fatalf("owner digest=%s", got)
	}
	return string(data)
}

func ps6137RunFixture(t *testing.T, source string, c config.NativeSnapshotReuseContract, want int, existing bool, mutations ...func(*analysis.Pass)) {
	t.Helper()
	dir, cleanup, err := analysistest.WriteFiles(map[string]string{"owner/owner.go": source})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	analyzer := *PS6137.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		for _, mutate := range mutations {
			mutate(pass)
		}
		n := 0
		pass.Report = func(d analysis.Diagnostic) {
			n++
			if len(d.SuggestedFixes) != 0 {
				t.Error("unsafe automatic rewrite")
			}
			if strings.Contains(d.Message, "already exists") != existing {
				t.Error("incorrect sibling advice")
			}
		}
		result, err := runPS6137WithContracts(pass, []config.NativeSnapshotReuseContract{c})
		if n != want {
			for _, stmt := range ps6137Functions(pass)[c.CandidateMethod].decl.Body.List {
				if a, ok := stmt.(*ast.AssignStmt); ok {
					var b bytes.Buffer
					_ = format.Node(&b, pass.Fset, a)
					t.Log(b.String())
				}
			}
			functions := ps6137Functions(pass)
			f := functions[c.CandidateMethod]
			own := functions[c.OwnMethod]
			r, s, valid := ps6137Result(f, &c)
			a, acq := ps6137Acquire(pass, f, &c)
			t.Errorf("findings=%d want%d; config=%v result=%v own=%v fill=%v acquire=%v prelude=%v guards=%v material=%v", n, want, c.Valid(), valid, ps6137Own(pass, own), ps6137Fill(pass, functions[c.FillCallable], own, &c, r, s), acq, ps6137Prelude(pass, f, a), ps6137AcquisitionGuards(pass, f, a), ps6137Materialize(pass, f, a, &c, r, s, functions[c.FillCallable]))
		}
		return result, err
	}
	analysistest.Run(t, dir, &analyzer, "owner")
}

func TestPS6137OnDiskCgoFixture(t *testing.T) {
	t.Parallel()
	if !build.Default.CgoEnabled {
		t.Skip("genuine cgo fixture requires cgo")
	}
	c := ps6137Contract()
	c.CandidateMethod = "ps6137.Recorder.Profile"
	c.LifecycleMethod = "ps6137.Recorder.Free"
	c.FillCallable = "ps6137.fillRecorderProfileEvents"
	c.OwnMethod = "ps6137.recorderProfileLabels.own"
	c.IntoMethod = "ps6137.Recorder.ProfileInto"
	analyzer := *PS6137.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6137WithContracts(pass, []config.NativeSnapshotReuseContract{c})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6137")
}

func TestPS6137PinnedCgoOwners(t *testing.T) {
	t.Parallel()
	if !build.Default.CgoEnabled {
		t.Skip("genuine cgo owner replay requires cgo")
	}
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent", true: "merge"}[after], func(t *testing.T) {
			t.Parallel()
			ps6137RunFixture(t, ps6137CgoTypes(t, after)+ps6137Owner(t, after), ps6137Contract(), 1, after)
		})
	}
}

func TestPS6137AdversarialOwnerFlow(t *testing.T) {
	t.Parallel()
	if !build.Default.CgoEnabled {
		t.Skip("genuine cgo owner replay requires cgo")
	}
	owner := ps6137CgoTypes(t, false) + ps6137Owner(t, false)
	for _, tc := range []struct {
		name, old, replacement string
		change                 func(*config.NativeSnapshotReuseContract)
	}{
		{"status polarity", "if rc != 0 {", "if rc == 0 {", nil},
		{"unsigned count guard", "var count, omittedMPS, omittedOverflow, omittedUnsupported C.int", "var count C.uint;var omittedMPS, omittedOverflow, omittedUnsupported C.int", nil},
		{"missing negative validation", "count < 0 || count > 0 && events == nil", "count > 0 && events == nil", nil},
		{"missing pointer validation", "count < 0 || count > 0 && events == nil", "count < 0", nil},
		{"nonpositive pointer guard", "count > 0 && events == nil", "count >= 0 && events == nil", nil},
		{"changed scalar source", "TimestampFrequency: uint64(frequency)", "TimestampFrequency: uint64(commandDurationNS)", nil},
		{"duplicate scalar outargs", "&frequency, &commandDurationNS", "&commandDurationNS, &commandDurationNS", nil},
		{"count decrement", "p := RecorderProfile{", "count--;p := RecorderProfile{", nil},
		{"range count write", "p := RecorderProfile{", "for count=range C.int(1){};p := RecorderProfile{", nil},
		{"count address escape", "p := RecorderProfile{", "q:=&count;*q=1;p := RecorderProfile{", nil},
		{"result rebind", "return p, nil", "p.Events=nil;return p, nil", nil},
		{"wrong allocation length", "make([]RecorderProfileEvent, int(count))", "make([]RecorderProfileEvent, int(count)+1)", nil},
		{"wrong native extent", "unsafe.Slice(events, int(count))", "unsafe.Slice(events, int(count)+1)", nil},
		{"wrong token extent", "unsafe.Slice(tokenStorage, len(p.Events))", "unsafe.Slice(tokenStorage, len(p.Events)+1)", nil},
		{"missing token pointer check", "rc != 0 || tokenStorage == nil", "rc != 0", nil},
		{"wrong token provider", "C.mtl_recorder_profile_label_tokens(r.handle,", "C.mtl_recorder_profile_label_tokens(nil,", nil},
		{"fill alias", "fillRecorderProfileEvents(&p, nativeEvents", "q:=&p;fillRecorderProfileEvents(q, nativeEvents", nil},
		{"opaque fill dispatch", "fillRecorderProfileEvents(&p, nativeEvents", "f:=fillRecorderProfileEvents;f(&p, nativeEvents", nil},
		{"native index mismatch", "event := &nativeEvents[i]", "event := &nativeEvents[0]", nil},
		{"result index mismatch", "p.Events[i] = RecorderProfileEvent{", "p.Events[0] = RecorderProfileEvent{", nil},
		{"token index mismatch", "labels.own(uintptr(tokens[i])", "labels.own(uintptr(tokens[0])", nil},
		{"event field mismatch", "Ticks:       uint64(event.ticks)", "Ticks:       uint64(event.durationNS)", nil},
		{"cached borrowed label", "owned := strings.Clone(view)", "owned := view", nil},
		{"native cache key retained", "labels.byText[owned] = owned", "labels.byText[view] = owned", nil},
		{"borrowed return", "labels.byText[owned] = owned\n\treturn owned", "labels.byText[owned] = owned\n\treturn view", nil},
		{"token equality removed", "token == labels.inline[i].token", "token == token", nil},
		{"content equality removed", "view == labels.inline[i].owned", "view == view", nil},
		{"wrong token cache key", "labels.rest[token]", "labels.rest[0]", nil},
		{"native slice overread", "int(C.MTL_RECORDER_PROFILE_LABEL_CAPACITY)", "int(C.MTL_RECORDER_PROFILE_LABEL_CAPACITY)+1", nil},
		{"scan loses bound", "n < len(bytes) && bytes[n] != 0", "bytes[n] != 0", nil},
		{"scan mutates via alias", "n := 0", "n := 0;q:=&n;(*q)++", nil},
		{"native pointer offset", "unsafe.Pointer(native)", "unsafe.Pointer(uintptr(unsafe.Pointer(native))+1)", nil},
		{"helper global escape", "view := unsafe.String", "globalNative=native;view := unsafe.String", nil},
		{"helper early owned wrong content", "func (labels *recorderProfileLabels) own(token uintptr, native *C.char) string {", "func (labels *recorderProfileLabels) own(token uintptr, native *C.char) string {if true{return \"wrong\"};", nil},
		{"helper receiver alias", "view := unsafe.String", "alias:=labels;alias.count=0;view := unsafe.String", nil},
		{"helper unknown callback", "view := unsafe.String", "callback();view := unsafe.String", nil},
		{"helper zero cache replaced", "var labels recorderProfileLabels", "labels:=globalLabels", nil},
		{"helper geometry mutation", "for i := range p.Events {", "p.Events=nil;for i := range p.Events {", nil},
		{"span mismatch", "p.Events[i].StartOffset + p.Events[i].Duration", "p.Events[i].StartOffset + p.Events[i].StartOffset", nil},
		{"unreviewed lifetime", "", "", func(c *config.NativeSnapshotReuseContract) { c.NativeStorageLifetimeReviewed = false }},
		{"unreviewed tokens", "", "", func(c *config.NativeSnapshotReuseContract) { c.TokenIdentityAndContentReviewed = false }},
		{"invalid scalar role", "", "", func(c *config.NativeSnapshotReuseContract) { c.ScalarOutArguments[0] = 8 }},
		{"wrong result field", "", "", func(c *config.NativeSnapshotReuseContract) { c.ResultSliceField = "Other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := owner
			c := ps6137Contract()
			if tc.old != "" {
				if !strings.Contains(source, tc.old) {
					t.Fatal("mutation witness absent")
				}
				source = strings.Replace(source, tc.old, tc.replacement, 1)
			}
			if tc.name == "unsigned count guard" {
				source = strings.Replace(source, "**e,int *n,", "**e,unsigned int *n,", 1)
				source = strings.Replace(source, "int* eventCount", "unsigned int* eventCount", 1)
			}
			source += `var globalNative *C.char;var globalLabels recorderProfileLabels;func callback(){}`
			if tc.change != nil {
				tc.change(&c)
			}
			ps6137RunFixture(t, source, c, 0, false)
		})
	}
}

func ps6137DirectSliceOwner(t *testing.T) string {
	t.Helper()
	owner := ps6137Owner(t, false)
	free := owner[strings.Index(owner, "func (r *Recorder) Free()"):strings.Index(owner, "func (r *Recorder) Profile()")]
	owner = owner[:strings.Index(owner, "func fillRecorderProfileEvents(")]
	return ps6137CgoTypes(t, false) + owner + free + `
func fillRecorderProfileEvents(p []RecorderProfileEvent,nativeEvents []C.mtl_recorder_profile_event_snapshot,tokens []C.uintptr_t){
 var labels recorderProfileLabels
 for i:=range p{
  event:=&nativeEvents[i]
  p[i]=RecorderProfileEvent{Label:labels.own(uintptr(tokens[i]),&event.label[0]),StartOffset:time.Duration(uint64(event.startOffsetNS)),Ticks:uint64(event.ticks),Duration:time.Duration(uint64(event.durationNS))}
 }
}
func(r *Recorder)Profile()([]RecorderProfileEvent,error){
 if r==nil||r.handle==nil{return nil,fmt.Errorf("after Free")}
 var events *C.mtl_recorder_profile_event_snapshot
 var count C.int
 rc:=C.direct_snapshot(r.handle,&events,&count)
 if rc!=0{return nil,fmt.Errorf("status %d",int(rc))}
 if count<0||count>0&&events==nil{return nil,fmt.Errorf("invalid storage")}
 if count==1{
  event:=events
  profileEvent:=RecorderProfileEvent{Label:C.GoString(&event.label[0]),StartOffset:time.Duration(uint64(event.startOffsetNS)),Ticks:uint64(event.ticks),Duration:time.Duration(uint64(event.durationNS))}
  return []RecorderProfileEvent{profileEvent},nil
 }
 p:=make([]RecorderProfileEvent,int(count))
 nativeEvents:=unsafe.Slice(events,int(count))
 if len(p)>1{
  var tokenStorage *C.uintptr_t
  if rc:=C.mtl_recorder_profile_label_tokens(r.handle,&tokenStorage);rc!=0||tokenStorage==nil{return nil,fmt.Errorf("tokens %d",int(rc))}
  fillRecorderProfileEvents(p,nativeEvents,unsafe.Slice(tokenStorage,len(p)))
 }
 return p,nil
}
`
}

func TestPS6137DirectSliceAndIntoSignatures(t *testing.T) {
	t.Parallel()
	if !build.Default.CgoEnabled {
		t.Skip("genuine cgo replay requires cgo")
	}
	owner := ps6137DirectSliceOwner(t)
	for _, tc := range []struct {
		name, sibling string
		existing      bool
	}{
		{"absent", "", false},
		{"pointer slice", `func(r *Recorder)ProfileInto(dst *[]RecorderProfileEvent)error{return nil}`, true},
		{"returned destination", `func(r *Recorder)ProfileInto(dst []RecorderProfileEvent)([]RecorderProfileEvent,error){return dst,nil}`, true},
		{"unrelated destination", `func(r *Recorder)ProfileInto(dst *RecorderProfile)error{return nil}`, false},
		{"wrong return", `func(r *Recorder)ProfileInto(dst []RecorderProfileEvent)error{return nil}`, false},
		{"wrong receiver", `type Other struct{};func(r *Other)ProfileInto(dst *[]RecorderProfileEvent)error{return nil}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := ps6137Contract()
			c.AcquireCallable = "C.direct_snapshot"
			c.ResultSliceField = ""
			c.ScalarOutArguments = nil
			c.ScalarResultFields = nil
			c.EventSpanField = ""
			c.EventStartField = ""
			c.EventDurationField = ""
			ps6137RunFixture(t, owner+tc.sibling, c, 1, tc.existing)
		})
	}
}

func TestPS6137ConfigAndAmbiguity(t *testing.T) {
	t.Parallel()
	c := ps6137Contract()
	if !c.Valid() || config.UsableNativeSnapshotReuseContractCount([]config.NativeSnapshotReuseContract{c}) != 1 {
		t.Fatal("valid owner contract rejected")
	}
	cfg := config.Config{NativeSnapshotReuseContracts: []config.NativeSnapshotReuseContract{c}}
	compiled := cfg.Compile()
	cfg.NativeSnapshotReuseContracts[0].ScalarOutArguments[0] = 99
	cfg.NativeSnapshotReuseContracts[0].ScalarResultFields[0] = "changed"
	cfg.NativeSnapshotReuseContracts[0].EventNumericFields[0].NativeField = "changed"
	if compiled.NativeSnapshotReuseContracts[0].ScalarOutArguments[0] != 3 || compiled.NativeSnapshotReuseContracts[0].ScalarResultFields[0] != "OmittedMPS" || compiled.NativeSnapshotReuseContracts[0].EventNumericFields[0].NativeField != "startOffsetNS" {
		t.Fatal("compiled config aliases mutable roles")
	}
	good := ps6137Contract()
	if config.UsableNativeSnapshotReuseContractCount([]config.NativeSnapshotReuseContract{good, good}) != 0 {
		t.Fatal("ambiguous candidate counted")
	}
	for _, change := range []func(*config.NativeSnapshotReuseContract){
		func(c *config.NativeSnapshotReuseContract) { c.HandleArgument = -1 }, func(c *config.NativeSnapshotReuseContract) { c.PointerOutArgument = 8 }, func(c *config.NativeSnapshotReuseContract) { c.CountOutArgument = c.HandleArgument },
		func(c *config.NativeSnapshotReuseContract) { c.ScalarResultFields = nil }, func(c *config.NativeSnapshotReuseContract) { c.EventNumericFields[0].ResultField = c.ResultStringField }, func(c *config.NativeSnapshotReuseContract) { c.EventNumericFields[0].NativeField = "x.y" },
		func(c *config.NativeSnapshotReuseContract) { c.LifecycleMethod = "missing" }, func(c *config.NativeSnapshotReuseContract) { c.OwnedOutputAndErrorSemanticsReviewed = false },
		func(c *config.NativeSnapshotReuseContract) { c.NativeRecordShapeAndTerminationReviewed = false }, func(c *config.NativeSnapshotReuseContract) { c.EventDurationField = c.EventStartField }, func(c *config.NativeSnapshotReuseContract) { c.OwnMethod = c.CandidateMethod },
	} {
		c := ps6137Contract()
		change(&c)
		if c.Valid() || config.UsableNativeSnapshotReuseContractCount([]config.NativeSnapshotReuseContract{c}) != 0 {
			t.Fatal("invalid contract counted")
		}
	}
	if PS6137.AutoFix || !PS6137.NeedsConfig {
		t.Fatal("unsafe metadata")
	}
}

func TestPS6137GeneratedCgoRejectsExtraEffects(t *testing.T) {
	t.Parallel()
	if !build.Default.CgoEnabled {
		t.Skip("genuine cgo replay requires cgo")
	}
	for _, mode := range []string{"extra statement", "alias rebind", "duplicate check"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ps6137RunFixture(t, ps6137CgoTypes(t, false)+ps6137Owner(t, false), ps6137Contract(), 0, false, func(pass *analysis.Pass) {
				f := ps6137Functions(pass)["owner.Recorder.Profile"]
				var wrapper *ast.FuncLit
				for _, stmt := range f.decl.Body.List {
					a, ok := stmt.(*ast.AssignStmt)
					if !ok || len(a.Rhs) != 1 {
						continue
					}
					call, ok := ps2110Unparen(a.Rhs[0]).(*ast.CallExpr)
					if !ok {
						continue
					}
					literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit)
					if ok {
						wrapper = literal
						break
					}
				}
				if wrapper == nil {
					t.Fatal("expected actual compiler-generated pointer wrapper")
				}
				var extra ast.Stmt
				switch mode {
				case "extra statement":
					extra = &ast.EmptyStmt{}
				case "alias rebind":
					a, ok := wrapper.Body.List[0].(*ast.AssignStmt)
					if !ok {
						t.Fatal("expected compiler alias")
					}
					copy := *a
					copy.Tok = token.ASSIGN
					extra = &copy
				case "duplicate check":
					for _, stmt := range wrapper.Body.List {
						if _, ok := stmt.(*ast.ExprStmt); ok {
							extra = stmt
							break
						}
					}
					if extra == nil {
						t.Fatal("expected actual pointer check")
					}
				}
				last := len(wrapper.Body.List) - 1
				wrapper.Body.List = append(wrapper.Body.List[:last], extra, wrapper.Body.List[last])
			})
		})
	}
}

func TestPS6137AmbiguousAnalyzerSilent(t *testing.T) {
	t.Parallel()
	if !build.Default.CgoEnabled {
		t.Skip("genuine cgo replay requires cgo")
	}
	dir, cleanup, err := analysistest.WriteFiles(map[string]string{"owner/owner.go": ps6137CgoTypes(t, false) + ps6137Owner(t, false)})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	analyzer := *PS6137.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		pass.Report = func(analysis.Diagnostic) { t.Error("unconfigured/ambiguous source reported") }
		c := ps6137Contract()
		for _, contracts := range [][]config.NativeSnapshotReuseContract{nil, {c, c}} {
			if _, err := runPS6137WithContracts(pass, contracts); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	analysistest.Run(t, dir, &analyzer, "owner")
}
