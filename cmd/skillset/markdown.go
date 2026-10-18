package main

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// markdown renders frontmatter-stripped markdown into one \n-joined string of
// ANSI-styled, width-wrapped lines. Block structure is parsed line by line;
// prose (headings, paragraphs, list items, quotes) is then parsed for inline
// markdown (*, **, backticks, links) before it is wrapped, so that stripping
// the delimiters does not change the wrap width.
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
	// flush joins the pending paragraph lines into one and wraps them. Inline
	// markdown is parsed per source line, so a delimiter pair can't span the
	// newline between two lines of the same paragraph.
	flush := func() {
		if len(para) == 0 {
			return
		}
		sep("para")
		var spans []span
		for i, line := range para {
			if i > 0 {
				spans = append(spans, span{text: " "})
			}
			spans = append(spans, inline(line)...)
		}
		out = append(out, wrapSpans(spans, "", "", width, ui.text)...)
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
			out = append(out, wrapSpans(inline(text), "", "", width, style)...)
			continue
		}
		if indent, marker, content, ok := listItem(line); ok {
			flush()
			sep("list")
			first := indent + marker
			rest := indent + strings.Repeat(" ", lipgloss.Width(marker))
			out = append(out, wrapSpans(inline(content), first, rest, width, ui.text)...)
			continue
		}
		if content, ok := quoteLine(line); ok {
			flush()
			sep("quote")
			out = append(out, wrapSpans(inline(content), "> ", "> ", width, ui.dim)...)
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

// span is one run of text carrying a single style. A zero style means the base
// style of the line it sits on.
type span struct {
	text  string
	style lipgloss.Style
}

// inline parses the inline markdown in one source line into styled spans. It is
// left-to-right and greedy: backtick, then **, then *, then a [label](dest)
// link; anything it cannot match cleanly is kept as literal text.
func inline(text string) []span {
	code, bold, italic := ui.label, ui.text.Bold(true), ui.text.Italic(true)
	var spans []span
	var lit []rune
	flush := func() {
		if len(lit) > 0 {
			spans = append(spans, span{text: string(lit)})
			lit = lit[:0]
		}
	}

	r := []rune(text)
	for i := 0; i < len(r); {
		switch c := r[i]; {
		case c == '`':
			if j := indexRune(r[i+1:], '`'); j >= 1 {
				flush()
				spans = append(spans, span{string(r[i+1 : i+1+j]), code})
				i += j + 2
				continue
			}
			lit = append(lit, c)
			i++
		case c == '*':
			k := starRun(r, i)
			// Three or more stars are ambiguous, so the whole run stays literal.
			if k >= 3 {
				lit = append(lit, r[i:i+k]...)
				i += k
				continue
			}
			// Bold wins over italic: a clean close is the next star run of the
			// exact same length, leaving non-empty content between the two.
			if j, ok := closingStar(r, i+k, k); ok {
				flush()
				if k == 2 {
					spans = append(spans, span{string(r[i+k : j]), bold})
				} else {
					spans = append(spans, span{string(r[i+k : j]), italic})
				}
				i = j + k
				continue
			}
			lit = append(lit, r[i:i+k]...)
			i += k
		case c == '[':
			// ![alt](src) is an image, which this renderer does not support, so a
			// link right after a ! stays literal rather than losing the !.
			if i > 0 && r[i-1] == '!' {
				lit = append(lit, c)
				i++
				continue
			}
			if label, dest, n, ok := parseLink(r[i:]); ok {
				flush()
				spans = append(spans, span{label, ui.label}, span{" " + dest, ui.dim})
				i += n
				continue
			}
			lit = append(lit, c)
			i++
		default:
			lit = append(lit, c)
			i++
		}
	}
	flush()
	return spans
}

// starRun is the length of the run of stars starting at r[i].
func starRun(r []rune, i int) int {
	n := 0
	for i+n < len(r) && r[i+n] == '*' {
		n++
	}
	return n
}

// closingStar reports the next star run at or after r[from:] and whether it is
// exactly want stars long. Only an equal-length run can close this opener, and
// the first one after the opener is the only candidate: any earlier different
// run would mean nested emphasis, which inline refuses to guess at.
func closingStar(r []rune, from, want int) (int, bool) {
	for j := from; j < len(r); j++ {
		if r[j] == '*' {
			return j, starRun(r, j) == want
		}
	}
	return 0, false
}

// parseLink reads a [label](dest) link starting at r[0]. It returns the label,
// the destination and the runes consumed, or ok=false when the syntax is broken.
func parseLink(r []rune) (label, dest string, n int, ok bool) {
	closeB := indexRune(r[1:], ']')
	if closeB < 0 {
		return "", "", 0, false
	}
	label = string(r[1 : 1+closeB])
	if label == "" {
		return "", "", 0, false
	}
	paren := 1 + closeB + 1
	if paren >= len(r) || r[paren] != '(' {
		return "", "", 0, false
	}
	closeP := indexRune(r[paren+1:], ')')
	if closeP < 0 {
		return "", "", 0, false
	}
	dest = string(r[paren+1 : paren+1+closeP])
	return label, dest, paren + 1 + closeP + 1, true
}

func indexRune(r []rune, c rune) int {
	for i, x := range r {
		if x == c {
			return i
		}
	}
	return -1
}

// flatten returns the spans' text as runes and, per rune, the index of the span
// that owns it, so wrapping can cut through a span without losing its style.
func flatten(spans []span) ([]rune, []int) {
	var r []rune
	var owner []int
	for i, sp := range spans {
		for _, c := range sp.text {
			r = append(r, c)
			owner = append(owner, i)
		}
	}
	return r, owner
}

// wrange is one word's [start,end) rune range into flattened text.
type wrange struct{ start, end int }

// wordsOf splits flattened text into words on whitespace, the same split the
// block renderer used to get with strings.Fields.
func wordsOf(r []rune) []wrange {
	var out []wrange
	for i := 0; i < len(r); {
		if unicode.IsSpace(r[i]) {
			i++
			continue
		}
		start := i
		for i < len(r) && !unicode.IsSpace(r[i]) {
			i++
		}
		out = append(out, wrange{start, i})
	}
	return out
}

// wrapSpans word-wraps styled spans to width columns, keeping each rune's style.
// The first line is prefixed with first and the rest with rest, both rendered in
// the base style and counted against width.
func wrapSpans(spans []span, first, rest string, width int, base lipgloss.Style) []string {
	r, owner := flatten(spans)
	words := wordsOf(r)
	if len(words) == 0 {
		return nil
	}
	var out []string
	var cur []wrange
	prefix := first
	curW := lipgloss.Width(prefix)

	emit := func() {
		if len(cur) == 0 {
			return
		}
		out = append(out, renderLine(prefix, cur, r, owner, spans, base))
		cur = nil
		prefix = rest
		curW = lipgloss.Width(prefix)
	}

	for _, w := range words {
		wW := lipgloss.Width(string(r[w.start:w.end]))
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
		out = append(out, hardBreakRunes(r[w.start:w.end], owner[w.start:w.end], spans, prefix, rest, width, base)...)
		prefix = rest
		curW = lipgloss.Width(prefix)
	}
	emit()
	return out
}

// renderLine renders prefix in the base style, then each word with its own
// styles, separated by single spaces.
func renderLine(prefix string, words []wrange, r []rune, owner []int, spans []span, base lipgloss.Style) string {
	var b strings.Builder
	if prefix != "" {
		b.WriteString(base.Render(prefix))
	}
	for i, w := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(renderRange(r[w.start:w.end], owner[w.start:w.end], spans, base))
	}
	return b.String()
}

