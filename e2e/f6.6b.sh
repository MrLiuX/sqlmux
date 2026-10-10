#!/usr/bin/env bash
# F6.6 的审查补充（specs/m6-config/task.md F6.6a / F6.6b；tech-design §6.9）：vim = false 下的撤销步、有选区时 Tab / S-Tab 缩进、
# 选区被括号 / 引号包住、没有选区时 Tab 补到制表位、C-h 是退格、提示行用 novim 层的键、WHERE 能拖选、没有可重做时 C-y 光标不动。只读。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/c"; printf 'vim = false\n' >"$D/c/config.toml"
cl() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
cx() { echo $(( $(e2e_flag cursor_x) - 103 )); }                                # 光标前面的字符数（第一列 111）
cy() { e2e_flag cursor_y; }
sel_bg() { [[ $(e2e_style "$1" "$2") == *"bg=#2d3f76"* ]]; }

start -C "$D/c"; key C-l
# ---- 撤销步：↵ 断步
e2e_type "abc"; key Enter; e2e_type "def"; sleep 0.3
key C-z
check "撤销：先撤掉 def（↵ 断开了撤销步），换行和 abc 还在" eval '[[ $(cl 1) == "abc" && $(cl 2) == "" ]]'
key C-z
check "再撤销：↵ 属于它前面那一步（abc ⏎），一起撤掉，缓冲区空了" eval '[[ $(cl 1) == "" && $(cl 2) == "" ]]'
key C-y; key C-y; sleep 0.3
check "两次重做：abc ⏎ 和 def 都回来" eval '[[ $(cl 1) == "abc" && $(cl 2) == "def" ]]'
x=$(cx); y=$(cy); key C-y; sleep 0.3
check "没有东西可重做时 C-y：光标不动、文字不变" eval '[[ $(cx) == $x && $(cy) == $y && $(cl 2) == "def" ]]'

# ---- C-h 是退格
key C-h; sleep 0.3
check "C-h：删掉前一个字符（def → de），焦点没有切走" eval '[[ $(cl 2) == "de" && $(focused) == 2 ]]'
key BSpace; key BSpace; key BSpace; key BSpace
# ---- 没有选区：Tab 补到下一个制表位；行首缩进里 Backspace 一次退一个制表位（tab_width = 2）
start -C "$D/c"; key C-l
e2e_type "x"; key Tab; sleep 0.3
check "没有选区：x 之后按 Tab 补到下一个制表位（第 2 列）" eval '[[ $(cx) == 2 ]]'
key Tab; key Tab; sleep 0.3
check "再按两次 Tab：每次补到下一个制表位（第 4、6 列）" eval '[[ $(cx) == 6 ]]'
key BSpace; key BSpace; key BSpace; key BSpace; key BSpace; key C-a; key Tab; key Tab; key Tab; sleep 0.3
check "行首缩进：三个 Tab 到第 6 列" eval '[[ $(cx) == 6 ]]'
key BSpace; sleep 0.3
check "行首缩进里 Backspace 一次退一个制表位（第 4 列）" eval '[[ $(cx) == 4 ]]'
key BSpace; key BSpace

# ---- 有选区：Tab / S-Tab 缩进 / 反缩进，选区保留
start -C "$D/c"; key C-l
e2e_type "one"; key Enter; e2e_type "two"; key Enter; e2e_type "three"; key Up; key Up; key C-a
key S-Down; key S-Down; sleep 0.3
key Tab; sleep 0.3
check "有选区：Tab 给选中的行缩进（第 1、2 行前面各多 2 个空格），第 3 行不动，选区还在" eval '[[ $(cl 1) == "  one" && $(cl 2) == "  two" && $(cl 3) == "three" ]] && sel_bg 106 2 || { echo "  [$(cl 1)] [$(cl 2)] [$(cl 3)]"; false; }'
key BTab; sleep 0.3
check "S-Tab 反缩进：回到 one / two，选区还在" eval '[[ $(cl 1) == "one" && $(cl 2) == "two" ]] && sel_bg 104 2 || { echo "  [$(cl 1)] [$(cl 2)]"; false; }'

# ---- 打开括号 / 引号把选区包起来
key Down; key Down; key C-a; for i in 1 2 3 4 5; do key S-Right; done
e2e_type "("; sleep 0.3
check "选中 three 再打 (：包成 (three)" eval '[[ $(cl 3) == "(three)" ]] || { echo "  [$(cl 3)]"; false; }'

# ---- 提示行：grid 用 novim 层的键、plain query 的 tab 栏不画 gt/gT
e2e_stop
mkdir -p "$D/c2"; printf 'vim = false\n' >"$D/c2/config.toml"
start -C "$D/c2"; open_table t_order; wait_for 8 settled
g=($(geom 1)); tb=$(e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + g[3] - 2)))
check "表格的提示行：方向键 ←↓↑→ 在前（novim 层的键），不是 hjkl" eval '[[ $tb == *"←↓↑→"* && $tb != *hjkl* ]] || { echo "  $tb"; false; }'
key C-l; g=($(geom 2)); tb=$(e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + g[3] - 2)))
check "plain query 的 tab 栏不画 gt/gT" eval '[[ $tb != *gt/gT* ]] || { echo "  $tb"; false; }'
key C-h; g=($(geom 0)); tf=$(e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + g[3] - 2)))
check "树的提示行：↓/↑ move" eval '[[ $tf == *"↓/↑"* ]] || { echo "  $tf"; false; }'

# ---- WHERE 能拖选
start -C "$D/c2"; open_table t_order; wait_for 8 settled
key /; e2e_type "id < 5"; sleep 0.3
e2e_drag 43 2 47 2; sleep 0.3
check "WHERE 里鼠标拖动选择：选区用 visual 底" eval 'sel_bg 44 2 && sel_bg 46 2'
e2e_type "9"; sleep 0.3
check "再输入替换选区" eval '[[ $(e2e_text 36 95 2 | sed "s/ *▾.*//; s/^ *WHERE *//; s/ *$//") != "id < 5" ]]'

e2e_done
