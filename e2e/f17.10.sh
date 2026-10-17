#!/usr/bin/env bash
# F17.10 保存前预览（specs/m17-polish-exec/task.md F17.10；tech-design §13「保存前预览」、§10.3）
# C-s / :w 先弹出这次要执行的语句原文和「改 N 行、增 N 行、删 N 行」，y / ↵ 执行，n / esc 回表格、标记都还在；超过 20 条折叠「…另 N 条」；
# [grid] preview_save = false 关掉后 C-s 直接存；Redis 的保存列命令。SQLite 文件里做（不用后端），Redis 自己起 redis-server。
export E2E_PREVIEW_SAVE=1   # lib.sh turns the box off for the older scripts; here it is the default under test
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
sqlite3 "$DB" "create table t_many (id integer primary key, v text); with recursive n(i) as (select 1 union all select i + 1 from n where i < 30) insert into t_many select i, 'v' || i from n"
lite() { sqlite3 "$DB" "$1"; }
NL=$(printf '\n')
mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "shop"\nengine = "sqlite"\ndsn = "%s"\n' "$DB" >"$D/c/connections.toml")
edit() { key Enter; e2e_type "$1"; sleep 0.3; }
box_open() { e2e_plain | grep -q "y 保存   n 取消"; }
box_has() { e2e_plain | grep -qF -- "$1"; }
nstmt() { e2e_plain | grep -cE "(^|│) +(update|delete from|insert into) \"main\"\."; }       # the statements in the box, one line each start (long ones wrap)
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
Z_NAME="set \"name\" = 'zed'" Z_MAIL="set \"email\" = 'x@y'" Q_MAIL="'x@y'"
names() { lite "select group_concat(name) from (select name from t_user order by id)"; }

SOLO=1 start -C "$D/c" "$E2E_BIN shop"
open_table t_user; wait_for 8 settled
key l; edit zed; key Enter; sleep 0.3
key j l; edit "x@y"; key Enter; sleep 0.3
key j; key d d; sleep 0.3
key C-s; sleep 1.2
check "C-s 先不存，弹预览框：标题「保存到 t_user：改 2 行、删 1 行」，底部 y 保存 / n 取消" eval 'box_open && box_has "保存到 t_user：改 2 行、删 1 行" || { e2e_plain | sed -n 14,28p | cut -c1-140; false; }'
check "框里一条一行的语句：1 条 delete、2 条 update，带参数内联后的值（zed、x@y）" eval '[[ $(nstmt) -eq 3 ]] && box_has "delete from \"main\".\"t_user\"" && box_has "$Z_NAME" && box_has "$Z_MAIL"' || { nstmt; false; }
check "库里还没动：名字 alice、bob、carol，carol 在，email 没改" eval '[[ $(names) == "alice,bob,carol" && $(lite "select count(*) from t_user where email = $Q_MAIL") == 0 ]]'
key n; sleep 0.6
check "n：框关了，库还是没动" eval '! box_open && [[ $(names) == "alice,bob,carol" ]]'
check "n 之后改动的标记都还在：表格里还是 zed、x@y、第 3 行带删除标记 −" eval 'e2e_plain | grep -q "│ zed " && e2e_plain | grep -q "x@y" && e2e_plain | grep -q "│ − │" || { e2e_plain | sed -n 3,9p | cut -c35-120; false; }'
key C-s; sleep 1.0; key Escape; sleep 0.6
check "再 C-s 又弹出同样的框（标记还在）；esc：框关了、库没动、标记还在" eval '! box_open && [[ $(names) == "alice,bob,carol" ]] && e2e_plain | grep -q "│ − │"'
key C-s; sleep 1.0; key Enter; wait_for 5 eval '! box_open'; sleep 1
check "C-s 再弹框、↵：执行，库里 name 是 zed、bob 的 email 是 x@y、carol 没了" eval '[[ $(names) == "zed,bob" && $(lite "select email from t_user where id = 2") == "x@y" ]] || { names; false; }'
check "存完框关了，标记清了（没有 −）" eval '! box_open && ! e2e_plain | grep -q "│ − │"'

