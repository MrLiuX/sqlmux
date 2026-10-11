#!/usr/bin/env bash
# F12.4 导出的数据安全（PG）（specs/m12-grid/task.md F12.4；tech-design §7.6「导出」）
# 手动事务里导出：「全部」走 Main，能导出没提交的行；成功、C-c 取消之后事务还在（idle in transaction）、未提交的行还在；大表 C-c 之后服务端没有残留语句。自建库 t_x（1203 行）、t_huge（200 万行）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/cellops.sh"
. "$(dirname "$0")/exporting.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q >/dev/null <<'SQL'
create table t_x (id int primary key, name text);
insert into t_x select g, 'n' || g from generate_series(1, 1203) g;
create table t_huge (id int primary key, v text);
insert into t_huge select g, md5(g::text) || md5((g + 1)::text) || md5((g + 2)::text) || md5((g + 3)::text) from generate_series(1, 2000000) g;
SQL
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
idle_tx() { psql "$E2E_DB" -At -c "select count(*) from pg_stat_activity where datname = current_database() and state like 'idle in transaction%'"; }
active_huge() { psql "$E2E_DB" -At -c "select count(*) from pg_stat_activity where datname = current_database() and state = 'active' and query ilike '%t_huge%' and pid <> pg_backend_pid()"; }
seen_tx_row() { psql "$E2E_DB" -At -c "select count(*) from t_x where id = 99999"; }

start -x 200 -C "$D/own"; sleep 1.5
open_table t_x; wait_for 8 settled
key C-l
e2e_keys C-p; sleep 0.3; e2e_type ">手动提交事务"; sleep 0.6; key Enter; sleep 0.6
setsql "insert into t_x values (99999, 'uncommitted')"; run
key C-h; sleep 0.3
check "（准备）手动事务：一行 99999 插进去还没提交（别的会话看不到），服务端有一个 idle in transaction" eval '[[ $(seen_tx_row) == 0 && $(idle_tx) == 1 ]] || { echo "  other sees $(seen_tx_row), idle $(idle_tx)"; false; }'

# ---- 手动事务里导出「全部」：走 Main，能看见没提交的行
export_pick 3; exported; sleep 0.5
check "JSON · 全部：文件里有 1204 个对象，包括没提交的 99999（走的是事务里的那条连接）" eval 'py "$E2E_TMP/t_x.json" <<PY
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if len(d) == 1204 and any(o["id"] == 99999 for o in d) else 1)
PY'
check "导出之后事务还在：服务端仍是 1 个 idle in transaction，别的会话仍看不到 99999" eval '[[ $(idle_tx) == 1 && $(seen_tx_row) == 0 ]] || { echo "  idle $(idle_tx), other sees $(seen_tx_row)"; false; }'

# ---- 大表 C-c：半截文件删掉，事务还在，没有残留语句
open_table t_huge; wait_for 20 settled
export_pick 1; sleep 0.5
e2e_keys C-c; sleep 2
check "导出 t_huge（200 万行）时 C-c：toast「导出已取消」，目录里没有 t_huge.csv" eval 'toast_has "导出已取消" && [[ $(fcount "^t_huge") == 0 ]] || { echo "  files: $(ls "$E2E_TMP" | tr "\n" " ")"; false; }'
check "服务端没有残留的 t_huge 语句（pg_stat_activity 里没有 active 的）" eval '[[ $(active_huge) == 0 ]] || { echo "  active: $(active_huge)"; false; }'
check "取消之后事务还在：仍是 1 个 idle in transaction，99999 还没提交" eval '[[ $(idle_tx) == 1 && $(seen_tx_row) == 0 ]] || { echo "  idle $(idle_tx), other sees $(seen_tx_row)"; false; }'
key C-l; sleep 0.3      # 回到 query：事务里的这一行还能读到
setsql "select count(*) as n from t_x where id = 99999"; run
check "同一个会话里 select count(*) 看到 99999（未提交的行没有被取消的导出带走）" eval 'rrow 3 | grep -qE "(^| )1( |$)"' || rrow 3
# ---- 成功的「全部」之后（再导出一次）事务也还在
key C-h; sleep 0.3; open_table t_x; wait_for 8 settled
sleep 3.2
export_pick 1; exported; sleep 0.5
check "再导出一次 CSV · 全部（t_x.csv，1205 行 = 表头 + 1204）：事务仍在，99999 仍未提交" eval '[[ $(wc -l <"$E2E_TMP/t_x.csv" | tr -d " ") == 1205 && $(idle_tx) == 1 && $(seen_tx_row) == 0 ]] || { echo "  lines $(wc -l <"$E2E_TMP/t_x.csv"), idle $(idle_tx), other $(seen_tx_row)"; false; }'
e2e_done
