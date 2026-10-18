#!/usr/bin/env bash
# F17.12 / F17.13 exec 连接的数据库层和 schema 勾选（specs/m17-polish-exec/task.md F17.12、F17.13；tech-design §7.8、§8.8）
# exec 连接的树是 连接 → 库（current_database() 那一个，默认展开）→ schema → Tables，query 标题 <库>.public；右键「选择 schema…」勾 schema 后
# connections.toml 里是 schemas = ["库.schema"]，假命令收到的 Tables 查询带 nspname in (…)，没勾的 schema 不列。假命令用 psql 在自建库里跑。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/cellops.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
DBN=sqlmux_e2e_$$
cat >"$D/fake.py" <<'PY'
#!/usr/bin/env python3
# a fake exec command (PG dialect): fake.py DSN LOG SQL -> a JSON array with one item for the statement, run by psql
import csv, io, json, subprocess, sys
dsn, log, sql = sys.argv[1], sys.argv[2], sys.argv[3]
with open(log, "a") as f: f.write(sql.replace("\n", " ") + "\n")
p = subprocess.run(["psql", dsn, "-X", "-q", "--csv", "-P", "null=\\N", "-c", sql], capture_output=True, text=True)
if p.returncode != 0:
    print(json.dumps([{"err": p.stderr.strip()}])); sys.exit(0)
rows = list(csv.reader(io.StringIO(p.stdout)))
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
CT() { echo "$E2E_TMP/config/sqlmux/connections.toml"; }
lg() { cat "$LOG"; }
side() { e2e_plain | noicon | sed 's/^│//; s/│.*//; s/^ *//; s/ *$//'; }
row_of() { side | grep -nE "$1" | head -1 | cut -d: -f1; }
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }
list_rows() { local g y; g=($(box)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y" | sed 's/^ *//; s/ *$//'; done; }
list_title() { local g; g=($(box)); [[ -n ${g[0]} ]] || return 0; e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "${g[1]}" | sed 's/[┌─┐ ]//g'; }
menu_pick() { rclick "$1" "$2"; sleep 0.6; menu_click "$3"; sleep 0.9; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
qtitle() { e2e_plain | sed -n 1p | noicon | grep -oE "exe  ?· [^ ]+ ▾" | head -1; }

e2e_start -C "$D/c" "$E2E_BIN exe"; wait_for 8 flag_is alternate_on 1; sleep 5
check "F17.12：树是 连接 exe → 库 ${DBN}（默认展开）→ schema agentable、public → Tables" eval 'side | grep -qE "^▾ exe *$" && side | grep -qE "^▾ ${DBN} *$" && side | grep -qE "^▸ agentable" && side | grep -qE "^▾ public" && side | grep -qE "^▾ Tables"' || { side | head -8; false; }
check "query 的标题是 exe · ${DBN}.public（库.schema）" eval 'qtitle | grep -qF "${DBN}.public"' || { e2e_plain | sed -n 1p | cut -c60-170; false; }
open_table t_user; wait_for 10 settled
check "默认库的表 tab 标题不带库名（pane ① 的标题是 t_user，不是 ${DBN}.public.t_user）" eval 't=$(e2e_plain | sed -n 1p | noicon | grep -oE "① [^┐]*"); [[ $t == "① t_user ─"* && $t != *"$DBN"* ]] || { echo "[$t]"; false; }'
y=$(row_of "^▾ exe")
menu_pick 8 "$y" "选择库…"
check "连接节点右键「选择库…」：列表里就一个库（命令连到的那个，固定勾选）：1/1，[x] ${DBN}" eval 'list_title | grep -q "库" && list_rows | head -1 | grep -qE "1/1" && list_rows | grep -qE "^\[x\] ${DBN}$"' || { list_title; list_rows; false; }
key Escape; sleep 0.5

# ---- 库节点右键「选择 schema…」：勾掉 agentable
y=$(row_of "^▾ ${DBN}")
menu_pick 10 "$y" "选择 schema…"
check "库节点右键「选择 schema…」：agentable、public 两行，2/2" eval 'list_title | grep -q "schema" && list_rows | head -1 | grep -qE "2/2" && list_rows | grep -qE "^\[x\] agentable" && list_rows | grep -qE "^\[x\] public"' || { list_title; list_rows; false; }
n0=$(lg | wc -l)
key Space; sleep 0.3; key Escape; sleep 3
check "写回 connections.toml：schemas = [\"${DBN}.public\"]，别的字段原样" eval 'grep -qF "schemas = [\"${DBN}.public\"]" "$(CT)" && grep -q "engine = \"exec\"" "$(CT)" && grep -q "dialect = \"postgres\"" "$(CT)" && grep -q "command = " "$(CT)" || { cat "$(CT)"; false; }'
check "树上：agentable 不列了，库节点尾注 1/2" eval '! side | grep -qE "^[▸▾] agentable" && side | grep -qE "^▾ ${DBN} +1/2$"' || { side | head -8; false; }
check "重载 catalog 之后假命令收到的 Tables 查询带 nspname in (…public…)，没有 agentable" eval 'lg | tail -n +$((n0 + 1)) | grep -qE "nspname in \\(.*public" && ! lg | tail -n +$((n0 + 1)) | grep -q "agentable"' || { lg | tail -n +$((n0 + 1)) | cut -c1-260 | head -6; false; }
quit_app
e2e_done
