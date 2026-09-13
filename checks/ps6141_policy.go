package checks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/allocationcampaign"
	"github.com/jxsl13/perfscan/internal/scanscope"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"
)

// This is pinned operator-reviewed policy, NOT independently reproduced
// benchmark evidence. CPU-specific or unknown target applicability cannot pass.
type ps6141Policy struct {
	Schema          int               `json:"schema"`
	Owner           string            `json:"owner"`
	Quantizer       string            `json:"quantizer"`
	Consumer        string            `json:"consumer"`
	ConsumerForm    string            `json:"consumerForm"`
	File            string            `json:"file"`
	QuantizerOffset int               `json:"quantizerOffset"`
	ConsumerOffset  int               `json:"consumerOffset"`
	Elements        int64             `json:"elements"`
	SourceSHA256    map[string]string `json:"sourceSHA256"`
	GoVersion       string            `json:"goVersion"`
	GOOS            string            `json:"goos"`
	GOARCH          string            `json:"goarch"`
	ContextSHA256   string            `json:"contextSHA256"`
	TargetPolicy    string            `json:"targetPolicy"`
	Boundary        string            `json:"boundary"`
	Review          string            `json:"review"`
}

func ps6141PolicyRead(reference config.SingleUseQuantizationExemption) (ps6141Policy, bool) {
	var policy ps6141Policy
	info, err := os.Lstat(reference.Policy)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return policy, false
	}
	file, err := os.Open(reference.Policy)
	if err != nil {
		return policy, false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return policy, false
	}
	data, err := io.ReadAll(io.LimitReader(file, (256<<10)+1))
	if err != nil || len(data) > 256<<10 {
		return policy, false
	}
	sum := sha256.Sum256(data)
	// Decode requires every canonical field. An extra case-fold alias must
	// not exploit encoding/json's case-insensitive struct-field matching.
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != reflect.TypeOf(policy).NumField() {
		return policy, false
	}
	if hex.EncodeToString(sum[:]) != reference.SHA256 || allocationcampaign.Decode(data, &policy) != nil {
		return policy, false
	}
	return policy, true
}

func ps6141Exempt(pass *analysis.Pass, owner *ast.FuncDecl, candidate ps6141Candidate, c *config.SingleUseQuantizationContract, guard ast.Stmt) bool {
	target, known := scanscope.Get(pass)
	if !known || c.ConsumerForm != "twoInputDot" && c.ConsumerForm != "sourceSummary" && c.ConsumerForm != "packedByteDot" {
		return false
	}
	object, _ := pass.TypesInfo.Defs[owner.Name].(*types.Func)
	if object == nil {
		return false
	}
	for _, reference := range c.BenchmarkExemptions {
		policy, ok := ps6141PolicyRead(reference)
		if !ok || policy.Schema != 1 || policy.Elements <= 0 || policy.Owner != ps6090FunctionID(object) || policy.Quantizer != c.Quantizer || policy.Consumer != c.Consumer || policy.ConsumerForm != c.ConsumerForm ||
			policy.TargetPolicy != "architecture-wide-operator-reviewed" || policy.Boundary != "complete-conversion-and-consumer" || strings.TrimSpace(policy.Review) == "" || len(policy.Review) > 4096 ||
			policy.GoVersion != target.GoVersion || policy.GOOS != target.GOOS || policy.GOARCH != target.GOARCH || policy.ContextSHA256 != target.ContextSHA256 {
			continue
		}
		quantFile := pass.Fset.File(candidate.quantizer.Pos())
		consumeFile := pass.Fset.File(candidate.consumer.Pos())
		if quantFile == nil || consumeFile != quantFile || policy.File != filepath.Base(quantFile.Name()) || policy.QuantizerOffset != quantFile.Offset(candidate.quantizer.Pos()) || policy.ConsumerOffset != quantFile.Offset(candidate.consumer.Pos()) || !ps6141SourcePartition(pass, policy.SourceSHA256) {
			continue
		}
		if c.ProducerForm == "destination" {
			guard = ps6141DestinationPolicyGuard(owner.Body, candidate.quantizer)
		}
		if ps6141ShapeGuard(pass, owner, candidate.quantizer, guard, policy.Elements, c) {
			return true
		}
	}
	return false
}

