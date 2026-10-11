#!/usr/bin/env bash
# F13.11 C-h is a backspace in the single-line inputs (specs/m13-quality/task.md F13.11; tech-design §7.9): the palette, the sidebar filter,
# the COLS filter and WHERE. (The cell editor's is in f3.21.) Each types a word with one letter too many, C-h, and reads what is left.
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
trap 'e2e_stop' EXIT
SOLO=1 start; open_table t_order; wait_for 8 settled

pal "abcX"; key C-h; sleep 0.3
check "命令面板：abcX 之后 C-h 删掉一个字，剩 abc，面板还开着" eval 'input_is abc && is_open'
key Escape; sleep 0.3

key /; sleep 0.3; e2e_type "t_orderX"; sleep 0.3
check "（准备）WHERE 里是 t_orderX" eval '[[ $(where_in) == t_orderX ]]'
key C-h; sleep 0.3
check "WHERE：C-h 删掉一个字，剩 t_order（焦点没换走）" eval '[[ $(where_in) == t_order ]]'
key Escape; key Escape; sleep 0.3

key g c; sleep 0.4; key /; e2e_type "amountX"; sleep 0.3
check "（准备）COLS 过滤 amountX：没有匹配，0/10" eval 'e2e_plain | grep -qE " 0/10 │"'
key C-h; sleep 0.3
check "COLS 过滤：C-h 删掉 X，amount 一列匹配，1/10" eval 'e2e_plain | grep -qE " 1/10 │"'
key Escape; sleep 0.3; key Escape; sleep 0.3; key Escape; sleep 0.3   # 离开输入、清掉过滤、关浮层
check "（准备）COLS 浮层关了" eval '! screen_has "a 全选"'

key C-h; sleep 0.3; key /; sleep 0.3; e2e_type "t_orderX"; sleep 0.3
check "（准备）侧栏过滤 t_orderX：没有匹配 0/11" eval 'e2e_plain | grep -qE " 0/11 "'
key C-h; sleep 0.3
check "侧栏过滤：C-h 删掉 X，t_order 和 t_order_item 匹配，2/11" eval 'e2e_plain | grep -qE " 2/11 "'
e2e_done
