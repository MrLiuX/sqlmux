#!/usr/bin/env bash
# F15.9 JSON 视图的键和编辑器的搜索高亮（specs/m15-pg-databases/task.md F15.9；tech-design §6.4 jsonview、§7.6「JSON 视图」、§11「搜索」、§7.7 search / search_current）
# JSON 视图是 gj（不是 gJ）；它的翻页和刷新沿用表格的键：用户在 [keys.grid] 里改成 < / >（并解绑 ] [）、把 R 换成 <F5>，JSON 视图里跟着变，提示行读出新键；
# 编辑器（query、值编辑器、JSON 视图）的 / 回车后所有匹配 search 底、当前匹配 search_current 底，n / N 换当前匹配，命令行留着 /模式 和右端 [1/3]（绕回加 W、超过 99 写 >99），下一个别的键把这行收掉，
# NORMAL 下 esc 清掉高亮；主题的 search / search_current 键，没写时退回 visual / yank。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
SRCH=#c8793a CUR=#e0af68                                                       # search / search_current, as the default theme has them (§7.7)
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.5; }
sch() { e2e_bgcells "$SRCH" | awk '{ printf "%s", $3 }'; }                      # the characters on the search background
cur() { e2e_bgcells "$CUR" | awk '$2 < 44 { printf "%s", $3 }'; }              # the ones on the current match's (the status bar's last row has the same color for its mode block)
curx() { e2e_bgcells "$CUR" | awk '$2 < 44 { print $1; exit }'; }               # where the current match starts
cmdline() { e2e_plain | python3 -c '
import sys
for l in sys.stdin:
    i = l.find(sys.argv[1])
    if i >= 0 and l[i + len(sys.argv[1]):].lstrip(" ").startswith(("W [", "[", "│")) and "select" not in l:
        print(l[i:].split("│")[0].rstrip()); break' "$1"; }              # the editor's command row from the pattern on: "/foo      W [1/3]"
tail_of() { cmdline "$1" | grep -oE "(W )?\[[>0-9a-z]*/[>0-9a-z]*\]$"; }

# ---- query：回车后的高亮和计数
start; sleep 1; key C-l; sleep 0.6
key i; e2e_type "select 1 as foo, 2 as foo, 3 as Foo"; key Escape; sleep 0.3
e2e_type "/foo"; sleep 0.3
check "输入 /foo 时（还没回车）：没有高亮（不做 incsearch）" eval '[[ -z $(sch) && -z $(cur) ]]' || { sch; cur; }
key Enter; sleep 0.5
check "回车：三处匹配（foo foo Foo，smartcase 全小写不分大小写）都有底色，当前一处是 search_current，其余两处是 search" eval '[[ $(cur) == foo && $(sch) == fooFoo ]]' || { echo "  cur=[$(cur)] search=[$(sch)]"; }
check "命令行留着 /foo，右端是 W [1/3]（光标在末尾，往下找绕回了第一处）" eval '[[ $(tail_of /foo) == "W [1/3]" ]]' || cmdline /foo
X1=$(curx)
key n; sleep 0.4
check "n：当前匹配换成第二处（底色跟着换：current 到第二个 foo，第一个变成 search），计数 [2/3]，没有 W" eval '[[ $(tail_of /foo) == "[2/3]" && $(curx) -gt $X1 && $(cur) == foo && $(sch) == fooFoo ]]' || { tail_of /foo; echo "  cur=[$(cur)] x=$(curx) was $X1"; }
key n; sleep 0.4
check "n：第三处（Foo），[3/3]；current 是 Foo" eval '[[ $(tail_of /foo) == "[3/3]" && $(cur) == Foo ]]' || { tail_of /foo; cur; }
key n; sleep 0.4
check "再 n：绕回第一处，W [1/3]" eval '[[ $(tail_of /foo) == "W [1/3]" && $(curx) == $X1 ]]' || { tail_of /foo; curx; }
key N; sleep 0.4
check "N：往回绕到第三处，W [3/3]（命令行显示的是 ?foo：方向反了，同 nvim）" eval '[[ $(tail_of "?foo") == "W [3/3]" && $(cur) == Foo ]]' || tail_of "?foo"
key l; sleep 0.4
check "下一个不是 n / N 的键（l）：命令行那一行的 /foo 和计数收掉" eval '[[ -z $(cmdline /foo) ]]' || cmdline /foo
check "高亮还留着（l 不清）：当前一处加其余两处" eval '[[ -n $(cur) && $(sch | wc -c) -ge 6 ]]' || { cur; sch; }
key Escape; sleep 0.4
check "NORMAL 下 esc：高亮清掉" eval '[[ -z $(sch) && -z $(cur) ]]' || { sch; cur; }
key n; sleep 0.4
check "esc 之后 n 再亮起来（搜索的模式还记得），计数回来" eval '[[ -n $(cur) && -n $(tail_of /foo) ]]' || { cur; tail_of /foo; }
key Escape; sleep 0.3
e2e_type "/Foo"; key Enter; sleep 0.5
check "smartcase：含大写的 /Foo 只匹配 Foo 一处：[1/1]，高亮只有 Foo" eval '[[ $(cur) == Foo && -z $(sch) && $(tail_of /Foo) == *"[1/1]" ]]' || { cur; sch; tail_of /Foo; }
key Escape; sleep 0.3
e2e_type "/zzz"; key Enter; sleep 0.5
check "没有匹配的 /zzz：没有高亮、没有计数" eval '[[ -z $(sch) && -z $(cur) && -z $(tail_of /zzz) ]]' || { sch; cur; tail_of /zzz; }
_e2e_kill

# ---- 超过 99：[2/>99]、[>99/>99]
start; sleep 1; key C-l; sleep 0.6
key i; paste "$(python3 -c 'print("x" * 120)')"; key Escape; key 0; sleep 0.3
e2e_type "/x"; key Enter; sleep 0.5
check "120 处匹配：计数 [2/>99]（光标在第一个 x 上，下一处是第 2 处；总数写 >99）" eval '[[ $(tail_of /x) == "[2/>99]" ]]' || cmdline /x
e2e_type "98n"; sleep 0.6
check "98n 到第 100 处：[>99/>99]（当前的序号也超过 99）" eval '[[ $(tail_of /x) == "[>99/>99]" ]]' || cmdline /x
_e2e_kill

# ---- 主题：search / search_current 两个键；没写时退回 visual / yank
mkdir -p "$D/th/themes"; printf 'theme = "x"\n' >"$D/th/config.toml"; printf 'search = "#112233"\nsearch_current = "#445566"\n' >"$D/th/themes/x.toml"
start -C "$D/th"; sleep 1; key C-l; sleep 0.6
key i; e2e_type "select 1 as foo, 2 as foo"; key Escape; e2e_type "/foo"; key Enter; sleep 0.5
check "主题写了 search = #112233、search_current = #445566：其余匹配和当前匹配用它们" eval '[[ $(e2e_bgcells "#112233" | wc -l | tr -d " ") == 3 && $(e2e_bgcells "#445566" | wc -l | tr -d " ") == 3 ]]' || { e2e_bgcells "#112233"; e2e_bgcells "#445566"; }
_e2e_kill
mkdir -p "$D/th2/themes"; printf 'theme = "x"\n' >"$D/th2/config.toml"; printf 'visual = "#112233"\nyank = "#445566"\n' >"$D/th2/themes/x.toml"
start -C "$D/th2"; sleep 1; key C-l; sleep 0.6
key i; e2e_type "select 1 as foo, 2 as foo"; key Escape; e2e_type "/foo"; key Enter; sleep 0.5
check "主题没写 search / search_current：退回 visual（#112233）和 yank（#445566）" eval '[[ $(e2e_bgcells "#112233" | wc -l | tr -d " ") -ge 3 && $(e2e_bgcells "#445566" | wc -l | tr -d " ") -ge 3 ]]' || { e2e_bgcells "#112233"; e2e_bgcells "#445566"; }
_e2e_kill

# ---- SQLite：值编辑器、JSON 视图里的搜索，gj 和用户的翻页键
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
sqlite3 "$DB" "create table n (id integer primary key, note text); insert into n values (1, 'foo bar foo'), (2, 'x')"
mkdir -p "$D/keys"; printf '[keys.grid]\n"]" = ""\n"[" = ""\n">" = "grid.page.next"\n"<" = "grid.page.prev"\n"R" = ""\n"<F5>" = "grid.refresh"\n' >"$D/keys/keymaps.toml"
in_json() { e2e_plain | sed -n 3p | grep -qF "JSON"; }
e2e_start "$E2E_BIN $DB"; wait_for 5 flag_is alternate_on 1; sleep 1; solo
open_table n; wait_for 8 settled
key l; key e; sleep 0.7
e2e_type "/foo"; key Enter; sleep 0.5
check "值编辑器：/foo 回车，两个 foo 都有底色（当前一处 search_current，另一处 search），命令行 [2/2]（光标在第一个 foo 上，下一处是第二个）" eval '[[ $(cur) == foo && $(sch) == foo && $(tail_of /foo) == "[2/2]" ]]' || { cur; sch; tail_of /foo; }
key n; sleep 0.4
check "n：回到第一个（W [1/2]），当前和其余的底色换位" eval '[[ $(tail_of /foo) == "W [1/2]" && $(cur) == foo ]]' || tail_of /foo
key Escape; sleep 0.4
check "值编辑器里高亮亮着时第一下 esc 只清高亮（M15 reviewer 退回后的 §11：编辑器还开着）" eval '[[ -z $(sch) && -z $(cur) ]] && e2e_plain | grep -qF " · TEXT"' || { sch; cur; }
key Escape; sleep 0.4
check "再按一下 esc 才关掉值编辑器" eval '! e2e_plain | grep -qF " · TEXT"'
key g J; sleep 0.8
check "gJ 不再是 JSON 视图（原来的键；表格里没绑）" eval '! in_json'
key g j; sleep 0.8
check "gj 进 JSON 视图" eval 'in_json'
e2e_type '/"id"'; key Enter; sleep 0.5
check "JSON 视图里 / 搜索：每个 \"id\" 有底色，[1/2]（两行：id 1 和 id 2），当前一处 search_current" eval '[[ $(tail_of "/\"id\"") == "[1/2]" && -n $(cur) && -n $(sch) ]]' || { tail_of '/"id"'; cur; sch; }
key n; sleep 0.4
check "n：[2/2]" eval '[[ $(tail_of "/\"id\"") == "[2/2]" ]]' || tail_of '/"id"'
key Escape; sleep 0.4
check "JSON 视图里高亮亮着时第一下 esc 只清高亮，还在 JSON 视图" eval '[[ -z $(sch) && -z $(cur) ]] && in_json' || { sch; cur; }
e2e_type '/"id"'; key Enter; sleep 0.4; key q; sleep 0.5
check "q 直接回表格（不先清高亮）" eval '! in_json'
key g j; sleep 0.6
e2e_type '/"id"'; key Enter; sleep 0.4; key Escape; key Escape; sleep 0.5
check "两下 esc：先清高亮、再回表格" eval '! in_json'
key g j; sleep 0.6
check "再 gj 进 JSON 视图" eval 'in_json'
key g j; sleep 0.6
check "再按 gj 回表格" eval '! in_json'
_e2e_kill
e2e_start -C "$D/keys" "$E2E_BIN $DB"; wait_for 5 flag_is alternate_on 1; sleep 1; solo
open_table t_order; wait_for 8 settled
key g j; sleep 1; wait_for 5 settled
check "用户把翻页改成 < / >：JSON 视图的提示行读出来：>/< 翻页 · esc 回表格" eval 'e2e_plain | grep -qF ">/< 翻页 · esc 回表格"' || e2e_plain | tail -4 | cut -c30-170
key ">"; sleep 1; wait_for 5 settled
check "> 翻页：PAGE 2/60，JSON 的第一个对象是 id 101" eval 'qb | grep -qF "PAGE 2/60" && e2e_plain | grep -qF "\"id\": 101"' || qb
key "]"; sleep 0.8
check "] 已经被用户解绑：不翻页（还是 PAGE 2/60）" eval 'qb | grep -qF "PAGE 2/60"'
key "<"; sleep 1; wait_for 5 settled
check "< 翻回：PAGE 1/60，第一个对象 id 1" eval 'qb | grep -qF "PAGE 1/60" && e2e_plain | grep -qF "\"id\": 1,"'
sqlite3 "$DB" "update t_order set note = 'CHANGED5' where id = 1"
key R; sleep 0.8
check "R 被用户解绑：不刷新（内容里还没有 CHANGED5）" eval '! e2e_plain | grep -qF CHANGED5'
key F5; sleep 1.2; wait_for 5 settled
check "<F5>（用户绑的 grid.refresh）刷新：库里改了 id 1 的 note，JSON 里跟着变" eval 'e2e_plain | grep -qF "\"note\": \"CHANGED5\""' || e2e_plain | sed -n 4,12p | cut -c30-150
key Escape; sleep 0.5
check "esc 回表格" eval '! in_json && e2e_plain | grep -qF " id │ user_id"'
e2e_done
