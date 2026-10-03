#!/usr/bin/env bash
# F5.1 多 session 与新建连接（specs/m5-workspace/task.md F5.1；tech-design §5、§6.8、§7.8「session 列表」「新建 session」「新建连接表单」「状态栏」、§12、§14）
# session 列表、表单的画法由 golden 覆盖；这里在真实终端和 PG 上测：新建 session（名字、默认布局、自己的一套连接、连不上不建）、
# 两个 session 来回切界面不变、列表的 l / h / ↵ / $ / x、关掉最后一个等于退出、后台 session 的应答不动当前界面、面板的会话范围，
# 以及新建连接写进 connections.toml（原有内容和注释原样保留、末尾没换行、文件不存在时新建 0600）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
APP=e2e-f51-$$ APP2=e2e-f51x-$$ ERROR=#f7768e
# 用户手写的 connections.toml：带注释，最后一行后面没有换行
mkdir -p "$D/c"; (umask 077; printf '# my connections, keep this comment\n[[connection]]\nname = "doraemon"   # the seed\nengine = "postgres"\ndsn = "%s"\n# the end, no newline after this line' "$SQLMUX_TEST_PG&application_name=$APP" >"$D/c/connections.toml")

conns() { psql "$SQLMUX_TEST_PG" -At -c "select count(*) from pg_stat_activity where application_name = '${1:-$APP}'"; }
sname() { bar | sed 's/^ //; s/ ▾.*//'; }                                    # 状态栏 session 块里的名字
sname_is() { [[ $(sname) == "$1" ]] || { echo "  session block: '$(sname)', want '$1'"; false; }; }
wins() { bar | sed -E 's/^ [^▾]*▾  //; s/ {3,}.*//'; }                      # 状态栏的 window 列表：0: data*  1: data
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }      # 浮层（session 列表 / 表单）的 X Y W H
slist_open() { e2e_plain | grep -q "┌─ session"; }
# srows：session 列表的每一行「文字|选中 0/1」，3 个以上的空格压成两个；window 行前面缩进两格
srows() { local g; slist_open || return 0; g=($(box))
  e2e_rows $((g[0] + 2)) $((g[0] + g[2] - 2)) $((g[1] + 1)) $((g[1] + g[3] - 2)) $((g[0] + 2)) | awk -F'|' '{ sub(/ +$/, "", $1); gsub(/   +/, "  ", $1); print $1 "|" ($2 == "#364a82" ? 1 : 0) }'; }
rows_are() { local got; got=$(srows | tr '\n' ';'); [[ $got == "$1" ]] || { echo "  rows: '$got'"; echo "  want  '$1'"; false; }; }
sl() { e2e_keys Space; e2e_type s; sleep 0.4; }                             # SPC s
new_session() { key n; wait_for 6 eval '[[ $(sname) == "$1" ]]'; sleep 0.5; }   # NAME：列表里按 n，等它 attach
clr() { local i; for ((i = 0; i < 30; i++)); do e2e_keys BSpace; done; sleep 0.2; }
title1() { e2e_plain | head -1; }
kinds() { list | cut -d'|' -f1 | awk '{ print $NF }' | sort -u | tr '\n' ' '; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }   # 焦点所在 console 的内容换成 SQL

