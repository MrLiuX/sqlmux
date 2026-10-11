#!/usr/bin/env bash
# F12.3 按值过滤、表头排序、表格右键菜单（PG）（specs/m12-grid/task.md F12.3；tech-design §7.6）
# * 把光标格拼成条件追加进 WHERE（字符串、数字、NULL、带引号的值；已有条件 AND；不重复；进历史）；表头单击循环 / S-click / gs / gS；转置下不响应；
# 右键菜单（格、表头、结果 tab 的格）。自建库 t_f（5 行）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/ordering.sh"
. "$(dirname "$0")/cellops.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_f (id int primary key, name text, qty int, tag text)" -c "insert into t_f values (1,'ann',30,'x'),(2,'bob',10,null),(3,'o''neil',20,'x'),(4,'ann',40,'y'),(5,'cat',10,null)" >/dev/null
psql "$E2E_DB" -q -c "create table t_u (id int primary key, doc json, bl bytea)" -c "insert into t_u values (1, '{\"a\":1}', '\\x00ff')" >/dev/null
QUOTE='"'
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
cell_body
e2e_done
