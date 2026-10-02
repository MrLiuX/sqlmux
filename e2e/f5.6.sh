#!/usr/bin/env bash
# F5.6 tab 上悬停的 × 底色画满（specs/m5-workspace/task.md F5.6；tech-design §7.7「图标后面留空格」、§7.8 tab 栏）
# 底色由 golden 覆盖（TestGoldenTabClose）；这里在真终端里测：× 和它后面那一格一起亮、都是关闭的命中区，
# 没有标记的 tab 借右边 │ 那一格，结果区最后一个 tab 后面是一格亮着的空格；悬停前后 tab 栏不移位。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
SELECT=#364a82 CLOSE=$(printf '\xef\x80\x8d')   # U+F00D
tabs_at() { local t; for t in "$@"; do tab_x "$P" "$t"; done | tr '\n' ' '; }     # 各个「N:」所在的列（看移位没有）
lit_at() { style_has "$1" "$(tab_y "$P")" "bg=$SELECT"; }
ch() { e2e_text "$1" "$1" "$(tab_y "$P")"; }
end_of() { local x; x=$(e2e_find "$1" "$(tab_y "$P")" | tr ' ' '\n' | awk '$1 > 33 { print; exit }'); echo $((x + $(strwidth "$1"))); }   # NAME：名字后面那一格

# ---- 表格 pane：1:t_order（没有标记）│ 2:t_user-（上一个）│ 3:t_sku*（当前）
SOLO=1 start; open_table t_order; open_table t_user; open_table t_sku; P=1; y=$(tab_y 1)
x0=$(tabs_at 1: 2: 3:)
c=$(end_of t_order)                                                              # 名字后面的空格，再右边是 │
e2e_move $((c - 4)) "$y"; sleep 0.3
check "悬停没有标记的 t_order：名字后面那一格换成 ×，右边的 │ 换成空格，指针不在 × 上时都不亮，tab 栏不移位" eval '[[ $(ch $c) == "$CLOSE" && $(ch $((c + 1))) == " " && $(tabs_at 1: 2: 3:) == "$x0" ]] && ! lit_at $c >/dev/null'
e2e_move "$c" "$y"; sleep 0.3
check "指针停在 × 上：× 和借来的那一格都是 select 底" eval 'lit_at $c && lit_at $((c + 1))'
e2e_move $((c + 1)) "$y"; sleep 0.3
check "停在借来的那一格上：同样两格都亮" eval 'lit_at $c && lit_at $((c + 1)) && [[ $(tabs_at 1: 2: 3:) == "$x0" ]]'
c=$(end_of t_user); e2e_move $((c + 1)) "$y"; sleep 0.3
check "上一个 tab t_user-：× 替换 -，停在后面那一格上两格都亮，t_order 恢复成 │" eval '[[ $(ch $c) == "$CLOSE" ]] && lit_at $c && lit_at $((c + 1)) && [[ $(ch $(( $(end_of t_order) + 1 ))) == "│" && $(tabs_at 1: 2: 3:) == "$x0" ]]'
c=$(end_of t_sku); e2e_move $((c + 1)) "$y"; sleep 0.3
check "当前 tab t_sku*：× 替换 *，后面那一格一起亮" eval '[[ $(ch $c) == "$CLOSE" ]] && lit_at $c && lit_at $((c + 1))'
c=$(end_of t_order); e2e_move "$c" "$y"; sleep 0.2; e2e_click $((c + 1)) "$y"; sleep 0.4; e2e_move 100 20; sleep 0.3   # 指针移开再看：留在原处会悬停到挪过来的下一个 tab
check "点 t_order 借来的那一格（原来的 │）：关掉 t_order，当前 tab 不变" tabs_are 1 "1:t_user- │ 2:t_sku*"
c=$(end_of t_user); e2e_move "$c" "$y"; sleep 0.2; e2e_click $((c + 1)) "$y"; sleep 0.4; e2e_move 100 20; sleep 0.3   # 指针移开再看：留在原处会悬停到挪过来的下一个 tab
check "点 t_user- 的 × 后面那一格：关掉 t_user" tabs_are 1 "1:t_sku*"

# ---- 结果区：1:日志 │ 2:console_1 #1（固定）│ 3:console_1 #2，当前 #1、上一个日志，最后一个 tab 没有标记
start; key C-l; key i; e2e_type "select 1 as a"; key Escape; key Enter; wait_for 8 eval '[[ -n $(geom 3) ]]'; sleep 0.3
key C-j; key P; key C-k; key Enter; sleep 1; click_tab 3 "1:"; click_tab 3 "2:"; P=3; y=$(tab_y 3)
rtabs() { tabbar 3 | sed -E 's/ {3,}.*//'; }
check "准备好了：③ 1:日志- │ 2:console_1 #1* │ 3:console_1 #2" eval '[[ $(rtabs) == "1:日志- │ 2:console_1 #1* │ 3:console_1 #2" ]] || { echo "  $(rtabs)"; false; }'
x0=$(tabs_at 1: 2: 3:); c=$(end_of "console_1 #2")
e2e_move $((c + 1)) "$y"; sleep 0.3
check "最后一个 tab（没有标记、右边没有 │）：名字后面是 ×，再后面一格亮着的空格，两格都亮，tab 栏不移位" eval '[[ $(ch $c) == "$CLOSE" && $(ch $((c + 1))) == " " && $(tabs_at 1: 2: 3:) == "$x0" ]] && lit_at $c && lit_at $((c + 1))'
c=$(end_of 日志); e2e_move $((c - 2)) "$y"; sleep 0.3
check "日志 tab 不能关：悬停不画 ×（- 还在）" eval '[[ $(ch $c) == "-" ]]'
c=$(end_of "console_1 #2"); e2e_move "$c" "$y"; sleep 0.2; e2e_click $((c + 1)) "$y"; sleep 0.4; e2e_move 100 20; sleep 0.3   # 指针移开再看：留在原处会悬停到挪过来的下一个 tab
check "点最后一个 tab 后面那格亮着的空格：关掉 console_1 #2" eval '[[ $(rtabs) == "1:日志- │ 2:console_1 #1*" ]] || { echo "  $(rtabs)"; false; }'

e2e_done