func ps6141SourcePartition(pass *analysis.Pass, hashes map[string]string) bool {
	if len(pass.Files) != len(hashes) || len(hashes) == 0 {
		return false
	}
	seen := make(map[string]bool, len(pass.Files))
	for _, file := range pass.Files {
		name := pass.Fset.PositionFor(file.Pos(), false).Filename
		base := filepath.Base(name)
		if seen[base] {
			return false
		}
		seen[base] = true
		data, err := ps6053ReadFile(pass, name)
		if err != nil {
			return false
		}
		// A fresh source hash alone cannot attest that an already-loaded AST
		// still describes those bytes. Require the full reparsed syntax too.
		freshSet := token.NewFileSet()
		fresh, err := parser.ParseFile(freshSet, name, data, parser.ParseComments)
		if err != nil {
			return false
		}
		var current, loaded bytes.Buffer
		if format.Node(&current, freshSet, fresh) != nil || format.Node(&loaded, pass.Fset, file) != nil || !bytes.Equal(current.Bytes(), loaded.Bytes()) {
			return false
		}
		if !ps6141SourcePositionsMatch(pass.Fset, file, freshSet, fresh) {
			return false
		}
		sum := sha256.Sum256(data)
		if hashes[base] != hex.EncodeToString(sum[:]) {
			return false
		}
	}
	return true
}

// Canonical formatting alone discards trivia and original physical offsets.
// Match each corresponding syntax node's physical byte span too, including the
// actual producer/consumer calls whose loaded offsets select the policy site.
func ps6141SourcePositionsMatch(loadedSet *token.FileSet, loaded *ast.File, freshSet *token.FileSet, fresh *ast.File) bool {
	loadedFile, freshFile := loadedSet.File(loaded.Pos()), freshSet.File(fresh.Pos())
	if loadedFile == nil || freshFile == nil || loadedFile.Size() != freshFile.Size() {
		return false
	}
	type span struct {
		kind       reflect.Type
		start, end int
	}
	positions := func(file *token.File, tree *ast.File) []span {
		var spans []span
		ast.Inspect(tree, func(node ast.Node) bool {
			if node != nil {
				spans = append(spans, span{reflect.TypeOf(node), file.Offset(node.Pos()), file.Offset(node.End())})
			}
			return true
		})
		return spans
	}
	return slices.Equal(positions(loadedFile, loaded), positions(freshFile, fresh))
}

func ps6141ShapeGuard(pass *analysis.Pass, owner *ast.FuncDecl, quant *ast.CallExpr, statement ast.Stmt, elements int64, c *config.SingleUseQuantizationContract) bool {
	guard, ok := statement.(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	condition, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ || !ps6141ConstantInt(pass, condition.Y, elements) {
		return false
	}
	if c.FloatInputArgument >= len(quant.Args) {
		return false
	}
	input, ok := quant.Args[c.FloatInputArgument].(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.Uses[input]
	fn, _ := pass.TypesInfo.Defs[owner.Name].(*types.Func)
	if fn == nil {
		return false
	}
	formal := false
	sig := fn.Type().(*types.Signature)
	for i := 0; i < sig.Params().Len(); i++ {
		formal = formal || sig.Params().At(i) == object
	}
	uses := 2
	if c.ProducerForm == "destination" {
		uses = 3
	}
	if !formal || !ps6141FormalLen(pass, condition.X, object) || ps6141ObjectUses(pass, owner.Body, object) != uses || !ps6141GuardDominates(pass, owner.Body, condition, quant) {
		return false
	}
	// The two uses must be this guard and quantizer. With immediate adjacency,
	// no assignment/address-taking/alias/unknown call can intervene or rebind it.
	expr, ok := guard.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	panicID, ok := call.Fun.(*ast.Ident)
	return ok && pass.TypesInfo.Uses[panicID] == types.Universe.Lookup("panic") && pass.TypesInfo.Types[call.Args[0]].Value != nil
}

// Destination admission already proves the adjacent owner-fresh make/fill
// pair and matching input geometry. Bind policy to its pre-allocation guard,
// never a distant name-only guard or a different fill site.
func ps6141DestinationPolicyGuard(body *ast.BlockStmt, quant *ast.CallExpr) ast.Stmt {
	var guard ast.Stmt
	ast.Inspect(body, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, stmt := range block.List {
			expr, ok := stmt.(*ast.ExprStmt)
			if ok && expr.X == quant && i >= 2 {
				if _, ok := block.List[i-1].(*ast.AssignStmt); ok {
					guard = block.List[i-2]
				}
			}
		}
		return true
	})
	return guard
}

func ps6141GuardDominates(pass *analysis.Pass, body *ast.BlockStmt, condition ast.Expr, quant *ast.CallExpr) bool {
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6079PanicCall(pass, call) })
	seen := make([]bool, len(graph.Blocks))
	queue := []*cfg.Block{graph.Blocks[0]}
	for len(queue) > 0 {
		block := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[block.Index] {
			continue
		}
		seen[block.Index] = true
		blocked := false
		for _, node := range block.Nodes {
			if node == condition {
				blocked = true
				break
			}
			found := false
			ast.Inspect(node, func(n ast.Node) bool { found = found || n == quant; return !found })
			if found {
				return false
			}
		}
		if !blocked {
			queue = append(queue, block.Succs...)
		}
	}
	return true
}
