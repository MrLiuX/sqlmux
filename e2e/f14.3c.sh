#!/usr/bin/env bash
# F14.3 树上的对象（SQLite）（specs/m14-sqlite/task.md F14.3；tech-design §7.8「对象节点」、§8.4）：同 f14.3.sh，在 seed 建的文件库上。
# 没有 Functions / Sequences 节点；外键没有名字（FOREIGN KEY (a) → t (b)）；autoindex 不列；索引 ↵ 是 sqlite_schema 里的那条 create index。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
mkdir -p "$D/own"; printf '[tree]\nwidth = 70\n' >"$D/own/config.toml"
tree() { e2e_plain | python3 -c "import sys; [print(l[:72].rstrip()) for l in list(sys.stdin)[:${1:-40}]]"; }
trow() { e2e_plain | python3 -c "
import sys
for i,l in enumerate(sys.stdin, 1):
    if sys.argv[1] in l[:72]: print(i); break" "$1"; }
tclick() { local y x; y=$(trow "$1"); [[ -n $y ]] || { echo "  no tree row '$1'"; return 1; }; x=$(e2e_plain | sed -n "${y}p" | python3 -c "import sys; l=sys.stdin.read(); print(max(l.find('▸'), l.find('▾')) + 1)"); e2e_click "$x" "$y"; sleep 0.5; }   # the arrow of the row that has TEXT
tleaf() { local y; y=$(trow "$1"); e2e_click 12 "$y"; sleep 0.6; }                                                                                    # the row's text: a leaf's ↵ is the click
thas() { tree 60 | grep -qF -- "$1"; }

title1() { e2e_plain | sed -n 1p | python3 -c "import sys; l = sys.stdin.read(); i = l.find('┌─ ①'); print(l[i:i + 40].strip() if i >= 0 else '')"; }
title1() { e2e_plain | sed -n 1p | python3 -c "import sys; l = sys.stdin.read(); i = l.find('┌─ ①'); print(l[i:i + 40].strip() if i >= 0 else '')"; }
title1() { e2e_plain | sed -n 1p | python3 -c "import sys; l = sys.stdin.read(); i = l.find('┌─ ①'); print(l[i:i + 40].strip() if i >= 0 else '')"; }
e2e_start -C "$D/own" "$E2E_BIN $DB"; wait_for 5 flag_is alternate_on 1; sleep 1; solo
check "SQLite 没有 Functions / Sequences 节点，也没有 schema 层" eval '! thas "Functions" && ! thas "Sequences" && ! e2e_plain | grep -qE "[▾▸] .*(main|public)"'
tclick t_order_item; sleep 0.5; tclick Keys
check "t_order_item：Indexes (0)（SQLite 的 autoindex 不列）；Keys：PRIMARY (order_id, line_no)、外键没有名字 FOREIGN KEY (order_id) → t_order (id)" eval 'thas "Indexes (0)" && thas "PRIMARY (order_id, line_no)" && thas "FOREIGN KEY (order_id) → t_order (id)"'
tleaf "FOREIGN KEY ("; sleep 1; wait_for 5 settled
check "外键 ↵：打开目标表 t_order" eval '[[ $(title1) == *" t_order "* ]]' || title1
tclick t_order_item; sleep 0.3; tclick t_sku; sleep 0.5; tclick Indexes
check "t_sku：Indexes (3)，有 t_sku_code (code) UNIQUE、t_sku_barcode (barcode) UNIQUE；Keys (0)" eval 'thas "Indexes (3)" && thas "t_sku_code (code) UNIQUE" && thas "t_sku_barcode (barcode) UNIQUE" && thas "Keys (0)"'
tleaf "t_sku_code ("; sleep 0.8
check "索引 ↵：DDL 浮窗是 sqlite_schema 里的 create index（CREATE UNIQUE INDEX t_sku_code on t_sku (code)）" eval 'screen_has "CREATE UNIQUE INDEX t_sku_code on t_sku (code)"' || e2e_plain | sed -n 1,12p | cut -c1-160
e2e_done
