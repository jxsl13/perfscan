package ps6120silent

type Buffer struct{}
type Device struct{}

func (*Device) Upload([]float32) *Buffer { return &Buffer{} }
func (*Device) Dispatch(*Buffer, int)    {}
func exactCodebook() []float32           { return []float32{8, 25, 43} }
func candidate(d *Device) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, row)
	}
}
