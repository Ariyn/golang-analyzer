package analyzer

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type Import struct {
	Name  string
	Alias string
	Path  string
}

func (i Import) Caller() string {
	if i.Alias != "" && i.Alias != "_" {
		return i.Alias
	}

	return i.Name
}

//type Typ struct {
//	Name string
//	Pkg  string
//}
//
//func (t Typ) String() string {
//	return ""
//}

type Parameter struct {
	Pkg                  string
	Name                 string
	IsPointer            bool
	Type                 string
	IsMultipleParameters bool
}

func (p Parameter) String() string {
	starExpr := ""
	if p.IsPointer {
		starExpr = "*"
	}

	name := p.Name
	typ := p.Type
	if p.IsMultipleParameters {
		typ = ""
	} else if name != "" {
		name = name + " "
	}

	return fmt.Sprintf("%s%s%s", name, starExpr, typ)
}

type SourceCode struct {
	Test, Pos token.Pos
	End       token.Pos
	Data      string
	File      *os.File
}

type Parameters []Parameter

func (ps Parameters) String() string {
	prmsString := make([]string, 0)

	for _, p := range ps {
		prmsString = append(prmsString, p.String())
	}

	return "(" + strings.Join(prmsString, ", ") + ")"
}

type FunctionCall struct {
	Package              string
	Receiver             string
	Name                 string
	Parameters           Parameters
	FunctionDeclarations FunctionStatement
	IsImportedFunction   bool
	File                 string
	FilePath             string
	Pos                  int
	LineNumber           int
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

// TODO: make these variables into
var variableTable = make(map[string]interface{})
var ImportTable = make(map[string]Import)
var functionsByName = make(map[string]FunctionStatement)

// TODO: make this global variable into channel
var functionCalls = make([]FunctionCall, 0)

func Parse() {
	log.SetFlags(log.LstdFlags | log.Llongfile)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "sample", func(info fs.FileInfo) bool {
		return true
	}, 0)

	if err != nil {
		panic(err)
	}

	f, err := os.Open("/Users/hwangminuk/Documents/go/src/github.com/ariyn/golang-analyzer/sample/main.go")
	if err != nil {
		panic(err)
	}

	functions := []FunctionStatement{}

	for pkgName, pkg := range pkgs {
		fch, insptr := inspector(context.TODO(), pkgName, f)
		go func(fch chan FunctionStatement) {
			ast.Inspect(pkg, insptr)
			close(fch)
		}(fch)

		for function := range fch {
			functions = append(functions, function)
		}
	}

	for index, function := range functionCalls {
		identifier := function.Identifier()
		decl, _ := functionsByName[identifier]

		f := fset.File(token.Pos(function.Pos))
		function.File = f.Name()
		//if !ok && !function.IsImportedFunction {
		//	panic(fmt.Errorf("not declared function called (%s)", function.Identifier()))
		//}

		// TODO: declations to declation
		function.FunctionDeclarations = decl
		decl.Calls = append(decl.Calls, function)

		functionsByName[identifier] = decl

		functionCalls[index] = function
	}

	for _, function := range functionCalls {
		log.Println(function.File, function.Identifier(), function.LineNumber)
	}
}

func ParseImport(is *ast.ImportSpec) Import {
	var alias string
	if is.Name != nil {
		alias = is.Name.Name
	}

	var path, name string
	if is.Path != nil {
		path = is.Path.Value[1 : len(is.Path.Value)-1]
		pathParts := strings.Split(path, "/")
		if len(pathParts) > 0 {
			name = pathParts[len(pathParts)-1]
		}
	}

	return Import{
		Name:  name,
		Alias: alias,
		Path:  path,
	}
}

func ParseFuncCall(pkgName string, ce *ast.CallExpr) (functionCall FunctionCall) {
	functionCall.Pos = int(ce.Pos())

	switch x := ce.Fun.(type) {
	case *ast.Ident:
		if x.Obj != nil {
			functionDecl := parseFuncDecl(pkgName, x.Obj.Decl.(*ast.FuncDecl))
			functionCall.Name = functionDecl.Identifier()
		} else {
			functionCall.Name = x.Name
		}
	case *ast.SelectorExpr:
		functionCall.Name = x.X.(*ast.Ident).Name + "." + x.Sel.Name
		functionCall.IsImportedFunction = x.X.(*ast.Ident).Obj == nil
	}

	return functionCall
}

func inspector(ctx context.Context, pkgName string, file *os.File) (fch chan FunctionStatement, f func(node ast.Node) bool) {
	sourceCode, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}

	fch = make(chan FunctionStatement)

	f = func(node ast.Node) bool {
		// golang does not allow adding method to exported type
		switch x := node.(type) {
		case *ast.FuncDecl:
			function := parseFuncDecl(pkgName, x)
			function.SourceCode.Data = string(sourceCode[x.Pos()-1 : x.End()])
			function.SourceCode.File = file
			functionsByName[function.Identifier()] = function
		case *ast.ImportSpec:
			imp := ParseImport(x)
			ImportTable[imp.Caller()] = imp
		case *ast.CallExpr:
			functionCall := ParseFuncCall(pkgName, x)
			functionCall.File = file.Name()
			functionCall.LineNumber = strings.Count(string(sourceCode[:functionCall.Pos]), "\n") + 1

			functionCalls = append(functionCalls, functionCall)
		}
		return true
	}

	return
}

func parseParameters(p *ast.Field) (parameters Parameters) {

	var prm Parameter

	switch prmType := p.Type.(type) {
	case *ast.Ident:
		prm.Type = prmType.Name
	case *ast.StarExpr:
		prm.IsPointer = true

		switch xx := prmType.X.(type) {
		case *ast.Ident:
			prm.Type = xx.Name
		case *ast.SelectorExpr:
			prm.Type = xx.X.(*ast.Ident).Name + "." + xx.Sel.Name // FIXME: replace type into Typ type
		}
	case *ast.SelectorExpr: // use exported type for parameter type
		prm.Type = prmType.X.(*ast.Ident).Name + "." + prmType.Sel.Name // FIXME: replace type into Typ type
	}

	if len(p.Names) == 0 {
		parameters = append(parameters, prm)
		return
	}

	for index, parameterName := range p.Names {
		prm.Name = parameterName.Name
		prm.IsMultipleParameters = index+1 != len(p.Names)

		parameters = append(parameters, prm)
	}

	return
}

func parseFuncDecl(pkgName string, x *ast.FuncDecl) FunctionStatement {
	var receiver Parameter
	if x.Recv != nil {
		receiver = parseParameters(x.Recv.List[0])[0]
		receiver.Pkg = pkgName
	}

	parameters := make(Parameters, 0)
	if x.Type.Params != nil {
		for _, p := range x.Type.Params.List {
			prms := parseParameters(p)

			for index, prm := range prms {
				prm.Pkg = pkgName
				prms[index] = prm
			}

			parameters = append(parameters, prms...)
		}
	}

	returns := make(Parameters, 0)
	if x.Type.Results != nil {
		for _, r := range x.Type.Results.List {
			rtrns := parseParameters(r)

			for index, rst := range rtrns {
				rst.Pkg = pkgName
				rtrns[index] = rst
			}

			returns = append(returns, rtrns...)
		}
	}

	return FunctionStatement{
		Package:    pkgName,
		Receiver:   receiver,
		Parameters: parameters,
		Name:       x.Name.Name,
		Returns:    returns,
		Body:       x.Body,
		SourceCode: SourceCode{
			Pos: x.Pos(),
			End: x.End(),
		},
	}
}
