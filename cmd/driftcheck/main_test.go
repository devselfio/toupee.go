package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// offlineTransport fails every request before any HTTP response, as when
// there is no network, without dialing.
type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp: no network")
}

var offlineClient = &http.Client{Transport: offlineTransport{}}

// tarEntry is one member of a test archive.
type tarEntry struct {
	name     string
	body     string
	typeflag byte // 0 means a regular file
}

func tarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: e.typeflag}
		switch e.typeflag {
		case 0:
			h.Typeflag = tar.TypeReg
			h.Size = int64(len(e.body))
		case tar.TypeSymlink:
			h.Linkname = e.body
		case tar.TypeDir:
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// corpusArchive is a GitHub-style archive of files under toupee-x/test/.
func corpusArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	entries := []tarEntry{{name: "toupee-x/", typeflag: tar.TypeDir}, {name: "toupee-x/test/", typeflag: tar.TypeDir}}
	for n, b := range files {
		entries = append(entries, tarEntry{name: "toupee-x/test/" + n, body: b})
	}
	return tarGz(t, entries)
}

type response struct {
	status int
	body   []byte
}

// fakeUpstream serves archives at /archive/<ref>.tar.gz and counts requests.
type fakeUpstream struct {
	*httptest.Server
	refs     map[string]response
	requests atomic.Int64
}

func newUpstream(t *testing.T, refs map[string]response) *fakeUpstream {
	t.Helper()
	u := &fakeUpstream{refs: refs}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.requests.Add(1)
		ref, ok := strings.CutPrefix(r.URL.Path, "/archive/")
		ref, ok2 := strings.CutSuffix(ref, ".tar.gz")
		resp, found := u.refs[ref]
		if !ok || !ok2 || !found {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(resp.status)
		w.Write(resp.body)
	}))
	t.Cleanup(u.Close)
	return u
}

var baseFiles = map[string]string{
	"var.test":           "<!-- test: a -->\n<!-- in -->\nx\n<!-- out -->\nx\n",
	"types-compare.test": "crlf\r\nbytes\r\n",
	"edge.test":          "",
}

func ok(body []byte) response { return response{http.StatusOK, body} }

// setup writes ORIGIN.md pinning ref at repository, the files, and, when
// recordRef is not "", a record of the files at recordRef.
func setup(t *testing.T, repository, ref, recordRef string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	origin := "# Corpus origin\n\nRepository: " + repository + "\nRef: " + ref + "\n"
	write(t, dir, "ORIGIN.md", origin)
	b := map[string][]byte{}
	for n, c := range files {
		write(t, dir, n, c)
		b[n] = []byte(c)
	}
	if recordRef != "" {
		write(t, dir, recordName, string(formatRecord(recordRef, digest(b))))
	}
	return dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// snapshot returns every file in dir with its contents.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

type result struct {
	code           int
	stdout, stderr string
}

func runIn(dir string, client *http.Client, args ...string) result {
	var out, errb bytes.Buffer
	code := run(append([]string{"-dir", dir}, args...), client, &out, &errb)
	return result{code, out.String(), errb.String()}
}

// runReadOnly runs driftcheck and fails the test if dir changed.
func runReadOnly(t *testing.T, dir string, client *http.Client, args ...string) result {
	t.Helper()
	before := snapshot(t, dir)
	r := runIn(dir, client, args...)
	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("driftcheck %v changed %s", args, dir)
	}
	return r
}

func copyFiles(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestOfflineMatch(t *testing.T) {
	dir := setup(t, "https://example.invalid/toupee", "v0.4.0", "v0.4.0", baseFiles)
	r := runReadOnly(t, dir, offlineClient)
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "matches SHA256SUMS at v0.4.0 (3 files)") {
		t.Errorf("stdout %q lacks the record match", r.stdout)
	}
	if !strings.Contains(r.stderr, "offline") || !strings.Contains(r.stderr, "record only") {
		t.Errorf("stderr %q lacks the offline notice", r.stderr)
	}
}

