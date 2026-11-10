package toupee

import (
	"slices"
	"strconv"
	"strings"
)

// evaluateCondition reports whether c holds in ctx. A missing variable is
// not an error: it fails every positive test and passes every negative one.
func evaluateCondition(c condition, ctx *renderContext) bool {
	switch c.source {
	case sourceSlot:
		_, present := ctx.slots[c.name]
		if c.comparison == compareNotExists || c.comparison == compareNotTruthy {
			return !present
		}
		return present
	case sourceAttr:
		v, present := ctx.attrs[c.name]
		return compare(c, present, v, v != "")
	}
	v, present := ctx.lookup(c.name)
	text, _ := scalarString(v) // a present nil, list or map compares as ""
	return compare(c, present, text, isTruthy(v))
}

// compare applies c's comparison to a subject that is present or not, with
// text as its string form and truthy as its truthiness.
func compare(c condition, present bool, text string, truthy bool) bool {
	switch c.comparison {
	case compareExists:
		return present
	case compareNotExists:
		return !present
	case compareTruthy:
		return present && truthy
	case compareNotTruthy:
		return !present || !truthy
	case compareNotEquals:
		return !present || text != c.operand
	}
	if !present {
		return false
	}
	switch c.comparison {
	case compareEquals:
		return text == c.operand
	case compareContains:
		return strings.Contains(text, c.operand)
	case compareStartsWith:
		return strings.HasPrefix(text, c.operand)
	case compareEndsWith:
		return strings.HasSuffix(text, c.operand)
	case compareMatches:
		return globMatch(text, c.operand)
	}
	return false
}

// isTruthy is Toupee's truthiness: a nonempty string, list or map, true, or
// a nonzero number. Nil is false.
func isTruthy(v Value) bool {
	switch v.Kind {
	case KindString:
		return v.String != ""
	case KindBoolean:
		return v.Boolean
	case KindInteger:
		return v.Integer != 0
	case KindFloat:
		return v.Float != 0
	case KindList:
		return len(v.List) > 0
	case KindMap:
		return len(v.Map) > 0
	}
	return false
}

// globMatch reports whether pattern matches all of text, byte by byte, as
// upstream's globMatch: '*' matches any run of bytes, '?' any one byte, and
// every other byte itself. A pattern byte equal to the text byte matches
// before it is treated as a wildcard. There are no classes or escapes.
func globMatch(text, pattern string) bool {
	ti, pi := 0, 0
	starPI, starTI := -1, 0
	for ti < len(text) {
		switch {
		case pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == text[ti]):
			ti++
			pi++
		case pi < len(pattern) && pattern[pi] == '*':
			starPI, starTI = pi, ti
			pi++
		case starPI >= 0:
			pi = starPI + 1
			starTI++
			ti = starTI
		default:
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// renderConditional renders the body of the first branch whose condition
// holds, else the else body, in ctx itself.
func (r *renderer) renderConditional(c *conditional, ctx *renderContext) (string, error) {
	for _, b := range c.branches {
		if evaluateCondition(b.condition, ctx) {
			return r.renderNodes(b.body, ctx)
		}
	}
	return r.renderNodes(c.elseBody, ctx)
}

// loopItems returns the items v iterates: a list, or the list under a map's
// "items" key.
func loopItems(v Value) ([]Value, bool) {
	switch v.Kind {
	case KindList:
		return v.List, true
	case KindMap:
		if items, ok := v.Map["items"]; ok && items.Kind == KindList {
			return items.List, true
		}
	}
	return nil, false
}

// sortLoopItems sorts items in place by the string at field in each, an
// absent or non-string key sorting as "". Order is bytewise, reversed when
// descending; equal keys keep their order.
func sortLoopItems(items []Value, field string, descending bool) {
	key := func(v Value) string {
		if k, ok := resolveStatic(v, field); ok && k.Kind == KindString {
			return k.String
		}
		return ""
	}
	slices.SortStableFunc(items, func(a, b Value) int {
		c := strings.Compare(key(a), key(b))
		if descending {
			return -c
		}
		return c
	})
}

// loopWindow skips offset items, then keeps at most limit. Either may be
// absent (nil) and either may exceed len(items).
func loopWindow(items []Value, limit, offset *uint64) []Value {
	if offset != nil {
		if *offset >= uint64(len(items)) {
			return items[len(items):]
		}
		items = items[*offset:]
	}
	if limit != nil && *limit < uint64(len(items)) {
		items = items[:*limit]
	}
	return items
}

// loopMetadata is the alias map for iteration index of total: index, number
// and length as decimal strings, and first and last as "true" only on the
// first and last iterations.
func loopMetadata(index, total int) Value {
	m := map[string]Value{
		"index":  {Kind: KindString, String: strconv.Itoa(index)},
		"number": {Kind: KindString, String: strconv.Itoa(index + 1)},
		"length": {Kind: KindString, String: strconv.Itoa(total)},
	}
	if index == 0 {
		m["first"] = Value{Kind: KindString, String: "true"}
	}
	if index == total-1 {
		m["last"] = Value{Kind: KindString, String: "true"}
	}
	return Value{Kind: KindMap, Map: m}
}

// renderLoop renders l's body once per item of its window, each in a fresh
// copy of ctx with the item, then the alias, bound. A missing or
// non-iterable collection, or an empty window, renders the else body in
// ctx. The collection itself is never reordered.
func (r *renderer) renderLoop(l *loop, ctx *renderContext) (string, error) {
	var window []Value
	if v, ok := ctx.lookup(l.collection); ok {
		if items, ok := loopItems(v); ok {
			items = slices.Clone(items)
			if l.sorted {
				sortLoopItems(items, l.sortField, l.descending)
			}
			window = loopWindow(items, l.limit, l.offset)
		}
	}
	if len(window) == 0 {
		return r.renderNodes(l.elseBody, ctx)
	}
	var b strings.Builder
	for i, item := range window {
		child := cloneRenderContext(ctx)
		child.data[l.item] = item
		if l.alias != "" {
			child.data[l.alias] = loopMetadata(i, len(window))
		}
		s, err := r.renderNodes(l.body, child)
		if err != nil {
			return "", err
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

// renderLet renders l's body in ctx, runs l's transform chain on the result
// and stores it as a string under l's literal name in ctx, replacing any
// earlier binding. It emits nothing and escapes nothing. A failing step is a
// MalformedElement at the t-let, and nothing is stored.
func (r *renderer) renderLet(l *letBinding, ctx *renderContext) error {
	s, err := r.renderNodes(l.body, ctx)
	if err != nil {
		return err
	}
	if s, err = r.applyTransformsAt(s, l.transforms, l.pos); err != nil {
		return err
	}
	ctx.data[l.name] = Value{Kind: KindString, String: s}
	return nil
}
