package config

import "slices"

// NativeSnapshotReuseContract separates typed source roles from reviewed native
// lifetime and workload policy. Review acknowledgements are not source proofs.
type NativeSnapshotReuseContract struct {
	CandidateMethod                         string                       `json:"candidateMethod" yaml:"candidateMethod"`
	AcquireCallable                         string                       `json:"acquireCallable" yaml:"acquireCallable"`
	TokensCallable                          string                       `json:"tokensCallable" yaml:"tokensCallable"`
	LifecycleMethod                         string                       `json:"lifecycleMethod" yaml:"lifecycleMethod"`
	ReceiverHandleField                     string                       `json:"receiverHandleField" yaml:"receiverHandleField"`
	ResultSliceField                        string                       `json:"resultSliceField" yaml:"resultSliceField"`
	ResultStringField                       string                       `json:"resultStringField" yaml:"resultStringField"`
	NativeStringField                       string                       `json:"nativeStringField" yaml:"nativeStringField"`
	FillCallable                            string                       `json:"fillCallable" yaml:"fillCallable"`
	OwnMethod                               string                       `json:"ownMethod" yaml:"ownMethod"`
	IntoMethod                              string                       `json:"intoMethod" yaml:"intoMethod"`
	HandleArgument                          int                          `json:"handleArgument" yaml:"handleArgument"`
	PointerOutArgument                      int                          `json:"pointerOutArgument" yaml:"pointerOutArgument"`
	CountOutArgument                        int                          `json:"countOutArgument" yaml:"countOutArgument"`
	ScalarOutArguments                      []int                        `json:"scalarOutArguments" yaml:"scalarOutArguments"`
	ScalarResultFields                      []string                     `json:"scalarResultFields" yaml:"scalarResultFields"`
	EventNumericFields                      []NativeSnapshotNumericField `json:"eventNumericFields" yaml:"eventNumericFields"`
	EventSpanField                          string                       `json:"eventSpanField" yaml:"eventSpanField"`
	EventStartField                         string                       `json:"eventStartField" yaml:"eventStartField"`
	EventDurationField                      string                       `json:"eventDurationField" yaml:"eventDurationField"`
	NativeStorageLifetimeReviewed           bool                         `json:"nativeStorageLifetimeReviewed" yaml:"nativeStorageLifetimeReviewed"`
	NativeRecordShapeAndTerminationReviewed bool                         `json:"nativeRecordShapeAndTerminationReviewed" yaml:"nativeRecordShapeAndTerminationReviewed"`
	ReadonlySynchronousExtractionReviewed   bool                         `json:"readonlySynchronousExtractionReviewed" yaml:"readonlySynchronousExtractionReviewed"`
	TokenIdentityAndContentReviewed         bool                         `json:"tokenIdentityAndContentReviewed" yaml:"tokenIdentityAndContentReviewed"`
	RepeatedExtractionPolicyReviewed        bool                         `json:"repeatedExtractionPolicyReviewed" yaml:"repeatedExtractionPolicyReviewed"`
	OwnedOutputAndErrorSemanticsReviewed    bool                         `json:"ownedOutputAndErrorSemanticsReviewed" yaml:"ownedOutputAndErrorSemanticsReviewed"`
}

// NativeSnapshotNumericField binds native and owned-result event field roles.
type NativeSnapshotNumericField struct {
	NativeField string `json:"nativeField" yaml:"nativeField"`
	ResultField string `json:"resultField" yaml:"resultField"`
}

func (c *NativeSnapshotReuseContract) Valid() bool {
	if !psTopKMethodIDValid(c.CandidateMethod) || !psTopKMethodIDValid(c.LifecycleMethod) || !psTopKMethodIDValid(c.OwnMethod) || !psTopKMethodIDValid(c.IntoMethod) ||
		!psTopKFunctionIDValid(c.FillCallable) || !psTopKIdentifierValid(c.ReceiverHandleField) || !psTopKIdentifierValid(c.ResultStringField) || !psTopKIdentifierValid(c.NativeStringField) ||
		(c.ResultSliceField != "" && !psTopKIdentifierValid(c.ResultSliceField)) || !nativeSnapshotReuseCallable(c.AcquireCallable) || !nativeSnapshotReuseCallable(c.TokensCallable) ||
		!c.NativeStorageLifetimeReviewed || !c.NativeRecordShapeAndTerminationReviewed || !c.ReadonlySynchronousExtractionReviewed || !c.TokenIdentityAndContentReviewed || !c.RepeatedExtractionPolicyReviewed || !c.OwnedOutputAndErrorSemanticsReviewed {
		return false
	}
	roles := slices.Concat([]int{c.HandleArgument, c.PointerOutArgument, c.CountOutArgument}, c.ScalarOutArguments)
	methods := []string{c.CandidateMethod, c.LifecycleMethod, c.OwnMethod, c.IntoMethod}
	for i, id := range methods {
		for _, other := range methods[:i] {
			if id == other {
				return false
			}
		}
	}
	seen := make([]bool, len(roles))
	for _, role := range roles {
		if role < 0 || role >= len(roles) || seen[role] {
			return false
		}
		seen[role] = true
	}
	if len(c.ScalarResultFields) != len(c.ScalarOutArguments) || len(c.EventNumericFields) == 0 {
		return false
	}
	fields := make(map[string]bool, len(c.EventNumericFields)+1)
	fields[c.ResultStringField] = true
	for _, f := range c.EventNumericFields {
		if !psTopKIdentifierValid(f.NativeField) || !psTopKIdentifierValid(f.ResultField) || fields[f.ResultField] {
			return false
		}
		fields[f.ResultField] = true
	}
	if c.EventSpanField != "" && (!fields[c.EventStartField] || !fields[c.EventDurationField] || c.EventStartField == c.EventDurationField || c.EventStartField == c.ResultStringField || c.EventDurationField == c.ResultStringField) {
		return false
	}
	fields = make(map[string]bool, len(c.ScalarResultFields)+2)
	if c.ResultSliceField != "" {
		fields[c.ResultSliceField] = true
	}
	for _, f := range c.ScalarResultFields {
		if !psTopKIdentifierValid(f) || fields[f] {
			return false
		}
		fields[f] = true
	}
	if c.EventSpanField != "" {
		if !psTopKIdentifierValid(c.EventSpanField) || fields[c.EventSpanField] || !psTopKIdentifierValid(c.EventStartField) || !psTopKIdentifierValid(c.EventDurationField) {
			return false
		}
	}
	return c.ResultSliceField != "" || len(c.ScalarResultFields) == 0 && c.EventSpanField == ""
}

func nativeSnapshotReuseCallable(id string) bool {
	return psTopKFunctionIDValid(id)
}

func UsableNativeSnapshotReuseContractCount(contracts []NativeSnapshotReuseContract) int {
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].CandidateMethod]++
	}
	n := 0
	for i := range contracts {
		if contracts[i].Valid() && counts[contracts[i].CandidateMethod] == 1 {
			n++
		}
	}
	return n
}

func cloneNativeSnapshotReuseContracts(contracts []NativeSnapshotReuseContract) []NativeSnapshotReuseContract {
	out := slices.Clone(contracts)
	for i := range out {
		out[i].ScalarOutArguments = slices.Clone(out[i].ScalarOutArguments)
		out[i].ScalarResultFields = slices.Clone(out[i].ScalarResultFields)
		out[i].EventNumericFields = slices.Clone(out[i].EventNumericFields)
	}
	return out
}
