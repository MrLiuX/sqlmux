// Package i18n is the interface's language (tech-design §14「界面语言」): the
// Chinese text is the key, an English table says it in English, and what the
// table lacks is shown as it was written.
package i18n

import "fmt"

// english is the default: what is shown before Set, and what any language
// but "zh" is (config.Parse has said it is one of the two).
var english = true

// Set picks the language, "en" or "zh", once, as the program starts.
func Set(lang string) { english = lang != "zh" }

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
