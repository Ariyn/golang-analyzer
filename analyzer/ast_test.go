package analyzer

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"reflect"
	"strings"
	"testing"
)

func getParsedNode(stmt string) (nodes []ast.Node, err error) {
	// https://golang.hotexamples.com/examples/go.parser/-/ParseExpr/golang-parseexpr-function-examples.html
	code := "func(){" + stmt + "}"

	fset := token.NewFileSet()
	node, err := parser.ParseExprFrom(fset, "sample.go", code, 0)
	if err != nil {
		return
	}

	body := node.(ast.Node).(*ast.FuncLit).Body.List
	if len(body) == 0 {
		err = errors.New("errEmptyStmt")
		return
	}

	for _, n := range body {
		nodes = append(nodes, n.(ast.Node))
	}

	return
}

func Test_StatementTypes(t *testing.T) {
	type testCase struct {
		statement string
		typ       interface{}
	}

	testCases := []testCase{{
		statement: "a := 1",
		typ:       &ast.AssignStmt{},
	}}

	for _, tt := range testCases {
		node, err := getParsedNode(tt.statement)
		assert.NoError(t, err)
		assert.IsType(t, tt.typ, node[0])
	}
}

func getParsedParameter(parameter string) []*ast.Field {
	code := "func" + parameter + "{}"
	fset := token.NewFileSet()

	node, err := parser.ParseExprFrom(fset, "sample.go", code, 0)
	if err != nil {
		panic(err)
	}

	return node.(*ast.FuncLit).Type.Params.List
}

func getNames(idents []*ast.Ident) (names []string) {
	for _, ident := range idents {
		names = append(names, ident.Name)
	}

	return
}

func Test_ParameterTypes(t *testing.T) {
	type parameter struct {
		Type string
		Name []string
	}

	type testCase struct {
		Raw        string
		Parameters []parameter
	}

	testCases := []testCase{{
		Raw: "(a int)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "int",
		}},
	}, {
		Raw: "(a, b int)",
		Parameters: []parameter{{
			Name: []string{"a", "b"},
			Type: "int",
		}},
	}, {
		Raw: "(a int, b int)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "int",
		}, {
			Name: []string{"b"},
			Type: "int",
		}},
	}, {
		Raw: "(a int, b string, c float64, d int64)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "int",
		}, {
			Name: []string{"b"},
			Type: "string",
		}, {
			Name: []string{"c"},
			Type: "float64",
		}, {
			Name: []string{"d"},
			Type: "int64",
		}},
	}, {
		Raw: "(a *int)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "*int",
		}},
	}, {
		Raw: "(a *int, b *string)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "*int",
		}, {
			Name: []string{"b"},
			Type: "*string",
		}},
	}, {
		Raw: "(a, b *int)",
		Parameters: []parameter{{
			Name: []string{"a", "b"},
			Type: "*int",
		}},
	}, {
		Raw: "(a *log.Logger)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "*log.Logger",
		}},
	}, {
		Raw: "(a, b *log.Logger)",
		Parameters: []parameter{{
			Name: []string{"a", "b"},
			Type: "*log.Logger",
		}},
	}, {
		Raw: "(a *log.Logger, b *os.File)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "*log.Logger",
		}, {
			Name: []string{"b"},
			Type: "*os.File",
		}},
	}, {
		Raw: "(a log.Logger)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "log.Logger",
		}},
	}, {
		Raw: "(a, b log.Logger)",
		Parameters: []parameter{{
			Name: []string{"a", "b"},
			Type: "log.Logger",
		}},
	}, {
		Raw: "(a log.Logger, b os.File)",
		Parameters: []parameter{{
			Name: []string{"a"},
			Type: "log.Logger",
		}, {
			Name: []string{"b"},
			Type: "os.File",
		}},
	}}

	for _, tt := range testCases {
		prms := getParsedParameter(tt.Raw)

		for j, prm := range prms {
			assert.Equal(t, tt.Parameters[j].Name, getNames(prm.Names))

			typ := tt.Parameters[j].Type
			if strings.Contains(typ, ".") {
				var selector string
				var name string
				if string(typ[0]) == "*" {
					sel := prm.Type.(*ast.StarExpr).X.(*ast.SelectorExpr)
					selector = sel.Sel.Name
					name = sel.X.(*ast.Ident).Name

					name = "*" + name
				} else {
					sel := prm.Type.(*ast.SelectorExpr)
					selector = sel.Sel.Name
					name = sel.X.(*ast.Ident).Name
				}
				assert.Equal(t, typ, name+"."+selector)
			} else if string(typ[0]) == "*" {
				assert.Equal(t, typ, "*"+prm.Type.(*ast.StarExpr).X.(*ast.Ident).Name)
			} else {
				assert.Equal(t, typ, prm.Type.(*ast.Ident).Name)
			}
		}
	}
}

