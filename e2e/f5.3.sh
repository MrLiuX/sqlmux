#!/usr/bin/env bash
# F5.3 只读连接与事务模式（specs/m5-workspace/task.md F5.3；tech-design §5、§7.8「console 标题」「状态栏」、§8.2、§10.1、§10.3、§10.5、§13）
# 标题的事务控件、状态栏的「事务中」「事务出错」、锁的画法由 golden 覆盖，解除只读的语句清单由集成测试覆盖；
# 这里在自建库上走真实的端到端：只读连接整次不发、DISCARD ALL 之后仍只读、表格拦下；手动模式下执行和保存都不提交，
# ✓ 提交、↺ 回滚、出错的事务、有事务时不让切回自动；关掉最后一个 console、退出时确认并回滚。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
APP_RW=e2e-f53w-$$ APP_RO=e2e-f53r-$$ WARN=#e0af68 ERROR=#f7768e FOCUS=#9ece6a INFO=#7dcfff DIM=#565f89
mkdir -p "$D/rw" "$D/ro"
(umask 077; printf '[[connection]]\nname = "rw"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB&application_name=$APP_RW" >"$D/rw/connections.toml"
 printf '[[connection]]\nname = "ro"\nengine = "postgres"\ndsn = "%s"\nread_only = true\n' "$E2E_DB&application_name=$APP_RO" >"$D/ro/connections.toml")

psql_n() { psql "$E2E_DB" -At -c "$1"; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }   # ② console 的内容换成 SQL
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.4; }
run() { key Enter; done_run; }
ctitle() { local g; g=($(geom 2)); e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "${g[1]}"; }   # ② 的标题行
tx_x() { local g; g=($(geom 2)); e2e_find "$1" "${g[1]}" | tr ' ' '\n' | awk -v l="${g[0]}" '$1 > l { print; exit }'; }   # TEXT 在 ② 标题上的列
click_title() { e2e_click "$(tx_x "$1")" "$(geom 2 | cut -d' ' -f2)"; sleep 0.4; }
txstate() { bar | grep -oE '事务中|事务出错'; }                              # 状态栏的事务块
txstate_is() { local c; [[ $(txstate) == "$1" ]] || { echo "  status bar: '$(bar)', want $1"; return 1; }; c=$(e2e_find "$1" "$(H)" | cut -d' ' -f1); style_has "$c" "$(H)" "fg=$2"; }
rval() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }   # ③ 结果第一行第一列
pane_has() { local g y; g=($(geom "$1")); for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "$y"; done | grep -qF -- "$2"; }
toast_row() { e2e_text 1 "$(e2e_flag pane_width)" $(($(H) - 1)); }
qbar() { local g; g=($(geom 1)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 2)); }   # ① 查询条的第二行
amount() { psql_n "select amount from t_order where id = $1"; }
idle_tx() { psql_n "select count(*) from pg_stat_activity where datname = current_database() and state like 'idle in transaction%'"; }
log_n() { local g y; g=($(geom 3)); for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "$y"; done | grep -cF -- "$1"; }   # ③ 里含 TEXT 的行数
trow() { local y; for y in $(seq 3 43); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }   # 侧栏里 NAME 的行
row2() { e2e_plain | grep -E "│ +2 │ +2 │"; }                               # ① 表格里 id 2 那一行
box() { e2e_plain | grep -oE '[^│]*(处修改未保存|有未提交的事务)[^│]*' | head -1 | sed 's/^ *//; s/ *$//'; }   # 确认框里的那句话

# ---- 只读连接：数据库层，Main 也设了 default_transaction_read_only，DISCARD ALL 之后仍是 on
start -x 200 -C "$D/ro"; key C-l
setsql "discard all"; run
setsql "show default_transaction_read_only"; run
check "ro 的 console：DISCARD ALL 之后 default_transaction_read_only 仍是 on（RuntimeParams）" eval '[[ $(rval) == on ]] || { echo "  got: $(rval)"; false; }'

