#!/usr/bin/env bash
# F6.5 布局文件（specs/m6-config/task.md F6.5；tech-design §14「布局文件」）
# 序列化 / 反序列化、坏文件、缺项有单测；这里在真实进程上测：退出再打开恢复一样的工作现场（last.toml）、layout.save / layout.open、
# --layout 打开命名布局（名字和路径）、布局里的表被删掉、连接名和 --layout 一起给、last.toml 读不了。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/cfg" "$D/st" "$D/data"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/cfg/connections.toml")
paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
wins() { bar | sed -E 's/^ [^▾]*▾  //; s/ {3,}.*//'; }
wh() { e2e_text 36 95 2 | sed -E 's/ {2,}(([^ ]+) ){0,2}▾ *$//; s/^ *WHERE *//; s/ *$//'; }
cline() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
toast_text() { e2e_text 1 "$(e2e_flag pane_width)" $(( $(H) - 1 )); }
# startl ARGS…：带命令行参数启动（配置、state、数据目录都用 $D 里的，重启时沿用）
startl() { e2e_start -C "${CFG:-$D/cfg}" -S "$D/st" -D "$D/data" "$E2E_BIN $*"; wait_for 6 eval 'flag_is alternate_on 1 || exited'; sleep 1.5; }
quit_all() { e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.5; }
save_layout() { e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; clear_input; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "$1"; key Enter; sleep 0.6; }
# snap：当前 window 的摘要「window 列表 | ① 的 tab 栏 | WHERE | ② 的 tab 栏」
# （tab 栏里「上一个」的 - 记号不存进布局，所以比较时去掉）
snap() { echo "$(wins) | $(tabbar 1) | $(wh) | $(tabbar 2)" | sed "s/-//g"; }

# ---- 状态 A：window 0 里 ① [t_user, t_order WHERE id < 50]、② query_1 写着 select 99 as marker；window 1 是默认布局
startl; two_tabs; key /; clear_in; e2e_type "id < 50"; key Enter; wait_for 8 settled
key C-l; key i; paste "select 99 as marker"; key Escape
e2e_keys Space; e2e_type c; sleep 0.6; e2e_keys Space; e2e_type p; sleep 0.5
SNA=$(snap)
check "（准备）状态 A：两个 window，① 里 t_user / t_order（WHERE id < 50），② query_1" eval '[[ $(wins) == "0: data*  1: data-2" && $(wh) == "id < 50" ]] || { echo "  $SNA"; false; }'
save_layout a
check "layout.save：状态栏输入布局名，↵ 写 layouts/a.toml，toast「已保存布局 a」" eval 'toast_text | grep -q "已保存布局 a" && [[ -f "$E2E_TMP/config/sqlmux/layouts/a.toml" ]]'
check "文件是 TOML：version = 1，两个 [[session.window]]，表 tab 有 where，query 有 n" eval 'f="$E2E_TMP/config/sqlmux/layouts/a.toml"; grep -q "^version = 1" "$f" && [[ $(grep -c "\[\[session.window\]\]" "$f") == 2 ]] && grep -q "where = \"id < 50\"" "$f" && grep -q "kind = \"query\"" "$f"'
# 非法名字
e2e_keys C-p; sleep 0.3; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "x/y"; key Enter; sleep 0.5
check "名字含 /：toast 拒绝，不写文件" eval 'toast_text | grep -q "名字不能含" && [[ ! -e "$E2E_TMP/config/sqlmux/layouts/x/y.toml" && $(ls "$E2E_TMP/config/sqlmux/layouts") == a.toml ]]'
e2e_keys C-p; sleep 0.3; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type ".x"; key Enter; sleep 0.5
check "名字以 . 开头：同样拒绝" eval 'toast_text | grep -q "名字不能含" && [[ $(ls "$E2E_TMP/config/sqlmux/layouts") == a.toml ]]'
e2e_keys C-p; sleep 0.3; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "zz"; key Escape; sleep 0.4
check "esc 放弃：不写文件" eval '[[ $(ls "$E2E_TMP/config/sqlmux/layouts") == a.toml ]]'

