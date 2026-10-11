#!/usr/bin/env bash
# F12.2 值编辑器（PG）（specs/m12-grid/task.md F12.2；tech-design §10「值编辑器」、§6.4 value）
# 表格上 e 开居中浮窗：JSON 格式化、gq、:w / :wq / C-s 写回、:q 有改动先问、只读（只读连接、结果 tab）改动的键 toast、? 搜索、空格 / C-p / C-l 不跳出。自建库 t_val。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/valeditor.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_val (id int primary key, doc jsonb, note text)" -c "insert into t_val values (1, '{\"b\":1,\"a\":[1,2],\"s\":\"hi\",\"t\":true,\"n\":null}', 'plain text'), (2, null, '{\"x\":1}')" >/dev/null
psql "$E2E_DB" -q -c "create table t_nopk (doc jsonb)" -c "insert into t_nopk values ('{\"a\":1}')" >/dev/null
JT=jsonb
doc1() { psql "$E2E_DB" -At -c "select doc from t_val where id = 1"; }
mkdir -p "$D/own" "$D/ro"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\nread_only = true\n' "$E2E_DB" >"$D/ro/connections.toml"; chmod 600 "$D/ro/connections.toml"
value_body
_e2e_kill
ro_body
e2e_done