# ---- 新建 session：名字、默认布局、自己的一套连接
start -C "$D/c"; open_table t_order; key 3 j; key l
check "启动：一个 session doraemon，它的连接有 Meta、Main 两条" eval 'sname_is doraemon && [[ $(conns) == 2 ]]'
sl
check "SPC s：session 列表，一行「doraemon  1 windows」，光标在当前 session 上" eval 'slist_open && rows_are "doraemon  1 windows|1;"'
key n
check "n：列表关掉，toast「正在连接 doraemon…」" eval '! slist_open && toast_is "正在连接 doraemon…"'
wait_for 6 eval '[[ $(sname) == 1 ]]'; sleep 0.5
check "连上之后 attach 新 session：名字 1（最小的没用过的数字），一个 window 0: data*" eval 'sname_is 1 && [[ $(wins) == "0: data*" ]]'
check "新 session 是默认布局：侧栏、① 引导页、② console_2（n 遍历所有 session，console_1 在 doraemon 里）" eval '[[ $(e2e_panes | awk "\$1 != \"-\" { print \$1 }" | tr "\n" " ") == "0 1 2 " ]] && tabs_are 2 "1:console_2*" && e2e_plain | grep -q "打开表"'
check "新 session 连上当前连接并展开（侧栏有 Tables (6)），它有自己的一套连接：库里多了两条" eval 'e2e_plain | head -12 | grep -q "Tables (6)" && [[ $(conns) == 4 ]]'

# ---- 两个 session 来回切：各自的界面不变
open_table t_user; key 2 j
sl
check "两个 session，光标在当前的 1 上" rows_are "doraemon  1 windows|0;1  1 windows|1;"
key k; key Enter
check "k ↵：attach doraemon、列表关掉；它的界面原样：① t_order、光标 4,2，② console_1" eval '! slist_open && sname_is doraemon && [[ $(title1) == *" t_order ─"* ]] && pos_is 4,2 && tabs_are 2 "1:console_1*"'
e2e_click 3 "$(H)"; sleep 0.4
check "单击状态栏的 session 块：打开列表" slist_open
g=($(box)); e2e_click $((g[0] + 3)) $((g[1] + 2)); sleep 0.4
check "单击 1 那一行：attach 1、列表关掉；它的界面原样：① t_user、光标 3,1，② console_2" eval '! slist_open && sname_is 1 && [[ $(title1) == *" t_user ─"* ]] && pos_is 3,1 && tabs_are 2 "1:console_2*"'

# ---- l / h：session 下面是它的 window；停在 window 上 ↵ 还切到这个 window
key Space; e2e_type c; sleep 0.5
check "（准备）SPC c：1 有两个 window，当前是新的那个" eval '[[ $(wins) == "0: data  1: data*" ]]'
sl; key k; key Enter; sl; key j; key l
check "l：展开 1，下面一级是它的两个 window（缩进）" rows_are "doraemon  1 windows|0;1  2 windows|1;  0: data|0;  1: data|0;"
key j; key h
check "window 行上 h：回到它的 session 行并收起" rows_are "doraemon  1 windows|0;1  2 windows|1;"
key l; key j; key Enter
check "停在 1 的 window 0 上 ↵：attach 1 并切到 window 0" eval '! slist_open && sname_is 1 && [[ $(wins) == "0: data*  1: data" ]]'

# ---- $ 原地重命名
sl; e2e_type '$'; sleep 0.3
check "\$：这一行原地变成输入框，模式 INSERT" eval 'mode_is INSERT && [[ $(srows | sed -n 2p) == 1* ]]'
clr; e2e_type doraemon; key Enter
check "改成已有的 doraemon：toast「已有同名 session」，名字不变" eval 'toast_is "已有同名 session" && rows_are "doraemon  1 windows|0;1  2 windows|1;" && sname_is 1'
e2e_type '$'; clr; key Enter
check "清空后 ↵：等于放弃，名字不变" rows_are "doraemon  1 windows|0;1  2 windows|1;"
e2e_type '$'; e2e_type xyz; key Escape
check "esc：放弃，名字不变，列表还开着" eval 'slist_open && rows_are "doraemon  1 windows|0;1  2 windows|1;" && mode_is COMMAND'
e2e_type '$'; clr; e2e_type work; key Enter
check "改成 work（k 进了输入框，不是上移）：这一行和状态栏的 session 块都变成 work" eval 'rows_are "doraemon  1 windows|0;work  2 windows|1;" && sname_is work'
key l; key j; e2e_type '$'; sleep 0.3
check "window 行上 \$：不做事" eval 'mode_is COMMAND && rows_are "doraemon  1 windows|0;work  2 windows|0;  0: data|1;  1: data|0;"'
key h; key Escape

