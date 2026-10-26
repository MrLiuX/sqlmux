#!/usr/bin/env bash
# F18.9 WHERE 的 C-r 下拉在没有历史和收藏时也要开（specs/m18-windows-mssql/task.md F18.9；tech-design §9.4「WHERE 的 C-r 历史 / 收藏下拉」）
# 新表上按 C-r：下拉照开，列表是 dim 色的一行「没有历史」，底部提示照画；↵ / C-e / C-f 无事，开始输入后仍是这一行，esc 关；
# 跑过一次 WHERE 之后再按 C-r，列出那一条。只读，用共用库；state 放临时目录（没有历史）。
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

start -S "$ST"; open_table t_order; wait_for 8 settled
edit; key C-r; sleep 0.6
check "新表（没有历史和收藏）上 WHERE 里按 C-r：下拉照开，列表里是「没有历史」一行" eval '[[ -n $(pop) ]] && items | grep -q "^没有历史$" || { items; false; }'
check "没有可选的项，没有分隔线；底部提示照画（C-p/C-n 移动 … esc 关闭）" eval '[[ $(items | grep -c "^─") == 0 ]] && items | tail -1 | grep -q "esc 关闭" || { items; false; }'
c=$(e2e_find "没有历史" "$(( $(pop | awk '{ print $2 }') + 1 ))" | cut -d' ' -f1)
check "「没有历史」是 dim 色（不是一个可选项的高亮）" eval '[[ -n $c ]] && ! style_has $c $(( $(pop | awk "{ print \$2 }") + 1 )) bg=#364a82 >/dev/null'
key Enter; sleep 0.3
check "↵：什么都不执行，下拉还开着，WHERE 还是空，没崩" eval 'running && [[ -n $(pop) ]] && items | grep -q "^没有历史$"'
key C-e; sleep 0.2; key C-f; sleep 0.2
check "C-e / C-f：无事，下拉还开着" eval 'running && [[ -n $(pop) ]]'
typ "abc"
check "开始输入之后仍是这一行（没有可过滤的）" eval 'running && items | grep -q "^没有历史$"'
key Escape; sleep 0.4
check "esc 关闭下拉，WHERE 输入框还在" eval '[[ -z $(pop) ]] && running'

# ---- 跑过一次 WHERE 之后再按 C-r：列出那一条，不再是「没有历史」
run "status = 'done'"
edit; key C-r; sleep 0.6
check "跑过 status = 'done' 之后 C-r：列出那一条，没有「没有历史」" eval 'items | grep -q "^status = .done." && ! items | grep -q "没有历史" || { items; false; }'
e2e_done
