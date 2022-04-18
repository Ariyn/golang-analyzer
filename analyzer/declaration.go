package analyzer

type DeclarationType int

const (
	FunctionDeclarationType = iota + 1
	VariableDeclarationType
	StructDeclarationType
)

type Declaration interface {
	Name() string
	Type() DeclarationType
}

var _ Declaration = FunctionDeclaration{}

type FunctionDeclaration struct {
	name      string
	Arguments Parameters
	Results   Parameters
}

func (fd FunctionDeclaration) Name() string {
	return ""
}

func (fd FunctionDeclaration) Type() DeclarationType {
	return FunctionDeclarationType
}
