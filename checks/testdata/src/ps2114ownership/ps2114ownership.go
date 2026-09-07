package ps2114ownership

import "sync"

// bufferOwner carries the same pointer token from acquire to release. The
// wrapper is allocated only by New on a cold miss, and Put receives that same
// pointer, so neither operation should be diagnosed.
type bufferOwner struct {
	token *[]byte
}

var ownedPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 1024)
		return &buf
	},
}

func acquireOwned() bufferOwner {
	token := ownedPool.Get().(*[]byte)
	*token = (*token)[:0]
	return bufferOwner{token: token}
}

func releaseOwned(owner *bufferOwner) {
	ownedPool.Put(owner.token)
	owner.token = nil
}

// A raw-slice API cannot carry the token back to the pool. Its Put remains a
// reported allocation boundary even though this package also has an owned
// path.
func releaseRaw(buf []byte) {
	ownedPool.Put(buf) // want `allocate the wrapper only on a cold miss, carry that same pointer through the owner, and return it to sync\.Pool; a fresh wrapper at every Put still allocates; removing allocations alone does not prove a wall-time win, so benchmark the actual call site`
}

// The ownership guidance applies to New as well as Put.
var rawFactoryPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 1024) // want `allocate the wrapper only on a cold miss, carry that same pointer through the owner, and return it to sync\.Pool; a fresh wrapper at every Put still allocates; removing allocations alone does not prove a wall-time win, so benchmark the actual call site`
	},
}
