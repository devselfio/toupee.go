package toupee

import "strings"

// Control-flow tag spellings, as Toupee v0.4.0's parser matches them.
const (
	ifOpen    = "<t-if "
	ifClose   = "</t-if>"
	forOpen   = "<t-for "
	forClose  = "</t-for>"
	elifOpen  = "<t-elif "
	elseOpen  = "<t-else"
	letCloser = "</t-let>"
)

var (
	letOpeners  = []string{"<t-let ", "<t-let>"}
	ifOpeners   = []string{"<t-if ", "<t-if>"}
	strayOpener = []string{"<t-else", "<t-elif "}
)

// hasBoolAttr reports whether tag has the valueless attribute name: a space,
// then name, then a space, '/', '>' or the end of tag. Double-quoted regions
// are skipped.
func hasBoolAttr(tag, name string) bool {
	for i := 0; i < len(tag); {
		switch {
		case tag[i] == '"':
			i++
			for i < len(tag) && tag[i] != '"' {
				i++
			}
			if i < len(tag) {
				i++
			}
		case tag[i] == ' ' && strings.HasPrefix(tag[i+1:], name):
			after := i + 1 + len(name)
			if after >= len(tag) || tag[after] == ' ' || tag[after] == '/' || tag[after] == '>' {
				return true
			}
			i++
		default:
			i++
		}
	}
	return false
}

// findMatchingClose returns the index in input of the closeTag that matches
// nesting level zero, counting each openTag as one level, or -1.
func findMatchingClose(input, openTag, closeTag string) int {
	nesting := 0
	for i := 0; i < len(input); {
		switch {
		case strings.HasPrefix(input[i:], openTag):
			nesting++
			i += len(openTag)
		case strings.HasPrefix(input[i:], closeTag):
			if nesting == 0 {
				return i
			}
			nesting--
			i += len(closeTag)
		default:
			i++
		}
	}
	return -1
}

// separator is a t-elif or t-else tag found in a block body: its offset in
// that body and its length.
type separator struct {
	pos, length int
	isElse      bool
}

// findSeparator returns the first t-else (and, when inConditional, t-elif)
// tag in body at or after from that is not inside a nested t-if or t-for,
// as upstream's findAtDepthZero does. Only "<t-if " and "<t-for " open a
// level. A closing tag of the block's own kind at level zero ends the search.
func findSeparator(body string, from int, inConditional bool) (separator, bool) {
	ifDepth, forDepth := 0, 0
	for i := from; i < len(body); {
		rest := body[i:]
		switch {
		case strings.HasPrefix(rest, ifOpen):
			ifDepth++
			i += len(ifOpen)
		case strings.HasPrefix(rest, ifClose):
			if inConditional && ifDepth == 0 {
				return separator{}, false
			}
			if ifDepth > 0 {
				ifDepth--
			}
			i += len(ifClose)
		case strings.HasPrefix(rest, forOpen):
			forDepth++
			i += len(forOpen)
		case strings.HasPrefix(rest, forClose):
			if !inConditional && forDepth == 0 {
				return separator{}, false
			}
			if forDepth > 0 {
				forDepth--
			}
			i += len(forClose)
		case ifDepth == 0 && forDepth == 0 && (inConditional && strings.HasPrefix(rest, elifOpen) || strings.HasPrefix(rest, elseOpen)):
			tagEnd := findTagEnd(rest)
			if tagEnd < 0 {
				return separator{}, false
			}
			return separator{pos: i, length: tagEnd + 1, isElse: strings.HasPrefix(rest, elseOpen)}, true
		default:
			i++
		}
	}
	return separator{}, false
}

// parseCondition reads the condition of a t-if or t-elif tag. The subject is
// the first of var, attr and slot present; with none it is the existence of
// the empty variable name.
func parseCondition(tag string) condition {
	for _, src := range []struct {
		attr   string
		source conditionSource
	}{{"var", sourceVariable}, {"attr", sourceAttr}, {"slot", sourceSlot}} {
		if name, ok := attrValue(tag, src.attr); ok {
			kind, operand := parseComparison(tag)
			return condition{source: src.source, name: name, comparison: kind, operand: operand}
		}
	}
	return condition{source: sourceVariable, comparison: compareExists}
}

