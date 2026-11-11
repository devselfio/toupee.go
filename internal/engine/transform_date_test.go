package engine

import "testing"

// TestDateTransform ports Toupee v0.4.0's in-source date tests
// (src/transform.zig) and pins its permissive parsing edges.
func TestDateTransform(t *testing.T) {
	const full = "%Y-%m-%d %H:%M:%S"
	a := func(args ...string) []string { return args }
	for _, tt := range []struct {
		name, value string
		args        []string
		want        string
	}{
		// Upstream's in-source tests.
		{"passthrough without format", "2026-03-03", nil, "2026-03-03"},
		{"month name, unpadded day", "2026-03-05", a("%B %e, %Y"), "March 5, 2026"},
		{"numeric", "2026-03-05", a("%d/%m/%Y"), "05/03/2026"},
		{"abbreviated month", "2026-01-15", a("%b %d"), "Jan 15"},
		{"with time", "2026-03-05T14:30:00", a(full), "2026-03-05 14:30:00"},
		{"invalid input passthrough", "not a date", a("%Y"), "not a date"},
		{"literal percent", "2026-03-05", a("%%"), "%"},
		{"Z is UTC", "2024-03-15T10:30:00Z", a(full), "2024-03-15 10:30:00"},
		{"positive hour offset", "2024-03-15T10:30:00+08", a(full), "2024-03-15 02:30:00"},
		{"negative hour offset", "2024-03-15T10:30:00-05", a(full), "2024-03-15 15:30:00"},
		{"hour:minute offset", "2024-03-15T10:30:00+05:30", a(full), "2024-03-15 05:00:00"},
		{"midnight backward", "2024-03-15T01:00:00+08", a(full), "2024-03-14 17:00:00"},
		{"midnight forward", "2024-03-15T23:00:00-05", a(full), "2024-03-16 04:00:00"},
		{"leap day", "2024-03-01T01:00:00+08", a(full), "2024-02-29 17:00:00"},
		{"year boundary", "2024-01-01T01:00:00+08", a(full), "2023-12-31 17:00:00"},
		{"date only", "2024-03-15", a("%Y-%m-%d"), "2024-03-15"},
		{"%A", "2024-01-01", a("%A"), "Monday"},
		{"%a", "2024-01-01", a("%a"), "Mon"},
		{"%p AM", "2024-01-01T09:00:00", a("%p"), "AM"},
		{"%p PM", "2024-01-01T14:00:00", a("%p"), "PM"},
		{"%P am", "2024-01-01T09:00:00", a("%P"), "am"},
		{"%P pm", "2024-01-01T14:00:00", a("%P"), "pm"},
		{"%I midnight", "2024-01-01T00:00:00", a("%I"), "12"},
		{"%I noon", "2024-01-01T12:00:00", a("%I"), "12"},
		{"%I 1am", "2024-01-01T01:00:00", a("%I"), "01"},
		{"%I 1pm", "2024-01-01T13:00:00", a("%I"), "01"},
		{"%F", "2024-03-15", a("%F"), "2024-03-15"},
		{"%T", "2024-03-15T14:30:45", a("%T"), "14:30:45"},
		{"realistic", "2024-01-01T14:30:00", a("%A, %B %e, %Y at %I:%M %p"), "Monday, January 1, 2024 at 02:30 PM"},

		// Beyond upstream's tests: the same parser, edge by edge.
		{"only the first argument", "2024-03-15T14:30:45", a("%H", "%M"), "14"},
		{"trimmed for parsing", " \n2024-03-15\t", a("%F"), "2024-03-15"},
		{"failure returns the untrimmed input", " 2024-13-01 ", a("%F"), " 2024-13-01 "},
		{"vertical tab is not trimmed", "\v2024-03-15", a("%F"), "\v2024-03-15"},
		{"too short", "2024-3-15", a("%F"), "2024-3-15"},
		{"day 31 in February is accepted", "2023-02-31", a("%F %a"), "2023-02-31 Fri"},
		{"day 32 is rejected", "2023-01-32", a("%F"), "2023-01-32"},
		{"space separator", "2024-03-15 08:05:09", a("%T"), "08:05:09"},
		{"hour is not range-checked", "2024-03-15T99:00:00", a("%H %I %p"), "99 03 PM"},
		{"T without full time is junk", "2024-03-15T10:30", a("%F"), "2024-03-15T10:30"},
		{"trailing junk after the date", "2024-03-15x", a("%F"), "2024-03-15x"},
		{"Z then anything", "2024-03-15T10:00:00Zjunk", a("%T"), "10:00:00"},
		{"offset on a date", "2024-03-15+01", a("%F %T"), "2024-03-14 23:00:00"},
		{"one-digit offset", "2024-03-15T10:00:00+2", a("%T"), "08:00:00"},
		{"HHMM offset reads hours only", "2024-03-15T10:00:00+0530", a("%T"), "05:00:00"},
		{"offset with trailing text", "2024-03-15T10:00:00-01:30xyz", a("%T"), "11:30:00"},
		{"bare sign is rejected", "2024-03-15T10:00:00+", a("%T"), "2024-03-15T10:00:00+"},
		{"year rollover forward", "2023-12-31T23:30:00-01", a(full), "2024-01-01 00:30:00"},
		{"non-leap February", "2023-03-01T00:00:00+01", a(full), "2023-02-28 23:00:00"},
		{"century non-leap", "1900-03-01T00:00:00+01", a("%F"), "1900-02-28"},
		{"400-year leap", "2000-03-01T00:00:00+01", a("%F %A"), "2000-02-29 Tuesday"},
		{"sign in fields", "+024-03-15", a("%Y"), "0024"},
		{"underscore in fields", "2_24-03-15", a("%Y"), "0224"},
		{"unknown directive kept", "2024-03-15", a("%Q %Y"), "%Q 2024"},
		{"trailing percent kept", "2024-03-15", a("%Y%"), "2024%"},
		{"empty format", "2024-03-15", a(""), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := builtinTransforms["date"](tt.value, tt.args)
			if err != nil || got != tt.want {
				t.Errorf("date(%q, %q) = %q, %v; want %q", tt.value, tt.args, got, err, tt.want)
			}
		})
	}
}

func TestParseTZOffset(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int32
		ok   bool
	}{
		{"", 0, true}, {"Z", 0, true}, {"Zx", 0, true}, {"+08", 480, true},
		{"-05", -300, true}, {"+05:30", 330, true}, {"-05:30", -330, true},
		{"+5", 300, true}, {"+", 0, false}, {"x", 0, false}, {"+x", 0, false},
		{"+ab:cd", 0, false},
	} {
		got, ok := parseTZOffset(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseTZOffset(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDayOfWeek(t *testing.T) {
	for _, tt := range []struct {
		y       uint16
		m, d, w uint8
	}{
		{2024, 1, 1, 1}, {2024, 2, 29, 4}, {2000, 1, 1, 6}, {1970, 1, 1, 4}, {2026, 11, 10, 2},
	} {
		if got := dayOfWeek(tt.y, tt.m, tt.d); got != tt.w {
			t.Errorf("dayOfWeek(%d-%d-%d) = %d, want %d", tt.y, tt.m, tt.d, got, tt.w)
		}
	}
}