func Test_parseParameters(t *testing.T) {
	type testCase struct {
		name           string
		raw            string
		wantParameters []Parameters
	}

	tests := map[string][]testCase{
		"이름이 있고, 타입이 값인 경우": {{
			name: "파라미터가 한개인 경우",
			raw:  "(a int)",
			wantParameters: []Parameters{{{
				Name: "a",
				Type: "int",
			}}},
		}, {
			name: "파라미터가 타입이 생략된 두개인 경우",
			raw:  "(a, b string)",
			wantParameters: []Parameters{{{
				Name:                 "a",
				IsMultipleParameters: true,
				Type:                 "string",
			}, {
				Name:                 "b",
				IsMultipleParameters: false,
				Type:                 "string",
			}}},
		}, {
			name: "파라미터가 타입이 다른 두개인 경우",
			raw:  "(a int, b string)",
			wantParameters: []Parameters{{{
				Name: "a",
				Type: "int",
			}}, {{
				Name: "b",
				Type: "string",
			}}},
		}},
		"이름이 있고, 타입이 포인터인 경우": {{
			name: "파라미터가 한개인 경우",
			raw:  "(a *string)",
			wantParameters: []Parameters{{{
				Name:      "a",
				IsPointer: true,
				Type:      "string",
			}}},
		}, {
			name: "파라미터가 두개 이상인 경우",
			raw:  "(a *string, b *int)",
			wantParameters: []Parameters{{{
				Name:      "a",
				IsPointer: true,
				Type:      "string",
			}}, {{
				Name:      "b",
				IsPointer: true,
				Type:      "int",
			}}},
		}, {
			name: "파라미터 두개 이상이 동일한 타입을 가진 경우",
			raw:  "(a, b *int)",
			wantParameters: []Parameters{{{
				Name:                 "a",
				IsMultipleParameters: true,
				IsPointer:            true,
				Type:                 "int",
			}, {
				Name:                 "b",
				IsMultipleParameters: false,
				IsPointer:            true,
				Type:                 "int",
			}}},
		}},
		"export된 selector인 경우": {{
			name: "파라미터가 한개인 경우",
			raw:  "(a os.File)",
			wantParameters: []Parameters{{{
				Name: "a",
				Type: "os.File",
			}}},
		}, {
			name: "파라미터가 타입이 생략된 두개인 경우",
			raw:  "(a, b os.File)",
			wantParameters: []Parameters{{{
				Name:                 "a",
				IsMultipleParameters: true,
				Type:                 "os.File",
			}, {
				Name:                 "b",
				IsMultipleParameters: false,
				Type:                 "os.File",
			}}},
		}, {
			name: "파라미터가 타입이 다른 두개인 경우",
			raw:  "(a log.Logger, b os.File)",
			wantParameters: []Parameters{{{
				Name: "a",
				Type: "log.Logger",
			}}, {{
				Name: "b",
				Type: "os.File",
			}}},
		}},
		"export된 selector이며 포인터 타입인 경우": {{
			name: "파라미터가 한개인 경우",
			raw:  "(a *os.File)",
			wantParameters: []Parameters{{{
				Name:      "a",
				IsPointer: true,
				Type:      "os.File",
			}}},
		}, {
			name: "파라미터가 두개 이상인 경우",
			raw:  "(a *os.File, b *log.Logger)",
			wantParameters: []Parameters{{{
				Name:      "a",
				IsPointer: true,
				Type:      "os.File",
			}}, {{
				Name:      "b",
				IsPointer: true,
				Type:      "log.Logger",
			}}},
		}, {
			name: "파라미터 두개 이상이 동일한 타입을 가진 경우",
			raw:  "(a, b *os.File)",
			wantParameters: []Parameters{{{
				Name:                 "a",
				IsMultipleParameters: true,
				IsPointer:            true,
				Type:                 "os.File",
			}, {
				Name:                 "b",
				IsMultipleParameters: false,
				IsPointer:            true,
				Type:                 "os.File",
			}}},
		}},
		"이름이 없는 경우": {{
			name: "파라미터가 한개인 경우",
			raw:  "(os.File)",
			wantParameters: []Parameters{{{
				Name: "",
				Type: "os.File",
			}}},
		}, {
			name: "타입이 포인터인 경우",
			raw:  "(*os.File)",
			wantParameters: []Parameters{{{
				Name:      "",
				IsPointer: true,
				Type:      "os.File",
			}}},
		}, {
			name: "파라미터 두개 이상의 타입인 경우",
			raw:  "(os.File, log.Logger)",
			wantParameters: []Parameters{{{
				Name: "",
				Type: "os.File",
			}}, {{
				Name: "",
				Type: "log.Logger",
			}}},
		}, {
			name: "파라미터 두개 이상의 타입이 포인터인 경우",
			raw:  "(*os.File, *log.Logger)",
			wantParameters: []Parameters{{{
				Name:      "",
				IsPointer: true,
				Type:      "os.File",
			}}, {{
				Name:      "",
				IsPointer: true,
				Type:      "log.Logger",
			}}},
		}}}

	p := Parser{}
	testMethod := func(tt testCase) func(t *testing.T) {
		return func(t *testing.T) {
			prms := getParsedParameter(tt.raw)

			for i := range prms {
				if gotParameters := p.ParseParameters(prms[i]); !reflect.DeepEqual(gotParameters, tt.wantParameters[i]) {
					t.Errorf("parseParameters() = %v, want %v", gotParameters, tt.wantParameters[i])
				}
			}
		}
	}

	for testCategory, testCases := range tests {
		t.Run(testCategory, func(t *testing.T) {
			for _, tt := range testCases {
				t.Run(tt.name, testMethod(tt))
			}
		})
	}
}

