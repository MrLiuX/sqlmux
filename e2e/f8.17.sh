#!/usr/bin/env bash
# F8.17 空前缀的补全列表不预选（specs/m8-feedback/task.md F8.17；tech-design §9.7「空前缀的列表不预选」）
# 前缀为空、自己弹出的列表没有选中项：↵ 是输入框的（换行 / 执行），Tab C-n ↓ 选第一项，S-Tab C-p ↑ 选最后一项；
# 打了字母之后第一项自动选中；接受 where 等关键字补出的空格也立即弹出列名；C-n 手动唤起的、INSERT 的「全部列」仍预选。
# console、WHERE、快速 SQL 三处一致。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }
names_are() { [[ $(names) == "$1" ]] || { echo "  candidates: '$(names)', want '$1'"; false; }; }
none() { [[ -z $(pop) ]] || { echo "  popup: $(names)"; false; }; }
sel() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0   # 选中那一行的候选名，没有选中项时为空
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do style_has $((g[0] + g[2] - 2)) $y bg=$SELECT >/dev/null && e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/  .*//; s/^ *//; s/ *$//'; done; }
nosel() { [[ -z $(sel) ]] || { echo "  selected: '$(sel)' of $(names)"; false; }; }
sel_is() { [[ $(sel) == "$1" ]] || { echo "  selected: '$(sel)', want '$1' of $(names)"; false; }; }
line() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
cc() { echo "$(e2e_flag cursor_y):$(( $(e2e_flag cursor_x) - 103 ))"; }
fresh() { key Escape; key Escape; key g g; key d G; key i; }
settle() { wait_for 8 settled; }
SELECT=#364a82
LOG="logged_at|msg|"      # t_log 的列（按表里的顺序）

start; key C-l; key i

# ---- console：用户的原话 —— 接受 where 补了空格，列名直接弹出，不用再敲一个空格
fresh; e2e_type "select * from t_log wher"; sleep 0.8
check "（准备）wher：弹出列表，第一项（where）已选中（有前缀）" eval 'sel_is where'
key Enter; sleep 0.8
check "↵ 接受 where（补空格）：列名立刻弹出（logged_at、msg），没有「全部列」" names_are "$LOG"
check "前缀为空、自己弹出的：没有选中项" nosel
check "文字是「where 」，光标在空格后" eval '[[ $(line 1) == "select * from t_log where" && $(cc) == 1:26 ]] || { echo "  [$(line 1)] $(cc)"; false; }'
key Enter; sleep 0.4
check "没有选中项时 ↵ 照常换行：列表关掉，光标到第 2 行行首，文字没有多出列名" eval '[[ -z $(pop) && $(line 1) == "select * from t_log where" && $(cc) == 2:0 ]] || { echo "  [$(line 1)] [$(line 2)] $(cc)"; false; }'

fresh; e2e_type "select * from t_log wher"; sleep 0.6; key Enter; sleep 0.8; key Tab; sleep 0.3
check "Tab：选中第一项 logged_at" eval 'sel_is logged_at'
key Enter; sleep 0.4
check "Tab ↵：接受第一列，补一个空格，列表关掉（前一个词是列名，不再弹）" eval '[[ $(line 1) == "select * from t_log where logged_at" && -z $(pop) && $(cc) == 1:36 ]] || { echo "  [$(line 1)] $(cc) $(names)"; false; }'

fresh; e2e_type "select * from t_log wher"; sleep 0.6; key Enter; sleep 0.8; e2e_type l; sleep 0.6
check "打了字母 l：按前缀过滤（logged_at 在前，后面是 l 开头的关键字），第一项自动选中" eval '[[ $(names) == "logged_at|"* && $(names) != "$LOG" ]] && sel_is logged_at'
key Enter; sleep 0.4
check "where↵l↵：得到 logged_at 加一个空格（不换行）" eval '[[ $(line 1) == "select * from t_log where logged_at" && -z $(line 2) && $(cc) == 1:36 ]] || { echo "  [$(line 1)] [$(line 2)] $(cc)"; false; }'

# ---- 往下 / 往上选：没有选中项时 Tab C-n ↓ 选第一项，S-Tab C-p ↑ 选最后一项
for k in Tab C-n Down; do
  fresh; e2e_type "select * from t_log wher"; sleep 0.6; key Enter; sleep 0.8; key $k; sleep 0.3
  check "没有选中项时 $k：选中第一项" eval 'sel_is logged_at'
done
for k in BTab C-p Up; do
  fresh; e2e_type "select * from t_log wher"; sleep 0.6; key Enter; sleep 0.8; key $k; sleep 0.3
  check "没有选中项时 $k（S-Tab 在 tmux 里是 BTab）：选中最后一项 msg" eval 'sel_is msg'
done
key Tab; sleep 0.3
check "选中之后照常循环：最后一项再往下回到第一项" eval 'sel_is logged_at'

