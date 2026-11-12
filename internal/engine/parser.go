package engine

import "strings"

// Openers recognised by the parser, as Toupee v0.4.0 spells them. Matching
// is on exact, case-sensitive prefixes.
var (
	varOpeners    = []string{"<t-var ", "<t-var/>", "<t-var>"}
	rawOpeners    = []string{"<t-raw ", "<t-raw/>", "<t-raw>"}
	commentOpener = "<t-comment"
	debugOpener   = "<t-debug"
	attrOpeners   = []string{"<t-attr ", "<t-attr/>"}
	slotOpener    = "<t-slot"
	includeOpener = "<t-include "
	extendOpeners = []string{"<t-extend ", "<t-extend>"}
	varCloser     = "</t-var>"
	rawCloser     = "</t-raw>"
	commentCloser = "</t-comment>"
)

// parser turns template source into nodes. Every position it records is an
// absolute byte offset in source.
//
// A fragment parser parses text derived from another source, such as a
// definition body with its indentation stripped. Its offsets map each byte
// of source to its offset in origin, and its errors are reported there.
type parser struct {
	source  string
	origin  string
	offsets []int
}

// parse parses source. A source whose first non-whitespace bytes open a
// t-extend is an extend: its remaining content is only definitions.
func parse(source string) (parsedTemplate, error) {
	p := &parser{source: source}
	start := len(source) - len(strings.TrimLeft(source, " \t\r\n"))
	if hasAnyPrefix(source[start:], extendOpeners) {
		ext, err := p.parseExtend(start, len(source))
		if err != nil {
			return parsedTemplate{}, err
		}
		return parsedTemplate{nodes: []node{{kind: extendNode, extension: ext}}, source: source}, nil
	}
	nodes, err := p.parseContent(0, len(source))
	if err != nil {
		return parsedTemplate{}, err
	}
	return parsedTemplate{nodes: nodes, source: source}, nil
}

// fragment returns a parser for text, whose byte i came from offset
// offsets[i] of p.source.
func (p *parser) fragment(text string, offsets []int) *parser {
	if p.offsets == nil {
		return &parser{source: text, origin: p.source, offsets: offsets}
	}
	mapped := make([]int, len(offsets))
	for i, o := range offsets {
		mapped[i] = p.originPos(o)
	}
	return &parser{source: text, origin: p.origin, offsets: mapped}
}

// originPos maps an offset in p.source to the source errors are reported in.
func (p *parser) originPos(pos int) int {
	switch {
	case p.offsets == nil:
		return pos
	case pos < len(p.offsets):
		return p.offsets[pos]
	case len(p.offsets) == 0:
		return 0
	}
	return p.offsets[len(p.offsets)-1] + 1
}

