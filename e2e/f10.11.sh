#!/usr/bin/env bash
# F10.11 console 改名叫 query（specs/m10-feedback3/task.md F10.11；tech-design §11、§14）
# 纯改名；这里测迁移和旧配置：只有旧 consoles/ 目录的数据目录启动后 queries/query_<n>.sql 在（改过名的文件不动）、state.json 的键是新名字、树里 Queries 节点列出它们、
# 打开能读到原来的内容；queries/ 已存在就不动 consoles/；旧的 [keys.console] 报「没有这个表」、[console] max_rows 不再生效、[query] max_rows 生效；sqlmux keys 里没有 console。
# 其余脚本里的 console 字样已经全部改成 query，在各自的功能里测。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkcfg() { rm -rf "$D/c"; mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/c/connections.toml"); [[ -n $1 ]] && printf '%s\n' "$1" >"$D/c/config.toml"; true; }
qfiles() { ls "$1" 2>/dev/null | tr '\n' ' '; }
sidebar() { local y; for y in $(seq 3 43); do e2e_text 1 32 "$y"; done | python3 -c "import re, sys; print(re.sub(r'[-]', '', sys.stdin.read()))"; }

# ---- 旧数据目录：只有 consoles/，一个改过名的文件，state.json 里是旧键
mkcfg ""
mkdir -p "$D/data/sqlmux/consoles" "$D/st/sqlmux"
printf 'select 2 as two' >"$D/data/sqlmux/consoles/console_2.sql"; printf 'select 9 as nine' >"$D/data/sqlmux/consoles/console_9.sql"; printf 'select 1 as mine' >"$D/data/sqlmux/consoles/mine.sql"; printf 'keep me' >"$D/data/sqlmux/consoles/notes.txt"
printf '{"consoles":{"console_2":{"conn":"doraemon","schema":"public"},"mine":{"conn":"doraemon","schema":"public"}}}' >"$D/st/sqlmux/state.json"
SOLO= start -C "$D/c" -D "$D/data" -S "$D/st"; sleep 1.5
DDIR=$D/data/sqlmux
check "迁移：consoles/ 没了，queries/ 里是 mine.sql（改过名的不动）、notes.txt（非 .sql 的不动）、query_2.sql、query_9.sql" eval '[[ ! -d $DDIR/consoles && $(qfiles "$DDIR/queries") == "mine.sql notes.txt query_2.sql query_9.sql " ]] || { echo "  $(ls $DDIR) / $(qfiles $DDIR/queries)"; false; }'
check "文件内容原样" eval '[[ $(cat "$DDIR/queries/query_2.sql") == "select 2 as two" && $(cat "$DDIR/queries/mine.sql") == "select 1 as mine" ]]'
check "state.json：键 consoles 变成 queries，query_2 记着连接，没有旧键" eval 'python3 - "$D/st/sqlmux/state.json" <<PY
import json, sys
d = json.load(open(sys.argv[1]))
ok = "queries" in d and "consoles" not in d and d["queries"].get("query_2", {}).get("conn") == "doraemon" and "console_2" not in d["queries"] and "mine" in d["queries"]
sys.exit(0 if ok else 1)
PY' || cat "$D/st/sqlmux/state.json"
check "树里是 Queries 节点（不是 Consoles），列出 mine、query_2、query_9（query_1 是这次新开的）" eval 's=$(sidebar); [[ $s == *Queries* && $s != *Consoles* && $s == *query_2* && $s == *query_9* && $s == *mine* ]] || { echo "$s" | tail -12; false; }'
check "没有旧词：界面里（树、tab、标题）不再出现 console" eval '! e2e_plain | grep -qi console'
# 从树里打开迁移过来的 query_2：内容读得到
key C-h; sleep 0.3
for i in $(seq 1 40); do [[ $(sidebar | grep -c "query_2") -ge 1 ]] && break; sleep 0.1; done
y=$(for y in $(seq 3 43); do [[ $(e2e_text 1 32 "$y") == *" query_2"* ]] && echo $y; done | tail -1)
e2e_click 12 "$y"; sleep 0.3; key Enter; sleep 1
check "树里 query_2 ↵：打开，内容是 select 2 as two" eval 'e2e_plain | grep -q "select 2 as two"'
check "notes.txt 原样" eval '[[ $(cat "$DDIR/queries/notes.txt") == "keep me" ]]'
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; sleep 1.5; }
quit_app
check "退出后 state.json 里写的是 queries 键（query_2 的连接还在），没有 consoles" eval 'python3 - "$D/st/sqlmux/state.json" <<PY
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if "queries" in d and "consoles" not in d and d["queries"].get("query_2", {}).get("conn") == "doraemon" else 1)
PY' || cat "$D/st/sqlmux/state.json"
e2e_stop

# ---- queries/ 已存在：不动、不合并 consoles/
mkcfg ""; rm -rf "$D/data" "$D/st"; mkdir -p "$D/data/sqlmux/consoles" "$D/data/sqlmux/queries" "$D/st"
printf 'old' >"$D/data/sqlmux/consoles/console_5.sql"; printf 'new' >"$D/data/sqlmux/queries/query_1.sql"
SOLO= start -C "$D/c" -D "$D/data" -S "$D/st"; sleep 1.5
check "queries/ 已存在：consoles/console_5.sql 原地不动，queries/ 里还是只有 query_1.sql，内容不变" eval '[[ $(qfiles "$D/data/sqlmux/consoles") == "console_5.sql " && $(qfiles "$D/data/sqlmux/queries") == "query_1.sql " && $(cat "$D/data/sqlmux/queries/query_1.sql") == new ]]'
e2e_stop

