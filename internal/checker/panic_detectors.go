package checker

import (
	"go/ast"
	"go/token"
)

// detectNilMapWrite finds writes to a map that was declared but never allocated.
//
// `var m map[K]V` allocates nothing, and writing to a nil map panics at runtime —
// unlike reading one, which returns the zero value. This is one of the two ways
// idiomatic-looking Go panics in production, and no rule caught it.
//
// A map is considered allocated if it is ever assigned to as a whole (`m = make(...)`,
// `m = map[K]V{...}`, or a reassignment from elsewhere), so the detector reports
// only declarations that are never given a value.
func detectNilMapWrite(file *ast.File, fset *token.FileSet, filePath string) []AntiPattern {
	declared := map[string]bool{}
	assigned := map[string]bool{}

	// Pass one: map declarations with no value, and every whole-map assignment.
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ValueSpec:
			if _, ok := node.Type.(*ast.MapType); ok && len(node.Values) == 0 {
				for _, name := range node.Names {
					declared[name.Name] = true
				}
			}
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					assigned[ident.Name] = true
				}
			}
		}
		return true
	})

	var results []AntiPattern
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			index, ok := lhs.(*ast.IndexExpr)
			if !ok {
				continue
			}
			ident, ok := index.X.(*ast.Ident)
			if !ok || !declared[ident.Name] || assigned[ident.Name] {
				continue
			}
			results = append(results, AntiPattern{
				Name:     "nil_map_write",
				Category: "code",
				Severity: "error",
				File:     filePath,
				Line:     fset.Position(index.Pos()).Line,
				Message:  "write to map " + ident.Name + " which is declared but never allocated — writing to a nil map panics; allocate it with make() first",
			})
		}
		return true
	})
	return results
}

// detectTypeAssertionWithoutOK finds single-value type assertions.
//
// `v.(T)` panics when v holds a different type. The two-value form and a type
// switch both make the failure explicit, and are what the language provides for
// values whose type is not guaranteed.
//
// Reported as a warning rather than an error: asserting without the comma-ok form
// is legitimate when the type is genuinely known, so this needs human judgement
// rather than a build failure.
func detectTypeAssertionWithoutOK(file *ast.File, fset *token.FileSet, filePath string) []AntiPattern {
	// Assertions on the right of a two-value assignment are the safe form.
	safe := map[token.Pos]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
			return true
		}
		if assertion, ok := assign.Rhs[0].(*ast.TypeAssertExpr); ok {
			safe[assertion.Pos()] = true
		}
		return true
	})

	var results []AntiPattern
	ast.Inspect(file, func(n ast.Node) bool {
		assertion, ok := n.(*ast.TypeAssertExpr)
		if !ok {
			return true
		}
		// A nil Type is the `v.(type)` guard of a type switch, which is safe.
		if assertion.Type == nil || safe[assertion.Pos()] {
			return true
		}
		results = append(results, AntiPattern{
			Name:     "type_assertion_without_ok",
			Category: "code",
			Severity: "warning",
			File:     filePath,
			Line:     fset.Position(assertion.Pos()).Line,
			Message:  "type assertion without the comma-ok form panics if the type differs — use `v, ok := x.(T)` or a type switch",
		})
		return true
	})
	return results
}
