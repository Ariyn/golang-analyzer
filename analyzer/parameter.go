package analyzer

import (
	"fmt"
	"strings"
)

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

type Parameters []Parameter

func (ps Parameters) String() string {
	prmsString := make([]string, 0)

	for _, p := range ps {
		prmsString = append(prmsString, p.String())
	}

	return "(" + strings.Join(prmsString, ", ") + ")"
}
