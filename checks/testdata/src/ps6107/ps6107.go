package ps6107

type Scalar float32

type Device struct{}

func (*Device) UploadF32([]float32) error   { return nil }
func (*Device) UploadScalar([]Scalar) error { return nil }

func consumeF32([]float32) error             { return nil }
func consumeTagged(_ int, _ []float32) error { return nil }

type FakeDevice struct{}

func (*FakeDevice) UploadF32([]float32) error { return nil }

type Decoder struct {
	dim     int
	other   int
	device  *Device
	fake    *FakeDevice
	escaped []float32
}

func (d *Decoder) Release() {}

func (d *Decoder) gatherInto(dst []float32, token int) {
	for i := range dst {
		dst[i] = float32(token + i)
	}
}

func (d *Decoder) fakeGatherInto(dst []float32, token int) {
	for i := range dst {
		dst[i] = float32(token + i)
	}
}

func (d *Decoder) flat(values []float32) error {
	host := make([]float32, len(values)) // want `flat-inline: configured sequential method ps6107.Decoder.flat allocates \[\]float32 staging with runtime extent len\(values\) \(4 bytes/element\), then performs a inline whole-slice overwrite and passes it once to configured synchronous non-retaining ps6107.consumeF32`
	for i := range host {
		host[i] = values[i]
	}
	return consumeF32(host)
}

func (d *Decoder) tagged(values []float32) error {
	host := make([]float32, len(values)) // want `tagged: configured sequential method ps6107.Decoder.tagged allocates \[\]float32 staging`
	for i := range host {
		host[i] = values[i]
	}
	return consumeTagged(7, host)
}

func (d *Decoder) rows(tokens []int) error {
	k := len(tokens)
	host := make([]float32, k*d.dim) // want `rows: configured sequential method ps6107.Decoder.rows allocates \[\]float32 staging with runtime extent k \* d.dim \(4 bytes/element\), then performs a contract-proven exact row-partition overwrite and passes it once to configured synchronous non-retaining ps6107.Device.UploadF32`
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	if err := d.device.UploadF32(host); err != nil {
		return err
	}
	return nil
}

func (d *Decoder) named(values []Scalar) error {
	host := make([]Scalar, len(values)) // want `named-inline: configured sequential method ps6107.Decoder.named allocates \[\]Scalar staging with runtime extent len\(values\) \(4 bytes/element\)`
	for i := 0; i < len(host); i++ {
		host[i] = values[i]
	}
	return d.device.UploadScalar(host)
}

func (d *Decoder) constant(values []float32) error {
	host := make([]float32, 16)
	for i := range host {
		host[i] = values[i]
	}
	return consumeF32(host)
}

func (d *Decoder) capacity(values []float32) error {
	host := make([]float32, len(values), len(values)+1)
	for i := range host {
		host[i] = values[i]
	}
	return consumeF32(host)
}

func (d *Decoder) conditional(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		if values[i] != 0 {
			host[i] = values[i]
		}
	}
	return consumeF32(host)
}

func (d *Decoder) compound(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] += values[i]
	}
	return consumeF32(host)
}

func (d *Decoder) readsZero(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = host[i] + values[i]
	}
	return consumeF32(host)
}

func (d *Decoder) aliases(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	alias := host
	return consumeF32(alias)
}

func (d *Decoder) stores(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	d.escaped = host
	return consumeF32(host)
}

func (d *Decoder) laterUse(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	err := consumeF32(host)
	_ = len(host)
	return err
}

func (d *Decoder) twice(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	_ = consumeF32(host)
	return consumeF32(host)
}

func (d *Decoder) async(values []float32) {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	go consumeF32(host)
}

func (d *Decoder) deferred(values []float32) {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	defer consumeF32(host)
}

func (d *Decoder) wrongConsumer(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	return d.fake.UploadF32(host)
}

func (d *Decoder) methodExpression(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	return (*Device).UploadF32(d.device, host)
}

