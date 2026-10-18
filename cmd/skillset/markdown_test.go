package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// md renders src and splits the result into lines with the ANSI stripped, so a
// test reads the text and the display widths rather than escape sequences.
func md(src string, width int) []string {
	return strings.Split(plain(markdown(src, width)), "\n")
}

// TestMarkdownHeadings: every ATX level strips the hashes and keeps the text.
func TestMarkdownHeadings(t *testing.T) {
	for _, tc := range []struct {
		hashes int
		name   string
	}{
		{1, "one"}, {2, "two"}, {3, "three"}, {4, "four"}, {5, "five"}, {6, "six"},
	} {
		src := strings.Repeat("#", tc.hashes) + " " + tc.name
		got := md(src, 80)
		if len(got) != 1 || got[0] != tc.name {
			t.Errorf("%q rendered %q, want just %q", src, got, tc.name)
		}
	}
}

// TestMarkdownHeadingNeedsASpace: a run of hashes that is not a heading stays.
func TestMarkdownHeadingNeedsASpace(t *testing.T) {
	if got := md("####### seven", 80); strings.Join(got, " ") != "####### seven" {
		t.Errorf("seven hashes rendered %q, want them kept literal", got)
	}
	if got := md("#tag", 80); len(got) != 1 || got[0] != "#tag" {
		t.Errorf("#tag rendered %q, want it literal", got)
	}
}

// TestMarkdownHeadingWraps: heading text wraps to the width without losing words.
func TestMarkdownHeadingWraps(t *testing.T) {
	const want = "a heading that is far too long for the width"
	got := md("# "+want, 12)
	for _, l := range got {
		if lipgloss.Width(l) > 12 {
			t.Errorf("heading line %q is %d columns, over 12", l, lipgloss.Width(l))
		}
	}
	if strings.Join(got, " ") != want {
		t.Errorf("heading text = %q, want %q", strings.Join(got, " "), want)
	}
}

// TestMarkdownParagraphJoinsAndWraps: adjacent lines are one paragraph, and a
// blank line starts the next one.
func TestMarkdownParagraphJoinsAndWraps(t *testing.T) {
	got := md("first line of\na paragraph\n\nsecond paragraph", 80)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "first line of a paragraph") {
		t.Errorf("paragraph was not joined:\n%s", joined)
	}
	if !strings.Contains(joined, "second paragraph") {
		t.Errorf("second paragraph is missing:\n%s", joined)
	}
}

// TestMarkdownParagraphWraps: prose wraps on words and never exceeds the width.
func TestMarkdownParagraphWraps(t *testing.T) {
	const src = "one two three four five six seven eight nine ten"
	got := md(src, 10)
	for _, l := range got {
		if lipgloss.Width(l) > 10 {
			t.Errorf("line %q is %d columns, over 10", l, lipgloss.Width(l))
		}
	}
	if strings.Join(got, " ") != src {
		t.Errorf("wrapped = %q, want the words preserved", strings.Join(got, " "))
	}
}

// TestMarkdownUnorderedList: the markers stay and the items run together.
func TestMarkdownUnorderedList(t *testing.T) {
	got := md("- one\n- two\n- three", 80)
	want := []string{"- one", "- two", "- three"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("list = %q, want %q", got, want)
	}
}

// TestMarkdownNumberedList: any digit count is a marker, kept verbatim.
func TestMarkdownNumberedList(t *testing.T) {
	got := md("1. first\n2. second\n10. tenth", 80)
	want := []string{"1. first", "2. second", "10. tenth"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("list = %q, want %q", got, want)
	}
}

// TestMarkdownListMarkers: all three unordered markers are recognised.
func TestMarkdownListMarkers(t *testing.T) {
	got := md("- dash\n* star\n+ plus\n9. nine", 80)
	want := []string{"- dash", "* star", "+ plus", "9. nine"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("list = %q, want %q", got, want)
	}
}

