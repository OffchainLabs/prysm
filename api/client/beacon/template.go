package beacon

import (
	"bytes"
	"fmt"
	"text/template"
)

type templateFn func(StateOrBlockId) string

var getBlockRootTpl templateFn
var getForkTpl templateFn

func init() {
	// idTemplate is used to create template functions that can interpolate StateOrBlockId values.
	idTemplate := func(ts string) func(StateOrBlockId) string {
		t := template.Must(template.New("").Parse(ts))
		f := func(id StateOrBlockId) string {
			b := bytes.NewBuffer(nil)
			err := t.Execute(b, struct{ Id string }{Id: string(id)})
			if err != nil {
				panic(fmt.Sprintf("invalid idTemplate: %s", ts))
			}
			return b.String()
		}
		// run the template to ensure that it is valid
		// this should happen load time (using package scoped vars) to ensure runtime errors aren't possible
		_ = f(IdGenesis)
		return f
	}

	getBlockRootTpl = idTemplate(getBlockRootPath)
	getForkTpl = idTemplate(getForkForStatePath)
}
