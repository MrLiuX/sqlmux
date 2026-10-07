package keymap

import (
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"sqlmux/internal/config"
	"sqlmux/internal/i18n"
)

//go:embed default.toml
var defaultTOML string

// noVimTOML is what vim = false adds over it (§6.9).
//
//go:embed default-novim.toml
var noVimTOML string

// tables is every [keys.*] / [map.*] table a config may use (§6.4, §6.6),
// in resolution priority order for the export. which-key is no scope: keys
// pressed over it resolve as without it (§6.5).
var tables = []string{
	"keys.confirm", "keys.palette", "keys.conns", "keys.form", "keys.where", "keys.cols", "keys.dropdown", "keys.sessions",
	"keys.complete", "keys.newrow", "keys.options", "keys.segments", "keys.keyhelp", "keys.cell", "keys.input",
	"keys.wherenormal", "keys.result", "keys.grid", "keys.tree", "keys.console", "keys.landing", "keys.normal", "keys.global",
	"map.console.normal", "map.console.visual", "map.grid.normal", "map.grid.visual",
	"map.tree.normal", "map.tree.visual", "map.normal", "map.visual",
}

// Binding is one effective line of a table.
type Binding struct {
	Keys   []Key  // as written, <Leader> not expanded
	Action string // keys.* tables: action with args, e.g. "window.select 3"
	RHS    []Key  // map.* tables: the key sequence it stands for (noremap)
}

// Problem is a config line the keymap rejected or warns about.
type Problem struct {
	Table, Key, Msg string
}

func (p Problem) String() string { return fmt.Sprintf("[%s] %s: %s", p.Table, p.Key, p.Msg) }

// Map is the effective keymap: defaults with the user's config on top.
type Map struct {
	Leader  []Key
	Timeout time.Duration

	tables        map[string][]Binding
	novim         map[string]bool      // table + key of what default-novim.toml bound: the hints name these first (§6.9)
	defaults      map[string][]Binding // for the export: what the user changed
	defaultLeader []Key
	tries         map[string]*node // merged per context, see trie
}

// New builds the keymap from the embedded defaults and the user's
// keymaps.toml in user. Lines with problems are skipped; the rest still
// apply.
func New(user *config.Config) (*Map, []Problem) {
	leader, def, err := config.ParseKeys(defaultTOML)
	if err != nil {
		panic("default.toml: " + err.Error())
	}
	m := &Map{tables: map[string][]Binding{}, tries: map[string]*node{}}
	m.Leader, _ = Parse(leader)
	m.defaultLeader = m.Leader
	var problems []Problem
	if user.Leader != "" {
		if ks, err := Parse(user.Leader); err != nil || len(ks) == 0 || slices.Contains(ks, Leader) {
			problems = append(problems, Problem{"keys", "leader", i18n.T("leader 写法不对：%q", user.Leader)})
		} else {
			m.Leader = ks
		}
	}
	// The leader is final before any binding is merged: "same key" is decided
	// on expanded sequences, so a user's "<Space>s" rebinds the default "<Leader>s".
	if ps := m.apply(def); len(ps) > 0 {
		panic(fmt.Sprint("default.toml: ", ps))
	}
	if !user.Vim { // the same defaults, for the export and the check, with the arrow keys on
		_, nv, err := config.ParseKeys(noVimTOML)
		if err != nil {
			panic("default-novim.toml: " + err.Error())
		}
		if ps := m.apply(nv); len(ps) > 0 {
			panic(fmt.Sprint("default-novim.toml: ", ps))
		}
		m.novim = map[string]bool{}
		for _, b := range nv {
			ks, _ := Parse(b.Key)
			m.novim[b.Table+" "+String(ks)] = true
		}
	}
	m.defaults = map[string][]Binding{}
	for t, bs := range m.tables {
		m.defaults[t] = slices.Clone(bs)
	}
	m.Timeout = time.Duration(user.Timeoutlen) * time.Millisecond
	problems = append(problems, m.apply(user.Bindings)...)
	return m, append(problems, m.ambiguities()...)
}

// apply merges one file's bindings: a key already bound in the same table
// is rebound in place (an ordinary remap, not a conflict), "" unbinds.
func (m *Map) apply(bs []config.Binding) []Problem {
	var ps []Problem
	seen := map[string]string{} // table + normalized key → spelling, within this file
	for _, b := range bs {
		bad := func(msg string) { ps = append(ps, Problem{b.Table, b.Key, msg}) }
		if !slices.Contains(tables, b.Table) {
			bad(i18n.T("没有这个表"))
			continue
		}
		keys, err := Parse(b.Key)
		if err != nil || len(keys) == 0 {
			bad(i18n.T("键位写法不对：%v", err))
			continue
		}
		id := b.Table + " " + String(m.expand(keys))
		if prev, dup := seen[id]; dup {
			bad(i18n.T("与 %q 是同一个键", prev))
			continue
		}
		seen[id] = b.Key
		nb := Binding{Keys: keys, Action: b.Value}
		if strings.HasPrefix(b.Table, "map.") {
			if nb.RHS, err = Parse(b.Value); err != nil {
				bad(i18n.T("映射写法不对：%v", err))
				continue
			}
			nb.Action = ""
		}
		m.set(b.Table, nb, b.Value == "")
	}
	return ps
}

