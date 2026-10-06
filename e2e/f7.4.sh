#!/usr/bin/env bash
# F7.3 / F7.4 新建连接表单、凭据（specs/m7-quality/task.md F7.3、F7.4；tech-design §7.8「新建连接表单」「新建连接的入口」、§13「凭据」、§14「启动时找不到连接」）
# 表单的画法有 golden，拼 dsn 的转义和校验有单测；这里在真实终端和 PG 上测：没有连接时启动、表单分字段填、密码画 *、保存后进钥匙串（lib.sh 放了假的
# security / secret-tool，不碰真的钥匙串）、connections.toml 里没有明文密码、保存后连上并展开、校验、侧栏 ＋、F5.1 留下的追加规则（原文不动、0600、
# esc、C-p、C-s）、没有钥匙串工具和钥匙串报错、dsn 里的 ${VAR}。写库的部分在自己建的库里做。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
ERROR=#f7768e
read -r PG_USER PG_PW PG_HOST PG_PORT PG_DB < <(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p.username, p.password, p.hostname, p.port, p.path[1:])' "$SQLMUX_TEST_PG")
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }
frows() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) "$y" | noicon | sed 's/^ *//; s/ *$//'; done; }   # 每行前面有字段的图标，noicon 去掉
form_open() { e2e_plain | grep -q "┌─ 新建连接"; }
field_y() { local g; g=($(box)); echo $(( g[1] + 1 + $1 )); }              # N：0 name 1 engine 2 host 3 port 4 user 5 password 6 database 7 read_only 8 高级（展开后 9 dsn 10 password_cmd）
# on_field N：焦点在第 N 个字段上：输入框有终端光标；engine（1）、高级（8）是开关，光标隐藏；read_only（7）的值是 select 底
on_field() { local g; g=($(box)); case $1 in
  1|8) [[ $(e2e_flag cursor_flag) == 0 ]] || { echo "  cursor visible on a switch ($1)"; false; } ;;
  7) style_has $((g[0] + 16)) "$(field_y 7)" bg=#364a82 ;;
  *) [[ $(( $(e2e_flag cursor_y) + 1 )) == $(field_y "$1") ]] || { echo "  cursor on row $(( $(e2e_flag cursor_y) + 1 )), field $1 is on $(field_y "$1")"; false; } ;;
  esac; }
fsep() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3]; y++)); do [[ $(e2e_text $((g[0] + 1)) $((g[0] + 3)) $y) == "───" ]] && { echo "$y"; return; }; done; }
ferr() { local g; g=($(box)); e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $(( $(fsep) - 1 )) | sed 's/ *$//'; }   # 分隔线上面那一行：校验的原因
ferr_is() { local g; g=($(box)); [[ $(ferr) == "$1" ]] && style_has $((g[0] + 2)) $(( $(fsep) - 1 )) fg=$ERROR || { echo "  error row: '$(ferr)', want '$1'"; false; }; }
clr() { local i; for ((i = 0; i < 40; i++)); do e2e_keys BSpace; done; sleep 0.2; }
# fill NAME HOST PORT USER PW DB：焦点在 name 上，一路 Tab 填下去（port 先清掉预填的）
fill() { e2e_type "$1"; key Tab; key Tab; e2e_type "$2"; key Tab; clr; e2e_type "$3"; key Tab; e2e_type "$4"; key Tab; e2e_type "$5"; key Tab; e2e_type "$6"; sleep 0.3; }
CT() { echo "$E2E_TMP/config/sqlmux/connections.toml"; }
nrow() { local y; for y in $(seq 3 30); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }   # 侧栏里 NAME 的行
toast_row() { e2e_text 1 "$(e2e_flag pane_width)" $(( $(H) - 1 )); }

