#!/usr/bin/env bash
# F17.6 命令面板的预览区：滚轮和焦点（specs/m17-polish-exec/task.md F17.6；tech-design §12「预览」）
# 滚轮在预览区上滚预览、在列表上动列表（预览跟着换）；palette.preview.focus（C-o）把焦点切进预览区，预览换成只读的 vim 编辑器
# （行号、j/k、/ 搜索、v / y），esc 先清搜索再回列表；列表换了候选（点别的一行）时预览重载、焦点回列表。自建 PG 库：十六张列很多的表，DDL 比预览区高。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
. "$(dirname "$0")/palette.sh"
cols=$(python3 -c "print(', '.join('c%02d int' % i for i in range(1, 31)))")
for t in t_w01 t_w02 t_w03 t_w04 t_w05 t_w06 t_w07 t_w08 t_w09 t_w10 t_w11 t_w12 t_w13 t_w14; do psql "$E2E_DB" -q -c "create table $t ($cols)"; done
cur() { t display -p -t t '#{cursor_x},#{cursor_y}'; }
sel() { selected | awk '{ print $1 }'; }                                        # the selected candidate's name
pv1() { preview | head -1 | sed 's/^ *[0-9]* //'; }                              # the preview's first row, a line number cut
first_is() { [[ $(pv1) == "$1" ]] || { echo "  preview row 1: '$(pv1)', want '$1'"; false; }; }
editor() { preview | head -1 | grep -qE '^ +1 create table'; }                 # the preview is the editor: its rows carry line numbers
pick_y() { local s1 s2; read s1 s2 <<<"$(seps)"; echo $((s2 + $1)); }           # screen row of the preview's Nth row
px() { echo $(( $(left) + 20 )); }

start -C "$D/own"
pal "@t_w"; sleep 1.5
check "（准备）@t_w：十四张表，第一项 t_w01 选中，预览是它的 DDL，从头开始" eval '[[ $(sel) == t_w01 && $(nrows) -ge 12 ]] && first_is "create table public.t_w01 (" && ! editor' || { sel; preview | head -3; false; }

# ---- 滚轮：在预览区上滚预览
e2e_wheel "$(px)" "$(pick_y 4)" down; sleep 0.5
check "预览区上滚轮向下：预览往下滚（第一行变成 …），选中项不动（还是 t_w01）" eval '[[ $(pv1) == "…" && $(sel) == t_w01 ]] || { pv1; sel; false; }'
e2e_wheel "$(px)" "$(pick_y 4)" down; sleep 0.4; e2e_wheel "$(px)" "$(pick_y 4)" down; sleep 0.4
check "再滚两下：更靠后（第二行的列号变大了）" eval 'preview | sed -n 2p | grep -qE "c(0[7-9]|1[0-9]|2[0-9])" || { preview | head -3; false; }'
e2e_wheel "$(px)" "$(pick_y 4)" up; sleep 0.4; e2e_wheel "$(px)" "$(pick_y 4)" up; sleep 0.4; e2e_wheel "$(px)" "$(pick_y 4)" up; sleep 0.4
check "向上滚回去：回到开头，第一行是 create table，选中项没动" eval 'first_is "create table public.t_w01 (" && [[ $(sel) == t_w01 ]]'

# ---- 滚轮：在列表上动列表，预览跟着换
read s1 s2 <<<"$(seps)"
e2e_wheel "$(px)" $((s1 + 3)) down; sleep 0.8
check "列表上滚轮向下：选中项往下走了（不再是 t_w01），预览换成新选中那张表的 DDL，从头开始" eval 'n=$(sel); [[ $n != t_w01 && -n $n ]] && first_is "create table public.$n (" || { sel; pv1; false; }'
e2e_wheel "$(px)" $((s1 + 3)) up; sleep 0.8
check "向上滚回：t_w01、预览也回来" eval '[[ $(sel) == t_w01 ]] && first_is "create table public.t_w01 ("'