func (m *Map) set(table string, b Binding, unbind bool) {
	bs := m.tables[table]
	i := slices.IndexFunc(bs, func(o Binding) bool { return m.same(o.Keys, b.Keys) })
	switch {
	case unbind && i >= 0:
		bs = slices.Delete(bs, i, i+1)
	case unbind:
	case i >= 0:
		bs[i] = b
	default:
		bs = append(bs, b)
	}
	m.tables[table] = bs
}

// ambiguities warns where one binding is a prefix of another in the same
// table: the shorter one then fires on its own only after timeoutlen, or
// when the next key continues none of them (§6.7, F8.15).
// ponytail: same table only; a grid "g" against normal's "gt" also makes an
// ambiguous node once merged. Check per context if that bites.
func (m *Map) ambiguities() []Problem {
	var ps []Problem
	for _, t := range tables {
		for _, a := range m.tables[t] {
			for _, b := range m.tables[t] {
				ka, kb := m.expand(a.Keys), m.expand(b.Keys)
				if len(ka) < len(kb) && slices.Equal(ka, kb[:len(ka)]) {
					ps = append(ps, Problem{t, String(a.Keys), fmt.Sprintf(i18n.T("是 %s 的前缀，单独按时要等 timeoutlen 之后才执行"), String(b.Keys))})
					break
				}
			}
		}
	}
	return ps
}

// same reports whether two bindings are the same key once <Leader> is expanded.
func (m *Map) same(a, b []Key) bool { return slices.Equal(m.expand(a), m.expand(b)) }

func (m *Map) expand(ks []Key) []Key {
	if !slices.Contains(ks, Leader) {
		return ks
	}
	var out []Key
	for _, k := range ks {
		if k == Leader {
			out = append(out, m.Leader...)
		} else {
			out = append(out, k)
		}
	}
	return out
}

// Hint is how the UI shows the first key bound to action in scope, or "" when
// unbound (§6.7). All key text on screen goes through here. With vim = false
// a key default-novim.toml bound comes first: ↓ for the grid, not j.
func (m *Map) Hint(action, scope string) string {
	first := ""
	for _, b := range m.tables["keys."+scope] {
		if b.Action != action {
			continue
		}
		if m.novim["keys."+scope+" "+String(b.Keys)] {
			return Display(m.expand(b.Keys))
		}
		if first == "" {
			first = Display(m.expand(b.Keys))
		}
	}
	return first
}