# ---- 只读连接：应用层，这一次里有一条写语句就整次都不发
setsql "$(printf 'select pg_advisory_lock(4242);\ndelete from t_log')"; key V; key G; key Enter; sleep 0.8
check "选中 select + delete 执行：整次都没发（select 拿的 advisory lock 没有），t_log 还是 3 行" eval '[[ $(psql_n "select count(*) from pg_locks where locktype = '"'advisory'"' and objid = 4242") == 0 && $(psql_n "select count(*) from t_log") == 3 ]]'
check "结果区切到日志 tab，记一行「ro 是只读连接，整次都没有执行」；不弹 toast，不出错误栏" eval '[[ $(tabbar 3) == "1:日志*"* ]] && pane_has 3 "ro 是只读连接，整次都没有执行" && [[ $(toast_row) != *只读* && -z $(errbar 2) ]]'
setsql "set default_transaction_read_only = off"; run
check "SET default_transaction_read_only = off：拦下，日志又记一行" eval '[[ $(tabbar 3) == "1:日志*"* && $(pane_has 3 "set default_transaction_read_only = off" && echo y) == y ]]'
setsql "begin read write"; run
check "BEGIN READ WRITE：拦下" eval '[[ $(tabbar 3) == "1:日志*"* ]] && pane_has 3 "begin read write"'
setsql "set session session characteristics as transaction read write"; run
check "SET SESSION SESSION CHARACTERISTICS AS TRANSACTION READ WRITE（作用域词连写）：拦下" eval '[[ $(tabbar 3) == "1:日志*"* ]] && pane_has 3 "set session session characteristics as transaction read write"'
n0=$(log_n "set default_transaction_read_only = off")
setsql 'select 1 as ✓$a$; set default_transaction_read_only = off; select 1 as ✓$a$'; key V; key Enter; sleep 0.8
check "✓\$a\$ 不是 dollar 引号的开头（0x80 以上的字节算标识符）：中间的 SET 认得出来，整次拦下" eval '(( $(log_n "set default_transaction_read_only = off") == n0 + 1 )) && [[ $(tabbar 3) == "1:日志*"* ]]'
setsql "set statement_timeout = 4321"; run; setsql "show statement_timeout"; run
check "别的 SET 放行：SET statement_timeout 执行了（show 出来是 4321ms）；拦下的三次不算执行，这是第 4 次（console_1 #4）" eval '[[ $(rval) == 4321ms && $(tabbar 3) == *"console_1 #4*"* ]] || { echo "  got: $(rval), tabs: $(tabbar 3)"; false; }'

# ---- 只读连接：表格的编辑、新增、删除都拦下
key C-h; open_table t_user; wait_for 8 eval '[[ $(qbar) == *行* ]]'
key l; key Enter
check "↵ 编辑单元格：toast「ro 是只读连接」，不进入编辑" eval 'toast_is "ro 是只读连接" && mode_is NORMAL'
wait_for 5 eval '[[ $(toast_row) != *只读连接* ]]'; key o
check "o 新增行：同样拦下" eval 'toast_is "ro 是只读连接" && ! e2e_plain | grep -qE "│ +\+ │"'
wait_for 5 eval '[[ $(toast_row) != *只读连接* ]]'; key d d
check "dd 标记删除：同样拦下" eval 'toast_is "ro 是只读连接" && ! e2e_plain | grep -qE "│ +− │"'
wait_for 5 eval '[[ $(toast_row) != *只读连接* ]]'; key i
check "i 编辑：同样拦下" eval 'toast_is "ro 是只读连接" && mode_is NORMAL'
wait_for 5 eval '[[ $(toast_row) != *只读连接* ]]'; key y y; key p
check "yy 之后 p 把行粘贴成新行：同样拦下" eval 'toast_is "ro 是只读连接" && ! e2e_plain | grep -qE "│ +\+ │"'

