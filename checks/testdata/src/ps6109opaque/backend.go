package ps6109opaque

type CommandRecorder interface {
	Encode()
	Free()
	Reset() error
}

type CommandFactory interface {
	Acquire() (CommandRecorder, error)
}

type Recorder struct{ Handle, State int }
type Device struct{ Next int }

func (d *Device) Fresh() int          { d.Next++; return d.Next }
func NewRecorder(d *Device) *Recorder { return &Recorder{Handle: d.Fresh()} }
func (d *Device) Acquire() (CommandRecorder, error) {
	if d.Next < 0 {
		return nil, nil
	}
	return NewRecorder(d), nil
}
func (r *Recorder) Encode()      { r.State++ }
func (r *Recorder) Free()        { r.Handle, r.State = 0, 0 }
func (r *Recorder) Reset() error { r.Handle++; r.State = 0; return nil }
