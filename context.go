package toupee

import "maps"

// cloneContext copies ctx's three top-level maps, so a render can write to
// its copy without touching the caller's. A nil map stays nil and an empty
// one stays empty. Nested Values are read-only and are shared.
func cloneContext(ctx Context) Context {
	return Context{
		Data:  maps.Clone(ctx.Data),
		Attrs: maps.Clone(ctx.Attrs),
		Slots: maps.Clone(ctx.Slots),
	}
}
