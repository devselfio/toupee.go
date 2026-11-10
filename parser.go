package toupee

import "strings"

// Openers recognised by the parser, as Toupee v0.4.0 spells them. Matching
// is on exact, case-sensitive prefixes.
var (
	varOpeners     = []string{"<t-var ", "<t-var/>", "<t-var>"}
	rawOpeners     = []string{"<t-raw ", "<t-raw/>", "<t-raw>"}
	commentOpener  = "<t-comment"
	varCloser      = "</t-var>"
	rawCloser      = "</t-raw>"
	commentCloser  = "</t-comment>"
	deferredOpener = []string{
		"<t-let ", "<t-let>",
		"<t-debug",
		"<t-attr ", "<t-attr/>",
		"<t-slot",
		"<t-include ",
		"<t-for ",
		"<t-if ", "<t-if>",
		"<t-else",
		"<t-elif ",
		"<t-extend ", "<t-extend>",
		"<t-define",
	}
)

// parser turns template source into nodes. Every position it records is an
// absolute byte offset in source.
type parser struct {
	source   string
	deferred bool
}

// parse parses source. When it meets a construct this port does not yet
// implement (control flow, let, debug, attributes, slots, composition, or
// a t-var:/t-attr: binding on an ordinary tag) it stops and returns a
// result marked deferred; Render then keeps the bootstrap behaviour for the
// whole input. A malformed supported element met before that is still an
// error.
func parse(source string) (parsedTemplate, error) {
	p := &parser{source: source}
	nodes, err := p.parseContent(0, len(source))
	if err != nil {
		return parsedTemplate{}, err
	}
	if p.deferred {
		return parsedTemplate{deferred: true}, nil
	}
	return parsedTemplate{nodes: nodes}, nil
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
// recognised element as literal text. It stops early once p.deferred is set.
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
		switch {
		case hasAnyPrefix(rest, varOpeners), hasAnyPrefix(rest, rawOpeners):
			flush(i)
			v, next, err := p.parseVariable(i, end, strings.HasPrefix(rest, "<t-raw"))
			if err != nil {
				return nil, err
			}
			if p.deferred {
				return nil, nil
			}
			nodes = append(nodes, node{kind: variableNode, variable: v})
			i, textStart = next, next
		case strings.HasPrefix(rest, commentOpener):
			flush(i)
			next, err := p.parseComment(i, end)
			if err != nil {
				return nil, err
			}
			i, textStart = next, next
		case hasAnyPrefix(rest, deferredOpener), isBoundTag(rest):
			p.deferred = true
			return nil, nil
		default:
			i++
		}
	}
	flush(end)
	return nodes, nil
}

// isBoundTag reports whether rest starts an ordinary opening tag carrying a
// t-var: or t-attr: attribute binding. As in upstream's parseBoundTag, a
// binding starts only where a space at index > 0 is followed by t-var: or
// t-attr:; any other occurrence (an attribute value such as title="t-var:",
// or a name such as data-t-var:x) leaves the tag literal. A tag without any
// binding renders as literal text upstream, so it must not defer here.
func isBoundTag(rest string) bool {
	if len(rest) < 2 || rest[1] == '/' || rest[1] == '!' {
		return false
	}
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return false
	}
	tag := rest[:tagEnd+1]
	for i := 1; i+1 < len(tag); i++ {
		if tag[i] == ' ' && (strings.HasPrefix(tag[i+1:], "t-var:") || strings.HasPrefix(tag[i+1:], "t-attr:")) {
			return true
		}
	}
	return false
}

// parseVariable parses the t-var or t-raw element opening at start, within
// source[:end], and returns it with the offset just past it. A block body
// ends at the first exact closing tag of the same kind, without regard to
// nesting, and is parsed whether or not it will be rendered.
func (p *parser) parseVariable(start, end int, raw bool) (variable, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return variable{}, 0, errorAt(p.source, start, MalformedElement, "unclosed variable tag")
	}
	selfClosing := tagEnd > 0 && rest[tagEnd-1] == '/'
	tag := rest[:tagEnd+1]
	name, ok := attrValue(tag, "name")
	if !ok {
		return variable{}, 0, errorAt(p.source, start, MalformedElement, "missing 'name' attribute")
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
			return variable{}, 0, errorAt(p.source, start, MalformedElement, "missing closing tag")
		}
		body, err := p.parseContent(bodyStart, bodyStart+closePos)
		if err != nil || p.deferred {
			return variable{}, 0, err
		}
		v.body, v.hasBody = body, true
		next = bodyStart + closePos + len(closer)
	}
	return v, next, nil
}

// parseComment consumes the t-comment element opening at start, within
// source[:end], without looking at its body, and returns the offset just
// past it. A self-closing comment consumes only its opening tag.
func (p *parser) parseComment(start, end int) (int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return 0, errorAt(p.source, start, MalformedElement, "unclosed <t-comment> tag")
	}
	if tagEnd > 0 && rest[tagEnd-1] == '/' {
		return start + tagEnd + 1, nil
	}
	bodyStart := start + tagEnd + 1
	closePos := strings.Index(p.source[bodyStart:end], commentCloser)
	if closePos < 0 {
		return 0, errorAt(p.source, start, MalformedElement, "missing closing </t-comment> tag")
	}
	return bodyStart + closePos + len(commentCloser), nil
}