// Actions lists every bound action ID, args stripped, once each: app's
// tests check it against the registry's titles.
func (m *Map) Actions() []string {
	var ids []string
	for _, t := range tables {
		for _, b := range m.tables[t] {
			if id, _, _ := strings.Cut(b.Action, " "); id != "" && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// Markdown renders the effective keymap as a table (`sqlmux keys`).
func (m *Map) Markdown() string {
	var b strings.Builder
	b.WriteString(i18n.T("| 作用域 | 键 | 操作 |\n|---|---|---|\n"))
	for _, t := range tables {
		for _, bd := range m.tables[t] {
			op := bd.Action
			if bd.RHS != nil {
				op = "→ " + String(bd.RHS)
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", strings.TrimPrefix(t, "keys."), mdEscape(Display(m.expand(bd.Keys))), mdEscape(op))
		}
	}
	return b.String()
}

func mdEscape(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// when is the note above each section of the export: when its keys apply (§6.4).
var when = map[string]string{
	"keys.global":      i18n.N("任何时候，输入框里也生效"),
	"keys.normal":      i18n.N("NORMAL 模式，焦点在任何 pane"),
	"keys.grid":        i18n.N("焦点在表格（NORMAL）"),
	"keys.tree":        i18n.N("焦点在 schema 树（NORMAL）"),
	"keys.console":     i18n.N("console 的 NORMAL / VISUAL（M3）"),
	"keys.landing":     i18n.N("引导页：新 tab、没有 tab 的 pane（M3）"),
	"keys.result":      i18n.N("结果区，叠在 grid 之上（M3）"),
	"keys.palette":     i18n.N("命令面板打开时"),
	"keys.dropdown":    i18n.N("ORDER / LIMIT 下拉框打开时"),
	"keys.complete":    i18n.N("补全列表弹出时（↵、esc 由输入框处理）"),
	"keys.where":       i18n.N("WHERE 的历史 / 收藏下拉打开时"),
	"keys.input":       i18n.N("任何输入框里"),
	"keys.wherenormal": i18n.N("WHERE 输入框在 NORMAL 下（F3.39）"),
	"keys.cols":        i18n.N("COLS 列表打开时"),
	"keys.cell":        i18n.N("编辑单元格时（M2）"),
	"keys.sessions":    i18n.N("session 列表打开时（M5）"),
	"keys.form":        i18n.N("连接表单打开时（M5）"),
	"keys.conns":       i18n.N("连接管理的列表有焦点时（F8.16）"),
	"keys.confirm":     i18n.N("确认框打开时"),
	"keys.newrow":      i18n.N("编辑还没保存的新行的格时，叠在选项 / 分段浮层之上（F3.33）"),
	"keys.options":     i18n.N("非时间列的单元格，选项浮层展开、而且有选项时"),
	"keys.segments":    i18n.N("时间列的单元格，分段浮层展开时"),
	"keys.keyhelp":     i18n.N("? 键位帮助打开时"),
}

// whenPlainConsole is keys.console's note with vim = false, where the console
// has no NORMAL and the Ctrl keys reach its editor (§6.9).
var whenPlainConsole = i18n.N("console 的编辑器，只有 Ctrl 开头的键生效，C-h 除外，它是退格（F6.6）；NORMAL / VISUAL 是 vim 模式的")

// TOML renders the effective keymap as a whole keymaps.toml (`sqlmux keys
// --format toml`, §6.7): every scope, with its defaults commented out, so
// an old export never pins a new version's defaults, and only what the
// user changed live, in the default's place: a rebinding, an unbinding
// `"key" = ""`, then keys of their own. Loaded back it gives the same
// keymap. titles come from app's registry, which keymap cannot import;
// titled actions bound nowhere come as a commented `"" = "<action>"` under
// the scope their ID prefix names.
func (m *Map) TOML(titles map[string]string) string {
	bound := m.Actions()
	unbound := map[string][]string{}
	for _, id := range slices.Sorted(maps.Keys(titles)) {
		if slices.Contains(bound, id) {
			continue
		}
		t := "keys.normal"
		if p, _, _ := strings.Cut(id, "."); slices.Contains([]string{"grid", "tree", "console", "result"}, p) {
			t = "keys." + p
		}
		unbound[t] = append(unbound[t], fmt.Sprintf(`# "" = %s  # %s`, strconv.Quote(id), titles[id]))
	}
	var b strings.Builder
	leader := "leader = " + strconv.Quote(String(m.Leader))
	if slices.Equal(m.Leader, m.defaultLeader) {
		leader = "# " + leader
	}
	fmt.Fprintf(&b, "[keys]\n%s\n", leader)
	for _, t := range tables {
		lines := []string{}
		for _, d := range m.defaults[t] {
			i := slices.IndexFunc(m.tables[t], func(o Binding) bool { return m.same(o.Keys, d.Keys) })
			switch {
			case i < 0:
				lines = append(lines, strconv.Quote(String(d.Keys))+` = ""`)
			case m.tables[t][i].Action == d.Action && slices.Equal(m.tables[t][i].RHS, d.RHS):
				lines = append(lines, "# "+tomlLine(d, titles))
			default:
				lines = append(lines, tomlLine(m.tables[t][i], titles))
			}
		}
		for _, bd := range m.tables[t] {
			if !slices.ContainsFunc(m.defaults[t], func(d Binding) bool { return m.same(d.Keys, bd.Keys) }) {
				lines = append(lines, tomlLine(bd, titles))
			}
		}
		lines = append(lines, unbound[t]...)
		if len(lines) > 0 {
			b.WriteString("\n")
			note := when[t]
			if t == "keys.console" && m.novim != nil {
				note = whenPlainConsole
			}
			if note != "" {
				fmt.Fprintf(&b, "# %s\n", i18n.T(note))
			}
			fmt.Fprintf(&b, "[%s]\n%s\n", t, strings.Join(lines, "\n"))
		}
	}
	b.WriteString(i18n.T("\n# 用户映射（§6.6）：键 → 一串键（noremap）。[map.normal] 所有 pane 通用，下面这种只在表格里生效\n# [map.grid.normal]\n# \"J\" = \"5j\"\n"))
	return b.String()
}

// tomlLine is b as a keymaps.toml line, with its action's title.
func tomlLine(b Binding, titles map[string]string) string {
	line := strconv.Quote(String(b.Keys)) + " = "
	if b.RHS != nil {
		return line + strconv.Quote(String(b.RHS))
	}
	line += strconv.Quote(b.Action)
	if id, _, _ := strings.Cut(b.Action, " "); titles[id] != "" {
		line += "  # " + titles[id]
	}
	return line
}
