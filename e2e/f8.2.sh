#!/usr/bin/env bash
# F8.2 以 ; 结尾的行之后，新行不继承缩进（specs/m8-feedback/task.md F8.2；tech-design §7.9）
# 规则本身有编辑器单测；这里在真实终端的 console 里测 ↵（INSERT）、o、O 三个入口。nvim 没有这条规则，不进 nvim 差分。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

paste() { local s=$1; [[ $s == *\; ]] && s="${s%;}\\;"; t set-buffer -b e2e -- "$s"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }   # 结尾的 ; 要写成 \;，否则 tmux 当命令分隔符吃掉
line() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
cc() { echo "$(e2e_flag cursor_y):$(( $(e2e_flag cursor_x) - 103 ))"; }
fresh() { key Escape; key Escape; key g g; key d G; key i; }

start; key C-l; key i

fresh; paste "  1 = 1;"; key Enter
check "INSERT：'  1 = 1;' ↵ → 新行在第一列" eval '[[ $(cc) == 2:0 ]] || { echo "  $(cc)"; false; }'
fresh; paste "  1 = 1"; key Enter
check "对照：'  1 = 1'（没有 ;）↵ → 新行继承 2 格缩进" eval '[[ $(cc) == 2:2 ]] || { echo "  $(cc)"; false; }'
fresh; paste "  1 = 1;  "; key Enter
check "; 之后还有尾部空白：同样算以 ; 结尾，新行在第一列" eval '[[ $(cc) == 2:0 ]] || { echo "  $(cc)"; false; }'
fresh; paste "  1 = 1; -- c"; key Enter
check "; 之后是注释：不算以 ; 结尾（只去掉尾部空白），新行继承缩进" eval '[[ $(cc) == 2:2 ]] || { echo "  $(cc)"; false; }'

# ---- o / O
fresh; paste "  1 = 1;"; key Escape; key o
check "NORMAL o：'  1 = 1;' 之后的新行在第一列，进入 INSERT" eval '[[ $(cc) == 2:0 ]] && mode_is INSERT || { echo "  $(cc)"; false; }'
fresh; paste "  1 = 1"; key Escape; key o
check "对照：'  1 = 1' o → 继承 2 格" eval '[[ $(cc) == 2:2 ]]'
fresh; paste $'  1 = 1;\n  2 = 2'; key Escape; key O
check "NORMAL O：看它上面的那一行 —— 光标行（第 2 行）的上一行是 ';' 结尾 → 缩进 0" eval '[[ $(cc) == 2:0 ]] || { echo "  $(cc)"; false; }'
fresh; paste $'  1 = 1\n  2 = 2;'; key Escape; key O
check "O：光标行的上一行不以 ; 结尾 → 照旧继承（2）" eval '[[ $(cc) == 2:2 ]] || { echo "  $(cc)"; false; }'
key Escape; key Escape

e2e_done
