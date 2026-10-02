// Package shell reads a command line the way the hook policy needs it: every simple command the
// line runs, with its assignments, its words and the files its redirections write. Commands inside
// substitutions, pipelines, functions and unquoted here-documents are commands of their own.
// It parses with mvdan.cc/sh, and what it cannot see through is listed in Opaque, never guessed.
package shell

import (
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// Computed stands in a word for a part only known when the line runs: $VAR, $(…), $((…)).
const Computed = "$"

// Command is one simple command.
type Command struct {
	// Assigns are the NAME=value words before the command, and the words of export or declare.
	Assigns []string
	// Words are the command and its arguments, quotes removed and braces expanded. A part only
	// known at run time reads as Computed.
	Words []string
	// Exact tells, per word, whether the word is known before the line runs: no computed part
	// and no unquoted glob.
	Exact []bool
	// Writes are the files its redirections write (>, >>, >|, &>, <>, >&file).
	Writes []string
	// Reads are the files its redirections read (<, <>).
	Reads []string
	// Into is the command its standard output is piped into, if any.
	Into []string
	// Child is set when the command runs in a child shell of the line: in ( ), $( ), <( ), a
	// pipeline or the background. What it changes in its shell (its directory) the rest of the
	// line does not see.
	Child bool
}

// Line is a parsed command line.
type Line struct {
	Commands []Command
	// Opaque lists what the parser could not see through: a parse error, a computed command name.
	Opaque []string
}

// Parse reads a bash command line.
func Parse(line string) Line {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(line), "")
	if err != nil {
		return Line{Opaque: []string{"a line the shell parser cannot read (" + err.Error() + ")"}}
	}

	p := parser{index: map[*syntax.CallExpr]int{}, child: children(file)}
	syntax.Walk(file, p.visit)
	p.link()

	return p.line
}

type parser struct {
	line  Line
	index map[*syntax.CallExpr]int
	pipes [][2]*syntax.CallExpr
	child map[*syntax.Stmt]bool
}

// children are the statements that run in a child shell of the line.
func children(file *syntax.File) map[*syntax.Stmt]bool {
	child := map[*syntax.Stmt]bool{}
	mark := func(n syntax.Node) {
		syntax.Walk(n, func(m syntax.Node) bool {
			if s, ok := m.(*syntax.Stmt); ok {
				child[s] = true
			}

			return true
		})
	}

	syntax.Walk(file, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Subshell, *syntax.CmdSubst, *syntax.ProcSubst:
			mark(x)
		case *syntax.BinaryCmd:
			if x.Op == syntax.Pipe || x.Op == syntax.PipeAll {
				mark(x)
			}
		case *syntax.Stmt:
			if x.Background || x.Coprocess {
				mark(x)
			}
		}

		return true
	})

	return child
}

func (p *parser) visit(node syntax.Node) bool {
	switch n := node.(type) {
	case *syntax.Stmt:
		p.statement(n)
	case *syntax.DeclClause:
		var c Command
		for _, a := range n.Args {
			c.Assigns = append(c.Assigns, assignment(a))
		}

		if len(c.Assigns) > 0 {
			p.line.Commands = append(p.line.Commands, c)
		}
	case *syntax.BinaryCmd:
		if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
			left, right := lastCall(n.X), firstCall(n.Y)
			if left != nil && right != nil {
				p.pipes = append(p.pipes, [2]*syntax.CallExpr{left, right})
			}
		}
	}

	return true
}

// statement records the simple command of a statement with the files its redirections write.
func (p *parser) statement(s *syntax.Stmt) {
	c := Command{Child: p.child[s]}

	for _, r := range s.Redirs {
		if target, ok := written(r); ok {
			c.Writes = append(c.Writes, target)
		}

		if r.Word != nil && (r.Op == syntax.RdrIn || r.Op == syntax.RdrInOut) {
			target, _ := literal(r.Word.Parts, false)
			c.Reads = append(c.Reads, target)
		}
	}

	call, isCall := s.Cmd.(*syntax.CallExpr)
	if isCall {
		p.call(call, &c)
	}

	if len(c.Words) == 0 && len(c.Assigns) == 0 && len(c.Writes) == 0 && len(c.Reads) == 0 {
		return
	}

	if isCall {
		p.index[call] = len(p.line.Commands)
	}

	p.line.Commands = append(p.line.Commands, c)
}