# ---- 事务模式：点 Tx: 自动 ▾ 切成手动，console 执行不提交，✓ 提交、↺ 回滚
start -x 200 -C "$D/rw"; open_table t_order; key C-l
check "rw 的 console 标题：Tx: 自动 ▾，没有 ✓ ↺；状态栏没有事务块" eval '[[ $(ctitle) == *"Tx: 自动 ▾ "* && $(ctitle) != *"✓"* && -z $(txstate) ]]'
click_title "Tx:"; e2e_type 手动; sleep 0.3; key Enter
check "点 Tx 选手动：标题 Tx: 手动 ▾ ✓ ↺，不在事务里时 ✓ ↺ 是 dim" eval '[[ $(ctitle) == *"Tx: 手动 ▾ ✓ ↺ "* ]] && style_has "$(tx_x ✓)" 1 fg=$DIM && style_has "$(tx_x ↺)" 1 fg=$DIM'
setsql "update t_order set amount = 1.23 where id = 1"; run
check "手动模式下执行 update：没提交（库里还是 1.99），状态栏 warn 色「事务中」，✓ focus 色、↺ info 色" eval '[[ $(amount 1) == 1.99 ]] && txstate_is 事务中 $WARN && style_has "$(tx_x ✓)" 1 fg=$FOCUS && style_has "$(tx_x ↺)" 1 fg=$INFO'
click_tab 3 "1:"
check "BEGIN 不写进日志：日志里有 update 那一行，没有 begin" eval 'pane_has 3 "console_1  update t_order set amount = 1.23 where id = 1" && ! pane_has 3 "console_1  begin"'
click_title "Tx:"; e2e_type 自动; sleep 0.3; key Enter
check "有未提交的事务时切回自动：被拦下，toast「当前连接有未提交的事务，先提交或回滚」" eval 'toast_is "当前连接有未提交的事务，先提交或回滚" && [[ $(ctitle) == *"Tx: 手动 ▾"* ]]'
click_title "↺"; wait_for 5 eval '[[ -z $(txstate) ]]'
check "点 ↺ 回滚：事务块消失，库里还是 1.99，没有 idle in transaction" eval '[[ -z $(txstate) && $(amount 1) == 1.99 && $(idle_tx) == 0 ]]'
setsql "update t_order set amount = 1.23 where id = 1"; run; click_title "✓"; wait_for 5 eval '[[ -z $(txstate) ]]'
check "再 update、点 ✓ 提交：库里变成 1.23，事务块消失" eval '[[ -z $(txstate) && $(amount 1) == 1.23 ]]'

# ---- 手动模式下每条语句前检查 BEGIN（照 psql）；command_no_begin 的语句不补；提交之后 catalog 重新加载
setsql "$(printf 'update t_order set amount = 3.33 where id = 3;\ncommit;\nupdate t_order set amount = 4.44 where id = 4')"; key V; key G; key Enter; done_run
check "update; commit; update：commit 之后那条又开了事务（第一条已提交，第二条没有），状态栏「事务中」" eval '[[ $(amount 3) == 3.33 && $(amount 4) == 4.99 ]] && txstate_is 事务中 $WARN'
click_title "↺"; wait_for 5 eval '[[ -z $(txstate) ]]'
check "↺：第二条撤销" eval '[[ $(amount 4) == 4.99 && $(amount 3) == 3.33 ]]'
setsql "vacuum t_log"; run
check "手动模式下 vacuum t_log：不补 BEGIN，照常执行，没有进事务、没有报错" eval '[[ -z $(txstate) && -z $(errbar 2) ]]'
setsql "create table e2e_new (id int)"; run; click_title "✓"; wait_for 5 eval '[[ -z $(txstate) ]]'; sleep 0.8
check "手动模式下 create table、✓ 提交：catalog 重新加载，树里多了 e2e_new（12 tables）" eval '[[ -n $(trow e2e_new) && $(e2e_text 1 32 2) == *"12 tables"* ]]'
setsql "drop table e2e_new"; run; click_title "✓"; wait_for 5 eval '[[ -z $(txstate) ]]'; sleep 0.8
check "drop、✓：树里没了（11 tables）" eval '[[ -z $(trow e2e_new) && $(e2e_text 1 32 2) == *"11 tables"* ]]'

