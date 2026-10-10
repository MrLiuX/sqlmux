#!/usr/bin/env bash
# F10.2 新建 session 先要名字（specs/m10-feedback3/task.md F10.2；tech-design §7.8「新建 session」）
# session.new / session 列表里的 n：状态栏单行输入「session 名：<预填的最小未用数字>」；↵ 用输入的名字（trim 后为空就用预填的），
# 重名 toast「已有同名 session」输入框留着，esc 什么都不建。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

sname() { bar | sed 's/^ //; s/ ▾.*//'; }
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.5; }
cmd() { pal ">$1"; key Enter; }
clr() { local i; for ((i = 0; i < 30; i++)); do e2e_keys BSpace; done; sleep 0.2; }
toast_text() { e2e_text 1 160 44; }
nsess() { leader s; local n; n=$(e2e_plain | grep -cE "[0-9]+ windows"); key Escape; sleep 0.3; echo "$n"; }   # session 列表里有几行

start
check "（准备）一个 session doraemon" eval '[[ $(sname) == doraemon ]]'

# ---- session 列表里的 n：先要名字，预填 1
leader s; key n; sleep 0.5
check "n：状态栏变成输入框「session 名：1」（预填最小的没用过的数字），INSERT，列表关了，还没建出 session" eval '[[ $(bar) == *"session 名：1"* ]] && mode_is INSERT && [[ $(sname) != 1 ]]'
clr; e2e_type "ops"; key Enter
check "输入 ops ↵：建出名为 ops 的 session 并 attach 过去（状态栏是 ops），回到 NORMAL" eval 'wait_for 10 eval "[[ \$(sname) == ops ]]" && mode_is NORMAL'
check "（建出来的是一个 window、一个空 pane，没有多开别的）" eval '[[ $(bar) == " ops ▾  0: data*"* ]]'

# ---- 空输入 ↵：用预填的数字名
cmd session.new; sleep 0.5
check "session.new：预填仍是 1（doraemon、ops 都不是数字名）" eval '[[ $(bar) == *"session 名：1"* ]] && mode_is INSERT'
clr; key Enter
check "删空再 ↵：用预填的 1 建出 session 1" eval 'wait_for 10 eval "[[ \$(sname) == 1 ]]"'

# ---- 重名：toast，输入框留着
cmd session.new; sleep 0.5
check "再 session.new：预填 2" eval '[[ $(bar) == *"session 名：2"* ]]'
clr; e2e_type "ops"; key Enter; sleep 0.5
check "输入已有的 ops ↵：toast「已有同名 session」，输入框还在（INSERT、文字是 ops），没有切走" eval 'toast_text | grep -q "已有同名 session" && [[ $(bar) == *"session 名：ops"* ]] && mode_is INSERT && [[ $(sname) != ops ]]'
clr; e2e_type "doraemon"; key Enter; sleep 0.5
check "改成 doraemon（第一个 session 的名字）也被拒绝" eval 'toast_text | grep -q "已有同名 session" && mode_is INSERT'

# ---- esc：什么都不建
key Escape; sleep 0.4
check "esc：放弃，还是 session 1，模式回 NORMAL" eval '[[ $(sname) == 1 ]] && mode_is NORMAL'
check "session 列表里还是三个 session（doraemon、ops、1），没有多出来的" eval '[[ $(nsess) == 3 ]]'
leader s; key n; sleep 0.5; clr; e2e_type "never"; key Escape; sleep 1.5
check "列表里 n、输入 never、esc：也不建；等 1.5 秒后仍是三个 session，当前还是 1" eval '[[ $(sname) == 1 && $(nsess) == 3 ]]'

# ---- 首尾空白被去掉
cmd session.new; sleep 0.5; clr; e2e_type "  w1  "; key Enter
check "输入「  w1  」↵：名字是 w1（trim）" eval 'wait_for 10 eval "[[ \$(sname) == w1 ]]"'
check "现在四个 session：doraemon、ops、1、w1" eval '[[ $(nsess) == 4 ]]'
e2e_done
