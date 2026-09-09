package ps6120

import "sync"

type Buffer struct{}
type Device struct{}

func (*Device) Upload([]float32) *Buffer                    { return &Buffer{} }
func (*Device) Dispatch(*Buffer, []float32, []float32, int) {}
func exactCodebook() []float32                              { return []float32{8, 25, 43, 8, 25, 43, 8, 25} }

func literal(d *Device, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := []float32{8, 25, 43, 8, 25, 43, 8, 25} // want `invariant codebook is freshly materialized and uploaded inside a repeated dispatch path`
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
func external(d *Device, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook() // want `construct and losslessly pack it once`
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
func dynamic(d *Device, input, output []float32) {
	for row := range output {
		wide := exactCodebook() // want `inside a dispatch loop that may execute repeatedly \(once per ranged element\)`
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}

type CheckedDevice struct{}

func exactCodebookChecked() ([]float32, error)           { return exactCodebook(), nil }
func (*CheckedDevice) Upload([]float32) (*Buffer, error) { return &Buffer{}, nil }
func (*CheckedDevice) Dispatch(*Buffer, int) error       { return nil }
func checked(d *CheckedDevice) error {
	for row := 0; row < 4; row++ {
		wide, err := exactCodebookChecked() // want `construct and losslessly pack it once`
		if err != nil {
			return err
		}
		resident, err := d.Upload(wide)
		if err != nil {
			return err
		}
		if err := d.Dispatch(resident, row); err != nil {
			return err
		}
	}
	return nil
}

func mapRange(d *Device, rows map[int]bool, input, output []float32) {
	for row := range rows {
		wide := exactCodebook() // want `inside a dispatch loop that may execute repeatedly \(once per ranged element\)`
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
func channelRange(d *Device, rows <-chan int, input, output []float32) {
	for row := range rows {
		wide := exactCodebook() // want `inside a dispatch loop that may execute repeatedly \(once per ranged element\)`
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
func stringRange(d *Device, rows string, input, output []float32) {
	for row := range rows {
		wide := exactCodebook() // want `inside a dispatch loop that may execute repeatedly \(once per ranged element\)`
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
func nested(d *Device, input, output []float32) {
	for range output {
		for row := 0; row < 4; row++ {
			wide := exactCodebook() // want `inside a repeated dispatch path`
			resident := d.Upload(wide)
			d.Dispatch(resident, input, output, row)
		}
	}
}
func once(d *Device, input, output []float32) {
	wide := exactCodebook()
	resident := d.Upload(wide)
	for row := 0; row < 4; row++ {
		d.Dispatch(resident, input, output, row)
	}
}

var actualOnceGuard sync.Once

func actualOnceSite(d *Device, input, output []float32) {
	actualOnceGuard.Do(func() { wide := exactCodebook(); resident := d.Upload(wide); d.Dispatch(resident, input, output, 0) })
	for row := 0; row < 4; row++ {
		d.Dispatch(&Buffer{}, input, output, row)
	}
}
func alias(d *Device, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		alias := wide
		resident := d.Upload(alias)
		d.Dispatch(resident, input, output, row)
	}
}
func zero(d *Device, input, output []float32) {
	for row := 0; row < 0; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
func unreachable(d *Device, input, output []float32) {
	return
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}

func oneTrip(d *Device, input, output []float32) {
	for row := 0; row < 1; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
func deadNested(d *Device, input, output []float32) {
	return
	for range output {
		for row := 0; row < 4; row++ {
			wide := exactCodebook()
			resident := d.Upload(wide)
			d.Dispatch(resident, input, output, row)
		}
	}
}

type GuardDevice struct{}

func (*GuardDevice) Upload([]float32) *Buffer      { return &Buffer{} }
func (*GuardDevice) DispatchAddress(*Buffer, *int) {}
func (*GuardDevice) DispatchMutating(*Buffer, int) {}
func indexMutation(d *GuardDevice) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.DispatchMutating(resident, func() int { row++; return row }())
	}
}
func indexAddress(d *GuardDevice) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.DispatchAddress(resident, &row)
	}
}

type ErrorDevice struct{}

func (*ErrorDevice) Upload([]float32) *Buffer                          { return &Buffer{} }
func (*ErrorDevice) Dispatch(*Buffer, []float32, []float32, int) error { return nil }
func discardedDispatchError(d *ErrorDevice, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}

type ScalarDevice struct{}

func (*ScalarDevice) Upload([]float32) int                    { return 1 }
func (*ScalarDevice) Dispatch(int, []float32, []float32, int) {}
func scalarDevice(d *ScalarDevice, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}

type InterfaceDevice struct{}

func (*InterfaceDevice) Upload([]float32) any                    { return &Buffer{} }
func (*InterfaceDevice) Dispatch(any, []float32, []float32, int) {}
func interfaceDevice(d *InterfaceDevice, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}

type InterfaceUploadDevice struct{}

func (*InterfaceUploadDevice) Upload(any) *Buffer                          { return &Buffer{} }
func (*InterfaceUploadDevice) Dispatch(*Buffer, []float32, []float32, int) {}
func interfaceUploadParam(d *InterfaceUploadDevice, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}

type InterfaceDispatchDevice struct{}

func (*InterfaceDispatchDevice) Upload([]float32) *Buffer                { return &Buffer{} }
func (*InterfaceDispatchDevice) Dispatch(any, []float32, []float32, int) {}
func interfaceDispatchParam(d *InterfaceDispatchDevice, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := exactCodebook()
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}

func wrongProducerGuard(d *CheckedDevice) error {
	for row := 0; row < 4; row++ {
		wide, err := exactCodebookChecked()
		if err != nil {
			return nil
		}
		resident, err := d.Upload(wide)
		if err != nil {
			return err
		}
		if err := d.Dispatch(resident, row); err != nil {
			return err
		}
	}
	return nil
}
func swallowedUploadError(d *CheckedDevice) error {
	for row := 0; row < 4; row++ {
		wide, err := exactCodebookChecked()
		if err != nil {
			return err
		}
		resident, err := d.Upload(wide)
		if err != nil {
			continue
		}
		if err := d.Dispatch(resident, row); err != nil {
			return err
		}
	}
	return nil
}
func roundedConstants(d *Device, input, output []float32) {
	for row := 0; row < 4; row++ {
		wide := []float32{16777216, 16777217, 25, 16777216, 16777217, 25, 16777216, 16777217} // want `inside a repeated dispatch path`
		resident := d.Upload(wide)
		d.Dispatch(resident, input, output, row)
	}
}
