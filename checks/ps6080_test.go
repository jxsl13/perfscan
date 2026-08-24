package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

type ps6080OverlayImporter struct {
	fallback types.Importer
	packages map[string]*types.Package
}

func (importer ps6080OverlayImporter) Import(path string) (*types.Package, error) {
	if pkg := importer.packages[path]; pkg != nil {
		return pkg, nil
	}
	return importer.fallback.Import(path)
}

func TestPS6080StandardErrorSentinel(t *testing.T) {
	t.Parallel()
	errorType := types.Universe.Lookup("error").Type()
	tests := []struct {
		name     string
		path     string
		variable string
		typ      types.Type
		want     bool
	}{
		{name: "errors unsupported", path: "errors", variable: "ErrUnsupported", typ: errorType, want: true},
		{name: "errors unknown", path: "errors", variable: "ErrOther", typ: errorType},
		{name: "context canceled", path: "context", variable: "Canceled", typ: errorType, want: true},
		{name: "io eof", path: "io", variable: "EOF", typ: errorType, want: true},
		{name: "filesystem invalid", path: "io/fs", variable: "ErrInvalid", typ: errorType, want: true},
		{name: "os unsupported", path: "os", variable: "ErrUnsupported", typ: errorType, want: true},
		{name: "net closed", path: "net", variable: "ErrClosed", typ: errorType, want: true},
		{name: "non error variable", path: "io", variable: "ErrCount", typ: types.Typ[types.Int]},
		{name: "non standard package", path: "example.com/dependency", variable: "ErrUnsupported", typ: errorType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := types.NewPackage(test.path, "dependency")
			variable := types.NewVar(token.NoPos, pkg, test.variable, test.typ)
			if existing := pkg.Scope().Insert(variable); existing != nil {
				t.Fatalf("inserted duplicate object %s", existing.Name())
			}
			pass := &analysis.Pass{TypesInfo: &types.Info{}}
			if got := ps6080StandardErrorSentinel(pass, variable); got != test.want {
				t.Fatalf("ps6080StandardErrorSentinel(%s.%s) = %t, want %t", test.path, test.variable, got, test.want)
			}
		})
	}
}

func TestPS6080TypeHasFreeParameter(t *testing.T) {
	t.Parallel()
	constraint := types.NewInterfaceType(nil, nil)
	constraint.Complete()
	parameter := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), constraint)
	parameterTuple := types.NewTuple(types.NewVar(token.NoPos, nil, "value", parameter))
	parameterSignature := types.NewSignatureType(nil, nil, nil, parameterTuple, nil, false)
	parameterMethod := types.NewFunc(token.NoPos, nil, "M", parameterSignature)
	parameterInterface := types.NewInterfaceType([]*types.Func{parameterMethod}, nil)
	parameterInterface.Complete()
	parameterStruct := types.NewStruct(
		[]*types.Var{types.NewVar(token.NoPos, nil, "F", parameterSignature)},
		[]string{""},
	)
	parameterNamed := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "Box", nil), parameterStruct, nil,
	)
	parameterNamed.SetTypeParams([]*types.TypeParam{parameter})
	parameterAlias := types.NewAlias(
		types.NewTypeName(token.NoPos, nil, "Alias", nil), types.NewPointer(parameter),
	)
	intTuple := types.NewTuple(types.NewVar(token.NoPos, nil, "value", types.Typ[types.Int]))
	intSignature := types.NewSignatureType(nil, nil, nil, intTuple, nil, false)
	intMethod := types.NewFunc(token.NoPos, nil, "M", intSignature)
	intInterface := types.NewInterfaceType([]*types.Func{intMethod}, nil)
	intInterface.Complete()

	tests := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "direct", typ: parameter, want: true},
		{name: "pointer", typ: types.NewPointer(parameter), want: true},
		{name: "slice", typ: types.NewSlice(parameter), want: true},
		{name: "map", typ: types.NewMap(types.Typ[types.String], parameter), want: true},
		{name: "channel", typ: types.NewChan(types.SendRecv, parameter), want: true},
		{name: "struct signature field", typ: parameterStruct, want: true},
		{name: "signature", typ: parameterSignature, want: true},
		{name: "interface method", typ: parameterInterface, want: true},
		{name: "generic named", typ: parameterNamed, want: true},
		{name: "alias", typ: parameterAlias, want: true},
		{name: "concrete pointer", typ: types.NewPointer(types.Typ[types.Int])},
		{name: "concrete signature", typ: intSignature},
		{name: "concrete interface", typ: intInterface},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ps6080TypeHasFreeParameter(test.typ, nil); got != test.want {
				t.Fatalf("ps6080TypeHasFreeParameter(%s) = %t, want %t", test.typ, got, test.want)
			}
		})
	}
}

