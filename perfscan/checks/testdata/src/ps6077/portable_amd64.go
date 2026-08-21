//go:build amd64

package ps6077

func PortableExp(values []float64) float64 {
	return portableAVX2(values)
}