func getParsedFuncDecl(rawCode string) *ast.FuncDecl {
	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, "sample.go", "package sample\n"+rawCode, 0)
	if err != nil {
		panic(err)
	}

	log.Printf("%#v", node)

	return node.Decls[0].(*ast.FuncDecl)
}

func Test_FuncDeclTypes(t *testing.T) {
	getParsedFuncDecl("func (x T) main() int { return }")
}

func Test_parseFuncDecl(t *testing.T) {

	const pkgName = "samplePkg"
	type args struct {
		pkgName string
		x       *ast.FuncDecl
	}
	tests := []struct {
		name string
		args args
		want FunctionStatement
	}{{
		name: "파라미터, 리턴값, 리시버가 없는 함수를 파싱하는 경우",
		args: args{
			pkgName: pkgName,
			x:       getParsedFuncDecl("func sampleFunc() {}"),
		},
		want: FunctionStatement{
			Package:    pkgName,
			Name:       "sampleFunc",
			Parameters: Parameters{},
			Returns:    Parameters{},
		},
	}, {
		name: "파라미터가 한개 있고 리턴값, 리시버가 없는 함수를 파싱하는 경우",
		args: args{
			pkgName: pkgName,
			x:       getParsedFuncDecl("func sampleFunc(a int) {}"),
		},
		want: FunctionStatement{
			Package: pkgName,
			Name:    "sampleFunc",
			Parameters: Parameters{{
				Pkg:  pkgName,
				Name: "a",
				Type: "int",
			}},
			Returns: Parameters{},
		},
	}, {
		name: "파라미터가 없고, 리턴값이 있고, 리시버가 없는 함수를 파싱하는 경우",
		args: args{
			pkgName: pkgName,
			x:       getParsedFuncDecl("func sampleFunc() (a int) {}"),
		},
		want: FunctionStatement{
			Package:    pkgName,
			Name:       "sampleFunc",
			Parameters: Parameters{},
			Returns: Parameters{{
				Pkg:  pkgName,
				Name: "a",
				Type: "int",
			}},
		},
	}, {
		name: "파라미터, 리턴값이 없고, 리시버가 있는 함수를 파싱하는 경우",
		args: args{
			pkgName: pkgName,
			x:       getParsedFuncDecl("func(a sampleStruct) sampleFunc() {}"),
		},
		want: FunctionStatement{
			Package:    pkgName,
			Name:       "sampleFunc",
			Parameters: Parameters{},
			Returns:    Parameters{},
			Receiver: Parameter{
				Pkg:  pkgName,
				Name: "a",
				Type: "sampleStruct",
			},
		},
	}}

	p := Parser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.SourceCode.Pos = tt.args.x.Pos()
			tt.want.SourceCode.End = tt.args.x.End()
			tt.want.Body = tt.args.x.Body

			if got := p.ParseFuncDecl(tt.args.pkgName, tt.args.x); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFuncDecl() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func getParsedImport(imp string) []*ast.ImportSpec {
	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, "sample.go", "package sample\n"+imp, 0)
	if err != nil {
		panic(err)
	}

	return node.Imports
}

