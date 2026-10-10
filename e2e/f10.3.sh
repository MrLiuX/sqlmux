#!/usr/bin/env bash
# F10.3 COLS 里调列顺序（specs/m10-feedback3/task.md F10.3；tech-design §7.8「COLS」、§14 order_cols）
# gc 打开 COLS：J / K 把当前列下移 / 上移一位，表格的列顺序立刻跟着变，光标仍在这一列上；过滤框非空时不动；yi、Y 按显示顺序；
# 顺序按 tab 记，换到别的 tab 再回来、退出重启后还在。画法由 golden 覆盖。只读，用共用库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/c" "$D/data" "$D/state"
SELECT=#364a82
header() { e2e_text 35 159 "$(hy)" | python3 -c "import re, sys; print(' '.join(re.findall(r'[a-z_]+', sys.stdin.read())[:${N:-3}]))"; }   # 表头的前 N（默认 3）个列名，空格隔开；COLS 浮层会盖住右边的列
dd() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }'; }
dd_rows() { local g y; g=($(dd)); for ((y = g[1] + 3; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/ *$//'; done; }
dd_names() { dd_rows | grep '^\[' | sed 's/^\[.\] //; s/ .*//' | tr '\n' ' '; }
dd_cur() { local g y; g=($(dd)); for ((y = g[1] + 3; y < g[1] + g[3] - 1; y++)); do style_has $((g[0] + 3)) $y bg=$SELECT >/dev/null && { e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/^\[.\] //; s/ .*//'; return; }; done; }
ORIG="id user_id status amount paid meta raw note created_at deleted_at"   # 列表里的 10 列
VIS=9                                                                       # 160 列下表格只放得下前 9 列
ORIG9=${ORIG% deleted_at}

start; open_table t_order; wait_for 8 settled
check "（准备）表头是表里的顺序" eval '[[ $(N=$VIS header) == "$ORIG9" ]] || { echo "  [$(N=$VIS header)] want [$ORIG9]"; false; }'
key gc
check "gc：列表里是同一个顺序，光标在 id" eval '[[ $(dd_names) == "$ORIG " && $(dd_cur) == id ]]'
key K
check "第一列上 K：没处可去，顺序不变" eval '[[ $(header) == "id user_id status" && $(dd_cur) == id ]]'
key j; key j; key K
check "光标在 status：K 上移一位，表格列顺序 id status user_id …，列表同步，光标仍在 status" eval '[[ $(header) == "id status user_id" && $(dd_cur) == status && $(dd_names) == "id status user_id amount paid meta raw note created_at deleted_at " ]] || { echo "  [$(header)] cur=$(dd_cur)"; false; }'
key J; key J
check "J J：status 下移两位，id user_id amount status …，光标仍在 status" eval '[[ $(header) == "id user_id amount" && $(dd_cur) == status ]] || { echo "  [$(header)] cur=$(dd_cur)"; false; }'
for i in $(seq 9); do key j; done; key J   # cols 列表里没有 G：j 到底（到底后 j 不动）
check "最后一列上 J：没处可去，顺序不变" eval '[[ $(header) == "id user_id amount" && $(dd_names) == "id user_id amount status paid meta raw note created_at deleted_at " ]]'
for i in $(seq 9); do key k; done; key K
check "（回到第一列）K 无事" eval '[[ $(header) == "id user_id amount" ]]'

# ---- 过滤框非空时 J / K 不动
key /; e2e_type "stat"; sleep 0.3; key Enter; key J
check "过滤 stat 后（焦点回列表）J：顺序不变" eval '[[ $(header) == "id user_id amount" ]]'
key K
check "过滤框非空时 K 也不动" eval '[[ $(header) == "id user_id amount" ]]'
key Escape; key Escape

# ---- 隐藏与顺序一起：隐藏 amount，光标挪到最近的可见列；其余顺序不变
key gc; key j; key j; key Space; key Escape
check "隐藏 amount：表头少一列，其余顺序不变" eval '[[ $(N=8 header) == "id user_id status paid meta raw note created_at" ]]'
key gc; key a; key Escape

# ---- yi / Y 按显示顺序
key g g; key 0; key j; key y; key i; sleep 0.6
check "yi：INSERT 的列清单按显示顺序（id, user_id, amount, status, …）" eval 'clip | grep -q "^insert into \"public\".\"t_order\" (\"id\", \"user_id\", \"amount\", \"status\", \"paid\""' || clip
key Y; sleep 0.6
check "Y：JSON 的键按显示顺序（id, user_id, amount, status, …）" eval 'clip | python3 -c "import json, sys; k = list(json.load(sys.stdin)); sys.exit(0 if k[:4] == [\"id\", \"user_id\", \"amount\", \"status\"] and len(k) == 10 else 1)"' || clip
key T; sleep 0.8
labels() { local y; for y in $(seq 6 15); do e2e_text 35 46 "$y" | python3 -c "import sys; import re; print(re.sub(r'[^a-z_]', '', sys.stdin.read()))"; done | tr '\n' ' ' | sed 's/^ *//; s/ *$//'; }   # 转置视图左边一列的字段名
check "转置视图也按显示顺序：左边一列是 id、user_id、amount、status、paid …" eval '[[ $(labels) == "id user_id amount status paid meta raw note created_at deleted_at" ]] || { echo "  [$(labels)]"; false; }'
key T

# ---- 顺序按 tab 记：换到别的 tab 再回来还在
check "（准备）t_order 当前顺序仍是 id user_id amount status …" eval '[[ $(N=$VIS header) == "id user_id amount status paid meta raw note created_at" ]]'
open_table t_user; wait_for 8 settled
check "t_user 有自己的顺序（不受影响）" eval '[[ $(header) == id* && $(header) != *"id user_id amount"* ]]'
key g T; sleep 0.5
check "回到 t_order tab：顺序还在" eval '[[ $(header) == "id user_id amount" ]] || { echo "  [$(N=$VIS header)] want [$ORIG9]"; false; }'
e2e_done
