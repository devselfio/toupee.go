package toupee

import (
	"bytes"
	"strings"
)

// detectIndent returns the spaces and tabs that end output when they follow
// its last LF: the indentation at which an include or slot is being
// inserted. Output without an LF, or whose last line has anything else,
// has no indentation.
func detectIndent(output string) string {
	lf := strings.LastIndexByte(output, '\n')
	if lf < 0 {
		return ""
	}
	after := output[lf+1:]
	if strings.Trim(after, " \t") != "" {
		return ""
	}
	return after
}

// appendIndented appends content to out, prefixing every line after the
// first that is not empty with indent. Empty content instead removes the
// spaces and tabs that end out, so an empty insertion leaves no dangling
// indentation.
func appendIndented(out *bytes.Buffer, content, indent string) {
	if content == "" {
		b := out.Bytes()
		n := len(b)
		for n > 0 && (b[n-1] == ' ' || b[n-1] == '\t') {
			n--
		}
		out.Truncate(n)
		return
	}
	if indent == "" {
		out.WriteString(content)
		return
	}
	first := true
	for line := range strings.SplitSeq(content, "\n") {
		if first {
			out.WriteString(line)
			first = false
			continue
		}
		out.WriteByte('\n')
		if line != "" {
			out.WriteString(indent)
			out.WriteString(line)
		}
	}
}

// stripCommonIndent removes the leading and trailing blank lines of
// content, splitting on LF, and the shortest run of leading spaces and tabs
// shared by its other nonblank lines. A blank line is one of only spaces,
// tabs and CRs; interior blank lines keep their bytes.
func stripCommonIndent(content string) string {
	s, _ := stripCommonIndentMap(content)
	return s
}

// stripCommonIndentMap is stripCommonIndent that also returns, for each byte
// of the result, its offset in content, so errors in the stripped text can
// be reported where they are in the source.
func stripCommonIndentMap(content string) (string, []int) {
	type line struct{ start, end int }
	var lines []line
	start := 0
	for i := 0; i <= len(content); i++ {
		if i == len(content) || content[i] == '\n' {
			lines = append(lines, line{start, i})
			start = i + 1
		}
	}
	isContent := func(l line) bool { return strings.Trim(content[l.start:l.end], " \t\r") != "" }
	first, last := 0, len(lines)
	for first < last && !isContent(lines[first]) {
		first++
	}
	for last > first && !isContent(lines[last-1]) {
		last--
	}
	lines = lines[first:last]
	strip := -1
	for _, l := range lines {
		if !isContent(l) {
			continue
		}
		ws := 0
		for l.start+ws < l.end && (content[l.start+ws] == ' ' || content[l.start+ws] == '\t') {
			ws++
		}
		if strip < 0 || ws < strip {
			strip = ws
		}
	}
	var b strings.Builder
	var offsets []int
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
			offsets = append(offsets, lines[i-1].end)
		}
		from := l.start
		if isContent(l) {
			from += strip
		}
		b.WriteString(content[from:l.end])
		for j := from; j < l.end; j++ {
			offsets = append(offsets, j)
		}
	}
	return b.String(), offsets
}