# ---- 面板的会话范围（§12）
pal '$'
check "\$ 范围：只列 session，名称是 session 名、所在位置是 window 数、类型标签「会话」" eval '[[ $(kinds) == "会话 " && $(list | cut -d"|" -f1 | tr "\n" ";") == "doraemon  1 windows  会话;work  2 windows  会话;" ]]'
clear_input; sleep 0.3
check "「所有」范围也列 session" eval 'list | grep -q "^doraemon  1 windows  会话|" && list | grep -q "^work  2 windows  会话|"'
e2e_type '$dor'; sleep 0.3; key Enter
check "在 doraemon 上 ↵：attach，面板关掉" eval 'closed && sname_is doraemon && [[ $(title1) == *" t_order ─"* ]]'
pal '$doraemon'; key Enter
check "在当前 session 上 ↵：只关面板" eval 'closed && sname_is doraemon'

# ---- x 关闭；名字取最小的没用过的数字
sl; new_session 1; sl; new_session 2
check "再建两个：名字 1、2，库里 4 个 session 共 8 条连接" eval 'sname_is 2 && [[ $(conns) == 8 ]]'
sl; key k; key x; wait_for 3 eval '[[ $(conns) == 6 ]]'
check "x 关掉不是当前的 1：列表还开着、光标落到下一行（2），当前还是 2，它的两条连接关掉了" eval 'slist_open && rows_are "doraemon  1 windows|0;work  2 windows|0;2  1 windows|1;" && sname_is 2 && [[ $(conns) == 6 ]]'
new_session 1
check "关掉 1 之后再 n：新 session 又叫 1，排在最后" eval 'sname_is 1 && { sl; rows_are "doraemon  1 windows|0;work  2 windows|0;2  1 windows|0;1  1 windows|1;"; }'
key k; key k; key Enter; sl
check "（准备）attach work，光标在第 2 行" rows_are "doraemon  1 windows|0;work  2 windows|1;2  1 windows|0;1  1 windows|0;"
key x
check "x 关掉当前的 work：attach 它的上一个 doraemon，列表还开着、光标留在第 2 行" eval 'slist_open && sname_is doraemon && rows_are "doraemon  1 windows|0;2  1 windows|1;1  1 windows|0;"'
key k; key x
check "x 关掉当前的、排第一的 doraemon：attach 它的下一个 2" eval 'slist_open && sname_is 2 && rows_are "2  1 windows|1;1  1 windows|0;"'

# ---- 有未保存的修改先确认；关之前把 console 写盘
key Escape; open_table t_user; key l; key Enter; e2e_type x; key Enter   # 2 的 ① t_user 改一格
key C-l; key i; e2e_type "select 42"; key Escape; cf=$(tabbar 2 | sed 's/^1://; s/\*$//')   # 2 的 ② console 写一句
sl; e2e_type x; sleep 0.4
check "x 关掉有修改的 2：确认框「有 1 处修改未保存，关闭会丢弃。」y 关闭" eval 'e2e_plain | grep -q "有 1 处修改未保存，关闭会丢弃。" && e2e_plain | grep -q "y 关闭   n 取消"'
key n
check "n：取消，两个 session 都在" eval '! e2e_plain | grep -q 处修改未保存 && rows_are "2  1 windows|1;1  1 windows|0;"'
e2e_type x; sleep 0.3; key y
check "再按 x、y：关掉 2，attach 1；2 的 console 写了盘" eval 'sname_is 1 && rows_are "1  1 windows|1;" && grep -q "select 42" "$E2E_TMP/data/sqlmux/consoles/$cf.sql"'
key x
check "x 关掉最后一个 session：等于退出，连接都关了" eval 'wait_for 3 exited && wait_for 3 eval "[[ \$(conns) == 0 ]]"'

