#!/usr/bin/env bash
# F12.3 按值过滤、表头排序、表格右键菜单（MySQL）（specs/m12-grid/task.md F12.3）：同 f12.3.sh，在 MySQL 8.4 的自建库上（标识符用反引号）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/ordering.sh"
. "$(dirname "$0")/cellops.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
e2e_mysql "sqlmux_e2e_$$" <<<"create table t_f (id int primary key, name varchar(20), qty int, tag varchar(20)); insert into t_f values (1,'ann',30,'x'),(2,'bob',10,null),(3,'o''neil',20,'x'),(4,'ann',40,'y'),(5,'cat',10,null)"
e2e_mysql "sqlmux_e2e_$$" <<<"create table t_u (id int primary key, doc json, bl blob); insert into t_u values (1, '{\"a\":1}', X'00FF')"
QUOTE='`'
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
cell_body
e2e_done
