# Corpus origin

The 22 `*.test` files here are Toupee's direct `test/*.test` files at
v0.4.0, copied byte for byte. They contain 519 cases. Never edit them
locally: a failing case belongs in the implementation or the explicit
known-failures list, not in a rewritten fixture.

Repository: https://github.com/devselfio/toupee
Ref: v0.4.0

The pin must be a vN.N.N tag, never a branch or commit. SHA256SUMS records
that tag with a first line `# Ref: <tag>`, then sorted lowercase SHA-256
digests, two spaces and a basename. `sha256sum -c SHA256SUMS` verifies
the bytes when run in this directory.

`make spec-drift` checks the local set and hashes against SHA256SUMS
and, when online, against the upstream tag. A transport failure before
any HTTP response falls back to the record with a notice; an HTTP error
or corrupt archive fails. A record for a different tag fails offline
and online, even when the fixture bytes happen to be unchanged.

To move the pin, replace the complete fixture set with the new tag's
direct test/*.test files, update Ref, then run:

    go run ./cmd/driftcheck -record

Recording requires the network and writes SHA256SUMS only after all
local files match upstream. Reassess case counts, identities and the
known-failures list explicitly when updating the pin.

To measure another ref without changing the pin or record, run:

    go run ./cmd/driftcheck main

This measurement requires the network; a branch is allowed as the
measurement argument, never as the ORIGIN pin. The known-failures file
and this documentation are local metadata, excluded from *.test hashes.