# ---- :w 也先预览；y 执行
key j; key l; edit "by colon"; key Enter; sleep 0.3
e2e_type ":w"; key Enter; sleep 1.0
check ":w 也弹预览：「保存到 t_user：改 1 行」，库没动" eval 'box_open && box_has "保存到 t_user：改 1 行" && [[ $(lite "select count(*) from t_user where '"'by colon'"' in (name, email, created_at, active)") == 0 ]] || { e2e_plain | sed -n 14,26p | cut -c1-140; false; }'
key y; wait_for 5 eval '! box_open'; sleep 1
check "y：执行，库里有了 by colon" eval '[[ $(lite "select count(*) from t_user where '"'by colon'"' in (name, email, created_at, active)") == 1 ]] || { lite "select * from t_user"; false; }'

# ---- 新增一行：增 1 行
open_table t_log; wait_for 8 settled
key o; sleep 0.3; key l; edit "t0"; key Enter; key l; edit "new line"; key Enter; sleep 0.3
key C-s; sleep 1.0
check "新增行：「保存到 t_log：增 1 行」，框里一条 insert into" eval 'box_open && box_has "保存到 t_log：增 1 行" && e2e_plain | grep -q "insert into \"main\".\"t_log\"" && e2e_plain | grep -q "new line" || { e2e_plain | sed -n 14,26p | cut -c1-140; false; }'
check "没存：t_log 还是原来的行数" eval '[[ $(lite "select count(*) from t_log") == 3 ]]'
key y; wait_for 5 eval '! box_open'; sleep 1
check "y：t_log 多了 new line 一行" eval '[[ $(lite "select count(*) from t_log") == 4 && $(lite "select count(*) from t_log where line = '"'new line'"'") == 1 ]]'

quit_app
_e2e_kill

# ---- 超过 20 条折叠（高一点的终端：20 条语句各占两行，45 行放不下）
SOLO=1 start -y 100 -C "$D/c" "$E2E_BIN shop"
open_table t_many; wait_for 8 settled
lite "update t_many set v = 'l1${NL}l2' where id = 1"
key g g; key l
key R; wait_for 8 settled; sleep 0.5
edit "once"; key Enter; sleep 0.3
key C-s; sleep 1.5
check "值里的换行在预览里显示成 ⏎（旧值 l1⏎l2 出现在 WHERE 里，语句仍是一行）" eval 'box_open && box_has "l1⏎l2"' || { e2e_plain | sed -n 3,20p | cut -c1-170; false; }
key n; sleep 0.6
key g g; key l
for i in $(seq 1 25); do edit "w$i"; key Enter; sleep 0.15; key j; done
key C-s; sleep 1.5
check "改 25 行：标题「改 25 行」，框里只列 20 条，最后一行「…另 5 条」" eval 'box_open && box_has "保存到 t_many：改 25 行" && [[ $(nstmt) -eq 20 ]] && box_has "…另 5 条" || { nstmt; e2e_plain | command grep -n "另\|…" | cut -c1-160; false; }'
key y; wait_for 5 eval '! box_open'; sleep 1.5
check "y：25 行都改了（w1…w25），26 到 30 没动" eval '[[ $(lite "select count(*) from t_many where v like '"'w%'"'") == 25 && $(lite "select v from t_many where id = 26") == v26 ]]'
quit_app
_e2e_kill

# ---- [grid] preview_save = false：C-s 直接存，没有框
printf '[grid]\npreview_save = false\n' >"$D/c/config.toml"
SOLO=1 e2e_start -C "$D/c" "$E2E_BIN shop"; wait_for 8 flag_is alternate_on 1; sleep 1; solo
open_table t_user; wait_for 8 settled
key l; edit "direct"; key Enter; sleep 0.3
key C-s; sleep 1.5
check "preview_save = false：C-s 直接存，没有预览框，库里 name 是 direct" eval '! box_open && [[ $(lite "select name from t_user where id = 1") == direct ]]'
quit_app
_e2e_kill
rm -f "$D/c/config.toml"

