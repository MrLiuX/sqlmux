#!/usr/bin/env bash
# F8.11 window 名不重复、默认名带序号；session 和 query 也能重命名（specs/m8-feedback/task.md F8.11；tech-design §5、§6.8、§11）
# 单测盖了命名规则、state.json 的键和布局文件的 name / 旧 n；这里在真实终端里测：三个 window 名字各不相同、重命名成已有的名字被拒绝（输入框留着）、
# session.rename、query.rename（改的是文件名，tab、标题、树、面板都跟着变，重启后内容还在，重名 / 非法名被拒绝，焦点不在 query 上什么都不做）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
CD=$D/data/sqlmux/queries
wins() { bar | sed -E 's/^ [^▾]*▾  //; s/ {3,}.*//'; }
wins_are() { [[ $(wins) == "$1" ]] || { echo "  windows: '$(wins)', want '$1'"; false; }; }
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.4; }
cmd() { pal ">$1"; key Enter; }
clr() { local i; for ((i = 0; i < 30; i++)); do e2e_keys BSpace; done; sleep 0.2; }
toast_text() { e2e_text 1 160 44; }
sname() { bar | sed 's/^ //; s/ ▾.*//'; }

# ---- window：默认名带序号
start
leader c; leader c
check "连开三个 window：data、data-2、data-3（状态栏上各不相同）" wins_are "0: data  1: data-2  2: data-3*"
cmd window.rename; clr; e2e_type "data"; key Enter; sleep 0.4
check "重命名成同 session 里已有的名字（data）：toast「已有同名 window」，输入框留着（还在 INSERT，名字是 data）" eval 'toast_text | grep -q "已有同名 window" && mode_is INSERT && [[ $(bar) == *"2: data"* ]]'
key Escape
check "esc 放弃：名字不变" wins_are "0: data  1: data-2  2: data-3*"
cmd window.rename; clr; e2e_type "data-2"; key Enter; sleep 0.4
check "改成 data-2（别的 window 的名字）也被拒绝" eval 'toast_text | grep -q "已有同名 window" && mode_is INSERT'
clr; e2e_type "report"; key Enter
check "改成没用过的名字：成功" wins_are "0: data  1: data-2  2: report*"
leader c
check "新建的 window 取最小的没用过的序号：data-3（原来的 data-3 已经改名了）" wins_are "0: data  1: data-2  2: report  3: data-3*"

# ---- session.rename
cmd session.rename
check "session.rename：状态栏变成输入框「session 名：doraemon」" eval '[[ $(bar) == *"session 名：doraemon"* ]] && mode_is INSERT'
clr; e2e_type "work"; key Enter; sleep 0.4
check "改成 work：状态栏上的 session 名是 work" eval '[[ $(sname) == work ]] && mode_is NORMAL'
cmd session.new; sleep 0.4; key Enter; sleep 2
check "（准备）session.new：新 session 叫 1，已经切过去" eval '[[ $(sname) == 1 ]]'
cmd session.rename; clr; e2e_type "work"; key Enter; sleep 0.4
check "改成已有的 session 名 work：toast「已有同名 session」，输入框留着" eval 'toast_text | grep -q "已有同名 session" && [[ $(bar) == *"session 名："* ]]'
key Escape
check "esc 放弃：名字还是 1" eval '[[ $(sname) == 1 ]]'

cmd layout.save
check "layout.save 的输入也只占状态栏左边，右边照常画 INSERT 模式块（同 window.rename）" eval 'mode_is INSERT'
key Escape

