#!/usr/bin/env bash
# F9.8 表格里看列和表的 DDL（specs/m9-feedback2/task.md F9.8；tech-design §7.5「查看 DDL」）
# grid.ddl.column（列的定义，当前单元格下方的小浮窗：列那一行、含这列的 CREATE INDEX、列注释；esc / C-c 只关浮窗，别的键先关浮窗再照常处理）、
# grid.ddl.table（表的 DDL，居中的大浮窗，2/3 宽高，j k ↑ ↓ 一行、C-d C-u 半页、gg G、esc 关）。都没有默认键：keymaps 里绑 fh / fH。
# 自建库，宽表 t_wide（30 列、c1 not null default 7 带注释、两个含 c1 的索引、表注释）。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "d"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/c/connections.toml")
printf '[keys.grid]\n"fh" = "grid.ddl.column"\n"fH" = "grid.ddl.table"\n' >"$D/k.toml"
cols=$(python3 -c 'print(", ".join("c%d int" % i for i in range(1, 31)))')
psql "$E2E_DB" -q -c "create table t_wide (id bigint primary key, $cols); create index t_wide_c1_idx on t_wide (c1); create index t_wide_c1_c3_idx on t_wide (c1, c3); alter table t_wide alter c1 set default 7, alter c1 set not null; comment on column t_wide.c1 is 'one'; comment on table t_wide is 'wide one'; insert into t_wide (id) values (1), (2)"
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
plines() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0; for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y" | sed 's/^ *//; s/ *$//'; done; }
body() { plines | sed '1d;$d'; }                                           # 浮窗里去掉上下边框的行
closed() { [[ -z $(pop) ]]; }
cur_cell() { local y x; for y in $(seq 4 20); do for x in $(seq 36 94); do [[ $(e2e_style $x $y) == *bg=#3d59a1* ]] && { echo "$x $y"; return; }; done; done; }

start -x 160 -y 45 -C "$D/c" -K "$D/k.toml"; pal "@t_wide"; sleep 1; key Enter; wait_for 8 eval '[[ -n $(grid_y) ]]'; sleep 0.6
key l
check "（准备）t_wide 打开，光标在 c1（行 1，列 2）" pos_is 1,2

# ---- 列的定义
read -r cx cy < <(cur_cell)                                              # 当前格（底色 #3d59a1）的屏幕位置，浮窗开着时它不是这个底色了
key f; key h; sleep 0.8
check "fh：弹出浮窗，标题是列名 c1" eval '[[ -n $(pop) && $(plines | head -1) == *" c1 "* ]] || { plines; false; }'
check "第一行是 DDL 里这一列那一行：c1 integer not null default 7" eval '[[ $(body | head -1) == "c1 integer not null default 7" ]] || { body; false; }'
check "下面是含 c1 的 CREATE INDEX（两条）和列注释" eval 'body | grep -q "CREATE INDEX t_wide_c1_idx ON" && body | grep -q "CREATE INDEX t_wide_c1_c3_idx ON" && body | grep -q "comment on column public.t_wide.c1 is '"'one'"';" || { body; false; }'
check "不含别的列的索引 / 注释（c3 单独的、c5 的注释都没有）" eval '! body | grep -q "c5"'
g=($(pop))
check "浮窗顶边在当前单元格下一行（y = ${cy} + 1），左边贴着单元格所在的列（在 ① 里，不在侧栏上）" eval '[[ ${g[1]} == $((cy + 1)) && ${g[0]} -ge 34 && ${g[0]} -le $((cx + 2)) ]] || { echo "  popup at ${g[*]}, cell at $cx $cy"; false; }'
key Escape; sleep 0.4
check "esc：浮窗关了，光标没动（还在 1,2）" eval 'closed && pos_is 1,2'

key f; key h; sleep 0.6; key l; sleep 0.5
check "浮窗开着时按 l：先关浮窗，再照常移动（1,3）" eval 'closed && pos_is 1,3'
key f; key h; sleep 0.6
check "c2（没有默认值、索引、注释）：浮窗只有一行 c2 integer" eval '[[ $(body | tr "\n" "|") == "c2 integer|" ]] || { body; false; }'
key C-c; sleep 0.5
check "C-c 只关浮窗，不是退出（程序还在，没有「再按一次 C-c」之外的退出）" eval 'closed && running'
key f; key h; sleep 0.6; e2e_wheel 60 12 down; sleep 0.5
check "浮窗开着时滚轮：先关浮窗，再照常滚表格（浮窗没了，程序在）" eval 'closed && running'
key j; key f; key h; sleep 0.6
check "第二行同一列也能看（行不影响列的定义）" eval '[[ $(body | head -1) == "c2 integer" ]]'
key Escape; sleep 0.3

# ---- 表的 DDL
key f; key H; sleep 1
g=($(pop))
check "fH：居中的大浮窗，宽高约窗口的 2/3（160×45 → 宽 ${g[2]}、高 ${g[3]}）" eval '(( g[2] >= 100 && g[2] <= 112 && g[3] >= 26 && g[3] <= 32 && g[0] >= 25 && g[0] <= 31 ))'
check "标题 public.t_wide，第一行 create table public.t_wide (，下面还有行：最后一行 …" eval '[[ $(plines | head -1) == *"public.t_wide"* && $(body | head -1) == "create table public.t_wide ("* && $(body | tail -1) == "…" ]] || { plines | head -3; false; }'
key j; sleep 0.3
check "j：下滚一行，第一行是 …、第二行是 id bigint not null," eval '[[ $(body | head -1) == "…" ]]'
key k; sleep 0.3
check "k：回到顶部（第一行 create table）" eval '[[ $(body | head -1) == "create table public.t_wide ("* ]]'
key C-d; sleep 0.3
check "C-d：半页（第一行 …，create table 不见了）" eval '[[ $(body | head -1) == "…" ]] && ! body | grep -q "create table public.t_wide ("'
key C-u; sleep 0.3
check "C-u：回到顶部" eval '[[ $(body | head -1) == "create table public.t_wide ("* ]]'
key G; sleep 0.4
check "G：到末尾：最后一行是列 / 表注释，CREATE INDEX 都看得到" eval 'body | grep -q "CREATE INDEX t_wide_c1_idx" && body | tail -1 | grep -q "^comment on" || { body | tail -4; false; }'
check "含表的注释和列的注释" eval 'body | grep -q "comment on table public.t_wide is '"'wide one'"';" && body | grep -q "comment on column public.t_wide.c1"'
key g; key g; sleep 0.4
check "gg：回到顶部" eval '[[ $(body | head -1) == "create table public.t_wide ("* ]]'
key Down; key Down; key Up; sleep 0.3
check "↓ ↓ ↑：一共下滚一行" eval '[[ $(body | head -1) == "…" ]]'
key Escape; sleep 0.4
check "esc：浮窗关掉，回到表格（光标位置不变）" eval 'closed && pos_is 2,3'
e2e_done
