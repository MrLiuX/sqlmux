#!/usr/bin/env bash
# F15.6 ORDER / COLS 框的宽度按内容（specs/m15-pg-databases/task.md F15.6；tech-design §7.8「ORDER」）
# 框宽 = 最长的「列名  类型」行加标记、边框、内边距（最少 30、最多 pane 宽），提示行不再把它撑到整个 pane：放不下折成两行，再放不下丢末尾的项（窄框的情形在 f12.9）。
# t_order（PG 共用库）：最长的类型是 timestamp with time zone，ORDER 和 COLS 一起看。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

# the box under the chip row (screen row 4): "x0 x1" of its top border in display columns
bbox() { e2e_plain | sed -n 4p | python3 -c '
import sys, unicodedata
w = lambda c: 2 if unicodedata.east_asian_width(c) in "WF" else 1
l = sys.stdin.read().rstrip("\n"); i = l.find("┌─"); j = l.find("┐", i)
if i >= 0 and j > i: x0 = sum(map(w, l[:i])); print(x0 + 1, x0 + sum(map(w, l[i:j + 1])))'; }
bw() { local g; g=($(bbox)); echo $((g[1] - g[0] + 1)); }
hint_rows() { e2e_plain | grep -cE "(j/k 移动|SPC 方向|a 移出|A 清空|↵ 只按这一列|/ 搜索)"; }   # the screen rows with any ORDER hint

start; open_table t_order; wait_for 8 settled
PW=$(e2e_plain | sed -n 1p | python3 -c 'import sys; l = sys.stdin.read().rstrip("\n"); print(len(l) - l.find("┌─ ①") - 2)')
key g o; sleep 0.6
OW=$(bw)
check "ORDER：框按内容宽（${OW} 列），不是 pane 宽（${PW}）：比最长的行（timestamp with time zone）宽不到 20 列，也不窄于 30" eval '[[ $OW -ge 30 && $OW -lt $((PW - 40)) && $OW -le 60 ]]' || { echo "  ow=$OW pane=$PW"; e2e_plain | sed -n 3,24p | cut -c30-160; }
check "ORDER：六项提示分成两行，一项不丢（j/k 移动 · SPC 方向 · a 移出 · A 清空 / ↵ 只按这一列 · / 搜索）" eval '[[ $(hint_rows) == 2 ]] && e2e_plain | grep -qF "j/k 移动 · SPC 方向 · a 移出 · A 清空" && e2e_plain | grep -qF "↵ 只按这一列 · / 搜索"' || e2e_plain | sed -n 15,24p | cut -c30-160
check "提示行在框里面：没有哪一行超出框的右边（框的右边那一列是 │）" eval 'g=($(bbox)); [[ ${g[1]} -le $((PW + 34)) ]] && e2e_plain | grep -F "A 清空" | grep -q "│"'
key Escape; sleep 0.4
key g c; sleep 0.6
CW=$(bw)
check "COLS：同一个框的做法，宽按内容（${CW} 列），和 ORDER 只差序号标记那几列（0–4）" eval '[[ $CW -ge 30 && $CW -lt $((PW - 40)) && $OW -ge $CW && $((OW - CW)) -le 4 ]]' || { echo "  cw=$CW ow=$OW"; e2e_plain | sed -n 3,20p | cut -c30-160; }
check "COLS 的提示行 J/K 移动 · a 全选 · A 全不选 在一行里（放得下，不折）" eval 'e2e_plain | grep -qF "J/K 移动 · a 全选 · A 全不选" && [[ $(e2e_plain | grep -c "A 全不选") == 1 ]]'
key Escape; sleep 0.4
e2e_done
