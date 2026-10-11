#!/usr/bin/env bash
# F12.1 多列排序（PG）（specs/m12-grid/task.md F12.1；tech-design §7.6「ORDER」、§14 布局文件的 order）
# go 开下拉，space 把光标所在列循环（追加 → 翻转 → 去掉），下拉前面标 1↓ 2↑，chip 显示整个列表，↵ 在别的列上清成单列，过滤框非空时 space 是字符；
# 布局文件的 order 是数组，旧的 order = "x" + desc = true 读成一列。自建库 t_ord（12 行）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/ordering.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_ord (id int primary key, source text, n int)" -c "insert into t_ord select g, (array['a','b','c'])[1 + g % 3], g * 10 from generate_series(1, 12) g" >/dev/null
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
order_body
e2e_done