// parseComparison reads a condition's comparison, in upstream's priority
// order: the valued comparisons first, then not-exists, truthy, not-truthy,
// and otherwise existence.
func parseComparison(tag string) (comparisonKind, string) {
	for _, c := range []struct {
		attr string
		kind comparisonKind
	}{
		{"equals", compareEquals},
		{"not-equals", compareNotEquals},
		{"contains", compareContains},
		{"starts-with", compareStartsWith},
		{"ends-with", compareEndsWith},
		{"matches", compareMatches},
	} {
		if v, ok := attrValue(tag, c.attr); ok {
			return c.kind, v
		}
	}
	switch {
	case hasBoolAttr(tag, "not-exists"):
		return compareNotExists, ""
	case hasBoolAttr(tag, "truthy"):
		return compareTruthy, ""
	case hasBoolAttr(tag, "not-truthy"):
		return compareNotTruthy, ""
	}
	return compareExists, ""
}

// parseUnsigned parses s as Zig's std.fmt.parseInt(u64, s, 10) does: an
// optional '+' or '-' sign, then decimal digits with single or repeated
// '_' separators that neither start nor end them. A '-' allows only zero.
// It reports false on any other input or on overflow.
func parseUnsigned(s string) (uint64, bool) {
	negative := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" || s[0] == '_' || s[len(s)-1] == '_' {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		d := uint64(c - '0')
		if n > (^uint64(0)-d)/10 {
			return 0, false
		}
		n = n*10 + d
	}
	if negative && n != 0 {
		return 0, false
	}
	return n, true
}

// parseUnsignedModifier reads the optional unsigned attribute name of a
// t-for tag opening at pos.
func (p *parser) parseUnsignedModifier(tag, name string, pos int) (*uint64, error) {
	v, ok := attrValue(tag, name)
	if !ok {
		return nil, nil
	}
	n, ok := parseUnsigned(v)
	if !ok {
		return nil, p.errorAt(pos, MalformedElement, "invalid integer value for attribute")
	}
	return &n, nil
}

// extractWord returns the bytes of input from start up to a space, '>' or
// '/'.
func extractWord(input string, start int) string {
	end := start
	for end < len(input) && input[end] != ' ' && input[end] != '>' && input[end] != '/' {
		end++
	}
	return input[start:end]
}

// parseLoopHeader reads the item, collection, alias and modifiers of the
// t-for tag opening at pos.
func (p *parser) parseLoopHeader(tag string, pos int) (*loop, error) {
	space := strings.IndexByte(tag[len(forOpen):], ' ')
	if space < 0 {
		return nil, p.errorAt(pos, MalformedElement, "missing item variable in <t-for>")
	}
	inTok := strings.Index(tag, " in ")
	if inTok < 0 {
		return nil, p.errorAt(pos, MalformedElement, "missing 'in' keyword in <t-for>")
	}
	l := &loop{
		pos:        pos,
		item:       tag[len(forOpen) : len(forOpen)+space],
		collection: extractWord(tag, inTok+4),
	}
	if as := strings.Index(tag[inTok+4:], " as "); as >= 0 {
		l.alias = extractWord(tag, inTok+4+as+4)
	}
	l.sortField, l.sorted = attrValue(tag, "sort")
	if order, ok := attrValue(tag, "order"); ok {
		l.descending = order == "desc"
	}
	var err error
	if l.limit, err = p.parseUnsignedModifier(tag, "limit", pos); err != nil {
		return nil, err
	}
	if l.offset, err = p.parseUnsignedModifier(tag, "offset", pos); err != nil {
		return nil, err
	}
	return l, nil
}

