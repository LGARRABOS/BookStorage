package database

import (
	"strconv"
	"strings"
)

// RebindQuestionToDollar converts each `?` placeholder to $1, $2, … for PostgreSQL.
// Queries in this project do not use `?` inside string literals.
func RebindQuestionToDollar(q string) string {
	if !strings.Contains(q, "?") {
		return q
	}
	var buf strings.Builder
	buf.Grow(len(q) + 8)
	n := 1
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			buf.WriteByte('$')
			buf.WriteString(strconv.Itoa(n))
			n++
		} else {
			buf.WriteByte(q[i])
		}
	}
	return buf.String()
}
