#!/usr/bin/env bash
# F15.5b 值编辑器打字、删行时宽度跟着内容走（specs/m15-pg-databases/task.md F15.5；审查提测的验收要点）
# 打字时变宽；在最长一行末尾 A 打字再 esc，文字不横向滚出去；右边的列里打到出界框整体左移、贴 pane 右边；dd 删掉最长行框变窄、u 撤回又变宽。
# SQLite 文件：a 列短文本，b 列右边的列短文本。
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
read -r P0 P1 <<<"$PANE"; PW=$((P1 - P0 + 1 - 2))   # the content width, the frame off
echo "  pane x $P0..$P1, content $PW wide"

K60=$(printf 'k%.0s' $(seq 1 60)); N100=$(printf 'n%.0s' $(seq 1 100))
cell 1 1; key e; sleep 0.7
W0=$(vw)
key A; e2e_type "$K60"; sleep 0.6
W1=$(vw)
check "A 打了 60 个字：框随之变宽（比开始宽，装得下 5 + 60 个字）" eval '[[ $W1 -gt $W0 && $W1 -ge 69 ]]' || { echo "  $W0 -> $W1"; e2e_plain | sed -n 1,10p | cut -c1-150; }
key Escape; sleep 0.5
check "esc 之后文字没有横向滚出去：开头的 short 和 60 个 k 都看得见，宽度没变" eval 'e2e_plain | grep -q "short$K60" && [[ $(vw) -eq $W1 ]]' || { echo "  w=$(vw)"; e2e_plain | sed -n 1,10p | cut -c1-150; }
key o; e2e_type "$N100"; key Escape; sleep 0.5
W2=$(vw)
check "o 加一行 100 个 n：框撑到更宽（≥ 104），两行都看得见" eval '[[ $W2 -gt $W1 && $W2 -ge 104 ]] && e2e_plain | grep -q "$N100" && e2e_plain | grep -q "short$K60"' || { echo "  $W1 -> $W2"; e2e_plain | sed -n 1,10p | cut -c1-150; }
key d d; sleep 0.5
check "dd 删掉最长那一行：框变窄（回到装得下 5 + 60 个字）" eval '[[ $(vw) -lt $W2 && $(vw) -eq $W1 ]]' || echo "  $W2 -> $(vw)"
key u; sleep 0.5
check "u 撤回：框又变宽" eval '[[ $(vw) -eq $W2 ]] && e2e_plain | grep -q "$N100"' || echo "  w=$(vw)"
e2e_type ":"; sleep 0.3; e2e_type q; key Enter; sleep 0.6; key y; sleep 0.6
check "（还原）:q 问「放弃修改？」，y：值编辑器关了" eval '! e2e_plain | grep -qF " · TEXT"' || e2e_plain | tail -4

# ---- 右边的列：打到出界，框整体左移、贴 pane 右边
cell 2 1; key e; sleep 0.7
key A; e2e_type "$K60$K60"; sleep 0.6
g=($(vbox))
check "b 列（右边）打了 120 个字：框撑宽，整体左移，右边不出 pane（≤ ${P1}），光标那一行还看得见" eval '[[ -n ${g[0]} && ${g[1]} -le $P1 && ${g[0]} -ge $P0 ]] && e2e_plain | grep -qF "${K60}"' || { echo "  box ${g[*]} pane $P0 $P1"; e2e_plain | sed -n 1,10p | cut -c1-170; }
key Escape; sleep 0.5; g=($(vbox))
check "esc 之后右边还是不出 pane，宽度不变" eval '[[ ${g[1]} -le $P1 ]] && e2e_plain | grep -qF "${K60}"' || { echo "  box ${g[*]}"; e2e_plain | sed -n 1,10p | cut -c1-170; }
e2e_done