# ---- 没有连接时启动：界面出来、空树一行提示、表单自动打开
E2E_NOCONN=1 e2e_start "$E2E_BIN"; wait_for 6 flag_is alternate_on 1; sleep 1.5
check "没有 connections.toml 启动：界面出来（不退出），表单自动打开" eval 'running && form_open'
check "树里是一行「还没有连接，按 ＋ 新建」" eval 'e2e_plain | sed -n 1,6p | grep -q "还没有连接，按"'
check "表单：分字段 name / engine / host / port 5432 / user / password / database / read_only [ ] / 高级 ▸，底栏有保存、取消和键位提示" eval '[[ $(frows | sed -n 1,9p | tr -s " " | tr "\n" ";") == "name;engine postgres │ mysql;host;port 5432;user;password;database;read_only [ ];高级 ▸;" ]] && frows | tail -1 | grep -q "Tab 下一项 · ↵ 保存 · esc 取消" && frows | tail -1 | grep -q "\[ 保存 \] \[ 取消 \]"'
check "光标在 name 上" on_field 0
key Escape; sleep 0.3
check "esc 关掉表单，没有连接也能用（面板能打开）" eval '! form_open && running && { e2e_keys C-p; sleep 0.4; palette_open; }'
key Escape
y=$(e2e_plain | awk '/还没有连接，按/ { print NR; exit }'); x=$(e2e_find "还没有连接" "$y" | cut -d' ' -f1)
e2e_click "$x" "$y"; sleep 0.5
check "单击空树那一行：打开表单" form_open
key Escape; sleep 0.3
e2e_click 30 2; sleep 0.5
check "单击侧栏第一行右端的 ＋：打开表单" form_open

# ---- 填好保存：密码画 *，进钥匙串，connections.toml 里没有明文密码，保存后连上并展开
fill first "$PG_HOST" "$PG_PORT" "$PG_USER" "$PG_PW" "$PG_DB"
check "密码字段输入画成 *（不是明文）：这一行有 ${#PG_PW} 个 *，没有密码本身" eval 'r=$(frows | sed -n 6p); [[ $r == *"$(printf "*%.0s" $(seq 1 ${#PG_PW}))"* && $r != *"$PG_PW"* ]] || { echo "  [$r]"; false; }'
key Enter; sleep 2.5
check "↵ 保存：表单关掉，connections.toml 新建、权限 0600、只有这一条" eval '! form_open && f=$(CT) && [[ -f $f && $(stat -f %Lp "$f") == 600 && $(grep -c "^\[\[connection\]\]" "$f") == 1 ]] && grep -q "name = \"first\"" "$f"'
check "dsn 是拼出来的（密码不进 dsn）：postgres://$PG_USER@$PG_HOST:$PG_PORT/$PG_DB…" eval 'grep -q "dsn = \"postgres://$PG_USER@$PG_HOST:$PG_PORT/$PG_DB" "$(CT)" && ! grep -q "$PG_USER:" "$(CT)"'
check "文件里没有明文密码，有 password_cmd（security find-generic-password … -s sqlmux-first -w）" eval '! grep -qE "^password *=" "$(CT)" && grep -q "password_cmd = .*sqlmux-first" "$(CT)"'
check "假钥匙串里存着密码（没有碰真的钥匙串）" eval '[[ $(cat "$E2E_TMP/keychain/sqlmux-first") == "$PG_PW" ]]'
check "保存第一个连接后连上并展开：树里有 Tables" eval 'wait_for 8 eval "e2e_plain | head -14 | grep -q \"Tables\""; e2e_plain | head -14 | grep -q "Tables"'
open_table t_user
check "能打开表（密码是 password_cmd 从钥匙串取的）" eval '[[ $(e2e_plain | head -1) == *" t_user ─"* ]]'

# ---- 校验：缺字段、端口不是数字、名字的规则、重名；焦点移到出错的字段
APP2=e2e-f74x-$$
mkdir -p "$D/c"; (umask 077; printf '# my connections, keep this comment\n[[connection]]\nname = "doraemon"   # the seed\nengine = "postgres"\ndsn = "%s"\n# the end, no newline after this line' "$SQLMUX_TEST_PG&application_name=$APP2" >"$D/c/connections.toml")
cp -p "$D/c/connections.toml" "$D/orig.toml"
start -C "$D/c"; pal ">conn.new"; key Enter
check "conn.new（命令面板里的「新建连接…」）：表单打开，光标在 name 上" eval 'form_open && on_field 0'
key Enter
check "空着 ↵：error 色「缺少 name」，焦点在 name" eval 'ferr_is "缺少 name" && on_field 0'
e2e_type ".x"; key Enter
check "name 以 . 开头：报名字的规则，焦点在 name" eval 'ferr_is "name = \".x\"：不能含 / 或 \\，也不能以 . 开头" && on_field 0'
clr; e2e_type doraemon; key Enter
check "和已有的连接重名：「name = \"doraemon\" 重复了」" eval 'ferr_is "name = \"doraemon\" 重复了" && on_field 0'
clr; e2e_type v; key Enter
check "填了 name、没填 host：「缺少 host」，焦点跳到 host" eval 'ferr_is "缺少 host" && on_field 2'
e2e_type localhost; key Enter
check "再没填 user：「缺少 user」，焦点跳到 user" eval 'ferr_is "缺少 user" && on_field 4'
e2e_type u; key Enter
check "再没填 database：「缺少 database」，焦点跳到 database" eval 'ferr_is "缺少 database" && on_field 6'
e2e_type d; key BTab; key BTab; key BTab; clr; e2e_type "12x"; key Enter
check "port 不是数字：报错，焦点跳到 port" eval 'on_field 3 && [[ -n $(ferr) ]] && ferr | grep -q port'
check "校验没通过：connections.toml 一字没变" eval 'cmp -s "$(CT)" "$D/orig.toml"'
key Escape; sleep 0.3