# ---- 只有旧的 consoles 键的 state.json：query_1 的 schema 照旧，下次保存写成 queries
mkcfg ""; rm -rf "$D/data" "$D/st"; mkdir -p "$D/st/sqlmux"
printf '{"consoles":{"console_1":{"conn":"doraemon","schema":"agentable"}}}' >"$D/st/sqlmux/state.json"
SOLO= start -C "$D/c" -D "$D/data" -S "$D/st"; sleep 1.5
check "旧 state.json：query_1 用记下来的 schema agentable（标题里是 sqlmux.agentable）" eval 'e2e_plain | head -1 | grep -q "sqlmux.agentable"'
quit_app
check "退出后写成 queries 键，query_1 仍是 agentable" eval 'python3 - "$D/st/sqlmux/state.json" <<PY
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if "consoles" not in d and d.get("queries", {}).get("query_1", {}).get("schema") == "agentable" else 1)
PY' || cat "$D/st/sqlmux/state.json"
e2e_stop

# ---- --layout 指向旧布局文件（kind = "console"、name = "console_4"）：能打开 query_4
mkcfg ""; rm -rf "$D/data" "$D/st"
SOLO= start -C "$D/c" -D "$D/data" -S "$D/st"; sleep 1.5
e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; for i in $(seq 20); do e2e_keys BSpace; done; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type lay; key Enter; sleep 0.6
mkdir -p "$D/lay"; cp "$E2E_TMP/config/sqlmux/layouts/lay.toml" "$D/lay/lay.toml"
sed -i '' -e 's/kind = "query"/kind = "console"/' -e 's/name = "query_1"/name = "console_4"/' "$D/lay/lay.toml"
check "（准备）布局文件改成旧写法：kind = console、name = console_4" eval 'grep -q "kind = .console." "$D/lay/lay.toml" && grep -q "name = .console_4." "$D/lay/lay.toml"'
e2e_stop
mkcfg ""; rm -rf "$D/data" "$D/st"; mkdir -p "$D/c/layouts" "$D/data/sqlmux/consoles"; cp "$D/lay/lay.toml" "$D/c/layouts/lay.toml"
printf 'select 4 as four' >"$D/data/sqlmux/consoles/console_4.sql"
e2e_start -C "$D/c" -S "$D/st" -D "$D/data" "$E2E_BIN --layout lay"; wait_for 6 eval 'flag_is alternate_on 1 || exited'; sleep 2
check "--layout lay（旧写法）：能打开，tab 是 query_4，内容是 select 4 as four，文件迁移成 queries/query_4.sql" eval 'e2e_plain | grep -q "query_4" && e2e_plain | grep -q "select 4 as four" && [[ -f $D/data/sqlmux/queries/query_4.sql && ! -d $D/data/sqlmux/consoles ]] || { e2e_plain | head -8; false; }'
e2e_stop

# ---- 旧配置键：[keys.console] 报没有这个表，[query] 生效、[console] 不再生效
CFGH=$D/cfg; mkdir -p "$CFGH/sqlmux"
printf '[keys.console]\n"x" = "console.run"\n' >|"$CFGH/sqlmux/keymaps.toml"
out=$(XDG_CONFIG_HOME=$CFGH XDG_STATE_HOME=$D/s XDG_DATA_HOME=$D/d "$E2E_BIN" keys --check 2>&1); rc=$?
check "旧的 [keys.console]：keys --check 退出码 1，报 [keys.console] … no such table" eval '[[ $rc == 1 && $out == *"[keys.console]"*"no such table"* ]] || { echo "  rc=$rc $out"; false; }'
printf '[keys.query]\n"x" = "query.run"\n' >|"$CFGH/sqlmux/keymaps.toml"
out=$(XDG_CONFIG_HOME=$CFGH XDG_STATE_HOME=$D/s XDG_DATA_HOME=$D/d "$E2E_BIN" keys --check 2>&1); rc=$?
check "新的 [keys.query] + query.run：退出码 0" eval '[[ $rc == 0 ]] || { echo "  rc=$rc $out"; false; }'
rm -f "$CFGH/sqlmux/keymaps.toml"
out=$(XDG_CONFIG_HOME=$CFGH XDG_STATE_HOME=$D/s XDG_DATA_HOME=$D/d "$E2E_BIN" keys 2>&1)
check "sqlmux keys 的输出：有 query.new / query.run，一个 console 都没有" eval 'grep -q "query.new" <<<"$out" && grep -q "query.run" <<<"$out" && ! grep -qi console <<<"$out"'
mkcfg $'[console]\nmax_rows = 7'; rm -rf "$D/data" "$D/st"
SOLO= start -C "$D/c" -D "$D/data" -S "$D/st"; sleep 1.5; key C-l; key i; e2e_type "select * from t_order"; key Escape; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.5
check "旧的 [console] max_rows = 7 不再生效：第一页还是 500+ 行" eval 'e2e_plain | grep -q "500+ 行"'
e2e_stop
mkcfg $'[query]\nmax_rows = 7'; rm -rf "$D/data" "$D/st"
SOLO= start -C "$D/c" -D "$D/data" -S "$D/st"; sleep 1.5; key C-l; key i; e2e_type "select * from t_order"; key Escape; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.5
check "[query] max_rows = 7 生效：第一页 7+ 行" eval 'e2e_plain | grep -q "7+ 行"'
e2e_done
