#!/usr/bin/env bash
# F12.4 导出 CSV / JSON（PG）（specs/m12-grid/task.md F12.4；tech-design §7.6「导出」、§11）
# ge 开下拉：CSV / JSON × 当前页 / 全部；文件写在当前目录，已存在加 -2；隐藏的列不导出，WHERE / ORDER 照旧；结果 tab 的「全部」去掉自动 LIMIT；C-c 取消删掉半截文件。自建库 t_x（1203 行）、t_huge（200 万行）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/ordering.sh"
. "$(dirname "$0")/cellops.sh"
. "$(dirname "$0")/exporting.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q >/dev/null <<'SQL'
create table t_x (id int primary key, name text, qty numeric(10,2), tag text, ts timestamptz);
insert into t_x select g, 'n,' || g || '"q"', g * 1.5, case when g % 3 = 0 then null else 't' end, now() from generate_series(1, 1203) g;
create table t_huge (id int primary key, v text);
insert into t_huge select g, md5(g::text) || md5((g + 1)::text) || md5((g + 2)::text) || md5((g + 3)::text) from generate_series(1, 2000000) g;
SQL
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
export_body
e2e_done