# ---- 后台 session 的应答：按应答里的 tab 处理，不动当前 session 的焦点和布局
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
mkdir -p "$D/own"; (umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB&application_name=$APP2" >"$D/own/connections.toml")
start -C "$D/own"; key C-l; setsql "select count(*) from t_order"
e2e_lock t_order; key Enter; sleep 0.5
check "（准备）doraemon 的 console 在等锁：状态栏 busy" eval '[[ $(bar) == *busy* ]]'
sl; new_session 1
check "新 session 1 不忙：busy 跟着当前连接，1 用的是自己的连接" eval 'sname_is 1 && [[ $(bar) != *busy* ]]'
e2e_unlock; sleep 1.5
check "doraemon 的查询回来了：1 的界面不动（没有结果区、焦点还在 ①）" eval '[[ $(e2e_panes | awk "\$1 != \"-\" { print \$1 }" | tr "\n" " ") == "0 1 2 " ]] && focus_is 1 && sname_is 1'
sl; key k; key Enter
check "回到 doraemon：结果进了它的结果区（6000）" eval 'sname_is doraemon && g=($(geom 3)) && e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | grep -q 6000'

# ---- 关掉在事务里的 session：确认，关掉它的连接，服务端回滚
sl; key j; key Enter; key C-l
setsql "begin"; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; setsql "delete from t_log"; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.3
check "（准备）1 的 console 里 begin、delete from t_log 没提交：库里 t_log 还是 3 行，1 的连接停在事务里" eval '[[ $(psql "$E2E_DB" -At -c "select count(*) from t_log") == 3 && $(psql "$E2E_DB" -At -c "select count(*) from pg_stat_activity where application_name = '"'$APP2'"' and state = '"'idle in transaction'"'") == 1 ]]'
sl; e2e_type x; sleep 0.4
check "x 关掉 1：确认框「doraemon 有未提交的事务，关闭会回滚。」（F5.3 起带连接名）" eval 'e2e_plain | grep -q "│ *doraemon 有未提交的事务，关闭会回滚。" && e2e_plain | grep -q "y 关闭   n 取消"'
key y; wait_for 3 eval '[[ $(conns "$APP2") == 2 ]]'
check "y：关掉 1、attach doraemon，它的连接断开，事务回滚（没有 idle in transaction 的连接，t_log 还是 3 行）" eval 'sname_is doraemon && [[ $(conns "$APP2") == 2 && $(psql "$E2E_DB" -At -c "select count(*) from t_log") == 3 && $(psql "$E2E_DB" -At -c "select count(*) from pg_stat_activity where datname = current_database() and state = '"'idle in transaction'"'") == 0 ]]'
key Escape

# ---- 新 session 连不上：toast，不建
psql "$SQLMUX_TEST_PG" -qAt -c "alter database sqlmux_e2e_$$ allow_connections false"
sl; key n; wait_for 6 eval 'e2e_text 1 160 $(($(H) - 1)) | grep -q "doraemon："'
check "库不让连：toast「doraemon：<驱动的错误>」，不建 session" eval 'e2e_text 1 160 $(($(H) - 1)) | grep -q "doraemon：.*FATAL: database \"sqlmux_e2e_$$\" is not curr" && sname_is doraemon && { sl; rows_are "doraemon  1 windows|1;"; }'
psql "$SQLMUX_TEST_PG" -qAt -c "alter database sqlmux_e2e_$$ allow_connections true"

# ---- 新建连接：表单、校验、写进 connections.toml
start -C "$D/c"; CT=$E2E_TMP/config/sqlmux/connections.toml
sl; new_session 1                                                          # 两个 session，看「所有 session 的树」
frows() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) "$y" | sed 's/ *$//'; done; }
form_open() { e2e_plain | grep -q "┌─ 新建连接"; }
field_y() { local g; g=($(box)); echo $(( g[1] + 1 + $1 )); }              # N：第 N 个字段（0 起：name engine dsn password_cmd read_only）的行
# on_field N：焦点在第 N 个字段上：输入框有终端光标，开关（engine、read_only）的值是 select 底
on_field() { local g; g=($(box)); if (( $1 == 1 || $1 == 4 )); then style_has $((g[0] + 16)) "$(field_y "$1")" bg=#364a82; else
  [[ $(( $(e2e_flag cursor_y) + 1 )) == $(field_y "$1") ]] || { echo "  cursor on row $(( $(e2e_flag cursor_y) + 1 )), field $1 is on $(field_y "$1")"; false; }; fi; }
