#!/usr/bin/env bash
# F5.2 多 window（specs/m5-workspace/task.md F5.2；tech-design §5、§6.8、§7.8「状态栏」「工作区节点」、§10.5、§12）
# 状态栏 window 列表的画法由 golden 覆盖（其余 window 的样式、窄宽度的省略补回在 f0.5，%report ↵ 在 f0.14）；
# 这里测真实终端里的新建、切换（SPC n / p / l、点状态栏、面板、树的工作区）、重命名（状态栏原地输入）、关闭（确认、写盘、
# 切到哪个、最后一个不关），以及各 window 的布局和结果区互不影响。只读共用库，不写。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

wins() { bar | sed -E 's/^ [^▾]*▾  //; s/ {3,}.*//'; }                      # 状态栏的 window 列表：0: data*  1: data
wins_are() { [[ $(wins) == "$1" ]] || { echo "  windows: '$(wins)', want '$1'"; false; }; }
panes_are() { local got; got=$(e2e_panes | awk '$1 != "-" { print $1 }' | tr '\n' ' '); [[ $got == "$1" ]] || { echo "  panes: '$got', want '$1'"; false; }; }
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.4; }
cmd() { pal ">$1"; key Enter; }                                             # 在面板里执行命令（window.rename 等不绑默认键）
title1() { e2e_plain | head -1; }
clr() { local i; for ((i = 0; i < 20; i++)); do e2e_keys BSpace; done; sleep 0.2; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
box() { e2e_plain | grep -oE '[^│]*处修改未保存[^│]*' | head -1 | sed 's/^ *//; s/ *$//'; }   # 确认框里的那句话
# 树的工作区：tsel 是光标行的文字（select 底），tree_to NAME 从最后一行往上把光标移到 NAME 那一行
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=#364a82* ]] && { e2e_text 1 32 "$y"; return; }; done; }
tree_to() { local i; key G; for ((i = 0; i < 40; i++)); do [[ $(tsel) == *" $1 "* ]] && return; key k; done; }
trow() { local y; for y in $(seq 3 43); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }   # 侧栏里 NAME 的第一行

# ---- SPC c 新建：默认布局，放在最后并切过去
start; open_table t_order
leader c
check "SPC c：新 window 叫 data，放在最后并切过去" wins_are "0: data  1: data*"
check "新 window 是默认布局：侧栏、① 引导页、② console_2（取最小没用过的 n），焦点在 ①" eval 'panes_are "0 1 2 " && tabs_are 2 "1:console_2*" && e2e_plain | grep -q "打开表" && focus_is 1'

# ---- 各 window 的布局、结果区互不影响
leader %
check "（准备）window 1 里分割 ①：侧栏和三个 pane" panes_are "0 1 2 3 "
leader p
check "SPC p：回到 window 0，它的布局原样：① t_order、② console_1，没有结果区" eval 'wins_are "0: data*  1: data" && panes_are "0 1 2 " && [[ $(title1) == *" t_order ─"* ]] && tabs_are 2 "1:console_1*"'
key C-l; setsql "select 1 as x"; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.3
check "window 0 的 console 执行：结果区 ③ 出现在 window 0 底部" eval 'panes_are "0 1 2 3 " && g=($(geom 3)) && (( g[1] > 20 && g[2] > 120 ))'
leader n
check "SPC n：window 1 原样（引导页、新分出来的 pane、console_2），没有 window 0 的结果区" eval 'wins_are "0: data  1: data*" && panes_are "0 1 2 3 " && tabs_are 3 "1:console_2*" && ! e2e_plain | grep -q "select 1 as x"'

# ---- SPC n / SPC p 到头绕回，SPC l 回到上一个用过的
leader c
check "（准备）第三个 window" wins_are "0: data  1: data  2: data*"
leader n
check "SPC n 在最后一个上：绕回 window 0" wins_are "0: data*  1: data  2: data"
leader p
check "SPC p 在第一个上：绕回 window 2" wins_are "0: data  1: data  2: data*"
leader l
check "SPC l：回到上一个用过的 window 0" wins_are "0: data*  1: data  2: data"
leader l
check "再按 SPC l：又回到 window 2" wins_are "0: data  1: data  2: data*"

# ---- 点状态栏的 window 列表
e2e_click "$(( $(e2e_find "1: data" "$(H)" | cut -d' ' -f1) + 2 ))" "$(H)"; sleep 0.4
check "单击状态栏上的 1: data：切到 window 1" wins_are "0: data  1: data*  2: data"

# ---- 重命名：状态栏上当前 window 那一段原地变成输入框
cmd window.rename
check "window.rename：当前 window 那一段变成输入框（里面是原来的名字），模式 INSERT" eval 'mode_is INSERT && [[ $(bar) =~ ^\ doraemon\ ▾\ \ 0:\ data\ \ 1:\ data\ +2:\ data\  ]] && [[ $(( $(e2e_flag cursor_y) + 1 )) == $(H) ]]'
e2e_type 'j:x'; sleep 0.2
check "没绑定的键当文字：j、:、x 都进了输入框，没有打开面板" eval '[[ $(bar) =~ ^\ doraemon\ ▾\ \ 0:\ data\ \ 1:\ dataj:x\ +2:\ data\  ]] && closed'
key Escape
check "esc：放弃，名字不变，回到 NORMAL" eval 'wins_are "0: data  1: data*  2: data" && mode_is NORMAL'
cmd window.rename; clr; key Enter
check "清空后 ↵：等于放弃" wins_are "0: data  1: data*  2: data"
cmd window.rename; clr; e2e_type report; key Enter
check "改成 report ↵：状态栏、面板、树的工作区都跟着变" eval 'wins_are "0: data  1: report*  2: data" && mode_is NORMAL && [[ -n $(trow report) ]]'

# ---- 面板：window 项 ↵ 切过去；别的 window 的 tab 项 ↵ 先切 window、再选 tab、聚焦它的 pane
leader p; key C-h; open_table t_user; key C-l                               # window 0：① t_order、t_user，焦点在 ② console_1
pal "%report"
check "（准备）在 window 0 里搜 %report：window 项排第一" eval '[[ $(selected) == "1: report  doraemon  窗口" ]] || { echo "  selected: $(selected)"; false; }'
key Enter
check "window 项 ↵：切到 report" eval 'closed && wins_are "0: data  1: report*  2: data"'
pal "%t_order"; key Enter
check "window 0 的 t_order tab 项 ↵：切到 window 0，① 选中 t_order，焦点到 ①" eval 'wins_are "0: data*  1: report  2: data" && [[ $(title1) == *" t_order ─"* ]] && tabs_are 1 "1:t_order* │ 2:t_user-" && focus_is 1'

# ---- 树的工作区：window 节点 ↵ / 单击切过去，tab 节点 ↵ 先切 window
y=$(trow report); e2e_click "$(e2e_find report "$y" | cut -d' ' -f1)" "$y"; sleep 0.4
check "单击树里的 window 节点 report：切到 report" wins_are "0: data  1: report*  2: data"
key C-h; key C-h; key C-h; tree_to data
check "（准备）report 里焦点到树上，光标在最后一个 window（2: data）上" eval 'focus_is 0 && [[ $(tsel) == *" data "* ]]'
key Enter
check "window 节点上 ↵：切到 window 2" wins_are "0: data  1: report  2: data*"
key C-h; key C-h; key C-h; tree_to console_2; key Enter
check "report 的 console_2 tab 节点 ↵：先切到 report，再聚焦 console_2 所在的 ③" eval 'wins_are "0: data  1: report*  2: data" && focus_is 3'

# ---- 树的展开状态按 window 的稳定 ID 记：关掉前面的 window 之后，折叠的还是原来那个（不按下标错到后一个上）
key C-h; key C-h; key C-h; tree_to report; key h
check "（准备）树里折叠 report：它下面的 pane 不显示" eval '[[ $(e2e_text 1 32 "$(trow report)") == *"▸"* ]]'
leader p; cmd window.close
check "关掉排在前面的 window 0（切到上一个用过的 report）：report 还是折叠的，原来的 window 2 还是展开的" eval 'wins_are "0: report*  1: data" && [[ $(e2e_text 1 32 "$(trow report)") == *"▸"* && $(e2e_text 1 32 "$(trow data)") == *"▾"* ]]'

# ---- 关闭：先写盘，有修改先确认；切到上一个用过的，没有就切到前面那个（第一个就后面那个）；最后一个不关
con2() { tabbar 2 | sed 's/^1://; s/\*$//'; }                                # ② 的 console tab 名，用来认出是哪个 window
leader n; key C-l; key C-l; key i; e2e_type "select 7"; key Escape; cf=$(con2)   # 原来的 window 2 的 console 写一句
cmd window.close
check "window.close：它的 console 写了盘，切到上一个用过的 report" eval 'wins_are "0: report*" && grep -q "select 7" "$E2E_TMP/data/sqlmux/consoles/$cf.sql"'
leader l
check "上一个用过的已经关了：SPC l 不做事" wins_are "0: report*"
leader c; leader c; c2=$(con2); leader c                                     # report、1、2、3，2 的 console 记作 c2
leader p; leader p; e2e_click $(( $(e2e_find "3: data" "$(H)" | cut -d' ' -f1) + 2 )) "$(H)"; sleep 0.4
check "（准备）从 1 点状态栏到 3" wins_are "0: report  1: data  2: data  3: data*"
cmd window.close
check "关掉 3：回到上一个用过的 1（不是它前面的 2）" wins_are "0: report  1: data*  2: data"
cmd window.close
check "再关 1（上一个用过的已经没了）：切到它前面的 report" wins_are "0: report*  1: data"
cmd window.close
check "再关排第一的 report：切到它后面那个（原来的 2）" eval 'wins_are "0: data*" && [[ $(con2) == "$c2" ]]'
leader c; open_table t_user; key l; key Enter; e2e_type x; key Enter              # 新 window 里 t_user 改一格
leader p; e2e_type ':'; sleep 0.3; e2e_type qa; key Enter
check "退出时统计所有 window 的修改：「有 1 处修改未保存，退出会丢弃。」" eval '[[ $(box) == "有 1 处修改未保存，退出会丢弃。" ]] || { echo "  $(box)"; false; }'
key n; leader n; cmd window.close
check "关掉有修改的 window：确认「有 1 处修改未保存，关闭会丢弃。」y 关闭" eval '[[ $(box) == "有 1 处修改未保存，关闭会丢弃。" ]] && e2e_plain | grep -q "y 关闭   n 取消" || { echo "  $(box)"; false; }'
key n
check "n：取消，window 还在" eval '[[ -z $(box) ]] && wins_are "0: data  1: data*"'
cmd window.close; key y
check "再关、y：关掉，切到上一个用过的" wins_are "0: data*"
cmd window.close
check "只剩一个 window：window.close 不做事" eval 'wins_are "0: data*" && running'

e2e_done
