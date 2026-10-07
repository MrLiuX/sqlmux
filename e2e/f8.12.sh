#!/usr/bin/env bash
# F8.12 session.close 在任何地方可用（specs/m8-feedback/task.md F8.12；tech-design §6.8）
# 两个 session 时关掉当前的、attach 到另一个；有未保存的修改先确认（同 session 列表里的 x）；只剩一个时走退出流程。
# session.close 没有默认键：这里绑 F8，也用面板执行一遍。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
printf '[keys.global]\n"<F8>" = "session.close"\n' >"$D/k.toml"
sname() { bar | sed 's/^ //; s/ ▾.*//'; }
box() { e2e_plain | grep -oE '[^│]*处修改未保存[^│]*' | head -1 | sed 's/^ *//; s/ *$//'; }
cmd() { pal ">$1"; key Enter; }
SOLO= start -K "$D/k.toml"; open_table t_order; wait_for 8 settled
check "（准备）一个 session doraemon，① 里 t_order" eval '[[ $(sname) == doraemon ]]'
cmd session.new; sleep 2
check "（准备）session.new：新 session 1，已经切过去" eval '[[ $(sname) == 1 ]]'
key F8; sleep 1
check "两个 session 时在 1 里按 F8（session.close）：关掉 1，attach 到 doraemon（它的 t_order 还在）" eval '[[ $(sname) == doraemon ]] && e2e_plain | head -1 | grep -q "t_order"'
key Escape

# ---- 有未保存的修改：先问（同 session 列表里的 x）
key o; sleep 0.3
cmd session.new; sleep 2
check "（准备）doraemon 里 t_order 有一条新增行没保存；新开了 session 1，已经切过去" eval '[[ $(sname) == 1 ]]'
cmd session.close; sleep 0.5
check "关一个没有修改的 session（1）：不问，直接关，回到 doraemon" eval '[[ $(sname) == doraemon ]] && [[ -z $(box) ]]'
cmd session.new; sleep 2; pal '$doraemon'; key Enter; sleep 0.8
check "（准备）又开了 session 1，再切回 doraemon（1 个 session 在后台）" eval '[[ $(sname) == doraemon ]]'
key F8; sleep 0.6
check "doraemon 里有 1 处修改没保存：F8 先问「doraemon 里有 1 处修改未保存，关闭会丢弃。」" eval '[[ $(box) == "doraemon 里有 1 处修改未保存，关闭会丢弃。" ]] || { echo "  [$(box)]"; false; }'
key n; sleep 0.4
check "n：取消，session 还在" eval '[[ $(sname) == doraemon && -z $(box) ]]'
key F8; sleep 0.6; key y; sleep 1.2
check "y：关掉 doraemon（改动丢弃），attach 到 1" eval '[[ $(sname) == 1 ]]'

# ---- 只剩一个：关掉等于退出
key F8; sleep 1.5
check "只剩一个 session：session.close 走退出流程，回到 shell" eval 'e2e_plain | grep -q "\[e2e-exit 0\]"'
e2e_done
