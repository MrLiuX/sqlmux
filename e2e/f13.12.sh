#!/usr/bin/env bash
# F13.12 布局文件里不认识的 split 不再丢掉 pane（specs/m13-quality/task.md F13.12；tech-design §14）
# 布局里 split = "H"：toast「不认识的 split \"H\"，按 h」，两个 pane 和它们的 tab 都还在（以前整个 pane 连同 tab 静默丢掉）。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/c/connections.toml")
start -C "$D/c" -S "$D/st" -D "$D/data"; open_table t_order; wait_for 8 settled
e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; for i in $(seq 20); do e2e_keys BSpace; done; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "lay"; key Enter; sleep 0.6
cp -R "$E2E_TMP/config/sqlmux/layouts" "$D/c/"
sed 's/split = "h"/split = "H"/; s/split = "v"/split = "V"/' "$D/c/layouts/lay.toml" >"$D/c/layouts/bad.toml"
check "（准备）改坏的布局文件里 split = \"H\"" eval 'grep -q "split = \"H\"" "$D/c/layouts/bad.toml"'
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5

e2e_start -C "$D/c" -S "$D/st2" -D "$D/data2" "$E2E_BIN --layout bad"; wait_for 6 flag_is alternate_on 1; sleep 1.2
check "split = \"H\"：启动照常（没有退出），toast「不认识的 split \"H\"，按 h」" eval 'running && screen_has "不认识的 split \"H\""'
check "两个 pane 都在：① 里是 t_order，② 里是 query_1（没有被静默丢掉）" eval '[[ $(tabbar 1) == *t_order* && $(tabbar 2) == *query_1* ]]'
e2e_done