# ---- 状态 B：关掉 window 1，① 里多开 t_sku；保存为 b；然后正常退出（last = B）
key Escape; e2e_keys Space; e2e_type n; sleep 0.5; e2e_keys C-p; sleep 0.3; e2e_type ">window.close"; sleep 0.4; key Enter; sleep 0.5
e2e_keys C-p; sleep 0.3; e2e_type "@t_sku"; sleep 0.3; e2e_keys C-t; sleep 0.8
SNB=$(snap)
check "（准备）状态 B：一个 window，② 里 query_1 / t_sku（关掉 window 1 后焦点在 ②）" eval '[[ $(wins) == "0: data*" && $(tabbar 2) == *"t_sku"* ]] || { echo "  $SNB"; false; }'
save_layout b
mkdir -p "$D/cfg/layouts"; cp "$E2E_TMP/config/sqlmux/layouts/"*.toml "$D/cfg/layouts/"
quit_all
check "正常退出（:qa）：写了 last.toml" eval '[[ -f "$D/st/sqlmux/last.toml" ]] && grep -q "^version = 1" "$D/st/sqlmux/last.toml"'
check "（准备）退出后回到 shell" exited
e2e_stop

# ---- 不带参数：恢复 last（状态 B）
startl
check "sqlmux 不带参数：恢复 last，window、tab、WHERE 和退出前一样（状态 B）" eval '[[ $(snap) == "$SNB" ]] || { echo "  got  $(snap)"; echo "  want $SNB"; false; }'
check "（恢复的表 tab 取到了数据）" eval 'wait_for 8 settled; [[ -n $(grid_y) ]]'
e2e_stop

# ---- --layout 名字：打开 a（两个 window、WHERE、query 里的文字）
startl --layout a
check "--layout a：两个 window，当前 window 的 ① / ② 和保存时一样" eval '[[ $(snap) == "$SNA" ]] || { echo "  got  $(snap)"; echo "  want $SNA"; false; }'
check "query 的内容回来了（select 99 as marker）" eval 'cline 1 | grep -q "select 99 as marker"'
key Escape
e2e_stop
startl --layout b
check "--layout b：状态 B" eval '[[ $(snap) == "$SNB" ]] || { echo "  got  $(snap)"; echo "  want $SNB"; false; }'
e2e_stop
startl --layout "$D/cfg/layouts/a.toml"
check "--layout 绝对路径：同样打开 a" eval '[[ $(wins) == "0: data*  1: data-2" ]]'
e2e_stop

# ---- layout.open：从默认布局里打开命名布局和 last
startl doraemon
check "sqlmux <连接名>：默认布局，不恢复 last（一个 window，① 是引导页）" eval '[[ $(wins) == "0: data*" ]] && screen_has "打开表"'
e2e_keys C-p; sleep 0.3; e2e_type ">打开布局"; sleep 0.5; key Enter; sleep 0.5
check "layout.open：下拉框（过滤框、分隔线、然后是列表）列出 last、a、b" eval 'b=($(e2e_panes | awk "\$1 == \"-\" { print \$2, \$3, \$4, \$5; exit }")); t=""; for ((y = b[1] + 1; y < b[1] + b[3] - 1; y++)); do t+=$(e2e_text $((b[0] + 2)) $((b[0] + b[2] - 3)) $y | sed "s/ *$//")","; done; [[ $t == *a,* && $t == *b,* && $t == *last* ]] || { echo "  $t"; false; }'
for ((i = 2; i < 6; i++)); do [[ $(e2e_text $((b[0] + 2)) $((b[0] + b[2] - 3)) $((b[1] + 1 + i)) | sed "s/ *$//") == a ]] && { for ((j = 2; j < i; j++)); do key Down; done; break; }; done
key Enter; sleep 2
check "选中 a：替换现有的全部 session，变成状态 A" eval 'wait_for 6 eval "[[ \$(wins) == \"0: data*  1: data-2\" ]]"; [[ $(snap) == "$SNA" ]] || { echo "  got  $(snap)"; false; }'
e2e_stop

