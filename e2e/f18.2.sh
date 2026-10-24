#!/usr/bin/env bash
# F18.2 e / 外部编辑器（specs/m18-windows-mssql/task.md F18.2；tech-design §7.8「导出」§11）：Windows 支持不能改 macOS / Linux 上的行为，只有一处有意的变化：
# 导出的文件名里 Windows 不允许的字符（< > : " / \ | ? * 和控制字符）在所有平台都换成 _（Redis 键 hash:big → hash_big.csv，再导一次 hash_big-2.csv，目录里没有带冒号的文件）；
# C-o / 「在 $EDITOR 中编辑」对名字里有空格、$、反引号的 query：$VISUAL 收到的就是那个文件的原样路径，名字不被展开、反引号里的命令不被执行。
# 每个脚本自己起 redis-server。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"
. "$(dirname "$0")/exporting.sh"

SOLO=1
D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
mkdir -p "$D/c"
printf '[[connection]]\nname = "rd"\nengine = "redis"\ndsn = "redis://localhost:%s/0"\n' "$RPORT" >"$D/c/connections.toml"; chmod 600 "$D/c/connections.toml"
redis_seed; rc -n 0 EVAL "for i = 1, 300 do redis.call('HSET', 'hash:big', 'field' .. i, 'value' .. i) end" 0 >/dev/null
PR=158
side() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[-] ?", "", l[1:33]).rstrip("│ ")) for l in sys.stdin.read().split("\n")]'; }
nth_row() { side | python3 -c "
import re, sys
n = int(sys.argv[2])
for i, l in enumerate(sys.stdin.read().split('\\n')):
    if re.search(sys.argv[1], l.strip()):
        n -= 1
        if n == 0: print(i + 1); break" "$1" "${2:-1}"; }
click_row() { local y; y=$(nth_row "$@"); [[ -n $y ]] || return 1; e2e_click 14 "$y"; sleep 0.5; }
expand() { click_row "^[▸▾] +$1( |\$)" "${2:-1}"; }
to_row() { local y i; y=$(nth_row "$1" "${2:-1}"); key C-h; key g g; for ((i = 4; i < y; i++)); do key j; done; }
open_key() { to_row "$1" "$2"; key Enter; sleep 1.2; }

e2e_start -C "$D/c" "$E2E_BIN"; wait_for 8 flag_is alternate_on 1; sleep 2; solo
expand hash; open_key '^big$'
check "（准备）hash:big 的键 tab 打开了" eval 'e2e_plain | sed -n 1p | grep -q "hash:big"' || e2e_plain | sed -n 1,4p | cut -c30-160
export_pick 0; exported; sleep 0.3
check "ge 导出 CSV · 当前页：文件名是 hash_big.csv，toast 里是这个文件名，目录里没有带冒号的文件" eval '[[ -f "$E2E_TMP/hash_big.csv" && $(ls "$E2E_TMP" | grep -c ":") == 0 ]] && toast_has "hash_big.csv" || { ls "$E2E_TMP"; false; }'
check "文件内容：表头 field,value，第一条 field1,value1 之类（不是空文件）" eval '[[ $(wc -l <"$E2E_TMP/hash_big.csv" | tr -d " ") -gt 100 ]]'
sleep 3.2
export_pick 0; exported; sleep 0.3
check "再导出一次：hash_big-2.csv，原来的那个还在" eval '[[ -f "$E2E_TMP/hash_big-2.csv" && -f "$E2E_TMP/hash_big.csv" ]] || { ls "$E2E_TMP"; false; }'

# ---- 名字里有空格、$、反引号的 query：C-o（query.external）时 $VISUAL 收到的是原样路径，不展开、不执行
mkdir -p "$D/data/sqlmux/queries"
QN='cost $HOME `touch PWN`'
printf 'select 1\n' >"$D/data/sqlmux/queries/$QN.sql"
printf '#!/bin/sh\nprintf "%%s\\n" "$1" >"%s/arg"\n' "$D" >"$D/ed"; chmod +x "$D/ed"
e2e_start -D "$D/data" "VISUAL=$D/ed $E2E_BIN"; wait_for 5 flag_is alternate_on 1; sleep 1.5
y=$(e2e_find "cost" "$(e2e_plain | awk '/cost \$HOME/ { print NR; exit }')" | head -1 | cut -d' ' -f1)
yy=$(e2e_plain | awk '/cost \$HOME/ { print NR; exit }')
check "（准备）树的 Queries 里列出了那个名字古怪的 query" eval '[[ -n $yy ]]'
key C-h; e2e_click 14 "$yy"; sleep 0.4; key Enter; sleep 1
check "（准备）它打开了（编辑器里是 select 1）" eval 'e2e_plain | grep -q "select 1"'
key C-p; sleep 0.3; e2e_type '>query.external'; sleep 0.4; key Enter; sleep 1.5
check "\$VISUAL 收到的参数就是那个文件的路径（名字原样：空格、\$HOME、反引号都在）" eval '[[ $(cat "$D/arg" 2>/dev/null) == */queries/"$QN.sql" ]] || { cat "$D/arg"; false; }'
check "名字没被 shell 展开：反引号里的 touch PWN 没有执行（当前目录里没有 PWN）" eval '! ls "$E2E_TMP" "$D" 2>/dev/null | grep -qx PWN && [[ ! -e PWN ]]'
check "编辑器回来之后界面恢复、还在跑" eval 'running && flag_is alternate_on 1'
e2e_done
