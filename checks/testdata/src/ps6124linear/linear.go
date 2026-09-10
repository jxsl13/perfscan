package linear

import "ps6124backend"

type Linear struct{}

func (*Linear) Forward(*backend.Context) { // want Forward:"global backend effects: routers=ps6124backend.Default;writers="
	_ = backend.Default()
}
