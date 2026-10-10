#!/usr/bin/env bash
# F5.19 命令面板的「窗口 &」和「Tab #」范围（specs/m5-workspace/task.md F5.19；tech-design §12「窗口 & 和 Tab #」）
# keymaps.toml 里 <Leader>k 绑 palette.tab、<Leader>j 绑 palette.window。顺序的细节有单测；这里在真实终端里测：
# 两个 tab 来回切（SPC k ↵ 回到上一个）、列表的内容和顺序、窗口范围只有 window、别的 window 里的 tab 也列、↵ 跳过去、前缀过滤。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
printf '[keys.global]\n"<Leader>k" = "palette.tab"\n"<Leader>j" = "palette.window"\n' >"$D/keymaps.toml"
wins() { bar | sed -E 's/^ [^▾]*▾  //; s/ {3,}.*//'; }
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.5; }
names() { list | cut -d'|' -f1 | sed 's/  .*//' | tr '\n' ',' ; }
sel() { selected | sed 's/  .*//'; }

start -K "$D/keymaps.toml"; two_tabs
check "（准备）① 里 t_user、t_order（当前），② query_1" eval 'tabs_are 1 "1:t_user- │ 2:t_order*"'
leader k
check "SPC k：面板打开，输入框里是 #，范围「Tab」" eval 'is_open && input_is "#"'
check "Tab 列表：上一个 t_user 排第一（选中），没聚焦过的 query_1 在中间，当前 t_order 排最后" eval '[[ $(names) == "t_user,query_1,t_order," && $(sel) == t_user ]] || { echo "  $(names) sel=$(sel)"; false; }'
check "每行有位置（连接 › window › pane）和类型标签「Tab」" eval 'list | head -1 | grep -q "^t_user  doraemon › 0: data › pane-1  Tab|1$"'
key Enter
check "↵：回到上一个 tab（t_user）" eval 'closed && tabs_are 1 "1:t_user* │ 2:t_order-" || tabs_are 1 "1:t_user*"'
leader k; key Enter
check "再 SPC k ↵：又回到 t_order（来回切）" eval 'closed && tabs_are 1 "1:t_user- │ 2:t_order*"'

# ---- 前缀过滤、Tab / S-Tab 范围
leader k; e2e_type "ord"; sleep 0.4
check "#ord：fzf 分数优先，t_order 排第一并选中（t_user、query_1 靠位置里的 d o r 字符也能匹配，排在后面）" eval '[[ $(sel) == t_order && $(names) == t_order,* ]] || { echo "  $(names)"; false; }'
key Escape
leader k; key BTab; sleep 0.3
check "S-Tab：#（Tab）→ 窗口 &" input_is "&"
key Tab; key Tab; sleep 0.3
check "Tab 两次：回到 # 再到 @（表）" input_is "@"
key Escape

# ---- 窗口范围：只列 window，按编号，显示成 0: data
leader j
check "SPC j：输入框里是 &，只有 window 0（「0: data」，类型「窗口」，选中）" eval 'input_is "&" && [[ $(list | cut -d"|" -f1 | sed "s/  */ /g") == "0: data doraemon 窗口" && $(sel) == "0: data" ]] || { list; false; }'
key Escape
e2e_keys Space; e2e_type c; sleep 0.8; e2e_keys Space; e2e_type %; sleep 0.4; key c; key C-h   # F8.18: the new window is one empty pane; split it and c a query_2 as before
leader j
check "新建 window 1 之后：&  列出 0: data 和 1: data-2，按编号" eval '[[ $(names) == "0: data,1: data-2," ]]'
key Up; key Enter; sleep 0.4
check "窗口范围里选 0: data ↵：切到 window 0" eval 'closed && [[ $(wins) == "0: data*  1: data-2" ]]'

# ---- 别的 window 里的 tab 也列；引导页不列；query、日志、结果 tab 都列
leader k
check "Tab 列表有 window 1 里的 query_2（位置 1: data-2），没有引导页；window 0 的 t_user、t_order、query_1 都在" eval 'n=$(names); [[ $n == *query_2* && $n == *t_user* && $n == *t_order* && $n == *query_1* ]] && list | grep -q "^query_2  doraemon › 1: data-2 › pane-2  Tab|" && ! list | grep -qi "引导\|landing\|新 tab" || { echo "  $(names)"; list; false; }'
key Escape
key C-l; key i; e2e_type "select 1"; key Escape; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.8
leader k
check "执行之后：日志 tab 和结果 tab（query_1 #1）也在列表里" eval 'n=$(names); [[ $n == *日志* && $n == *"query_1 #1"* ]] || { echo "  $n"; false; }'
key Escape

# ---- 审查补的：窗口范围不按「最近用过的」重排；焦点在侧栏时不记聚焦；两个命令有标题、能在面板里搜到
e2e_stop; start -K "$D/keymaps.toml"; two_tabs
e2e_keys Space; e2e_type c; sleep 0.8; e2e_keys Space; e2e_type p; sleep 0.5
leader j; key Down; key Enter; sleep 0.5
leader j
check "刚在窗口范围里选过 window 1：再打开，顺序仍是 0: data、1: data-2-2（不按最近用过的重排）" eval '[[ $(names) == "0: data,1: data-2," ]]'
key Escape
key C-h; sleep 0.3
leader k
check "焦点在侧栏时不记聚焦：最后聚焦的那个 tab 仍算当前，排最后；打开就是 ↵ 回到上一个" eval 'n=$(names); [[ $n == *, ]] && last=${n%,}; last=${last##*,}; [[ -n $last ]] && ! [[ $(sel) == "$last" ]] || { echo "  $(names) sel=$(sel)"; false; }'
key Escape
e2e_keys C-p; sleep 0.3; e2e_type ">palette"; sleep 0.6
check "命令面板里搜得到 palette.window、palette.tab，有标题" eval 'list | grep -q "palette.window" && list | grep -q "palette.tab" && ! list | grep "palette.window\|palette.tab" | grep -q "^palette\."'
key Escape

e2e_done