func (d *Decoder) wrongRows(tokens []int) error {
	host := make([]float32, len(tokens)*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim-1], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) wrongOverwriter(tokens []int) error {
	host := make([]float32, len(tokens)*d.dim)
	for row, token := range tokens {
		d.fakeGatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) wrongWidth(tokens []int, width int) error {
	host := make([]float32, len(tokens)*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*width:(row+1)*width], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) rowAlias(tokens []int) error {
	host := make([]float32, len(tokens)*d.dim)
	for row, token := range tokens {
		part := host[row*d.dim : (row+1)*d.dim]
		d.gatherInto(part, token)
	}
	return d.device.UploadF32(host)
}

func mutateInt(value *int) { *value = 1 }

func (d *Decoder) addressedExtent(tokens []int) error {
	k := len(tokens)
	mutateInt(&k)
	host := make([]float32, k*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) capturedExtent(tokens []int) error {
	k := len(tokens)
	func() { k++ }()
	host := make([]float32, k*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) stringRows(tokens string) error {
	host := make([]float32, len(tokens)*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], int(token))
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) mapRows(tokens map[int]int) error {
	host := make([]float32, len(tokens)*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) snapshotInput(tokens []int) error {
	k := len(tokens)
	tokens = tokens[:0]
	host := make([]float32, k*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) snapshotWidth(tokens []int) error {
	k := len(tokens) * d.dim
	d.dim = 0
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func mutateSliceHeader(values *[]int) { *values = (*values)[:0] }

func (d *Decoder) addressedInput(tokens []int) error {
	k := len(tokens)
	mutateSliceHeader(&tokens)
	host := make([]float32, k*d.dim)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) capturedWidth(tokens []int) error {
	k := len(tokens) * d.dim
	func() { d.dim = 0 }()
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) mutateWidth() { d.dim = 0 }

func (d *Decoder) opaqueWidth(tokens []int) error {
	k := len(tokens) * d.dim
	d.mutateWidth()
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) stableUnrelated(tokens []int) error {
	k := len(tokens) * d.dim
	d.other = 1
	host := make([]float32, k) // want `stable-unrelated: configured sequential method ps6107.Decoder.stableUnrelated allocates \[\]float32 staging`
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func (d *Decoder) wholeReceiverReset(tokens []int) error {
	k := len(tokens) * d.dim
	*d = Decoder{}
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func resetBoxed(value any) { value.(*Decoder).dim = 0 }

func (d *Decoder) boxedReceiverMutation(tokens []int) error {
	k := len(tokens) * d.dim
	resetBoxed(any(d))
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

type decoderHolder struct{ decoder *Decoder }

func resetDecoder(d *Decoder) { d.dim = 0 }

func (d *Decoder) nestedReceiverMutation(tokens []int, holder *decoderHolder) error {
	k := len(tokens) * holder.decoder.dim
	resetDecoder(holder.decoder)
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*holder.decoder.dim:(row+1)*holder.decoder.dim], token)
	}
	return d.device.UploadF32(host)
}

var mutableGlobalWidth = 4

func resetGlobalWidth() { mutableGlobalWidth = 0 }

func (d *Decoder) globalSnapshotMutation(tokens []int) error {
	k := len(tokens) * mutableGlobalWidth
	resetGlobalWidth()
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*mutableGlobalWidth:(row+1)*mutableGlobalWidth], token)
	}
	return d.device.UploadF32(host)
}

var escapedDecoder *Decoder

func rememberDecoder(d *Decoder) { escapedDecoder = d }
func resetEscapedDecoder()       { escapedDecoder.dim = 0 }

func (d *Decoder) previouslyEscaped(tokens []int) error {
	rememberDecoder(d)
	k := len(tokens) * d.dim
	resetEscapedDecoder()
	host := make([]float32, k)
	for row, token := range tokens {
		d.gatherInto(host[row*d.dim:(row+1)*d.dim], token)
	}
	return d.device.UploadF32(host)
}

func bumpIndex(value *int) float32 { *value++; return 1 }

func (d *Decoder) indexMutation(n int) error {
	host := make([]float32, n)
	for i := 0; i < len(host); i++ {
		host[i] = bumpIndex(&i)
	}
	return consumeF32(host)
}

func stagingSize(n int) int { return n }

func (d *Decoder) hiddenSizeCall(n int) error {
	host := make([]float32, stagingSize(n))
	for i := range host {
		host[i] = float32(i)
	}
	return consumeF32(host)
}

func (d *Decoder) unreachable(n int) error {
	return nil
	host := make([]float32, n)
	for i := range host {
		host[i] = float32(i)
	}
	return consumeF32(host)
}

func (d *Decoder) pointerElements(values []*int) error {
	host := make([]*int, len(values))
	for i := range host {
		host[i] = values[i]
	}
	return nil
}

func (d Decoder) valueReceiver(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	return consumeF32(host)
}

type Other struct{}

func (*Other) Release() {}

func (d *Decoder) noMatchingLifecycle(values []float32) error {
	host := make([]float32, len(values))
	for i := range host {
		host[i] = values[i]
	}
	return consumeF32(host)
}
