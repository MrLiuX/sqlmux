#!/usr/bin/env bash
# F8.19 断线重连，PG（specs/m8-feedback/task.md F8.19；tech-design §8.2「断线重连」）
# 判定、重拨、重跑的分档由 db 层单测和集成测试覆盖；这里在真实终端里用 pg_terminate_backend 杀掉服务端的连接，等价于 tsh 代理断开：
# 表格 R 刷新自动恢复（用户看不到错误、树节点不变红）、console 只读语句自动重跑、写语句不重跑（错误栏 + toast「已重连，请重新执行」）、
# 手动事务里被杀（toast「事务已丢失」，模式不变）、重连失败（节点变红、toast）、数据库恢复后再请求就回来。全在自建库上做。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'psql "$SQLMUX_TEST_PG" -qAt -c "alter database sqlmux_e2e_$$ allow_connections true" >/dev/null 2>&1; e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
APP=e2e-f819-$$ ERROR=#f7768e WARN=#e0af68 DB=sqlmux_e2e_$$
mkdir -p "$D/c"
(umask 077; printf '[[connection]]\nname = "rw"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB&application_name=$APP" >"$D/c/connections.toml")

psql_n() { psql "$E2E_DB" -At -c "$1"; }
admin() { psql "$SQLMUX_TEST_PG" -qAt -c "$1"; }                       # 连在共用库上（自建库不让连时也能改它）
conns() { admin "select count(*) from pg_stat_activity where application_name = '$APP' and datname = '$DB'"; }
kill_all() { admin "select count(pg_terminate_backend(pid)) from pg_stat_activity where application_name = '$APP' and datname = '$DB'" >/dev/null
  wait_for 5 eval '[[ $(conns) == 0 ]]'; }                             # 等服务端真的把它们都关了
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.4; }
run() { key Enter; done_run; }
rval() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }
ctitle() { local g; g=($(geom 2)); e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "${g[1]}"; }
toast_row() { e2e_text 1 "$(e2e_flag pane_width)" $(($(H) - 1)); }
toast_text() { toast_row | sed 's/[ │└┘─┌┐]//g'; }
trow() { local y; for y in $(seq 3 43); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }
node_red() { local y c; y=$(trow rw); [[ -n $y ]] || return 1; c=$(e2e_find rw "$y" | cut -d' ' -f1); [[ " $(e2e_style "$c" "$y") " == *" fg=$ERROR "* ]]; }
txstate() { bar | grep -oE '事务中|事务出错'; }
log_has() { local g y; g=($(geom 3)); for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "$y"; done | grep -qF -- "$1"; }

start -x 200 -C "$D/c"; e2e_record "$D/rec"; open_table t_user; wait_for 8 settled
check "（准备）t_user 打开，rw 有 Meta、Main 两条连接，树节点不是红的" eval '[[ $(conns) == 2 ]] && ! node_red'
CNT=$(psql_n "select count(*) from t_user")

# ---- 表格：杀掉连接后 R 刷新，数据回来，没有错误
kill_all
key R; wait_for 8 settled; sleep 1
check "杀掉两条连接后在表格上按 R：数据回来（行数不变），没有错误栏" eval '[[ $(e2e_plain | grep -cE "│ +[0-9]+ │ +[0-9]+ │ user_") -ge 5 && -z $(errbar 1 | head -1) ]] || { echo "  err: $(errbar 1 | head -1)"; false; }'
check "没有 toast，树节点不变红，Meta 已经重连（库里有 1 条连接）" eval '[[ -z $(toast_text) ]] && ! node_red && (( $(conns) >= 1 ))'

# ---- console：只读语句自动重跑
key C-l; setsql "select count(*) from t_user"
kill_all; run; sleep 0.8
check "杀掉连接后 console 执行 select：自动重连重跑，结果是 ${CNT}，没有错误栏、没有 toast" eval '[[ $(rval) == "$CNT" && -z $(errbar 2 | head -1) && -z $(toast_text) ]] || { echo "  rval=[$(rval)] err=[$(errbar 2 | head -1)] toast=[$(toast_row)]"; false; }'

