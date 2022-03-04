package analyzer

import (
	"context"
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

type SourceCode struct {
	Test, Pos token.Pos
	End       token.Pos
	Data      string
	File      *os.File
}

type FilterFunc func(info fs.FileInfo) bool

type Parser struct {
	fset            *token.FileSet
	path            string
	functionsByName map[string]FunctionStatement
	functionCalls   []FunctionCall
	importTable     map[string]Import
	filter          FilterFunc
	mode            parser.Mode
	customInspector func(ctx context.Context, p *Parser, pkgName string) (fch chan FunctionStatement, f func(node ast.Node) bool)
}

func NewParser(path string) (p Parser) {
	p = Parser{
		fset:            token.NewFileSet(),
		path:            path,
		functionsByName: make(map[string]FunctionStatement),
		functionCalls:   make([]FunctionCall, 0),
		importTable:     make(map[string]Import),
		filter: func(info fs.FileInfo) bool {
			return true
		},
	}

	return
}

func (p *Parser) SetMode(mode parser.Mode) {
	p.mode = mode
}

func (p *Parser) SetFilter(filter FilterFunc) {
	p.filter = filter
}

func (p *Parser) Parse() {
	pkgs, err := parser.ParseDir(p.fset, p.path, p.filter, p.mode)

	if err != nil {
		panic(err)
	}

	functions := make([]FunctionStatement, 0)

	for pkgName, pkg := range pkgs {
		var fch chan FunctionStatement
		var insptr func(node ast.Node) bool

		if p.customInspector != nil {
			fch, insptr = p.customInspector(context.TODO(), p, pkgName)
		} else {
			fch, insptr = inspector(context.TODO(), p, pkgName)
		}

		go func(fch chan FunctionStatement) {
			ast.Inspect(pkg, insptr)
			close(fch)
		}(fch)

		for function := range fch {
			functions = append(functions, function)
		}
	}

	for index, function := range p.functionCalls {
		identifier := function.Identifier()

		if decl, ok := p.functionsByName[identifier]; ok {
			function.FunctionDeclaration = decl
			decl.Calls = append(decl.Calls, function)

			p.functionsByName[identifier] = decl
		}

		f := p.fset.File(token.Pos(function.Pos))
		function.File = f.Name()
		function.LineNumber = f.Line(token.Pos(function.Pos))

		p.functionCalls[index] = function
	}
}

func (p *Parser) ParseImport(is *ast.ImportSpec) Import {
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

func (p *Parser) ParseFuncCall(pkgName string, ce *ast.CallExpr) (functionCall FunctionCall) {
	functionCall.Pos = int(ce.Pos())
	functionCall.Package = pkgName

	// TODO: import된 함수에 대해서, package가 잘 파싱되지 않을 가능성이 있음
	switch x := ce.Fun.(type) {
	case *ast.Ident:
		functionCall.IsImportedFunction = x.Obj == nil
		if functionCall.IsImportedFunction {
			functionCall.Name = pkgName + "." + x.Name
		} else {
			switch x2 := x.Obj.Decl.(type) {
			case *ast.FuncDecl:
				functionDecl := p.ParseFuncDecl(pkgName, x2)
				functionCall.Name = functionDecl.Identifier()
			case *ast.AssignStmt:
				functionCall.Name = pkgName + "." + x2.Lhs[0].(*ast.Ident).Name
			}
		}
	case *ast.SelectorExpr: // TODO: 현재 두번 이상 selector가 들어있다면 파싱에 오류가 생김.
		switch x2 := x.X.(type) {
		case *ast.Ident:
			functionCall.Name = x2.Name + "." + x.Sel.Name
			functionCall.IsImportedFunction = x2.Obj == nil
		case *ast.CallExpr:
			switch x3 := x2.Fun.(type) {
			case *ast.Ident:
				functionCall.Name = x3.Name + "()" + "." + x.Sel.Name // TODO: x3.Name이 아니라, x3()가 리턴하는 타입이 들어가야 함
			case *ast.SelectorExpr:
				functionCall.Name = x3.X.(*ast.Ident).Name + "." + x3.Sel.Name
			}
		case *ast.SelectorExpr: // TODO: a().b().c().d.e.f() 이처럼, 여러개의 selector가 중첩되어 있을 수 있음. recursive하게 수정 필요.
			log.Println(x2, x2.Pos(), x2.End(), p.fset.File(x2.Pos()).Name(), p.fset.File(x2.Pos()).Line(x2.Pos()))
		}
	}

	return functionCall
}

func (p *Parser) ParseFuncDecl(pkgName string, x *ast.FuncDecl) FunctionStatement {
	var receiver Parameter
	if x.Recv != nil {
		receiver = p.ParseParameters(x.Recv.List[0])[0]
		receiver.Pkg = pkgName
	}

	parameters := make(Parameters, 0)
	if x.Type.Params != nil {
		for _, parms := range x.Type.Params.List {
			prms := p.ParseParameters(parms)

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
			rtrns := p.ParseParameters(r)

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

func (p *Parser) ParseParameters(field *ast.Field) (parameters Parameters) {

	var prm Parameter

	switch prmType := field.Type.(type) {
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

	if len(field.Names) == 0 {
		parameters = append(parameters, prm)
		return
	}

	for index, parameterName := range field.Names {
		prm.Name = parameterName.Name
		prm.IsMultipleParameters = index+1 != len(field.Names)

		parameters = append(parameters, prm)
	}

	return
}

func inspector(ctx context.Context, p *Parser, pkgName string) (fch chan FunctionStatement, f func(node ast.Node) bool) {
	fch = make(chan FunctionStatement)

	f = func(node ast.Node) bool {
		// golang does not allow adding method to exported type
		switch x := node.(type) {
		case *ast.FuncDecl:
			function := p.ParseFuncDecl(pkgName, x)
			tokenFile := p.fset.File(function.SourceCode.Pos)
			if file, err := os.Open(tokenFile.Name()); err == nil {
				b, _ := ioutil.ReadAll(file)
				if b != nil {
					function.SourceCode.Data = string(b[tokenFile.Offset(x.Pos())-1 : tokenFile.Offset(x.End())])
				}
			}
			p.functionsByName[function.Identifier()] = function
		case *ast.ImportSpec:
			imp := p.ParseImport(x)
			p.importTable[imp.Caller()] = imp
		case *ast.CallExpr:
			functionCall := p.ParseFuncCall(pkgName, x)
			p.functionCalls = append(p.functionCalls, functionCall)
		}
		return true
	}

	return
}
