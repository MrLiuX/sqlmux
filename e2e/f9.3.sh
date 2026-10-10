#!/usr/bin/env bash
# F9.9 palette.session、F9.10 session.next / prev / last（specs/m9-feedback2/task.md；tech-design §6.8）
# palette.session 没有默认键，绑 F9 试：打开面板并输入 $（session 范围）；SPC ) / SPC ( 按 session 列表的顺序切、到头绕回，SPC L 在最近两个之间来回，
# 只有一个 session 时都无事。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
printf '[keys.global]\n"<F9>" = "palette.session"\n' >|"$D/k.toml"
sname() { bar | sed 's/^ //; s/ ▾.*//'; }
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.8; }
cmd() { pal ">$1"; key Enter; }
newsess() { cmd session.new; sleep 0.4; key Enter; wait_for 8 eval '[[ $(sname) == "$1" ]]'; sleep 0.5; }

SOLO= start -K "$D/k.toml"
check "（准备）一个 session doraemon" eval '[[ $(sname) == doraemon ]]'
leader ")"
check "只有一个 session：SPC ) 无事" eval '[[ $(sname) == doraemon ]]'
leader "("
check "只有一个 session：SPC ( 无事" eval '[[ $(sname) == doraemon ]]'
leader L
check "只有一个 session：SPC L 无事（没有上一次）" eval '[[ $(sname) == doraemon ]]'

# ---- palette.session
key F9; sleep 0.6
check "F9（palette.session）：命令面板打开，输入框是「$」（session 范围）" eval 'palette_open && [[ $(input) == \$ ]] || { echo "  input: [$(input)]"; false; }'
check "面板列出 session（doraemon 一行，范围是 session）" eval '[[ $(list | head -1) == *doraemon* ]] || { list | head -3; false; }'
key Escape; sleep 0.3

# ---- 三个 session：next / prev 绕回
newsess 1; newsess 2
check "（准备）三个 session：doraemon、1、2，现在在 2" eval '[[ $(sname) == 2 ]]'
leader ")"
check "SPC ) 在最后一个（2）上：绕回第一个 doraemon" eval '[[ $(sname) == doraemon ]]'
leader ")"
check "SPC ) 再按：1" eval '[[ $(sname) == 1 ]]'
leader "("
check "SPC (：回到 doraemon" eval '[[ $(sname) == doraemon ]]'
leader "("
check "SPC ( 在第一个上：绕回最后一个 2" eval '[[ $(sname) == 2 ]]'

# ---- last：最近两个之间来回
leader L
check "SPC L：回到上一次用的（刚才在 doraemon）" eval '[[ $(sname) == doraemon ]]'
leader L
check "再按 SPC L：又回到 2" eval '[[ $(sname) == 2 ]]'
leader ")"; leader ")"
check "（准备）2 → doraemon → 1" eval '[[ $(sname) == 1 ]]'
leader L
check "SPC L：上一次是 doraemon，不是 2" eval '[[ $(sname) == doraemon ]]'
e2e_done
