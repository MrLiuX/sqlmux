#!/usr/bin/env bash
# F17.13 MySQL 连接上的「选择 schema…」（specs/m17-polish-exec/task.md F17.13；tech-design §7.8「库 / schema 的勾选」）
# MySQL 的 schema 就是库：连接节点右键「选择 schema…」，DSN 里的库固定勾选；写回 connections.toml 的 schemas = ["名字"]（MySQL 写名字，不带库前缀），
# 树上没勾的 schema 不列、连接节点尾注 勾了的/总数；a 全选之后回到全部、文件里没有 schemas。自建的 MySQL 库做 DSN 里的那个。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/cellops.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
OWN=sqlmux_e2e_$$
mkdir -p "$D/c"; printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/c/connections.toml"; chmod 600 "$D/c/connections.toml"
CT() { echo "$E2E_TMP/config/sqlmux/connections.toml"; }
side() { e2e_plain | noicon | sed 's/^│//; s/│.*//; s/^ *//; s/ *$//'; }
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }
list_rows() { local g y; g=($(box)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y" | sed 's/^ *//; s/ *$//'; done; }
list_title() { local g; g=($(box)); [[ -n ${g[0]} ]] || return 0; e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "${g[1]}" | sed 's/[┌─┐ ]//g'; }
menu_pick() { rclick "$1" "$2"; sleep 0.6; menu_click "$3"; sleep 0.9; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }

e2e_start -C "$D/c" "$E2E_BIN my"; wait_for 8 flag_is alternate_on 1; sleep 3
total=$(printf 'select count(*) from information_schema.schemata where schema_name not in (%s)' "'information_schema','mysql','performance_schema','sys'" | e2e_mysql | tail -1)
check "（准备）树里：连接 my（没有尾注）→ schema agentable、sqlmux、${OWN}，自己的库展开" eval 'side | grep -qE "^▾ my *$" && side | grep -qE "^▸ agentable" && side | grep -qE "^▸ sqlmux *$" && side | grep -qE "^▾ ${OWN} *$"' || { side | head -8; false; }
y=$(side | grep -nE "^▾ my *$" | head -1 | cut -d: -f1)
menu_pick 8 "$y" "选择 schema…"
check "连接节点右键 → 选择 schema…（MySQL 是 schema 不是库）：标题「schema」，三个 schema 都 [x]，3/3" eval 'list_title | grep -q "schema" && list_rows | head -1 | grep -qE "3/3" && list_rows | grep -qE "^\[x\] agentable" && list_rows | grep -qE "^\[x\] sqlmux +[0-9]+$" && list_rows | grep -qE "^\[x\] ${OWN}"' || { list_title; list_rows; false; }
key Space; sleep 0.3
check "space 勾掉光标所在的 agentable：2/3，[ ] agentable，别的 [x]" eval 'list_rows | head -1 | grep -qE "2/3" && list_rows | grep -qE "^\[ \] agentable" && list_rows | grep -qE "^\[x\] sqlmux +[0-9]+$" && list_rows | grep -qE "^\[x\] ${OWN}"' || { list_rows; false; }
key Escape; sleep 2.5
check "写回：schemas = [\"sqlmux\", \"${OWN}\"]（MySQL 写名字，不带库前缀），别的字段原样" eval 'grep -qF "schemas = [\"sqlmux\", \"${OWN}\"]" "$(CT)" && grep -q "engine = \"mysql\"" "$(CT)" && grep -q "name = \"my\"" "$(CT)" && [[ $(grep -c "^\[\[connection\]\]" "$(CT)") == 1 ]] || { cat "$(CT)"; false; }'
check "树上：agentable 不列了，sqlmux 和自己的库还在，连接节点尾注 2/3" eval '! side | grep -qE "^[▸▾] agentable *$" && side | grep -qE "^▸ sqlmux *$" && side | grep -qE "^▾ ${OWN}" && side | grep -qE "^▾ my +2/3$"' || { side | head -8; false; }
y=$(side | grep -nE "^▾ my" | head -1 | cut -d: -f1)
menu_pick 8 "$y" "选择 schema…"
key a; sleep 0.3; key Escape; sleep 2.5
check "a 全选再 esc：回到全部：连接节点没有尾注，agentable、sqlmux 回来了，connections.toml 里没有 schemas 了" eval '! grep -q "schemas" "$(CT)" && ! side | grep -qE "^▾ my +[0-9]+/[0-9]+$" && side | grep -qE "^▸ agentable" && side | grep -qE "^▸ sqlmux *$"' || { cat "$(CT)"; side | head -8; false; }
# ---- A 全不选：DSN 里的库固定勾选（不会关在 0 个上）：1/3，写出来就那一个
y=$(side | grep -nE "^▾ my" | head -1 | cut -d: -f1)
menu_pick 8 "$y" "选择 schema…"
key A; sleep 0.3
check "A 全不选：DSN 里的库（${OWN}）固定勾选不掉：1/3，别的 [ ]（不会是 0/3）" eval 'list_rows | head -1 | grep -qE "1/3" && list_rows | grep -qE "^\[x\] ${OWN}" && list_rows | grep -qE "^\[ \] agentable" && list_rows | grep -qE "^\[ \] sqlmux +"' || { list_rows; false; }
key Escape; sleep 2.5
check "写回 schemas = [\"${OWN}\"]，树上只剩自己的库，连接节点尾注 1/3" eval 'grep -qF "schemas = [\"${OWN}\"]" "$(CT)" && ! side | grep -qE "^[▸▾] (agentable|sqlmux) *$" && side | grep -qE "^▾ ${OWN}" && side | grep -qE "^▾ my +1/3$"' || { cat "$(CT)"; side | head -8; false; }
quit_app
e2e_done
