#!/usr/bin/env bash
# F17.7 外部执行器引擎，PG 方言（specs/m17-polish-exec/task.md F17.7；tech-design §8.8、§14）
# engine = "exec"、dialect = "postgres"、command = [argv 模板，{sql} 被整条语句替换]：每条语句一次子进程，stdout 是 JSON（一条语句一项：columns / rows / n / err），
# 退出码非 0 时 stderr 原文就是错误；树、翻页、query 都走它，catalog / 分页 / 引号是 postgres 的；只读（写被挡）；取消 = 杀子进程；
# 连接时跑一次 select current_database()，失败时 stderr 原文是错误；command 里没有 {sql} 启动时报错。
# 假命令（fk/fakepg.py 的拷贝）用 psql 在自建的库里跑语句，把结果转成约定的 JSON，每次调用的语句记在日志里。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/cellops.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_x (id int primary key, name text)" -c "insert into t_x select i, 'n' || i from generate_series(1, 1200) i" -c "create table t_y (a int)" -c "insert into t_y values (1), (null)"
cat >"$D/fake.py" <<'PY'
#!/usr/bin/env python3
# a fake exec command (PG dialect): fake.py DSN LOG SQL -> a JSON array with one item for the statement, run by psql
import csv, io, json, subprocess, sys
dsn, log, sql = sys.argv[1], sys.argv[2], sys.argv[3]
with open(log, "a") as f: f.write(sql.replace("\n", " ") + "\n")
if "FAILME" in sql:
    sys.stderr.write("boom: no panel is open\n"); sys.exit(3)
if "SLEEPY" in sql:
    import time; time.sleep(30)
if "ITEMERR" in sql:
    print(json.dumps([{"err": "ERROR: item says no"}])); sys.exit(0)
p = subprocess.run(["psql", dsn, "-X", "-q", "--csv", "-P", "null=\\N", "-c", sql], capture_output=True, text=True)
if p.returncode != 0:
    print(json.dumps([{"err": p.stderr.strip()}])); sys.exit(0)
rows = list(csv.reader(io.StringIO(p.stdout)))
if not rows:
    print(json.dumps([{"columns": [], "rows": [], "n": 0}])); sys.exit(0)
cols, data = rows[0], [[None if v == "\\N" else v for v in r] for r in rows[1:]]
print(json.dumps([{"columns": cols, "rows": data, "n": len(data)}]))
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
runq() { e2e_click 110 6; sleep 0.3; key Escape; key g g; key d G; key i; e2e_type "$1"; key Escape; key Enter; sleep "${2:-2}"; }
has() { e2e_plain | grep -qF -- "$1"; }
side() { e2e_plain | noicon | sed 's/^│//; s/│.*//; s/^ *//; s/ *$//'; }

e2e_start -C "$D/c" "$E2E_BIN exe"; wait_for 8 flag_is alternate_on 1; sleep 4
check "连接时先跑一次 select current_database()（假命令的日志第一条）" eval '[[ $(lg | head -1) == "select current_database()"* ]] || { lg | head -2; false; }'
check "树：exe → public → Tables 里有 t_x、t_y（catalog 是 postgres 的，经命令取来）" eval 'side | grep -qE "^▸ t_x( |$)" && side | grep -qE "^▸ t_y( |$)" && side | grep -qE "^▾ public" || { side | head -14; false; }'
check "树里没有「别的库」：只有 exe 一个连接下的 schema（agentable、public），没有 postgres / sqlmux 这些库节点" eval '! side | grep -qE "^[▸▾] +(postgres|sqlmux)( |$)" || { side | head -30; false; }'
open_table t_x; wait_for 10 settled; sleep 1
check "打开 t_x：1200 行分 12 页，PAGE 1/12，第一行 1 n1" eval 'e2e_plain | grep -q "PAGE 1/12" && e2e_plain | grep -qE "│ +1 │ +1 │ n1 " || { e2e_plain | sed -n 1,8p | cut -c30-150; false; }'
check "日志里有分页的两条语句：select … order by \"id\" limit 101 offset 0 和 select count(*)" eval 'lg | grep -qF "select * from \"public\".\"t_x\" order by \"id\" limit 101 offset 0" && lg | grep -qF "select count(*) from \"public\".\"t_x\"" || { lg | tail -3; false; }'
key "]"; wait_for 10 settled; sleep 1
check "] 翻页：PAGE 2/12，第一行 101 n101；日志里是 offset 100" eval 'e2e_plain | grep -q "PAGE 2/12" && e2e_plain | grep -qE "│ +101 │ +101 │ n101 " && lg | grep -qF "limit 101 offset 100"' || { lg | tail -2; e2e_plain | sed -n 1,8p | cut -c30-150; false; }
key /; e2e_type "id > 1195"; key Enter; wait_for 10 settled; sleep 1
check "WHERE id > 1195：5 行，PAGE 1/1，日志里 where id > 1195" eval 'e2e_plain | grep -q "PAGE 1/1" && e2e_plain | grep -qE "│ +5 │ +1200 │ n1200 " && lg | grep -qF "where ( id > 1195 )"' || { lg | tail -2; e2e_plain | sed -n 1,10p | cut -c30-150; false; }