# ---- C-o：焦点进预览，预览是只读的 vim 编辑器
e2e_keys C-o; sleep 0.6
check "C-o：预览换成编辑器（每行带行号，第一行 1 create table public.t_w01 (），光标在预览里（第一行）" eval 'editor && first_is "create table public.t_w01 (" && [[ $(cur | cut -d, -f2) == $(pick_y 1) || $(cur | cut -d, -f2) == $(( $(pick_y 1) - 1 )) ]]' || { preview | head -2; cur; pick_y 1; }
y0=$(cur | cut -d, -f2)
e2e_type j; sleep 0.3; e2e_type j; sleep 0.3
check "j j：光标在预览里往下两行（列表的选中项没动，输入框还是 @t_w）" eval '[[ $(cur | cut -d, -f2) == $((y0 + 2)) && $(sel) == t_w01 ]] && input_is "@t_w"' || { cur; sel; input; }
e2e_type x; sleep 0.3
check "x 是编辑器的按键（删一个字符都不行：只读），不进输入框" eval 'input_is "@t_w" && editor'
e2e_type '"+yy'; sleep 0.8
check "\"+yy：光标那一行（第三行，  c02 integer,）进了（假）系统剪贴板" eval '[[ $(clip) == "  c02 integer," ]] || { echo "  clip: [$(clip)]"; false; }'
e2e_type "/c05"; key Enter; sleep 0.5
check "/c05 ↵：搜索，预览里亮出命中（底行 /c05 [1/1]），光标移到 c05 那一行" eval 'preview | tail -1 | grep -q "/c05" && preview | tail -1 | grep -q "\[1/1\]" || { preview | tail -2; false; }'
key Escape; sleep 0.4
check "esc（搜索高亮亮着）：先清搜索，还在预览里（编辑器、行号都在）" eval 'editor && ! preview | tail -1 | grep -q "/c05" || { preview | tail -2; false; }'
key Escape; sleep 0.4
check "再 esc：回列表，预览不再是编辑器（没有行号）" eval '! editor && [[ $(sel) == t_w01 ]]'
e2e_type x; sleep 0.5
check "之后打字进输入框（@t_wx），列表过滤掉了（焦点在输入框上）" eval 'input_is "@t_wx"'
e2e_keys BSpace; sleep 0.8

# ---- 焦点在预览里：:q 回列表；面板底下的 pane / 窗口不被动到；? 是反向搜索
panes() { e2e_panes | awk '$1 != "-"' | wc -l | tr -d ' '; }
np=$(panes); tb=$(e2e_plain | tail -2 | head -1 | cut -c1-60)
e2e_keys C-o; sleep 0.5
e2e_type ":q"; key Enter; sleep 0.5
check ":q（预览里的 vim）：回列表，不是关面板，不是编辑器了" eval 'is_open && ! editor && input_is "@t_w"' || { input; editor && echo editor; false; }
e2e_keys C-o; sleep 0.5
key Space; e2e_type c; sleep 0.5; key g t; sleep 0.3; key C-h; sleep 0.3; key C-l; sleep 0.3
check "焦点在预览区：Space c、gt、C-h、C-l 都是预览自己的（vim 里没有这些），面板底下的 pane 数和窗口栏没变，还在预览里" eval '[[ $(panes) == "$np" && $(e2e_plain | tail -2 | head -1 | cut -c1-60) == "$tb" ]] && editor && is_open' || { panes; echo "want $np"; false; }
e2e_type "?c05"; key Enter; sleep 0.5
check "? 是 vim 的反向搜索：底行 ?c05，命中亮出来" eval 'preview | tail -1 | grep -qF "?c05"' || { preview | tail -2; false; }
key Escape; key Escape; sleep 0.5
check "（收尾）esc esc 回列表" eval '! editor && is_open'

# ---- 焦点在预览里，鼠标移到列表别的一行（悬停选中）：候选换了，预览重载、焦点回列表
e2e_keys C-o; sleep 0.5
check "（准备）C-o 又进预览" editor
read s1 s2 <<<"$(seps)"
e2e_move "$(px)" $((s1 + 3)); sleep 1.2
check "鼠标移到列表第三行：选中 t_w03，预览重载成它的 DDL（不是编辑器、没有行号）" eval '[[ $(sel) == t_w03 ]] && ! editor && first_is "create table public.t_w03 ("' || { sel; preview | head -2; }
e2e_type x; sleep 0.5
check "焦点回到列表（输入框）：打字进输入框" eval 'input_is "@t_wx"'
e2e_keys BSpace; sleep 0.5

# ---- 没有预览的范围里 C-o 不抢焦点
clear_input; e2e_type ">"; sleep 0.6; e2e_keys C-o; sleep 0.4; e2e_type "x"; sleep 0.5
check "命令范围（没有预览）：C-o 不起作用，打字还是进输入框（>x）" eval 'input_is ">x"'
key Escape; sleep 0.3
e2e_done
