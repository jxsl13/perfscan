package traceevidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func readRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("artifact %s is not a bounded regular file", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(errors.New("artifact changed while opening"), statErr, f.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	closeErr := f.Close()
	if int64(len(data)) > limit {
		return nil, errors.New("artifact grew beyond its byte limit")
	}
	return data, errors.Join(readErr, closeErr)
}

func bundleDigest(root string, limit int64) (string, error) {
	return bundleDigestContext(context.Background(), root, limit)
}

func bundleDigestContext(ctx context.Context, root string, limit int64) (string, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return "", errors.New("missing regular trace bundle directory")
	}
	hash := sha256.New()
	var total int64
	files, entries := 0, 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > 100000 {
			return errors.New("trace bundle exceeds entry limit")
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("trace bundle contains a symlink or special file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || !filepath.IsLocal(relative) {
			return errors.New("trace bundle member escapes its root")
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(relative))
		_, _ = hash.Write([]byte{0})
		if info.IsDir() {
			_, _ = io.WriteString(hash, "directory\x00")
			return nil
		}
		if info.Size() > limit-total {
			return errors.New("trace bundle exceeds byte limit")
		}
		digest, size, err := digestRegular(ctx, path, limit-total)
		if err != nil {
			return err
		}
		total += size
		files++
		_, _ = hash.Write(digest)
		_, _ = hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	if files == 0 || total == 0 {
		return "", errors.New("trace bundle has no nonempty regular data")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func digestRegular(ctx context.Context, path string, limit int64) ([]byte, int64, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, 0, errors.New("trace member is missing, non-regular or over its byte limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, 0, errors.Join(errors.New("trace member changed while opening"), err, f.Close())
	}
	hash := sha256.New()
	size, readErr := io.Copy(hash, io.LimitReader(&contextReader{ctx, f}, limit+1))
	after, statErr := f.Stat()
	closeErr := f.Close()
	if readErr != nil || statErr != nil || closeErr != nil {
		return nil, 0, errors.Join(readErr, statErr, closeErr)
	}
	if size > limit || size != info.Size() || after.Size() != info.Size() || after.ModTime() != info.ModTime() {
		return nil, 0, errors.New("trace member changed or exceeded byte limit while hashing")
	}
	return hash.Sum(nil), size, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

func timeoutMarkers(output, trace string) bool {
	limit, completed, saved := -1, -1, -1
	nativeLimit, nativeCompleted, basename := false, false, false
	for i, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "Reached specified time limit", "Reached specified time limit, ending recording...":
			if limit >= 0 {
				return false
			}
			limit = i
			nativeLimit = line == "Reached specified time limit, ending recording..."
		case "Recording completed", "Recording completed. Saving output file...":
			if completed >= 0 {
				return false
			}
			completed = i
			nativeCompleted = line == "Recording completed. Saving output file..."
		default:
			if name, ok := strings.CutPrefix(line, "Output file saved as: "); ok {
				if saved >= 0 || name != trace && name != "capture.trace" {
					return false
				}
				saved, basename = i, name != trace
			}
		}
	}
	// Observed xctrace 16.0 (17F113) emits only this basename, despite the
	// fixed absolute --output argument. The collector independently requires
	// that fresh exact bundle and exports it; the text is not a path authority.
	return limit >= 0 && completed > limit && saved > completed && (!basename || nativeLimit && nativeCompleted && filepath.Base(trace) == "capture.trace")
}

func workloadMarkers(data []byte, started, completed string) bool {
	start, finish := -1, -1
	for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		switch line {
		case started:
			if start != -1 {
				return false
			}
			start = i
		case completed:
			if finish != -1 {
				return false
			}
			finish = i
		}
	}
	return start >= 0 && finish > start
}

func workloadInputMarkers(data []byte, started, opened, completed string) bool {
	return workloadMarkers(data, started, opened) && workloadMarkers(data, opened, completed)
}

type xmlNode struct {
	name     string
	attrs    map[string]string
	text     strings.Builder
	children []*xmlNode
}

// Decode the entire bounded XML document. Namespaces, duplicate attributes,
// DTDs, extra roots, trailing data, excessive depth and malformed EOF fail.
func parseXML(data []byte) (*xmlNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root *xmlNode
	var stack []*xmlNode
	count, prologs := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if root == nil || len(stack) != 0 {
				return nil, errors.New("incomplete XML document")
			}
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			count++
			if value.Name.Space != "" || len(stack) >= 128 || count > 1000000 {
				return nil, errors.New("unsupported XML namespace/depth/element count")
			}
			node := &xmlNode{name: value.Name.Local, attrs: make(map[string]string, len(value.Attr))}
			for _, attr := range value.Attr {
				if attr.Name.Space != "" || attr.Name.Local == "xmlns" {
					return nil, errors.New("namespaced XML attributes unsupported")
				}
				if _, duplicate := node.attrs[attr.Name.Local]; duplicate {
					return nil, errors.New("duplicated XML attribute")
				}
				node.attrs[attr.Name.Local] = attr.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("multiple XML roots")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("unexpected XML closing element")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if len(bytes.TrimSpace(value)) != 0 {
					return nil, errors.New("XML trailing or leading non-whitespace")
				}
			} else {
				stack[len(stack)-1].text.Write(value)
			}
		case xml.Directive:
			return nil, errors.New("XML directives are not capture evidence")
		case xml.ProcInst:
			prologs++
			if value.Target != "xml" || root != nil || prologs != 1 {
				return nil, errors.New("unexpected XML processing instruction")
			}
		}
	}
}

