#!/usr/bin/env bash
# F13.7 模态浮层开着时滚轮不滚底下的表 + F13.11 单行输入框的 C-h 是退格（specs/m13-quality/task.md F13.7 / F13.11）
# COLS、命令面板、session 列表开着时滚轮滚的不是底下的表（以前会滚）；WHERE 输入框、命令面板的搜索框里 C-h 删一个字符。只读，用共用库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

first_id() { e2e_text 35 159 $(( $(grid_y) + 1 )) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }

start; open_table t_order; wait_for 8 settled
check "（准备）t_order 第一行 id 1，光标 1,1" eval '[[ $(first_id) == 1 ]] && pos_is 1,1'
# 滚轮在表上：确实会滚（对照，说明下面的检查不是空检查）
e2e_wheel 80 20 down; e2e_wheel 80 20 down; e2e_wheel 80 20 down; sleep 0.5
check "（对照）没有浮层时滚轮滚表：第一行不再是 id 1" eval '[[ $(first_id) != 1 ]]'
key g g; sleep 0.3
check "（准备）gg 回到顶部" eval '[[ $(first_id) == 1 ]]'

key g c; sleep 0.5
for i in 1 2 3 4 5 6; do e2e_wheel 80 30 down; sleep 0.1; done; sleep 0.4
check "COLS 浮层开着时滚轮：底下的表没滚（第一行还是 id 1，光标没动）" eval '[[ $(first_id) == 1 ]] && pos_is 1,1'
key Escape; sleep 0.4

e2e_keys C-p; sleep 0.5
for i in 1 2 3 4 5 6; do e2e_wheel 80 38 down; sleep 0.1; done; sleep 0.4
key Escape; key Escape; sleep 0.4
check "命令面板开着时滚轮：底下的表没滚" eval '[[ $(first_id) == 1 ]] && pos_is 1,1'

e2e_keys Space; sleep 0.3; e2e_type s; sleep 0.6
for i in 1 2 3 4 5 6; do e2e_wheel 80 38 down; sleep 0.1; done; sleep 0.4
key Escape; sleep 0.4
check "session 列表开着时滚轮：底下的表没滚" eval '[[ $(first_id) == 1 ]] && pos_is 1,1'

# ---- F13.11：C-h 是退格
key /; sleep 0.3; e2e_type "abc"; e2e_keys C-h; sleep 0.3
check "WHERE 输入框里打 abc，C-h：变成 ab" eval '[[ $(where_in) == ab ]] || { echo "  [$(where_in)]"; false; }'
key Escape; key Escape; sleep 0.3
e2e_keys C-p; sleep 0.4; e2e_type "@t_us"; sleep 0.3; e2e_keys C-h; sleep 0.5
check "命令面板的搜索框里 @t_us，C-h：删掉一个字符（@t_u）" eval 'e2e_plain | grep -qE "@t_u( |$)" && ! e2e_plain | grep -q "@t_us"'
e2e_done
