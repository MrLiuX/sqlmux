#!/usr/bin/env bash
# F8.21 console 文件：编号递增、树里的 Consoles 节点、↵ 打开、dd 删除（specs/m8-feedback/task.md F8.21；tech-design §11、§7.8「Consoles 节点」）
# 关掉的 console 文件留着、树里 Consoles 下还有它，↵ 重新打开内容还在；新建的编号跳过有文件的和开着的；dd 删文件（确认）之后再新建得到空的
# 同号 console；开着的 dd 被拒绝；别的节点上 dd 无事；改名后树跟着变。节点的画法由 golden 覆盖。只读，用共用库（console 只写自己的文件）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

CD=$E2E_TMP; D=
files() { ls "$E2E_TMP/data/sqlmux/consoles" 2>/dev/null | sed 's/\.sql$//' | tr '\n' ' '; }
cat_f() { cat "$E2E_TMP/data/sqlmux/consoles/$1.sql" 2>/dev/null; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
# 侧栏：「工作区」下面的 console 名（窗口里开着的，排序后）和「Consoles」下面的 名字:连接名，各一行
sections() { local y; for y in $(seq 3 43); do e2e_text 2 31 "$y"; done | python3 -c '
import re, sys
rows = [re.sub(r"^[\s▾▸-]+|\s+$", "", l) for l in sys.stdin.read().split("\n")]
def part(a, b):
    if a not in rows: return []
    r = rows[rows.index(a) + 1:]
    r = r[:next((i for i, x in enumerate(r) if x.startswith("──") or (b and x == b)), len(r))]
    return [x for x in r if x]
ws = [x for x in part("工作区", "Consoles") if x.startswith("console_") or not x.startswith(("data", "pane"))]
co = [re.sub(r"\s+", ":", x) for x in part("Consoles", None)]
print("WS=" + " ".join(sorted(x for x in ws if not x.startswith("data") and not x.startswith("pane"))) + " | CO=" + " ".join(co))'; }
ws_is() { [[ $(sections) == "WS=$1 | CO="* ]] || { echo "  sections: [$(sections)], want WS=$1"; false; }; }
co_is() { [[ $(sections) == *"| CO=$1" ]] || { echo "  sections: [$(sections)], want CO=$1"; false; }; }
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=#364a82* ]] && { e2e_text 1 32 "$y"; return; }; done; }
tree_focus() { local i; for ((i = 0; i < 6; i++)); do [[ $(focused) == 0 ]] && return 0; key C-h; done; }
tree_to() { local i; tree_focus; key G; for ((i = 0; i < 60; i++)); do [[ $(tsel) == *" $1 "* ]] && return 0; key k; done; return 1; }
newcon() { pal ">console.new"; key Enter; sleep 0.8; }
type_in() { key i; e2e_type "$1"; key Escape; sleep 1.5; }                   # 写一句，等 1 秒自动保存
box() { e2e_plain | grep -oE '删除 [^？]*？' | head -1; }
CO3="console_1:doraemon console_2:doraemon console_3:doraemon"

SOLO= start; key C-l
check "启动：Consoles 节点下有 console_1（开着，文件还没有，因为没打过字），右端是连接名 doraemon" eval 'co_is "console_1:doraemon" && ws_is "console_1"'
type_in "one"; newcon; type_in "two"; newcon; type_in "three"
check "三个 console：文件 console_1、_2、_3 都落了盘，Consoles 下三行，右端都是 doraemon" eval '[[ $(files) == "console_1 console_2 console_3 " ]] && co_is "$CO3" && ws_is "console_1 console_2 console_3"'

# ---- 关掉 console_3：文件还在，树里 Consoles 下还有它，工作区下没了；↵ 重新打开，内容还在
e2e_type ':q'; key Enter; sleep 0.8
check "关掉 console_3 的 tab：工作区下没有它，Consoles 下还在，文件还在" eval 'ws_is "console_1 console_2" && co_is "$CO3" && [[ $(cat_f console_3) == three ]]'
tree_to console_3; key Enter; sleep 1
check "Consoles 下选 console_3 ↵：重新打开，工作区下又有了，内容是 three" eval 'ws_is "console_1 console_2 console_3" && e2e_plain | grep -q "▶ *1 three" || { sections; false; }'
check "（没有另开重复的 console_3 文件，也没多出别的）" eval '[[ $(files) == "console_1 console_2 console_3 " ]]'
e2e_type ':q'; key Enter; sleep 0.8

# ---- 新建的编号跳过有文件的：console_1、_2、_3 都有文件，新建得 console_4
newcon
check "console_3 关着（文件在）时新建：得 console_4，不是 console_3" eval 'ws_is "console_1 console_2 console_4" && co_is "console_1:doraemon console_2:doraemon console_3:doraemon console_4:doraemon"'
check "console_4 没打过字：没有文件，树里 Consoles 下仍列出它（占着编号）" eval '[[ $(files) == "console_1 console_2 console_3 " ]]'
type_in "four"
check "写了字之后落盘：console_4.sql 是 four" eval '[[ $(cat_f console_4) == four ]]'
e2e_type ':q'; key Enter; sleep 0.8

# ---- dd：删文件（确认）；开着的拒绝；别的节点上无事
tree_to console_3; key d; key d; sleep 0.5
check "关着的 console_3 上 dd：弹确认「删除 console_3？」" eval '[[ $(box) == "删除 console_3？" ]] || { echo "  box: [$(box)]"; false; }'
key n; sleep 0.4
check "n：取消，文件和树里的项都在" eval '[[ -z $(box) && $(cat_f console_3) == three ]] && co_is "console_1:doraemon console_2:doraemon console_3:doraemon console_4:doraemon"'
key d; key d; sleep 0.4; key y; sleep 0.8
check "y：文件删了，Consoles 下没有 console_3 了" eval '[[ $(files) == "console_1 console_2 console_4 " ]] && co_is "console_1:doraemon console_2:doraemon console_4:doraemon"'
newcon
check "再新建：得到全新的 console_3（空内容，没有文件）" eval 'ws_is "console_1 console_2 console_3" && [[ $(files) == "console_1 console_2 console_4 " ]] && l1=$(g=($(geom $(focused))); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 1)) | tr -d " ▶"); [[ $l1 == 1 ]] || { echo "  line 1: [$l1]"; false; }'
tree_to console_1; key d; key d; sleep 0.6
check "开着的 console_1 上 dd：拒绝，toast「先关掉它的 tab」，没有确认框，文件还在" eval '[[ -z $(box) && -n $(cat_f console_1) ]] && toast_is "先关掉它的 tab"'
tree_to t_user; key d; key d; sleep 0.5
check "表节点上 dd：什么都不发生（没有确认框）" eval '[[ -z $(box) ]]'

# ---- 改名：树里跟着变
key C-l; key C-l 2>/dev/null
check "（准备）焦点在 console_3 上" eval '[[ $(tabbar $(focused)) == *console_3* ]]'
pal ">console.rename"; key Enter; sleep 0.4; for i in $(seq 12); do e2e_keys BSpace; done; e2e_type "mine"; key Enter; sleep 1
check "console.rename 改成 mine：工作区和 Consoles 下都是 mine，console_N 的排在前面，改过名的排在后面" eval 'ws_is "console_1 console_2 mine" || ws_is "console_1 mine console_2"; co_is "console_1:doraemon console_2:doraemon console_4:doraemon mine:doraemon" || { sections; false; }'
e2e_done
