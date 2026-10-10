#!/usr/bin/env bash
# F10.4 query 结果翻页（MySQL）（specs/m10-feedback3/task.md F10.4；tech-design §11「结果翻页」）：同 f10.4.sh，在 MySQL 8.4 的自建库上。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/paging.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
e2e_mysql "sqlmux_e2e_$$" <<<"create table t_page (id int primary key); set session cte_max_recursion_depth = 5000; insert into t_page with recursive s(n) as (select 1 union all select n + 1 from s where n < 1203) select n from s"
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
paging_body
e2e_done
