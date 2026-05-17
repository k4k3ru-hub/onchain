//
// helper.go
//
package onchain

import (
    "unicode/utf8"
)




//
// Truncate runes.
//
func truncateRunes(s string, max int) string {
    if max <= 0 {
        return ""
    }

    if utf8.RuneCountInString(s) <= max {
        return s
    }

    r := []rune(s)

    if max <= 3 {
        return string(r[:max])
    }

    return string(r[:max]) + "..."
}
