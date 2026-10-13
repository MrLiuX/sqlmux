#!/usr/bin/env bash
# F15.9b 值编辑器、JSON 视图提示行上「关闭 / 回表格」按钮：高亮亮着也直接关；有没写回的改动时先问「放弃修改？」（docs 79fd4e1）
# 与 esc 的区别：esc 在高亮亮着时只清高亮，点按钮不分，直接关。SQLite 表 n。
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

DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
sqlite3 "$DB" "create table n (id integer primary key, note text); insert into n values (1, 'foo bar foo'), (2, 'x')"
in_json() { e2e_plain | sed -n 3p | grep -qF "JSON"; }
e2e_start "$E2E_BIN $DB"; wait_for 5 flag_is alternate_on 1; sleep 1; solo
open_table n; wait_for 8 settled
# click the first screen row's item that holds $1: its first character
clk() { local r; r=$(e2e_plain | python3 -c '
import sys, unicodedata
w = lambda c: 2 if unicodedata.east_asian_width(c) in "WF" else 1
for y, l in enumerate(sys.stdin, 1):
    i = l.rfind(sys.argv[1])
    if i >= 0: print(sum(map(w, l[:i])) + 1, y); break' "$1"); [[ -n $r ]] || { echo "  no $1 on the screen"; return 1; }; e2e_click ${r% *} ${r#* }; sleep 0.8; }
pop_open() { e2e_plain | grep -qF " · TEXT"; }

key l; key e; sleep 0.7
e2e_type "/foo"; key Enter; sleep 0.5
check "（准备）值编辑器里搜索 foo：高亮亮着" eval '[[ -n $(sch) || -n $(cur) ]]'
clk "esc 关闭"
check "点「esc 关闭」：高亮亮着也直接关（esc 要两下）" eval '! pop_open'
key g j; sleep 0.8
e2e_type "/note"; key Enter; sleep 0.5
check "（准备）JSON 视图里搜索 note：在 JSON 视图，高亮亮着" eval 'in_json && [[ -n $(sch) || -n $(cur) ]]'
clk "esc 回表格"
check "点「esc 回表格」：直接回表格（esc 要两下）" eval '! in_json'
key l; key e; sleep 0.7; key A; e2e_type zz; key Escape; sleep 0.4
check "（准备）值编辑器里改了字（NORMAL，没写回）" eval 'pop_open'
clk "esc 关闭"
check "点「esc 关闭」有没写回的改动：不关，问「放弃修改？」" eval 'pop_open && e2e_plain | grep -qF "放弃修改"' || e2e_plain | tail -4 | cut -c1-160
key n; sleep 0.5
check "回答 n：值编辑器还在" eval 'pop_open'
clk "esc 关闭"; key y; sleep 0.6
check "再点关闭、y：放弃，关了" eval '! pop_open'
e2e_done
