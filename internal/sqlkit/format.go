package sqlkit

import (
	_ "embed"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// sql-formatter 15.9.0's standalone bundle, nearley's runtime in it (MIT,
// both in sql-formatter.LICENSE), run in goja (§9.5).
//
//go:embed sql-formatter.min.js
var formatterJS string

// unicodeNames are the long Unicode property names in the bundle goja's
// regexps cannot read, what stands for them, and how often each is there:
// a new bundle with other counts fails the test (§9.5).
var unicodeNames = []struct {
	from, to string
	n        int
}{{`\p{Alphabetic}`, `\p{L}`, 3}, {`\p{Mark}`, `\p{M}`, 1}, {`\p{Decimal_Number}`, `\p{Nd}`, 1}}

// formatter is the VM, made the first time it is needed and kept; goja
// runs one call at a time.
var formatter struct {
	sync.Mutex
	vm     *goja.Runtime
	format goja.Callable
	err    error
}

// Options are what sql-formatter is told besides the language.
type Options struct {
	KeywordCase string // lower, upper or preserve; data types' too (§9.5)
	TabWidth    int
}

// FormatTimeout bounds a format, here or by formatprg (§9.5): goja takes
// seconds over a few KB of SQL. A var for the tests.
var FormatTimeout = 5 * time.Second

// ErrTimeout is a format that took longer than FormatTimeout.
var ErrTimeout = errors.New("timeout")

// Format is sql as sql-formatter lays it out in d's language (§9.5). Past
// FormatTimeout the VM is interrupted, to be used again.
func Format(sql string, d Dialect, o Options) (string, error) {
	f := &formatter
	f.Lock()
	defer f.Unlock()
	if f.vm == nil && f.err == nil {
		f.vm, f.format, f.err = load()
	}
	if f.err != nil {
		return "", f.err
	}
	opts := f.vm.NewObject()
	_ = opts.Set("language", map[Dialect]string{PG: "postgresql", MySQL: "mysql"}[d])
	_ = opts.Set("keywordCase", o.KeywordCase)
	_ = opts.Set("dataTypeCase", o.KeywordCase) // PG's type names are keywords (15.x has them apart)
	_ = opts.Set("tabWidth", o.TabWidth)
	vm, fired := f.vm, make(chan struct{})
	timer := time.AfterFunc(FormatTimeout, func() { vm.Interrupt(ErrTimeout); close(fired) })
	v, err := f.format(goja.Undefined(), vm.ToValue(sql), opts)
	if !timer.Stop() {
		<-fired // it may be interrupting still
	}
	vm.ClearInterrupt() // an interrupt that came as the format ended waits for the next one
	if ie := (*goja.InterruptedError)(nil); errors.As(err, &ie) {
		return "", ErrTimeout
	}
	if ex := (*goja.Exception)(nil); errors.As(err, &ex) { // what the formatter said, its first line
		msg := ex.Value().String()
		if o, ok := ex.Value().(*goja.Object); ok {
			msg = o.Get("message").String()
		}
		first, _, _ := strings.Cut(msg, "\n")
		return "", errors.New(first)
	}
	if err != nil {
		return "", err
	}
	return v.String(), nil
}

// FormatAll is Format of a text of several statements, each by a call of its
// own: the VM's time grows faster than the text, 7 KB of statements taking 4 s
// at once and 1.4 s one by one. Between them is what sql-formatter leaves, an
// empty line, and the comments there as they were (§9.5). One statement, or a
// text the split has no account of (an empty statement, a comment before a ;),
// goes in one call.
func FormatAll(s string, d Dialect, o Options) (string, error) {
	stmts := Statements(s, d)
	if len(stmts) < 2 {
		return Format(s, d, o)
	}
	var units []string
	comment := strings.TrimSpace(s[:stmts[0].Start]) // what is before the next statement
	for i, st := range stmts {
		end := len(s)
		if i+1 < len(stmts) {
			end = stmts[i+1].Start
		}
		terminated, after, ok := terminator(s[st.End:end], d)
		if !ok {
			return Format(s, d, o)
		}
		text := s[st.Start:st.End]
		if terminated {
			text += ";"
		}
		out, err := Format(text, d, o)
		if err != nil {
			return "", err
		}
		if comment != "" {
			out = comment + "\n" + out
		}
		units, comment = append(units, out), after
	}
	if comment != "" {
		units = append(units, comment)
	}
	return strings.Join(units, "\n\n"), nil
}

// terminator reads what lies after a statement up to the next: blanks, the ;
// that ends it if it is there, and the comments, which are after. ok is false
// for anything else.
func terminator(gap string, d Dialect) (terminated bool, after string, ok bool) {
	rest := 0
	for _, t := range Scan(gap, d) {
		switch {
		case t.Kind == Space:
		case t.Kind == Punct && gap[t.Start] == ';' && !terminated:
			terminated, rest = true, t.End
		case t.Kind == Comment && terminated:
		default:
			return false, "", false
		}
	}
	return terminated, strings.TrimSpace(gap[rest:]), true
}

func load() (*goja.Runtime, goja.Callable, error) {
	js := formatterJS
	for _, u := range unicodeNames {
		js = strings.ReplaceAll(js, u.from, u.to)
	}
	vm := goja.New()
	if _, err := vm.RunString(js); err != nil { // the UMD bundle sets the global sqlFormatter
		return nil, nil, err
	}
	format, ok := goja.AssertFunction(vm.Get("sqlFormatter").ToObject(vm).Get("format"))
	if !ok {
		return nil, nil, errors.New("sql-formatter.min.js: no sqlFormatter.format")
	}
	return vm, format, nil
}
