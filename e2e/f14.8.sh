#!/usr/bin/env bash
# F14.8 gqq 之后视口横向偏了 2 格（specs/m14-sqlite/task.md F14.8）
# 一行很长的语句，光标在行尾（视口横向滚动过），gqq 把它格式化成多行：替换之后视口回到左边，第一行行首的 select 完整（以前盖住 2 个字符：「lect」），
# 关键字行都是完整的（from / where / order by），光标移动之前就是这样。用共用库，只读（只格式化，不执行）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"

qhas() { e2e_plain | python3 -c 'import sys; print(*[l.rstrip("\n")[62:] for l in sys.stdin], sep="\n")' | grep -qF -- "$1"; }
start; key C-l
setsql "select id,name from t_user where id=1 and name='a' order by id"
key g q q; sleep 1.5
check "gqq 之后（光标没动过）：第一行是完整的「1 select」，不是「1 lect」" eval 'qhas "1 select" && ! qhas "1 lect"' || e2e_plain | sed -n 1,12p | cut -c60-170
check "其余关键字行也完整：from / where / order by（不是 om / ere / der by）" eval 'qhas " from" && qhas " where" && qhas " order by" && ! qhas " om" && ! qhas " ere"'
key g g; key g q G; sleep 1.5
check "gqG 之后同样：第一行完整" eval 'qhas "1 select" && ! qhas "1 lect"'
e2e_done
