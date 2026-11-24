// SPDX-License-Identifier: GPL-2.0-only

package archtest

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rm4n0s/errors"
)

// The Tags of the error-tag scan's errors.
const (
	TagExceptionsUnreadable = "ErrortagsExceptionsUnreadable"
	TagExceptionMalformed   = "ErrortagsExceptionMalformed"
)

// ExceptionsFile is the file, from the module root, that lists the Tags that need no test, one
// per line as "- `Tag`: reason". It is the only exception list (CLAUDE.md "Definition of done").
const ExceptionsFile = "docs/ERROR-TAGS-UNTESTED.md"

// constructors are the functions of github.com/rm4n0s/errors whose first argument is a Tag.
var constructors = map[string]bool{"New": true, "NewErr": true, "NewErrf": true}

// routeLiteral matches a string that is a route, or the end of one: frames of letters, digits,
// "_", "*", "." and "->", then a "." and the Tag, which starts with a capital.
var routeLiteral = regexp.MustCompile(`^[A-Za-z0-9_.*>-]*\.([A-Z][A-Za-z0-9]*)$`)

// exceptionLine matches a line of the exceptions file that names a Tag.
var exceptionLine = regexp.MustCompile("^- `([A-Za-z0-9]+)`(.*)$")

// TagUse is one place where the non-test code creates an error with a Tag.
type TagUse struct {
	// Tag is the error's Tag.
	Tag string
	// File is the file's path from the module root, with slashes.
	File string
	// Line is the line of the constructor call.
	Line int
	// Func is the function that calls the constructor: "Func" or "Type.Method".
	Func string
}

// TagProblem is a constructor call that breaks a rule about Tags, whatever its tests are.
type TagProblem struct {
	// File is the file's path from the module root, with slashes.
	File string
	// Line is the line of the constructor call.
	Line int
	// Message says what is wrong.
	Message string
}

// TagReport is the result of ScanTags.
type TagReport struct {
	// Uses are the Tags the non-test code creates, sorted by Tag, then file and line.
	Uses []TagUse
	// Problems are the constructor calls whose Tag is not a string literal or a Tag constant, and
	// the calls outside a function.
	Problems []TagProblem
	// asserted holds the Tags that a test asserts.
	asserted map[string]bool
	// excepted maps the Tags of the exceptions file to their reasons.
	excepted map[string]string
}

// Asserted says whether a test asserts the Tag.
func (r *TagReport) Asserted(tag string) bool {
	return r.asserted[tag]
}

// Excepted returns the reason the exceptions file gives for a Tag, and whether it lists it.
func (r *TagReport) Excepted(tag string) (string, bool) {
	reason, ok := r.excepted[tag]
	return reason, ok
}

// Untested returns the uses whose Tag no test asserts. The Tags of the exceptions file are left
// out unless all is true.
func (r *TagReport) Untested(all bool) []TagUse {
	var out []TagUse
	for _, use := range r.Uses {
		if r.asserted[use.Tag] {
			continue
		}
		if _, ok := r.excepted[use.Tag]; ok && !all {
			continue
		}
		out = append(out, use)
	}
	return out
}

