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

type Structure struct {
	PkgName    string
	Name       string
	Parameters Parameters
	methods    []FunctionStatement
}

//Class07 : equals()
//Class07 : Object[] elementData
//Class01 : size()
func (s Structure) Mermaid() (mStr string) {
	mStrs := make([]string, 0)
	for _, p := range s.Parameters {
		mStrs = append(mStrs, fmt.Sprintf("%s : %s %s", s.Name, p.Type, p.Name))
	}

	return "class " + s.Name + "\n" + strings.Join(mStrs, "\n")
}

func (s Structure) Methods() []FunctionStatement {
	return s.methods
}

type Parser struct {
	fset            *token.FileSet
	path            string
	functionsByName map[string]FunctionStatement // TODO: what if 2 packages has same name? like context.
	functionCalls   []FunctionCall
	importTable     map[string]Import
	filter          FilterFunc
	structureTypes  map[string]Structure
	mode            parser.Mode
	inspector       func(ctx context.Context, p *Parser, path string, pkgName string) (fch chan FunctionStatement, f func(node ast.Node) bool)
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
		structureTypes: make(map[string]Structure),
		inspector:      inspector,
	}

	return
}

func (p *Parser) LineInfo(pos token.Pos) (fileName string, lineNumber int) {
	f := p.fset.File(pos)
	fileName = f.Name()

	lineNumber = f.Line(pos)
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

func (p Parser) Structures() []Structure {
	ss := make([]Structure, 0)
	for _, s := range p.structureTypes {
		ss = append(ss, s)
	}

	return ss
}

func (p Parser) Function(name string) (function FunctionStatement, ok bool) {
	function, ok = p.functionsByName[name]
	return
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

	fch, insptr := p.inspector(context.TODO(), p, p.path, pkgs.Name.Name)

	go func(fch chan FunctionStatement) {
		ast.Inspect(pkgs, insptr)
		close(fch)
	}(fch)

	for function := range fch {
		functions = append(functions, function)
	}

	// e.Match([]string{"GET", "POST"}, "/test", server.Test)
	// 이런식으로 함수 자체가 넘어 갔을때, functionCalls에는 집계되지 않음.
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
	// TODO: recursive parse?
	pkgs, err := parser.ParseDir(p.fset, p.path, p.filter, p.mode)

	if err != nil {
		panic(err)
	}

	functions := make([]FunctionStatement, 0)

	path := p.path
	for pkgName, pkg := range pkgs {
		fch, insptr := p.inspector(context.TODO(), p, path, pkgName)

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
			//log.Println(function, function.Identifier(), function.Parent, decl)
			function.FunctionDeclaration = decl
			decl.Calls = append(decl.Calls, function)

			p.functionsByName[identifier] = decl
		}

		f := p.fset.File(token.Pos(function.Pos))
		function.File = f.Name()
		function.LineNumber = f.Line(token.Pos(function.Pos))

		p.functionCalls[index] = function
	}

	for _, f := range p.functionsByName {
		id := f.Receiver.Pkg + "." + f.Receiver.Type
		if strct, ok := p.structureTypes[id]; ok {
			strct.methods = append(strct.methods, f)
			p.structureTypes[id] = strct
		}
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

	if len(functionDeclarations) != 0 {
		fd := functionDeclarations[len(functionDeclarations)-1]
		//log.Println(functionCall.Name, fd)

		functionCall.Parent = fd
		//} else {
		//	log.Println(functionCall.Name, "no function declarations")
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

	if fc, ok := p.functionsByName[pkgName+"."+x.Sel.Name+"()"]; ok {
		s.ParentType = fc.Returns.String()
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
	//pos := p.fset.Position(x.Pos())

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
		t.Name = x2.Value
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
		// ret.Items.([]model.Subscriber)
		//log.Printf("unknown %s:%d %#v", pos.Filename, pos.Line, x2)
	}

	return
}

var functionDeclarations []*FunctionStatement

func (p *Parser) ParseFuncDecl(path, pkgName string, x *ast.FuncDecl) (fs FunctionStatement) {
	fs = FunctionStatement{
		Package: pkgName,
		Path:    path,
		Name:    x.Name.Name,
		Body:    x.Body,
		SourceCode: SourceCode{
			Pos: x.Pos(),
			End: x.End(),
		},
		Node: x,
	}

	var receiver Parameter
	if x.Recv != nil {
		receiver = p.ParseParameters(x.Recv.List[0])[0]
		receiver.Pkg = pkgName

		fs.Receiver = receiver
	}

	fs.Parameters, fs.Returns = p.ParseFuncType(pkgName, x.Type)

	return fs
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
	case *ast.ArrayType: // sample/echo/context_test.go:680 *ast.ArrayType
		//log.Printf("%s:%d %#v %#v", pos.Filename, pos.Line, x.Elt, x.Len)
		size := ""
		if prmType.Len != nil {
			log.Printf("%#v", prmType.Len)
		}
		//log.Printf("%#v", prmType)
		prm.Type = "[" + size + "]" + p.ParseType("", prmType.Elt).String()
	case *ast.MapType:
		prm.Type = "map[" + p.ParseType("", prmType.Key).String() + "]" + p.ParseType("", prmType.Value).String()
	case *ast.ChanType:
		if prmType.Dir == ast.RECV {
			prm.Type = "chan<- "
		} else if prmType.Dir == ast.SEND {
			prm.Type = "<-chan "
		} else {
			prm.Type = "chan "
		}

		prm.Type += p.ParseType("", prmType.Value).String()
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

var currentNode ast.Node

type SymbolTable struct {
	Parent   *SymbolTable
	symbols  map[string]string // TODO: string into type
	Children []*SymbolTable
}

var symbolStack = make([]ast.Node, 0)

func stackPush(stack *[]ast.Node, value ast.Node) {
	*stack = append(*stack, value)
}

func stackPop(stack *[]ast.Node) (value ast.Node) {
	value = (*stack)[len(*stack)-1]
	*stack = (*stack)[:len(*stack)-1]
	return
}

func inspector(ctx context.Context, p *Parser, path string, pkgName string) (fch chan FunctionStatement, f func(node ast.Node) bool) {
	fch = make(chan FunctionStatement)

	Nodes = make([]ast.Node, 0)
	f = func(node ast.Node) bool {
		// golang does not allow adding method to exported type
		if node == nil {
			n := stackPop(&symbolStack)
			if len(functionDeclarations) != 0 && n == functionDeclarations[len(functionDeclarations)-1].Node {
				functionDeclarations = functionDeclarations[:len(functionDeclarations)-1]
			}
			return false
		}
		stackPush(&symbolStack, node)

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
			_ = stackPop(&symbolStack)
			return false
		case *ast.FuncDecl:
			function := p.ParseFuncDecl(path, pkgName, x)
			tokenFile := p.fset.File(function.SourceCode.Pos)
			if file, err := os.Open(tokenFile.Name()); err == nil {
				b, _ := ioutil.ReadAll(file)
				if b != nil {
					function.SourceCode.Data = string(b[tokenFile.Offset(x.Pos())-1 : tokenFile.Offset(x.End())])
				}

				function.Path = file.Name()
			}
			p.functionsByName[function.Identifier()] = function

			functionDeclarations = append(functionDeclarations, &function)
		case *ast.ImportSpec:
			imp := p.ParseImport(x)
			p.importTable[imp.Caller()] = imp
		case *ast.CallExpr:
			functionCall := p.ParseFuncCall(pkgName, x)
			p.functionCalls = append(p.functionCalls, functionCall)
			_ = stackPop(&symbolStack)
			return false
		case *ast.TypeSpec:
			if x2, ok := x.Type.(*ast.StructType); ok {
				strct := p.parseStruct(pkgName, x.Name.Name, x2)
				p.structureTypes[strct.PkgName+"."+strct.Name] = strct
			}
		}
		return true
	}

	return
}

func (p Parser) parseStruct(pkgName, structName string, stct *ast.StructType) (s Structure) {
	s.Parameters = make(Parameters, 0)
	s.PkgName = pkgName
	s.Name = structName
	s.methods = make([]FunctionStatement, 0)

	for _, field := range stct.Fields.List {
		parameter := p.ParseParameters(field)
		s.Parameters = append(s.Parameters, parameter...)
	}

	return s
}
