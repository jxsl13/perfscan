package metal

type Buffer struct{}

type ShapeKey int

type CommandBuffer struct{}

func (*CommandBuffer) EncodeRoPE(*Buffer, ShapeKey) {}
func (*CommandBuffer) AppendKV(*Buffer, ShapeKey)   {}
func (*CommandBuffer) Commit()                      {}
func (*CommandBuffer) WaitUntilCompleted()          {}

type Device struct{}

func (*Device) NewCommandBuffer() *CommandBuffer { return &CommandBuffer{} }
