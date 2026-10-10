#!/usr/bin/env bash
# F5.14 标点用运算符的颜色 + F5.15 树的 pane 节点加图标（specs/m5-workspace/task.md；tech-design §7.3、§7.7、§7.8「工作区节点」）
# 着色由 golden 覆盖；这里在真实终端里看 console、WHERE 输入框两处（快速 SQL 的输入行没有 SQL 着色，见批次报告）的 ; , ( ) * 是 sql_operator 色（默认主题 #89ddff），
# . 和普通文字不是；树的工作区里 pane 节点前有 pane 图标（U+F0DB），ascii 模式下是 |。只读。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
OP=#89ddff
PANE_ICON=$(printf '\xef\x83\x9b')
# fg_of TEXT ROW [N]：ROW 行上第 N 处（默认第一处）TEXT 的前景色
fg_of() { local x; x=$(e2e_find "$1" "$2" | tr ' ' '\n' | sed -n "${3:-1}p"); e2e_style "$x" "$2" | grep -oE 'fg=#[0-9a-f]+' | cut -c4-; }
is_op() { [[ $(fg_of "$@") == $OP ]] || { echo "  '$1' (row $2): fg $(fg_of "$@"), want $OP"; false; }; }
not_op() { local c; c=$(fg_of "$@"); [[ $c != $OP && -n $c ]] || { echo "  '$1' (row $2): fg '$c' should not be $OP"; false; }; }

start; key C-l; key i
e2e_type "select a.b, (1); "; sleep 0.5; key Escape; sleep 0.3
check "console：, ( ) ; 是运算符色" eval 'is_op "," 2 && is_op "(" 2 && is_op ")" 2 && is_op ";" 2'
check "console：schema.table 里的 . 不着色（不是运算符色），列 b 也不是" eval 'not_op "." 2 && not_op "b," 2'
check "console：关键字 select 不是运算符色" not_op select 2
key A; key Enter; e2e_type "select * from t_log\;"; sleep 0.5; key Escape; sleep 0.3
check "console：* 和 ; 是运算符色" eval 'is_op "*" 3 && is_op ";" 3'
key d G

# ---- 树的工作区（F10.5：没有 pane 节点，tab 行首是所属 pane 的编号）
check "树的工作区：没有 pane-n 节点" eval '! e2e_plain | grep -qE "pane-[0-9]"'
check "工作区里 console_1 的行首是 ②（它所在的 pane）" eval 'e2e_plain | grep -qE "② .*console_1"'

# ---- WHERE 输入框
e2e_stop; SOLO=1 start; open_table t_order; wait_for 8 settled
key /; clear_in; e2e_type "id in (1, 2) and note = 'x;'"; sleep 0.5
y=2
check "WHERE：( ) , 是运算符色，= 也是；引号里的 ; 不是" eval 'is_op "(" $y && is_op ")" $y && is_op "," $y && is_op "=" $y && not_op ";" $y'
key Escape; key Escape; key Escape

e2e_done