# ---- 鼠标：悬停选中（没有选中项时也一样），点击接受
item_y() { local g y; g=($(pop)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do [[ $(e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y) == "$1"* ]] && { echo $y; return; }; done; }
fresh; e2e_type "select * from t_log wher"; sleep 0.6; key Enter; sleep 0.8
g=($(pop)); e2e_move $((g[0] + 3)) "$(item_y msg)"; sleep 0.4
check "没有选中项时鼠标悬停在 msg 上：选中它" eval 'sel_is msg'
e2e_click $((g[0] + 3)) "$(item_y msg)"; sleep 0.5
check "点击：接受 msg（补一个空格），列表关掉" eval '[[ $(line 1) == "select * from t_log where msg" && -z $(pop) ]] || { echo "  [$(line 1)] $(names)"; false; }'

# ---- esc 关掉之后不再自己弹，再打空格才弹
fresh; e2e_type "select * from t_log wher"; sleep 0.6; key Enter; sleep 0.8; key Escape; sleep 0.3
check "esc：列表关掉，回到 NORMAL" eval 'none && mode_is NORMAL'
key A; sleep 0.8
check "A 回 INSERT（光标紧跟空格）：不弹" none
e2e_type " "; sleep 0.8
check "再打一个空格：列名又弹出，仍无选中项" eval 'names_are "$LOG" && nosel'

# ---- 手动唤起的仍预选第一项（C-n 在 NORMAL 下关掉列表后再唤起）
key Escape; sleep 0.3; key A; key C-n; sleep 0.8
check "C-n 手动唤起：列出该位置的候选，第一项已选中（只有「自动弹出且前缀为空」的才不预选）" eval '[[ $(names) == "$LOG"* ]] && sel_is logged_at'

# ---- INSERT 的「全部列」仍预选（F5.13）
fresh; e2e_type "insert into t_log("; sleep 1
check "insert into t_log(：弹出「全部列」，已选中（↵ 一下全加上）" eval '[[ -n $(pop) && -n $(sel) ]] && items | head -1 | grep -q "全部 2 列" || { echo "  $(items | tr "\n" "|")  sel=$(sel)"; false; }'

# ---- 其他触发词同样不预选：select、and、order by
fresh; e2e_type "select * from t_log where msg = 'a' and "; sleep 1
check "and ␠：弹出列名，无选中项" eval 'names_are "$LOG" && nosel'
fresh; e2e_type "select * from t_log order by "; sleep 1
check "order by ␠：弹出列名，无选中项" eval 'names_are "$LOG" && nosel'
key Escape; key Escape

# ---- 快速 SQL：同一套
start; e2e_keys C-p; sleep 0.4; e2e_type ";select * from t_log wher"; sleep 1
check "快速 SQL：wher 弹出 where（有前缀，已选中）" eval 'sel_is where'
key Enter; sleep 1
check "↵ 接受 where：列名弹出，无选中项" eval 'names_are "$LOG" && nosel'
key Tab; sleep 0.3
check "快速 SQL：Tab 选第一项 logged_at" eval 'sel_is logged_at'
key Enter; sleep 0.5
check "快速 SQL：Tab ↵ 接受，输入里是 where logged_at" eval 'e2e_plain | grep -q ";select \* from t_log where logged_at" && [[ $(e2e_panes | awk '"'"'$1 == "-"'"'"' | wc -l) -eq 1 ]]'
key Escape; key Escape; key Escape; sleep 0.3

# ---- WHERE 输入框：and↵↵ 执行；Tab ↵ 选第一列
e2e_stop; SOLO=1 start; open_table t_order; settle
key /; clear_in; e2e_type "id > 5 and"; sleep 0.8
check "WHERE：and 弹出 and（有前缀，已选中）" eval 'sel_is and'
key Enter; sleep 0.8
check "↵ 接受 and（补空格）：列名弹出（有 id、status、paid），无选中项" eval '[[ "|$(names)" == *"|id|"* && "|$(names)" == *"|paid|"* ]] && nosel'
check "输入框是「id > 5 and 」" eval '[[ $(where_in) == "id > 5 and" ]] || { echo "  [$(where_in)]"; false; }'
key Enter; settle; sleep 0.5
check "没有选中项时 ↵：不接受列名，执行（and↵↵），列表关掉，错误栏报错" eval '[[ -z $(pop) && $(where_in) == "id > 5 and" && -n $(errbar 1 | head -1) ]] || { echo "  [$(where_in)] pop=[$(pop)] err=[$(errbar 1 | head -1)]"; false; }'
check "（执行时去掉了尾部空白：输入框里没有多出列名）" eval '! where_in | grep -q "and [a-z]"'
key Escape; key Escape; key /; clear_in; e2e_type "id > 5 and"; sleep 0.6; key Enter; sleep 0.8; key Tab; sleep 0.3
check "WHERE：and↵ 后 Tab 选第一列 id" eval 'sel_is id'
key Enter; sleep 0.4
check "Tab ↵：id 加一个空格进输入框，列表关掉" eval '[[ $(where_in) == "id > 5 and id" && -z $(pop) ]] || { echo "  [$(where_in)] $(names)"; false; }'
key Escape; key Escape

e2e_done
