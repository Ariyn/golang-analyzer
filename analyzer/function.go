package analyzer

import (
	"fmt"
	"go/ast"
	"strings"
)

type FunctionCall struct {
	Package             string
	Receiver            string
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

type FunctionStatement struct {
	Package    string
	Receiver   Parameter
	Name       string
	Parameters Parameters
	Returns    Parameters
	Body       *ast.BlockStmt
	SourceCode SourceCode
	Calls      []FunctionCall
}

func (fs FunctionStatement) Identifier() (idf string) {
	idfs := []string{fs.Package}

	if fs.Receiver.Type != "" {
		idfs = append(idfs, fs.Receiver.Type)
	}
	idfs = append(idfs, fs.Name)

	return strings.Join(idfs, ".")
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