func TestPS6080StandardErrorSentinelMutation(t *testing.T) {
	t.Parallel()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "sentinels.go", `package sentinels
import (
	. "context"
	. "dependency"
	"errors"
	"io"
	fs "io/fs"
	"net"
	"os"
	"unsafe"
)
type callback func()
var _ = func() { Canceled = nil }
var _ = callback(func() { Canceled = nil })
func mutate() { errors.ErrUnsupported = nil }
var sentinelPointer = &os.ErrInvalid
func genericMutation[T any](value T) {
	switch any(value).(type) {
	case int:
		io.EOF = nil
	}
}
func genericPointerMutation[T any](value *T) {
	switch any(value).(type) {
	case *int:
		io.ErrUnexpectedEOF = nil
	}
}
func genericCaseMutation[T any]() {
	switch any(0).(type) {
	case T:
		io.ErrNoProgress = nil
	}
}
func init() {
	genericMutation(0)
	genericPointerMutation(new(int))
	genericCaseMutation[int]()
}
func deadMutation() {
	dormant := func() { Canceled = nil }
	alias := dormant
	convertedAlias := callback(alias)
	_ = dormant
	_ = alias
	_ = convertedAlias
	_ = func() { Canceled = nil }
	_ = callback(func() { Canceled = nil })
	compared := func() { Canceled = nil }
	_ = compared == nil
	_ = []func(){compared}
	_ = struct{ hook func() }{hook: compared}
	_ = &struct{ hook func() }{hook: compared}
	var assigned func()
	assigned = func() { Canceled = nil }
	_ = assigned
	var overwritten func()
	overwritten = func() { Canceled = nil }
	overwritten = func() { Canceled = nil }
	_ = overwritten
	deadCall := func() { Canceled = nil }
	if false {
		deadCall()
	}
	if false {
		Canceled = nil
	}
	for false {
		Canceled = nil
	}
	for _, Canceled = range []error{} {
	}
	for Canceled = range (func(func(error) bool))(nil) {
	}
	if false {
		func() { Canceled = nil }()
	}
	switch any(0).(type) {
	case string:
		Canceled = nil
	}
	select {
	case (chan struct{})(nil) <- struct{}{}:
		Canceled = nil
	default:
	}
	_ = unsafe.Sizeof(func() { Canceled = nil })
	_ = len([1]func(){func() { Canceled = nil }})
}
func comparedAndInvokedClosureMutation() {
	compared := func() { net.ErrClosed = nil }
	if compared != nil {
		compared()
	}
}
var storedCallbacks []func()
func storedAggregateMutation() {
	stored := func() { fs.ErrClosed = nil }
	storedCallbacks = []func(){stored}
}
func returnedAggregateMutation() []func() {
	returned := func() { os.ErrDeadlineExceeded = nil }
	return []func(){returned}
}
func passedAggregateMutation(invoke func([]func())) {
	passed := func() { os.ErrProcessDone = nil }
	invoke([]func(){passed})
}
func deadAfterReturnMutation() {
	deadAfterReturn := func() { Canceled = nil }
	return
	deadAfterReturn()
}
func invokedClosureMutation() {
	mutate := func() { io.ErrShortBuffer = nil }
	alias := mutate
	convertedAlias := callback(alias)
	convertedAlias()
}
func escapedClosureMutation(invoke func(func())) {
	mutate := func() { os.ErrPermission = nil }
	invoke(mutate)
}
func escapedImportedBinding() {
	Hook = func() { DeadlineExceeded = nil }
	_ = Hook
}
func returnedClosure() (hook func()) {
	hook = func() { io.ErrClosedPipe = nil }
	return
}
func anonymousFactoryMutation() {
	factory := func() (hook func()) {
		hook = func() { io.ErrShortWrite = nil }
		return
	}
	hook := factory()
	hook()
}
func nestedDormantMutation() {
	target := func() { Canceled = nil }
	outer := func() {
		alias := target
		alias()
	}
	_ = target
	_ = outer
	var left, right func()
	left = func() { right() }
	right = func() {
		left()
		Canceled = nil
	}
	_ = left
	_ = right
	var installed func()
	installer := func() { installed = target }
	_ = installer
	if installed != nil {
		installed()
	}
}
func nestedInvokedMutation() {
	target := func() { os.ErrExist = nil }
	outer := func() { target() }
	outer()
}
func nestedImmediateMutation() {
	target := func() { os.ErrNotExist = nil }
	func() { target() }()
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	dependency := types.NewPackage("dependency", "dependency")
	callbackType := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	dependency.Scope().Insert(types.NewVar(token.NoPos, dependency, "Hook", callbackType))
	dependency.MarkComplete()
	pkg, err := (&types.Config{Importer: ps6080OverlayImporter{
		fallback: importer.Default(),
		packages: map[string]*types.Package{"dependency": dependency},
	}}).Check(
		"sentinels", fileSet, []*ast.File{file}, info,
	)
	if err != nil {
		t.Fatal(err)
	}
	importedVariable := func(path, name string) *types.Var {
		for _, dependency := range pkg.Imports() {
			if dependency.Path() == path {
				variable, _ := dependency.Scope().Lookup(name).(*types.Var)
				return variable
			}
		}
		return nil
	}
	assigned := importedVariable("errors", "ErrUnsupported")
	exposed := importedVariable("os", "ErrInvalid")
	unchanged := importedVariable("context", "Canceled")
	generic := importedVariable("io", "EOF")
	genericPointer := importedVariable("io", "ErrUnexpectedEOF")
	genericCase := importedVariable("io", "ErrNoProgress")
	invokedClosure := importedVariable("io", "ErrShortBuffer")
	escapedClosure := importedVariable("os", "ErrPermission")
	escapedImported := importedVariable("context", "DeadlineExceeded")
	returnedClosure := importedVariable("io", "ErrClosedPipe")
	anonymousFactory := importedVariable("io", "ErrShortWrite")
	nestedInvoked := importedVariable("os", "ErrExist")
	nestedImmediate := importedVariable("os", "ErrNotExist")
	comparedAndInvoked := importedVariable("net", "ErrClosed")
	storedAggregate := importedVariable("io/fs", "ErrClosed")
	returnedAggregate := importedVariable("os", "ErrDeadlineExceeded")
	passedAggregate := importedVariable("os", "ErrProcessDone")
	pass := &analysis.Pass{
		Fset:      fileSet,
		Files:     []*ast.File{file},
		Pkg:       pkg,
		TypesInfo: info,
	}
	ps6080VariableInitCaches.Store(pass, &ps6080VariableInitializerCache{})
	defer ps6080VariableInitCaches.Delete(pass)

	if ps6080StandardErrorSentinel(pass, assigned) {
		t.Fatal("assigned standard sentinel was treated as a stable failure")
	}
	if ps6080StandardErrorSentinel(pass, exposed) {
		t.Fatal("address-exposed standard sentinel was treated as a stable failure")
	}
	if !ps6080StandardErrorSentinel(pass, unchanged) {
		t.Fatal("unchanged standard sentinel was not treated as a stable failure")
	}
	if ps6080StandardErrorSentinel(pass, generic) {
		t.Fatal("generic type-switch mutation was treated as unreachable")
	}
	if ps6080StandardErrorSentinel(pass, genericPointer) {
		t.Fatal("composite generic dynamic type-switch mutation was treated as unreachable")
	}
	if ps6080StandardErrorSentinel(pass, genericCase) {
		t.Fatal("generic case type-switch mutation was treated as unreachable")
	}
	if ps6080StandardErrorSentinel(pass, invokedClosure) {
		t.Fatal("invoked-closure mutation was treated as unreachable")
	}
	if ps6080StandardErrorSentinel(pass, escapedClosure) {
		t.Fatal("escaped-closure mutation was treated as unreachable")
	}
	if ps6080StandardErrorSentinel(pass, escapedImported) {
		t.Fatal("closure stored through a dot-imported package variable was treated as local and dormant")
	}
	if ps6080StandardErrorSentinel(pass, returnedClosure) {
		t.Fatal("closure returned through a named result was treated as dormant")
	}
	if ps6080StandardErrorSentinel(pass, anonymousFactory) {
		t.Fatal("closure bare-returned by an invoked anonymous factory was treated as dormant")
	}
	if ps6080StandardErrorSentinel(pass, nestedInvoked) {
		t.Fatal("closure called by an invoked outer closure was treated as dormant")
	}
	if ps6080StandardErrorSentinel(pass, nestedImmediate) {
		t.Fatal("closure called by an immediately invoked outer closure was treated as dormant")
	}
	if ps6080StandardErrorSentinel(pass, comparedAndInvoked) {
		t.Fatal("nil-compared and invoked closure was treated as dormant")
	}
	if ps6080StandardErrorSentinel(pass, storedAggregate) {
		t.Fatal("closure stored in an aggregate was treated as dormant")
	}
	if ps6080StandardErrorSentinel(pass, returnedAggregate) {
		t.Fatal("closure returned in an aggregate was treated as dormant")
	}
	if ps6080StandardErrorSentinel(pass, passedAggregate) {
		t.Fatal("closure passed in an aggregate was treated as dormant")
	}
	pass.Files = nil
	if ps6080StandardErrorSentinel(pass, assigned) || ps6080StandardErrorSentinel(pass, exposed) ||
		ps6080StandardErrorSentinel(pass, generic) || ps6080StandardErrorSentinel(pass, genericPointer) ||
		ps6080StandardErrorSentinel(pass, genericCase) || ps6080StandardErrorSentinel(pass, invokedClosure) ||
		ps6080StandardErrorSentinel(pass, escapedClosure) || ps6080StandardErrorSentinel(pass, escapedImported) ||
		ps6080StandardErrorSentinel(pass, returnedClosure) || ps6080StandardErrorSentinel(pass, anonymousFactory) ||
		ps6080StandardErrorSentinel(pass, nestedInvoked) || ps6080StandardErrorSentinel(pass, nestedImmediate) ||
		ps6080StandardErrorSentinel(pass, comparedAndInvoked) ||
		ps6080StandardErrorSentinel(pass, storedAggregate) ||
		ps6080StandardErrorSentinel(pass, returnedAggregate) ||
		ps6080StandardErrorSentinel(pass, passedAggregate) {
		t.Fatal("standard-sentinel mutation cache was not reused")
	}
}

func TestPS6080TupleReturnSummaryCached(t *testing.T) {
	t.Parallel()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "tuple.go", `package tuple
import (
	"dep"
	"errors"
	"os"
	"runtime"
	"unsafe"
)
func helper() (bool, error) { return false, errors.New("unsupported") }
func outer() (bool, error) { return helper() }
func namedHelper() (ok bool, err error) { ok, err = helper(); return }
func namedOuter() (bool, error) { return namedHelper() }
func namedExplicit() (ok bool, err error) {
	ok = false
	err = errors.New("unsupported")
	return ok, err
}
func namedExplicitOuter() (bool, error) { return namedExplicit() }
func repeated() (a, b bool) { a = false; return a, a }
func repeatedOuter() (bool, bool) { return repeated() }
func swapped() (a, b bool) { a = false; b = true; return b, a }
func swappedOuter() (bool, bool) { return swapped() }
func known() (ok bool, err error) { ok = true; err = nil; return }
func unknown(flag bool) (ok bool, err error) {
	ok = true
	if flag { err = errors.New("dynamic") }
	return
}
func shadowedNilError() (bool, error) {
	var nil error = errors.New("not nil")
	return true, nil
}
func shadowedNilErrorOuter() (bool, error) { return shadowedNilError() }
func supportOrPanic(flag bool) (bool, error) {
	if flag { return true, nil }
	panic("unsupported")
}
func panicOuter() (bool, error) { return supportOrPanic(false) }
func supportOrLoop(flag bool) (bool, error) {
	if flag { return true, nil }
	for {}
}
func loopOuter() (bool, error) { return supportOrLoop(false) }
func stop() { select {} }
func directStop() (bool, error) { stop(); return true, nil }
func directStopOuter() (bool, error) { return directStop() }
func nestedStop() (bool, error) {
	_ = func() int { select {} }()
	return true, nil
}
func nestedStopOuter() (bool, error) { return nestedStop() }
func nestedHelperStop() int {
	_ = func() int { select {} }()
	return 1
}
func nestedHelper() (bool, error) {
	_ = nestedHelperStop()
	return true, nil
}
func nestedHelperOuter() (bool, error) { return nestedHelper() }
func deferredStop() (bool, error) { defer panic("unsupported"); return true, nil }
func deferredStopOuter() (bool, error) { return deferredStop() }
func nestedDeferredStop() { defer panic("unsupported") }
func nestedDeferred() (bool, error) {
	nestedDeferredStop()
	return true, nil
}
func nestedDeferredOuter() (bool, error) { return nestedDeferred() }
func exitStop() (bool, error) { os.Exit(1); return true, nil }
func exitStopOuter() (bool, error) { return exitStop() }
func goexitStop() (bool, error) { runtime.Goexit(); return true, nil }
func goexitStopOuter() (bool, error) { return goexitStop() }
func externalBodylessStop() (bool, error) { dep.Stop(); return true, nil }
func externalBodylessStopOuter() (bool, error) { return externalBodylessStop() }
type stopper interface { Stop() }
type blockingStopper struct{}
func (blockingStopper) Stop() { select {} }
func dynamicStop(value stopper) (bool, error) { value.Stop(); return true, nil }
func dynamicStopOuter() (bool, error) { return dynamicStop(blockingStopper{}) }
func synchronousSend(channel chan int) (bool, error) {
	channel <- 1
	return true, nil
}
func synchronousSendOuter() (bool, error) { return synchronousSend(make(chan int)) }
func synchronousReceive(channel chan int) (bool, error) {
	<-channel
	return true, nil
}
func synchronousReceiveOuter() (bool, error) { return synchronousReceive(make(chan int)) }
func nilSynchronousReceive() (bool, error) {
	<-(chan int)(nil)
	return true, nil
}
func nilSynchronousReceiveOuter() (bool, error) { return nilSynchronousReceive() }
func blockingSelectNoDefault(channel chan int) (bool, error) {
	select { case <-channel: }
	return true, nil
}
func blockingSelectNoDefaultOuter() (bool, error) {
	return blockingSelectNoDefault(make(chan int))
}
func returningSelectDefault(channel chan int) (bool, error) {
	select { case <-channel: default: }
	return true, nil
}
func returningSelectDefaultOuter() (bool, error) {
	return returningSelectDefault(make(chan int))
}
func closeNil() (bool, error) {
	close((chan int)(nil))
	return true, nil
}
func closeNilOuter() (bool, error) { return closeNil() }
func closeUnknown(channel chan int) (bool, error) {
	close(channel)
	return true, nil
}
func closeUnknownOuter() (bool, error) { return closeUnknown(make(chan int)) }
func dynamicMake(length int) (bool, error) {
	_ = make([]int, length)
	return true, nil
}
func dynamicMakeOuter() (bool, error) { return dynamicMake(-1) }
func fixedMake() (bool, error) {
	_ = make([]int, 1, 2)
	return true, nil
}
func fixedMakeOuter() (bool, error) { return fixedMake() }
func deleteUncomparableKey() (bool, error) {
	delete(map[any]int{}, any([]int{}))
	return true, nil
}
func deleteUncomparableKeyOuter() (bool, error) { return deleteUncomparableKey() }
func deleteComparableKey() (bool, error) {
	delete(map[any]int{}, any("key"))
	return true, nil
}
func deleteComparableKeyOuter() (bool, error) { return deleteComparableKey() }
func implicitDeleteUncomparableKey() (bool, error) {
	delete(map[any]int{}, []int{})
	return true, nil
}
func implicitDeleteUncomparableKeyOuter() (bool, error) {
	return implicitDeleteUncomparableKey()
}
func nilMapWrite() (bool, error) {
	(map[int]int)(nil)[0] = 1
	return true, nil
}
func nilMapWriteOuter() (bool, error) { return nilMapWrite() }
func nilMapLookup() (bool, error) {
	_ = (map[int]int)(nil)[0]
	return true, nil
}
func nilMapLookupOuter() (bool, error) { return nilMapLookup() }
func unknownMapWrite(values map[int]int) (bool, error) {
	values[0] = 1
	return true, nil
}
func unknownMapWriteOuter() (bool, error) { return unknownMapWrite(make(map[int]int)) }
func madeMapWrite() (bool, error) {
	make(map[int]int)[0] = 1
	return true, nil
}
func madeMapWriteOuter() (bool, error) { return madeMapWrite() }
func unhashableMapLiteral() (bool, error) {
	_ = map[any]int{any([]int{}): 1}
	return true, nil
}
func unhashableMapLiteralOuter() (bool, error) { return unhashableMapLiteral() }
func unknownMapLiteral(key any) (bool, error) {
	_ = map[any]int{key: 1}
	return true, nil
}
func unknownMapLiteralOuter() (bool, error) { return unknownMapLiteral("key") }
func comparableMapLiteral() (bool, error) {
	_ = map[any]int{any("key"): 1}
	return true, nil
}
func comparableMapLiteralOuter() (bool, error) { return comparableMapLiteral() }
func implicitUnhashableMapLiteral() (bool, error) {
	_ = map[any]int{[]int{}: 1}
	return true, nil
}
func implicitUnhashableMapLiteralOuter() (bool, error) {
	return implicitUnhashableMapLiteral()
}
func implicitUnhashableMapIndex() (bool, error) {
	_ = map[any]int{}[[]int{}]
	return true, nil
}
func implicitUnhashableMapIndexOuter() (bool, error) {
	return implicitUnhashableMapIndex()
}
func implicitComparableMapKeys() (bool, error) {
	values := map[any]int{"key": 1}
	_ = values["key"]
	delete(values, "key")
	return true, nil
}
func implicitComparableMapKeysOuter() (bool, error) { return implicitComparableMapKeys() }
func failedTypeAssertion() (bool, error) {
	_ = any(1).(string)
	return true, nil
}
func failedTypeAssertionOuter() (bool, error) { return failedTypeAssertion() }
func failedTypeAssertionCommaOK() (bool, error) {
	_, _ = (any(1).(string))
	return true, nil
}
func failedTypeAssertionCommaOKOuter() (bool, error) {
	return failedTypeAssertionCommaOK()
}
func invalidSliceToArray() (bool, error) {
	_ = [1]int([]int{})
	return true, nil
}
func invalidSliceToArrayOuter() (bool, error) { return invalidSliceToArray() }
func validSliceToArray() (bool, error) {
	_ = [0]int([]int{})
	return true, nil
}
func validSliceToArrayOuter() (bool, error) { return validSliceToArray() }
func invalidSliceBounds() (bool, error) {
	_ = ([]int)(nil)[:1]
	return true, nil
}
func invalidSliceBoundsOuter() (bool, error) { return invalidSliceBounds() }
func validSliceBounds() (bool, error) {
	_ = ([]int)(nil)[:0]
	return true, nil
}
func validSliceBoundsOuter() (bool, error) { return validSliceBounds() }
func nilPointerDereference() (bool, error) {
	_ = *(*int)(nil)
	return true, nil
}
func nilPointerDereferenceOuter() (bool, error) { return nilPointerDereference() }
type fieldHolder struct { value int }
func (fieldHolder) Value() {}
func (*fieldHolder) PointerValue() {}
func nilPointerField() (bool, error) {
	_ = (*fieldHolder)(nil).value
	return true, nil
}
func nilPointerFieldOuter() (bool, error) { return nilPointerField() }
func nilPointerValueMethod() (bool, error) {
	(*fieldHolder)(nil).Value()
	return true, nil
}
func nilPointerValueMethodOuter() (bool, error) { return nilPointerValueMethod() }
func nilPointerReceiverMethod() (bool, error) {
	(*fieldHolder)(nil).PointerValue()
	return true, nil
}
func nilPointerReceiverMethodOuter() (bool, error) { return nilPointerReceiverMethod() }
type methodInterface interface{ Method() }
type methodReceiver struct{}
func (*methodReceiver) Method() {}
func boxedNilPointerReceiver() (bool, error) {
	methodInterface((*methodReceiver)(nil)).Method()
	return true, nil
}
func boxedNilPointerReceiverOuter() (bool, error) { return boxedNilPointerReceiver() }
func nilInterfaceMethod() (bool, error) {
	var receiver methodInterface
	receiver.Method()
	return true, nil
}
func nilInterfaceMethodOuter() (bool, error) { return nilInterfaceMethod() }
func nilSliceIndex() (bool, error) {
	_ = ([]int)(nil)[0]
	return true, nil
}
func nilSliceIndexOuter() (bool, error) { return nilSliceIndex() }
func uncomparableInterfaceComparison() (bool, error) {
	_ = any([]int{}) == any([]int{})
	return true, nil
}
func uncomparableInterfaceComparisonOuter() (bool, error) {
	return uncomparableInterfaceComparison()
}
func differentInterfaceComparison() (bool, error) {
	_ = any([]int{}) == any(map[int]int{})
	return true, nil
}
func differentInterfaceComparisonOuter() (bool, error) {
	return differentInterfaceComparison()
}
func unknownInterfaceComparison(left, right any) (bool, error) {
	_ = left == right
	return true, nil
}
func unknownInterfaceComparisonOuter() (bool, error) {
	return unknownInterfaceComparison("left", "right")
}
func comparableInterfaceComparison() (bool, error) {
	_ = any("left") == any("right")
	return true, nil
}
func comparableInterfaceComparisonOuter() (bool, error) {
	return comparableInterfaceComparison()
}
func unknownNilInterfaceComparison(value any) (bool, error) {
	_ = value == nil
	return true, nil
}
func unknownNilInterfaceComparisonOuter() (bool, error) {
	return unknownNilInterfaceComparison("value")
}
func unknownComparableInterfaceComparison(value any) (bool, error) {
	_ = value == any("known")
	return true, nil
}
func unknownComparableInterfaceComparisonOuter() (bool, error) {
	return unknownComparableInterfaceComparison("value")
}
func unknownUncomparableInterfaceComparison(value any) (bool, error) {
	_ = value == any([]int{})
	return true, nil
}
func unknownUncomparableInterfaceComparisonOuter() (bool, error) {
	return unknownUncomparableInterfaceComparison("value")
}
func signedShift(amount int) (bool, error) {
	_ = 1 << amount
	return true, nil
}
func signedShiftOuter() (bool, error) { return signedShift(1) }
func unsignedShift(amount uint) (bool, error) {
	_ = 1 << amount
	return true, nil
}
func unsignedShiftOuter() (bool, error) { return unsignedShift(1) }
func fixedShift() (bool, error) {
	_ = 1 << 1
	return true, nil
}
func fixedShiftOuter() (bool, error) { return fixedShift() }
func unevaluatedFailedTypeAssertion() (bool, error) {
	_ = false && any(1).(string) == ""
	_ = unsafe.Sizeof(any(1).(string))
	return true, nil
}
func unevaluatedFailedTypeAssertionOuter() (bool, error) {
	return unevaluatedFailedTypeAssertion()
}
func nilChannel() (bool, error) {
	for range (chan int)(nil) {}
	return true, nil
}
func nilChannelOuter() (bool, error) { return nilChannel() }
func nilIterator() (bool, error) {
	for range (func(func(int) bool))(nil) {}
	return true, nil
}
func nilIteratorOuter() (bool, error) { return nilIterator() }
func nilSlice() (bool, error) {
	for range ([]int)(nil) {}
	return true, nil
}
func nilSliceOuter() (bool, error) { return nilSlice() }
func nilMap() (bool, error) {
	for range (map[int]int)(nil) {}
	return true, nil
}
func nilMapOuter() (bool, error) { return nilMap() }
func selfStop() (bool, error) { return selfStop() }
func selfStopOuter() (bool, error) { return selfStop() }
func mutualStopA() (bool, error) { return mutualStopB() }
func mutualStopB() (bool, error) { return mutualStopA() }
func mutualStopOuter() (bool, error) { return mutualStopA() }
func recursiveBase(stop bool) (bool, error) {
	if stop { return true, nil }
	return recursiveBase(true)
}
func recursiveBaseOuter() (bool, error) { return recursiveBase(false) }
func conditionalRecursion(stop bool) {
	if stop { return }
	conditionalRecursion(false)
}
func conditionalRecursionSupport() (bool, error) {
	conditionalRecursion(false)
	return true, nil
}
func conditionalRecursionOuter() (bool, error) { return conditionalRecursionSupport() }
func unevaluatedShort() (bool, error) {
	_ = false && func() bool { select {} }()
	return true, nil
}
func unevaluatedSize() (bool, error) {
	_ = unsafe.Sizeof(func() int { select {} }())
	return true, nil
}
func unevaluatedAlign() (bool, error) {
	_ = unsafe.Alignof(func() int { select {} }())
	return true, nil
}
func unevaluatedOffset() (bool, error) {
	_ = unsafe.Offsetof(struct{ value int }{}.value)
	return true, nil
}
func unevaluatedShortOuter() (bool, error) { return unevaluatedShort() }
func unevaluatedSizeOuter() (bool, error) { return unevaluatedSize() }
func unevaluatedAlignOuter() (bool, error) { return unevaluatedAlign() }
func unevaluatedOffsetOuter() (bool, error) { return unevaluatedOffset() }
func localCallableStop() (bool, error) {
	stopper := func() { select {} }
	stopper()
	return true, nil
}
func localCallableStopOuter() (bool, error) { return localCallableStop() }
type MutableStopFunc func()
func (function *MutableStopFunc) Stop() { *function = func() { select {} } }
func pointerReceiverCallableStop() (bool, error) {
	stopper := MutableStopFunc(func() {})
	stopper.Stop()
	stopper()
	return true, nil
}
func pointerReceiverCallableStopOuter() (bool, error) {
	return pointerReceiverCallableStop()
}
var packageStopper = func() { select {} }
var ExportedPackageStopper = func() { select {} }
func packageCallableStop() (bool, error) {
	packageStopper()
	return true, nil
}
func packageCallableStopOuter() (bool, error) { return packageCallableStop() }
func exportedPackageCallableStop() (bool, error) {
	ExportedPackageStopper()
	return true, nil
}
func exportedPackageCallableStopOuter() (bool, error) { return exportedPackageCallableStop() }
func packageCallableLeaf() {}
var packageForwarder = func() { packageCallableLeaf() }
func packageFanout0() { packageForwarder() }
func packageFanout1() { packageForwarder() }
func packageFanout2() { packageForwarder() }
func packageFanout3() { packageForwarder() }
func packageFanout4() { packageForwarder() }
func packageFanout5() { packageForwarder() }
func packageFanout6() { packageForwarder() }
func packageFanout7() { packageForwarder() }
func packageCallableReturn() (bool, error) {
	packageForwarder()
	return true, nil
}
func packageCallableReturnOuter() (bool, error) { return packageCallableReturn() }
type StopFunc func()
func convertedCallableStop() (bool, error) {
	StopFunc(func() { select {} })()
	return true, nil
}
func convertedCallableStopOuter() (bool, error) { return convertedCallableStop() }
func ambiguousCallableStop(call func()) (bool, error) {
	call()
	return true, nil
}
func ambiguousCallableStopOuter() (bool, error) {
	return ambiguousCallableStop(func() { select {} })
}
func recoverPanic() { _ = recover() }
func recoveredDeferred() (bool, error) {
	defer func() { _ = recover() }()
	defer panic("recovered")
	return true, nil
}
func recoveredDeferredOuter() (bool, error) { return recoveredDeferred() }
func recoveredNamedDeferred() (bool, error) {
	defer recoverPanic()
	defer panic("recovered")
	return true, nil
}
func recoveredNamedDeferredOuter() (bool, error) { return recoveredNamedDeferred() }
func recoveredCloseNil() (bool, error) {
	defer recoverPanic()
	defer close((chan int)(nil))
	return true, nil
}
func recoveredCloseNilOuter() (bool, error) { return recoveredCloseNil() }
func recoveredDirectAssertion() (ok bool, err error) {
	ok = true
	defer recoverPanic()
	_ = any(1).(string)
	return
}
func recoveredDirectAssertionOuter() (bool, error) { return recoveredDirectAssertion() }
func recoveredUnnamedDirectAssertion() (bool, error) {
	defer recoverPanic()
	_ = any(1).(string)
	return true, nil
}
func recoveredUnnamedDirectAssertionOuter() (bool, error) {
	return recoveredUnnamedDirectAssertion()
}
func unrecoveredDeferred() (bool, error) {
	defer panic("not recovered")
	defer func() { _ = recover() }()
	return true, nil
}
func unrecoveredDeferredOuter() (bool, error) { return unrecoveredDeferred() }
func conditionalRecoveredDeferred(recoverIt bool) (bool, error) {
	if recoverIt { defer func() { _ = recover() }() }
	defer panic("not always recovered")
	return true, nil
}
func conditionalRecoveredDeferredOuter() (bool, error) {
	return conditionalRecoveredDeferred(false)
}
func impossibleSelectSupport() (bool, error) {
	select {
	case <-(chan int)(nil):
		stop()
	default:
	}
	return true, nil
}
func impossibleSelectSupportOuter() (bool, error) { return impossibleSelectSupport() }
func liveSelectStop(channel chan int) (bool, error) {
	select {
	case <-channel:
		stop()
	default:
	}
	return true, nil
}
func liveSelectStopOuter() (bool, error) { return liveSelectStop(make(chan int)) }
func blockingChannel() chan int { select {} }
func eagerSelectOperand() (bool, error) {
	select {
	case <-blockingChannel():
	default:
	}
	return true, nil
}
func eagerSelectOperandOuter() (bool, error) { return eagerSelectOperand() }
func impossibleTypeSwitchSupport() (bool, error) {
	switch any(1).(type) {
	case string:
		stop()
	case int:
	}
	return true, nil
}
func impossibleTypeSwitchSupportOuter() (bool, error) { return impossibleTypeSwitchSupport() }
func liveTypeSwitchStop() (bool, error) {
	switch any("live").(type) {
	case string:
		stop()
	case int:
	}
	return true, nil
}
func liveTypeSwitchStopOuter() (bool, error) { return liveTypeSwitchStop() }
func shadowedNilTypeSwitchSupport() (bool, error) {
	{
		nil := 1
		switch any(nil).(type) {
		case int:
		default:
			stop()
		}
	}
	return true, nil
}
func shadowedNilTypeSwitchSupportOuter() (bool, error) {
	return shadowedNilTypeSwitchSupport()
}
func selectedTypeWithStoppingDefault() (bool, error) {
	switch any(1).(type) {
	case int:
	default:
		stop()
	}
	return true, nil
}
func selectedTypeWithStoppingDefaultOuter() (bool, error) {
	return selectedTypeWithStoppingDefault()
}
func selectedStoppingDefault() (bool, error) {
	switch any("live").(type) {
	case int:
	default:
		stop()
	}
	return true, nil
}
func selectedStoppingDefaultOuter() (bool, error) { return selectedStoppingDefault() }
func capturedCallableReturn() (bool, error) {
	leaf := func() {}
	wrapper := func() { leaf() }
	wrapper()
	return true, nil
}
func capturedCallableReturnOuter() (bool, error) { return capturedCallableReturn() }
func capturedCallableStop() (bool, error) {
	leaf := func() { select {} }
	wrapper := func() { leaf() }
	wrapper()
	return true, nil
}
func capturedCallableStopOuter() (bool, error) { return capturedCallableStop() }
var packageCapturedReturn = func() {
	leaf := func() {}
	func() { leaf() }()
}
func packageCapturedReturnSupport() (bool, error) {
	packageCapturedReturn()
	return true, nil
}
func packageCapturedReturnSupportOuter() (bool, error) {
	return packageCapturedReturnSupport()
}
var packageCapturedStop = func() {
	leaf := func() { select {} }
	func() { leaf() }()
}
func packageCapturedStopSupport() (bool, error) {
	packageCapturedStop()
	return true, nil
}
func packageCapturedStopSupportOuter() (bool, error) {
	return packageCapturedStopSupport()
}
func namedPanicker() { panic("named") }
func recoveredNamedPanicker() (bool, error) {
	defer recoverPanic()
	defer namedPanicker()
	return true, nil
}
func recoveredNamedPanickerOuter() (bool, error) { return recoveredNamedPanicker() }
func recoveredLiteralPanicker() (bool, error) {
	defer recoverPanic()
	defer func() { panic("literal") }()
	return true, nil
}
func recoveredLiteralPanickerOuter() (bool, error) { return recoveredLiteralPanicker() }
func exitNow() { os.Exit(1) }
func unrecoveredExitDefer() (bool, error) {
	defer recoverPanic()
	defer exitNow()
	return true, nil
}
func unrecoveredExitDeferOuter() (bool, error) { return unrecoveredExitDefer() }
func unrecoveredLoopDefer() (bool, error) {
	defer recoverPanic()
	defer stop()
	return true, nil
}
func unrecoveredLoopDeferOuter() (bool, error) { return unrecoveredLoopDefer() }
func blockingInt() int { select {} }
func consumeTwo(any, any) {}
func blockedArgumentBeforeTypeAssertion() {
	consumeTwo(blockingInt(), any(1).(string))
}
func unrecoveredBlockedArgumentBeforeTypeAssertion() (bool, error) {
	defer recoverPanic()
	defer blockedArgumentBeforeTypeAssertion()
	return true, nil
}
func unrecoveredBlockedArgumentBeforeTypeAssertionOuter() (bool, error) {
	return unrecoveredBlockedArgumentBeforeTypeAssertion()
}
func returningArgumentBeforeTypeAssertion() {
	consumeTwo(1, any(1).(string))
}
func recoveredReturningArgumentBeforeTypeAssertion() (bool, error) {
	defer recoverPanic()
	defer returningArgumentBeforeTypeAssertion()
	return true, nil
}
func recoveredReturningArgumentBeforeTypeAssertionOuter() (bool, error) {
	return recoveredReturningArgumentBeforeTypeAssertion()
}
func conditionalTypeAssertionPanic() {
	flag := len(os.Args) > 0
	_ = flag && any(1).(string) == ""
}
func unrecoveredConditionalTypeAssertionPanic() (bool, error) {
	defer recoverPanic()
	defer conditionalTypeAssertionPanic()
	return true, nil
}
func unrecoveredConditionalTypeAssertionPanicOuter() (bool, error) {
	return unrecoveredConditionalTypeAssertionPanic()
}
func guaranteedTypeAssertionPanic() {
	_ = true && any(1).(string) == ""
}
func recoveredGuaranteedTypeAssertionPanic() (bool, error) {
	defer recoverPanic()
	defer guaranteedTypeAssertionPanic()
	return true, nil
}
func recoveredGuaranteedTypeAssertionPanicOuter() (bool, error) {
	return recoveredGuaranteedTypeAssertionPanic()
}
func immediateDeferredArgumentPanic() (bool, error) {
	defer consumeTwo(any(1).(string), 0)
	return true, nil
}
func immediateDeferredArgumentPanicOuter() (bool, error) {
	return immediateDeferredArgumentPanic()
}
func safeDeferredArgument() (bool, error) {
	defer consumeTwo(any("value").(string), 0)
	return true, nil
}
func safeDeferredArgumentOuter() (bool, error) { return safeDeferredArgument() }
func immediateDeferredCalleePanic() (bool, error) {
	defer any(1).(func())()
	return true, nil
}
func immediateDeferredCalleePanicOuter() (bool, error) {
	return immediateDeferredCalleePanic()
}
func safeDeferredCallee() (bool, error) {
	defer func() {}()
	return true, nil
}
func safeDeferredCalleeOuter() (bool, error) { return safeDeferredCallee() }
func dynamicCompoundDivide(divisor int) (bool, error) {
	value := 1
	value /= divisor
	return true, nil
}
func dynamicCompoundDivideOuter() (bool, error) { return dynamicCompoundDivide(1) }
func fixedCompoundDivide() (bool, error) {
	value := 1
	value /= 1
	return true, nil
}
func fixedCompoundDivideOuter() (bool, error) { return fixedCompoundDivide() }
func signedCompoundShift(amount int) (bool, error) {
	value := 1
	value <<= amount
	return true, nil
}
func signedCompoundShiftOuter() (bool, error) { return signedCompoundShift(1) }
func unsignedCompoundShift(amount uint) (bool, error) {
	value := 1
	value <<= amount
	return true, nil
}
func unsignedCompoundShiftOuter() (bool, error) { return unsignedCompoundShift(1) }
type embeddedInner struct{ field int }
func (embeddedInner) PromotedValue() {}
func (*embeddedInner) PromotedPointer() {}
type embeddedOuter struct{ *embeddedInner }
func promotedNilField() (bool, error) {
	_ = embeddedOuter{}.field
	return true, nil
}
func promotedNilFieldOuter() (bool, error) { return promotedNilField() }
func promotedNilValueMethod() (bool, error) {
	embeddedOuter{}.PromotedValue()
	return true, nil
}
func promotedNilValueMethodOuter() (bool, error) { return promotedNilValueMethod() }
func promotedNilPointerMethod() (bool, error) {
	embeddedOuter{}.PromotedPointer()
	return true, nil
}
func promotedNilPointerMethodOuter() (bool, error) { return promotedNilPointerMethod() }
func promotedNilOuterPointerMethod() (bool, error) {
	(*embeddedOuter)(nil).PromotedPointer()
	return true, nil
}
func promotedNilOuterPointerMethodOuter() (bool, error) {
	return promotedNilOuterPointerMethod()
}
func promotedNonNilOuterPointerMethod() (bool, error) {
	(&embeddedOuter{}).PromotedPointer()
	return true, nil
}
func promotedNonNilOuterPointerMethodOuter() (bool, error) {
	return promotedNonNilOuterPointerMethod()
}
func promotedNonNilField() (bool, error) {
	_ = (embeddedOuter{embeddedInner: &embeddedInner{}}).field
	return true, nil
}
func promotedNonNilFieldOuter() (bool, error) { return promotedNonNilField() }
func unknownPromotedFieldWrite(value embeddedOuter) (bool, error) {
	value.field = 1
	return true, nil
}
func unknownPromotedFieldWriteOuter() (bool, error) {
	return unknownPromotedFieldWrite(embeddedOuter{embeddedInner: &embeddedInner{}})
}
func unknownPromotedFieldIncrement(value embeddedOuter) (bool, error) {
	value.field++
	return true, nil
}
func unknownPromotedFieldIncrementOuter() (bool, error) {
	return unknownPromotedFieldIncrement(embeddedOuter{embeddedInner: &embeddedInner{}})
}
func directPromotedFieldWrite() (bool, error) {
	(&embeddedOuter{embeddedInner: &embeddedInner{}}).field = 1
	return true, nil
}
func directPromotedFieldWriteOuter() (bool, error) { return directPromotedFieldWrite() }
func unknownPointerArrayIndex(values *[1]int) (bool, error) {
	_ = values[0]
	return true, nil
}
func unknownPointerArrayIndexOuter() (bool, error) {
	return unknownPointerArrayIndex(&[1]int{})
}
func unknownPointerArraySlice(values *[1]int) (bool, error) {
	_ = values[:]
	return true, nil
}
func unknownPointerArraySliceOuter() (bool, error) {
	return unknownPointerArraySlice(&[1]int{})
}
func directPointerArrayIndex() (bool, error) {
	_ = (&[1]int{})[0]
	return true, nil
}
func directPointerArrayIndexOuter() (bool, error) { return directPointerArrayIndex() }
func directPointerArraySlice() (bool, error) {
	_ = (&[1]int{})[:]
	return true, nil
}
func directPointerArraySliceOuter() (bool, error) { return directPointerArraySlice() }
func hardPanicker() { panic(blockingInt()) }
func unrecoveredHardPanicker() (bool, error) {
	defer recoverPanic()
	defer hardPanicker()
	return true, nil
}
func unrecoveredHardPanickerOuter() (bool, error) { return unrecoveredHardPanicker() }
func exitingPanicker() {
	defer os.Exit(1)
	panic("unreachable recovery")
}
func unrecoveredExitingPanicker() (bool, error) {
	defer recoverPanic()
	defer exitingPanicker()
	return true, nil
}
func unrecoveredExitingPanickerOuter() (bool, error) { return unrecoveredExitingPanicker() }
func blockedBeforePanic() {
	_ = blockingInt()
	panic("unreachable")
}
func channelBeforePanic() {
	<-(chan int)(nil)
	panic("unreachable")
}
func unrecoveredChannelBeforePanic() (bool, error) {
	defer recoverPanic()
	defer channelBeforePanic()
	return true, nil
}
func unrecoveredChannelBeforePanicOuter() (bool, error) {
	return unrecoveredChannelBeforePanic()
}
func unrecoveredBlockedBeforePanic() (bool, error) {
	defer recoverPanic()
	defer blockedBeforePanic()
	return true, nil
}
func unrecoveredBlockedBeforePanicOuter() (bool, error) {
	return unrecoveredBlockedBeforePanic()
}
func blockedGoArgumentBeforePanic() {
	go func(int) {}(blockingInt())
	panic("unreachable")
}
func unrecoveredBlockedGoArgumentBeforePanic() (bool, error) {
	defer recoverPanic()
	defer blockedGoArgumentBeforePanic()
	return true, nil
}
func unrecoveredBlockedGoArgumentBeforePanicOuter() (bool, error) {
	return unrecoveredBlockedGoArgumentBeforePanic()
}
func alternatingLeafPanic() { panic("alternating") }
func alternatingRecoveredReturn() {
	defer recoverPanic()
	alternatingLeafPanic()
}
func alternatingPanicAfterReturn() {
	alternatingRecoveredReturn()
	panic("alternating")
}
func alternatingSupport() (ok bool, err error) {
	ok = true
	defer recoverPanic()
	alternatingPanicAfterReturn()
	return
}
func alternatingSupportOuter() (bool, error) { return alternatingSupport() }
func recoveredPartialSupportToFailure() (ok bool, err error) {
	ok = true
	defer recoverPanic()
	ok, (map[int]int)(nil)[0] = false, 1
	return
}
func recoveredPartialSupportToFailureOuter() (bool, error) {
	return recoveredPartialSupportToFailure()
}
func recoveredPartialFailureToSupport() (ok bool, err error) {
	ok = false
	defer recoverPanic()
	ok, (map[int]int)(nil)[0] = true, 1
	return
}
func recoveredPartialFailureToSupportOuter() (bool, error) {
	return recoveredPartialFailureToSupport()
}
func chain0() {}
func chain1() { chain0() }
func chain2() { chain1() }
func chain3() { chain2() }
func chain4() { chain3() }
func chain5() { chain4() }
func chain6() { chain5() }
func chain7() { chain6() }
func wideChainCaller() (bool, error) {
	chain0()
	chain1()
	chain2()
	chain3()
	chain4()
	chain5()
	chain6()
	chain7()
	return true, nil
}
func wideChainCallerOuter() (bool, error) { return wideChainCaller() }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	dependency := types.NewPackage("dep", "dep")
	dependency.Scope().Insert(types.NewFunc(
		token.NoPos, dependency, "Stop", types.NewSignatureType(
			nil, nil, nil, types.NewTuple(), types.NewTuple(), false,
		),
	))
	dependency.MarkComplete()
	pkg, err := (&types.Config{Importer: ps6080OverlayImporter{
		fallback: importer.Default(),
		packages: map[string]*types.Package{"dep": dependency},
	}}).Check(
		"tuple", fileSet, []*ast.File{file}, info,
	)
	if err != nil {
		t.Fatal(err)
	}
	helper, _ := pkg.Scope().Lookup("helper").(*types.Func)
	forwarded := make(map[string]ast.Expr)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, direct := ps2110Unparen(call.Fun).(*ast.Ident)
		functionObject, local := info.ObjectOf(identifier).(*types.Func)
		if direct && local && functionObject.Pkg() == pkg {
			forwarded[functionObject.Name()] = call
		}
		return true
	})
	if forwarded["helper"] == nil {
		t.Fatal("forwarded tuple call not found")
	}
	pass := &analysis.Pass{Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	cache := &ps6080ReturnFailureCache{}
	ps6080ReturnFailureCaches.Store(pass, cache)
	defer ps6080ReturnFailureCaches.Delete(pass)
	for index, nilSupports := range []bool{false, true} {
		if !ps6080CallResultAlwaysFailure(
			pass, forwarded["helper"], index, nilSupports, cache,
			make(map[ps6080ReturnFailureKey]bool),
		) {
			t.Fatalf("tuple result %d was not summarized as failure", index)
		}
		if ps6080CallResultAlwaysSupports(
			pass, forwarded["helper"], index, nilSupports, cache,
			make(map[ps6080ReturnFailureKey]bool),
		) {
			t.Fatalf("tuple result %d was summarized as support", index)
		}
	}
	summary := ps6080FunctionReturnSummaryFor(pass, cache, helper)
	if summary.builds != 1 {
		t.Fatalf("tuple return summary built %d times, want 1", summary.builds)
	}
	for _, name := range []string{"namedHelper", "namedExplicit"} {
		for index, nilSupports := range []bool{false, true} {
			if !ps6080CallResultAlwaysFailure(
				pass, forwarded[name], index, nilSupports, cache,
				make(map[ps6080ReturnFailureKey]bool),
			) {
				t.Fatalf("%s tuple result %d was not summarized as failure", name, index)
			}
		}
	}
	for index := range 2 {
		if !ps6080CallResultAlwaysFailure(
			pass, forwarded["repeated"], index, false, cache,
			make(map[ps6080ReturnFailureKey]bool),
		) {
			t.Fatalf("repeated named result %d was not substituted", index)
		}
	}
	if !ps6080CallResultAlwaysSupports(
		pass, forwarded["swapped"], 0, false, cache,
		make(map[ps6080ReturnFailureKey]bool),
	) || !ps6080CallResultAlwaysFailure(
		pass, forwarded["swapped"], 1, false, cache,
		make(map[ps6080ReturnFailureKey]bool),
	) {
		t.Fatal("swapped named results were not projected by destination")
	}
	for _, name := range []string{"supportOrPanic", "supportOrLoop"} {
		for index, nilSupports := range []bool{false, true} {
			if ps6080CallResultAlwaysSupports(
				pass, forwarded[name], index, nilSupports, cache,
				make(map[ps6080ReturnFailureKey]bool),
			) {
				t.Fatalf("%s result %d ignored a non-returning path", name, index)
			}
		}
	}
	for _, name := range []string{
		"directStop", "nestedStop", "nestedHelper", "deferredStop", "nestedDeferred",
		"exitStop", "goexitStop", "externalBodylessStop", "dynamicStop", "synchronousSend",
		"synchronousReceive", "nilSynchronousReceive", "blockingSelectNoDefault", "closeNil",
		"closeUnknown", "dynamicMake", "deleteUncomparableKey",
		"implicitDeleteUncomparableKey", "nilMapWrite",
		"unknownMapWrite", "unhashableMapLiteral", "unknownMapLiteral",
		"implicitUnhashableMapLiteral", "implicitUnhashableMapIndex",
		"failedTypeAssertion", "invalidSliceToArray", "invalidSliceBounds",
		"nilPointerDereference", "nilPointerField", "nilPointerValueMethod", "nilSliceIndex",
		"uncomparableInterfaceComparison", "unknownInterfaceComparison",
		"unknownUncomparableInterfaceComparison", "signedShift",
		"nilInterfaceMethod", "nilChannel",
		"nilIterator", "selfStop", "mutualStopA",
		"conditionalRecursionSupport", "localCallableStop", "packageCallableStop",
		"pointerReceiverCallableStop", "exportedPackageCallableStop", "convertedCallableStop",
		"ambiguousCallableStop", "recoveredUnnamedDirectAssertion", "unrecoveredDeferred",
		"conditionalRecoveredDeferred", "liveSelectStop", "eagerSelectOperand", "liveTypeSwitchStop",
		"selectedStoppingDefault", "capturedCallableStop", "packageCapturedStopSupport",
		"unrecoveredExitDefer", "unrecoveredLoopDefer",
		"unrecoveredHardPanicker", "unrecoveredExitingPanicker",
		"unrecoveredBlockedArgumentBeforeTypeAssertion", "unrecoveredConditionalTypeAssertionPanic",
		"immediateDeferredArgumentPanic", "immediateDeferredCalleePanic",
		"dynamicCompoundDivide", "signedCompoundShift", "promotedNilField",
		"promotedNilValueMethod", "promotedNilOuterPointerMethod",
		"unknownPromotedFieldWrite", "unknownPromotedFieldIncrement",
		"unknownPointerArrayIndex", "unknownPointerArraySlice",
		"unrecoveredChannelBeforePanic", "unrecoveredBlockedBeforePanic",
		"unrecoveredBlockedGoArgumentBeforePanic",
	} {
		for index, nilSupports := range []bool{false, true} {
			if ps6080CallResultAlwaysSupports(
				pass, forwarded[name], index, nilSupports, cache,
				make(map[ps6080ReturnFailureKey]bool),
			) {
				t.Fatalf("%s result %d ignored a guaranteed nonreturn path", name, index)
			}
		}
	}
	if ps6080CallResultAlwaysSupports(
		pass, forwarded["shadowedNilError"], 1, true, cache,
		make(map[ps6080ReturnFailureKey]bool),
	) {
		t.Fatal("shadowed nil error was treated as the predeclared nil value")
	}
	for _, name := range []string{"selfStop", "mutualStopA"} {
		if ps6080CallMayReturnWithFacts(
			pass, forwarded[name].(*ast.CallExpr), nil, cache,
			cache.returnability.mayReturn, make(map[*ast.BlockStmt]bool),
		) {
			t.Fatalf("%s recursion was not classified as guaranteed nonreturn", name)
		}
	}
	if !ps6080CallMayReturnWithFacts(
		pass, forwarded["recursiveBase"].(*ast.CallExpr), nil, cache,
		cache.returnability.mayReturn, make(map[*ast.BlockStmt]bool),
	) {
		t.Fatal("recursive function with a base return was classified as guaranteed nonreturn")
	}
	if cache.returnability.builds != 1 {
		t.Fatalf("returnability index built %d times, want 1", cache.returnability.builds)
	}
	if cache.returnability.packageBuilds != 1 {
		t.Fatalf("package callable index built %d times, want 1", cache.returnability.packageBuilds)
	}
	nestedHelperStop := pkg.Scope().Lookup("nestedHelperStop").(*types.Func)
	nestedHelperStopDeclaration := ps6080FunctionDeclaration(pass, cache, nestedHelperStop)
	contextValue, found := cache.returnability.contexts.Load(nestedHelperStopDeclaration.Body)
	if !found || contextValue.(*ps6080ReturnabilityBodyContext).builds != 1 {
		t.Fatal("returnability body context was not built exactly once")
	}
	localCallableStop := pkg.Scope().Lookup("localCallableStop").(*types.Func)
	localCallableDeclaration := ps6080FunctionDeclaration(pass, cache, localCallableStop)
	localContextValue, found := cache.returnability.contexts.Load(localCallableDeclaration.Body)
	if !found || localContextValue.(*ps6080ReturnabilityBodyContext).callableBuilds.Load() != 1 {
		t.Fatal("local callable initializer index was not built exactly once")
	}
	var packageForwarderLiteral *ast.FuncLit
	ast.Inspect(file, func(node ast.Node) bool {
		values, valueSpec := node.(*ast.ValueSpec)
		if !valueSpec || len(values.Names) != len(values.Values) {
			return true
		}
		for index, name := range values.Names {
			if name.Name == "packageForwarder" {
				packageForwarderLiteral, _ = ps2110Unparen(values.Values[index]).(*ast.FuncLit)
				return false
			}
		}
		return true
	})
	if packageForwarderLiteral == nil {
		t.Fatal("shared package literal not found")
	}
	sharedContextValue, found := cache.returnability.contexts.Load(packageForwarderLiteral.Body)
	if !found {
		t.Fatal("shared package literal returnability context was not built")
	}
	sharedContext := sharedContextValue.(*ps6080ReturnabilityBodyContext)
	if sharedContext.mayEvaluations.Load() > 2 || sharedContext.mustEvaluations.Load() > 2 {
		t.Fatalf(
			"shared literal returnability evaluated may=%d must=%d, want at most 2 each",
			sharedContext.mayEvaluations.Load(), sharedContext.mustEvaluations.Load(),
		)
	}
	for _, name := range []string{
		"nilSlice", "nilMap", "unevaluatedShort", "unevaluatedSize", "unevaluatedAlign",
		"unevaluatedOffset", "recoveredDeferred", "recoveredNamedDeferred", "recoveredCloseNil",
		"recoveredDirectAssertion", "packageCallableReturn",
		"returningSelectDefault", "failedTypeAssertionCommaOK", "validSliceToArray",
		"validSliceBounds", "fixedMake", "deleteComparableKey", "nilMapLookup",
		"madeMapWrite", "comparableMapLiteral", "comparableInterfaceComparison",
		"implicitComparableMapKeys",
		"unknownNilInterfaceComparison", "unknownComparableInterfaceComparison",
		"unsignedShift", "fixedShift", "safeDeferredArgument", "safeDeferredCallee",
		"fixedCompoundDivide", "unsignedCompoundShift", "promotedNilPointerMethod",
		"promotedNonNilOuterPointerMethod", "promotedNonNilField",
		"directPromotedFieldWrite",
		"directPointerArrayIndex", "directPointerArraySlice",
		"differentInterfaceComparison", "unevaluatedFailedTypeAssertion",
		"nilPointerReceiverMethod", "boxedNilPointerReceiver", "impossibleSelectSupport",
		"impossibleTypeSwitchSupport",
		"shadowedNilTypeSwitchSupport", "selectedTypeWithStoppingDefault", "capturedCallableReturn",
		"recoveredNamedPanicker",
		"packageCapturedReturnSupport", "recoveredLiteralPanicker",
		"recoveredReturningArgumentBeforeTypeAssertion", "recoveredGuaranteedTypeAssertionPanic",
		"alternatingSupport", "wideChainCaller",
	} {
		for index, nilSupports := range []bool{false, true} {
			if !ps6080CallResultAlwaysSupports(
				pass, forwarded[name], index, nilSupports, cache,
				make(map[ps6080ReturnFailureKey]bool),
			) {
				t.Fatalf("%s result %d did not retain completing empty-range support", name, index)
			}
		}
	}
	if ps6080CallResultAlwaysSupports(
		pass, forwarded["recoveredPartialSupportToFailure"], 0, false, cache,
		make(map[ps6080ReturnFailureKey]bool),
	) {
		t.Fatal("partially committed support-to-failure assignment retained stale support")
	}
	if ps6080CallResultAlwaysFailure(
		pass, forwarded["recoveredPartialFailureToSupport"], 0, false, cache,
		make(map[ps6080ReturnFailureKey]bool),
	) {
		t.Fatal("partially committed failure-to-support assignment retained stale failure")
	}
	for index := range 8 {
		name := fmt.Sprintf("chain%d", index)
		functionObject := pkg.Scope().Lookup(name).(*types.Func)
		declaration := ps6080FunctionDeclaration(pass, cache, functionObject)
		contextValue, found := cache.returnability.contexts.Load(declaration.Body)
		if !found {
			t.Fatalf("%s returnability context was not built", name)
		}
		context := contextValue.(*ps6080ReturnabilityBodyContext)
		mayEvaluations := context.mayEvaluations.Load()
		mustEvaluations := context.mustEvaluations.Load()
		if mayEvaluations > 2 || mustEvaluations > 2 {
			t.Fatalf(
				"%s returnability evaluated may=%d must=%d, want at most 2 each",
				name, mayEvaluations, mustEvaluations,
			)
		}
	}
	for name, expected := range map[string]struct {
		mustReturn bool
		mustPanic  bool
	}{
		"alternatingRecoveredReturn":  {mustReturn: true},
		"alternatingPanicAfterReturn": {mustPanic: true},
		"alternatingSupport":          {mustReturn: true},
	} {
		functionObject := pkg.Scope().Lookup(name).(*types.Func)
		declaration := ps6080FunctionDeclaration(pass, cache, functionObject)
		if cache.returnability.mustReturn[declaration.Body] != expected.mustReturn ||
			cache.returnability.mustPanic[declaration.Body] != expected.mustPanic {
			t.Fatalf(
				"%s joint fixed point got return=%v panic=%v, want return=%v panic=%v",
				name, cache.returnability.mustReturn[declaration.Body],
				cache.returnability.mustPanic[declaration.Body], expected.mustReturn, expected.mustPanic,
			)
		}
		contextValue, found := cache.returnability.contexts.Load(declaration.Body)
		if !found {
			t.Fatalf("%s returnability context was not built", name)
		}
		context := contextValue.(*ps6080ReturnabilityBodyContext)
		if context.mustEvaluations.Load() > 2 || context.panicEvaluations.Load() > 2 {
			t.Fatalf(
				"%s joint fixed point evaluated panic=%d must=%d, want at most 2 each",
				name, context.panicEvaluations.Load(), context.mustEvaluations.Load(),
			)
		}
	}
	wideChainCaller := pkg.Scope().Lookup("wideChainCaller").(*types.Func)
	wideDeclaration := ps6080FunctionDeclaration(pass, cache, wideChainCaller)
	wideContextValue, found := cache.returnability.contexts.Load(wideDeclaration.Body)
	if !found {
		t.Fatal("wide fan-in returnability context was not built")
	}
	wideContext := wideContextValue.(*ps6080ReturnabilityBodyContext)
	if wideContext.mayEvaluations.Load() != 1 || wideContext.mustEvaluations.Load() != 1 ||
		wideContext.panicEvaluations.Load() != 1 {
		t.Fatalf(
			"wide fan-in evaluated panic=%d may=%d must=%d, want exactly 1 each",
			wideContext.panicEvaluations.Load(), wideContext.mayEvaluations.Load(),
			wideContext.mustEvaluations.Load(),
		)
	}
	functionReturn := func(name string) (*ps6080Function, *ast.ReturnStmt) {
		for _, declaration := range file.Decls {
			functionDeclaration, ok := declaration.(*ast.FuncDecl)
			if !ok || functionDeclaration.Name.Name != name {
				continue
			}
			object, _ := info.Defs[functionDeclaration.Name].(*types.Func)
			signature, _ := object.Type().(*types.Signature)
			var returned *ast.ReturnStmt
			ast.Inspect(functionDeclaration.Body, func(node ast.Node) bool {
				if candidate, ok := node.(*ast.ReturnStmt); ok {
					returned = candidate
				}
				return true
			})
			return &ps6080Function{
				declaration: functionDeclaration, object: object,
				signature: signature, body: functionDeclaration.Body,
			}, returned
		}
		return nil, nil
	}
	knownFunction, knownReturn := functionReturn("known")
	if !ps6080StatementAlwaysSupports(pass, knownFunction, knownReturn) {
		t.Fatal("known nil named error result was not treated as support")
	}
	unknownFunction, unknownReturn := functionReturn("unknown")
	if ps6080StatementAlwaysSupports(pass, unknownFunction, unknownReturn) {
		t.Fatal("merged unknown named error result was treated as known nil support")
	}
	namedDeclaration := ps6080FunctionDeclaration(
		pass, cache, pkg.Scope().Lookup("namedHelper").(*types.Func),
	)
	namedIndexValue, found := cache.namedStates.Load(namedDeclaration.Body)
	if !found || namedIndexValue.(*ps6080NamedReturnStateIndex).builds != 1 {
		t.Fatal("named return dataflow was not built exactly once")
	}
}

