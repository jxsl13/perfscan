package ps2114

import (
	"bytes"
	"sync"
)

// --- positive: []byte pool stored by value -------------------------------

var bufPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 1024) // want `sync\.Pool New returns non-pointer value \(type \[\]byte\); every pool cycle boxes it, allocating; return a pointer instead`
	},
}

func useValuePool() int {
	b := bufPool.Get().([]byte)
	b = append(b[:0], "payload"...)
	n := len(b)
	bufPool.Put(b) // want `sync\.Pool\.Put of non-pointer value \(type \[\]byte\) boxes it into an interface, allocating on every Put; pool a pointer instead`
	return n
}

// --- positive: struct pooled by value ------------------------------------

type frame struct {
	seq     int
	payload [64]byte
}

var framePool = sync.Pool{
	New: func() any {
		return frame{} // want `sync\.Pool New returns non-pointer value \(type frame\); every pool cycle boxes it, allocating; return a pointer instead`
	},
}

func useFramePool() {
	f := framePool.Get().(frame)
	f.seq++
	framePool.Put(f) // want `sync\.Pool\.Put of non-pointer value \(type frame\) boxes it into an interface, allocating on every Put; pool a pointer instead`
}

// --- positive: New assigned as a field, array value ----------------------

var latePool sync.Pool

func init() {
	latePool.New = func() any {
		return [16]byte{} // want `sync\.Pool New returns non-pointer value \(type \[16\]byte\); every pool cycle boxes it, allocating; return a pointer instead`
	}
}

// --- positive: pool accessed through a pointer ---------------------------

func putThroughPointer(p *sync.Pool, s string) {
	p.Put(s) // want `sync\.Pool\.Put of non-pointer value \(type string\) boxes it into an interface, allocating on every Put; pool a pointer instead`
}

// --- negative: pointer-storing pools are the remedy ----------------------

var ptrPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 1024)
		return &b
	},
}

func usePtrPool() int {
	p := ptrPool.Get().(*[]byte)
	*p = append((*p)[:0], "payload"...)
	n := len(*p)
	ptrPool.Put(p)
	return n
}

var bufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func useBufferPool() string {
	b := bufferPool.Get().(*bytes.Buffer)
	b.Reset()
	b.WriteString("x")
	s := b.String()
	bufferPool.Put(b)
	return s
}

// --- negative: pointer-like kinds (map, chan, func) never box ------------

var mapPool = sync.Pool{
	New: func() any { return make(map[string]int, 8) },
}

func useMapPool() {
	m := mapPool.Get().(map[string]int)
	m["hits"]++
	mapPool.Put(m)
}

// --- negative: interface-typed argument, dynamic type unknown ------------

func putOpaque(v any) {
	bufPool.Put(v)
}

// --- negative: untyped nil -----------------------------------------------

func putNil() {
	bufPool.Put(nil)
}

// --- negative: zero-size value boxes without allocating ------------------

func putZeroSize() {
	bufPool.Put(struct{}{})
}

// --- negative: generic wrapper; instantiation may be pointer-like --------

func putGeneric[T any](p *sync.Pool, v T) {
	p.Put(v)
}

// --- negative: a Put method on a type that is not sync.Pool --------------

type notPool struct{}

func (notPool) Put(v any) {}

func useNotPool() {
	var np notPool
	np.Put(make([]byte, 8))
}

// --- negative: nested func literal inside New belongs to itself ----------

var factoryPool = sync.Pool{
	New: func() any {
		return func() []byte { // a func value: one word, no boxing alloc
			return make([]byte, 8) // this return is the nested literal's, not New's
		}
	},
}
