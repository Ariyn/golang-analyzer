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

	functions := []FunctionStatement{}

	for pkgName, pkg := range pkgs {
		fch, insptr := inspector(context.TODO(), pkgName, fset)
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

		if decl, ok := functionsByName[identifier]; ok {
			function.FunctionDeclaration = decl
			decl.Calls = append(decl.Calls, function)

			functionsByName[identifier] = decl
		}

		f := fset.File(token.Pos(function.Pos))
		function.File = f.Name()
		function.LineNumber = f.Line(token.Pos(function.Pos))

		//if !ok && !function.IsImportedFunction {
		//	panic(fmt.Errorf("not declared function called (%s)", function.Identifier()))
		//}

		functionCalls[index] = function
	}

	log.Println(len(functionCalls))
	//for _, function := range functionCalls {
	//	if !function.IsImportedFunction {
	//		log.Println(function.File, function.Identifier(), function.LineNumber, "delcation", function.FunctionDeclaration.SourceCode.Data)
	//	}
	//}
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
			switch x2 := x.Obj.Decl.(type) {
			case *ast.FuncDecl:
				functionDecl := parseFuncDecl(pkgName, x2)
				functionCall.Name = functionDecl.Identifier()
			case *ast.AssignStmt:
				// TODO: _ = routeFunc(c)의 경우, x.Obj.Decl의 타일이 *ast.assignStmt로 분류된다.
				log.Println(x.Obj.Decl, x.Pos(), x.End(), fset.File(x.Pos()).Name(), fset.File(x.Pos()).Line(x.Pos()))
			}
		} else {
			functionCall.Name = pkgName + "." + x.Name
		}
	case *ast.SelectorExpr:
		switch x2 := x.X.(type) {
		case *ast.Ident:
			functionCall.Name = x2.Name + "." + x.Sel.Name
			functionCall.IsImportedFunction = x2.Obj == nil
		case *ast.SelectorExpr: // TODO: a().b().c().d.e.f() 이처럼, 여러개의 selector가 중첩되어 있을 수 있음. recursive하게 수정 필요.
			log.Println(x2, x2.Pos(), x2.End(), fset.File(x2.Pos()).Name(), fset.File(x2.Pos()).Line(x2.Pos()))
		}
	}

	return functionCall
}

func inspector(ctx context.Context, pkgName string, fset *token.FileSet) (fch chan FunctionStatement, f func(node ast.Node) bool) {
	fch = make(chan FunctionStatement)

	f = func(node ast.Node) bool {
		// golang does not allow adding method to exported type
		switch x := node.(type) {
		case *ast.FuncDecl:
			function := parseFuncDecl(pkgName, x)
			tokenFile := fset.File(function.SourceCode.Pos)
			if file, err := os.Open(tokenFile.Name()); err == nil {
				b, _ := ioutil.ReadAll(file)
				if b != nil {
					function.SourceCode.Data = string(b[tokenFile.Offset(x.Pos())-1 : tokenFile.Offset(x.End())])
				}
			}
			functionsByName[function.Identifier()] = function
		case *ast.ImportSpec:
			imp := ParseImport(x)
			ImportTable[imp.Caller()] = imp
		case *ast.CallExpr:
			functionCall := ParseFuncCall(pkgName, x)
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