func TestPS6080GlobalAnalysisCachesAreReused(t *testing.T) {
	t.Parallel()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "cache.go", `package cache
func kernel() {}
var target = kernel
var table = map[int]func(){0: kernel}
var alias = table
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	pkg, err := (&types.Config{}).Check("cache", fileSet, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	ps6080FunctionValueCaches.Store(pass, &ps6080FunctionValueTargetCache{})
	defer ps6080FunctionValueCaches.Delete(pass)
	ps6080GlobalAliasCaches.Store(pass, &sync.Map{})
	defer ps6080GlobalAliasCaches.Delete(pass)

	target := pkg.Scope().Lookup("target")
	kernel, _ := pkg.Scope().Lookup("kernel").(*types.Func)
	if !ps6080FunctionValueTargets(pass)[target][kernel] {
		t.Fatal("function-value target was not indexed")
	}
	table := pkg.Scope().Lookup("table")
	alias := pkg.Scope().Lookup("alias")
	aliasInfo := ps6080GlobalAliasInfoFor(pass, table)
	if !aliasInfo.aliases[alias] {
		t.Fatal("package map alias was not indexed")
	}
	sentinel := types.NewVar(token.NoPos, pkg, "sentinel", table.Type())
	context := ps6080NewMapAliasContext(pass, table, aliasInfo.aliases, aliasInfo.initialAliases, file)
	context.aliases[sentinel] = true
	if ps6080GlobalAliasInfoFor(pass, table).aliases[sentinel] {
		t.Fatal("map-alias context mutated the shared cache")
	}

	pass.Files = nil
	if !ps6080FunctionValueTargets(pass)[target][kernel] {
		t.Fatal("function-value target cache was not reused")
	}
	if !ps6080GlobalAliasInfoFor(pass, table).aliases[alias] {
		t.Fatal("global alias cache was not reused")
	}
}

func TestPS6080InitMapWritesScale(t *testing.T) {
	t.Parallel()
	const writeCount = 256
	var source strings.Builder
	source.Grow(writeCount * 32)
	source.WriteString(`package initmaps
const disabled = false
var table = make(map[int]func(), 256)
func init() {
	if disabled { return }
`)
	for index := range writeCount {
		fmt.Fprintf(&source, "\ttable[%d] = func() {}\n", index)
	}
	source.WriteString("}\n")
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "initmaps.go", source.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("initmaps", fileSet, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	table, ok := pkg.Scope().Lookup("table").(*types.Var)
	if !ok {
		t.Fatal("table variable was not type checked")
	}
	if got := len(ps6080InitMapWrites(pass)[table]); got != writeCount {
		t.Fatalf("indexed init writes = %d, want %d", got, writeCount)
	}
}

func TestPS6080InitMapWritesSkipsUnrelatedInitFlow(t *testing.T) {
	t.Parallel()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "unrelated.go", `package unrelated
var enabled bool
func init() {
	for index := 0; index < 256; index++ {
		if enabled { break }
	}
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	pkg, err := (&types.Config{}).Check("unrelated", fileSet, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	cache := &ps6080ReturnFailureCache{}
	ps6080ReturnFailureCaches.Store(pass, cache)
	defer ps6080ReturnFailureCaches.Delete(pass)
	if writes := ps6080InitMapWrites(pass); len(writes) != 0 {
		t.Fatalf("unrelated init indexed %d maps", len(writes))
	}
	if cache.returnability.builds != 0 {
		t.Fatalf("unrelated init built returnability facts %d times, want 0", cache.returnability.builds)
	}
}

func TestPS6080SiteEvidencePosition(t *testing.T) {
	t.Parallel()
	domain := types.NewConst(token.Pos(7), nil, "DomainB", types.Typ[types.Int], constant.MakeInt64(1))
	alias := types.NewConst(token.Pos(9), nil, "AliasB", types.Typ[types.Int], constant.MakeInt64(1))
	site := &ps6080Site{
		position: token.Pos(11),
		constants: map[*types.Const]token.Pos{
			alias: token.Pos(17),
		},
	}
	if got := ps6080SiteEvidencePosition(site, domain); got != token.Pos(17) {
		t.Fatalf("explicit alias evidence position = %d, want 17", got)
	}
	clear(site.constants)
	if got := ps6080SiteEvidencePosition(site, domain); got != site.position {
		t.Fatalf("materialized open evidence position = %d, want site position %d", got, site.position)
	}
}

func TestPS6080(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6080.Analyzer, "ps6080", "ps6080_bindings")
}

func TestPS6080Roles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		context string
		role    ps6080Role
	}{
		{name: "storage", context: "quantBlockByteSize", role: ps6080StorageRole},
		{name: "quant size storage", context: "quantSize", role: ps6080StorageRole},
		{name: "quant type size storage", context: "quant_type_size", role: ps6080StorageRole},
		{name: "acronym type size storage", context: "QTypeSize", role: ps6080StorageRole},
		{name: "prototype size is not storage", context: "prototypeSize"},
		{name: "archetype bytes is not storage", context: "archetypeBytes"},
		{name: "storage quota lacks quant context", context: "storageQuota"},
		{name: "decode", context: "portableDequantize", role: ps6080DecodeRole},
		{name: "matmul", context: "QMatMul", role: ps6080MatmulRole},
		{name: "matrix multiply", context: "matrix_multiply", role: ps6080MatmulRole},
		{name: "mat vec", context: "mat_vec", role: ps6080MatmulRole},
		{name: "quant mat mul", context: "q_mat_mul", role: ps6080MatmulRole},
		{name: "mulmat", context: "MulMat", role: ps6080MatmulRole},
		{name: "quant mulmat", context: "QMulMat", role: ps6080MatmulRole},
		{name: "underscore mulmat", context: "mul_mat", role: ps6080MatmulRole},
		{name: "underscore quant mulmat", context: "q_mul_mat", role: ps6080MatmulRole},
		{name: "numbered mulmat", context: "MulMat4", role: ps6080MatmulRole},
		{name: "gemm", context: "Gemm", role: ps6080MatmulRole},
		{name: "numbered gemm", context: "Gemm2D", role: ps6080MatmulRole},
		{name: "lowercase gemm", context: "gemm", role: ps6080MatmulRole},
		{name: "gemma model is not gemm", context: "GemmaModel"},
		{name: "mul material is not mulmat", context: "MulMaterial"},
		{name: "format multiplier is not matmul", context: "FormatMultiplierOne"},
		{name: "quant format multiplier is not matmul", context: "QuantFormatMultiplier"},
		{name: "combined", context: "qmatmul dequant", role: ps6080MatmulRole | ps6080DecodeRole},
		{name: "resize is not storage", context: "resizeQuantBuffer"},
		{name: "directive is not a role", context: "helper\n//perfscan:quant-matmul-coverage-validated QMatMul reason"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ps6080Roles(test.context); got != test.role {
				t.Errorf("ps6080Roles(%q) = %d, want %d", test.context, got, test.role)
			}
		})
	}
}

func TestPS6080BackendContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		backend bool
	}{
		{name: "cudaQMatMul", backend: true},
		{name: "CUDAQMatMul", backend: true},
		{name: "metalMatVec", backend: true},
		{name: "MetalMatVec", backend: true},
		{name: "GPUQMatMul", backend: true},
		{name: "MPSQMatMul", backend: true},
		{name: "OpenCLQMatMul", backend: true},
		{name: "openCLQMatMul", backend: true},
		{name: "ROCmQMatMul", backend: true},
		{name: "QMatMul"},
		{name: "qMatMulCPUFallback"},
		{name: "clampsQMatMul"},
		{name: "debugpuQMatMul"},
		{name: "openclampQMatMul"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ps6080BackendContext(test.name); got != test.backend {
				t.Errorf("ps6080BackendContext(%q) = %t, want %t", test.name, got, test.backend)
			}
		})
	}
}
