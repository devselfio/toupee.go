package toupee

import (
	"bytes"
	"testing"
)

func TestDetectIndent(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"hello", ""},
		{"  ", ""}, // a whitespace-only first line has no detected indent
		{"hello\n  ", "  "},
		{"hello\n\t\t", "\t\t"},
		{"hello\nworld", ""},
		{"a\n  b\n    ", "    "},
		{"a\n  x ", ""},
		{"a\r\n  ", "  "},
	} {
		if got := detectIndent(tt.in); got != tt.want {
			t.Errorf("detectIndent(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAppendIndented(t *testing.T) {
	for _, tt := range []struct{ out, content, indent, want string }{
		{"<a>\n  ", "x\ny\n\nz", "  ", "<a>\n  x\n  y\n\n  z"},
		{"<a>", "x\ny", "", "<a>x\ny"},
		{"<a>\n  ", "x\n", "  ", "<a>\n  x\n"},
		{"<a>\n \t ", "", " \t ", "<a>\n"},
		{"<a> ", "", "", "<a>"},
		{"", "", "", ""},
	} {
		var b bytes.Buffer
		b.WriteString(tt.out)
		appendIndented(&b, tt.content, tt.indent)
		if got := b.String(); got != tt.want {
			t.Errorf("appendIndented(%q, %q, %q) = %q, want %q", tt.out, tt.content, tt.indent, got, tt.want)
		}
	}
}

func TestStripCommonIndent(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"\n  \n\t\n", ""},
		{"x", "x"},
		{"\n    <p>a</p>\n      <b/>\n    \n    c\n  ", "<p>a</p>\n  <b/>\n    \nc"}, // an interior blank line keeps its bytes
		{"\n\t<p>a</p>\r\n\t<p>b</p>\n", "<p>a</p>\r\n<p>b</p>"},
		{"foo\n      bar", "foo\n      bar"},
		{"  one line  ", "one line  "},
	} {
		if got := stripCommonIndent(tt.in); got != tt.want {
			t.Errorf("stripCommonIndent(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestStripCommonIndentOffsets checks that each byte of the stripped text
// maps back to its offset in the original.
func TestStripCommonIndentOffsets(t *testing.T) {
	in := "\n  ab\n    c\n"
	got, offsets := stripCommonIndentMap(in)
	if got != "ab\n  c" {
		t.Fatalf("stripped = %q", got)
	}
	if len(offsets) != len(got) {
		t.Fatalf("%d offsets for %d bytes", len(offsets), len(got))
	}
	for i := range got {
		if in[offsets[i]] != got[i] {
			t.Errorf("byte %d %q maps to offset %d %q", i, got[i], offsets[i], in[offsets[i]])
		}
	}
}

// TestSlotLineBreakConsumption renders a slot followed by LF, CRLF and
// other bytes: one following line break is consumed, nothing else.
func TestSlotLineBreakConsumption(t *testing.T) {
	ctx := Context{Slots: map[string]string{"": "<i>x</i>\n"}}
	for _, tt := range []struct{ in, want string }{
		{"<b>\n  <t-slot />\n</b>", "<b>\n  <i>x</i>\n</b>"},
		{"<b>\r\n  <t-slot />\r\n</b>", "<b>\r\n  <i>x</i>\n</b>"},
		{"<b>\n  <t-slot></t-slot>\n</b>", "<b>\n  <i>x</i>\n</b>"},
		{"<b>\n  <t-slot />\n\n</b>", "<b>\n  <i>x</i>\n\n</b>"},
		{"<t-slot />x", "<i>x</i>\nx"},
		{"<t-slot />\r", "<i>x</i>\n\r"},
	} {
		got, err := Render(tt.in, ctx, nil)
		if err != nil || got != tt.want {
			t.Errorf("Render(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

// TestEmptyInsertionTrimsIndent shows an empty fill and an empty include
// removing the spaces and tabs before their insertion point.
func TestEmptyInsertionTrimsIndent(t *testing.T) {
	loader := func(name string) (string, error) { return "", nil }
	for _, tt := range []struct {
		in   string
		ctx  Context
		want string
	}{
		{"<ul>\n  \t<t-slot />\n</ul>", Context{Slots: map[string]string{"": ""}}, "<ul>\n</ul>"},
		{"<ul>\n  <t-include template=\"e\" />\n</ul>", Context{}, "<ul>\n\n</ul>"},
		// An absent slot without a default body inserts nothing at all.
		{"<ul>\n  <t-slot />\n</ul>", Context{}, "<ul>\n  </ul>"},
		{"<ul>\n  <t-include template=\"e\">\n  <p>a</p>\n  <p>b</p>\n  </t-include>\n</ul>", Context{}, "<ul>\n\n</ul>"},
	} {
		got, err := Render(tt.in, tt.ctx, loader)
		if err != nil || got != tt.want {
			t.Errorf("Render(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}
