package ps6109badmethod

type recorder struct{ handle, state int }
type device struct{}

func (d *device) Fresh() int              { return 1 }
func newRecorder(d *device) *recorder     { return &recorder{handle: d.Fresh()} }
func (d *device) Acquire() *recorder      { return newRecorder(d) }
func (r *recorder) Encode()               {}
func (r *recorder) Free()                 { r.handle = 0 }
func (r *recorder) Other()                { r.handle = 0 }
func (r *recorder) Reset()                { r.handle = 1 }
func (r *recorder) ResetInt() int         { r.handle = 1; return 0 }
func (r *recorder) EncodeVariadic(...int) {}
func candidate() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.Acquire()
		recorder.Encode()
		recorder.Free()
	}
}

func variadicCandidate() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.Acquire()
		recorder.EncodeVariadic()
		recorder.Free()
	}
}