ferr() { frows | sed -n 6p; }                                              # 底栏上方那一行：校验的原因
ferr_is() { local g; g=($(box)); [[ $(ferr) == "$1" ]] && style_has $((g[0] + 2)) $(( g[1] + 6 )) fg=$ERROR || { echo "  error row: '$(ferr)', want '$1'"; false; }; }
# extra 的 DSN 去掉密码，密码由带引号的 password_cmd 给：重启后连得上，说明这一段编码、读回都对
read -r NOPW PW < <(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p._replace(netloc=p.netloc.replace(":" + p.password + "@", "@")).geturl(), p.password)' "$SQLMUX_TEST_PG")
DSN2="$NOPW&application_name=$APP2"
pal ">conn.new"; key Enter
check "conn.new：表单「新建连接」，五个字段，engine 是 postgres、read_only 是 false，底栏 Tab 下一项 · ↵ 保存 · esc 取消，INSERT，光标在 name 上" eval 'form_open && [[ $(frows | sed -n 1,5p | tr -s " " | tr "\n" ";") == "name;engine postgres;dsn;password_cmd;read_only false;" && $(frows | tail -1) == "Tab 下一项 · ↵ 保存 · esc 取消" ]] && mode_is INSERT && on_field 0'
key Enter
check "空着 ↵：底栏上方 error 色「缺少 name」，焦点在 name" eval 'form_open && ferr_is "缺少 name" && on_field 0'
e2e_type ".x"; key Tab
check "Tab：到 engine" on_field 1
key Tab; e2e_type "$DSN2"; key Tab; e2e_type "echo \"$PW\""; key Enter
check "name 以 . 开头：报名字的规则，焦点回到 name" eval 'ferr_is "name = \".x\"：不能含 / 或 \\，也不能以 . 开头" && on_field 0'
clr; e2e_type doraemon; key Enter
check "和已有的连接重名：「name = \"doraemon\" 重复了」" eval 'ferr_is "name = \"doraemon\" 重复了" && on_field 0'
clr; e2e_type extra; key BTab
check "S-Tab 从 name 绕回 read_only" on_field 4
key Space
check "read_only 上 space：切成 true" eval '[[ $(frows | sed -n 5p | tr -s " ") == "read_only true" ]]'
key Tab
check "Tab 从 read_only 绕回 name" on_field 0
key Enter; sleep 0.3
check "↵ 保存：表单关掉；原来的内容一字不差留在前面（注释、末尾没换行的最后一行补了换行）" eval '! form_open && n=$(wc -c < "$D/c/connections.toml") && cmp -s <(head -c $n "$CT") "$D/c/connections.toml" && [[ $(tail -c +$((n + 1)) "$CT" | head -c 1 | xxd -p) == 0a ]]'
check "追加的一段：name / engine / dsn / password_cmd / read_only = true；权限还是 0600" eval 'a=$(tail -c +$(( $(wc -c < "$D/c/connections.toml") + 1 )) "$CT"); [[ $a == *"[[connection]]"* && $a == *"name = \"extra\""* && $a == *"engine = \"postgres\""* && $a == *"dsn = \"$DSN2\""* && $a == *password_cmd*"echo "*"$PW"* && $a == *"read_only = true"* && $(stat -f %Lp "$CT") == 600 ]] || { echo "  appended: $a"; false; }'
nrow() { local y; for y in $(seq 3 30); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }   # 侧栏里 NAME 的行
exy() { nrow extra; }
check "1 的树里多出 extra：未连接（▸），保存时没去连" eval 'y=$(exy); [[ -n $y && $(e2e_text 1 32 "$y") == *"▸"* && $(conns "$APP2") == 0 ]]'
sl; key k; key Enter
check "doraemon 的树里也有 extra" eval 'sname_is doraemon && [[ -n $(exy) ]]'
y=$(exy); e2e_click "$(e2e_find extra "$y" | cut -d' ' -f1)" "$y"; wait_for 6 eval '[[ $(conns "$APP2") == 2 ]]'; sleep 0.5
check "单击 extra：这时才去连，连上后展开" eval '[[ $(conns "$APP2") == 2 && $(e2e_text 1 32 "$(exy)") == *"▾"* ]]'
sl; key n
check "树光标在 extra 上（当前连接是 extra）时新建 session：toast「正在连接 extra…」" toast_is "正在连接 extra…"
wait_for 6 eval '[[ $(sname) == 2 ]]'; sleep 0.5
check "新 session 2 只连 extra 并展开它，doraemon 没连（▸）；console_n 也在 extra 上" eval 'sname_is 2 && [[ $(e2e_text 1 32 "$(exy)") == *"▾"* && $(e2e_text 1 32 "$(nrow doraemon)") == *"▸"* && $(conns "$APP2") == 4 && $(conns) == 4 ]] && e2e_text 105 160 1 | grep -q " extra "'
mkdir -p "$D/c2"; cp -p "$CT" "$D/c2/connections.toml"
start -C "$D/c2"
check "改过的 connections.toml 启动时照常读：树里 doraemon 之后是 extra" eval '[[ -n $(exy) ]] && (( $(nrow doraemon) < $(exy) ))'
y=$(exy); e2e_click "$(e2e_find extra "$y" | cut -d' ' -f1)" "$y"; wait_for 6 eval '[[ $(conns "$APP2") == 2 ]]'
check "重启后展开 extra：用读回来的 password_cmd（带引号）连上" eval '[[ $(conns "$APP2") == 2 ]]'

