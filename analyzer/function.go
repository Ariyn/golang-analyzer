package analyzer

import (
	"fmt"
	"go/ast"
	"strings"
)

type Field interface {
	String() string
}

type Selector struct {
	Parent           string
	ParentType       string
	Field            Field
	ImportedSelector bool
}

func (s Selector) String() string {
	//log.Println(s.Field, s.Parent, s.ParentType)
	return fmt.Sprintf("%s.%s", s.Field.String(), s.Parent)
}

type Array struct {
	Name  string
	Index string
}

func (a Array) String() string {
	return fmt.Sprintf("%s[%s]", a.Name, a.Index)
}

type BinaryOperator struct {
}

type Type struct {
	Name string
}

func (t Type) String() string {
	return t.Name
}

type Variable struct {
	Name      string
	Type      string
	IsPointer bool
}

func (v Variable) String() string {
	return v.Name
}

type FunctionCall struct {
	Package             string
	Parent              *FunctionStatement
	Name                string
	Parameters          Parameters
	FunctionDeclaration FunctionStatement
	IsImportedFunction  bool
	File                string
	FilePath            string
	Pos                 int
	LineNumber          int
}

func (fc FunctionCall) Identifier() string {
	return fc.Name
}

func (fc FunctionCall) Position() string {
	return fmt.Sprintf("%s:%d", fc.File, fc.LineNumber)
}

func (fc FunctionCall) String() string {
	return fmt.Sprintf("%s%s", fc.Name, fc.Parameters.String())
}

type FunctionStatement struct {
	Path       string
	Package    string
	Receiver   Parameter
	Name       string
	Parameters Parameters
	Returns    Parameters
	Body       *ast.BlockStmt
	SourceCode SourceCode
	Calls      []FunctionCall
	Node       ast.Node
}

func (fs FunctionStatement) Identifier() (idf string) {
	idfs := []string{fs.Package}

	if fs.Receiver.Type != "" {
		idfs = append(idfs, fs.Receiver.Type)
	}
	idfs = append(idfs, fs.Name)

	idf = strings.Join(idfs, ".")
	return
}

func (fs FunctionStatement) String() string {
	receiver := "(" + fs.Receiver.String() + ") "
	if fs.Receiver.Type == "" {
		receiver = ""
	}

	returns := ""
	if len(fs.Returns) == 1 {
		returns = " " + fs.Returns[0].String()
	} else if len(fs.Returns) > 1 {
		returns = " " + fs.Returns.String()
	}

	return fmt.Sprintf("func %s%s%s%s", receiver, fs.Name, fs.Parameters, returns)
}