# ---- query 里跑：结果、psql 的错误（item 的 err）、命令失败（stderr 原文）、item 里直接给的 err
runq "select count(*) from t_x"
check "query：select count(*) from t_x → 1200" eval 'e2e_plain | grep -qE "│ 1200 " || { e2e_plain | tail -12 | cut -c60-170; false; }'
runq "select * from nope"
check "语句出错（item 的 err = psql 的报错）：错误原文显示出来（relation \"nope\" does not exist）" eval 'has "relation \"nope\" does not exist" || { e2e_plain | tail -8 | cut -c60-200; false; }'
runq "select 'FAILME'"
check "命令退出码非 0：stderr 原文就是错误（boom: no panel is open）" eval 'has "boom: no panel is open" || { e2e_plain | tail -8 | cut -c60-200; false; }'
runq "select 'ITEMERR'"
check "输出里的 item 带 err：原文显示（ERROR: item says no）" eval 'has "item says no" || { e2e_plain | tail -8 | cut -c60-200; false; }'
runq "select null::int as n, 'x' as t"
check "NULL 是 NULL（JSON null，画成 <null>）" eval 'has "<null>" || { e2e_plain | tail -8 | cut -c60-200; false; }'

# ---- 取消 = 杀子进程
runq "select 'SLEEPY'" 1.5
check "（准备）长查询在跑：假命令的子进程在" eval 'ps -axo command | grep -aq "[f]ake.py.*SLEEPY" || { ps -axo command | grep -a SLEEPY | cut -c1-300 | head -3; lg | tail -2; e2e_plain | sed -n 1,8p | cut -c60-175; false; }'
key C-c; sleep 1.5
check "C-c：取消，子进程被杀（ps 里没有 fake.py … SLEEPY 了）" eval '! ps -axo command | grep -aq "[f]ake.py.*SLEEPY" || { ps -axo command | grep -a SLEEPY | head -3; false; }'
runq "select 7 as seven"
check "取消之后接着跑：select 7 → 7（连接还能用，每次都是新进程）" eval 'e2e_plain | grep -qE "│ +7 +│"'
# ---- 只读：写语句被挡，命令一次都没收到
n0=$(lg | wc -l)
runq "update t_x set name = 'z' where id = 1"
check "update 被挡（exe 是只读连接），库没动，假命令没有收到这条语句" eval 'has "exe 是只读连接" && [[ $(psql "$E2E_DB" -At -c "select name from t_x where id = 1") == n1 ]] && ! lg | grep -qi "update t_x"' || { lg | tail -2; e2e_plain | tail -6 | cut -c60-200; false; }
runq "insert into t_y values (9)"
check "insert 也被挡，t_y 还是 2 行" eval '[[ $(psql "$E2E_DB" -At -c "select count(*) from t_y") == 2 ]] && ! lg | grep -qi "insert into t_y"'
check "整个会话里假命令收到的语句：没有一条 insert / update / delete / create / drop / alter" eval '! lg | grep -qiE "^ *(insert|update|delete|create|drop|alter|truncate) " || { lg | grep -iE "^ *(insert|update|delete|create|drop|alter|truncate) "; false; }'
open_table t_y; wait_for 10 settled; sleep 0.5
key l; key Enter; sleep 0.5
check "表格里格子进不了编辑：toast「exe 是只读连接」" eval 'mode_is NORMAL && has "exe 是只读连接" || { e2e_plain | tail -4 | cut -c1-160; false; }'

quit_app
_e2e_kill

# ---- 连接失败：stderr 原文就是错误
(umask 077; cat >"$D/c/connections.toml" <<TOML
[[connection]]
name = "bad"
engine = "exec"
dialect = "postgres"
command = ["python3", "$D/fake.py", "$E2E_DB", "$LOG", "{sql}"]

[[connection]]
name = "bad2"
engine = "exec"
dialect = "postgres"
command = ["sh", "-c", "echo 'panel closed: open pgAdmin first' >&2; exit 1", "{sql}"]
TOML
)
e2e_start -C "$D/c" "$E2E_BIN bad2"; wait_for 8 flag_is alternate_on 1; sleep 1
check "连接时命令失败：stderr 原文（panel closed: open pgAdmin first）就是错误，显示出来" eval 'has "panel closed: open pgAdmin first" || { e2e_plain | head -12 | cut -c1-150; false; }'
quit_app
_e2e_kill
(umask 077; cat >"$D/c/connections.toml" <<TOML
[[connection]]
name = "noph"
engine = "exec"
dialect = "postgres"
command = ["echo", "hi"]
TOML
)
e2e_start -C "$D/c" "$E2E_BIN noph"; sleep 3
check "command 里没有 {sql}：报错（说要有 {sql}），不连" eval 'e2e_plain | tr -d " \n" | grep -qF "要有{sql}" || { e2e_plain | head -8 | cut -c1-150; false; }'
e2e_done
