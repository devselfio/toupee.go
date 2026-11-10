package toupee

import (
	"strconv"
	"strings"
)

// resolve looks path up in ctx.Data. A path containing "$(" is dynamic.
func resolve(ctx Context, path string) (Value, bool) {
	root := Value{Kind: KindMap, Map: ctx.Data}
	if strings.Contains(path, "$(") {
		return resolveDynamic(root, ctx, path)
	}
	return resolveStatic(root, path)
}

// resolveStatic follows dot-separated map keys and decimal list indices
// from current. An empty path is current itself; empty segments name empty
// map keys. A missing key, a bad or out-of-range index, or a step through a
// scalar is missing.
func resolveStatic(current Value, path string) (Value, bool) {
	if path == "" {
		return current, true
	}
	seg, rest, more := strings.Cut(path, ".")
	next, ok := child(current, seg)
	if !ok {
		return Value{}, false
	}
	if !more {
		return next, true
	}
	return resolveStatic(next, rest)
}

// child returns current's entry for key: a map key, or a list index.
func child(current Value, key string) (Value, bool) {
	switch current.Kind {
	case KindMap:
		v, ok := current.Map[key]
		return v, ok
	case KindList:
		idx, err := strconv.ParseUint(key, 10, 64)
		if err != nil || idx >= uint64(len(current.List)) {
			return Value{}, false
		}
		return current.List[idx], true
	}
	return Value{}, false
}

// resolveDynamic navigates path from current, where a $(expr) segment is
// replaced by the scalar that expr resolves to from the root of ctx, used as
// a single key. Empty segments are skipped.
func resolveDynamic(current Value, ctx Context, path string) (Value, bool) {
	for path != "" {
		seg, rest := nextPathSegment(path)
		path = rest
		if seg == "" {
			continue
		}
		key := seg
		if strings.HasPrefix(seg, "$(") {
			if len(seg) < 3 {
				return Value{}, false
			}
			inner, ok := resolve(ctx, seg[2:len(seg)-1])
			if !ok {
				return Value{}, false
			}
			if key, ok = scalarString(inner); !ok {
				return Value{}, false
			}
		}
		next, ok := child(current, key)
		if !ok {
			return Value{}, false
		}
		current = next
	}
	return current, true
}

// nextPathSegment splits off path's first segment. A segment starting with
// "$(" runs to its balanced ')', so dots inside it do not separate.
func nextPathSegment(path string) (segment, rest string) {
	if strings.HasPrefix(path, "$(") {
		i, depth := 2, 1
		for i < len(path) {
			c := path[i]
			i++
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		return path[:i], strings.TrimPrefix(path[i:], ".")
	}
	segment, rest, _ = strings.Cut(path, ".")
	return segment, rest
}
