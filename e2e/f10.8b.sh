#!/usr/bin/env bash
# F10.8 flash 跳转的细节（specs/m10-feedback3/task.md F10.8；tech-design §6.10）
# 跨 pane：在表格里 s 跳到 console 的字符（聚焦 console、光标放到那个字符），在 console 里 s 跳到表格的单元格；结果区的单元格也能跳；
# C-c 等同 esc（不算「连按两次退出」的第一次）；粘贴追加到模式；console 在 VISUAL 时从命令面板执行 jump.start 进 JUMP、不删选区。
# 基本流程在 f10.8.sh。标签的画法由 golden 覆盖。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

WARN=#e0af68
nlabels() { e2e_bgcells "$WARN" | wc -l | tr -d ' '; }
labels() { e2e_bgcells "$WARN" | awk '{ print $3 }' | tr '\n' ' '; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.4; }
cline() { local g; g=($(geom 2)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 1)) | sed 's/^ *[▶ ]* *[0-9]* //; s/ *$//'; }   # ② 第一行的文字

SOLO= start; key C-l
key i; e2e_type "select 1 as marker"; key Escape; sleep 0.4
key C-h; sleep 0.3                                                            # 焦点到 ①（空 pane），表格开在这里
open_table t_order; wait_for 8 settled
check "（准备）焦点在 ① 的表格上，console 里是 select 1 as marker" eval 'focus_is 1 && [[ $(cline) == *"select 1 as marker"* ]]'

# ---- 表格里 s 跳到 console 的字符
key s; e2e_type marker; sleep 0.5
check "表格里 s、marker：屏幕上只有 console 里那一处匹配，一个标签" eval 'mode_is JUMP && [[ $(nlabels) == 1 ]]'
l=$(labels | awk '{ print $1 }'); e2e_type "$l"; sleep 0.5
check "按标签：焦点到 ② console，回到 NORMAL，光标在 marker 的第一个字符（从 0 数的第 12 列）" eval 'focus_is 2 && mode_is NORMAL && (( $(e2e_flag cursor_x) - 103 == 12 )) || { echo "  focus $(focused) cursor_x $(e2e_flag cursor_x)"; false; }'

# ---- console 里 s 跳到表格的单元格
key s; e2e_type pen; sleep 0.5
check "console 里 s、pen：表格里 pending 的格子出现标签" eval 'mode_is JUMP && (( $(nlabels) >= 5 ))'
l=$(labels | awk '{ print $1 }'); e2e_type "$l"; sleep 0.5
check "按标签：焦点到 ① 表格，光标在第 4 行 status（4,3），回到 NORMAL" eval 'focus_is 1 && mode_is NORMAL && pos_is 4,3 || { echo "  focus $(focused) pos $(pos)"; false; }'

# ---- C-c 等同 esc：不算连按两次退出的第一次
key s; e2e_type pen; sleep 0.4; key C-c; sleep 0.4
check "JUMP 里 C-c：退出，光标不动（4,3），没有「再按一次 C-c 退出」的提示" eval 'mode_is NORMAL && pos_is 4,3 && ! screen_has "再按一次" && [[ $(nlabels) == 0 ]]'
key C-c; sleep 0.3
check "（紧接着）NORMAL 下的第一个 C-c 才是退出提示的第一次" eval 'screen_has "再按一次"' || true
key Escape; sleep 2.2

# ---- 粘贴追加到模式
key s; paste pen; sleep 0.5
check "JUMP 里粘贴 pen：追加到模式，出现标签，仍在 JUMP，没有写进别处" eval 'mode_is JUMP && (( $(nlabels) >= 5 ))'
key Escape; sleep 0.3

# ---- console VISUAL 里从面板执行 jump.start：进 JUMP，选区不被删掉
key C-l; key 0; key v; key l; key l; key l; sleep 0.3
check "（准备）console 里 v 选了 4 个字符（VISUAL）" eval 'mode_is VISUAL'
pal ">跳转到屏幕"; key Enter; sleep 0.6
check "面板执行 jump.start：进 JUMP（console VISUAL 里也进），console 的文字没被删" eval 'mode_is JUMP && [[ $(cline) == *"select 1 as marker"* ]]'
key Escape; sleep 0.4
key Escape; sleep 0.3
check "esc 退出 JUMP 后 console 文字还是原样" eval '[[ $(cline) == *"select 1 as marker"* ]]'

# ---- 结果区的单元格也能跳
key C-l; key i; key Escape; key g g; key d G; key i; paste "select 'zz' || 'q' as k"; key Escape; key g g; key Enter; done_run
check "（准备）执行后底部出现结果区 ③" eval '[[ -n $(geom 3) ]]'
key s; e2e_type zzq; sleep 0.5
check "s、zzq：结果区里那一格出现标签" eval 'mode_is JUMP && [[ $(nlabels) == 1 ]]'
l=$(labels | awk '{ print $1 }'); e2e_type "$l"; sleep 0.5
check "按标签：焦点到结果区 ③，回到 NORMAL" eval 'focus_is 3 && mode_is NORMAL'
e2e_done
