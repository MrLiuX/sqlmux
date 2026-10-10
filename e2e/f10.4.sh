#!/usr/bin/env bash
# F10.4 console 结果默认 500 行并翻页（PG）（specs/m10-feedback3/task.md F10.4；tech-design §11「结果翻页」、§9.4）
# 1203 行的表：第 1 页 500+ 行，] 翻到 501–1000+，再 ] 到 1001–1203，最后一页 ] 无事，[ 往回翻，R 回第 1 页，带 LIMIT 的语句没有翻页。
# SQL 的拼法（LIMIT 501 OFFSET 500）和三页 id 连续在集成测试里；这里看真实终端里的按键、标题和翻页后的内容。自建库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/paging.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_page as select g as id from generate_series(1, 1203) g" >/dev/null
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
paging_body
e2e_done
