#!/usr/bin/env bash
# F8.13 palette.table：直接打开命令面板的表范围（specs/m8-feedback/task.md F8.13；tech-design §12「命令」）
# palette.table 没有默认键，用户绑一个键就能一步进入「所有已连上的连接的全部表」的搜索。这里用 keymaps.toml 绑 F7，在真实终端里测：
# 面板打开、输入框里是 @、范围是「表」、候选含两个连接的表、输入表名后 ↵ 打开它。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/c"; (umask 077; cat >"$D/c/connections.toml" <<EOT
[[connection]]
name = "d1"
engine = "postgres"
dsn = "$SQLMUX_TEST_PG"

[[connection]]
name = "d2"
engine = "postgres"
dsn = "$SQLMUX_TEST_PG"
EOT
)
printf '[keys.global]\n"<F7>" = "palette.table"\n' >"$D/c/keymaps.toml"

start -C "$D/c"
key F7; sleep 0.6
check "F7（palette.table）：面板打开，输入框里是 @" eval 'input_is "@"'
check "范围标签「表 @」是当前范围" eval 'p=$(e2e_plain); grep -q "表 @" <<<"$p"'
check "只列表（所有行的类型标签都是「表」），有 t_order、t_user，d1 的在里面" eval 'list | grep -q "^t_order  d1\\.public" && list | grep -q "^t_user  d1" && ! list | awk -F"|" "{ print \\$1 }" | grep -qv "表\$"'
key Escape; sleep 0.3

# ---- d2 连上之后，两个连接的表都在
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=#364a82* ]] && { e2e_text 1 32 "$y"; return; }; done; }
tree_to() { local i; key G; for ((i = 0; i < 60; i++)); do [[ $(tsel) == *" $1 "* ]] && return; key k; done; }
key C-h; sleep 0.3; tree_to d2; key l; wait_for 10 eval 'e2e_plain | cut -c1-33 | grep -c "agentable" | grep -q 2'
check "（准备）d2 在树里展开，连上了" eval 'e2e_plain | cut -c1-33 | grep -c "agentable" | grep -q 2'
key F7; sleep 0.8; e2e_type "t_order"; sleep 0.6
check "palette.table 输入 t_order：候选里有 d1 和 d2 两个连接的 t_order（位置 d1.public / d2.public）" eval 'list | grep -q "^t_order  d1\.public" && list | grep -q "^t_order  d2\.public" || { list | head -20; false; }'
clear_input; e2e_type "@t_sku d2"; sleep 0.5
check "输入「t_sku d2」：第一项是 d2 的 t_sku" eval '[[ $(list | head -1) == "t_sku  d2.public"* ]] || { list | head -3; false; }'
key Enter; wait_for 8 eval '[[ -n $(grid_y) ]]'
check "↵：打开 d2 的 t_sku（pane 标题里是 t_sku）" eval 'e2e_plain | head -1 | grep -q "t_sku"'

e2e_done
