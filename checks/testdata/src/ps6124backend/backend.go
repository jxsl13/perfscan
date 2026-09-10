package backend

type Kind int

const (
	CPU Kind = iota
	GPU
)

type Backend struct{}
type Context struct{}

func Get(Kind) (*Backend, error)               { return nil, nil }
func NewContext() *Context                     { return nil }
func (*Context) WithBackend(*Backend) *Context { return nil }
func Preference() []Kind                       { return nil }
func SetPreference(...Kind)                    {}
func Default() *Backend                        { return nil }
