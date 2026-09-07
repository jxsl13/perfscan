package ps6109noconfig

type Recorder struct{}

func NewRecorder() *Recorder { return &Recorder{} }
func (*Recorder) Encode()    {}
func (*Recorder) Free()      {}

func convincingNamesStaySilent() {
	for generation := 0; generation < 8; generation++ {
		recorder := NewRecorder()
		recorder.Encode()
		recorder.Free()
	}
}
