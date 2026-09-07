package ps6109backend

type NativeHandle int

type CommandRecorder interface {
	Encode()
	Free()
	Reset() error
}

type CommandFactory interface {
	Acquire() (CommandRecorder, error)
}

type OpaqueFactory interface {
	AcquireOpaque() (CommandRecorder, error)
}

type Recorder struct {
	Handle NativeHandle
	State  int
}

type Device struct{ Next int }

func (d *Device) Fresh() NativeHandle {
	d.Next++
	return NativeHandle(d.Next)
}

func NewRecorder(d *Device) *Recorder { return &Recorder{Handle: d.Fresh()} }

func NewRecorderAlternate(d *Device) *Recorder {
	if d.Next < 0 {
		return nil
	}
	return &Recorder{Handle: d.Fresh()}
}

func (d *Device) Acquire() (CommandRecorder, error) { // want Acquire:`proved reusable one-shot wrapper factory`
	return NewRecorder(d), nil
}

// AcquireOpaque has the configured-looking signature but an intentionally
// unproved body, so an importer must not trust its spelling.
func (d *Device) AcquireOpaque() (CommandRecorder, error) {
	if d.Next == 0 {
		return NewRecorder(d), nil
	}
	return NewRecorder(d), nil
}

func (r *Recorder) Encode() { r.State++ }
func (r *Recorder) Free()   { r.Handle, r.State = 0, 0 }
func (r *Recorder) Reset() error {
	r.Handle++
	r.State = 0
	return nil
}
