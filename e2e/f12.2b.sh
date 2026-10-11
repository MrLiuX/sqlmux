#!/usr/bin/env bash
# F12.2 值编辑器（MySQL）（specs/m12-grid/task.md F12.2）：同 f12.2.sh，json 列在 MySQL 8.4 的自建库上（标题 doc · json）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/valeditor.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
e2e_mysql "sqlmux_e2e_$$" <<<"create table t_val (id int primary key, doc json, note text); insert into t_val values (1, '{\"b\":1,\"a\":[1,2],\"s\":\"hi\",\"t\":true,\"n\":null}', 'plain text'), (2, null, '{\"x\":1}')"
JT=json
doc1() { e2e_mysql "sqlmux_e2e_$$" <<<"select doc from t_val where id = 1"; }
mkdir -p "$D/own" "$D/ro"; printf '[[connection]]\nname = "doraemon"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
printf '[[connection]]\nname = "doraemon"\nengine = "mysql"\ndsn = "%s"\nread_only = true\n' "$E2E_MYDB" >"$D/ro/connections.toml"; chmod 600 "$D/ro/connections.toml"
value_body
_e2e_kill
ro_body
e2e_done
