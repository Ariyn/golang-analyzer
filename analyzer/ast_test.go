package analyzer

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"go/ast"
	"go/parser"
	"go/token"
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
	tests := []struct {
		name           string
		raw            string
		wantParameters []Parameters
	}{{
		name: "파라미터가 한개인 경우, 잘 파싱됨",
		raw:  "(a int)",
		wantParameters: []Parameters{{{
			Name: "a",
			Type: "int",
		}}},
	}, {
		name: "파라미터가 타입이 생략된 두개인 경우, 잘 파싱됨",
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
		name: "파라미터가 타입이 다른 두개인 경우, 잘 파싱됨",
		raw:  "(a int, b string)",
		wantParameters: []Parameters{{{
			Name: "a",
			Type: "int",
		}}, {{
			Name: "b",
			Type: "string",
		}}},
	}, {
		name: "파라미터의 타입이 포인터인 경우, 잘 파싱됨",
		raw:  "(a *string)",
		wantParameters: []Parameters{{{
			Name:      "a",
			IsPointer: true,
			Type:      "string",
		}}},
	}, {
		name: "파라미터의 타입이 포인터이며 두개 이상인 경우, 잘 파싱됨",
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
		name: "파라미터의 타입이 포인터이며 두개 이상이 동일한 타입을 가진 경우, 잘 파싱됨",
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
	}, {
		name: "selector가 있고 파라미터가 한개인 경우, 잘 파싱됨",
		raw:  "(a os.File)",
		wantParameters: []Parameters{{{
			Name: "a",
			Type: "os.File",
		}}},
	}, {
		name: "파라미터가 타입이 생략된 두개인 경우, 잘 파싱됨",
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
		name: "파라미터가 타입이 다른 두개인 경우, 잘 파싱됨",
		raw:  "(a log.Logger, b os.File)",
		wantParameters: []Parameters{{{
			Name: "a",
			Type: "log.Logger",
		}}, {{
			Name: "b",
			Type: "os.File",
		}}},
	}, {
		name: "파라미터의 타입이 포인터인 경우, 잘 파싱됨",
		raw:  "(a *os.File)",
		wantParameters: []Parameters{{{
			Name:      "a",
			IsPointer: true,
			Type:      "os.File",
		}}},
	}, {
		name: "파라미터의 타입이 포인터이며 두개 이상인 경우, 잘 파싱됨",
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
		name: "파라미터의 타입이 포인터이며 두개 이상이 동일한 타입을 가진 경우, 잘 파싱됨",
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
	}, {
		name: "이름이 없는경우 잘 파싱됨",
		raw:  "(os.File)",
		wantParameters: []Parameters{{{
			Name: "",
			Type: "os.File",
		}}},
	}, {
		name: "이름이 없는경우, 타입이 포인터라도 잘 파싱됨",
		raw:  "(*os.File)",
		wantParameters: []Parameters{{{
			Name:      "",
			IsPointer: true,
			Type:      "os.File",
		}}},
	}, {
		name: "이름이 없는경우, 두개 이상의 타입이 있더라도 잘 파싱됨",
		raw:  "(os.File, log.Logger)",
		wantParameters: []Parameters{{{
			Name: "",
			Type: "os.File",
		}}, {{
			Name: "",
			Type: "log.Logger",
		}}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prms := getParsedParameter(tt.raw)

			for i := range prms {
				if gotParameters := parseParameters(prms[i]); !reflect.DeepEqual(gotParameters, tt.wantParameters[i]) {
					t.Errorf("parseParameters() = %v, want %v", gotParameters, tt.wantParameters[i])
				}
			}
		})
	}
}