func TestParseImport(t *testing.T) {
	type args struct {
		is *ast.ImportSpec
	}
	tests := []struct {
		name string
		args args
		want Import
	}{{
		name: "standard library를 임포트 했을때, 잘 파싱 되는지",
		args: args{
			is: getParsedImport(`import "log"`)[0],
		},
		want: Import{
			Name: "log",
			Path: "log",
		},
	}, {
		name: "standard library를 alias와 함께 임포트 했을때, 잘 파싱 되는지",
		args: args{
			is: getParsedImport(`import a "log"`)[0],
		},
		want: Import{
			Name:  "log",
			Alias: "a",
			Path:  "log",
		},
	}, {
		name: "특정 레포의 패키지를 임포트 했을때, 잘 파싱 되는지",
		args: args{
			is: getParsedImport(`import "github.com/ariyn/golang-analyzer/analyzer"`)[0],
		},
		want: Import{
			Name: "analyzer",
			Path: "github.com/ariyn/golang-analyzer/analyzer",
		},
	}, {
		name: "특정 레포의 패키지를 alias와 함께 임포트 했을때, 잘 파싱 되는지",
		args: args{
			is: getParsedImport(`import a "github.com/ariyn/golang-analyzer/analyzer"`)[0],
		},
		want: Import{
			Name:  "analyzer",
			Alias: "a",
			Path:  "github.com/ariyn/golang-analyzer/analyzer",
		},
	}}

	p := Parser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.ParseImport(tt.args.is); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseImport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func getParsedFunctionCall(source string) (fset *token.FileSet, ce *ast.CallExpr) {
	fset = token.NewFileSet()

	node, err := parser.ParseFile(fset, "sample.go", "package sample\n"+source, 0)
	if err != nil {
		panic(err)
	}

	var expr ast.Stmt
	for _, decl := range node.Decls {
		if e, ok := decl.(*ast.FuncDecl); ok && e.Name.Name == "main" {
			expr = e.Body.List[len(e.Body.List)-1]
		}
	}

	switch x := expr.(type) {
	case *ast.ExprStmt:
		ce = x.X.(*ast.CallExpr)
	}
	return
}

// TODO: 테스트 해야하는 엣지 케이스들
// a().b().c().d.e.f() 이처럼, 여러개의 selector가 중첩된 상태
func TestParseFuncCall(t *testing.T) {
	const pkgName = "sample"
	type args struct {
		pkgName string
	}
	tests := []struct {
		name             string
		args             args
		sourceCode       string
		wantFunctionCall FunctionCall
	}{{
		name: "파라미터가 없는 함수를 호출하는 경우",
		args: args{
			pkgName: pkgName,
		},
		sourceCode: `func main() {getA()}; func getA() {}`,
		wantFunctionCall: FunctionCall{
			Package: pkgName,
			Name:    pkgName + ".getA",
		},
	}, {
		name: "변수에 할당된 함수를 호출하는 경우",
		args: args{
			pkgName: pkgName,
		},
		sourceCode: `func main() {getA := func(){}; getA()}`,
		wantFunctionCall: FunctionCall{
			Package: pkgName,
			Name:    pkgName + ".getA",
		},
	}, {
		name: "함수 본문이 없는 메서드를 호출하는 경우",
		args: args{
			pkgName: pkgName,
		},
		sourceCode: `func main() {x.getA()}`,
		wantFunctionCall: FunctionCall{
			Package:            pkgName,
			Name:               "x.getA",
			IsImportedFunction: true,
		},
	}, {
		name: "함수 본문이 있는 메서드를 호출하는 경우",
		args: args{
			pkgName: pkgName,
		},
		sourceCode: `type x int; func main() {x.getA()}; func (_ x) getA() {return}`,
		wantFunctionCall: FunctionCall{
			Package: pkgName,
			Name:    "x.getA",
		},
	}, {
		name: "여러개의 메서드를 연속해서 호출하는 경우",
		args: args{
			pkgName: pkgName,
		},
		sourceCode: `func main() {x.getA().getB()}`,
		wantFunctionCall: FunctionCall{
			Package: pkgName,
			Name:    "x.getA",
		},
	}, {
		name: "함수에서 리턴된 메서드를 연속해서 호출하는 경우",
		args: args{
			pkgName: pkgName,
		},
		sourceCode: `func main() {getA().getB()}`,
		wantFunctionCall: FunctionCall{
			Package: pkgName,
			Name:    "getA().getB",
		},
	}}

	p := Parser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fc := getParsedFunctionCall(tt.sourceCode)
			tt.wantFunctionCall.Pos = int(fc.Pos())
			if gotFunctionCall := p.ParseFuncCall(tt.args.pkgName, fc); !reflect.DeepEqual(gotFunctionCall, tt.wantFunctionCall) {
				t.Errorf("ParseFuncCall() = %#v\nwant %#v", gotFunctionCall, tt.wantFunctionCall)
			}
		})
	}
}
