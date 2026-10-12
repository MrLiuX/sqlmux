#!/usr/bin/env bash
# F14.3 树上的对象（MySQL）（specs/m14-sqlite/task.md F14.3；tech-design §7.8「对象节点」、§8.4）：同 f14.3.sh，在共用的 MySQL 库上（只读）。
# 没有 Sequences 节点；索引名 PRIMARY；外键 ↵ 跳到目标表；函数 ↵ 在新 query 里放 SHOW CREATE FUNCTION 的结果。
# 共用库要有 fn_greet / fn_order_total 和 t_order_user_created（seed 里的 F14.3 部分，要 docker compose up -d -V 重导，或 worker 手工补过）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/own"; printf '[tree]\nwidth = 70\n' >"$D/own/config.toml"; printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$SQLMUX_TEST_MYSQL" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
tree() { e2e_plain | python3 -c "import sys; [print(l[:72].rstrip()) for l in list(sys.stdin)[:${1:-40}]]"; }
trow() { e2e_plain | python3 -c "
import sys
for i,l in enumerate(sys.stdin, 1):
    if sys.argv[1] in l[:72]: print(i); break" "$1"; }
tclick() { local y x; y=$(trow "$1"); [[ -n $y ]] || { echo "  no tree row '$1'"; return 1; }; x=$(e2e_plain | sed -n "${y}p" | python3 -c "import sys; l=sys.stdin.read(); print(max(l.find('▸'), l.find('▾')) + 1)"); e2e_click "$x" "$y"; sleep 0.5; }   # the arrow of the row that has TEXT
tleaf() { local y; y=$(trow "$1"); e2e_click 12 "$y"; sleep 0.6; }                                                                                    # the row's text: a leaf's ↵ is the click
thas() { tree 60 | grep -qF -- "$1"; }

title1() { e2e_plain | sed -n 1p | python3 -c "import sys; l = sys.stdin.read(); i = l.find('┌─ ①'); print(l[i:i + 40].strip() if i >= 0 else '')"; }
title1() { e2e_plain | sed -n 1p | python3 -c "import sys; l = sys.stdin.read(); i = l.find('┌─ ①'); print(l[i:i + 40].strip() if i >= 0 else '')"; }
start -C "$D/own"; solo
check "MySQL 没有 Sequences 节点；Functions (2)：fn_greet(who varchar(64))、fn_order_total(order_id bigint)" eval '! thas "Sequences" && thas "Functions (" && true'
tclick Functions
check "Functions 的叶子带参数签名" eval 'thas "fn_greet(who varchar(64))" && thas "fn_order_total(order_id bigint)"'
tclick t_order_item; sleep 0.5; tclick Indexes; tclick Keys
check "t_order_item：Indexes 里 PRIMARY (order_id, line_no) UNIQUE（含主键那条）" eval 'thas "PRIMARY (order_id, line_no) UNIQUE"'
check "Keys：PRIMARY (order_id, line_no)、外键 t_order_item_ibfk_1 (order_id) → sqlmux.t_order (id)" eval 'thas "PRIMARY (order_id, line_no)" && thas "t_order_item_ibfk_1 (order_id) → sqlmux.t_order (id)"'
tleaf "t_order_item_ibfk_1 ("; sleep 1; wait_for 5 settled
check "外键 ↵：打开目标表 t_order" eval '[[ $(title1) == *" t_order "* ]]' || title1
tclick t_order_item; tclick "t_order  "; sleep 0.5; tclick Indexes; sleep 0.3
check "（准备）t_order 的 Indexes 里有 t_order_user_created (user_id, created_at)" eval 'thas "t_order_user_created (user_id, created_at)"' || tree 40
tleaf "t_order_user_created"; sleep 0.8
check "索引 ↵：DDL 浮窗是那条 create index（MySQL 自己拼，带反引号的 CREATE INDEX t_order_user_created ON … (user_id, created_at)）" eval 'e2e_plain | grep -qiE "create index .?t_order_user_created.? on .*user_id.*created_at"' || e2e_plain | sed -n 1,14p | cut -c1-180
key Escape; sleep 0.3
tleaf "fn_greet("; sleep 1.2; wait_for 5 settled
check "函数 ↵：新 query 里是 SHOW CREATE FUNCTION 的定义（CREATE … FUNCTION fn_greet(…) … concat 'hello '）" eval 'e2e_plain | grep -qiE "function .?fn_greet" && screen_has "hello"' || e2e_plain | sed -n 1,10p | cut -c1-180
e2e_done