# ---- Redis 的保存同样列命令
redis_start R || exit 1
(umask 077; printf '[[connection]]\nname = "rd"\nengine = "redis"\ndsn = "redis://localhost:%s/0"\n' "$RPORT" >"$D/c/connections.toml")
redis_seed
side() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[-] ?", "", l[1:33]).rstrip("│ ")) for l in sys.stdin.read().split("\n")]'; }
nth_row() { side | python3 -c "
import re, sys
n = int(sys.argv[2])
for i, l in enumerate(sys.stdin.read().split('\\n')):
    if re.search(sys.argv[1], l.strip()):
        n -= 1
        if n == 0: print(i + 1); break" "$1" "${2:-1}"; }
click_row() { local y; y=$(nth_row "$@"); [[ -n $y ]] || return 1; e2e_click 14 "$y"; sleep 0.5; }
set_cell() { key Enter; e2e_type "$1"; sleep 0.2; key Enter; }
e2e_start -C "$D/c" "$E2E_BIN rd"; wait_for 8 flag_is alternate_on 1; sleep 2
click_row '^[▸▾] +hash( |$)'
y=$(nth_row '^user$'); key C-h; key g g; for ((i = 4; i < y; i++)); do key j; done; key Enter; sleep 1.2
key g g; key l; set_cell 31
key j; key j; key j; key h; set_cell nick
key k; key d; key d
key o; set_cell city; key l; set_cell Paris
key C-s; sleep 1.2
check "Redis hash 保存：弹预览框，列的是命令（HSET hash:user age 31、HSET city Paris、HDEL meta、改名的 HDEL name + HSET nick）" eval 'box_open && box_has "HSET hash:user" && box_has "HDEL hash:user" && box_has "age 31" && box_has "city Paris" && box_has "meta" || { e2e_plain | sed -n 8,30p | cut -c1-140; false; }'
check "没存：库里 age 还是 30，name 还在，meta 还在" eval '[[ $(rc hget hash:user age) == 30 && $(rc hexists hash:user name) == 1 && $(rc hexists hash:user meta) == 1 ]]'
key n; sleep 0.6
check "n：框关了，什么都没发（库没动），标记还在（保存按钮的计数还是 4）" eval '! box_open && [[ $(rc hget hash:user age) == 30 ]] && e2e_text 36 96 4 | grep -qE "PAGE 1/1[^0-9]+4([^0-9]|$)" || { e2e_text 36 96 4; false; }'
key C-s; sleep 1.0; key y; sleep 1.5
check "y：写回，age 31、nick Ann、city Paris，没有 name、meta" eval '[[ $(rc hget hash:user age) == 31 && $(rc hget hash:user nick) == Ann && $(rc hget hash:user city) == Paris && $(rc hexists hash:user name) == 0 && $(rc hexists hash:user meta) == 0 ]]'
click_row '^[▸▾] +list( |$)'
y=$(nth_row '^queue$'); key C-h; key g g; for ((i = 4; i < y; i++)); do key j; done; key Enter; sleep 1.5
key g g; key d d; sleep 0.3; key C-s; sleep 1.2
check "Redis list：删第一行后预览列 LREM list:queue …（删除的元素不在表里，显示 <deleted> 占位），库还没动（5 个元素）" eval 'box_open && box_has "LREM list:queue" && [[ $(rc llen list:queue) == 5 ]]' || { e2e_plain | sed -n 8,30p | cut -c1-150; false; }
key n; sleep 0.5
check "n：库没动，框关了" eval '! box_open && [[ $(rc llen list:queue) == 5 ]]'
e2e_done
