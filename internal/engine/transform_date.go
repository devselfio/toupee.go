package engine

import (
	"strconv"
	"strings"
)

// The date built-in ports Toupee v0.4.0's src/transform.zig dateTransform,
// parseIsoDate, parseTzOffset, adjustMinutes and formatDate, permissive
// edges included, rather than Go's time parsing.

// dateParts is a parsed date and time, normalised to UTC.
type dateParts struct {
	year                             uint16
	month, day, hour, minute, second uint8
}

// transformDate returns value unchanged without a format. Otherwise it
// parses value, trimmed of space, tab, LF and CR, and formats it with
// args[0], or returns value untrimmed when it does not parse. Later
// arguments are ignored.
func transformDate(value string, args []string) (string, error) {
	if len(args) == 0 {
		return value, nil
	}
	d, ok := parseISODate(strings.Trim(value, toupeeSpace))
	if !ok {
		return value, nil
	}
	return formatDate(d, args[0]), nil
}

// parseZigInt parses s as Zig's std.fmt.parseInt(T, s, 10) does for an
// integer type spanning [lo, hi]: an optional sign, then decimal digits
// with '_' allowed between them.
func parseZigInt(s string, lo, hi int64) (int64, bool) {
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" || s[0] == '_' || s[len(s)-1] == '_' {
		return 0, false
	}
	var n int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if n > hi && (!neg || -n < lo) {
			return 0, false
		}
	}
	if neg {
		n = -n
	}
	if n < lo || n > hi {
		return 0, false
	}
	return n, true
}

// parseISODate reads YYYY-MM-DD, optionally followed by 'T' or ' ' and
// HH:MM:SS, then an optional timezone that parseTZOffset accepts. Fields are
// parsed as Zig integers, so a sign or '_' is allowed in them. Month must be
// 1-12 and day 1-31, whatever the month; the time fields are not
// range-checked. A nonzero offset is applied to bring the time to UTC.
func parseISODate(s string) (dateParts, bool) {
	if len(s) < 10 || s[4] != '-' || s[7] != '-' {
		return dateParts{}, false
	}
	year, ok1 := parseZigInt(s[0:4], 0, 65535)
	month, ok2 := parseZigInt(s[5:7], 0, 255)
	day, ok3 := parseZigInt(s[8:10], 0, 255)
	if !ok1 || !ok2 || !ok3 || month < 1 || month > 12 || day < 1 || day > 31 {
		return dateParts{}, false
	}
	d := dateParts{year: uint16(year), month: uint8(month), day: uint8(day)}
	tzStart := 10
	if len(s) >= 19 && (s[10] == 'T' || s[10] == ' ') && s[13] == ':' && s[16] == ':' {
		hour, ok1 := parseZigInt(s[11:13], 0, 255)
		minute, ok2 := parseZigInt(s[14:16], 0, 255)
		second, ok3 := parseZigInt(s[17:19], 0, 255)
		if !ok1 || !ok2 || !ok3 {
			return dateParts{}, false
		}
		d.hour, d.minute, d.second = uint8(hour), uint8(minute), uint8(second)
		tzStart = 19
	}
	if tzStart < len(s) {
		offset, ok := parseTZOffset(s[tzStart:])
		if !ok {
			return dateParts{}, false
		}
		if offset != 0 {
			d = adjustDateMinutes(d, -offset)
		}
	}
	return d, true
}

// parseTZOffset reads a timezone prefix of s as minutes east of UTC: empty
// or 'Z' is 0; otherwise a sign then HH:MM, two-character hours or a
// one-character hour. Anything after the part it reads is ignored.
func parseTZOffset(s string) (int32, bool) {
	if s == "" || s[0] == 'Z' {
		return 0, true
	}
	if s[0] != '+' && s[0] != '-' {
		return 0, false
	}
	sign := int64(1)
	if s[0] == '-' {
		sign = -1
	}
	const lo, hi = -1 << 31, 1<<31 - 1
	rest := s[1:]
	switch {
	case len(rest) >= 5 && rest[2] == ':':
		h, ok1 := parseZigInt(rest[0:2], lo, hi)
		m, ok2 := parseZigInt(rest[3:5], lo, hi)
		if !ok1 || !ok2 {
			return 0, false
		}
		return int32(sign * (h*60 + m)), true
	case len(rest) >= 2:
		h, ok := parseZigInt(rest[0:2], lo, hi)
		if !ok {
			return 0, false
		}
		return int32(sign * h * 60), true
	case len(rest) == 1:
		h, ok := parseZigInt(rest, lo, hi)
		if !ok {
			return 0, false
		}
		return int32(sign * h * 60), true
	}
	return 0, false
}

