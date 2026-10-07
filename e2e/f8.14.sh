#!/usr/bin/env bash
# F8.14 WHERE 下拉：历史在上、收藏在下、中间一条线；入口写上键位（specs/m8-feedback/task.md F8.14；tech-design §9.7、§7.8 查询条）
# 画面（有历史和收藏 / 只有历史 / 右端键位文字）有 golden；这里在真实终端里测：下拉的顺序、分隔线、底部提示读 keymap、WHERE 行右端的键位
# 随 where.history 的键变、放不下时只留 ▾、点键位文字也能打开下拉。只读，用共用库；state 放临时目录。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); ST=$D/state; trap 'e2e_stop; rm -rf "$D"' EXIT
typ() { e2e_type "$1"; sleep 0.4; }
edit() { key /; clear_in; }
run() { edit; typ "$1"; key Enter; wait_for 8 settled; sleep 0.2; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }'; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | noicon | tr -s ' ' | sed 's/^ //; s/ $//'; done; }
rowhint() { e2e_text 35 159 2; }

# ---- 只有历史：没有分隔线
start -S "$ST"; open_table t_order; wait_for 8 settled
run "id < 50"; run "status = 'done'"
edit; key C-r; sleep 0.5
check "只有历史：两项，没有分隔线，底部有一行提示" eval '[[ $(items | grep -c "^─") == 0 && $(items | grep -c "^id < 50\\|^status") == 2 && $(items | tail -1) == "C-p/C-n 移动"* ]] || { items; false; }'
check "底部提示的键位文字来自 keymap：移动 C-p/C-n、↵ 应用、C-f 收藏 / 取消收藏、esc 关闭" eval 'items | tail -1 | grep -q "^C-p/C-n 移动 · ↵ 应用 · C-f 收藏 / 取消收藏 · esc 关闭$"'
key C-f
check "C-f 收藏第一项（最新的 status = 'done'）：历史在上，一条横线，收藏在下" eval 'it=$(items); [[ $(grep -c "^─" <<<"$it") == 1 ]] && n=$(grep -n "^─" <<<"$it" | cut -d: -f1) && [[ $(sed -n "$((n + 1))p" <<<"$it") == "status = "* ]] && sed -n "1p" <<<"$it" | grep -q "^status = " || { echo "$it"; false; }'
key Escape; key Escape

# ---- WHERE 行右端：history 图标 + 键 + ▾
check "WHERE 行右端：dim 色的 C-r ▾（默认键 C-r，来自 keymap）" eval 'rowhint | grep -q "C-r ▾" && c=$(e2e_find "C-r" 2 | cut -d" " -f1) && style_has $c 2 fg=#565f89'
# 点键位文字也能打开下拉
c=$(e2e_find "C-r" 2 | cut -d' ' -f1); e2e_click $((c + 1)) 2; sleep 0.6
check "点 WHERE 行右端的 C-r：打开历史下拉" eval '[[ -n $(pop) ]] && mode_is COMMAND'
key Escape; key Escape

# ---- where.history 换了键：右端显示新键
printf '[keys.input]\n"<C-r>" = ""\n"<C-g>" = "where.history"\n' >|"$D/k.toml"
start -S "$ST" -K "$D/k.toml"; open_table t_order; wait_for 8 settled
check "把 where.history 绑到 C-g（C-r 解绑）：WHERE 行右端显示 C-g ▾" eval 'rowhint | grep -q "C-g ▾" && ! rowhint | grep -q "C-r"'
edit; key C-g; sleep 0.5
check "C-g 打开下拉" eval '[[ -n $(pop) ]]'
key Escape; key Escape

# ---- 输入框留不下 16 列：只留 ▾（宽度由 pane 决定，和输入的字数无关）
rowhint2() { e2e_text 27 $(( $(e2e_flag pane_width) - 2 )) 2; }
start -S "$ST" -x 80 -y 24; open_table t_order; wait_for 8 settled
check "80 宽（① 约 53 列）：右端有 C-r ▾" eval 'e2e_plain | sed -n 2p | grep -q "C-r ▾"'
start -S "$ST" -x 52 -y 24; open_table t_order; wait_for 8 settled
check "52 宽（① 窄得留不下 16 列输入）：右端只留 ▾，没有 C-r" eval 'r=$(e2e_plain | sed -n 2p); grep -q "▾" <<<"$r" && ! grep -q "C-r" <<<"$r" || { echo "  $r"; false; }'
e2e_done