# ---- engine 切换时没改过的 port 跟着变；改过的不动；mysql 的 dsn 是 mysql://
pal ">conn.new"; key Enter; key Tab; key Right; sleep 0.3
check "engine 上按 →：选中 mysql，port 没改过，跟着变成 3306" eval 'frows | sed -n 2p | grep -q "postgres │ mysql" && [[ $(frows | sed -n 4p) == "port 3306" ]]'
key Left; sleep 0.3
check "← 切回 postgres：port 回到 5432" eval '[[ $(frows | sed -n 4p) == "port 5432" ]]'
key Tab; key Tab; clr; e2e_type 1234; key BTab; key BTab; key Space; sleep 0.3
check "改过 port（1234）之后再切 engine（space）：port 不动" eval '[[ $(frows | sed -n 4p) == "port 1234" ]] && frows | sed -n 2p | grep -q mysql'
key Escape; sleep 0.3

# ---- 开关：read_only 上 space；高级展开后 dsn / password_cmd；S-Tab 从 name 绕回高级
pal ">conn.new"; key Enter; key BTab
check "S-Tab 从 name 绕回最后一个字段「高级」" on_field 8
key Tab
check "Tab 从「高级」绕回 name" on_field 0
for i in 1 2 3 4 5 6 7; do key Tab; done
check "Tab 七次到 read_only（值是 select 底）" on_field 7
key Space
check "read_only 上 space：切成 [x]" eval '[[ $(frows | sed -n 8p) == "read_only [x]" ]]'
key Tab; key Space
check "「高级」上 space：展开（▾），多出 dsn 和 password_cmd 两行" eval '[[ $(frows | sed -n 9p) == "高级 ▾" ]] && frows | grep -qx "dsn" && frows | grep -qx "password_cmd"'
key Tab
check "Tab：到 dsn（输入框有光标）" on_field 9
key Escape; sleep 0.3

# ---- 高级里直接填 dsn（host / port / user / database 不参与）：保存追加一段，原文一字不动、末尾没换行的最后一行补了换行
read -r NOPW PW < <(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p._replace(netloc=p.netloc.replace(":" + p.password + "@", "@")).geturl(), p.password)' "$SQLMUX_TEST_PG")
DSN2="$NOPW&application_name=$APP2"
pal ">conn.new"; key Enter; e2e_type extra
for i in 1 2 3 4 5 6 7; do key Tab; done; key Space; key Tab; key Space
key Tab; e2e_type "$DSN2"; key Tab; e2e_type "echo \"$PW\""; key BTab; key BTab; key BTab; sleep 0.2
key Enter; sleep 0.4
check "↵ 保存：表单关掉；原来的内容一字不差留在前面（注释、末尾没换行的最后一行补了换行）" eval '! form_open && n=$(wc -c < "$D/orig.toml") && cmp -s <(head -c $n "$(CT)") "$D/orig.toml" && [[ $(tail -c +$((n + 1)) "$(CT)" | head -c 1 | xxd -p) == 0a ]]'
check "追加的一段：name = extra、engine、dsn（填的）、password_cmd（填的，带引号），权限 0600" eval 'a=$(tail -c +$(( $(wc -c < "$D/orig.toml") + 1 )) "$(CT)"); [[ $a == *"[[connection]]"* && $a == *"name = \"extra\""* && $a == *"engine = \"postgres\""* && $a == *"dsn = \"$DSN2\""* && $a == *password_cmd*"echo "*"$PW"* && $(stat -f %Lp "$(CT)") == 600 ]] || { echo "$a"; false; }'
check "树里多出 extra：未连接（▸），保存没去连（libpq 的连接数还是 0）" eval 'y=$(nrow extra); [[ -n $y && $(e2e_text 1 32 "$y") == *"▸"* && $(psql "$SQLMUX_TEST_PG" -At -c "select count(*) from pg_stat_activity where application_name = '"'$APP2'"' and pid <> pg_backend_pid()") == 2 ]]'
e2e_stop

e2e_done
