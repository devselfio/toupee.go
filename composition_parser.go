package toupee

import "strings"

const (
	slotCloser    = "</t-slot>"
	includeCloser = "</t-include>"
	defineCloser  = "</t-define>"
)

// parseAttrOutput parses the self-closing t-attr element opening at start,
// within source[:end]. Its tag ends at the first "/>".
func (p *parser) parseAttrOutput(start, end int) (attrOutput, int, error) {
	rest := p.source[start:end]
	close := strings.Index(rest, "/>")
	if close < 0 {
		return attrOutput{}, 0, p.errorAt(start, MalformedElement, "unclosed <t-attr> tag")
	}
	name, ok := attrValue(rest[:close+2], "name")
	if !ok {
		return attrOutput{}, 0, p.errorAt(start, MalformedElement, "missing 'name' attribute on <t-attr>")
	}
	return attrOutput{name: name, pos: start}, start + close + 2, nil
}

// parseSlot parses the t-slot element opening at start, within
// source[:end]. A missing name is the anonymous slot "". A block's body is
// the default content. One LF or CRLF right after the element is consumed.
func (p *parser) parseSlot(start, end int) (slot, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return slot{}, 0, p.errorAt(start, MalformedElement, "unclosed <t-slot> tag")
	}
	tag := rest[:tagEnd+1]
	name, _ := attrValue(tag, "name")
	s := slot{name: name, pos: start}
	next := start + tagEnd + 1
	if tagEnd == 0 || rest[tagEnd-1] != '/' {
		bodyStart := next
		closePos := strings.Index(p.source[bodyStart:end], slotCloser)
		if closePos < 0 {
			return slot{}, 0, p.errorAt(start, MalformedElement, "missing closing </t-slot> tag")
		}
		body, err := p.parseContent(bodyStart, bodyStart+closePos)
		if err != nil {
			return slot{}, 0, err
		}
		s.body = body
		next = bodyStart + closePos + len(slotCloser)
	}
	switch {
	case strings.HasPrefix(p.source[next:end], "\r\n"):
		next += 2
	case strings.HasPrefix(p.source[next:end], "\n"):
		next++
	}
	return s, next, nil
}

// parseInclude parses the t-include element opening at start, within
// source[:end]. A block's body has its common indentation stripped before
// its definitions and anonymous fill are taken from it.
func (p *parser) parseInclude(start, end int) (include, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return include{}, 0, p.errorAt(start, MalformedElement, "unclosed <t-include> tag")
	}
	tag := rest[:tagEnd+1]
	name, ok := attrValue(tag, "template")
	if !ok {
		return include{}, 0, p.errorAt(start, MalformedElement, "missing 'template' attribute on <t-include>")
	}
	inc := include{template: name, attrs: parseIncludeAttrs(tag), pos: start}
	next := start + tagEnd + 1
	if rest[tagEnd-1] != '/' {
		bodyStart := next
		closePos := strings.Index(p.source[bodyStart:end], includeCloser)
		if closePos < 0 {
			return include{}, 0, p.errorAt(start, MalformedElement, "missing closing </t-include> tag")
		}
		next = bodyStart + closePos + len(includeCloser)
		body, offsets := stripCommonIndentMap(p.source[bodyStart : bodyStart+closePos])
		if body != "" {
			for i := range offsets {
				offsets[i] += bodyStart
			}
			fp := p.fragment(body, offsets)
			res, err := fp.parseDefinitions(0, len(body))
			if err != nil {
				return include{}, 0, err
			}
			inc.defines, inc.anonymousSource = res.defines, res.anonymousSource
		}
	}
	inc.isolated = hasBoolAttr(tag, "isolated")
	if spec, ok := attrValue(tag, "context"); ok {
		inc.bindings = parseContextBindings(spec)
	}
	return inc, next, nil
}

