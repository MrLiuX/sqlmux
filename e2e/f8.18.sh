#!/usr/bin/env bash
# F8.18 新建 window / session 只有一个 pane（specs/m8-feedback/task.md F8.18；tech-design §5、§7.8「新建 session」）
# window.new（SPC c）：一个空 pane（引导页），不建 console；session.new（面板、session 列表的 n）：一个 window、一个空 pane，
# 不建 console 文件；启动时的默认布局（左树、引导页、console）不变；没有 console 时引导页上的 c 照旧新建。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

wins() { bar | sed -E 's/^ [^▾]*▾  //; s/ {3,}.*//'; }
wins_are() { [[ $(wins) == "$1" ]] || { echo "  windows: '$(wins)', want '$1'"; false; }; }
panes_are() { local got; got=$(e2e_panes | awk '$1 != "-" { print $1 }' | tr '\n' ' '); [[ $got == "$1" ]] || { echo "  panes: '$got', want '$1'"; false; }; }
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.5; }
cmd() { pal ">$1"; key Enter; }
# 侧栏里「工作区」下面的树：每个 window 和它的 pane / tab，一行一项，去掉缩进和展开符，用 / 连成一行
tree() { local y; for y in $(seq 3 43); do e2e_text 2 31 "$y"; done | python3 -c '
import re, sys
rows = [re.sub(r"\s+", " ", re.sub(r"^[\s▾▸\ue000-\uf8ff]+|\s+$", "", l)) for l in re.sub(r"[\ue000-\uf8ff]", "", sys.stdin.read()).split("\n")]
rest = rows[rows.index("工作区") + 1:] if "工作区" in rows else []
print("/".join(r for r in rest[:next((i for i, r in enumerate(rest) if r.startswith("──") or r.startswith("Consoles")), len(rest))] if r))'; }
tree_is() { [[ $(tree) == "$1" ]] || { echo "  tree: '$(tree)', want '$1'"; false; }; }
cdir() { ls "$E2E_TMP/data/sqlmux/consoles" 2>/dev/null | tr '\n' ' '; }

# ---- 启动：默认布局不变（左树、引导页、console）
start
check "启动：侧栏、① 引导页、② console_1（默认布局不变）" eval 'panes_are "0 1 2 " && tabs_are 2 "1:console_1*" && e2e_plain | grep -q "打开表"'
check "工作区树：window data 下直接是 ② console_1（F10.5：没有 pane 层，空 pane ① 不列）" tree_is "data/② console_1"
before=$(cdir)

# ---- window.new：一个空 pane
leader c
check "SPC c：新 window data-2，放在最后并切过去" wins_are "0: data  1: data-2*"
check "新 window 只有侧栏和一个 pane ①，焦点在 ①，是引导页（有「打开表」）" eval 'panes_are "0 1 " && focus_is 1 && e2e_plain | grep -q "打开表"'
check "① 里没有 console（没有 tab 条上的 console_n）" eval '[[ $(tabbar 1) != *console* ]]'
check "工作区树：新 window 下没有 tab（window 0 原样）" tree_is "data/② console_1/data-2"
check "没有建 console 文件：consoles 目录和 SPC c 之前一样" eval '[[ $(cdir) == "$before" ]] || { echo "  [$(cdir)] was [$before]"; false; }'
leader c
check "再 SPC c：data-3，同样没有 tab" eval 'wins_are "0: data  1: data-2  2: data-3*" && panes_are "0 1 " && tree_is "data/② console_1/data-2/data-3"'

# ---- 没有 console 时引导页上的 c 照旧新建（取最小没用过的 console_n，原地替换引导页）
key c; sleep 0.8
check "引导页上按 c：新建 console_2 替掉引导页，pane 还是一个" eval 'panes_are "0 1 " && tabs_are 1 "1:console_2*" && ! e2e_plain | grep -q "打开表"'
check "console_2 的文件在写盘之前不存在或为空（没有凭空多出文件）" eval '[[ ! -s "$E2E_TMP/data/sqlmux/consoles/console_2.sql" ]]'

# ---- 新 window 里照常分割、开表
leader p
check "（准备）回到 data-2（单 pane）" wins_are "0: data  1: data-2*  2: data-3"
open_table t_user
check "新 window 里开表：① 里是 t_user，仍然只有一个 pane" eval 'panes_are "0 1 " && [[ $(e2e_plain | head -1) == *" t_user ─"* ]]'
leader %
check "SPC %：分成两个 pane（新的那个是引导页）" panes_are "0 1 2 "

# ---- session.new：一个 window、一个空 pane、不建 console 文件
before=$(cdir)
cmd session.new; sleep 0.4; key Enter; wait_for 8 eval '[[ $(bar) == " 1 ▾"* ]]'; sleep 0.6
check "session.new：新 session 1，一个 window 0: data*" eval '[[ $(bar) == " 1 ▾"* ]] && wins_are "0: data*"'
check "新 session：侧栏和一个空 pane ①（引导页），没有 console" eval 'panes_are "0 1 " && focus_is 1 && e2e_plain | grep -q "打开表" && [[ $(tabbar 1) != *console* ]]'
check "工作区树：只有一个 window，下面没有 tab" tree_is "data"
check "连着当前连接并展开（侧栏有 Tables (6)），没有建 console 文件" eval 'e2e_plain | head -12 | grep -q "Tables (6)" && [[ $(cdir) == "$before" ]] || { echo "  [$(cdir)] was [$before]"; false; }'

# ---- session 列表里的 n：同样
leader s; key n; sleep 0.4; key Enter; wait_for 8 eval '[[ $(bar) == " 2 ▾"* ]]'; sleep 0.6
check "session 列表里按 n：新 session 2，同样一个 window、一个空 pane、没有 console 文件" eval '[[ $(bar) == " 2 ▾"* ]] && wins_are "0: data*" && panes_are "0 1 " && tree_is "data" && [[ $(cdir) == "$before" ]] && [[ $(tabbar 1) != *console* ]]'
check "新 session 里 c 新建 console：取最小没用过的 console_n（console_1 在 doraemon 里、console_2 在 doraemon 的 data-3 里），所以是 console_3" eval 'key c; sleep 0.8; tabs_are 1 "1:console_3*" && panes_are "0 1 "'

# ---- 原来的 session 不受影响
pal '$doraemon'; key Enter; sleep 0.5
check "切回 doraemon：停在 data-2（t_user 和分出来的引导页），原来的 window 数不变" eval 'wins_are "0: data  1: data-2*  2: data-3" && panes_are "0 1 2 "'
leader p
check "SPC p 到 window 0：启动时的默认布局原样（引导页、console_1）" eval 'wins_are "0: data*  1: data-2  2: data-3" && panes_are "0 1 2 " && tabs_are 2 "1:console_1*"'

e2e_done
