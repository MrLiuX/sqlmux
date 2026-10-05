// Package i18n is the interface's language (tech-design §14「界面语言」): the
// Chinese text is the key, an English table says it in English, and what the
// table lacks is shown as it was written.
package i18n

import (
	"fmt"
	"strings"
)

var english bool

// Set picks the language, "zh" or "en", once, as the program starts.
func Set(lang string) error {
	switch lang {
	case "zh":
		english = false
	case "en":
		english = true
	default:
		return fmt.Errorf("language = %q：只能是 \"zh\" 或 \"en\"", lang)
	}
	return nil
}

// Lang is the language in use.
func Lang() string {
	if english {
		return "en"
	}
	return "zh"
}

// T is zh in the language in use, with args put in as fmt.Sprintf does,
// when there are any.
func T(zh string, args ...any) string {
	if english {
		if en, ok := en[zh]; ok {
			zh = en
		}
	}
	if len(args) == 0 {
		return zh
	}
	return fmt.Sprintf(zh, args...)
}

// N marks zh as a text to translate where it is not shown yet, a title in a
// table: it is what it was given, and T(it) is shown (gettext's N_).
func N(zh string) string { return zh }

// Verbs is the formatting verbs of s in order, for the test that an English
// text takes the same arguments as its Chinese one.
func Verbs(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		j := i + 1
		for j < len(s) && strings.IndexByte("+-# 0123456789.*[]", s[j]) >= 0 {
			j++
		}
		if j < len(s) && s[j] != '%' {
			out = append(out, s[i:j+1])
		}
		i = j
	}
	return out
}
