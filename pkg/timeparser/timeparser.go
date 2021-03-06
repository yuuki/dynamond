package timeparser

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/pkg/errors"
)

const (
	timeFormat = "15:0420060102"
)

type TimeParserError struct {
	msg string
	error
}

func (e *TimeParserError) Error() string {
	if e.error == nil {
		return e.msg
	}
	return e.error.Error() + " " + e.msg
}

// graphite-web can parse various time formats,
// but epoch and string representations are currently supported.
// (graphite-web supports "any other at(1)-compatible time format.")

// ParseAtTime parses parameters that specify the relative or absolute time period.
// eg. '1444508126', 'now', 'now-24h'
func ParseAtTime(s string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}

	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Replace(s, "_", "", -1)
	s = strings.Replace(s, ",", "", -1)
	s = strings.Replace(s, " ", "", -1)

	var (
		ref    string
		offset string
	)

	// unix time ?
	if i, err := strconv.ParseInt(s, 10, 32); err == nil {
		return time.Unix(i, 0).In(loc), nil
	}

	if strings.Contains(s, ":") && len(s) == 13 {
		t, err := time.ParseInLocation(timeFormat, s, loc)
		if err != nil {
			return time.Time{}, errors.WithStack(
				&TimeParserError{
					fmt.Sprintf("invalid time format (%s)", s),
					err,
				},
			)
		}
		return t.In(loc), nil
	} else if strings.Contains(s, "+") {
		v := strings.SplitN(s, "+", 2)
		ref, offset = v[0], v[1]
		offset = "+" + offset
	} else if strings.Contains(s, "-") {
		v := strings.SplitN(s, "-", 2)
		ref, offset = v[0], v[1]
		offset = "-" + offset
	} else {
		ref, offset = s, ""
	}

	var (
		r time.Time
		o time.Duration
	)

	if ref == "" || ref == "now" {
		r = time.Now().Round(time.Second)
	} else {
		return time.Time{}, errors.WithStack(
			&TimeParserError{
				msg: fmt.Sprintf("unsupported day reference (%s)", s),
			},
		)
	}
	o, err := ParseTimeOffset(offset)
	if err != nil {
		return time.Time{}, err
	}
	return r.In(loc).Add(o), nil
}

// ParseTimeOffset parses the offset into time.Duration.
// eg. offset: 1h
func ParseTimeOffset(offset string) (time.Duration, error) {
	t := time.Duration(0)

	if offset == "" {
		return t, nil
	}

	var sign int
	if unicode.IsDigit(rune(offset[0])) {
		sign = 1
	} else {
		switch offset[0] {
		case '+':
			sign = 1
		case '-':
			sign = -1
		}
		offset = offset[1:]
	}

	for offset != "" {
		i := 0
		for i < len(offset) && unicode.IsDigit(rune(offset[i])) {
			i++
		}
		num := offset[:i]
		offset = offset[i:]
		i = 0
		for i < len(offset) && isAlpha(rune(offset[i])) {
			i++
		}
		unit := offset[:i]
		offset = offset[i:]

		n, _ := strconv.Atoi(num)
		t2 := time.Duration(n)
		if strings.HasPrefix(unit, "s") {
			t2 *= time.Second
		} else if unit == "m" || strings.HasPrefix(unit, "min") {
			t2 *= time.Minute
		} else if strings.HasPrefix(unit, "h") {
			t2 *= time.Hour
		} else if strings.HasPrefix(unit, "d") {
			t2 *= 24 * time.Hour
		} else if strings.HasPrefix(unit, "w") {
			t2 *= 7 * 24 * time.Hour
		} else if strings.HasPrefix(unit, "mon") {
			t2 *= 30 * 24 * time.Hour
		} else if strings.HasPrefix(unit, "y") {
			t2 *= 365 * 24 * time.Hour
		} else {
			return time.Duration(0), errors.WithStack(
				&TimeParserError{
					msg: fmt.Sprintf("invalid offset unit (%s)", unit),
				},
			)
		}

		t += time.Duration(sign) * t2
	}

	return t, nil
}

func isAlpha(s rune) bool {
	if s < 'A' || s > 'z' {
		return false
	} else if s > 'Z' && s < 'a' {
		return false
	}
	return true
}
