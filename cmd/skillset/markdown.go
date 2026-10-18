package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// markdown renders frontmatter-stripped markdown into one \n-joined string of
// ANSI-styled, width-wrapped lines. It handles block-level structure only;
// inline markdown (*, **, backticks, links) is left as literal source text.
func markdown(src string, width int) string {
	width = max(1, width)
	var out []string
	var para []string
	fence := rune(0) // the open fence's char, 0 while not inside one
	last := ""       // the kind of block just emitted, so separators can skip
	// lipgloss expands tabs to spaces by default; code keeps its tabs verbatim.
	code := ui.dim.TabWidth(-1)

	// sep puts one blank line between unlike blocks. A list or a quote runs its
	// own lines together, so consecutive ones of the same kind stay tight.
	sep := func(kind string) {
		if last == kind {
			return
		}
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
		last = kind
	}
	// flush joins the pending paragraph lines into one and wraps them.
	flush := func() {
		if len(para) == 0 {
			return
		}
		sep("para")
		out = append(out, wrap(strings.Join(para, " "), "", "", width, ui.text)...)
		para = para[:0]
	}

	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if fence != 0 {
			if closingFence(line, fence) {
				out = append(out, code.Render(trunc(line, width)))
				fence = 0
			} else {
				out = append(out, wrapCode(line, width, code)...)
			}
			continue
		}
		if c, ok := openingFence(line); ok {
			flush()
			sep("code")
			out = append(out, code.Render(trunc(line, width)))
			fence = c
			continue
		}
		if level, text, ok := heading(line); ok {
			flush()
			sep("heading")
			style := ui.text.Bold(true)
			if level == 1 {
				style = ui.title
			}
			out = append(out, wrap(text, "", "", width, style)...)
			continue
		}
		if indent, marker, content, ok := listItem(line); ok {
			flush()
			sep("list")
			first := indent + marker
			rest := indent + strings.Repeat(" ", lipgloss.Width(marker))
			out = append(out, wrap(content, first, rest, width, ui.text)...)
			continue
		}
		if content, ok := quoteLine(line); ok {
			flush()
			sep("quote")
			out = append(out, wrap(content, "> ", "> ", width, ui.dim)...)
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			last = "" // a blank line ends whatever block was running
			continue
		}
		para = append(para, strings.TrimSpace(line))
	}
	flush()
	return strings.Join(out, "\n")
}

// heading reads an ATX heading: 1-6 #s then a space or the end of the line.
func heading(line string) (level int, text string, ok bool) {
	n := 0
	for n < len(line) && n < 6 && line[n] == '#' {
		n++
	}
	if n == 0 || (n < len(line) && line[n] != ' ' && line[n] != '\t') {
		return 0, "", false
	}
	return n, strings.TrimSpace(line[n:]), true
}

// listItem reads a list item: a space indent, then "- ", "* ", "+ " or "N. ".
// It returns the indent, the marker (with its trailing space) and the content.
func listItem(line string) (indent, marker, content string, ok bool) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	indent, rest := line[:i], line[i:]
	for _, m := range []string{"- ", "* ", "+ "} {
		if after, found := strings.CutPrefix(rest, m); found {
			return indent, m, after, true
		}
	}
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j > 0 && j+1 < len(rest) && rest[j] == '.' && rest[j+1] == ' ' {
		return indent, rest[:j+2], rest[j+2:], true
	}
	return "", "", "", false
}

// quoteLine reads a block quote line: ">" then an optional single space.
func quoteLine(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, ">")
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(rest, " "), true
}

// openingFence reports the char of a fenced-code opening line, which may carry
// an info string after the ``` or ~~~.
func openingFence(line string) (rune, bool) {
	rest := strings.TrimLeft(line, " ")
	switch {
	case strings.HasPrefix(rest, "```"):
		return '`', true
	case strings.HasPrefix(rest, "~~~"):
		return '~', true
	}
	return 0, false
}

// closingFence is a line of three or more of the fence char and nothing else.
func closingFence(line string, fence rune) bool {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return false
	}
	for _, r := range t {
		if r != fence {
			return false
		}
	}
	return true
}

// wrapCode soft-wraps one code line to width columns, preserving its bytes and
// its line boundaries, so an empty line still comes through as an empty line.
func wrapCode(line string, width int, style lipgloss.Style) []string {
	if line == "" {
		return []string{""}
	}
	var out []string
	for rem := line; rem != ""; {
		piece, rest := take(rem, width)
		out = append(out, style.Render(piece))
		rem = rest
	}
	return out
}

// wrap breaks text into lines no wider than width columns. The first line is
// prefixed with first and the rest with rest, both counted against width. Words
// longer than a line are broken rather than dropped.
func wrap(text, first, rest string, width int, style lipgloss.Style) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var out []string
	var cur []string
	prefix := first
	curW := lipgloss.Width(prefix)

	emit := func() {
		if len(cur) == 0 {
			return
		}
		out = append(out, style.Render(prefix+strings.Join(cur, " ")))
		cur = nil
		prefix = rest
		curW = lipgloss.Width(prefix)
	}

	for _, w := range words {
		wW := lipgloss.Width(w)
		space := 0
		if len(cur) > 0 {
			space = 1
		}
		if curW+space+wW <= width {
			cur = append(cur, w)
			curW += space + wW
			continue
		}
		if len(cur) > 0 {
			emit()
		}
		if wW <= width-lipgloss.Width(prefix) {
			cur = append(cur, w)
			curW = lipgloss.Width(prefix) + wW
			continue
		}
		out = append(out, hardBreak(w, prefix, rest, width, style)...)
		prefix = rest
		curW = lipgloss.Width(prefix)
	}
	emit()
	return out
}

// hardBreak renders one over-long word, breaking it across lines with first on
// the first line and rest on the continuation lines.
func hardBreak(word, first, rest string, width int, style lipgloss.Style) []string {
	var out []string
	p := first
	for word != "" {
		avail := width - lipgloss.Width(p)
		if avail < 1 {
			avail = 1
		}
		piece, rem := take(word, avail)
		out = append(out, style.Render(p+piece))
		word = rem
		p = rest
	}
	return out
}

// take splits s into the longest prefix fitting in width columns and the rest.
// A single rune wider than width is kept whole rather than dropped.
func take(s string, width int) (string, string) {
	if width < 1 {
		width = 1
	}
	if lipgloss.Width(s) <= width {
		return s, ""
	}
	r := []rune(s)
	var cur []rune
	w := 0
	for i := 0; i < len(r); i++ {
		rw := lipgloss.Width(string(r[i]))
		if w+rw > width {
			if i == 0 {
				return string(r[i]), string(r[i+1:])
			}
			return string(cur), string(r[i:])
		}
		cur = append(cur, r[i])
		w += rw
	}
	return s, ""
}
