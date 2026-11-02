package radar

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// goReferenceText hides comments and regexp arguments from built-in matching.
// It follows parameters forwarded through local functions without executing code.
// Byte offsets and newlines are preserved for evidence locations. If parsing
// fails completely, the original input is retained for best-effort matching.
func goReferenceText(data []byte) []byte {
	positions := token.NewFileSet()
	file, _ := parser.ParseFile(positions, "", data, parser.ParseComments)
	if file == nil {
		return data
	}
	imports := map[string]bool{}
	for _, imp := range file.Imports {
		if name, err := strconv.Unquote(imp.Path.Value); err == nil && name == "regexp" {
			alias := "regexp"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			imports[alias] = true
		}
	}
	params := map[*ast.Object][]*ast.Object{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		for _, field := range fn.Type.Params.List {
			if len(field.Names) == 0 {
				params[fn.Name.Obj] = append(params[fn.Name.Obj], nil)
			}
			for _, name := range field.Names {
				params[fn.Name.Obj] = append(params[fn.Name.Obj], name.Obj)
			}
		}
	}
	isRegexp := func(call *ast.CallExpr) bool {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Obj != nil || !imports[pkg.Name] {
			return false
		}
		switch sel.Sel.Name {
		case "Compile", "CompilePOSIX", "MustCompile", "MustCompilePOSIX", "Match", "MatchReader", "MatchString":
			return true
		}
		return false
	}
	var calls []*ast.CallExpr
	var queue []*ast.Object
	edges := map[*ast.Object][]*ast.Object{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		calls = append(calls, call)
		if isRegexp(call) && len(call.Args) > 0 {
			if arg, ok := call.Args[0].(*ast.Ident); ok && arg.Obj != nil {
				queue = append(queue, arg.Obj)
			}
		}
		if callee, ok := call.Fun.(*ast.Ident); ok {
			for i, param := range params[callee.Obj] {
				if i >= len(call.Args) {
					break
				}
				if arg, ok := call.Args[i].(*ast.Ident); ok && arg.Obj != nil && param != nil {
					edges[param] = append(edges[param], arg.Obj)
				}
			}
		}
		return true
	})
	regexpParams := map[*ast.Object]bool{}
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]
		if !regexpParams[obj] {
			regexpParams[obj] = true
			queue = append(queue, edges[obj]...)
		}
	}
	text := bytes.Clone(data)
	hide := func(node ast.Node) {
		start, end := positions.Position(node.Pos()).Offset, positions.Position(node.End()).Offset
		for i := start; i < end && i < len(text); i++ {
			if text[i] != '\n' && text[i] != '\r' {
				text[i] = ' '
			}
		}
	}
	for _, comment := range file.Comments {
		hide(comment)
	}
	for _, call := range calls {
		if isRegexp(call) && len(call.Args) > 0 {
			hide(call.Args[0])
		}
		if callee, ok := call.Fun.(*ast.Ident); ok {
			for i, param := range params[callee.Obj] {
				if i < len(call.Args) && regexpParams[param] {
					hide(call.Args[i])
				}
			}
		}
	}
	return text
}
