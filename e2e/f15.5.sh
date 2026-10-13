#!/usr/bin/env bash
# F15.5 值编辑器的宽度按内容（specs/m15-pg-databases/task.md F15.5；tech-design §10.3「值编辑器」）
# 浮窗宽 = 最长一行的显示宽度 + 边框、内边距：最少 pane 内容区的一半（最小 40）、最多 pane 内容区的宽；左边对齐光标所在格，右边出界整体左移到贴 pane 右边。
# SQLite 文件里四行：短文本、80 个字的长行、300 个字的超长行、右边的列里 110 个字的长行（要左移）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
DB=$D/w.db
python3 - "$DB" <<'P'
import sqlite3, sys
c = sqlite3.connect(sys.argv[1])
c.execute("create table t (id integer primary key, a text, b text)")
c.execute("insert into t values (1, 'short', 'x')")
c.execute("insert into t values (2, ?, 'y')", ('L' * 80,))
c.execute("insert into t values (3, ?, 'z')", ('M' * 300,))
c.execute("insert into t values (4, 'w', ?)", ('R' * 110,))
c.commit()
P
# the value editor's top border on the screen: "x0 x1 y" in display columns (a wide character counts two), "" when there is none
vbox() { e2e_plain | python3 -c '
import sys, unicodedata
w = lambda c: 2 if unicodedata.east_asian_width(c) in "WF" else 1
for y, l in enumerate(sys.stdin, 1):
    l = l.rstrip("\n"); i = l.find("┌─ ")
    if i < 0 or " · TEXT" not in l[i:]: continue
    x0 = sum(map(w, l[:i])); j = l.find("┐", i)
    print(x0 + 1, x0 + sum(map(w, l[i:j + 1])), y); break'; }
vw() { local g; g=($(vbox)); echo $((g[1] - g[0] + 1)); }
vx() { local g; g=($(vbox)); echo "${g[0]}"; }
cell() { key g g; key 0; local i; for ((i = 1; i < $2; i++)); do key j; done; for ((i = 0; i < $1; i++)); do key l; done; }   # row $2, the $1 columns right of id

e2e_start "$E2E_BIN $DB"; wait_for 5 flag_is alternate_on 1; sleep 1; solo
open_table t; wait_for 8 settled
# the pane's content area: the data pane's frame from the screen (its top border's ┌ to ┐)
PANE=$(e2e_plain | python3 -c '
import sys
l = sys.stdin.read().split("\n")[0]; i = l.find("┌─ ①"); print(i + 1, len(l))')
read -r P0 P1 <<<"$PANE"; PW=$((P1 - P0 + 1 - 2))   # the content width, the frame off
echo "  pane x $P0..$P1, content $PW wide"
ACOL=$(e2e_plain | python3 -c '
import sys, unicodedata
w = lambda c: 2 if unicodedata.east_asian_width(c) in "WF" else 1
for l in sys.stdin:
    i = l.find("│ a ")
    if i >= 0 and "id" in l: print(sum(map(w, l[:i])) + 1); break')
cell 1 1; key e; sleep 0.7
W1=$(vw); X1=$(vx)
check "短文本（short）：宽 = pane 内容区的一半（$((PW / 2))，最小 40），不随内容缩小" eval '[[ $W1 -ge 40 && $W1 -ge $((PW / 2 - 1)) && $W1 -le $((PW / 2 + 1)) ]]' || { echo "  w=$W1"; e2e_plain | sed -n 1,14p; }
key Escape; sleep 0.3
cell 1 2; key e; sleep 0.7
W2=$(vw)
check "80 个字的长行：撑开，比一半宽，装得下这一行（≥ 80 + 边框内边距）又没到 pane 宽" eval '[[ $W2 -gt $W1 && $W2 -ge 84 && $W2 -lt $PW ]]' || { echo "  w=$W2 (half $W1)"; e2e_plain | sed -n 1,14p; }
check "长行：整行都看得见（80 个 L 一个不少）" eval '[[ $(e2e_plain | grep -c "LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL") == 0 && $(e2e_plain | grep -o "L\{80\}" | head -1 | wc -c) -eq 81 ]]' || e2e_plain | sed -n 5,9p
# a 列的格从表头里 "│ a" 的那条竖线算起；浮窗没开时的屏幕在 Escape 之后才有，所以先记下来
check "左边对齐光标所在格（a 列）：浮窗的左边就是 a 列的左边（${ACOL}），不是居中" eval '[[ $(vx) -ge $ACOL && $(vx) -le $((ACOL + 2)) ]]' || { vbox; echo "a column at $ACOL"; }
key Escape; sleep 0.3
cell 1 3; key e; sleep 0.7
W3=$(vw)
check "300 个字的超长行：到 pane 内容区的宽为止（${PW}），不再宽、也不超出 pane" eval '[[ $W3 -ge $((PW - 1)) && $W3 -le $((PW + 1)) ]]' || { echo "  w=$W3"; vbox; }
g=($(vbox))
check "超长行：浮窗在 pane 里面（左边 ≥ pane 左边、右边 ≤ pane 右边）" eval '[[ ${g[0]} -ge $P0 && ${g[1]} -le $P1 ]]' || { vbox; echo "pane $P0 $P1"; }
key Escape; sleep 0.3
cell 2 4; key e; sleep 0.7
W4=$(vw); g=($(vbox))
check "右边的列里 110 个字的长行：撑到 ≥ 114 宽，右边出界就整体左移、贴 pane 右边（右边 = pane 右边 ${P1}）" eval '[[ $W4 -ge 114 && ${g[1]} -eq $P1 || ${g[1]} -eq $((P1 - 1)) ]]' || { echo "  w=$W4"; vbox; echo "pane $P0 $P1"; }
check "移到左边之后整行还是看得见（110 个 R）" eval '[[ $(e2e_plain | grep -c "R\{110\}") -ge 1 ]]' || e2e_plain | sed -n 5,12p | cut -c1-160
key Escape; sleep 0.3
# 窗口变窄之后的三档仍按 pane 的宽算：把终端缩到 100 列，短文本也是内容区的一半，超长到内容区的宽
e2e_resize 100 40; sleep 0.8
cell 1 3; key e; sleep 0.7
P=$(e2e_plain | python3 -c '
import sys
l = sys.stdin.read().split("\n")[0]; i = l.find("┌─ ①"); print(len(l) - i - 2)')
W5=$(vw)
check "终端缩到 100 列后，超长行的浮窗仍到 pane 内容区的宽（${P}）" eval '[[ $W5 -ge $((P - 1)) && $W5 -le $((P + 1)) ]]' || { echo "  w=$W5"; e2e_plain | sed -n 1,10p; }
e2e_done