# ---- 布局里的表被删掉：其余照常恢复，toast 一条
sed 's/table = "t_user"/table = "no_such_table"/' "$D/cfg/layouts/a.toml" >"$D/cfg/layouts/c.toml"
startl --layout c
check "布局里的 t_user 不存在：其余恢复（t_order 带 WHERE），tab 栏里没有 t_user" eval '[[ $(wh) == "id < 50" && $(tabbar 1) == *t_order* && $(tabbar 1) != *t_user* ]] || { echo "  $(snap)"; false; }'
check "有 toast 说明哪张表没有了" eval 'screen_has "no_such_table"'
e2e_stop

# ---- 连接名和 --layout 一起给：报错退出
startl --layout a doraemon; sleep 0.5
check "--layout a 加连接名：退出码 1，提示 give a connection name or --layout, not both（英文，F7.6）" eval 'screen_has "[e2e-exit 1]" && screen_has "give a connection name or --layout, not both"'
e2e_stop
startl doraemon --layout a; sleep 0.5
check "连接名在前、--layout 在后：同样退出码 1（flag 包在连接名处停止解析，给的是用法提示）" eval 'screen_has "[e2e-exit 1]"'
e2e_stop

# ---- last.toml 读不了：toast、默认布局、改名 .broken
printf 'this is [not toml' >"$D/st/sqlmux/last.toml"
startl
check "last.toml 坏了：默认布局（一个 window、引导页），程序照常运行" eval 'running && [[ $(wins) == "0: data*" ]] && screen_has "打开表"'
check "toast 提到 last.toml，坏文件改名为 last.toml.broken" eval 'screen_has "last.toml" && [[ -f "$D/st/sqlmux/last.toml.broken" && $(cat "$D/st/sqlmux/last.toml.broken") == "this is [not toml" ]]'
e2e_stop
# ---- 两个 session：退出再打开，两个都在，当前的是退出时的那个
rm -f "$D/st/sqlmux/last.toml" "$D/st/sqlmux/last.toml.broken"
sname() { bar | sed 's/^ //; s/ ▾.*//'; }
startl; open_table t_order; e2e_keys Space; e2e_type s; sleep 0.4; key n; sleep 0.4; key Enter; wait_for 8 eval '[[ $(sname) == 1 ]]'; sleep 1.5; open_table t_user
quit_all; e2e_stop
startl; sleep 1.5
check "两个 session（doraemon 和 1）恢复，当前是退出时的 1，里面开着 t_user" eval '[[ $(sname) == 1 && $(tabbar 1) == *t_user* ]] && e2e_keys Space && e2e_type s && sleep 0.5 && e2e_plain | grep -q "doraemon  1 windows" && e2e_plain | grep -q "1  1 windows"'
key Escape

# ---- 审查补的：保留名 last 不能当布局名；--layout 启动时连不上是启动错误（退出），layout.open 里连不上只是 toast
rm -f "$D/st/sqlmux/last.toml" "$D/st/sqlmux/last.toml.broken"
cp -R "$D/cfg" "$D/cfg2"
printf '\n[[connection]]\nname = "bad"\nengine = "postgres"\ndsn = "postgres://x:y@127.0.0.1:1/z?sslmode=disable&connect_timeout=2"\n' >>"$D/cfg2/connections.toml"
sed 's/connection = "doraemon"/connection = "bad"/' "$D/cfg/layouts/a.toml" >"$D/cfg2/layouts/d.toml"
CFG=$D/cfg2 startl doraemon
e2e_keys C-p; sleep 0.3; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "last"; key Enter; sleep 0.5
check "layout.save 的名字是保留名 last：toast 拒绝，layouts 里没有 last.toml" eval 'toast_text | grep -q "last" && [[ ! -e "$E2E_TMP/config/sqlmux/layouts/last.toml" ]]'
e2e_keys C-p; sleep 0.3; e2e_type ">打开布局"; sleep 0.5; key Enter; sleep 0.5; e2e_type d; sleep 0.4; key Enter; sleep 4
check "layout.open 一个启动连接连不上的布局：程序不退出，toast 提到连接 bad" eval 'running && screen_has "bad"'
CFG=$D/cfg2 startl --layout d; sleep 3
check "--layout 启动时连不上（bad）：F12.5 起也不退出，和 sqlmux <连接名> 一样进界面，toast 提到 bad" eval 'running && ! screen_has "[e2e-exit" && screen_has "bad"'
e2e_stop


e2e_done