func children(node *xmlNode, name string) []*xmlNode {
	var result []*xmlNode
	for _, child := range node.children {
		if child.name == name {
			result = append(result, child)
		}
	}
	return result
}

func tablePaths(data []byte, schemas []Schema) (map[string]string, error) {
	root, err := parseXML(data)
	if err != nil {
		return nil, fmt.Errorf("invalid TOC XML: %w", err)
	}
	if root.name != "trace-toc" {
		return nil, errors.New("unexpected TOC root")
	}
	runs := children(root, "run")
	if len(runs) != 1 || runs[0].attrs["number"] != "1" {
		return nil, errors.New("fresh trace must contain exactly run 1")
	}
	sets := children(runs[0], "data")
	if len(sets) != 1 {
		return nil, errors.New("missing or ambiguous run-1 TOC data")
	}
	paths := make(map[string]string, len(schemas))
	for _, required := range schemas {
		count := 0
		for i, table := range children(sets[0], "table") {
			if table.attrs["schema"] == required.Name {
				count++
				paths[required.Name] = "//trace-toc[1]/run[1]/data[1]/table[" + strconv.Itoa(i+1) + "]"
			}
		}
		if count != 1 {
			return nil, fmt.Errorf("missing or ambiguous required schema %s in run 1", required.Name)
		}
	}
	return paths, nil
}

func validateTable(data []byte, xpath string, required Schema, tocPaths ...string) (int64, error) {
	root, err := parseXML(data)
	if err != nil {
		return 0, fmt.Errorf("invalid required-table XML: %w", err)
	}
	nodes := children(root, "node")
	if root.name != "trace-query-result" || len(nodes) != 1 {
		return 0, errors.New("table export does not identify the exact selected run/schema")
	}
	// Native xctrace exports identify the selected node with a resolved
	// positional XPath, NOT necessarily the selector supplied to --xpath.
	// Accept that spelling only when it was derived from this observed TOC.
	selected := nodes[0].attrs["xpath"]
	matched := selected == xpath
	for _, path := range tocPaths {
		matched = matched || path != "" && (selected == path || selected == strings.TrimPrefix(path, "/"))
	}
	if !matched {
		return 0, errors.New("table export path differs from observed TOC selection")
	}
	schemas := children(nodes[0], "schema")
	if len(schemas) != 1 || schemas[0].attrs["name"] != required.Name {
		return 0, errors.New("exported table schema identity differs")
	}
	columns := make([]string, 0, len(schemas[0].children))
	for _, col := range children(schemas[0], "col") {
		mnemonics := children(col, "mnemonic")
		if len(mnemonics) != 1 {
			return 0, errors.New("table column has no unique mnemonic")
		}
		name := strings.TrimSpace(mnemonics[0].text.String())
		if !schemaName(name) || slices.Contains(columns, name) {
			return 0, errors.New("invalid or duplicated table column")
		}
		columns = append(columns, name)
	}
	for _, name := range required.Columns {
		if !slices.Contains(columns, name) {
			return 0, fmt.Errorf("required table column %s absent", name)
		}
	}
	rows := children(nodes[0], "row")
	if len(rows) == 0 {
		return 0, errors.New("required table has no rows")
	}
	ids := make(map[string]*xmlNode)
	var references []*xmlNode
	var inspect func(*xmlNode) error
	inspect = func(node *xmlNode) error {
		if id, exists := node.attrs["id"]; exists {
			if _, err := strconv.ParseUint(id, 10, 64); err != nil || ids[id] != nil {
				return errors.New("invalid or duplicated table value identity")
			}
			ids[id] = node
		}
		if ref, exists := node.attrs["ref"]; exists {
			if _, err := strconv.ParseUint(ref, 10, 64); err != nil || node.attrs["id"] != "" || len(node.children) != 0 || strings.TrimSpace(node.text.String()) != "" {
				return errors.New("invalid table reference")
			}
			references = append(references, node)
		}
		for _, child := range node.children {
			if err := inspect(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, row := range rows {
		if len(row.children) != len(columns) {
			return 0, errors.New("truncated or mismatched table row")
		}
		if err := inspect(row); err != nil {
			return 0, err
		}
	}
	for _, ref := range references {
		definition := ids[ref.attrs["ref"]]
		if definition == nil || definition.name != ref.name {
			return 0, errors.New("unresolved table value reference")
		}
	}
	var populated func(*xmlNode, map[*xmlNode]bool) bool
	populated = func(node *xmlNode, active map[*xmlNode]bool) bool {
		if node == nil || active[node] {
			return false
		}
		active[node] = true
		defer delete(active, node)
		if reference, ok := node.attrs["ref"]; ok {
			return populated(ids[reference], active)
		}
		if strings.TrimSpace(node.text.String()) != "" {
			return true
		}
		for _, child := range node.children {
			if populated(child, active) {
				return true
			}
		}
		return false
	}
	for _, row := range rows {
		for i, name := range columns {
			if slices.Contains(required.Columns, name) && !populated(row.children[i], make(map[*xmlNode]bool)) {
				return 0, errors.New("required table value is empty or unresolved")
			}
		}
	}
	return int64(len(rows)), nil
}
