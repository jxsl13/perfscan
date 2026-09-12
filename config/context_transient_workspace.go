package config

import "slices"

// ContextTransientWorkspaceContract supplies ownership/execution facts which
// cannot be inferred from a constructor's make length or a device API name.
// Source matching separately checks the allocation and concrete row-call flow.
type ContextTransientWorkspaceContract struct {
	AllocationMethod                             string   `json:"allocationMethod" yaml:"allocationMethod"`
	AllocatorParameter                           string   `json:"allocatorParameter" yaml:"allocatorParameter"`
	MaximumRowsField                             string   `json:"maximumRowsField" yaml:"maximumRowsField"`
	ScratchFields                                []string `json:"scratchFields" yaml:"scratchFields"`
	GeometryPreservingMethods                    []string `json:"geometryPreservingMethods,omitempty" yaml:"geometryPreservingMethods"`
	RowMethod                                    string   `json:"rowMethod" yaml:"rowMethod"`
	RowArgument                                  int      `json:"rowArgument" yaml:"rowArgument"`
	OneRowExecutionMethod                        string   `json:"oneRowExecutionMethod" yaml:"oneRowExecutionMethod"`
	BatchExecutionMethod                         string   `json:"batchExecutionMethod" yaml:"batchExecutionMethod"`
	BatchRowsBinding                             string   `json:"batchRowsBinding" yaml:"batchRowsBinding"`
	ConstructorOnlyAllocationReviewed            bool     `json:"constructorOnlyAllocationReviewed" yaml:"constructorOnlyAllocationReviewed"`
	AllocatorCallbackOwnershipReviewed           bool     `json:"allocatorCallbackOwnershipReviewed" yaml:"allocatorCallbackOwnershipReviewed"`
	AllExecutionPathsTransientActiveRowsReviewed bool     `json:"allExecutionPathsTransientActiveRowsReviewed" yaml:"allExecutionPathsTransientActiveRowsReviewed"`
	RecurrentOneRowPathsReviewed                 bool     `json:"recurrentOneRowPathsReviewed" yaml:"recurrentOneRowPathsReviewed"`
	ReceiverCallsSequential                      bool     `json:"receiverCallsSequential" yaml:"receiverCallsSequential"`
	CheckedGeometryReviewed                      bool     `json:"checkedGeometryReviewed" yaml:"checkedGeometryReviewed"`
	SynchronizationLifecycleReviewed             bool     `json:"synchronizationLifecycleReviewed" yaml:"synchronizationLifecycleReviewed"`
	ExternalObservationReviewed                  bool     `json:"externalObservationReviewed" yaml:"externalObservationReviewed"`
}

func (c *ContextTransientWorkspaceContract) Valid() bool {
	if !psTopKMethodIDValid(c.AllocationMethod) || !psTopKMethodIDValid(c.RowMethod) ||
		!psTopKMethodIDValid(c.OneRowExecutionMethod) || !psTopKMethodIDValid(c.BatchExecutionMethod) ||
		c.AllocationMethod == c.RowMethod || c.OneRowExecutionMethod == c.BatchExecutionMethod ||
		!psTopKIdentifierValid(c.AllocatorParameter) || !psTopKIdentifierValid(c.MaximumRowsField) ||
		!psTopKIdentifierValid(c.BatchRowsBinding) || c.RowArgument < 0 || len(c.ScratchFields) == 0 ||
		!c.ConstructorOnlyAllocationReviewed || !c.AllocatorCallbackOwnershipReviewed ||
		!c.AllExecutionPathsTransientActiveRowsReviewed || !c.RecurrentOneRowPathsReviewed ||
		!c.ReceiverCallsSequential || !c.CheckedGeometryReviewed ||
		!c.SynchronizationLifecycleReviewed || !c.ExternalObservationReviewed {
		return false
	}
	seen := make(map[string]bool, len(c.ScratchFields))
	for _, field := range c.ScratchFields {
		if !psTopKIdentifierValid(field) || field == c.MaximumRowsField || seen[field] {
			return false
		}
		seen[field] = true
	}
	for _, method := range c.GeometryPreservingMethods {
		if !psTopKMethodIDValid(method) {
			return false
		}
	}
	return true
}

// Duplicate allocation-site contracts are contradictory, even when one is
// incomplete; the analyzer must not pick whichever entry happens to come first.
func UsableContextTransientWorkspaceContractCount(contracts []ContextTransientWorkspaceContract) int {
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].AllocationMethod]++
	}
	n := 0
	for i := range contracts {
		if contracts[i].Valid() && counts[contracts[i].AllocationMethod] == 1 {
			n++
		}
	}
	return n
}

func cloneContextTransientWorkspaceContracts(contracts []ContextTransientWorkspaceContract) []ContextTransientWorkspaceContract {
	result := slices.Clone(contracts)
	for i := range result {
		result[i].ScratchFields = slices.Clone(result[i].ScratchFields)
		result[i].GeometryPreservingMethods = slices.Clone(result[i].GeometryPreservingMethods)
	}
	return result
}
