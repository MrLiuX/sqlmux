#!/usr/bin/env bash
# F10.3 order_cols（specs/m10-feedback3/task.md F10.3；tech-design §7.8「COLS」、§14）：COLS 里调过的顺序写进布局文件的 order_cols，
# 用 --layout 读回来之后表格和 COLS 列表还是这个顺序。只读，用共用库的 t_order。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/keep" "$D/c"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/c/connections.toml")
header() { e2e_text 35 159 "$(hy)" | python3 -c "import re, sys; print(' '.join(re.findall(r'[a-z_]+', sys.stdin.read())[:${N:-3}]))"; }
save_layout() { e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; for i in $(seq 20); do e2e_keys BSpace; done; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "$1"; key Enter; sleep 0.6; }
dd() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }'; }
dd_names() { local g y; g=($(dd)); for ((y = g[1] + 3; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/ *$//'; done | grep '^\[' | sed 's/^\[.\] //; s/ .*//' | tr '\n' ' '; }

e2e_start -C "$D/c" -S "$D/st" -D "$D/data" "$E2E_BIN"; wait_for 6 flag_is alternate_on 1; sleep 1.2; open_table t_order; wait_for 8 settled
key gc; key j; key j; key K; key j; key j; key J; key Escape; sleep 0.4     # status 上移到第 2 位，再把 amount 下移一位：id status user_id paid amount …
check "（准备）COLS 里调了顺序：表头前 4 列 id status user_id paid？不是就按实际的记下来" eval 'h=$(N=5 header); echo "  header: $h"; [[ $h == id\ status\ user_id* ]]'
ORDER=$(N=6 header)
save_layout o
check "layout.save o：布局文件里有 order_cols，前面是 id、status、user_id" eval 'grep -q "^ *order_cols" "$E2E_TMP/config/sqlmux/layouts/o.toml" && grep "^ *order_cols" "$E2E_TMP/config/sqlmux/layouts/o.toml" | grep -q "\"id\", \"status\", \"user_id\"" || { grep -n "order_cols" "$E2E_TMP/config/sqlmux/layouts/o.toml"; false; }'
cp -R "$E2E_TMP/config/sqlmux/layouts" "$D/keep/"
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2

cp -R "$D/keep/layouts" "$D/c/"
e2e_start -C "$D/c" -S "$D/st2" -D "$D/data2" "$E2E_BIN --layout o"; wait_for 6 flag_is alternate_on 1; sleep 2; wait_for 8 settled
check "--layout o：表格的列顺序是保存时的（$ORDER）" eval 'h=$(N=6 header); [[ $h == "$ORDER" ]] || { echo "  [$h] want [$ORDER]"; false; }'
key gc
check "COLS 列表也是这个顺序，光标在第一列" eval '[[ $(dd_names) == id\ status\ user_id* ]] || { echo "  [$(dd_names)]"; false; }'
e2e_done
