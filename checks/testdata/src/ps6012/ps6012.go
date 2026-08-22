package ps6012

type Tensor struct{}

type Op struct {
	Kind   string
	Starts []int
	Ends   []int
}

const (
	OpSlice   = "Slice"
	OpReshape = "Reshape"
	OpAdd     = "Add"
	OpMul     = "Multiply"
	OpConcat  = "Concat"
)

type Backend struct{}

func (*Backend) Execute(op Op, inputs ...Tensor) Tensor { return Tensor{} }
func (*Backend) ExecuteResult(op Op, inputs ...Tensor) (Tensor, error) {
	return Tensor{}, nil
}

func twoPerBatch(be *Backend, input Tensor, batch int) Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		row := be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input) // want `estimated backend dispatches scale as 2B\+1 \(2 per iteration \+ 1 post-loop, runtime bound batch\); high-leverage form`
		shaped := be.Execute(Op{Kind: OpReshape}, row)
		parts = append(parts, shaped)
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

func fourPerBatchThreeAfter(be *Backend, input Tensor, batch int) Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		row := be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input) // want `estimated backend dispatches scale as 4B\+3 \(4 per iteration \+ 3 post-loop, runtime bound batch\); high-leverage form`
		shaped := be.Execute(Op{Kind: OpReshape}, row)
		added := be.Execute(Op{Kind: OpAdd}, shaped)
		multiplied := be.Execute(Op{Kind: OpMul}, added)
		parts = append(parts, multiplied)
	}
	return be.Execute(Op{Kind: OpAdd},
		be.Execute(Op{Kind: OpReshape},
			be.Execute(Op{Kind: OpConcat}, parts...)))
}

func indexedRange(be *Backend, inputs []Tensor) Tensor {
	parts := make([]Tensor, len(inputs))
	for i := range inputs {
		parts[i] = be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, inputs[i]) // want `estimated backend dispatches scale as B\+1 \(1 per iteration \+ 1 post-loop, runtime bound inputs\)`
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

func multiResultExecute(be *Backend, input Tensor, batch int) (Tensor, error) {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		row, err := be.ExecuteResult(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input) // want `estimated backend dispatches scale as B\+1`
		if err != nil {
			return Tensor{}, err
		}
		parts = append(parts, row)
	}
	return be.Execute(Op{Kind: OpConcat}, parts...), nil
}

func constantBound(be *Backend, input Tensor) Tensor {
	parts := make([]Tensor, 0, 4)
	for i := 0; i < 4; i++ {
		parts = append(parts, be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input))
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

func fixedArrayRange(be *Backend, inputs [4]Tensor) Tensor {
	parts := make([]Tensor, 4)
	for i := range inputs {
		parts[i] = be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, inputs[i])
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

func noConcat(be *Backend, input Tensor, batch int) []Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		parts = append(parts, be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input))
	}
	return parts
}

func concatDifferentCollection(be *Backend, input Tensor, batch int, alreadyPacked []Tensor) Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		parts = append(parts, be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input))
	}
	return be.Execute(Op{Kind: OpConcat}, alreadyPacked...)
}

func indexDoesNotDriveSlice(be *Backend, input Tensor, batch int) Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		_ = i
		parts = append(parts, be.Execute(Op{Kind: OpSlice, Starts: []int{0}, Ends: []int{1}}, input))
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

func dispatchResultNotCollected(be *Backend, input Tensor, batch int, unrelated []Tensor) Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		_ = be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input)
		parts = append(parts, unrelated[i])
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

// perfscan:retain-batch-loop
func explicitlyRetained(be *Backend, input Tensor, batch int) Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		parts = append(parts, be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input))
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

func measuredRetainedWinner(be *Backend, input Tensor, batch int) Tensor {
	// The benchmark measured this as the retained winner for independent jobs.
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		parts = append(parts, be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input))
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}

func independentSequences(be *Backend, input Tensor, batch int) Tensor {
	// Independent sequence semantics require separate kernels here.
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		parts = append(parts, be.Execute(Op{Kind: OpSlice, Starts: []int{i}, Ends: []int{i + 1}}, input))
	}
	return be.Execute(Op{Kind: OpConcat}, parts...)
}