func TestOfflineDrift(t *testing.T) {
	dir := setup(t, "https://example.invalid/toupee", "v0.4.0", "v0.4.0", baseFiles)
	write(t, dir, "var.test", "edited locally\n")       // changed
	write(t, dir, "bind.test", "local only\n")          // removed (local-only)
	os.Remove(filepath.Join(dir, "types-compare.test")) // added (record-only)
	write(t, dir, "notes.md", "not a corpus file\n")    // ignored
	os.Mkdir(filepath.Join(dir, "sub.test"), 0o755)     // not a regular file
	r := runReadOnly(t, dir, offlineClient)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1\n%s%s", r.code, r.stdout, r.stderr)
	}
	want := "removed\tbind.test\nadded\ttypes-compare.test\nchanged\tvar.test\n"
	if !strings.HasPrefix(r.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to start, sorted by name, with:\n%s", r.stdout, want)
	}
	if !strings.Contains(r.stdout, "3 files differ from SHA256SUMS at v0.4.0") {
		t.Errorf("stdout %q lacks the summary", r.stdout)
	}
}

// A record for another tag fails even when the bytes are unchanged.
func TestPinRecordMismatch(t *testing.T) {
	up := newUpstream(t, map[string]response{"v0.4.1": ok(corpusArchive(t, baseFiles))})
	for name, client := range map[string]*http.Client{"offline": offlineClient, "online": up.Client()} {
		t.Run(name, func(t *testing.T) {
			dir := setup(t, up.URL, "v0.4.1", "v0.4.0", baseFiles)
			r := runReadOnly(t, dir, client)
			if r.code != 1 {
				t.Fatalf("exit %d, want 1\n%s%s", r.code, r.stdout, r.stderr)
			}
			if !strings.Contains(r.stdout, "SHA256SUMS records v0.4.0 but ORIGIN.md pins v0.4.1") {
				t.Errorf("stdout %q lacks the pin mismatch", r.stdout)
			}
		})
	}
}

func TestUnableToCheck(t *testing.T) {
	record := string(formatRecord("v0.4.0", digest(map[string][]byte{"var.test": []byte(baseFiles["var.test"])})))
	tests := []struct {
		name    string
		origin  string // "" means no ORIGIN.md
		record  string // "" means no SHA256SUMS
		noFiles bool
		wantErr string
	}{
		{name: "no origin", origin: "", record: record, wantErr: "ORIGIN.md"},
		{name: "no ref line", origin: "Repository: https://x.invalid/t\n", record: record, wantErr: `no "Ref" line`},
		{name: "no repository line", origin: "Ref: v0.4.0\n", record: record, wantErr: `no "Repository" line`},
		{name: "two ref lines", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\nRef: v0.4.0\n", record: record, wantErr: "appears 2 times"},
		{name: "ref with space", origin: "Repository: https://x.invalid/t\nRef: v0.4.0 tag\n", record: record, wantErr: "without spaces"},
		{name: "ref without space after colon", origin: "Repository: https://x.invalid/t\nRef:v0.4.0\n", record: record, wantErr: "without spaces"},
		{name: "no record", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: "", wantErr: "no SHA256SUMS"},
		{name: "record without header", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: strings.SplitN(record, "\n", 2)[1], wantErr: "first line"},
		{name: "record header only", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: "# Ref: v0.4.0\n", wantErr: "records no files"},
		{name: "upper-case digest", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: "# Ref: v0.4.0\n" + strings.Repeat("A", 64) + "  var.test\n", wantErr: "SHA256SUMS:2"},
		{name: "one space", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: "# Ref: v0.4.0\n" + strings.Repeat("a", 64) + " var.test\n", wantErr: "SHA256SUMS:2"},
		{name: "short digest", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: "# Ref: v0.4.0\n" + strings.Repeat("a", 63) + "  var.test\n", wantErr: "SHA256SUMS:2"},
		{name: "duplicate entry", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: record + strings.SplitN(record, "\n", 2)[1], wantErr: "listed twice"},
		{name: "no test files", origin: "Repository: https://x.invalid/t\nRef: v0.4.0\n", record: record, noFiles: true, wantErr: "no *.test files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.origin != "" {
				write(t, dir, "ORIGIN.md", tt.origin)
			}
			if tt.record != "" {
				write(t, dir, recordName, tt.record)
			}
			if !tt.noFiles {
				write(t, dir, "var.test", baseFiles["var.test"])
			}
			r := runReadOnly(t, dir, offlineClient)
			if r.code != 2 {
				t.Fatalf("exit %d, want 2\n%s%s", r.code, r.stdout, r.stderr)
			}
			if !strings.Contains(r.stderr, tt.wantErr) {
				t.Errorf("stderr %q does not contain %q", r.stderr, tt.wantErr)
			}
		})
	}
}