// parseExtend parses the t-extend element opening at start and the
// definitions after it, through end. No closing tag is needed, and content
// outside definitions is ignored.
func (p *parser) parseExtend(start, end int) (extension, error) {
	tagEnd := findTagEnd(p.source[start:end])
	if tagEnd < 0 {
		return extension{}, p.errorAt(start, MalformedElement, "unclosed <t-extend> tag")
	}
	name, ok := attrValue(p.source[start:start+tagEnd+1], "template")
	if !ok {
		return extension{}, p.errorAt(start, MalformedElement, "missing 'template' attribute on <t-extend>")
	}
	res, err := p.parseDefinitions(start+tagEnd+1, end)
	if err != nil {
		return extension{}, err
	}
	return extension{template: name, defines: res.defines, pos: start}, nil
}

// parseDefinitions parses source[start:end] as t-define elements and
// anonymous content. A definition names its slot with name, else slot,
// else is the anonymous slot "". A repeated name is a
// DuplicateSlotDefinition at the second opener. Bodies are parsed for their
// syntax only; their indentation-stripped source is what fills a slot. The
// other bytes, trimmed of spaces, tabs, CRs and LFs, are the anonymous fill.
func (p *parser) parseDefinitions(start, end int) (defineResult, error) {
	var res defineResult
	seen := map[string]bool{}
	var anon []byte
	var anonOffsets []int
	for i := start; i < end; {
		rest := p.source[i:end]
		if !strings.HasPrefix(rest, "<t-define>") && !strings.HasPrefix(rest, "<t-define ") {
			anon = append(anon, p.source[i])
			anonOffsets = append(anonOffsets, i)
			i++
			continue
		}
		def, next, err := p.parseDefinition(i, end)
		if err != nil {
			return defineResult{}, err
		}
		if seen[def.name] {
			return defineResult{}, p.errorAt(i, DuplicateSlotDefinition, def.name)
		}
		seen[def.name] = true
		res.defines = append(res.defines, def)
		i = next
	}
	lo, hi := 0, len(anon)
	for lo < hi && strings.IndexByte(" \t\r\n", anon[lo]) >= 0 {
		lo++
	}
	for hi > lo && strings.IndexByte(" \t\r\n", anon[hi-1]) >= 0 {
		hi--
	}
	if lo < hi {
		src := string(anon[lo:hi])
		fp := p.fragment(src, anonOffsets[lo:hi])
		if _, err := fp.parseContent(0, len(src)); err != nil {
			return defineResult{}, err
		}
		res.anonymousSource = src
	}
	return res, nil
}

// parseDefinition parses the t-define element opening at start, within
// source[:end], and returns it with the offset just past it.
func (p *parser) parseDefinition(start, end int) (definition, int, error) {
	rest := p.source[start:end]
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return definition{}, 0, p.errorAt(start, MalformedElement, "unclosed <t-define> tag")
	}
	tag := rest[:tagEnd+1]
	name, ok := attrValue(tag, "name")
	if !ok {
		name, _ = attrValue(tag, "slot")
	}
	bodyStart := start + tagEnd + 1
	closePos := strings.Index(p.source[bodyStart:end], defineCloser)
	if closePos < 0 {
		return definition{}, 0, p.errorAt(start, MalformedElement, "missing closing </t-define> tag")
	}
	src, offsets := stripCommonIndentMap(p.source[bodyStart : bodyStart+closePos])
	for i := range offsets {
		offsets[i] += bodyStart
	}
	fp := p.fragment(src, offsets)
	body, err := fp.parseContent(0, len(src))
	if err != nil {
		return definition{}, 0, err
	}
	def := definition{name: name, source: src, body: body, pos: start}
	return def, bodyStart + closePos + len(defineCloser), nil
}

// boundTagEnd returns the index of the '>' ending the ordinary opening tag
// rest starts with when the tag contains t-var: or t-attr:, else -1.
// Closing tags and <! constructs are never bound.
func boundTagEnd(rest string) int {
	if len(rest) < 2 || rest[1] == '/' || rest[1] == '!' {
		return -1
	}
	tagEnd := findTagEnd(rest)
	if tagEnd < 0 {
		return -1
	}
	tag := rest[:tagEnd+1]
	if !strings.Contains(tag, "t-var:") && !strings.Contains(tag, "t-attr:") {
		return -1
	}
	return tagEnd
}

