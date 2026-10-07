#!/usr/bin/env bash
# F8.3 console.format 格式化整个文件，光标不动（specs/m8-feedback/task.md F8.3；tech-design §6.8 console.format、§9.5）
# 两条语句都要格式化（不只光标所在的那条）；格式化之后光标留在原来的行号和列号，超出时夹到行尾 / 文件末尾；一次 u 撤回。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
printf '[keys.global]\n"<F6>" = "console.format"\n' >"$D/keymaps.toml"
paste() { local s=$1; [[ $s == *\; ]] && s="${s%;}\;"; t set-buffer -b e2e -- "$s"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }   # 结尾的 ; 要写成 \;
cline() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
clines() { local i; for ((i = 1; i <= $1; i++)); do cline $i; done; }
cur() { echo "$(e2e_flag cursor_y):$(( $(e2e_flag cursor_x) - 103 ))"; }

start -K "$D/keymaps.toml"; key C-l; key i
paste $'select id,status from t_order where id=1;\ndelete from t_log where id=1;'
key Escape; key g g; key 0; key l; key l; key l; key l; key l; key l; key l
check "（准备）光标在第一行的第 7 个字符（NORMAL 夹在行末：select id,… 很长，所以是 1:7）" eval '[[ $(cur) == 1:7 ]] || { echo "  $(cur)"; false; }'
key F6; sleep 1.5
check "F6：第一条语句格式化（select / from / where 各占行）" eval 'clines 20 | grep -qx "select" && clines 20 | grep -qx "from" && clines 20 | grep -qx "where"'
check "第二条语句也格式化（不只光标所在的那条）：delete from t_log / where / id = 1; 各占行（where 出现两次）" eval '[[ $(clines 20 | grep -c "^  id = 1;$") == 2 && $(clines 20 | grep -c "^where$") == 2 && $(clines 20 | grep -c "^delete from t_log$") == 1 ]] || { clines 20; false; }'
check "光标留在原来的行号；第 1 行现在是 select（6 个字符），列号夹到行末（NORMAL 在最后一个字符上：1:5），不再移到语句开头" eval '[[ $(cur) == 1:5 ]] || { echo "  $(cur)"; false; }'
key u
check "u 一次撤回：回到两行原文" eval '[[ $(cline 1) == "select id,status from t_order where id=1;" && $(cline 2) == "delete from t_log where id=1;" ]]'

# ---- 光标在第二条语句里：格式化之后行号、列号不变
key g g; key j; key 0; key l; key l; c1=$(cur)
check "（准备）光标在第 2 行第 2 个字符" eval '[[ $c1 == 2:2 ]]'
key F6; sleep 1.5
check "光标在第二条语句里：F6 之后光标还在 2:2，没有跳到语句开头（那里是第 9 行附近）" eval '[[ $(cur) == "$c1" ]] || { echo "  $(cur) want $c1"; false; }'

# ---- 格式化之后文件变短：光标夹到文件末尾
key u; key g g; key d G; key i; paste $'select\n\n\n\n\n1;'; key Escape; key G; key 0
check "（准备）6 行，光标在最后一行（行号 6）" eval '[[ $(cur) == 6:0 ]] || { echo "  $(cur)"; false; }'
key F6; sleep 1.5
check "格式化成两行（select / 缩进的 1;），光标夹到最后一行，列号不超过行末" eval '[[ $(cline 1) == "select" && $(cline 2) == "  1;" && $(cur) == 2:0 ]] || { echo "  $(cur)"; clines 4; false; }'

e2e_done
