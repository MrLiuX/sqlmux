#!/usr/bin/env bash
# F10.8 flash 式跳转（specs/m10-feedback3/task.md F10.8；tech-design §6.10）
# s 进 JUMP，打字后每个匹配处出现标签（warn 底），按标签跳到那里：表格里是 grid.goto，树里光标到节点（不展开、不打开）；
# 标签不用匹配后面紧跟的字符，只有一个匹配也不自动跳，backspace 删到空仍在 JUMP，esc 退出光标不动，↵ 跳到最近的匹配，smartcase。
# 标签的画法、backdrop 由 golden 覆盖。只读，用共用库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

WARN=#e0af68 SELECT=#364a82
labels() { e2e_bgcells "$WARN" | awk '{ print $3 }' | tr '\n' ' '; }                # 屏幕上的标签字母
nlabels() { e2e_bgcells "$WARN" | wc -l | tr -d ' '; }
label_xy() { e2e_bgcells "$WARN" | awk -v c="$1" '$3 == c { print $1, $2; exit }'; }  # 标签 C 的位置
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=$SELECT* ]] && { e2e_text 1 32 "$y"; return; }; done; }
jump_is() { mode_is JUMP; }

start; open_table t_order; wait_for 8 settled
key j; key l                                                                        # 光标在 (2,2)
p0=$(pos)
key s
check "s：模式块 JUMP，还没有标签" eval 'jump_is && [[ $(nlabels) == 0 ]]'
e2e_type pen; sleep 0.5
check "打 pen：每个 status = pending 的单元格出现标签（warn 底），模式仍是 JUMP" eval 'jump_is && (( $(nlabels) >= 5 ))'
check "标签里没有 d（pen 后面紧跟的字符），也没有重复" eval '[[ " $(labels)" != *" d "* ]] && [[ $(labels | tr " " "\n" | sort | uniq -d | wc -l | tr -d " ") == 0 ]]' || labels
check "标签从 a 开始、按离光标的距离分配：最近的一个（第 4 行 status）是 a" eval 'xy=($(label_xy a)); [[ -n ${xy[0]} ]] && [[ $(e2e_text 35 130 ${xy[1]}) == *" 4 │ "* || $(e2e_text 35 130 ${xy[1]}) == *"4 │"* ]]'
xy=($(label_xy a))
e2e_type a; sleep 0.5
check "按 a：跳到那一格（第 4 行、status 列 = 3），退出 JUMP 回 NORMAL" eval 'mode_is NORMAL && pos_is 4,3 && [[ $(nlabels) == 0 ]]'

# ---- backspace / esc / ↵ / smartcase / 一个匹配不自动跳
key g g; key 0; p1=$(pos)
key s; e2e_type pen; key BSpace; key BSpace; key BSpace; sleep 0.3
check "backspace 删到空：仍在 JUMP，标签没了" eval 'jump_is && [[ $(nlabels) == 0 ]]'
key Escape; sleep 0.3
check "esc：退出 JUMP，光标不动" eval 'mode_is NORMAL && pos_is 1,1'
key s; e2e_type Pen; sleep 0.4
check "smartcase：含大写的 Pen 区分大小写，pending 不匹配，没有标签，还在 JUMP" eval 'jump_is && [[ $(nlabels) == 0 ]]'
key BSpace; key BSpace; key BSpace; e2e_type pen; sleep 0.4
check "全小写 pen 不分大小写：有标签" eval '(( $(nlabels) >= 5 ))'
key Enter; sleep 0.4
check "↵：跳到最近的匹配（第 4 行 status），退出 JUMP" eval 'mode_is NORMAL && pos_is 4,3'
key g g; key 0; key s; e2e_type "00:07:00"; sleep 0.4
check "（只有一个匹配）00:07:00 只在第 7 行出现，只有一个标签，也不自动跳：还在 JUMP，光标没动" eval 'jump_is && [[ $(nlabels) == 1 ]] && pos_is 1,1'
key Escape

# ---- 树：光标到节点，不展开、不打开
key C-h; sleep 0.3; key g g
key s; e2e_type t_us; sleep 0.5
check "树里 s、t_us：t_user 节点出现标签" eval 'jump_is && [[ $(nlabels) -ge 1 ]]'
l=$(labels | awk '{ print $1 }'); tabs0=$(e2e_text 34 159 43)
e2e_type "$l"; sleep 0.5
check "按标签：树的光标到 t_user，模式回 NORMAL" eval 'mode_is NORMAL && [[ $(tsel) == *t_user* ]] || { echo "  cursor: [$(tsel)]"; false; }'
check "没有展开 t_user，也没有打开新 tab（tab 栏不变）" eval '[[ $(tsel) != *▾* && $(e2e_text 34 159 43) == "$tabs0" ]]'
e2e_done