// ScanTags finds every error Tag that the non-test code of the module at root creates (the first
// argument of New, NewErr and NewErrf of github.com/rm4n0s/errors) and which of them a test
// asserts (docs/SPEC.md §4.2 rule 7).
//
// A Tag must be a string literal or a string constant whose name starts with "Tag" or "tag";
// anything else is a problem. A Tag counts as asserted when a _test.go file has one of:
//
//   - a string literal that is a route ending with the Tag, such as "Store.Add->ParseKeys.KeyTooWeak"
//     (what HasRoute is given, directly or through a table);
//   - a comparison of something's .Tag with the Tag, as a literal or a constant;
//   - a field or key whose name contains "tag" set to the Tag (a table's wantTag, or the Tag of an
//     &errors.Error{Tag: ...} given to errors.Is);
//   - a mention of the Tag's constant.
func ScanTags(root string) (*TagReport, error) {
	mod, err := readGoMod(root)
	if err != nil {
		return nil, err
	}
	files, err := loadSources(root)
	if err != nil {
		return nil, err
	}
	excepted, err := readExceptions(root)
	if err != nil {
		return nil, err
	}
	report := &TagReport{asserted: map[string]bool{}, excepted: excepted}
	consts := stringConstants(files)
	for _, f := range files {
		resolve := func(e ast.Expr) (string, bool) { return resolveTag(f, mod.module, consts, e) }
		if f.test {
			collectAsserted(f, resolve, report.asserted)
			continue
		}
		collectUses(f, resolve, report)
	}
	sort.SliceStable(report.Uses, func(i, j int) bool {
		a, b := report.Uses[i], report.Uses[j]
		if a.Tag != b.Tag {
			return a.Tag < b.Tag
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	report.Problems = append(report.Problems, duplicateTags(report.Uses)...)
	return report, nil
}

// platformSuffixes are the file name endings that mark a file as one platform's version.
var platformSuffixes = []string{"_linux", "_windows", "_darwin", "_unix", "_other"}

// platformOf returns the platform suffix of a Go file's name, or "".
func platformOf(file string) string {
	name := strings.TrimSuffix(path.Base(file), ".go")
	for _, suffix := range platformSuffixes {
		if strings.HasSuffix(name, suffix) {
			return suffix
		}
	}
	return ""
}

// duplicateTags reports each Tag that is created at more than one place: every failure case has
// its own Tag. The versions of one function in the files of different platforms may share a Tag.
// The uses come sorted by Tag.
func duplicateTags(uses []TagUse) []TagProblem {
	var out []TagProblem
	for i := 0; i < len(uses); {
		j := i + 1
		for j < len(uses) && uses[j].Tag == uses[i].Tag {
			j++
		}
		group := uses[i:j]
		i = j
		if len(group) < 2 {
			continue
		}
		platforms := map[string]bool{}
		variants := true
		for _, use := range group {
			p := platformOf(use.File)
			if p == "" || platforms[p] || use.Func != group[0].Func || path.Dir(use.File) != path.Dir(group[0].File) {
				variants = false
			}
			platforms[p] = true
		}
		if variants {
			continue
		}
		for _, use := range group[1:] {
			out = append(out, TagProblem{File: use.File, Line: use.Line,
				Message: "the Tag " + use.Tag + " is already created at " + group[0].File + ":" + strconv.Itoa(group[0].Line) + "; every failure case has its own Tag"})
		}
	}
	return out
}

// readExceptions reads the exceptions file of root; a missing file means no exception.
func readExceptions(root string) (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ExceptionsFile)))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, errors.NewErr(TagExceptionsUnreadable, err).SetMetadata("root", root)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; scanner.Scan(); n++ {
		m := exceptionLine.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		reason := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m[2]), ":"))
		if !strings.HasPrefix(strings.TrimSpace(m[2]), ":") || reason == "" {
			return nil, errors.New(TagExceptionMalformed, "an exception needs a reason: - `Tag`: reason",
				"file", ExceptionsFile, "line", n, "tag", m[1])
		}
		out[m[1]] = reason
	}
	return out, nil
}

// isTagName says whether a constant's name marks it as a Tag.
func isTagName(name string) bool {
	return strings.HasPrefix(name, "Tag") || strings.HasPrefix(name, "tag")
}

// stringConstants collects the string constants of every package: folder (from the module root,
// with slashes), then name, then value.
func stringConstants(files []*sourceFile) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, f := range files {
		dir := path.Dir(f.rel)
		for _, decl := range f.ast.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(lit.Value)
					if err != nil {
						continue
					}
					if out[dir] == nil {
						out[dir] = map[string]string{}
					}
					out[dir][name.Name] = value
				}
			}
		}
	}
	return out
}

