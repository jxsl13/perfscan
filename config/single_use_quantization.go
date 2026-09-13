package config

import (
	"encoding/hex"
	"slices"
)

type SingleUseQuantizationExemption struct {
	Policy string `json:"policy" yaml:"policy"`
	SHA256 string `json:"sha256" yaml:"sha256"`
}

// SingleUseQuantizationContract names semantic APIs. It does not attest fresh
// storage, single use, amortization, numerical equivalence or a measured gain.
type SingleUseQuantizationContract struct {
	Quantizer          string `json:"quantizer" yaml:"quantizer"`
	Consumer           string `json:"consumer" yaml:"consumer"`
	FloatInputArgument int    `json:"floatInputArgument" yaml:"floatInputArgument"`
	// destination names a source-visible void producer filling owner-fresh storage.
	ProducerForm        string `json:"producerForm" yaml:"producerForm"`
	DestinationArgument int    `json:"destinationArgument" yaml:"destinationArgument"`
	PackedArgument      int    `json:"packedArgument" yaml:"packedArgument"`
	RowsArgument        int    `json:"rowsArgument" yaml:"rowsArgument"`
	// Empty retains the initial self-dot form. twoInputDot has no row formal:
	// the complete body must establish one matching packed/weight traversal.
	// sourceSummary composes source-visible allocation, filling and consumer
	// helpers over pointer-free packed value layouts. Meaning review does not
	// attest storage ownership, effects, internal fanout or numerical equivalence.
	// packedByteDot additionally requires source-proved current-block byte lanes,
	// returned signed lane/header origins and the producer's exact byte stride.
	// It does not attest authentic activation provenance or numeric equivalence.
	ConsumerForm                      string                           `json:"consumerForm" yaml:"consumerForm"`
	WeightArgument                    int                              `json:"weightArgument" yaml:"weightArgument"`
	QuantizationAndDotMeaningReviewed bool                             `json:"quantizationAndDotMeaningReviewed" yaml:"quantizationAndDotMeaningReviewed"`
	BenchmarkExemptOwners             []string                         `json:"benchmarkExemptOwners" yaml:"benchmarkExemptOwners"`
	BenchmarkExemptionReason          string                           `json:"benchmarkExemptionReason" yaml:"benchmarkExemptionReason"`
	BenchmarkExemptions               []SingleUseQuantizationExemption `json:"benchmarkExemptions" yaml:"benchmarkExemptions"`
}

func (c *SingleUseQuantizationContract) Valid() bool {
	// Stage one has no source-bound shape/site evidence for exemptions. Do not
	// allow an owner name and prose to suppress every unrelated call or shape.
	if c != nil && (len(c.BenchmarkExemptOwners) != 0 || c.BenchmarkExemptionReason != "") {
		return false
	}
	if c == nil || !psTopKFunctionIDValid(c.Quantizer) || !psTopKFunctionIDValid(c.Consumer) || c.Quantizer == c.Consumer ||
		!c.QuantizationAndDotMeaningReviewed || c.PackedArgument < 0 || c.PackedArgument > 1 {
		return false
	}
	switch c.ProducerForm {
	case "", "return":
		if c.FloatInputArgument != 0 || c.DestinationArgument != 0 {
			return false
		}
	case "destination":
		if c.ConsumerForm != "sourceSummary" || c.FloatInputArgument < 0 || c.FloatInputArgument > 1 || c.DestinationArgument < 0 || c.DestinationArgument > 1 || c.FloatInputArgument == c.DestinationArgument {
			return false
		}
	default:
		return false
	}
	switch c.ConsumerForm {
	case "", "selfDot":
		if c.RowsArgument < 0 || c.RowsArgument > 1 || c.PackedArgument == c.RowsArgument || c.WeightArgument != 0 {
			return false
		}
	case "twoInputDot", "sourceSummary", "packedByteDot":
		if c.RowsArgument != -1 || c.WeightArgument < 0 || c.WeightArgument > 1 || c.PackedArgument == c.WeightArgument {
			return false
		}
	default:
		return false
	}
	seen := make(map[string]bool, len(c.BenchmarkExemptions))
	for _, exemption := range c.BenchmarkExemptions {
		_, err := hex.DecodeString(exemption.SHA256)
		if exemption.Policy == "" || len(exemption.SHA256) != 64 || err != nil || seen[exemption.Policy] {
			return false
		}
		seen[exemption.Policy] = true
	}
	if (len(c.BenchmarkExemptOwners) == 0) != (c.BenchmarkExemptionReason == "") {
		return false
	}
	for i, owner := range c.BenchmarkExemptOwners {
		if !psTopKFunctionIDValid(owner) || slices.Contains(c.BenchmarkExemptOwners[:i], owner) {
			return false
		}
	}
	return true
}

// UsableSingleUseQuantizationContractCount mirrors the advisory's fail-closed
// ambiguity rule. Even two individually valid claims for one boundary conflict.
func UsableSingleUseQuantizationContractCount(in []SingleUseQuantizationContract) int {
	claims := map[string]int{}
	for i := range in {
		c := &in[i]
		claims[c.Quantizer+"\x00"+c.Consumer]++
	}
	count := 0
	for i := range in {
		c := &in[i]
		if c.Valid() && claims[c.Quantizer+"\x00"+c.Consumer] == 1 {
			count++
		}
	}
	return count
}

func cloneSingleUseQuantizationContracts(values []SingleUseQuantizationContract) []SingleUseQuantizationContract {
	result := slices.Clone(values)
	for i := range result {
		result[i].BenchmarkExemptOwners = slices.Clone(result[i].BenchmarkExemptOwners)
		result[i].BenchmarkExemptions = slices.Clone(result[i].BenchmarkExemptions)
	}
	return result
}