# ---- 手动模式下保存：不提交，表格看得到自己的修改；回滚后撤销、表格重取
key C-h; key g g; key 0; key j; key 3 l; key Enter; e2e_type 8.88; key Enter; key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* ]]'
check "手动模式下 C-s：查询条「已保存 1 行（未提交）」，库里 id 2 还是 2.99，状态栏「事务中」" eval '[[ $(qbar) == *"已保存 1 行（未提交） · "* && $(amount 2) == 2.99 ]] && txstate_is 事务中 $WARN || { echo "  query bar: $(qbar)"; false; }'
check "表格取数走 Main：重取之后表格里 id 2 是 8.88（自己没提交的修改）" eval 'e2e_plain | grep -E "│ +2 │ +2 │" | grep -q "8.88"'
key /; e2e_type "nosuchcol = 1"; key Enter; sleep 1
check "事务中 WHERE 写错：错误栏报错，事务没有进 E（取数包在 SAVEPOINT 里），仍是「事务中」" eval '[[ $(errbar 1) == *nosuchcol* ]] && txstate_is 事务中 $WARN'
key /; clear_in; key Enter; wait_for 8 eval '[[ -z $(errbar 1) ]]'
pal ">tx.rollback"; key Enter; wait_for 5 eval '[[ -z $(txstate) ]]'; sleep 0.5
check "tx.rollback：修改撤销，库里还是 2.99，表格重取回 2.99" eval '[[ $(amount 2) == 2.99 ]] && e2e_plain | grep -E "│ +2 │ +2 │" | grep -q "2.99"'

# ---- 出错的事务：状态栏「事务出错」，提交改为回滚
key C-l; setsql "select 1/0"; run
check "手动模式下执行出错：状态栏 error 色「事务出错」" txstate_is 事务出错 $ERROR
pal ">tx.commit"; key Enter; wait_for 5 eval '[[ -z $(txstate) ]]'
check "出错的事务里提交：toast「事务已出错，提交改为了回滚」，事务块消失" eval 'toast_is "事务已出错，提交改为了回滚" && [[ -z $(txstate) ]]'

# ---- tx.mode：开关类命令，面板里切换不关面板
pal ">tx.mode"
check "面板里 tx.mode「手动提交事务」现在是 ON" eval 'list | head -1 | grep -q "^手动提交事务  tx.mode  ON"'
key Enter; sleep 0.3
check "↵：切成自动，面板不关、显示 OFF；标题变回 Tx: 自动 ▾" eval 'is_open && list | head -1 | grep -q "OFF" && { key Escape; [[ $(ctitle) == *"Tx: 自动 ▾ "* ]]; }'

# ---- console 有执行在跑时不能关（关 tab、关 pane、关 window）
setsql "select count(*) from t_order"; e2e_lock t_order; key Enter; sleep 0.5
check "（准备）console 在等锁" eval '[[ $(bar) == *busy* ]]'
e2e_type ':q'; key Enter; sleep 0.3
check "执行在跑时 :q：不关，toast「正在执行，结束后再关闭」" eval 'toast_is "正在执行，结束后再关闭" && tabs_are 2 "1:console_1*"'
wait_for 5 eval '[[ $(toast_row) != *正在执行* ]]'; e2e_keys Space; e2e_type x; sleep 0.4
check "SPC x 关它所在的 pane：同样不关" eval 'toast_is "正在执行，结束后再关闭" && tabs_are 2 "1:console_1*"'
wait_for 5 eval '[[ $(toast_row) != *正在执行* ]]'; e2e_keys Space; e2e_type c; sleep 0.5; e2e_keys Space; e2e_type p; sleep 0.4
pal ">window.close"; key Enter
check "window.close 关它所在的 window：同样不关" eval 'toast_is "正在执行，结束后再关闭" && [[ $(bar) == *" 0: data*  1: data-2 "* ]]'
e2e_unlock; done_run; e2e_keys Space; e2e_type n; sleep 0.4; pal ">window.close"; key Enter
check "执行回来之后：关掉多出来的 window 不用确认（不在事务里）" eval '[[ $(bar) == *" 0: data* "* && $(bar) != *"1: data-2"* ]]'
key C-l; key C-l

