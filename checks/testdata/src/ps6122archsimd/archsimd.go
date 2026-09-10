package ps6122archsimd

type Int64x2 struct{}
type Uint64x2 struct{}
type Int32x4 struct{}
type Uint32x4 struct{}
type Int16x8 struct{}
type Uint16x8 struct{}
type Int8x16 struct{}
type Uint8x16 struct{}
type Int8x2 struct{}

func BroadcastInt64x2(int64) Int64x2 { return Int64x2{} }
func (Int64x2) Add(Int64x2) Int64x2  { return Int64x2{} }
func (Int64x2) ToBits() Uint64x2     { return Uint64x2{} }

func (Int64x2) ShiftAllLeft(uint64) Int64x2    { return Int64x2{} }
func (Int64x2) ShiftAllRight(uint64) Int64x2   { return Int64x2{} }
func (Uint64x2) ShiftAllRight(uint64) Uint64x2 { return Uint64x2{} }
func (Int32x4) ShiftAllLeft(uint64) Int32x4    { return Int32x4{} }
func (Uint32x4) ShiftAllRight(uint64) Uint32x4 { return Uint32x4{} }
func (Uint16x8) ShiftAllLeft(uint64) Uint16x8  { return Uint16x8{} }

// Deliberately malformed lookalikes exercise signature and exact-type guards.
func (Int8x16) ShiftAllLeft(int) Int8x16                { return Int8x16{} }
func (Uint8x16) ShiftAllRight(uint64) Int8x16           { return Int8x16{} }
func (Int16x8) ShiftAllLeft(distance ...uint64) Int16x8 { return Int16x8{} }
func (Int8x2) ShiftAllLeft(uint64) Int8x2               { return Int8x2{} }