// errorAt returns a RenderError for the offset pos in p.source, located in
// the source the parser's text came from.
func (p *parser) errorAt(pos int, kind ErrorKind, message string) *RenderError {
	if p.offsets == nil {
		return errorAt(p.source, pos, kind, message)
	}
	return errorAt(p.origin, p.originPos(pos), kind, message)
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// parseContent parses source[start:end], keeping every byte that is not a
// recognised element as literal text. Comments and t-debug yield no node.
func (p *parser) parseContent(start, end int) ([]node, error) {
	var nodes []node
	textStart := start
	flush := func(to int) {
		if to > textStart {
			nodes = append(nodes, node{kind: textNode, text: p.source[textStart:to]})
		}
	}
	for i := start; i < end; {
		if p.source[i] != '<' {
			i++
			continue
		}
		rest := p.source[i:end]
		var n node
		var next int
		var err error
		switch {
		case hasAnyPrefix(rest, varOpeners), hasAnyPrefix(rest, rawOpeners):
			n.kind = variableNode
			n.variable, next, err = p.parseVariable(i, end, strings.HasPrefix(rest, "<t-raw"))
		case strings.HasPrefix(rest, commentOpener):
			flush(i)
			if next, err = p.parseComment(i, end); err != nil {
				return nil, err
			}
			i, textStart = next, next
			continue
		case strings.HasPrefix(rest, debugOpener):
			flush(i)
			if next, err = p.parseDebug(i, end); err != nil {
				return nil, err
			}
			i, textStart = next, next
			continue
		case hasAnyPrefix(rest, attrOpeners):
			n.kind = attrOutputNode
			n.attrOutput, next, err = p.parseAttrOutput(i, end)
		case strings.HasPrefix(rest, slotOpener):
			n.kind = slotNode
			n.slot, next, err = p.parseSlot(i, end)
		case strings.HasPrefix(rest, includeOpener):
			n.kind = includeNode
			n.include, next, err = p.parseInclude(i, end)
		case hasAnyPrefix(rest, letOpeners):
			n, next, err = p.parseLet(i, end)
		case strings.HasPrefix(rest, forOpen):
			n, next, err = p.parseLoop(i, end)
		case hasAnyPrefix(rest, ifOpeners):
			n, next, err = p.parseConditional(i, end)
		case hasAnyPrefix(rest, strayOpener):
			return nil, p.errorAt(i, MalformedElement, "stray <t-else> or <t-elif> outside conditional or loop")
		case boundTagEnd(rest) >= 0:
			n.kind = boundTagNode
			n.boundTag, next, err = p.parseBoundTag(i, end)
		default:
			i++
			continue
		}
		if err != nil {
			return nil, err
		}
		flush(i)
		nodes = append(nodes, n)
		i, textStart = next, next
	}
	flush(end)
	return nodes, nil
}

// parseVariable parses the t-var or t-raw element opening at start, within
// source[:end], and returns it with the offset just past it. A block body
// ends at the first exact closing tag of the same kind, without regard to
// nesting, and is parsed whether or not it will be rendered.
func (p *parser) parseVariable(start, end int, raw bool) (variable, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return variable{}, 0, p.errorAt(start, MalformedElement, "unclosed variable tag")
	}
	selfClosing := tagEnd > 0 && rest[tagEnd-1] == '/'
	tag := rest[:tagEnd+1]
	name, ok := attrValue(tag, "name")
	if !ok {
		return variable{}, 0, p.errorAt(start, MalformedElement, "missing 'name' attribute")
	}
	v := variable{name: name, raw: raw, pos: start}
	if spec, ok := attrValue(tag, "transform"); ok {
		v.transforms = parseTransformSpec(spec)
	}
	next := start + tagEnd + 1
	if !selfClosing {
		closer := varCloser
		if raw {
			closer = rawCloser
		}
		bodyStart := start + tagEnd + 1
		closePos := strings.Index(p.source[bodyStart:end], closer)
		if closePos < 0 {
			return variable{}, 0, p.errorAt(start, MalformedElement, "missing closing tag")
		}
		body, err := p.parseContent(bodyStart, bodyStart+closePos)
		if err != nil {
			return variable{}, 0, err
		}
		v.body, v.hasBody = body, true
		next = bodyStart + closePos + len(closer)
	}
	return v, next, nil
}

// parseDebug consumes the opening tag of the t-debug element at start,
// within source[:end], and returns the offset just past it. It does not look
// for a closing tag or a body: Toupee treats only the opening tag as the
// element. This port has no debug mode, so the element renders as Toupee
// renders it with debug off, as nothing, and yields no node.
func (p *parser) parseDebug(start, end int) (int, error) {
	tagEnd := findTagEnd(p.source[start:end])
	if tagEnd < 0 {
		return 0, p.errorAt(start, MalformedElement, "unclosed <t-debug> tag")
	}
	return start + tagEnd + 1, nil
}

// parseComment consumes the t-comment element opening at start, within
// source[:end], without looking at its body, and returns the offset just
// past it. A self-closing comment consumes only its opening tag.
func (p *parser) parseComment(start, end int) (int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return 0, p.errorAt(start, MalformedElement, "unclosed <t-comment> tag")
	}
	if tagEnd > 0 && rest[tagEnd-1] == '/' {
		return start + tagEnd + 1, nil
	}
	bodyStart := start + tagEnd + 1
	closePos := strings.Index(p.source[bodyStart:end], commentCloser)
	if closePos < 0 {
		return 0, p.errorAt(start, MalformedElement, "missing closing </t-comment> tag")
	}
	return bodyStart + closePos + len(commentCloser), nil
}