# ---- query.rename：改的是文件名
mkdir -p "$D/c"
SOLO= start -C "$D/c" -D "$D/data" -S "$D/state"; key C-l; key i; e2e_type "select 1 as marker"; key Escape; sleep 1.5
check "（准备）query_1.sql 在 queries 目录里，内容是 select 1 as marker" eval '[[ "$(cat "$CD/query_1.sql")" == "select 1 as marker" ]]'
cmd query.rename
check "query.rename：状态栏变成输入框，原来的名字 query_1 在里面" eval '[[ $(bar) == *"query 名：query_1"* ]] && mode_is INSERT'
clr; e2e_type "myquery"; key Enter; sleep 1.2
check "改成 myquery：文件改名（myquery.sql 有、query_1.sql 没有，内容在）" eval '[[ -f "$CD/myquery.sql" && ! -e "$CD/query_1.sql" && "$(cat "$CD/myquery.sql")" == "select 1 as marker" ]]'
check "tab 栏是 1:myquery，pane 标题里是 myq…（标题放不下截短）" eval 'tabbar 2 | grep -q "1:myquery" && e2e_plain | head -1 | grep -q "myq"'
check "工作区树里是 myquery，没有 query_1" eval 'e2e_text 1 32 1 >/dev/null; p=$(e2e_plain | cut -c1-33); grep -q "myquery" <<<"$p" && ! grep -q "query_1" <<<"$p"'
pal "#"; e2e_type "myquery"; sleep 0.4
check "命令面板的 Tab 范围（#myquery）里有它，没有 query_1" eval 'list | grep -q "myquery" && ! list | grep -q "query_1"'
key Escape; key Escape
# 重名 / 非法名
cmd query.new; sleep 0.8; cmd query.rename; clr; e2e_type "myquery"; key Enter; sleep 0.4
check "改成已有的 query 名（myquery）：toast「已有同名 query」，输入框留着" eval 'toast_text | grep -q "已有同名 query" && [[ $(bar) == *"query 名："* ]]'
clr; e2e_type "a/b"; key Enter; sleep 0.4
check "名字里有 /：被拒绝（输入框留着，文件没有多出来）" eval '[[ $(bar) == *"query 名："* ]] && [[ ! -e "$CD/a" ]] && ! ls "$CD" | grep -q "^a"'
clr; e2e_type "MyQuery"; key Enter; sleep 0.4
check "改成和另一个打开着的 query 只差大小写的名字（MyQuery 对 myquery）：被拒绝（大小写不敏感的文件系统上是同一个文件）" eval 'toast_text | grep -q "已有同名 query" && [[ $(bar) == *"query 名："* ]] && [[ ! -e "$CD/MyQuery.sql" || -e "$CD/myquery.sql" ]]'
clr; e2e_type ".hidden"; key Enter; sleep 0.4
check "以 . 开头：被拒绝" eval '[[ $(bar) == *"query 名："* ]] && ! ls -a "$CD" | grep -q "^\\.hidden"'
key Escape

# ---- state.json 的 queries 键跟着改名；内容在文件里（重启后读的就是它）
sc() { python3 -c "import json,sys; c = json.load(open(sys.argv[1]))['queries']; print(' '.join(sorted(c)))" "$D/state/sqlmux/state.json"; }
wait_for 6 eval '[[ $(sc) == *myquery* ]]'
check "state.json 的 queries 里记的是 myquery（改名后新建的 query 又叫 query_1，所以 query_1 也在）" eval 'python3 -c "import json,sys; c = json.load(open(sys.argv[1]))[\"queries\"]; sys.exit(0 if \"myquery\" in c else 1)" "$D/state/sqlmux/state.json" || { echo "  queries: $(sc)"; false; }'
check "myquery.sql 里还是原来的内容（重启后打开的就是它）" eval '[[ "$(cat "$CD/myquery.sql")" == "select 1 as marker" ]]'

# ---- 焦点不在 query 上：什么都不做、不 toast
key C-h; sleep 0.3; : >|"$E2E_TMP/x"; cmd query.rename; sleep 0.5
check "焦点在侧栏时 query.rename：不进输入框，也没有 toast" eval '[[ $(bar) != *"query 名"* ]] && mode_is NORMAL'
e2e_done
