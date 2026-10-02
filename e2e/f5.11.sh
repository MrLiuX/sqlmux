#!/usr/bin/env bash
# F5.11 console 的连接与文件（specs/m5-workspace/task.md F5.11；tech-design §5、§7.8「console 标题」、§8.6、§11、§14 state.json）
# 标题的画法与让位由 golden 覆盖；这里在真实 PG 上测：用连接下拉换连接后在新连接上执行、没连上的先连再换、有事务时不让换、
# DDL 之后刷新的是这个 console 的连接、结果 tab 记着来源连接、重启后连接和 schema 都恢复。
# 自建库，三个连接：d、s 是同一个库的两个名字（s 的 password_cmd 先睡 1.5 秒），bad 连不上。连接名取短的，② 的标题才放得下。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); ST=$D/state
trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
APP_D=e2e-f511d-$$ APP_S=e2e-f511s-$$ DB=sqlmux_e2e_$$ PK=#73daca
read -r S_DSN PW < <(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p._replace(netloc=p.netloc.replace(":" + p.password + "@", "@")).geturl(), p.password)' "$E2E_DB")
mkdir -p "$D/c" "$ST"; (umask 077; cat >"$D/c/connections.toml" <<EOF
[[connection]]
name = "d"
engine = "postgres"
dsn = "$E2E_DB&application_name=$APP_D"

[[connection]]
name = "s"
engine = "postgres"
dsn = "$S_DSN&application_name=$APP_S"
password_cmd = "sleep 1.5; echo $PW"

[[connection]]
name = "bad"
engine = "postgres"
dsn = "${E2E_DB%%\?*}_nosuchdb?sslmode=disable"
EOF
)
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }    # ② console 的内容换成 SQL
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.3; }
run() { key Enter; done_run; }
ctitle() { e2e_text 105 160 1; }                                                     # ② 的标题
on() { [[ $(ctitle) == *" $1 · $DB.$2 ▾"* ]] || { echo "  ② title: '$(ctitle)', want $1 · $DB.$2"; false; }; }   # CONN SCHEMA
dd() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
dd_rows() { local g y; g=($(dd)); for ((y = g[1] + 3; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) "$y" | sed 's/ *$//'; done; }
conn_dd() { local x; x=$(e2e_find " $1 · $DB" 1 | cut -d' ' -f1); e2e_click $((x + 1)) 1; sleep 0.4; }   # CONN：点 ② 标题上的连接名
switch_to() { conn_dd "$1"; e2e_type "$2"; sleep 0.3; key Enter; }                  # FROM TO
rval() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }   # ③ 结果第一行第一列
APPSQL="select current_setting('application_name') as app"

start -C "$D/c" -S "$ST" -D "$D/data"
key C-l
check "② console_1 在启动时的连接上：d · <库>.public ▾" on d public
conn_dd d
check "点标题上的连接名：下拉框列出全部连接 d、s、bad，当前的 d 用 pk 色标出" eval '[[ $(dd_rows | tr "\n" " ") == "d s bad " ]] && g=($(dd)) && style_has $((g[0] + 2)) $((g[1] + 3)) fg=$PK || { dd_rows; false; }'
e2e_type s; sleep 0.3; key Enter
check "选还没连上的 s：下拉框关掉，先留在 d（s 在连）" eval '[[ -z $(dd) ]] && on d public'
check "连上之后才换过去：s · <库>.public ▾" eval 'wait_for 5 eval "on s public >/dev/null" || on s public'
setsql "$APPSQL"; run
check "在 s 上执行：走的是 s 的连接" eval '[[ $(rval) == "$APP_S" ]] || { rval; false; }'
key C-j; key P; key C-k                                                          # 固定这个结果 tab，之后拿它重跑

# ---- 连不上的、有事务时：留在原来的连接
switch_to s bad
check "选 bad：连不上，toast「bad：…」，console 还在 s 上" wait_for 5 eval 'screen_has "bad：" && on s public'
sleep 3.5
setsql "begin"; run
switch_to s d; sleep 0.3
check "s 的 Main 在事务里：不让换，toast「当前连接有未提交的事务，先提交或回滚」" eval 'screen_has "当前连接有未提交的事务，先提交或回滚" && on s public'
setsql "rollback"; run; sleep 3

# ---- DDL 之后刷新的是 console 所在的连接（s），d 的 catalog 不动
key g s; e2e_type agen; sleep 0.3; key Enter
check "gs 选 agentable：s · <库>.agentable ▾" on s agentable
setsql "create table t_f511 (i int)"; run; sleep 0.5
pal "@t_f511"
check "在 s 上建表：s 重新加载了 catalog，面板列出 t_f511  s.agentable；d 没刷新，不列" eval '[[ $(list | cut -d"|" -f1 | grep "^t_f511 ") == "t_f511  s.agentable  表" ]] || { list; false; }'
e2e_keys Escape; sleep 0.3

# ---- 结果 tab 记着来源连接：console 换到 d 之后，在固定的 #1 上 R 还是走 s
switch_to s d
check "换到 d（没有事务）：d · <库>.public ▾（schema 取 d 的）" eval 'wait_for 3 eval "on d public >/dev/null" || on d public'
setsql "$APPSQL"; run
check "在 d 上执行：走 d" eval '[[ $(rval) == "$APP_D" ]]'
click_tab 3 "2:"; key R; done_run
check "固定的 console_1 #1 上 R：照样在 s 上重跑（结果 tab 记着来源连接）" eval '[[ $(rval) == "$APP_S" ]] || { rval; false; }'
e2e_click 130 10; sleep 0.3                                                    # 点回 ② 的 console

# ---- 换回 s、选 agentable，重启后恢复（state.json 记 console_1 的连接和 schema）
pal ">切换连接"; e2e_keys Enter; sleep 0.4
check "命令「切换连接…」：打开 ② 的连接下拉" eval '[[ $(dd_rows | tr "\n" " ") == "d s bad " ]]'
e2e_type s; sleep 0.3; key Enter; wait_for 3 eval "on s public >/dev/null"; key g s; e2e_type agen; sleep 0.3; key Enter; sleep 1.5
check "console_1 换回 s、agentable" on s agentable
check "console 文件在 consoles/console_1.sql（不再按连接分目录）" eval '[[ -f $D/data/sqlmux/consoles/console_1.sql && ! -d $D/data/sqlmux/consoles/d ]]'
key :; e2e_type qa; key Enter; wait_for 3 screen_has "[e2e-exit 0]"
start -C "$D/c" -S "$ST" -D "$D/data"
check "重启：console_1 先挂在启动时的 d，后台连上 s 之后换回 s · <库>.agentable ▾" eval 'wait_for 6 eval "on s agentable >/dev/null" || on s agentable'
key C-l; setsql "show search_path"; run
check "恢复后执行：search_path 是 agentable 在前" eval '[[ $(rval) == "\"agentable\","* ]] || { rval; false; }'

e2e_done