// parseBoundTag parses the opening tag at start, within source[:end], into
// literal segments and bindings. A binding starts where an ASCII space
// after the first byte is followed by t-var: or t-attr:, and runs through
// ATTR="REF"; the space belongs to the binding. Everything else, including
// t-var: in an attribute value or within a name, is literal.
func (p *parser) parseBoundTag(start, end int) (boundTag, int, error) {
	tagEnd := findTagEnd(p.source[start:end])
	if tagEnd < 0 {
		return boundTag{}, 0, p.errorAt(start, MalformedElement, "unclosed tag")
	}
	tag := p.source[start : start+tagEnd+1]
	var segs []tagSegment
	literalStart := 0
	for i := 1; i < len(tag); {
		if tag[i] != ' ' || i+1 >= len(tag) {
			i++
			continue
		}
		var prefix string
		switch {
		case strings.HasPrefix(tag[i+1:], "t-var:"):
			prefix = "t-var:"
		case strings.HasPrefix(tag[i+1:], "t-attr:"):
			prefix = "t-attr:"
		default:
			i++
			continue
		}
		if i > literalStart {
			segs = append(segs, tagSegment{literal: tag[literalStart:i]})
		}
		j := i + 1 + len(prefix)
		attrStart := j
		for j < len(tag) && tag[j] != '=' {
			j++
		}
		htmlAttr := tag[attrStart:j]
		j = min(j+2, len(tag)) // skip `="`
		refStart := j
		for j < len(tag) && tag[j] != '"' {
			j++
		}
		ref := tag[refStart:j]
		j = min(j+1, len(tag))
		segs = append(segs, tagSegment{binding: &attributeBinding{htmlAttr: htmlAttr, refName: ref, variable: prefix == "t-var:"}})
		i, literalStart = j, j
	}
	if literalStart < len(tag) {
		segs = append(segs, tagSegment{literal: tag[literalStart:]})
	}
	return boundTag{segments: segs, pos: start}, start + tagEnd + 1, nil
}

// parseIncludeAttrs returns the attributes of an include tag other than
// template, isolated and context. Values are the literal bytes between
// double quotes; an attribute without a value is "". A later attribute of
// the same name wins.
func parseIncludeAttrs(tag string) map[string]string {
	attrs := map[string]string{}
	i := 1
	for i < len(tag) && tag[i] != ' ' && tag[i] != '/' && tag[i] != '>' {
		i++
	}
	reserved := func(name string) bool { return name == "template" || name == "isolated" || name == "context" }
	for i < len(tag) {
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		if i >= len(tag) || tag[i] == '/' || tag[i] == '>' {
			break
		}
		nameStart := i
		for i < len(tag) && tag[i] != '=' && tag[i] != ' ' && tag[i] != '/' && tag[i] != '>' {
			i++
		}
		name := tag[nameStart:i]
		if i < len(tag) && tag[i] == '=' {
			i++
			if i < len(tag) && tag[i] == '"' {
				i++
				valStart := i
				for i < len(tag) && tag[i] != '"' {
					i++
				}
				value := tag[valStart:i]
				if i < len(tag) {
					i++
				}
				if !reserved(name) {
					attrs[name] = value
				}
			}
		} else if !reserved(name) {
			attrs[name] = ""
		}
	}
	return attrs
}

// parseContextBindings parses an include's context= value: comma-separated
// entries, trimmed of ASCII spaces, each "PATH as KEY" or a PATH bound to its
// last dot-separated component. Blank entries are ignored.
func parseContextBindings(spec string) []contextBinding {
	var bindings []contextBinding
	for entry := range strings.SplitSeq(spec, ",") {
		entry = strings.Trim(entry, " ")
		if entry == "" {
			continue
		}
		if path, key, ok := strings.Cut(entry, " as "); ok {
			bindings = append(bindings, contextBinding{path: strings.Trim(path, " "), key: strings.Trim(key, " ")})
			continue
		}
		key := entry
		if dot := strings.LastIndexByte(entry, '.'); dot >= 0 {
			key = entry[dot+1:]
		}
		bindings = append(bindings, contextBinding{path: entry, key: key})
	}
	return bindings
}
