#!/usr/bin/env bash
# F12.4 导出 CSV / JSON（MySQL）（specs/m12-grid/task.md F12.4）：同 f12.4.sh，在 MySQL 8.4 的自建库上。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/ordering.sh"
. "$(dirname "$0")/cellops.sh"
. "$(dirname "$0")/exporting.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
e2e_mysql "sqlmux_e2e_$$" <<<"create table t_x (id int primary key, name varchar(40), qty decimal(10,2), tag varchar(8), ts datetime); set session cte_max_recursion_depth = 3000000; insert into t_x with recursive s(n) as (select 1 union all select n + 1 from s where n < 1203) select n, concat('n,', n, '\"q\"'), n * 1.5, if(n % 3 = 0, null, 't'), now() from s; create table t_huge (id int primary key, v varchar(140)); insert into t_huge with recursive s(n) as (select 1 union all select n + 1 from s where n < 2000000) select n, concat(md5(n), md5(n + 1), md5(n + 2), md5(n + 3)) from s"
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
export_body
e2e_done
