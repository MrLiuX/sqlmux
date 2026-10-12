#!/usr/bin/env bash
# F14.1b SQLite 自己的规矩（specs/m14-sqlite/task.md F14.1 复审；tech-design §8.1「SQLite」）
# REAL 显示最短往返、DATE / DATETIME / TIMESTAMP 按标准文本显示（T 和 Z 丢掉）、blob 行标识整表只读并给原因、可空的主键不算行标识（退回 rowid，多一列只读 rowid）、
# INT PRIMARY KEY 不是 rowid 的别名、WITHOUT ROWID 的主键是 NOT NULL 不用 rowid。seed 里的 F14.1 复审那几张表。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
lite() { sqlite3 "$DB" "$1"; }
# the grid's header row and data rows as cells (the sidebar left out): hdr "rowid k v", cellv ROW N (N-th column, 0-based, the row number column not counted)
gcells() { e2e_plain | sed -n "$1p" | python3 -c "
import sys
l = sys.stdin.read().rstrip(\"\\n\")[33:]
cs = [c.strip() for c in l.split(\"│\")]
print(*[c for c in cs[1:] if True][:-1] if cs and cs[-1] == \"\" else cs[1:], sep=\"|\")"; }
edit() { key Enter; e2e_type "$1"; sleep 0.3; }
hdr() { gcells "$(hy)" | LC_ALL=C tr -cd ' -~|' | tr '|' ' ' | sed 's/  */ /g; s/^ *//; s/ *$//'; }   # the column names, space apart
cellv() { gcells "$(( $(grid_y) + $1 ))" | awk -F'|' -v n="$(( $2 + 2 ))" '{ print $n }'; }   # ROW N: row ROW's N-th column (0-based)
e2e_start "$E2E_BIN $DB"; wait_for 5 flag_is alternate_on 1; sleep 1; solo
open_table t_real; wait_for 8 settled
check "REAL 按最短往返显示：1/7 是 0.14285714285714285，0.1+0.2 是 0.30000000000000004，123456789.123456789 是 123456789.12345679，100.0 是 100.0" eval '[[ $(cellv 1 1) == 0.14285714285714285 && $(cellv 2 1) == 0.30000000000000004 && $(cellv 5 1) == 123456789.12345679 && $(cellv 7 1) == 100.0 ]]' || e2e_plain | sed -n 4,12p | cut -c33-100
open_table t_dt; wait_for 8 settled
check "DATE / DATETIME / TIMESTAMP 声明的列：标准文本 YYYY-MM-DD HH:MM:SS[.SSS]（不带时区），2026-01-02 是 2026-01-02 00:00:00，.250 保留，T 和 Z 的写法显示时丢掉" eval '[[ $(cellv 1 1) == "2026-01-02 03:04:05" && $(cellv 1 2) == "2026-01-02 00:00:00" && $(cellv 1 3) == "2026-01-02 03:04:05.250" && $(cellv 2 1) == "2026-01-02 03:04:05" && $(cellv 2 2) == "<null>" ]]' || e2e_plain | sed -n 4,8p | cut -c33-140
open_table t_bk; wait_for 8 settled
key l; key Enter; sleep 0.6
check "行标识里有 blob 列（t_bk 的 id 是 blob(16)）：整表只读，给原因「t_bk 的行标识列 id 是 blob(16)，只读」，进不了编辑" eval 'screen_has "行标识列 id 是 blob(16)，只读" && mode_is NORMAL'
open_table t_free; wait_for 8 settled
check "可空的主键（k primary key，没声明类型也没 NOT NULL）不算行标识：最前面多一列只读 rowid，排序按 rowid" eval '[[ $(hdr) == "rowid k v" ]] && qb | grep -qF "ORDER rowid"' || hdr
key j; key l l; edit TWO2; key Enter; sleep 0.3; key C-s; sleep 1.2; wait_for 5 settled
check "按 rowid 改 k = two 那行的 v：只有那一行变了（k 是 1 / two / 0 的混合类型也不会错行）" eval '[[ $(lite "select group_concat(v) from (select v from t_free order by rowid)") == "one,TWO2,zero" ]]' || lite "select rowid, * from t_free"
open_table t_free_wr; wait_for 8 settled
check "WITHOUT ROWID 的 t_free_wr：主键 k 是 NOT NULL，不用 rowid（表头是 k v，没有 rowid 列）" eval '[[ $(hdr) == "k v" ]]' || hdr
open_table t_int; wait_for 8 settled
check "id INT PRIMARY KEY 不是 rowid 的别名（INT 不是 INTEGER）：可空，退回 rowid，多一列 rowid" eval '[[ $(hdr) == "rowid id v" ]]' || hdr
e2e_done
