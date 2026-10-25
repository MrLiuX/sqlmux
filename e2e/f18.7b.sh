#!/usr/bin/env bash
# F18.7 SQL Server（specs/m18-windows-mssql/task.md F18.7 a；tech-design §8.1「SQL Server」、§10）：f5.4（MySQL）的写那一半，在 SQL Server 2022 上：
# 编辑保存（decimal、bit、nvarchar 的 Unicode，别人改过时报错，排序规则不分大小写时只差大小写也查得出）、唯一索引做行标识、没有键的表只读、
# 事务（begin tran / rollback、手动模式下 ✓ 提交）。写库只在自建库 sqlmux_e2e_<pid>（lib.sh e2e_own_msdb）；共用的 sqlmux 库不动。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
[[ -n $SQLMUX_TEST_MSSQL ]] || { echo "e2e: SQLMUX_TEST_MSSQL is not set" >&2; exit 1; }
e2e_own_msdb || { echo "e2e: could not create the private SQL Server database" >&2; exit 1; }
OWN=sqlmux_e2e_$$
mkdir -p "$D/b"
(umask 077; printf '[[connection]]\nname = "ms"\nengine = "mssql"\ndsn = "%s"\n' "$E2E_MSDB" >"$D/b/connections.toml")

ms() { e2e_mssql "$OWN" <<<"$1"; }                                        # 在自建库上执行，输出不带表头，一行一条
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.4; }
run() { key Enter; done_run; }
rval() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }
click_title() { local g x; g=($(geom 2)); x=$(e2e_find "$1" "${g[1]}" | tr ' ' '\n' | awk -v l="${g[0]}" '$1 > l { print; exit }'); e2e_click "$x" "${g[1]}"; sleep 0.4; }
qbar() { local g; g=($(geom 1)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 2)); }
txstate() { bar | grep -oE '事务中|事务出错'; }
col_x() { e2e_find "$1" "$(hy)" | tr ' ' '\n' | awk '$1 > 34 { print; exit }'; }
row_y() { echo $(( $(grid_y) + $1 )); }
cell() { local x; x=$(col_x "$1"); e2e_text "$x" $((x + ${3:-10})) "$(row_y "$2")" | sed 's/ *│.*//; s/^ *//; s/ *$//'; }
goto() { key g g; key 0; [[ $2 -gt 1 ]] && key $(($2 - 1)) j; local n; n=$(e2e_text 35 $(( $(col_x "$1") - 1 )) $(hy) | tr -cd '│' | wc -m); n=$((n - 1)); [[ $n -gt 0 ]] && key "$n" l; true; }
saves() { local q s; s=$(printf '\xef\x83\x87'); q=$(qbar); q=${q#*"$s"}; q=${q%%[!\ 0-9]*}; echo $q; }

start -x 200 -C "$D/b"; sleep 2
N999=$(ms "select count(*) from t_order where amount = 9.99"); N299=$(ms "select count(*) from t_order where amount = 2.99")   # 这两个值在表里不止一行
open_table t_order; wait_for 8 settled
# ---- 编辑保存：decimal（别的行也有这个值：2.99 在表里不止一行）、bit、nvarchar
goto amount 2; key Enter; e2e_type 9.99; key Enter
goto paid 1; key Enter; e2e_type false; sleep 0.3; key Enter
check "第 2 行 amount 改 9.99、第 1 行 paid（bit）下拉选 false：两处修改" eval '[[ $(saves) == 2 && $(cell paid 1) == false* ]] || { echo "  saves $(saves) paid $(cell paid 1) / $(qbar)"; false; }'
key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* || -n $(errbar 1) ]]'; sleep 0.4
check "C-s：「已保存 2 行」，库里 id 2 的 amount 是 9.99、id 1 的 paid 是 0，只动了这两行（amount = 9.99 的行多一行、2.99 的少一行）" eval '[[ $(qbar) == *"已保存 2 行"* && $(ms "select cast(amount as varchar(20)) from t_order where id = 2") == 9.99 && $(ms "select cast(paid as varchar(2)) from t_order where id = 1") == 0 && $(ms "select count(*) from t_order where amount = 9.99") == $((N999 + 1)) && $(ms "select count(*) from t_order where amount = 2.99") == $((N299 - 1)) ]] || { echo "  $(qbar) / $(errbar 1) / amount2=$(ms "select cast(amount as varchar(20)) from t_order where id = 2") paid1=[$(ms "select cast(paid as varchar(2)) from t_order where id = 1")] n9.99=$(ms "select count(*) from t_order where amount = 9.99") n2.99=$(ms "select count(*) from t_order where amount = 2.99")"; false; }'
wait_for 5 eval '[[ -z $(saves) ]]'; sleep 0.5
goto status 3; key Enter; sleep 0.3; e2e_type queued; sleep 0.3; key Enter; key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* || -n $(errbar 1) ]]'; sleep 0.4
check "status（check 约束的 nvarchar，值在表里重复上千次）改一行：只改这一行（id 3），其余 failed 的行不动" eval '[[ $(qbar) == *"已保存 1 行"* && $(ms "select count(*) from t_order where status = '"'"'failed'"'"'") == 1499 && $(ms "select status from t_order where id = 3") == queued ]] || { echo "  $(qbar) / $(errbar 1) / failed rows: $(ms "select count(*) from t_order where status = '"'"'failed'"'"'")"; false; }'