func isLeapYear(year int) bool {
	return year%4 == 0 && year%100 != 0 || year%400 == 0
}

func daysInMonth(month, year int) int {
	switch month {
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeapYear(year) {
			return 29
		}
		return 28
	}
	return 31
}

// adjustDateMinutes adds offset minutes to d's time of day, carrying whole
// days into the day, month and year. Seconds are unchanged.
func adjustDateMinutes(d dateParts, offset int32) dateParts {
	total := int(d.hour)*60 + int(d.minute) + int(offset)
	day, month, year := int(d.day), int(d.month), int(d.year)
	for total < 0 {
		total += 24 * 60
		day--
		if day < 1 {
			month--
			if month < 1 {
				month = 12
				year--
			}
			day = daysInMonth(month, year)
		}
	}
	for total >= 24*60 {
		total -= 24 * 60
		day++
		if day > daysInMonth(month, year) {
			day = 1
			month++
			if month > 12 {
				month = 1
				year++
			}
		}
	}
	return dateParts{
		year: uint16(year), month: uint8(month), day: uint8(day),
		hour: uint8(total / 60), minute: uint8(total % 60), second: d.second,
	}
}

var (
	monthNames    = [...]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	monthAbbrevs  = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	weekdayNames  = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	weekdayAbbrev = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
)

// dayOfWeek is Tomohiko Sakamoto's algorithm: 0 is Sunday, 6 Saturday.
func dayOfWeek(year uint16, month, day uint8) uint8 {
	t := [...]int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	y := int(year)
	if month < 3 {
		y--
	}
	ymod := ((y % 400) + 400) % 400
	return uint8((ymod + ymod/4 - ymod/100 + t[month-1] + int(day)) % 7)
}

// appendPadded writes v in decimal, zero-padded on the left to width.
func appendPadded(b *strings.Builder, v int, width int) {
	s := strconv.Itoa(v)
	for i := len(s); i < width; i++ {
		b.WriteByte('0')
	}
	b.WriteString(s)
}

// formatDate expands %Y %m %d %e %H %M %S %B %b %A %a %p %P %I %F %T and %%
// in format with English names; %e is the unpadded day. Any other byte after
// a percent is kept with its percent, and a final percent is kept.
func formatDate(d dateParts, format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 == len(format) {
			b.WriteByte(format[i])
			continue
		}
		i++
		switch c := format[i]; c {
		case 'Y':
			appendPadded(&b, int(d.year), 4)
		case 'm':
			appendPadded(&b, int(d.month), 2)
		case 'd':
			appendPadded(&b, int(d.day), 2)
		case 'e':
			b.WriteString(strconv.Itoa(int(d.day)))
		case 'H':
			appendPadded(&b, int(d.hour), 2)
		case 'M':
			appendPadded(&b, int(d.minute), 2)
		case 'S':
			appendPadded(&b, int(d.second), 2)
		case 'B':
			b.WriteString(monthNames[d.month-1])
		case 'b':
			b.WriteString(monthAbbrevs[d.month-1])
		case 'A':
			b.WriteString(weekdayNames[dayOfWeek(d.year, d.month, d.day)])
		case 'a':
			b.WriteString(weekdayAbbrev[dayOfWeek(d.year, d.month, d.day)])
		case 'p':
			if d.hour >= 12 {
				b.WriteString("PM")
			} else {
				b.WriteString("AM")
			}
		case 'P':
			if d.hour >= 12 {
				b.WriteString("pm")
			} else {
				b.WriteString("am")
			}
		case 'I':
			h := int(d.hour % 12)
			if h == 0 {
				h = 12
			}
			appendPadded(&b, h, 2)
		case 'F':
			appendPadded(&b, int(d.year), 4)
			b.WriteByte('-')
			appendPadded(&b, int(d.month), 2)
			b.WriteByte('-')
			appendPadded(&b, int(d.day), 2)
		case 'T':
			appendPadded(&b, int(d.hour), 2)
			b.WriteByte(':')
			appendPadded(&b, int(d.minute), 2)
			b.WriteByte(':')
			appendPadded(&b, int(d.second), 2)
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(c)
		}
	}
	return b.String()
}
