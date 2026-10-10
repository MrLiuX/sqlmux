#!/usr/bin/env bash
# F10.10 跨 session 删 console 不能回滚别人的事务（specs/m10-feedback3/task.md F10.10；tech-design §7.8「Consoles 节点」）
# session b 里有两个 console 在同一个连接上，其中一个 begin + insert 还没提交；在 session doraemon 的树里删 b 开着的、不是最后一个的 console：
# 不再问、不回滚，b 里的事务还在（状态栏事务中，服务端 idle in transaction）；删最后一个 console 时才再问一次，n 取消后事务还在。自建库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_tx (id int primary key)" >/dev/null
APP=e2e-f1010b-$$
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s&application_name=%s"\n' "$E2E_DB" "$APP" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
SELECT=#364a82
sname() { bar | sed 's/^ //; s/ ▾.*//'; }
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.6; }
clr() { local i; for ((i = 0; i < 30; i++)); do e2e_keys BSpace; done; sleep 0.2; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.4; }
run() { key Enter; done_run; }
txstate() { bar | grep -oE '事务中|事务出错'; }
idle_tx() { psql "$E2E_DB" -At -c "select count(*) from pg_stat_activity where application_name = '$APP' and datname = current_database() and state like 'idle in transaction%'"; }
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=$SELECT* ]] && { e2e_text 1 32 "$y" | python3 -c "import re, sys; print(re.sub(r'^[│ ▸▾-]+', '', sys.stdin.read().strip()).rstrip('│ '))"; return; }; done; }
tree_focus() { local i; for ((i = 0; i < 6; i++)); do [[ $(focused) == 0 ]] && return 0; key C-h; done; }
tree_to() { local i; tree_focus; key G; for ((i = 0; i < 60; i++)); do [[ $(tsel) == "$1"* ]] && return 0; key k; done; return 1; }
box() { e2e_plain | grep -oE '删除 [^？]*？' | head -1; }
tx_box() { e2e_plain | grep -q "事务"; }

SOLO= start -C "$D/own"
# ---- session b：两个 console，一个开着事务
leader s; key n; sleep 0.4; clr; e2e_type b; key Enter; wait_for 10 eval '[[ $(sname) == b ]]'; sleep 1
key c; sleep 1
setsql "begin"; run
setsql "insert into t_tx values (1)"; run
check "（准备）session b 的 console 里 begin + insert：状态栏事务中，服务端有一条 idle in transaction" eval '[[ $(txstate) == 事务中 && $(idle_tx) == 1 ]] || { echo "  tx=$(txstate) idle=$(idle_tx)"; false; }'
e2e_keys Space; e2e_type '"'; sleep 0.6; key c; sleep 1
check "（准备）b 里再分一个 pane、新建第二个 console（同一个连接）" eval '[[ $(e2e_plain | grep -c "console_[0-9]") -ge 2 ]]'

# ---- 回到 doraemon，在树里删 b 里第二个 console（不是最后一个）
leader s; key k; key Enter; wait_for 6 eval '[[ $(sname) == doraemon ]]'; sleep 0.8
tree_to console_3
check "（准备）doraemon 的树里 Consoles 下选中 console_3（b 里开着的第二个）" eval '[[ $(tsel) == console_3* ]]'
key d; key d; sleep 0.5
check "dd：弹确认「删除 console_3？」" eval '[[ $(box) == "删除 console_3？" ]]'
key y; sleep 1
check "y：不再问事务（它不是 b 那条连接上最后一个 console），没有「回滚」的提示" eval '! tx_box && ! e2e_plain | grep -q "回滚"'
check "服务端 b 的事务还在（idle in transaction 仍是 1），t_tx 里还看不到那一行" eval '[[ $(idle_tx) == 1 && $(psql "$E2E_DB" -At -c "select count(*) from t_tx") == 0 ]] || { echo "  idle=$(idle_tx)"; false; }'
leader s; key j; key Enter; wait_for 6 eval '[[ $(sname) == b ]]'; sleep 0.8
check "切回 b：状态栏还是事务中；console_3 的 tab 关了，console_2 还在" eval '[[ $(txstate) == 事务中 ]] && ! e2e_plain | grep -q "console_3" && e2e_plain | grep -q "console_2"'

# ---- 删最后一个 console：再问一次事务，n 取消后事务还在
leader s; key k; key Enter; wait_for 6 eval '[[ $(sname) == doraemon ]]'; sleep 0.8
tree_to console_2; key d; key d; sleep 0.4; key y; sleep 0.8
check "删 b 里最后一个 console（连接在事务里）：y 之后再问一次，提到事务" eval 'tx_box'
key n; sleep 0.8
check "n：取消，b 的事务还在（idle in transaction 1），console_2 还在树里" eval '[[ $(idle_tx) == 1 ]] && e2e_plain | cut -c1-33 | grep -q "console_2"'
e2e_done
