package ps2107

import (
	"fmt"
)

// %d over a plain int: fixed to strconv.Itoa.
func decimal(i int) string {
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// %d over a narrower signed type: fixed to strconv.FormatInt.
func narrow(i int16) string {
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// %d over an unsigned type: fixed to strconv.FormatUint.
func wide(u uint64) string {
	return fmt.Sprintf("%d", u) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// %t over a plain bool: fixed to strconv.FormatBool.
func truth(b bool) string {
	return fmt.Sprintf("%t", b) // want `fmt\.Sprintf of a single %t value boxes the argument and walks fmt's formatter state machine; strconv\.FormatBool converts it directly`
}

// %x over a plain []byte: fixed to hex.EncodeToString.
func dump(bs []byte) string {
	return fmt.Sprintf("%x", bs) // want `fmt\.Sprintf of a single %x \[\]byte value boxes the argument and walks fmt's formatter state machine; hex\.EncodeToString converts it directly`
}

// %s over a plain string: the call is an identity — fixed to the value.
func same(s string) string {
	return fmt.Sprintf("%s", s) // want `fmt\.Sprintf of a single %s string value is an identity format through fmt's reflection path; use the string value directly`
}

type port int

// Named integer type: it could implement fmt.Formatter, which fmt honors
// for %d and strconv would not — reported, no fix.
func named(p port) string {
	return fmt.Sprintf("%d", p) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

type blob []byte

// Named byte-slice type: could implement fmt.Formatter and is not
// assignable to hex's []byte parameter — reported, no fix.
func namedBytes(b blob) string {
	return fmt.Sprintf("%x", b) // want `fmt\.Sprintf of a single %x \[\]byte value boxes the argument and walks fmt's formatter state machine; hex\.EncodeToString converts it directly`
}

// strconv is shadowed at the call site: the rewrite could not reference
// the package — reported, no fix.
func shadowed(i int) string {
	strconv := i
	_ = strconv
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// Two verbs need real formatting: silent.
func pair(a, b int) string {
	return fmt.Sprintf("%d-%d", a, b)
}

// Literal text around the verb is not a bare conversion: silent.
func labeled(i int) string {
	return fmt.Sprintf("id=%d", i)
}

// %x over a string is out of scope: silent.
func hexString(s string) string {
	return fmt.Sprintf("%x", s)
}