# ---- nvarchar 的 Unicode 往返；再让别人改过一行，乐观校验报行已变化（值唯一的 t_user.name），库里还是别人改的
open_table t_user; wait_for 8 settled
goto name 4; key Enter; e2e_type "中文é"; key Enter; key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* || -n $(errbar 1) ]]'; sleep 0.4
check "nvarchar 的 Unicode 往返：name 改成「中文é」保存，库里读回来一样" eval '[[ $(qbar) == *"已保存 1 行"* && $(ms "select name from t_user where id = 4") == "中文é" ]] || { echo "  $(qbar) / $(errbar 1) / $(ms "select name from t_user where id = 4")"; false; }'

ms "update t_user set name = N'zz3' where id = 3"
goto name 3; key Enter; e2e_type x3; key Enter; key C-s; sleep 1
check "别人把 user_3 改成 zz3：保存报「id = 3 的行数据已变化或行不存在，已回滚」，库里还是 zz3" eval '[[ $(errbar 1) == "id = 3 的行数据已变化或行不存在，已回滚"* && $(ms "select name from t_user where id = 3") == zz3 ]] || { echo "  $(errbar 1) / $(ms "select name from t_user where id = 3")"; false; }'
# ---- 唯一索引做行标识（t_sku 没有主键，有 code 上的唯一索引）；没有键的表只读（t_log）
open_table t_sku; wait_for 8 settled
goto title 1; key Enter; e2e_type "First"; key Enter; key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* || -n $(errbar 1) ]]'; sleep 0.4
check "t_sku（没有主键，code 上有唯一索引）：改第 1 行 title，「已保存 1 行」，库里被改的是 code 最小的那一行，别的行没动" eval '[[ $(qbar) == *"已保存 1 行"* && $(ms "select count(*) from t_sku where title = '"'"'First'"'"'") == 1 && $(ms "select code from t_sku where title = '"'"'First'"'"'") == $(ms "select min(code) from t_sku") ]] || { echo "  $(qbar) / $(errbar 1)"; false; }'
open_table t_log; wait_for 8 settled
goto msg 1; key Enter; sleep 0.4
check "t_log（没有主键也没有唯一索引）：↵ 编辑被拦，toast「t_log 没有主键，也没有全部列都非空的唯一索引，只读」" eval 'e2e_plain | grep -qa "t_log 没有主键，也没有全部列都非空的唯一索引，只读"'
key Escape

# ---- 事务：begin transaction / rollback；手动模式下 ✓ 提交。SQL Server 的读会等写的锁，所以「没提交」用 lock_timeout 看：别的连接读被锁住
blocked() { ! e2e_mssql "$OWN" <<<"set lock_timeout 600
$1" >/dev/null 2>&1; }
key C-l
setsql "begin transaction"; run
check "query 里 begin transaction：状态栏「事务中」" eval '[[ $(txstate) == 事务中 ]]'
setsql "delete from t_log"; run
check "（准备）事务里删了 t_log：别的连接读 t_log 被锁住（没提交）" eval 'blocked "select count(*) from t_log"'
setsql "rollback"; run
check "rollback：事务块消失，t_log 还是 3 行" eval '[[ -z $(txstate) && $(ms "select count(*) from t_log") == 3 ]]'
click_title "Tx:"; e2e_type 手动; sleep 0.3; key Enter
setsql "update t_event set kind = N'manual' where id = 1"; run
check "手动模式下 update：没提交（别的连接读 t_event 被锁住），「事务中」" eval 'blocked "select kind from t_event where id = 1" && [[ $(txstate) == 事务中 ]]'
click_title "✓"; wait_for 5 eval '[[ -z $(txstate) ]]'
check "点 ✓ 提交：库里变成 manual，事务块消失" eval '[[ $(ms "select kind from t_event where id = 1") == manual && -z $(txstate) ]]'
e2e_done