// A pin that is not vN.N.N is refused before any request, in every mode.
func TestRefusesNonTagPin(t *testing.T) {
	up := newUpstream(t, map[string]response{})
	for _, ref := range []string{"main", "36f77a31bc37f36b30e33c0019741fdfd32c9a97", "0.4.0", "v0.4", "v0.4.0-rc1", "v0.4.0.1", "V0.4.0"} {
		for _, args := range [][]string{nil, {"-record"}, {"v0.4.0"}} {
			dir := setup(t, up.URL, ref, "v0.4.0", baseFiles)
			r := runReadOnly(t, dir, up.Client(), args...)
			if r.code != 2 || !strings.Contains(r.stderr, "is not a tag") {
				t.Errorf("pin %q, args %v: exit %d, stderr %q; want 2 and a refusal", ref, args, r.code, r.stderr)
			}
		}
	}
	if n := up.requests.Load(); n != 0 {
		t.Errorf("the server got %d requests; a bad pin must be refused first", n)
	}
}

func TestIsTag(t *testing.T) {
	for ref, want := range map[string]bool{
		"v0.4.0": true, "v10.20.30": true, "v0.4": false, "0.4.0": false, "v0.4.0-rc1": false,
		"main": false, "v0.4.0\n": false, " v0.4.0": false, "": false,
	} {
		if got := isTag(ref); got != want {
			t.Errorf("isTag(%q) = %v, want %v", ref, got, want)
		}
	}
}

func TestOnlineMatch(t *testing.T) {
	up := newUpstream(t, map[string]response{"v0.4.0": ok(corpusArchive(t, baseFiles))})
	dir := setup(t, up.URL, "v0.4.0", "v0.4.0", baseFiles)
	r := runReadOnly(t, dir, up.Client())
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "matches Toupee at v0.4.0 (3 files)") || r.stderr != "" {
		t.Errorf("stdout %q, stderr %q", r.stdout, r.stderr)
	}
}

func TestOnlineDrift(t *testing.T) {
	upstream := copyFiles(baseFiles)
	upstream["var.test"] = "upstream edit\n"
	upstream["new.test"] = "upstream only\n"
	delete(upstream, "edge.test")
	up := newUpstream(t, map[string]response{"v0.4.0": ok(corpusArchive(t, upstream))})
	dir := setup(t, up.URL, "v0.4.0", "v0.4.0", baseFiles)
	r := runReadOnly(t, dir, up.Client())
	if r.code != 1 {
		t.Fatalf("exit %d, want 1\n%s%s", r.code, r.stdout, r.stderr)
	}
	want := "driftcheck: " + dir + " matches SHA256SUMS at v0.4.0 (3 files)\n" +
		"removed\tedge.test\nadded\tnew.test\nchanged\tvar.test\n"
	if !strings.HasPrefix(r.stdout, want) || !strings.Contains(r.stdout, "3 files differ from Toupee at v0.4.0") {
		t.Errorf("stdout:\n%s\nwant to start with:\n%s", r.stdout, want)
	}
}