# ---- esc / C-c 取消；文件不存在时新建 0600；单击开关字段和底栏的「保存」
CT=$E2E_TMP/config/sqlmux/connections.toml; sum=$(md5 -q "$CT")
pal ">conn.new"; key Enter; e2e_type zzz; key Escape
check "esc：表单关掉，什么都没写" eval '! form_open && [[ $(md5 -q "$CT") == "$sum" ]]'
pal ">conn.new"; key Enter; e2e_type zzz; key C-c
check "C-c 等同 esc" eval '! form_open && running && [[ $(md5 -q "$CT") == "$sum" ]]'
rm "$CT"
pal ">conn.new"; key Enter; e2e_type fresh; key Tab; key Tab; e2e_type "$DSN2"
g=($(box)); e2e_click $((g[0] + 16)) "$(field_y 4)"; sleep 0.3
check "单击 read_only 的值：切成 true，焦点到它上面" eval '[[ $(frows | sed -n 5p | tr -s " ") == "read_only true" ]] && on_field 4'
x=$(e2e_find "保存" $(( g[1] + g[3] - 2 )) | cut -d' ' -f1); e2e_click "$x" $(( g[1] + g[3] - 2 )); sleep 0.4
check "单击底栏的「保存」：connections.toml 不存在就新建，权限 0600，里面只有这一条；没填的 password_cmd 不写" eval '! form_open && [[ -f $CT && $(stat -f %Lp "$CT") == 600 && $(grep -c "^\[\[connection\]\]" "$CT") == 1 ]] && grep -q "name = \"fresh\"" "$CT" && ! grep -q password_cmd "$CT"'

e2e_done
