#!/usr/bin/env bash
# F10.1 WHERE 历史下拉撑满（specs/m10-feedback3/task.md F10.1；tech-design §9.6「历史 / 收藏」）
# 历史比 8 条多时，下拉撑到 data pane 内容区的底边（不是 8 行）；pane 在窗口下半边、下面放不下时翻到输入框上方，在输入框上一行结束。
# 画法由 golden 覆盖；这里在真实终端里数行数、看位置。只读，用共用库的 t_order（WHERE 只是过滤）。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }       # 下拉框：X Y W H
where() { key /; clear_in; e2e_type "$1"; sleep 0.2; key Enter; wait_for 8 settled; sleep 0.2; }
many() { local i; for ((i = 1; i <= $1; i++)); do where "id = $i"; done; }     # 历史里放 N 条

start; open_table t_order; wait_for 8 settled
many 14
key /; key Escape; key 0; key /; sleep 0.5                                   # 空 WHERE 输入框 → 历史下拉
g=($(pop)); p=($(geom 1))
check "上面的大 pane（历史 14 条）：下拉打开，比 8 行高（撑到条目数，不是 8）" eval '[[ -n ${g[*]} ]] && (( g[3] >= 14 + 2 )) || { echo "  dropdown [${g[*]}]"; false; }'
check "下拉在输入框下面起：框顶在 WHERE 行的下一行" eval 'ys=$(( p[1] + 1 )); (( g[1] == ys + 1 )) || { echo "  box y=${g[1]} want $(( ys + 1 )) (pane ${p[*]})"; false; }'
check "最后一行不超出 pane 的内容区：框底在 pane 底边框上方" eval '(( g[1] + g[3] - 1 < p[1] + p[3] - 1 )) || { echo "  box [${g[*]}] pane [${p[*]}]"; false; }'
key Escape; key Escape; key Escape; sleep 0.3                              # 关下拉 → 输入框 NORMAL → 回表格

# ---- 补全列表仍是 8 行
key /; clear_in; e2e_type "a"; sleep 0.6
g=($(pop))
check "WHERE 里打 a（历史里没有以 a 开头的）：补全列表（列名、关键字）最多 8 行（加框 10），不撑满" eval '[[ -n ${g[*]} ]] && (( g[3] <= 8 + 2 )) || { echo "  completion [${g[*]}]"; false; }'
key Escape; key Escape; key Escape; sleep 0.3

# ---- pane 在窗口下半边：下面放不下时翻到输入框上方
many 40                                                                       # 历史最多 100 条，这里 40 条（+前面 14 条里重复的 id = 1..14 合并）
e2e_keys Space; e2e_type '"'; sleep 0.6                                       # ① 下面分出 ②，焦点在 ②（空 pane）
open_table t_order; wait_for 8 settled
many 3
g=($(geom 2)); y0=${g[1]}
key /; key Escape; key 0; key /; sleep 0.5
b=($(pop))
check "下面的 pane ②：历史下拉打开" eval '[[ -n ${b[*]} ]]'
check "下面放不下，翻到输入框上方：框底在 WHERE 输入框的上一行，没有盖住输入框" eval 'iy=$(( y0 + 1 )); (( b[1] + b[3] - 1 == iy - 1 )) || { echo "  box [${b[*]}] input row $iy (pane ②: ${g[*]})"; false; }'
check "输入框那一行还是 WHERE 的输入，没被盖住" eval 'text_has 36 40 $(( y0 + 1 )) WHERE'
e2e_done