// call reads a simple command's assignments and words, braces expanded.
func (p *parser) call(call *syntax.CallExpr, c *Command) {
	for _, a := range call.Assigns {
		c.Assigns = append(c.Assigns, assignment(a))
	}

	for _, w := range call.Args {
		// Split a copy: the walk goes on into the original word, which must keep its parts.
		word := &syntax.Word{Parts: append([]syntax.WordPart(nil), w.Parts...)}
		syntax.SplitBraces(word)

		for expanded, err := range expand.BracesSeq(nil, word) {
			if err != nil {
				p.line.Opaque = append(p.line.Opaque, "a brace expansion too large to read ("+err.Error()+")")
				break
			}

			text, exact := literal(expanded.Parts, false)
			c.Words = append(c.Words, text)
			c.Exact = append(c.Exact, exact)
		}
	}

	if len(c.Words) > 0 && !c.Exact[0] {
		p.line.Opaque = append(p.line.Opaque, "a command computed at run time ("+strings.Join(c.Words, " ")+")")
	}
}

// link records, for each pipe, the command the left side's output goes into.
func (p *parser) link() {
	for _, pair := range p.pipes {
		left, okLeft := p.index[pair[0]]
		right, okRight := p.index[pair[1]]

		if okLeft && okRight {
			p.line.Commands[left].Into = p.line.Commands[right].Words
		}
	}
}

func firstCall(s *syntax.Stmt) *syntax.CallExpr {
	switch c := s.Cmd.(type) {
	case *syntax.CallExpr:
		return c
	case *syntax.BinaryCmd:
		return firstCall(c.X)
	}

	return nil
}

func lastCall(s *syntax.Stmt) *syntax.CallExpr {
	switch c := s.Cmd.(type) {
	case *syntax.CallExpr:
		return c
	case *syntax.BinaryCmd:
		return lastCall(c.Y)
	}

	return nil
}

// assignment is NAME=value, or a word of export or declare that assigns nothing: a bare NAME, an
// option (-x), or a name computed at run time (Computed).
func assignment(a *syntax.Assign) string {
	switch {
	case a.Name == nil && a.Value != nil:
		text, _ := literal(a.Value.Parts, false)
		return text
	case a.Name == nil:
		return ""
	case a.Naked:
		return a.Name.Value
	case a.Value == nil:
		return a.Name.Value + "="
	}

	text, _ := literal(a.Value.Parts, false)

	return a.Name.Value + "=" + text
}

// written is the file a redirection writes, if it writes one.
func written(r *syntax.Redirect) (string, bool) {
	if r.Word == nil {
		return "", false
	}

	target, _ := literal(r.Word.Parts, false)

	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.RdrAll, syntax.AppAll, syntax.RdrInOut:
		return target, true
	case syntax.DplOut:
		// >&2 and >&- duplicate a descriptor; >&file writes the file.
		if target == "-" || strings.Trim(target, "0123456789") == "" {
			return "", false
		}

		return target, true
	default:
		return "", false
	}
}

// literal is a word's text with its quotes removed. A part only known at run time reads as
// Computed and makes the word inexact, and so does an unquoted glob.
func literal(parts []syntax.WordPart, quoted bool) (string, bool) {
	var b strings.Builder

	exact := true

	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if !quoted && strings.ContainsAny(p.Value, "*?[") {
				exact = false
			}

			b.WriteString(unescape(p.Value, quoted))
		case *syntax.SglQuoted:
			// $'…' decodes escapes (\x67 is g): read as computed rather than decode it here.
			if p.Dollar && strings.Contains(p.Value, `\`) {
				b.WriteString(Computed)

				exact = false

				continue
			}

			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			text, ok := literal(p.Parts, true)
			b.WriteString(text)

			exact = exact && ok
		case *syntax.ParamExp:
			// A plain $NAME or ${NAME} keeps its name, so a check that knows the variable can
			// resolve it; anything else ($1, ${X:-y}, ${#X}) reads as Computed.
			if name := plainParam(p); name != "" {
				b.WriteString("${" + name + "}")
			} else {
				b.WriteString(Computed)
			}

			exact = false
		default:
			b.WriteString(Computed)

			exact = false
		}
	}

	return b.String(), exact
}

// plainParam is the name of a plain $NAME or ${NAME} expansion, or "" for any other.
func plainParam(p *syntax.ParamExp) string {
	operated := p.Excl || p.Length || p.Width || p.Index != nil || p.Slice != nil || p.Repl != nil || p.Names != 0 || p.Exp != nil
	if p.Param == nil || operated || !isName(p.Param.Value) {
		return ""
	}

	return p.Param.Value
}

// isName reports whether s is a variable name: a letter or _, then letters, digits or _.
func isName(s string) bool {
	for i, r := range s {
		letter := r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}

	return s != ""
}

// unescape removes the backslashes the shell removes: before any character outside quotes, and
// before $ ` " \ and a newline inside double quotes.
func unescape(s string, quoted bool) string {
	if !strings.Contains(s, `\`) {
		return s
	}

	var b strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}

		next := s[i+1]

		switch {
		case next == '\n':
			i++
		case !quoted || strings.IndexByte("$`\"\\", next) >= 0:
			b.WriteByte(next)

			i++
		default:
			b.WriteByte(s[i])
		}
	}

	return b.String()
}
