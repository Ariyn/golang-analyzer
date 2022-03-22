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
	"sync"
)

const (
	breakLine = 1199
	breakFile = "sample/echo/binder_test.go"
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
	inspector       func(ctx context.Context, p *Parser, pkgName string) (fch chan FunctionStatement, f func(node ast.Node) bool)
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
		inspector: inspector,
	}

	return
}

func (p *Parser) SetMode(mode parser.Mode) {
	p.mode = mode
}

func (p *Parser) SetFilter(filter FilterFunc) {
	p.filter = filter
}

func (p Parser) FuncCalls() []FunctionCall {
	return p.functionCalls
}

func (p Parser) Functions() (functions []FunctionStatement) {
	for _, f := range p.functionsByName {
		functions = append(functions, f)
	}
	return
}

func (p *Parser) ParseFile(source string) {
	pkgs, err := parser.ParseFile(p.fset, p.path, source, p.mode)

	if err != nil {
		panic(err)
	}

	functions := make([]FunctionStatement, 0)

	fch, insptr := p.inspector(context.TODO(), p, pkgs.Name.Name)

	go func(fch chan FunctionStatement) {
		ast.Inspect(pkgs, insptr)
		close(fch)
	}(fch)

	for function := range fch {
		functions = append(functions, function)
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

func (p *Parser) Parse() {
	pkgs, err := parser.ParseDir(p.fset, p.path, p.filter, p.mode)

	if err != nil {
		panic(err)
	}

	functions := make([]FunctionStatement, 0)

	for pkgName, pkg := range pkgs {

		fch, insptr := p.inspector(context.TODO(), p, pkgName)

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

func (p *Parser) ParseArgs(pkgName string, args []ast.Expr) (parms Parameters) {
	for _, a := range args {
		//pos := p.fset.Position(a.Pos())

		var prm Parameter

		prm.IsArgument = true

		prm.Name = p.ParseType(pkgName, a).String()

		//switch x := a.(type) {
		//case *ast.Ident:
		//	prm.Name = x.Name
		//	//log.Printf("%#v, %#v", x, x.Obj.Decl)
		//case *ast.BasicLit:
		//	prm.Name = x.Value
		//case *ast.CallExpr:
		//	prm.Name = p.ParseFuncCall(pkgName, x).String()
		//case
		//default:
		//	log.Printf("unknown case %s:%d (%#v)", pos.Filename, pos.Line, x)
		//}

		parms = append(parms, prm)
	}

	return
}

// TODO: p.ParseExpr( expr ast.Expr)이 필요한거 아닐까? 계속 recursive하게 호출해서 내가 원하는 타입을 리턴받을 수 있도록 (리턴도 interface로 받아서 타입 체크 해야할 듯)

func (p *Parser) ParseFuncCall(pkgName string, ce *ast.CallExpr) (functionCall FunctionCall) {
	pos := p.fset.Position(ce.Pos())

	functionCall.Pos = int(ce.Pos())
	functionCall.Package = pkgName

	functionCall.Parameters = p.ParseArgs(pkgName, ce.Args)

	// TODO: import된 함수에 대해서, package가 잘 파싱되지 않을 가능성이 있음
	switch x := ce.Fun.(type) {
	case *ast.Ident:
		functionCall.Name = pkgName + "." + x.Name
	case *ast.SelectorExpr: // sample/echo/response.go:87 &ast.SelectorExpr
		s := p.ParseSelector(pkgName, x)
		functionCall.Name = s.String()
	case *ast.ParenExpr: // sample/echo/bind_test.go:280 *ast.ParenExpr
		//log.Printf("%s:%d %#v", pos.Filename, pos.Line, x.X)
		functionCall.Name = "(" + p.ParseType(pkgName, x.X).String() + ")"
	case *ast.CallExpr: // sample/echo/ip.go:101 *ast.CallExpr
		functionCall.Name = p.ParseFuncCall(pkgName, x).String()
		//log.Printf("%s:%d %#v %#v", pos.Filename, pos.Line, x.Fun, x.Args)
	case *ast.ArrayType: // sample/echo/context_test.go:680 *ast.ArrayType
		//log.Printf("%s:%d %#v %#v", pos.Filename, pos.Line, x.Elt, x.Len)
		size := ""
		if x.Len != nil {
			log.Printf("%#v", x.Len)
		}

		functionCall.Name = "[" + size + "]" + p.ParseType(pkgName, x.Elt).String()
	case *ast.IndexExpr: // sample/echo/echo.go:961 *ast.IndexExpr
		functionCall.Name = p.ParseArray(pkgName, x).String()
	case *ast.FuncLit: // sample/echo/echo_test.go:1423 *ast.FuncLit
		parameters, results := p.ParseFuncType(pkgName, x.Type)
		functionCall.Name = "func" + parameters.String() + results.String()
	case *ast.InterfaceType: // sample/echo/echo_test.go:1068 *ast.InterfaceType
		//log.Printf("%s:%d %#v", pos.Filename, pos.Line, )
		//functions := make([]string, 0)
		for _, f := range x.Methods.List {
			log.Printf("%#v %#v %#v", f.Tag, f.Type, f.Names)
		}
		functionCall.Name = fmt.Sprintf("interface{%#v}", x.Methods)
	default:
		log.Printf("unknown case %s:%d (%#v)", pos.Filename, pos.Line, x)
	}

	return functionCall
}

func (p *Parser) ParseSelector(pkgName string, x *ast.SelectorExpr) (s Selector) {
	pos := p.fset.Position(x.X.Pos())

	s.Parent = x.Sel.Name
	switch x2 := x.X.(type) {
	case *ast.Ident:
		s.Field = Variable{Name: x2.Name}
		s.ImportedSelector = x2.Obj == nil
	case *ast.CallExpr:
		s.Field = p.ParseFuncCall(pkgName, x2)
	case *ast.SelectorExpr: // TODO: a().b().c().d.e.f() 이처럼, 여러개의 selector가 중첩되어 있을 수 있음. recursive하게 수정 필요.
		//log.Println(x2, x2.Pos(), x2.End(), p.fset.File(x2.Pos()).Name(), p.fset.File(x2.Pos()).Line(x2.Pos()))
		s.Field = p.ParseSelector(pkgName, x2)
	case *ast.TypeAssertExpr:
		typ := p.ParseType(pkgName, x2.Type)
		s.Field = typ
	case *ast.UnaryExpr: // sample/echo/bind_test.go:280 *ast.UnaryExpr
		log.Printf("%#v, %#v", x2.Op, x2.X)
		log.Println(pos.Filename, pos.Line, s.Parent)
	case *ast.IndexExpr: // sample/echo/router_test.go:2466 *ast.IndexExpr
		//log.Printf("%#v, %#v", x2.X, x2.Index)
		//log.Println(pos.Filename, pos.Line, s.Parent)
		s.Field = p.ParseArray(pkgName, x2)
	case *ast.ParenExpr: // sample/echo/bind_test.go:280 *ast.ParenExpr
		//log.Printf("%s:%d %#v", pos.Filename, pos.Line, x2.X)
		s.Field = p.ParseType(pkgName, x2.X)
	default:
		log.Printf("unknown case %s:%d (%#v)", pos.Filename, pos.Line, x2)
	}

	return
}

func (p *Parser) ParseArray(pkgName string, x *ast.IndexExpr) (a Array) {
	a.Name = p.ParseType(pkgName, x.X).String()
	a.Index = p.ParseType(pkgName, x.Index).String()

	return
}

func (p *Parser) ParseType(pkgName string, x ast.Expr) (t Type) {
	if x == nil {
		return
	}
	pos := p.fset.Position(x.Pos())

	switch x2 := x.(type) {
	case *ast.StarExpr:
		t = p.ParseType(pkgName, x2.X)
	case *ast.Ident:
		//log.Println(x2.Name)
		t.Name = x2.Name
	case *ast.SelectorExpr:
		s := p.ParseSelector(pkgName, x2)
		t.Name = s.String()
	case *ast.BinaryExpr: // sample/echo/router_test.go:2469 *ast.Binary
		//log.Printf("binary %s:%d %#v %#v %#v", pos.Filename, pos.Line, x2.X, x2.Op.String(), x2.Y)
		t.Name = fmt.Sprintf("%s %s %s", p.ParseType(pkgName, x2.X), x2.Op, p.ParseType(pkgName, x2.Y))
	case *ast.BasicLit: // sample/echo/router_test.go:2469 *ast.BasicLit
	case *ast.UnaryExpr: // sample/echo/bind_test.go:280 *ast.UnaryExpr
		t.Name = x2.Op.String() + p.ParseType(pkgName, x2.X).String()
	case *ast.CompositeLit: // sample/echo/bind_test.go:280 *ast.CompositeLit
		//for _, b := range x2.Elts {
		//	log.Println(p.ParseType(pkgName, b))
		//}
		t.Name = p.ParseType(pkgName, x2.Type).String() + "{}"
	case *ast.CallExpr:
		t.Name = p.ParseFuncCall(pkgName, x2).String()
	case *ast.IndexExpr: // sample/echo/echo.go:961 *ast.IndexExpr
		t.Name = p.ParseArray(pkgName, x2).String()
	case *ast.ArrayType: // sample/echo/context_test.go:680 *ast.ArrayType
		//log.Printf("%s:%d %#v %#v", pos.Filename, pos.Line, x.Elt, x.Len)
		size := ""
		if x2.Len != nil {
			log.Printf("%#v", x2.Len)
		}

		t.Name = "[" + size + "]" + p.ParseType(pkgName, x2.Elt).String()
	case *ast.FuncLit: // sample/echo/echo_test.go:1423 *ast.FuncLit
		//log.Printf("%s:%d %#v, %#v", pos.Filename, pos.Line, x.Type, x.Body)

		parameters := make(Parameters, 0)

		if x2.Type.Params != nil {
			for _, parms := range x2.Type.Params.List {
				prms := p.ParseParameters(parms)

				for index, prm := range prms {
					prm.Pkg = pkgName
					prms[index] = prm
				}

				parameters = append(parameters, prms...)
			}
		}

		results := make(Parameters, 0)
		if x2.Type.Results != nil {
			for _, parms := range x2.Type.Results.List {
				prms := p.ParseParameters(parms)

				for index, prm := range prms {
					prm.Pkg = pkgName
					prms[index] = prm
				}

				results = append(results, prms...)
			}
		}

		t.Name = "func" + parameters.String() + results.String()
	case *ast.MapType: // sample/echo/binder_test.go:63 *ast.MapType
		t.Name = "map[" + p.ParseType(pkgName, x2.Key).String() + "]" + p.ParseType(pkgName, x2.Value).String()
	case *ast.ChanType: // sample/echo/context_test.go:173 *ast.ChanType
		dir := x2.Dir
		t.Name = "chan"

		if dir == ast.RECV {
			t.Name = "<-" + t.Name
		} else {
			t.Name = t.Name + "<-"
		}

		t.Name += p.ParseType(pkgName, x2.Value).String()
	case *ast.StructType: // sample/echo/context_test.go:720 *ast.StructType

		fields := make(Parameters, 0)
		if x2.Fields != nil {
			for _, parms := range x2.Fields.List {
				prms := p.ParseParameters(parms)

				for index, prm := range prms {
					prm.Pkg = pkgName
					prms[index] = prm
				}

				fields = append(fields, prms...)
			}
		}

		t.Name = "struct {" + fields.String() + "}"
	case *ast.SliceExpr: // sample/echo/context.go:285 *ast.SliceExpr
		low := p.ParseType(pkgName, x2.Low).String()
		high := p.ParseType(pkgName, x2.High).String()
		max := p.ParseType(pkgName, x2.Max).String()

		index := low + ":" + high
		if x2.Slice3 {
			index += ":" + max
		}
		t.Name = p.ParseType(pkgName, x2.X).String() + "[" + index + "]"
	case *ast.InterfaceType: // sample/echo/echo_test.go:1083 *ast.InterfaceType
	case *ast.FuncType:
		parameters, returns := p.ParseFuncType(pkgName, x2)
		t.Name = "func" + parameters.String() + returns.String()
	default:
		log.Printf("unknown %s:%d %#v", pos.Filename, pos.Line, x2)
	}

	return
}

func (p *Parser) ParseFuncDecl(pkgName string, x *ast.FuncDecl) FunctionStatement {
	var receiver Parameter
	if x.Recv != nil {
		receiver = p.ParseParameters(x.Recv.List[0])[0]
		receiver.Pkg = pkgName
	}

	parameters, returns := p.ParseFuncType(pkgName, x.Type)

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

func (p *Parser) ParseFuncType(pkgName string, typ *ast.FuncType) (parameters, returns Parameters) {
	//parameters := make(Parameters, 0)
	if typ.Params != nil {
		for _, parms := range typ.Params.List {
			prms := p.ParseParameters(parms)

			for index, prm := range prms {
				prm.Pkg = pkgName
				prms[index] = prm
			}

			parameters = append(parameters, prms...)
		}
	}

	//returns := make(Parameters, 0)
	if typ.Results != nil {
		for _, r := range typ.Results.List {
			rtrns := p.ParseParameters(r)

			for index, rst := range rtrns {
				rst.Pkg = pkgName
				rtrns[index] = rst
			}

			returns = append(returns, rtrns...)
		}
	}

	return
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

var Nodes []ast.Node
var mu sync.Mutex

func inspector(ctx context.Context, p *Parser, pkgName string) (fch chan FunctionStatement, f func(node ast.Node) bool) {
	fch = make(chan FunctionStatement)

	Nodes = make([]ast.Node, 0)

	f = func(node ast.Node) bool {
		// golang does not allow adding method to exported type

		if node == nil {
			return false
		}

		mu.Lock()
		Nodes = append(Nodes, node)
		mu.Unlock()

		//pos := p.fset.Position(node.Pos())
		//file := p.fset.File(node.Pos())
		//if pos.Line == breakLine && file.Name() == breakFile {
		//	log.Printf("%s:%d %#v", file.Name(), pos.Line, node)
		//}

		switch x := node.(type) {
		case *ast.FuncType:
			//log.Printf("%#v", x)
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