// TestMarkdownListHangsContinuation: a wrapped item indents under its marker.
func TestMarkdownListHangsContinuation(t *testing.T) {
	got := md("- a fairly long list item that must wrap", 16)
	if got[0] != "- a fairly long" {
		t.Errorf("first line = %q", got[0])
	}
	for _, l := range got[1:] {
		if !strings.HasPrefix(l, "  ") {
			t.Errorf("continuation %q is not indented two spaces", l)
		}
		if lipgloss.Width(l) > 16 {
			t.Errorf("continuation %q is %d columns, over 16", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownNumberedListHangs: the hanging indent matches the marker's width.
func TestMarkdownNumberedListHangs(t *testing.T) {
	got := md("10. a long enough item to wrap around", 18)
	if !strings.HasPrefix(got[0], "10. ") {
		t.Errorf("first line %q lost the marker", got[0])
	}
	for _, l := range got[1:] {
		if !strings.HasPrefix(l, "    ") {
			t.Errorf("continuation %q is not indented four spaces", l)
		}
		if lipgloss.Width(l) > 18 {
			t.Errorf("continuation %q is %d columns, over 18", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownNestedListKeepsIndent: a nested item keeps its source indentation.
func TestMarkdownNestedListKeepsIndent(t *testing.T) {
	got := md("- top\n  - nested", 80)
	want := []string{"- top", "  - nested"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("list = %q, want %q", got, want)
	}
}

// TestMarkdownBlockQuote: the marker is stripped and re-applied, lines together.
func TestMarkdownBlockQuote(t *testing.T) {
	got := md("> a quoted line\n> and another", 80)
	want := []string{"> a quoted line", "> and another"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("quote = %q, want %q", got, want)
	}
}

// TestMarkdownBlockQuoteWraps: wrapped quote lines all carry the marker.
func TestMarkdownBlockQuoteWraps(t *testing.T) {
	got := md("> a quoted line that is long enough to wrap", 20)
	for _, l := range got {
		if !strings.HasPrefix(l, "> ") {
			t.Errorf("quote line %q lost its marker", l)
		}
		if lipgloss.Width(l) > 20 {
			t.Errorf("quote line %q is %d columns, over 20", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownQuoteWithoutSpace: ">text" still gets the "> " prefix.
func TestMarkdownQuoteWithoutSpace(t *testing.T) {
	got := md(">tight", 80)
	if len(got) != 1 || got[0] != "> tight" {
		t.Errorf(">tight rendered %q, want the marker spaced", got)
	}
}

// TestMarkdownFencedCodeBackticks: the info string and the code both survive.
func TestMarkdownFencedCodeBackticks(t *testing.T) {
	got := md("```go\npackage main\nfunc main() {}\n```", 80)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "package main") || !strings.Contains(joined, "func main() {}") {
		t.Errorf("code lost:\n%s", joined)
	}
	if !strings.Contains(joined, "```go") {
		t.Errorf("info string lost:\n%s", joined)
	}
}

// TestMarkdownFencedCodeTilde: a tilde fence works like a backtick one.
func TestMarkdownFencedCodeTilde(t *testing.T) {
	got := md("~~~\ncode here\n~~~", 80)
	if joined := strings.Join(got, "\n"); !strings.Contains(joined, "code here") {
		t.Errorf("tilde fence lost its code:\n%s", joined)
	}
}

// TestMarkdownCodeIsNotInterpreted: nothing inside a fence is re-parsed.
func TestMarkdownCodeIsNotInterpreted(t *testing.T) {
	got := md("```\n# not a heading\n- not a list\n> not a quote\n```", 80)
	joined := strings.Join(got, "\n")
	for _, want := range []string{"# not a heading", "- not a list", "> not a quote"} {
		if !strings.Contains(joined, want) {
			t.Errorf("fence re-parsed %q:\n%s", want, joined)
		}
	}
}

// TestMarkdownUnclosedFence: an unclosed fence keeps its content to the end.
func TestMarkdownUnclosedFence(t *testing.T) {
	got := md("before\n```\ncode that never closes", 80)
	if joined := strings.Join(got, "\n"); !strings.Contains(joined, "code that never closes") {
		t.Errorf("unclosed fence dropped its content:\n%s", joined)
	}
}

// TestMarkdownCodeSoftWraps: a long code line is split by width, not content.
func TestMarkdownCodeSoftWraps(t *testing.T) {
	line := strings.Repeat("a", 35)
	got := md("```\n"+line+"\n```", 10)
	var code []string
	for _, l := range got {
		if l == "```" {
			continue
		}
		code = append(code, l)
	}
	if strings.Join(code, "") != line {
		t.Errorf("soft-wrapped code = %q, want %q", strings.Join(code, ""), line)
	}
	for _, l := range code {
		if lipgloss.Width(l) > 10 {
			t.Errorf("code chunk %q is %d columns, over 10", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownCodeKeepsIndent: indentation inside a fence is left as written.
func TestMarkdownCodeKeepsIndent(t *testing.T) {
	got := md("```\n\tindented\n  spaced\n```", 80)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "\tindented") || !strings.Contains(joined, "  spaced") {
		t.Errorf("indentation lost:\n%s", joined)
	}
}

// TestMarkdownUnsupportedStaysLiteral: unknown syntax is reflowed, not parsed.
func TestMarkdownUnsupportedStaysLiteral(t *testing.T) {
	got := md("| a | b |\n|---|---|\n![alt](img.png)\n<a href=\"x\">link</a>", 80)
	joined := strings.Join(got, "\n")
	for _, want := range []string{"| a | b |", "|---|---|", "![alt](img.png)", `<a href="x">link</a>`} {
		if !strings.Contains(joined, want) {
			t.Errorf("unsupported syntax changed, missing %q in:\n%s", want, joined)
		}
	}
}

// TestMarkdownEmptyInput: nothing in, nothing out.
func TestMarkdownEmptyInput(t *testing.T) {
	if got := markdown("", 80); got != "" {
		t.Errorf("empty input rendered %q", got)
	}
	if got := markdown("\n\n\n", 80); got != "" {
		t.Errorf("blank input rendered %q", got)
	}
}

// TestMarkdownTinyWidthDoesNotHang: zero and negative widths floor to one, and
// no line overflows, no character is dropped or reordered.
func TestMarkdownTinyWidthDoesNotHang(t *testing.T) {
	for _, w := range []int{0, 1, -3} {
		got := md("hello world", w)
		for _, l := range got {
			if lipgloss.Width(l) > 1 {
				t.Errorf("width %d produced a %d-column line %q", w, lipgloss.Width(l), l)
			}
		}
		if joined := strings.Join(got, ""); joined != "helloworld" {
			t.Errorf("width %d mangled the text: %q", w, joined)
		}
	}
}

// TestMarkdownLongWordBreaks: a word wider than the width is split, not dropped.
func TestMarkdownLongWordBreaks(t *testing.T) {
	word := strings.Repeat("x", 37)
	got := md(word, 10)
	if strings.Join(got, "") != word {
		t.Errorf("long word was not preserved: %q", strings.Join(got, ""))
	}
	for _, l := range got {
		if lipgloss.Width(l) > 10 {
			t.Errorf("chunk %q is %d columns, over 10", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownWideUnicode: wide characters count as two columns when wrapping.
func TestMarkdownWideUnicode(t *testing.T) {
	const text = "日本語のテキスト"
	got := md(text, 4)
	if strings.Join(got, "") != text {
		t.Errorf("wide text mangled: %q", strings.Join(got, ""))
	}
	for _, l := range got {
		if lipgloss.Width(l) > 4 {
			t.Errorf("line %q is %d columns, over 4", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownCombiningCharacters: a combining mark stays with its base letter.
func TestMarkdownCombiningCharacters(t *testing.T) {
	eacute := "e\u0301" // one cell: an e and a combining acute
	src := eacute + eacute + eacute
	got := md(src, 2)
	if strings.Join(got, "") != src {
		t.Errorf("combining text mangled: %q", strings.Join(got, ""))
	}
	for _, l := range got {
		if lipgloss.Width(l) > 2 {
			t.Errorf("line %q is %d columns, over 2", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownInlineCode: backticks are stripped and the inner text kept literal.
func TestMarkdownInlineCode(t *testing.T) {
	got := md("use `go build` to compile", 80)
	if got[0] != "use go build to compile" {
		t.Errorf("inline code = %q, want the backticks stripped", got[0])
	}
}

// TestMarkdownBold: the ** delimiters go and the inner text stays.
func TestMarkdownBold(t *testing.T) {
	got := md("make it **really bold** now", 80)
	if got[0] != "make it really bold now" {
		t.Errorf("bold = %q, want the delimiters stripped", got[0])
	}
}

// TestMarkdownItalic: single * delimiters go and the inner text stays.
func TestMarkdownItalic(t *testing.T) {
	got := md("a *whisper* here", 80)
	if got[0] != "a whisper here" {
		t.Errorf("italic = %q, want the delimiters stripped", got[0])
	}
}

// TestMarkdownBoldBeforeItalic: ** is matched before *, so a three-star run, a
// bold pair that nests an italic, and a bare star are all left literal.
func TestMarkdownBoldBeforeItalic(t *testing.T) {
	for _, tc := range []string{"***x***", "**a *b**", "**", "*"} {
		if got := md(tc, 80); got[0] != tc {
			t.Errorf("%q rendered %q, want it kept literal", tc, got)
		}
	}
}

// TestMarkdownLinkShowsLabelAndDestination: the label and the URL both stay on
// screen, label first and the URL muted after it.
func TestMarkdownLinkShowsLabelAndDestination(t *testing.T) {
	got := md("see [the docs](https://example.com) first", 80)
	if got[0] != "see the docs https://example.com first" {
		t.Errorf("link = %q, want label then destination", got[0])
	}
}

// TestMarkdownStarInsideCodeStaysLiteral: backticks shield their contents from
// emphasis, but an italic outside them still parses.
func TestMarkdownStarInsideCodeStaysLiteral(t *testing.T) {
	got := md("`a * b` and *c*", 80)
	if got[0] != "a * b and c" {
		t.Errorf("code/italic = %q, want the star inside backticks kept", got[0])
	}
}

// TestMarkdownUnderscoreIsNotEmphasis: snake_case and dunder names survive.
func TestMarkdownUnderscoreIsNotEmphasis(t *testing.T) {
	got := md("snake_case __init__ _private_", 80)
	if got[0] != "snake_case __init__ _private_" {
		t.Errorf("underscores = %q, want them untouched", got[0])
	}
}

// TestMarkdownUnmatchedStaysLiteral: a missing closer is left as source text.
func TestMarkdownUnmatchedStaysLiteral(t *testing.T) {
	for _, tc := range []string{"a `b c", "[x](", "*odd"} {
		if got := md(tc, 80); got[0] != tc {
			t.Errorf("%q rendered %q, want it literal", tc, got)
		}
	}
}

// TestMarkdownInlineInHeading: inline parsing applies to heading text too.
func TestMarkdownInlineInHeading(t *testing.T) {
	if got := md("# **Bold** title", 80); got[0] != "Bold title" {
		t.Errorf("heading = %q, want inline bold applied", got[0])
	}
	if got := md("## run `go test` now", 80); got[0] != "run go test now" {
		t.Errorf("heading = %q, want inline code applied", got[0])
	}
}

// TestMarkdownInlineInList: inline parsing applies to list-item content.
func TestMarkdownInlineInList(t *testing.T) {
	got := md("- use `go build`\n- a *quiet* note", 80)
	want := []string{"- use go build", "- a quiet note"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("list = %q, want %q", got, want)
	}
}

// TestMarkdownInlineInQuote: inline parsing applies to block-quote content.
func TestMarkdownInlineInQuote(t *testing.T) {
	got := md("> see **bold** and `code` here", 80)
	if got[0] != "> see bold and code here" {
		t.Errorf("quote = %q, want inline applied", got[0])
	}
}

// TestMarkdownInlineNotInFence: nothing inside a fence is styled or stripped.
func TestMarkdownInlineNotInFence(t *testing.T) {
	got := md("```\n`not code` and **not bold**\n```", 80)
	joined := strings.Join(got, "\n")
	for _, want := range []string{"`not code`", "**not bold**"} {
		if !strings.Contains(joined, want) {
			t.Errorf("fence lost literal %q:\n%s", want, joined)
		}
	}
}

// TestMarkdownMixedDocument: the full preview path keeps block structure and
// inline formatting apart, and a table stays literal.
func TestMarkdownMixedDocument(t *testing.T) {
	src := "# **Bold** title\n\n" +
		"a paragraph with `code`, *italic* and a [link](https://x).\n\n" +
		"- an *item* with **bold**\n\n" +
		"> quoted `code`\n\n" +
		"```go\n`raw` and **raw**\n```\n\n" +
		"| a | b |\n"
	got := strings.Join(md(src, 80), "\n")
	for _, want := range []string{
		"Bold title",
		"a paragraph with code, italic and a link https://x.",
		"- an item with bold",
		"> quoted code",
		"```go",
		"`raw` and **raw**",
		"| a | b |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mixed document missing %q in:\n%s", want, got)
		}
	}
}

// TestMarkdownInlineWraps: stripped delimiters are counted before wrapping, so a
// marked word wraps like any other and never overflows.
func TestMarkdownInlineWraps(t *testing.T) {
	got := md("a **bold word** here", 8)
	if strings.Join(got, " ") != "a bold word here" {
		t.Errorf("wrapped = %q, want the words preserved", strings.Join(got, " "))
	}
	for _, l := range got {
		if lipgloss.Width(l) > 8 {
			t.Errorf("line %q is %d columns, over 8", l, lipgloss.Width(l))
		}
	}
}

// TestMarkdownInlineWideUnicode: wide runes inside a marked span wrap by width.
func TestMarkdownInlineWideUnicode(t *testing.T) {
	got := md("**日本語** の テキスト", 6)
	if strings.Join(got, "") != "日本語のテキスト" {
		t.Errorf("wide marked text mangled: %q", strings.Join(got, ""))
	}
	for _, l := range got {
		if lipgloss.Width(l) > 6 {
			t.Errorf("line %q is %d columns, over 6", l, lipgloss.Width(l))
		}
	}
}