// parseLoop parses the t-for element opening at start, within source[:end],
// and returns it with the offset just past it. A t-else at loop level zero
// splits its body from its else body.
func (p *parser) parseLoop(start, end int) (node, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return node{}, 0, p.errorAt(start, MalformedElement, "unclosed <t-for> tag")
	}
	l, err := p.parseLoopHeader(rest[:tagEnd+1], start)
	if err != nil {
		return node{}, 0, err
	}
	bodyStart := start + tagEnd + 1
	closePos := findMatchingClose(p.source[bodyStart:end], forOpen, forClose)
	if closePos < 0 {
		return node{}, 0, p.errorAt(start, MalformedElement, "missing closing </t-for> tag")
	}
	bodyEnd := bodyStart + closePos
	sep, hasElse := findSeparator(p.source[bodyStart:bodyEnd], 0, false)
	mainEnd := bodyEnd
	if hasElse {
		mainEnd = bodyStart + sep.pos
	}
	if l.body, err = p.parseContent(bodyStart, mainEnd); err != nil || p.deferred {
		return node{}, 0, err
	}
	if hasElse {
		if l.elseBody, err = p.parseContent(mainEnd+sep.length, bodyEnd); err != nil || p.deferred {
			return node{}, 0, err
		}
	}
	return node{kind: loopNode, loop: l}, bodyEnd + len(forClose), nil
}

// parseConditional parses the t-if element opening at start, within
// source[:end], with its t-elif and t-else branches, and returns it with the
// offset just past it. Every branch is parsed, whichever will render.
func (p *parser) parseConditional(start, end int) (node, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return node{}, 0, p.errorAt(start, MalformedElement, "unclosed <t-if> tag")
	}
	bodyStart := start + tagEnd + 1
	closePos := findMatchingClose(p.source[bodyStart:end], ifOpen, ifClose)
	if closePos < 0 {
		return node{}, 0, p.errorAt(start, MalformedElement, "missing closing </t-if> tag")
	}
	bodyEnd := bodyStart + closePos
	body := p.source[bodyStart:bodyEnd]
	c := &conditional{pos: start}

	tag := rest[:tagEnd+1]
	from := 0
	for {
		sep, more := findSeparator(body, from, true)
		branchEnd := len(body)
		if more {
			branchEnd = sep.pos
		}
		nodes, err := p.parseContent(bodyStart+from, bodyStart+branchEnd)
		if err != nil || p.deferred {
			return node{}, 0, err
		}
		c.branches = append(c.branches, conditionalBranch{condition: parseCondition(tag), body: nodes})
		if !more {
			break
		}
		from = sep.pos + sep.length
		if sep.isElse {
			if c.elseBody, err = p.parseContent(bodyStart+from, bodyEnd); err != nil || p.deferred {
				return node{}, 0, err
			}
			break
		}
		tag = body[sep.pos:from]
	}
	return node{kind: conditionalNode, conditional: c}, bodyEnd + len(ifClose), nil
}

// parseLet parses the t-let element opening at start, within source[:end],
// and returns it with the offset just past it. Its body ends at the first
// </t-let>, without regard to nesting. A transform= attribute is parsed as
// on t-var; its names are checked only when the binding renders.
func (p *parser) parseLet(start, end int) (node, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return node{}, 0, p.errorAt(start, MalformedElement, "unclosed <t-let> tag")
	}
	tag := rest[:tagEnd+1]
	name, ok := attrValue(tag, "name")
	if !ok {
		return node{}, 0, p.errorAt(start, MalformedElement, "missing 'name' attribute on <t-let>")
	}
	bodyStart := start + tagEnd + 1
	closePos := strings.Index(p.source[bodyStart:end], letCloser)
	if closePos < 0 {
		return node{}, 0, p.errorAt(start, MalformedElement, "missing closing </t-let> tag")
	}
	body, err := p.parseContent(bodyStart, bodyStart+closePos)
	if err != nil || p.deferred {
		return node{}, 0, err
	}
	l := &letBinding{pos: start, name: name, body: body}
	if spec, ok := attrValue(tag, "transform"); ok {
		l.transforms = parseTransformSpec(spec)
	}
	return node{kind: letNode, let: l}, bodyStart + closePos + len(letCloser), nil
}
