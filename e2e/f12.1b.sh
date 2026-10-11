#!/usr/bin/env bash
# F12.1 多列排序（MySQL）（specs/m12-grid/task.md F12.1）：同 f12.1.sh，在 MySQL 8.4 的自建库上。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/ordering.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
e2e_mysql "sqlmux_e2e_$$" <<<"create table t_ord (id int primary key, source varchar(8), n int); insert into t_ord values (1,'b',10),(2,'c',20),(3,'a',30),(4,'b',40),(5,'c',50),(6,'a',60),(7,'b',70),(8,'c',80),(9,'a',90),(10,'b',100),(11,'c',110),(12,'a',120)"
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
order_body
e2e_done
