#!/usr/bin/env bash
# F17.11 exec：命令截断的结果（specs/m17-polish-exec/task.md F17.11；tech-design §8.8）
# 命令一次最多回 1000 行，只有第一批带 truncated: true：sqlmux 自己发的语句（catalog、分页、计数）由 exec 的 Conn 包成
# select * from (<sql>) as sqlmux_page limit 1000 offset <已收行数> 接着翻、拼成一份，页不满或空页才停（整倍数时多一次空页往返）；
# 用户的语句（快速 SQL、控制台）不翻，只标已截断：快速 SQL 只跑一次命令。假命令用 psql 在自建库里跑，每批最多 1000 行，调用记在日志里。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_x (id int primary key, name text)" -c "insert into t_x select i, 'n' || i from generate_series(1, 1200) i"
cat >"$D/fake.py" <<'PY'
#!/usr/bin/env python3
# a fake exec command (PG dialect) that answers at most 1000 rows a run: fake.py DSN LOG SQL; only a run that is not a page of sqlmux's own says truncated
import csv, io, json, subprocess, sys
dsn, log, sql = sys.argv[1], sys.argv[2], sys.argv[3]
with open(log, "a") as f: f.write(sql.replace("\n", " ") + "\n")
p = subprocess.run(["psql", dsn, "-X", "-q", "--csv", "-P", "null=\\N", "-c", sql], capture_output=True, text=True)
if p.returncode != 0:
    print(json.dumps([{"err": p.stderr.strip()}])); sys.exit(0)
rows = list(csv.reader(io.StringIO(p.stdout)))
cols, data = rows[0], [[None if v == "\\N" else v for v in r] for r in rows[1:]]
cut = len(data) > 1000
item = {"columns": cols, "rows": data[:1000], "n": min(len(data), 1000)}
if cut and "sqlmux_page" not in sql: item["truncated"] = True
print(json.dumps([item]))
PY
LOG=$D/calls.log; : >"$LOG"
mkdir -p "$D/c"
(umask 077; cat >"$D/c/connections.toml" <<TOML
[[connection]]
name = "exe"
engine = "exec"
dialect = "postgres"
command = ["python3", "$D/fake.py", "$E2E_DB", "$LOG", "{sql}"]
TOML
)
lg() { cat "$LOG"; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
side() { e2e_plain | noicon | sed 's/^│//; s/│.*//; s/^ *//; s/ *$//'; }
tables_n() { side | grep -oE "^▾ +Tables \([0-9]+\)" | head -1 | grep -oE "[0-9]+"; }   # the number on the Tables node of public
real_n() { psql "$E2E_DB" -At -c "select count(*) from pg_class c join pg_namespace n on n.oid = c.relnamespace where n.nspname = 'public' and c.relkind in ('r', 'p') and not c.relispartition"; }
all_n() { psql "$E2E_DB" -At -c "select count(*) from pg_class c join pg_namespace n on n.oid = c.relnamespace where c.relkind in ('r', 'p', 'v', 'm', 'f') and not c.relispartition and n.nspname !~ '^pg_' and n.nspname <> 'information_schema'"; }
runq() { e2e_click 110 6; sleep 0.3; key Escape; key g g; key d G; key i; e2e_type "$1"; key Escape; key Enter; sleep "${2:-2}"; }
start_app() { : >"$LOG"; e2e_start -C "$D/c" "$E2E_BIN exe"; wait_for 8 flag_is alternate_on 1; sleep "${1:-6}"; }

# ---- 4000 张表：Tables 查询按 schema 排序，前 1000 行放不下所有表和后面的 schema；树里要全部列出
{ echo "create schema zz_last; create table zz_last.t_end (a int);"; for i in $(seq 1 4000); do echo "create table public.g$(printf %04d $i) (a int);"; done; } >"$D/many.sql"
psql "$E2E_DB" -q -f "$D/many.sql"
echo "# total catalog rows now: $(all_n), public tables $(real_n)"
start_app 12
check "4000 多张表：public 的 Tables 节点上的数字就是真数（$(real_n)），不是被截的 1000 以内" eval '[[ $(tables_n) == "$(real_n)" ]] || { echo "  tree says $(tables_n), real $(real_n)"; false; }' 
pal "@t_end"; sleep 1
check "排在最后的 schema 里的表 zz_last.t_end（在前 1000 行之外）面板里找得到" eval 'list | cut -d"|" -f1 | grep -q "^t_end  .*zz_last"' || { list | head -4; false; }
key Escape; sleep 0.3
check "日志里有翻页的语句：select * from (…) as sqlmux_page limit 1000 offset 1000" eval 'lg | grep -qF "as sqlmux_page limit 1000 offset 1000"' || { lg | cut -c1-150 | tail -5; false; }
quit_app
_e2e_kill

# ---- 整倍数：catalog 正好 2000 行，全在，只多一次空页往返
drop_n=$(( $(all_n) - 2000 ))
python3 - "$E2E_DB" "$drop_n" <<'PY'
import subprocess, sys
dsn, n = sys.argv[1], int(sys.argv[2])
sql = "".join("drop table public.g%04d;" % i for i in range(4000, 4000 - n, -1))
subprocess.run(["psql", dsn, "-q", "-c", sql], check=True)
PY
echo "# total catalog rows now: $(all_n)"
start_app 12
check "整倍数（catalog 正好 2000 行）：Tables 节点上的数字就是真数（$(real_n)）" eval '[[ $(all_n) == 2000 && $(tables_n) == "$(real_n)" ]] || { echo "  all $(all_n) tree $(tables_n) real $(real_n)"; false; }'
check "日志里翻到了 offset 2000 的那一页（空页，一次多余的往返），之后没有 offset 3000" eval 'lg | grep -qF "as sqlmux_page limit 1000 offset 2000" && ! lg | grep -qF "as sqlmux_page limit 1000 offset 3000"' || { lg | grep -o "limit 1000 offset [0-9]*" | sort | uniq -c; false; }
quit_app
_e2e_kill

# ---- 快速 SQL：只跑一次命令，提示已截断；控制台里同一条：也只一次，截断提示照旧
start_app 5
n0=$(lg | wc -l)
pal ";select * from t_x"; sleep 1; [[ -n $(pop) ]] && { key Escape; sleep 0.3; }; key Enter; sleep 3
check "快速 SQL 查 1200 行的 t_x：命令只收到这一条语句一次（不翻页），面板标题是「N+ 行」（已截断）" eval 'c=$(lg | tail -n +$((n0 + 1)) | grep -cF "select * from t_x"); [[ $c == 1 ]] && ! lg | tail -n +$((n0 + 1)) | grep -q sqlmux_page && title | grep -qE "^[0-9]+\+ 行"' || { lg | tail -n +$((n0 + 1)) | cut -c1-150; title; false; }
key Escape; sleep 0.4
n1=$(lg | wc -l)
runq "select * from t_x" 4
check "控制台（query）里同一条：也只跑一次、不翻页，结果标成已截断（标题带 +）" eval 'c=$(lg | tail -n +$((n1 + 1)) | grep -cF "select * from t_x"); [[ $c == 1 ]] && ! lg | tail -n +$((n1 + 1)) | grep -q sqlmux_page && e2e_plain | grep -qE "[0-9]+\+ 行"' || { lg | tail -n +$((n1 + 1)) | cut -c1-150; e2e_plain | grep -a "行" | head -3 | cut -c40-170; false; }
quit_app
e2e_done
