package ps6107silent

func consume([]float32) error { return nil }

type Decoder struct{}

func (*Decoder) Release() {}

func (d *Decoder) stage(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	return consume(host)
}
