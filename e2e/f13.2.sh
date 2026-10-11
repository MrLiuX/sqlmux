#!/usr/bin/env bash
# F13.2 两个进程写同一个 query 文件不再互相覆盖（specs/m13-quality/task.md F13.2；tech-design §11）
# 文件在 sqlmux 之外被改过（另一个 sqlmux 或编辑器）：:w 第一次拒写并 toast，文件保持被外部改过的样子；再 :w 一次才覆盖（同 vim 的 W11）。没被外部改过时 :w 照常。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
F=$D/data/sqlmux/queries/query_1.sql
wcmd() { e2e_type ":"; sleep 0.3; e2e_type "w"; key Enter; sleep 0.6; }

start -D "$D/data" -S "$D/st"; sleep 1; key C-l
key i; e2e_type "select 1"; key Escape; wcmd
check "（准备）:w 写出 query_1.sql：select 1" eval '[[ $(cat "$F") == "select 1" ]]'
sleep 3.5
printf 'select 2 -- written by someone else\n' >|"$F"
key A; e2e_type " + 1"; key Escape; sleep 0.3
wcmd
check "文件被外部改过之后 :w：toast「文件在别处改过，没有保存；再保存一次会覆盖它」" eval 'screen_has "文件在别处改过"'
check "…文件还是外部写的那份，没有被覆盖" eval '[[ $(cat "$F") == "select 2 -- written by someone else" ]]'
sleep 3.5
wcmd
check "再 :w 一次：这次是确认，写成 sqlmux 里的内容（select 1 + 1）" eval '[[ $(cat "$F") == "select 1 + 1" ]] || { cat "$F"; false; }'
sleep 3.5
key A; e2e_type " + 2"; key Escape; wcmd
check "之后再改再 :w：照常写，不拒（文件没再被外部改过）" eval '[[ $(cat "$F") == "select 1 + 1 + 2" ]] && ! screen_has "文件在别处改过"'

# ---- 关闭 / 退出走同一个 flush：被外部改过的文件，:wq 第一次拒写、不退出
sleep 3.5
printf 'select 9\n' >|"$F"
key A; e2e_type " + 3"; key Escape; sleep 0.3
e2e_type ":"; sleep 0.3; e2e_type "wq"; key Enter; sleep 0.8
check ":wq 遇到外部改过的文件：拒写，文件不变（select 9），程序没有退出" eval 'running && [[ $(cat "$F") == "select 9" ]]' || cat "$F"
e2e_done
