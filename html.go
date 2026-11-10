package toupee

import "strings"

// findTagEnd returns the index of the '>' that closes the tag input starts
// with, ignoring any '>' inside double quotes, or -1 when there is none.
// Single quotes have no quoting effect.
func findTagEnd(input string) int {
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '"':
			i++
			for i < len(input) && input[i] != '"' {
				i++
			}
		case '>':
			return i
		}
	}
	return -1
}

// attrValue returns the bytes between the quotes of name="VALUE" in tag. The
// name must start the tag or follow an ASCII space, and be followed directly
// by '="'. No entities are decoded and single quotes are not accepted.
func attrValue(tag, name string) (string, bool) {
	for i := 0; i < len(tag); i++ {
		if !strings.HasPrefix(tag[i:], name) {
			continue
		}
		if i > 0 && tag[i-1] != ' ' {
			continue
		}
		after := i + len(name)
		if after+1 < len(tag) && tag[after] == '=' && tag[after+1] == '"' {
			start := after + 2
			if end := strings.IndexByte(tag[start:], '"'); end >= 0 {
				return tag[start : start+end], true
			}
		}
	}
	return "", false
}

// escapeHTML escapes exactly &, <, >, " and ' with Toupee's entity
// spellings and leaves every other byte unchanged.
func escapeHTML(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	for i := 0; i < len(input); i++ {
		switch c := input[i]; c {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#x27;")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