// corruptions returns archives that must fail as "unable to check".
func corruptions(t *testing.T) map[string]response {
	good := corpusArchive(t, baseFiles)
	badCRC := bytes.Clone(good)
	badCRC[len(badCRC)-8] ^= 0xff // first byte of the CRC-32 trailer
	badSize := bytes.Clone(good)
	badSize[len(badSize)-1] ^= 0xff // last byte of the ISIZE trailer
	return map[string]response{
		"not found":         {http.StatusNotFound, []byte("nope")},
		"server error":      {http.StatusInternalServerError, nil},
		"forbidden":         {http.StatusForbidden, nil},
		"empty corpus":      ok(tarGz(t, []tarEntry{{name: "toupee-x/README.md", body: "no tests"}})),
		"not gzip":          ok([]byte("<html>not an archive</html>")),
		"empty body":        ok(nil),
		"truncated gzip":    ok(good[:len(good)/2]),
		"missing trailer":   ok(good[:len(good)-4]),
		"bad crc":           ok(badCRC),
		"bad size":          ok(badSize),
		"gzip of non-tar":   ok(gzipOf(t, bytes.Repeat([]byte("x"), 1024))),
		"gzip of short tar": ok(gzipOf(t, []byte("short"))),
	}
}

func gzipOf(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(b)
	gz.Close()
	return buf.Bytes()
}

func TestOnlineFailures(t *testing.T) {
	for name, resp := range corruptions(t) {
		t.Run(name, func(t *testing.T) {
			up := newUpstream(t, map[string]response{"v0.4.0": resp})
			dir := setup(t, up.URL, "v0.4.0", "v0.4.0", baseFiles)
			r := runReadOnly(t, dir, up.Client())
			if r.code != 2 {
				t.Fatalf("exit %d, want 2\n%s%s", r.code, r.stdout, r.stderr)
			}
			if strings.Contains(r.stderr, "offline") {
				t.Errorf("an HTTP response was reported as offline: %q", r.stderr)
			}
		})
	}
}

// redirectThenOffline answers the archive request with a redirect and then
// fails the redirected request as if the network were gone, without dialing.
type redirectThenOffline struct{ requests atomic.Int64 }

func (rt *redirectThenOffline) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.requests.Add(1)
	if strings.HasPrefix(req.URL.Path, "/toupee/archive/") {
		return &http.Response{
			StatusCode: http.StatusFound,
			Status:     "302 Found",
			Header:     http.Header{"Location": {"/elsewhere"}},
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
	return nil, errors.New("dial tcp: no network")
}

// A redirect is an HTTP response, so a fetch that fails after one is not
// offline: the check, recording and measurement all fail with exit 2 instead
// of falling back to the record. A redirect that ends in the archive still
// counts as online.
func TestRedirectFailures(t *testing.T) {
	loop := func(t *testing.T) (*httptest.Server, *atomic.Int64) {
		var n atomic.Int64
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			http.Redirect(w, r, "/loop", http.StatusFound)
		}))
		t.Cleanup(s.Close)
		return s, &n
	}
	clients := map[string]func(t *testing.T) (string, *http.Client, *atomic.Int64){
		"redirect loop": func(t *testing.T) (string, *http.Client, *atomic.Int64) {
			s, n := loop(t)
			return s.URL, s.Client(), n
		},
		"redirect refused by the client": func(t *testing.T) (string, *http.Client, *atomic.Int64) {
			s, n := loop(t)
			c := s.Client()
			c.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("no redirects") }
			return s.URL, c, n
		},
		"redirect then no network": func(t *testing.T) (string, *http.Client, *atomic.Int64) {
			rt := &redirectThenOffline{}
			return "https://example.invalid/toupee", &http.Client{Transport: rt}, &rt.requests
		},
	}
	modes := map[string][]string{
		"check":   nil,
		"record":  {"-record"},
		"measure": {"main"},
	}
	for cname, mk := range clients {
		for mname, args := range modes {
			t.Run(cname+"/"+mname, func(t *testing.T) {
				url, client, n := mk(t)
				recordRef := "v0.4.0"
				if mname == "record" {
					recordRef = ""
				}
				dir := setup(t, url, "v0.4.0", recordRef, baseFiles)
				r := runReadOnly(t, dir, client, args...)
				if r.code != 2 {
					t.Errorf("exit %d, want 2\n%s%s", r.code, r.stdout, r.stderr)
				}
				if strings.Contains(r.stderr, "offline") {
					t.Errorf("a fetch that got a redirect was reported as offline: %q", r.stderr)
				}
				if n.Load() == 0 {
					t.Errorf("no request reached the server")
				}
			})
		}
	}
	t.Run("followed redirect", func(t *testing.T) {
		up := newUpstream(t, map[string]response{"v0.4.0": ok(corpusArchive(t, baseFiles))})
		front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, up.URL+r.URL.Path, http.StatusFound)
		}))
		t.Cleanup(front.Close)
		dir := setup(t, front.URL, "v0.4.0", "v0.4.0", baseFiles)
		r := runReadOnly(t, dir, front.Client())
		if r.code != 0 || !strings.Contains(r.stdout, "matches Toupee at v0.4.0") {
			t.Errorf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
	})
	t.Run("offline before any response", func(t *testing.T) {
		dir := setup(t, "https://example.invalid/toupee", "v0.4.0", "v0.4.0", baseFiles)
		r := runReadOnly(t, dir, offlineClient)
		if r.code != 0 || !strings.Contains(r.stderr, "offline") {
			t.Errorf("exit %d, stderr %q", r.code, r.stderr)
		}
	})
}

