package ps6109misbound

type recorder struct{ handle, state int }
type device struct{}

func (d *device) Fresh() int          { return 1 }
func newRecorder(d *device) *recorder { return &recorder{handle: d.Fresh()} }
func (d *device) Acquire() *recorder { // want Acquire:`proved reusable one-shot wrapper factory`
	return newRecorder(d)
}
func (r *recorder) Encode() {}
func (r *recorder) Free()   { r.handle = 0 }
func (r *recorder) Other()  { r.handle = 0 }
func (r *recorder) Reset()  { r.handle = 1 }

func candidate() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.Acquire()
		recorder.Encode()
		recorder.Free()
	}
}