# ---- console：写语句不重跑
setsql "update t_user set name = 'zz819' where id = 1"
kill_all; : >|"$D/rec"; run
check "toast「rw：已重连，请重新执行」（只显示 3 秒，看录下来的输出，不靠截屏的时机）" grep -qF "rw：已重连，请重新执行" "$D/rec"
check "杀掉连接后 console 执行 update：不重跑，错误栏有原来的错误" eval '[[ -n $(errbar 2 | head -1) ]] || { echo "  no error bar"; false; }'
check "那条 update 没有执行（库里 id 1 的 name 不是 zz819），树节点不变红" eval '[[ $(psql_n "select name from t_user where id = 1") != zz819 ]] && ! node_red'
run
check "再按一次执行：成功（Main 已经重连），库里 id 1 的 name 是 zz819，错误栏没了" eval '[[ $(psql_n "select name from t_user where id = 1") == zz819 && -z $(errbar 2 | head -1) ]]'

# ---- 手动事务里被杀：事务丢失，模式不变
g=($(geom 2)); x=$(e2e_find "Tx:" "${g[1]}" | tr ' ' '\n' | awk -v l="${g[0]}" '$1 > l { print; exit }'); e2e_click "$x" "${g[1]}"; sleep 0.4; e2e_type 手动; sleep 0.3; key Enter
setsql "update t_user set name = 'tx819' where id = 2"; run
check "（准备）手动模式：update 在事务里没提交，状态栏「事务中」" eval '[[ $(txstate) == 事务中 && $(psql_n "select name from t_user where id = 2") != tx819 ]]'
kill_all; : >|"$D/rec"
setsql "select 1 as x"; run; sleep 0.8
check "事务里连接被杀、再执行一条 select：toast「rw：连接断开，事务已丢失」（看录下来的输出）" grep -qF "rw：连接断开，事务已丢失" "$D/rec"
check "模式还是手动（标题 Tx: 手动），丢掉的事务没有提交（id 2 的 name 不是 tx819）" eval '[[ $(ctitle) == *"Tx: 手动"* && $(psql_n "select name from t_user where id = 2") != tx819 ]] || { echo "  title=[$(ctitle)]"; false; }'
check "手动模式里重跑的 select 前面补了 BEGIN：状态栏又是「事务中」，服务端只有这一个新事务（idle in transaction 1 条）" eval '[[ $(txstate) == 事务中 && $(psql_n "select count(*) from pg_stat_activity where datname = current_database() and state like '"'idle in transaction%'"'") == 1 ]]'

# ---- 重连失败：库不让连了。节点变红、toast；库恢复后再请求就回来
admin "alter database $DB allow_connections false"
kill_all; : >|"$D/rec"
key C-h; key R
check "toast 带连接名和错误（rw：…，看录下来的输出）" eval 'sleep 1; grep -qF "rw：" "$D/rec" || { echo "  rec: $(tail -c 300 "$D/rec" | iconv -c -f utf-8 -t utf-8)"; false; }'
check "库不让连、杀掉连接后按 R：树节点 rw 变红" eval 'node_red'
check "tab 都留着（t_user 还在，console 还在）" eval '[[ $(e2e_plain | head -1) == *" t_user ─"* ]] && [[ $(tabbar 2) == *console_1* ]]'
admin "alter database $DB allow_connections true"
key R; wait_for 10 settled; sleep 1
check "库恢复后再按 R：数据回来，树节点不再红，库里有连接" eval '! node_red && [[ $(e2e_plain | grep -cE "│ +[0-9]+ │ +[0-9]+ │ user_") -ge 5 ]] && (( $(conns) >= 1 ))'

e2e_done
