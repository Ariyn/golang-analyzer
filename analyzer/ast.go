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
	Name string
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

type FunctionStatement struct {
	Package    string
	Receiver   Parameter
	Name       string
	Parameters Parameters
	Returns    Parameters
	Body       *ast.BlockStmt
	SourceCode SourceCode
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

var variableTable = make(map[string]interface{})

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

	for _, function := range functions {
		log.Printf("%s", function)
	}
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

		//case *ast.Ident:
		//	log.Println("  ident ", x.Name)
		//	if x.Obj != nil {
		//		log.Printf("    %#v", x.Obj)
		//	}
		//case *ast.Package:
		//	log.Println("  package ", x.Imports, x.Name, x.Files, x.Scope)
		case *ast.ImportSpec:
			if x.Name != nil {
				log.Printf("import name is %s", x.Name.Name)
			}
			log.Printf("  import spec %s", x.Path.Value)
			//case *ast.CallExpr:
			//	log.Printf("  call expr, %#v", x.Fun)
			//	if s, ok := x.Fun.(*ast.SelectorExpr); ok {
			//		log.Printf("    selector expr %#v, %#v", s.X.(*ast.Ident), s.Sel)
			//	}
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