func TestReadCorpusTarFiltering(t *testing.T) {
	archive := tarGz(t, []tarEntry{
		{name: "toupee-x/", typeflag: tar.TypeDir},
		{name: "toupee-x/test/", typeflag: tar.TypeDir},
		{name: "toupee-x/test/var.test", body: "direct"},
		{name: "toupee-x/test/types-compare.test", body: "hyphen\r\n"},
		{name: "toupee-x/test/sub/", typeflag: tar.TypeDir},
		{name: "toupee-x/test/sub/nested.test", body: "nested"},
		{name: "toupee-x/tests/wrong.test", body: "wrong dir"},
		{name: "toupee-x/src/test/deep.test", body: "deeper"},
		{name: "toupee-x/root.test", body: "root level"},
		{name: "top.test", body: "no root dir"},
		{name: "toupee-x/test/README.md", body: "not a test"},
		{name: "toupee-x/test/link.test", body: "var.test", typeflag: tar.TypeSymlink},
		{name: "toupee-x/test/dir.test/", typeflag: tar.TypeDir},
	})
	got, err := readCorpusTar(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]byte{"var.test": []byte("direct"), "types-compare.test": []byte("hyphen\r\n")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatRecord(t *testing.T) {
	sums := map[string]string{
		"var.test":           strings.Repeat("b", 64),
		"types-compare.test": strings.Repeat("a", 64),
		"bind.test":          strings.Repeat("c", 64),
	}
	want := "# Ref: v1.2.3\n" +
		strings.Repeat("c", 64) + "  bind.test\n" +
		strings.Repeat("a", 64) + "  types-compare.test\n" +
		strings.Repeat("b", 64) + "  var.test\n"
	got := formatRecord("v1.2.3", sums)
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	rec, err := parseRecord(got)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Ref != "v1.2.3" || !reflect.DeepEqual(rec.Sums, sums) {
		t.Errorf("round trip: got %+v", rec)
	}
}

func TestRecord(t *testing.T) {
	up := newUpstream(t, map[string]response{"v0.4.0": ok(corpusArchive(t, baseFiles))})
	dir := setup(t, up.URL, "v0.4.0", "", baseFiles)
	write(t, dir, recordName, "# Ref: v0.3.0\nstale\n")
	r := runIn(dir, up.Client(), "-record")
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\n%s%s", r.code, r.stdout, r.stderr)
	}
	b := map[string][]byte{}
	for n, c := range baseFiles {
		b[n] = []byte(c)
	}
	got, _ := os.ReadFile(filepath.Join(dir, recordName))
	if want := formatRecord("v0.4.0", digest(b)); !bytes.Equal(got, want) {
		t.Errorf("recorded\n%s\nwant\n%s", got, want)
	}
	if r := runReadOnly(t, dir, offlineClient); r.code != 0 {
		t.Errorf("the new record does not check: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

func TestRecordWritesNothingOnFailure(t *testing.T) {
	differs := copyFiles(baseFiles)
	differs["var.test"] = "upstream edit\n"
	cases := corruptions(t)
	cases["differs"] = ok(corpusArchive(t, differs))
	for name, resp := range cases {
		t.Run(name, func(t *testing.T) {
			up := newUpstream(t, map[string]response{"v0.4.0": resp})
			dir := setup(t, up.URL, "v0.4.0", "", baseFiles)
			write(t, dir, recordName, "# Ref: v0.3.0\nexisting contents\n")
			r := runReadOnly(t, dir, up.Client(), "-record")
			want := 2
			if name == "differs" {
				want = 1
			}
			if r.code != want {
				t.Errorf("exit %d, want %d\n%s%s", r.code, want, r.stdout, r.stderr)
			}
		})
	}
	t.Run("offline", func(t *testing.T) {
		dir := setup(t, "https://example.invalid/toupee", "v0.4.0", "", baseFiles)
		r := runReadOnly(t, dir, offlineClient, "-record")
		if r.code != 2 {
			t.Errorf("exit %d, want 2: recording needs the network", r.code)
		}
		if _, err := os.Stat(filepath.Join(dir, recordName)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a record was written offline")
		}
	})
	t.Run("with REF", func(t *testing.T) {
		dir := setup(t, "https://example.invalid/toupee", "v0.4.0", "v0.4.0", baseFiles)
		if r := runReadOnly(t, dir, offlineClient, "-record", "main"); r.code != 2 {
			t.Errorf("exit %d, want 2", r.code)
		}
	})
}

// Measuring another ref, even a branch, reads neither the record nor moves
// the pin, and needs a response from the network.
func TestMeasure(t *testing.T) {
	ahead := copyFiles(baseFiles)
	ahead["new.test"] = "on main only\n"
	up := newUpstream(t, map[string]response{
		"main":   ok(corpusArchive(t, ahead)),
		"v0.4.0": ok(corpusArchive(t, baseFiles)),
	})
	dir := setup(t, up.URL, "v0.4.0", "", baseFiles) // no record needed

	r := runReadOnly(t, dir, up.Client(), "main")
	if r.code != 1 || !strings.Contains(r.stdout, "added\tnew.test\n") || !strings.Contains(r.stdout, "differ from Toupee at main") {
		t.Errorf("measure main: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	r = runReadOnly(t, dir, up.Client(), "v0.4.0")
	if r.code != 0 {
		t.Errorf("measure v0.4.0: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	r = runReadOnly(t, dir, up.Client(), "no-such-branch")
	if r.code != 2 || !strings.Contains(r.stderr, "not found") {
		t.Errorf("measure a missing ref: exit %d, stderr %q", r.code, r.stderr)
	}
	r = runReadOnly(t, dir, offlineClient, "main")
	if r.code != 2 || !strings.Contains(r.stderr, "cannot measure main offline") {
		t.Errorf("measure offline: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestUsage(t *testing.T) {
	dir := setup(t, "https://example.invalid/toupee", "v0.4.0", "v0.4.0", baseFiles)
	var out, errb bytes.Buffer
	if code := run([]string{"-h"}, offlineClient, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "Usage: driftcheck") {
		t.Errorf("-h: exit %d, stdout %q", code, out.String())
	}
	if r := runReadOnly(t, dir, offlineClient, "a", "b"); r.code != 2 || !strings.Contains(r.stderr, "at most one REF") {
		t.Errorf("two REFs: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := runReadOnly(t, dir, offlineClient, "-bogus"); r.code != 2 {
		t.Errorf("unknown flag: exit %d", r.code)
	}
}

// expectedRecord is the checksum record of Toupee v0.4.0's 22 test files.
const expectedRecord = `# Ref: v0.4.0
1e7c0003be156977cdcf10343c48fc5f89fd6ac23581db907bece04f629b3669  bind.test
aa4c2999f12b1952893c36b7518e592398a26dad12616a54ff07a87099e27eee  comment.test
a8cd94cbd44dc476944dd774c8e5146e6ee0c17f32f70672a4a2af0b80fed07a  conditional.test
b57def346a9568d3ee1984e1d1db4af3408f4008818773a4e6cb4928f176ffd6  dynamic-path.test
7e730f469dfa905916de4932507efd7fbe4f473e07ca77fcd83c5006d1dc6f0e  edge.test
aaa4d02d21438d73802303c33b7b695f970b876054994da60d1e09a832b2faf4  error.test
9eb4b6d2646ecef5c1e2a2ce9da474c952511506d43f191fd2d776dc5685a289  extend.test
6f5aaaf926cbf027c90cf8c6e34c868aafd8912eef6fa7e6357fdacf6d9ec114  include.test
2a9a77dcb4a7f63b52d83c5d2c61056de625cc63b75d4d22dac1b96d49190211  integration.test
3feb316712057e82748265b9e3b14aa39be108a3f21080f85875ef4f4f19e4c8  isolation.test
121f2917297405fd589360bfe2cc444c94612b8decec56f02ee9a5ec214550ed  let.test
a569887369da51bce1a1f749b5cdd15a0efc4146b050e90bb8cc2e587c426937  loop.test
725e1cde095ea25bba53b445f9c23f81a81eeb5db2b455c3208b0fec94c03967  nesting.test
0581cc1b2a6cb14265b3e75b4b40aacef64e9afe58e8c3e3d8066fd1b1a8f62c  raw.test
311619c80c08f8abc566a8af5b45c80a373d0733ae09f4720b4a34a0522b1d68  regression.test
ff350d70a7267de951a3190b05f5f11b234eb962ddd88b9bd9cf7b87fc550c1e  slot.test
32113e1511163192db3dcf1ad4b881890be3cbd60d06946c16e3d297f64d3abd  transform.test
530bbc2c68949cdcb80fde04b2995cb5d94a0cc6e6fdc499298a973a33c50c26  types-compare.test
f80318ed0218ad4c0aeabf0d4317f2296aca2ad77e24df97bbbafdf9685cac5b  types-extremes.test
0bd00313a995d61958515850ce86caf0a1660f64bd66ce8ad3ad5ddc46ff9824  types-render.test
9d22d1dc026ae1a5ee24fc865015c29c85dbe9d3a958d84192dcce99a4610ed1  types-transform.test
fc2ac7a3735e86d544e49eb9e91534e215f1b3a1621ce643971159c48367e418  var.test
`

// TestRecordRealCorpus checks the checked-in corpus offline: the pin, the
// canonical 22-entry record and every file's digest.
func TestRecordRealCorpus(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "spec")
	content, err := os.ReadFile(filepath.Join(dir, "ORIGIN.md"))
	if err != nil {
		t.Fatal(err)
	}
	o, err := parseOrigin(content)
	if err != nil {
		t.Fatal(err)
	}
	if o.Repository != "https://github.com/devselfio/toupee" || o.Ref != "v0.4.0" {
		t.Errorf("ORIGIN.md pins %+v, want https://github.com/devselfio/toupee at v0.4.0", o)
	}
	raw, err := os.ReadFile(filepath.Join(dir, recordName))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != expectedRecord {
		t.Errorf("SHA256SUMS is not the expected v0.4.0 record:\n%s", raw)
	}
	rec, err := parseRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(formatRecord(rec.Ref, rec.Sums), raw) {
		t.Error("SHA256SUMS is not in canonical form")
	}
	if rec.Ref != o.Ref || len(rec.Sums) != 22 {
		t.Errorf("record at %s with %d files, want %s with 22", rec.Ref, len(rec.Sums), o.Ref)
	}
	local, err := readLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changes := compareSums(digest(local), rec.Sums); len(changes) > 0 {
		t.Errorf("checked-in corpus differs from its record: %v", changes)
	}
	if r := runReadOnly(t, dir, offlineClient); r.code != 0 {
		t.Errorf("offline pin check: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}
