#!/usr/bin/env bash
# F17.9 生产连接（specs/m17-polish-exec/task.md F17.9；tech-design §13「生产连接」、§14）
# production = true：pane 边框、状态栏的连接块、tab 标题用 production 色（默认同 error，主题可覆盖）；这个连接上每一次写都先确认
# （表格保存 = F17.10 的预览框、query 里判为写的语句、Redis 的写命令、树上的删表 / 删键 / 改名 / 设 TTL），不看 confirm_dangerous，
# 确认框标题「<连接名> 是生产连接」；没写 read_only 的生产连接默认只读；命令行 URL / 文件的临时连接没有标记；连接表单有 production 开关。
# SQLite 文件里做 SQL 的部分（不用后端），Redis 自己起 redis-server。config.toml 里 preview_save = false：生产连接上关不掉。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/cellops.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
lite() { sqlite3 "$DB" "$1"; }
redis_start R || exit 1; redis_seed
RED=#f7768e
mkdir -p "$D/c" "$D/th/themes"
(umask 077; cat >"$D/c/connections.toml" <<TOML
[[connection]]
name = "shop"
engine = "sqlite"
dsn = "$DB"

[[connection]]
name = "prod"
engine = "sqlite"
dsn = "$DB"
production = true
read_only = false

[[connection]]
name = "prodro"
engine = "sqlite"
dsn = "$DB"
production = true

[[connection]]
name = "rd"
engine = "redis"
dsn = "redis://localhost:$RPORT/0"

[[connection]]
name = "rdp"
engine = "redis"
dsn = "redis://localhost:$RPORT/0"
production = true
read_only = false
TOML
)
printf '[grid]\npreview_save = false\n' >"$D/c/config.toml"
cp "$D/c/connections.toml" "$D/th/connections.toml"; printf 'theme = "x"\n\n[grid]\npreview_save = false\n' >"$D/th/config.toml"; printf 'production = "#ff8800"\n' >"$D/th/themes/x.toml"
edit() { key Enter; e2e_type "$1"; sleep 0.3; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
sstart() { e2e_start -C "${DIR:-$D/c}" "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2; }
fg_of() { e2e_style "$1" "$2" | grep -o 'fg=#[0-9a-f]*' | cut -c4-; }
# the three marks of the connection NAME: a pane's border and its active tab's title, the status bar's connection block, colors joined
marks() { local g x; g=($(geom "${2:-1}")); x=$(e2e_find "$1 ·" "$(H)" | cut -d' ' -f1)
  echo "$(fg_of "${g[0]}" $((g[1] + 2))) $(fg_of $((g[0] + 2)) $((g[1] + g[3] - 2))) $(fg_of "$x" "$(H)")"; }   # NAME [PANE]: PANE is 1 unless the connection's pane is the query ②
box_has() { e2e_plain | grep -qF -- "$1"; }
confirm_open() { e2e_plain | grep -qE "y (执行|保存|删除|改名|设置)   n 取消"; }
runq() { e2e_click 110 6; sleep 0.3; key Escape; key g g; key d G; key i; e2e_type "$1"; key Escape; key Enter; sleep 1.3; }     # the query pane ②: its text replaced by one statement, the cursor's line run
side() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[-] ?", "", l[1:33]).rstrip("│ ")) for l in sys.stdin.read().split("\n")]'; }
nth_row() { side | python3 -c "
import re, sys
n = int(sys.argv[2])
for i, l in enumerate(sys.stdin.read().split('\\n')):
    if re.search(sys.argv[1], l.strip()):
        n -= 1
        if n == 0: print(i + 1); break" "$1" "${2:-1}"; }
click_row() { local y; y=$(nth_row "$@"); [[ -n $y ]] || return 1; e2e_click 14 "$y"; sleep 0.5; }

# ---- 对照：普通连接 shop：没有标记，写不额外问
sstart shop
open_table t_user; wait_for 8 settled
check "shop（普通连接）：焦点 pane 的边框、tab 标题、状态栏的连接块都不是 production 色（$RED）" eval '[[ $(marks shop) != *"$RED"* ]] || { marks shop; false; }'
key l; edit zed; key Enter; sleep 0.3; key C-s; sleep 1.5
check "shop：preview_save = false，C-s 直接存（没有任何确认框），库里 zed" eval '! confirm_open && [[ $(lite "select name from t_user where id = 1") == zed ]]'
runq "update t_user set name = 'x1' where id = 2"
check "shop：有 WHERE 的 update 直接跑（confirm_dangerous = false，没有确认框）" eval '! confirm_open && [[ $(lite "select name from t_user where id = 2") == x1 ]] || { e2e_plain | sed -n 14,24p | cut -c30-150; false; }'
y=$(nth_row '^▸ +t_aud( |$)'); rclick 10 "$y"; sleep 0.5; menu_click "删除表…"; sleep 0.8
check "shop：删表的确认框只有「删除表 main.t_aud，不能撤销」，没有「是生产连接」那一行" eval 'confirm_open && box_has "DROP TABLE \"main\".\"t_aud\"" && ! box_has "是生产连接" || { e2e_plain | sed -n 12,26p | cut -c30-150; false; }'
key n; sleep 0.4
quit_app
_e2e_kill

# ---- prod：production = true、read_only = false
lite "update t_user set name = 'alice' where id = 1; update t_user set name = 'bob' where id = 2"
sstart prod
open_table t_user; wait_for 8 settled
check "prod：焦点 pane 的边框、tab 标题、状态栏的连接块都是 production 色（默认同 error，$RED）" eval '[[ $(marks prod) == "$RED $RED $RED" ]] || { marks prod; false; }'
key Escape; e2e_keys Space; e2e_type z; sleep 0.8
check "prod：pane 放大（SPC z）时边框也是 production 色" eval '[[ $(marks prod | cut -d" " -f1) == "$RED" ]] || { marks prod; e2e_panes | head -3; false; }'
e2e_keys Space; e2e_type z; sleep 0.6
key l; edit zed; key Enter; sleep 0.3; key C-s; sleep 1.5
check "prod：改一格 C-s：弹框，标题「prod 是生产连接」，正文是预览「保存到 t_user：改 1 行」和语句（preview_save = false 也一样，关不掉），库里还没动" eval 'confirm_open && box_has "prod 是生产连接" && box_has "保存到 t_user：改 1 行" && box_has "update \"main\".\"t_user\" set \"name\" = '"'zed'"'" && [[ $(lite "select name from t_user where id = 1") == alice ]] || { e2e_plain | sed -n 12,28p | cut -c25-150; false; }'
key n; sleep 0.6
check "n：什么都没发，库还是 alice，框关了" eval '! confirm_open && [[ $(lite "select name from t_user where id = 1") == alice ]]'
key C-s; sleep 1.0; key y; sleep 1.5
check "再 C-s、y：写进去了" eval '[[ $(lite "select name from t_user where id = 1") == zed ]]'
runq "update t_user set name = 'x2' where id = 2"
check "prod：有 WHERE 的 update 也先问：标题「prod 是生产连接」，「这次执行有 1 条写语句」，列出语句；库还没动" eval 'confirm_open && box_has "prod 是生产连接" && box_has "这次执行有 1 条写语句" && box_has "update t_user set name = '"'x2'"' where id = 2" && [[ $(lite "select name from t_user where id = 2") == bob ]] || { e2e_plain | sed -n 12,28p | cut -c25-150; false; }'
key n; sleep 0.6
check "n：库没动（bob），框关了" eval '! confirm_open && [[ $(lite "select name from t_user where id = 2") == bob ]]'
runq "update t_user set name = 'x2' where id = 2"; key y; sleep 1.3
check "y：执行，库里 x2" eval '[[ $(lite "select name from t_user where id = 2") == x2 ]]'
runq "select count(*) from t_user"
check "prod：select 不问" eval '! confirm_open'
y=$(nth_row '^▸ +t_aud( |$)'); rclick 10 "$y"; sleep 0.5; menu_click "删除表…"; sleep 0.8
check "prod：树上删表：标题「prod 是生产连接」，正文是 DROP TABLE \"main\".\"t_aud\"" eval 'confirm_open && box_has "prod 是生产连接" && box_has "DROP TABLE \"main\".\"t_aud\"" || { e2e_plain | sed -n 12,26p | cut -c30-150; false; }'
key n; sleep 0.5
check "n：t_aud 还在" eval '[[ $(lite "select count(*) from sqlite_master where name = '"'t_aud'"'") == 1 ]]'
rclick 10 "$y"; sleep 0.5; menu_click "删除表…"; sleep 0.8; key y; sleep 1.5
check "y：t_aud 删了" eval '[[ $(lite "select count(*) from sqlite_master where name = '"'t_aud'"'") == 0 ]]'
quit_app
_e2e_kill

# ---- prodro：production = true，没写 read_only：默认只读
sstart prodro
open_table t_user; wait_for 8 settled
check "prodro：同样标出来（production 色）" eval '[[ $(marks prodro) == "$RED $RED $RED" ]] || { marks prodro; false; }'
key l; key Enter; sleep 0.5
check "prodro：没写 read_only 的生产连接默认只读：↵ 进不了编辑，toast「prodro 是只读连接」" eval 'mode_is NORMAL && e2e_plain | grep -q "prodro 是只读连接" || { e2e_plain | tail -3 | cut -c1-160; false; }'
quit_app
_e2e_kill

# ---- 主题的 production 色覆盖
DIR=$D/th sstart prod
open_table t_user; wait_for 8 settled
check "主题里 production = \"#ff8800\"：prod 的三处都是 #ff8800" eval '[[ $(marks prod) == "#ff8800 #ff8800 #ff8800" ]] || { marks prod; false; }'
quit_app
_e2e_kill

# ---- 命令行直接给的文件：临时连接，没有标记
e2e_start "$E2E_BIN $DB"; wait_for 8 flag_is alternate_on 1; sleep 2
open_table t_user; wait_for 8 settled
check "sqlmux <文件>：临时连接没有 production 标记" eval '[[ $(marks shop) != *"$RED"* ]] || { marks shop; false; }'
quit_app
_e2e_kill

# ---- Redis：生产连接 rdp 上的写
reseed() { rc flushall >/dev/null; redis_seed; }
reseed; sstart rd
click_row '^[▸▾] +str( |$)'; y=$(nth_row '^plain$')
rclick 10 "$y"; sleep 0.5; menu_click "删除…"; sleep 0.8
check "rd（普通连接）：树上删键直接执行（没有确认框），键没了" eval '! confirm_open && [[ $(rc exists str:plain) == 0 ]]' || { rc exists str:plain; e2e_plain | sed -n 12,26p | cut -c20-150; false; }
key n; sleep 0.4
quit_app
_e2e_kill
reseed; sstart rdp
click_row '^[▸▾] +str( |$)'; y=$(nth_row '^plain$')
rclick 10 "$y"; sleep 0.5; menu_click "删除…"; sleep 0.8
check "rdp：树上删键：标题「rdp 是生产连接」，「删除键 str:plain？」" eval 'confirm_open && box_has "rdp 是生产连接" && box_has "删除键 str:plain" || { e2e_plain | sed -n 12,26p | cut -c20-150; false; }'
key n; sleep 0.5
check "n：键还在" eval '[[ $(rc exists str:plain) == 1 ]]'
rclick 10 "$y"; sleep 0.5; menu_click "改名…"; sleep 0.8; e2e_type "2"; key Enter; sleep 0.8
check "rdp：改名：输入新名字后也先问：「rdp 是生产连接」「把键 str:plain 改名为 str:plain2？」" eval 'confirm_open && box_has "rdp 是生产连接" && box_has "把键 str:plain 改名为 str:plain2" || { e2e_plain | sed -n 12,26p | cut -c20-150; false; }'
key n; sleep 0.5
check "n：没改名（str:plain 还在，没有 str:plain2）" eval '[[ $(rc exists str:plain) == 1 && $(rc exists str:plain2) == 0 ]]'
rclick 10 "$y"; sleep 0.5; menu_click "设 TTL…"; sleep 0.8; e2e_type "100"; key Enter; sleep 0.8
check "rdp：设 TTL：先问「设置键 str:plain 的 TTL：100？」" eval 'confirm_open && box_has "rdp 是生产连接" && box_has "TTL：100" || { e2e_plain | sed -n 12,26p | cut -c20-150; false; }'
key y; sleep 1.2
check "y：TTL 设上了（100 秒以内）" eval '(( $(rc ttl str:plain) > 0 && $(rc ttl str:plain) <= 100 ))'
rclick 10 "$y"; sleep 0.5; menu_click "设 TTL…"; sleep 0.8; key Enter; sleep 0.8
check "rdp：设 TTL 留空（不过期）：确认写「不过期」" eval 'confirm_open && box_has "rdp 是生产连接" && box_has "不过期"' || { e2e_plain | sed -n 12,26p | cut -c20-150; false; }
key y; sleep 1.2
check "y：留空 = PERSIST，TTL 没有了（redis-cli ttl 是 -1）" eval '[[ $(rc ttl str:plain) == -1 ]] || { rc ttl str:plain; false; }'
rc expire str:plain 500 >/dev/null
rclick 10 "$y"; sleep 0.5; menu_click "设 TTL…"; sleep 0.8; for i in 1 2 3 4 5; do key BSpace; done; e2e_type "0"; key Enter; sleep 0.8
check "rdp：设 TTL 输入 0：确认也写「不过期」" eval 'confirm_open && box_has "rdp 是生产连接" && box_has "不过期" && ! box_has "TTL：0？"' || { e2e_plain | sed -n 12,26p | cut -c20-150; false; }
key y; sleep 1.2
check "y：输入 0 也是 PERSIST（ttl -1）" eval '[[ $(rc ttl str:plain) == -1 ]] || { rc ttl str:plain; false; }'
rclick 10 "$y"; sleep 0.5; menu_click "设 TTL…"; sleep 0.8; for i in 1 2 3 4 5; do key BSpace; done; e2e_type "60"; key Enter; sleep 0.8
check "rdp：设 TTL 输入 60：确认写「TTL：60」" eval 'confirm_open && box_has "rdp 是生产连接" && box_has "TTL：60"' || { e2e_plain | sed -n 12,26p | cut -c20-150; false; }
key y; sleep 1.2
check "y：EXPIRE 60（ttl 在 1 到 60 之间）" eval '(( $(rc ttl str:plain) > 0 && $(rc ttl str:plain) <= 60 ))' || { rc ttl str:plain; false; }
ny=$(nth_row '^[▸▾] +hash( |$)'); rclick 10 "$ny"; sleep 0.5; menu_click "删除…"; sleep 0.8
check "rdp：树上删命名空间：标题「rdp 是生产连接」，列出要删的命名空间" eval 'confirm_open && box_has "rdp 是生产连接" && box_has "hash"' || { e2e_plain | sed -n 12,26p | cut -c20-150; false; }
key n; sleep 0.5
check "n：hash 下面的键都还在（hash:user 还在）" eval '[[ $(rc exists hash:user) == 1 ]]'
rclick 10 "$y"; sleep 0.5; menu_click "删除…"; sleep 0.8; key y; sleep 1.2
check "删除键 y：键没了" eval '[[ $(rc exists str:plain) == 0 ]]'
runq "SET newk 1"
check "rdp：query（控制台）里的写命令 SET newk 1 也先问；库里还没有 newk" eval 'confirm_open && box_has "rdp 是生产连接" && [[ $(rc exists newk) == 0 ]] || { e2e_plain | cut -c1-170; echo "exists newk: $(rc exists newk)"; false; }'
key n; sleep 0.5
runq "SET newk 1"; key y; sleep 1.2
check "y：newk 写进去了" eval '[[ $(rc get newk) == 1 ]]'
runq "GET newk"
check "读命令 GET 不问，也不用回头确认（结果出来了）" eval '! confirm_open'
check "rdp：Redis 的生产连接也标出来（焦点的 query pane 边框、tab 标题、状态栏都是 production 色）" eval '[[ $(marks rdp 2) == "$RED $RED $RED" ]] || { marks rdp; false; }'
quit_app
_e2e_kill

# ---- 连接表单有 production 开关，存进 connections.toml
read -r PG_USER PG_PW PG_HOST PG_PORT PG_DB < <(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p.username, p.password, p.hostname, p.port, p.path[1:])' "$SQLMUX_TEST_PG")
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }
frows() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 28)) $((g[0] + g[2] - 2)) "$y" | noicon | sed 's/^ *//; s/ *$//'; done; }
clr() { local i; for ((i = 0; i < 40; i++)); do e2e_keys BSpace; done; sleep 0.2; }
E2E_NOCONN=1 e2e_start "$E2E_BIN"; wait_for 6 flag_is alternate_on 1; sleep 1.5
check "没有连接时表单自动打开：read_only [ ] 下面是 production [ ]，再下面是高级 ▸" eval '[[ $(frows | sed -n 9,11p | tr "\n" ";") == "read_only [ ];production [ ];高级 ▸;" ]] || { frows; false; }'
e2e_type "formprod"; key Tab; key Tab; key Tab; e2e_type "$PG_HOST"; key Tab; clr; e2e_type "$PG_PORT"; key Tab; e2e_type "$PG_USER"; key Tab; e2e_type "$PG_PW"; key Tab; e2e_type "$PG_DB"
key Tab; key Tab; e2e_type " "; sleep 0.3
check "Tab 到 production 上 space：切成 [x]，read_only 跟着勾上（生产连接一开始就是只读）" eval '[[ $(frows | sed -n 9,10p | tr "\n" ";") == "read_only [x];production [x];" ]] || { frows; false; }'
key Enter; sleep 2
f=$E2E_TMP/config/sqlmux/connections.toml
check "↵ 保存：connections.toml 里有 name = \"formprod\"、production = true 和 read_only = true" eval 'grep -q "name = \"formprod\"" "$f" && grep -q "production = true" "$f" && grep -q "read_only = true" "$f" || { cat "$f"; false; }'
quit_app
e2e_done