// resolveTag gives the Tag an expression stands for: a string literal, a Tag constant of the
// file's package, or a Tag constant of another package of the module.
func resolveTag(f *sourceFile, module string, consts map[string]map[string]string, e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(x.Value)
		return value, err == nil
	case *ast.Ident:
		if !isTagName(x.Name) {
			return "", false
		}
		value, ok := consts[path.Dir(f.rel)][x.Name]
		return value, ok
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		if !ok || !isTagName(x.Sel.Name) {
			return "", false
		}
		for _, spec := range f.ast.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !hasPathPrefix(p, module) {
				continue
			}
			name := p[strings.LastIndex(p, "/")+1:]
			if spec.Name != nil {
				name = spec.Name.Name
			}
			if name != pkg.Name {
				continue
			}
			value, ok := consts[strings.TrimPrefix(strings.TrimPrefix(p, module), "/")][x.Sel.Name]
			return value, ok
		}
	}
	return "", false
}

// collectUses adds the constructor calls of a non-test file to the report.
func collectUses(f *sourceFile, resolve func(ast.Expr) (string, bool), report *TagReport) {
	errorsName := f.importName(errorsModule)
	if errorsName == "" {
		return
	}
	visit := func(funcName string, root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !constructors[sel.Sel.Name] {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != errorsName {
				return true
			}
			line := f.line(call.Pos())
			if funcName == "" {
				report.Problems = append(report.Problems, TagProblem{File: f.rel, Line: line,
					Message: "errors." + sel.Sel.Name + " outside a function: its route would be init; a sentinel is &errors.Error{Tag: ...}"})
				return true
			}
			if len(call.Args) == 0 {
				return true
			}
			tag, ok := resolve(call.Args[0])
			if !ok {
				report.Problems = append(report.Problems, TagProblem{File: f.rel, Line: line,
					Message: "the Tag of errors." + sel.Sel.Name + " is not a string literal or a Tag constant"})
				return true
			}
			report.Uses = append(report.Uses, TagUse{Tag: tag, File: f.rel, Line: line, Func: funcName})
			return true
		})
	}
	for _, decl := range f.ast.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			visit(funcDeclName(d), d)
		default:
			visit("", d)
		}
	}
}

// funcDeclName names a function as a route does: "Func" or "Type.Method".
func funcDeclName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	t := d.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
			continue
		case *ast.IndexExpr:
			t = x.X
			continue
		case *ast.IndexListExpr:
			t = x.X
			continue
		case *ast.Ident:
			return x.Name + "." + d.Name.Name
		}
		return d.Name.Name
	}
}

// collectAsserted adds the Tags a test file asserts to the set (the rules are ScanTags's).
func collectAsserted(f *sourceFile, resolve func(ast.Expr) (string, bool), asserted map[string]bool) {
	mark := func(e ast.Expr) {
		if tag, ok := resolve(e); ok {
			asserted[tag] = true
		}
	}
	isTagField := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "Tag"
	}
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ImportSpec:
			return false
		case *ast.BasicLit:
			if x.Kind != token.STRING {
				return true
			}
			if value, err := strconv.Unquote(x.Value); err == nil {
				if m := routeLiteral.FindStringSubmatch(value); m != nil {
					asserted[m[1]] = true
				}
			}
		case *ast.BinaryExpr:
			if x.Op != token.EQL && x.Op != token.NEQ {
				return true
			}
			if isTagField(x.X) {
				mark(x.Y)
			}
			if isTagField(x.Y) {
				mark(x.X)
			}
		case *ast.KeyValueExpr:
			if key, ok := x.Key.(*ast.Ident); ok && strings.Contains(strings.ToLower(key.Name), "tag") {
				mark(x.Value)
			}
		case *ast.Ident:
			if isTagName(x.Name) {
				mark(x)
			}
		case *ast.SelectorExpr:
			if isTagName(x.Sel.Name) {
				mark(x)
			}
		}
		return true
	})
}
