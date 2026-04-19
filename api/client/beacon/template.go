package beacon

import "strings"

// idReplacer returns a function that performs a simple {{.Id}} substitution in the
// given path template. This replaces the previous text/template approach which used
// reflection and allocated multiple intermediate buffers on every call.
func idReplacer(pathTemplate string) func(StateOrBlockId) string {
	return func(id StateOrBlockId) string {
		return strings.Replace(pathTemplate, "{{.Id}}", string(id), 1)
	}
}

var getBlockRootTpl = idReplacer(getBlockRootPath)
var getForkTpl = idReplacer(getForkForStatePath)
