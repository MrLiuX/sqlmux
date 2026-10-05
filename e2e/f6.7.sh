#!/usr/bin/env bash
# F6.7 危险语句先确认（specs/m6-config/task.md F6.7；tech-design §13「危险语句先确认」）
# 危险判定的表驱动用例（CTE / 子查询里的 where、delete … using、MySQL delete … limit、alter table … drop）有单测；这里在真实终端和 PG 上测：
# console 执行没带 where 的 delete / update、truncate、drop 弹确认框，n 取消整次（不执行、不记日志）、y 执行；带 where 的不弹；一次多条只问一次；
# 表格保存含删除行时先确认，n 回到表格标记还在；只改格子的保存不问；confirm_dangerous = false 时都不弹。写库的部分在自己建的库里做。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
q() { psql "$E2E_DB" -At -c "$1"; }
q "create table t_del (id int primary key, v text); insert into t_del select g, 'r' || g from generate_series(1, 5) g; create table t_tmp (id int); create table t_tmp2 (id int)" >/dev/null
mkdir -p "$D/on" "$D/off"
(umask 077; for d in on off; do printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/$d/connections.toml"; done)
printf 'confirm_dangerous = false\n' >"$D/off/config.toml"
paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.5; }
asked() { e2e_plain | grep -q "危险语句"; }
box() { e2e_plain | grep -E '危险语句|^ *│ +[│ ]*(delete|update|truncate|drop)|│ +(delete|update|truncate|drop|alter)' | sed 's/^[^│]*│//; s/│.*//; s/^ *//; s/ *$//'; }
# run SQL：焦点在 console 里，换成这段 SQL（粘贴不触发补全），↵ 执行
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
run() { setsql "$1"; key Enter; sleep 0.8; }
has_result_pane() { [[ $(e2e_panes | awk '$1 != "-"' | grep -c .) == 4 ]]; }

start -C "$D/on"; key C-l
run "delete from t_log"
check "console 里执行 delete from t_log（没有 where）：弹确认框「这次执行有 1 条危险语句：」，列出这条，按钮 y 执行 / n 取消" eval 'asked && e2e_plain | grep -q "这次执行有 1 条危险语句：" && e2e_plain | grep -q "delete from t_log" && e2e_plain | grep -q "y 执行   n 取消"'
key n; sleep 0.5
check "n：整次不执行，表里还是 3 行，没有结果区（不算一次执行、不记日志）" eval '! asked && [[ $(q "select count(*) from t_log") == 3 ]] && ! has_result_pane'
key Enter; sleep 0.8
key y; done_run
check "再执行、y：执行了，表空了" eval '! asked && [[ $(q "select count(*) from t_log") == 0 ]]'
q "insert into t_log select now(), 'm' || g from generate_series(1, 3) g" >/dev/null

run "delete from t_log where msg = 'm1'"
check "带 where 的 delete 不弹，直接执行（少了一行）" eval '! asked && [[ $(q "select count(*) from t_log") == 2 ]]'
run "update t_del set v = 'x' where id = 1"
check "带 where 的 update 不弹" eval '! asked && [[ $(q "select v from t_del where id = 1") == x ]]'
run "update t_del set v = 'all'"
check "没有 where 的 update 弹" asked
key n; sleep 0.4
check "n 之后 v 没变" eval '[[ $(q "select count(*) from t_del where v = '"'all'"'") == 0 ]]'
run "truncate t_tmp"
check "truncate 弹" asked
key n; sleep 0.4
run "drop table t_tmp"
check "drop table 弹" asked
key n; sleep 0.4
check "n 之后表还在" eval '[[ $(q "select count(*) from pg_tables where tablename = '"'t_tmp'"'") == 1 ]]'
run "drop table t_tmp"
key y; done_run
check "y：drop 执行了，t_tmp 没了" eval '[[ $(q "select count(*) from pg_tables where tablename = '"'t_tmp'"'") == 0 ]]'

# ---- 一次多条：只问一次，列出所有危险的；n 取消整次（连不危险的也不执行）
TABS_BEFORE=$(tabbar 3)
setsql $'select 1;\ndelete from t_log;\nupdate t_del set v = \'z\';\ndrop table t_tmp2;'
key V; key G; key Enter; sleep 0.8
check "多条：只弹一次，「这次执行有 3 条危险语句：」，三条都列出，select 不列" eval 'asked && e2e_plain | grep -q "这次执行有 3 条危险语句：" && e2e_plain | grep -q "delete from t_log" && e2e_plain | grep -q "update t_del set" && e2e_plain | grep -q "drop table t_tmp2"'
key Escape; sleep 0.4
state() { echo "asked=$(asked && echo y || echo n) log=$(q 'select count(*) from t_log') tmp2=$(q "select count(*) from pg_tables where tablename = 't_tmp2'") panes=$(e2e_panes | awk '$1 != "-"' | grep -c .) tabs=$(tabbar 3)"; }
check "esc 取消整次：表和数据都没动，不危险的 select 也没有跑（结果区的 tab 栏不变）" eval '[[ $(state) == "asked=n log=2 tmp2=1 panes=4 tabs=$TABS_BEFORE" ]] || { echo "  $(state)"; false; }'

# ---- 表格：保存含删除行时先确认
key C-h; open_table t_del; wait_for 8 settled
key d; key d; key j; key d; key d
key C-s; sleep 0.6
check "dd 两行再 C-s：弹「将删除 2 行」确认，库里还没动" eval 'e2e_plain | grep -q "将删除 2 行" && [[ $(q "select count(*) from t_del") == 5 ]]'
key n; sleep 0.5
check "n：回到表格，库里还是 5 行，两处删除标记还在（保存按钮上计数 2）" eval '! e2e_plain | grep -q "将删除" && [[ $(q "select count(*) from t_del") == 5 ]] && e2e_text 36 160 3 | grep -qE "(^|[^0-9])2( |$)" || { e2e_text 36 160 3; false; }'
key C-s; sleep 0.5; key y; wait_for 8 eval '[[ $(q "select count(*) from t_del") == 3 ]]'
check "再 C-s、y：删了 2 行，库里 3 行" eval '[[ $(q "select count(*) from t_del") == 3 ]]'
# 只改格子的保存不问
key Escape; key l; key Enter; sleep 0.3; e2e_type "edited"; key Escape; sleep 0.3
key C-s; sleep 1.2
check "只改一个格子再 C-s：不弹确认，直接保存" eval '! e2e_plain | grep -q "将删除" && [[ $(q "select count(*) from t_del where v = '"'edited'"'") == 1 ]]'

# ---- confirm_dangerous = false：都不弹
start -C "$D/off"; key C-l
run "delete from t_log"
check "confirm_dangerous = false：delete from t_log 直接执行" eval '! asked && wait_for 6 eval "[[ \$(q \"select count(*) from t_log\") == 0 ]]"'
run "drop table t_tmp2"
check "drop table 也直接执行" eval '! asked && wait_for 6 eval "[[ \$(q \"select count(*) from pg_tables where tablename = '"'t_tmp2'"'\") == 0 ]]"'
key C-h; open_table t_del; wait_for 8 settled
key d; key d; key C-s; sleep 1.2
check "表格删除行再 C-s：也不弹，直接保存（库里 2 行）" eval '! e2e_plain | grep -q "将删除" && wait_for 6 eval "[[ \$(q \"select count(*) from t_del\") == 2 ]]"'

e2e_done
