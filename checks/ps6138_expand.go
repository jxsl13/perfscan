package checks

import "strings"

type ps6138Macro struct{ parameter, body string }

// This is not a C preprocessor. A bounded, explicit subset is accepted; every
// other directive or macro use is a barrier, including register/opcode aliases.
func ps6138Expand(source string) ([]ps6138Instruction, bool) {
	if len(source) > 1<<20 {
		return nil, false
	}
	comment := 0
	for i := 0; i < len(source); i++ {
		if comment == 1 {
			if source[i] == '\n' {
				comment = 0
			}
			continue
		}
		if comment == 2 {
			if source[i] == '*' && i+1 < len(source) && source[i+1] == '/' {
				comment = 0
				i++
			}
			continue
		}
		if source[i] == '/' && i+1 < len(source) {
			if source[i+1] == '/' {
				comment = 1
				i++
			} else if source[i+1] == '*' {
				comment = 2
				i++
			}
		}
	}
	if comment == 2 {
		return nil, false
	}
	// Macro-dependent/target-dependent conditions are not modeled. In
	// particular, a function-like macro must not be mistaken for an undefined
	// object macro by another check's restricted preprocessor model.
	for line := range strings.SplitSeq(source, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#if") && line != "#if 0" && line != "#if 1" {
			return nil, false
		}
		if strings.HasPrefix(line, "#else") && line != "#else" {
			return nil, false
		}
		if strings.HasPrefix(line, "#endif") && line != "#endif" {
			return nil, false
		}
	}
	text, ok := ps6128ActiveAssembly(source)
	if !ok {
		return nil, false
	}
	lines := strings.Split(text, "\n")
	macros := make(map[string]ps6138Macro)
	var out []ps6138Instruction
	offset := 0
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		pos := offset
		offset += len(lines[index]) + 1
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#include") {
			if line != "#include \"textflag.h\"" {
				return nil, false
			}
			continue
		}
		if strings.HasPrefix(line, "#undef ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "#undef "))
			if !ps6138Identifier(name) {
				return nil, false
			}
			delete(macros, name)
			continue
		}
		if strings.HasPrefix(line, "#define ") {
			definition := strings.TrimSpace(strings.TrimPrefix(line, "#define "))
			p := strings.IndexByte(definition, '(')
			q := strings.IndexByte(definition, ')')
			if p <= 0 || q < p {
				return nil, false
			}
			name := definition[:p]
			parameter := strings.TrimSpace(definition[p+1 : q])
			if !ps6138Identifier(name) || (parameter != "" && !ps6138Identifier(parameter)) {
				return nil, false
			}
			body := strings.TrimSpace(definition[q+1:])
			for strings.HasSuffix(body, "\\") {
				if index+1 == len(lines) {
					return nil, false
				}
				body = strings.TrimSuffix(body, "\\") + " " + strings.TrimSpace(lines[index+1])
				index++
				offset += len(lines[index]) + 1
				if len(body) > 16384 {
					return nil, false
				}
			}
			macros[name] = ps6138Macro{parameter, body}
			if len(macros) > 128 {
				return nil, false
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			return nil, false
		}
		if !ps6138ExpandLine(line, pos, macros, nil, 0, &out) {
			return nil, false
		}
		if len(out) > 8192 {
			return nil, false
		}
	}
	return out, true
}

func ps6138Identifier(s string) bool {
	if s == "" {
		return false
	}
	for n, c := range s {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || n > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func ps6138ExpandLine(line string, pos int, macros map[string]ps6138Macro, stack map[string]bool, depth int, out *[]ps6138Instruction) bool {
	if depth > 8 || len(*out) > 8192 {
		return false
	}
	for part := range strings.SplitSeq(line, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p := strings.IndexByte(part, '('); p > 0 && strings.HasSuffix(part, ")") {
			name := part[:p]
			if macro, exists := macros[name]; exists {
				if stack[name] {
					return false
				}
				arg := strings.TrimSpace(part[p+1 : len(part)-1])
				if macro.parameter == "" && arg != "" {
					return false
				}
				if macro.parameter != "" {
					if _, ok := ps6138Immediate("$" + arg); !ok {
						return false
					}
				}
				body := macro.body
				if macro.parameter != "" {
					body = ps6138Substitute(body, macro.parameter, arg)
				}
				if stack == nil {
					stack = make(map[string]bool)
				}
				stack[name] = true
				ok := ps6138ExpandLine(body, pos, macros, stack, depth+1, out)
				delete(stack, name)
				if !ok {
					return false
				}
				continue
			}
		}
		if label, yes := strings.CutSuffix(part, ":"); yes {
			if !ps6138Identifier(label) {
				return false
			}
			*out = append(*out, ps6138Instruction{"LABEL", []string{label}, pos})
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		op := fields[0]
		for name := range macros {
			if ps6128IdentifierToken(part, name) {
				return false
			}
		}
		for segment := range strings.SplitSeq(op, ".") {
			if !ps6138Identifier(segment) {
				return false
			}
		}
		var args []string
		if len(fields) > 1 {
			for a := range strings.SplitSeq(strings.Join(fields[1:], ""), ",") {
				if a == "" {
					return false
				}
				args = append(args, a)
			}
		}
		*out = append(*out, ps6138Instruction{op, args, pos})
	}
	return true
}

func ps6138Substitute(body, name, value string) string {
	var result strings.Builder
	result.Grow(len(body))
	for index := 0; index < len(body); {
		end := index
		for end < len(body) && (body[end] == '_' || body[end] >= 'A' && body[end] <= 'Z' || body[end] >= 'a' && body[end] <= 'z' || body[end] >= '0' && body[end] <= '9') {
			end++
		}
		if end == index {
			result.WriteByte(body[index])
			index++
			continue
		}
		if body[index:end] == name {
			result.WriteString(value)
		} else {
			result.WriteString(body[index:end])
		}
		index = end
	}
	return result.String()
}
