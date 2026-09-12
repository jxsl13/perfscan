package config

import "slices"

// ActiveBoundFallbackContract describes reviewed API semantics, not semantics
// inferred from names or an interface method signature.
type ActiveBoundFallbackContract struct {
	WrapperMethod                         string `json:"wrapperMethod" yaml:"wrapperMethod"`
	ProviderType                          string `json:"providerType" yaml:"providerType"`
	CapabilityType                        string `json:"capabilityType" yaml:"capabilityType"`
	BoundedMethod                         string `json:"boundedMethod" yaml:"boundedMethod"`
	CapacityMethod                        string `json:"capacityMethod" yaml:"capacityMethod"`
	ProviderArgument                      int    `json:"providerArgument" yaml:"providerArgument"`
	BufferArguments                       []int  `json:"bufferArguments" yaml:"bufferArguments"`
	OperationArgument                     int    `json:"operationArgument" yaml:"operationArgument"`
	RowsArgument                          int    `json:"rowsArgument" yaml:"rowsArgument"`
	WidthArgument                         int    `json:"widthArgument" yaml:"widthArgument"`
	BoundedActivePrefixReviewed           bool   `json:"boundedActivePrefixReviewed" yaml:"boundedActivePrefixReviewed"`
	FallbackCapacityWideReviewed          bool   `json:"fallbackCapacityWideReviewed" yaml:"fallbackCapacityWideReviewed"`
	SameOperationBufferRolesReviewed      bool   `json:"sameOperationBufferRolesReviewed" yaml:"sameOperationBufferRolesReviewed"`
	ActiveRowsMayBeBelowCapacityReviewed  bool   `json:"activeRowsMayBeBelowCapacityReviewed" yaml:"activeRowsMayBeBelowCapacityReviewed"`
	InactiveTailUnobservedReviewed        bool   `json:"inactiveTailUnobservedReviewed" yaml:"inactiveTailUnobservedReviewed"`
	ProviderAndBuildPathsReviewed         bool   `json:"providerAndBuildPathsReviewed" yaml:"providerAndBuildPathsReviewed"`
	ErrorsShapeAndSynchronizationReviewed bool   `json:"errorsShapeAndSynchronizationReviewed" yaml:"errorsShapeAndSynchronizationReviewed"`
}

func (c *ActiveBoundFallbackContract) Valid() bool {
	if !psTopKMethodIDValid(c.WrapperMethod) || !psTopKFunctionIDValid(c.ProviderType) || !psTopKFunctionIDValid(c.CapabilityType) ||
		c.ProviderType == c.CapabilityType || !psTopKIdentifierValid(c.BoundedMethod) || !psTopKIdentifierValid(c.CapacityMethod) ||
		c.BoundedMethod == c.CapacityMethod || len(c.BufferArguments) != 3 ||
		!c.BoundedActivePrefixReviewed || !c.FallbackCapacityWideReviewed || !c.SameOperationBufferRolesReviewed ||
		!c.ActiveRowsMayBeBelowCapacityReviewed || !c.InactiveTailUnobservedReviewed || !c.ProviderAndBuildPathsReviewed || !c.ErrorsShapeAndSynchronizationReviewed {
		return false
	}
	roles := slices.Concat([]int{c.ProviderArgument, c.OperationArgument, c.RowsArgument, c.WidthArgument}, c.BufferArguments)
	seen := make([]bool, len(roles))
	for _, role := range roles {
		if role < 0 || role >= len(roles) || seen[role] {
			return false
		}
		seen[role] = true
	}
	return true
}

func UsableActiveBoundFallbackContractCount(contracts []ActiveBoundFallbackContract) int {
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].WrapperMethod]++
	}
	n := 0
	for i := range contracts {
		if contracts[i].Valid() && counts[contracts[i].WrapperMethod] == 1 {
			n++
		}
	}
	return n
}

func cloneActiveBoundFallbackContracts(contracts []ActiveBoundFallbackContract) []ActiveBoundFallbackContract {
	result := slices.Clone(contracts)
	for i := range result {
		result[i].BufferArguments = slices.Clone(result[i].BufferArguments)
	}
	return result
}