# ---- 自动模式下自己 begin：也算事务中，保存走 SAVEPOINT、不提交
setsql "begin"; run
check "自动模式下自己 begin：状态栏也显示「事务中」" txstate_is 事务中 $WARN
key C-h; key g g; key 0; key j; key 3 l; key Enter; e2e_type 7.77; key Enter; key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* ]]'
check "这时保存：「已保存 1 行（未提交）」，库里还是 2.99（没有把用户的事务一起提交）" eval '[[ $(qbar) == *"（未提交）"* && $(amount 2) == 2.99 ]] || { echo "  query bar: $(qbar)"; false; }'
check "格子显示新值 7.77" eval 'row2 | grep -q 7.77'
key g g; key 0; key j; key 3 l; key Enter; e2e_type 6.66; key Enter; key C-s; wait_for 8 eval 'row2 | grep -q 6.66'; sleep 0.3
check "再改同一格、保存：也成功（没有错误栏），「（未提交）」，表格 6.66，库里仍 2.99" eval '[[ -z $(errbar 1) && $(qbar) == *"（未提交）"* && $(amount 2) == 2.99 ]] && row2 | grep -q 6.66'

# ---- 关 console：这个连接上还有别的 console 时不问；关掉最后一个时确认，先关再回滚
pal ">console.new"; key Enter; sleep 0.5                                      # ① 里新开 console_2，也在 rw 上
key C-l; e2e_type ':q'; key Enter; sleep 0.4
check "rw 上还有 console_2：:q 关掉 ② 的 console_1 不问，事务还在" eval '[[ -z $(box) ]] && txstate_is 事务中 $WARN && [[ $(tabbar 1) == *console_2* ]]'
pal "%console_2"; key Enter; e2e_type ':q'; key Enter; sleep 0.3
check ":q 关掉 rw 上最后一个 console：确认框「rw 有未提交的事务，关闭会回滚。」" eval '[[ $(box) == "rw 有未提交的事务，关闭会回滚。" ]] || { echo "  $(box)"; false; }'
key n
check "n：console 还在，事务还在" eval '[[ -z $(box) && $(tabbar 1) == *console_2* ]] && txstate_is 事务中 $WARN'
e2e_type ':q'; key Enter; sleep 0.3; key y; wait_for 5 eval '[[ $(idle_tx) == 0 ]]'; sleep 0.5
check "y：关掉 console 并回滚：没有 idle in transaction，库里 id 2 还是 2.99，表格重取" eval '[[ $(idle_tx) == 0 && $(amount 2) == 2.99 && -z $(txstate) && $(tabbar 1) != *console* ]] && e2e_plain | grep -E "│ +2 │ +2 │" | grep -q "2.99"'

# ---- 关 window：里面的 console 是这个连接上最后一个时同样确认
e2e_keys Space; e2e_type c; sleep 0.5; key C-l; setsql "begin"; run; setsql "delete from t_log"; run
check "（准备）新 window 的 console 里 begin、delete from t_log：事务中" eval 'txstate_is 事务中 $WARN'
pal ">window.close"; key Enter; sleep 0.3
check "window.close：确认框「rw 有未提交的事务，关闭会回滚。」" eval '[[ $(box) == "rw 有未提交的事务，关闭会回滚。" ]] || { echo "  $(box)"; false; }'
key y; wait_for 5 eval '[[ $(idle_tx) == 0 ]]'
check "y：关掉 window 并回滚（t_log 还是 3 行）" eval '[[ $(idle_tx) == 0 && $(psql_n "select count(*) from t_log") == 3 && -z $(txstate) ]]'

# ---- 退出：确认，靠断开连接回滚
pal ">console.new"; key Enter; sleep 0.5; setsql "begin"; run; setsql "delete from t_log"; run
check "（准备）新 console 里 begin、delete from t_log：事务中" eval 'txstate_is 事务中 $WARN && [[ $(psql_n "select count(*) from t_log") == 3 ]]'
key C-h; e2e_type ':'; sleep 0.3; e2e_type qa; key Enter; sleep 0.3
check ":qa：确认框「rw 有未提交的事务，退出会回滚。」" eval '[[ $(box) == "rw 有未提交的事务，退出会回滚。" ]] || { echo "  $(box)"; false; }'
key y
check "y：退出，断开连接，事务回滚（t_log 还是 3 行）" eval 'wait_for 3 exited && wait_for 3 eval "[[ \$(psql_n \"select count(*) from pg_stat_activity where application_name = '"'$APP_RW'"'\") == 0 ]]" && [[ $(psql_n "select count(*) from t_log") == 3 ]]'

e2e_done