// renderRange renders runes, merging consecutive runes that share a span so each
// style is emitted once rather than per rune.
func renderRange(r []rune, owner []int, spans []span, base lipgloss.Style) string {
	var b strings.Builder
	for i := 0; i < len(r); {
		idx := owner[i]
		j := i + 1
		for j < len(r) && owner[j] == idx {
			j++
		}
		b.WriteString(spans[idx].style.Inherit(base).Render(string(r[i:j])))
		i = j
	}
	return b.String()
}

// hardBreakRunes breaks one over-long word across lines, splitting its styles
// along with its runes.
func hardBreakRunes(r []rune, owner []int, spans []span, first, rest string, width int, base lipgloss.Style) []string {
	var out []string
	p := first
	for len(r) > 0 {
		avail := width - lipgloss.Width(p)
		if avail < 1 {
			avail = 1
		}
		n := takeRunes(r, avail)
		var b strings.Builder
		if p != "" {
			b.WriteString(base.Render(p))
		}
		b.WriteString(renderRange(r[:n], owner[:n], spans, base))
		out = append(out, b.String())
		r = r[n:]
		owner = owner[n:]
		p = rest
	}
	return out
}

// takeRunes returns how many leading runes fit in width columns. A single rune
// wider than width is kept whole rather than dropped.
func takeRunes(r []rune, width int) int {
	if width < 1 {
		width = 1
	}
	w := 0
	for i, c := range r {
		cw := lipgloss.Width(string(c))
		if w+cw > width {
			if i == 0 {
				return 1
			}
			return i
		}
		w += cw
	}
	return len(r)
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
