#!/usr/bin/env bash
# F17.14 手动列宽的细节（specs/m17-polish-exec/task.md F17.14；tech-design §7.8「列宽」3）
# 定宽列不随内容变（翻页）、窗口变窄时不被挤；拖动中跟手；在另一个 pane 里拖焦点切过去；
# 转置视图里 C-w > 不动；键位帮助里有 C-w 一组；C-w 没吃掉输入框里原有的删词。只读，用共用库的 t_order。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

head_() { e2e_text "${XL:-35}" "${XR:-159}" "${HY:-$(hy)}"; }
cell() { head_ | python3 -c "
import sys
t = sys.stdin.read().rstrip('\n'); x = ${XL:-35} + len(t.split('│')[0]) + 1; i = 0
for seg in t.split('│')[1:-1]:
    i += 1
    if i == $1:
        print(len(seg) if '$2' == 'w' else x + len(seg)); break
    x += len(seg) + 1
"; }
cw() { cell "$1" w; }
sx() { cell "$1" x; }
cw_is() { local got; got=$(cw "$1"); [[ $got == "$2" ]] || { echo "  $1: $got wide, want $2 ($(head_))"; false; }; }

start; open_table t_order; wait_for 8 settled
ST=$(cw 3)

# ---- 不随内容变：status 收窄 3 列，翻到下一页（内容变了）还是这个宽
key l; key l; key 3; key C-w; key '<'
check "（准备）status 窄 3 列" eval 'cw_is 3 $((ST - 3))'
key ']'; wait_for 8 settled; sleep 0.5
check "翻到下一页：status 还是窄 3 列" eval 'cw_is 3 $((ST - 3))'
key '['; wait_for 8 settled; sleep 0.5
key C-w; key '='

# ---- 窗口变窄时定宽列不被挤：status 加宽 6 列，窗口 160 → 130 列，宽度不变；别的长列收缩
key 6; key C-w; key '>'
check "（准备）status 宽 +6" eval 'cw_is 3 $((ST + 6))'
e2e_resize 130 45; sleep 1; XR=129
check "窗口缩到 130 列：status 仍是 +6 列" eval 'cw_is 3 $((ST + 6))'
e2e_resize 160 45; sleep 1; XR=159
key C-w; key '='

# ---- 转置视图里 C-w > 不动，转回来 status 没变
key T; sleep 0.6; key C-w; key '>'; sleep 0.3; key T; sleep 0.5
check "转置视图里按 C-w >，转回来：status 宽没变" eval 'cw_is 3 $ST'

# ---- 拖动中跟手、松开后定住
key g g; key 0
X=$(sx 2); Y=$(hy)
e2e_down "$X" "$Y"; e2e_drag_to $((X + 4)) "$Y"; sleep 0.4
check "按住拖到 +4：还没松开，user_id 右边的 │ 已经在 +4" eval '[[ $(sx 2) == $((X + 4)) ]] || { echo "  sep at $(sx 2), want $((X + 4))"; false; }'
e2e_drag_to $((X + 1)) "$Y"; sleep 0.4
check "接着往回拖到 +1：│ 跟着回来" eval '[[ $(sx 2) == $((X + 1)) ]] || { echo "  sep at $(sx 2), want $((X + 1))"; false; }'
e2e_up $((X + 1)) "$Y"; sleep 0.4
check "松开：定在 +1" eval '[[ $(sx 2) == $((X + 1)) ]]'
key C-w; key '='

# ---- 两个 pane：在右边那个 pane 的表头上拖，焦点切过去，宽度照样变（不是第一个 pane 的那份）
two_panes; key C-l; open_table t_user; wait_for 8 settled; key C-h
G=($(geom 2)); XL=$((G[0] + 1)); XR=$((G[0] + G[2] - 2)); HY=$((G[1] + 3))
focus_is 1
R0=$(cw 1); L0=$(XL=35 XR=$((G[0] - 3)) HY=$(hy) cw 1); X=$(sx 1)
echo "  ② at ${G[*]}, header: $(head_), id at x=$X"
e2e_down "$X" "$HY"; e2e_drag_to $((X + 3)) "$HY"; e2e_up $((X + 3)) "$HY"; sleep 0.5
check "在 ② 的表头上拖第一列右边的 │：它宽 +3" eval 'cw_is 1 $((R0 + 3))'
check "拖完焦点在 ②" eval 'focus_is 2'
check "第一个 pane 的列宽没变" eval 'got=$(XL=35 XR=$((G[0] - 3)) HY=$(hy) cw 1); [[ -n $L0 && $got == "$L0" ]] || { echo "  [$got] want [$L0]"; false; }'
unset XL XR HY

# ---- 键位帮助里有 C-w 一组
key '?'; sleep 0.6
check "?：键位帮助里有 C-w 一组" eval 'e2e_plain | grep -q "C-w" || { e2e_plain | head -5; false; }'
key Escape; sleep 0.3

# ---- C-w 没吃掉输入框里原有的删词
e2e_keys C-p; sleep 0.3; e2e_type "abc def"; sleep 0.3; key C-w
check "命令面板输入框：abc def 上 C-w 删掉一个词，剩 abc" eval 'e2e_plain | grep -q "abc" && ! e2e_plain | grep -q "abc def"' || e2e_plain | grep -n "abc" | head -3
key Escape; key Escape
e2e_done
