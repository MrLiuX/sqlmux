#!/usr/bin/env bash
# F8.1 补全列表打开时 ↵ 只接受，列名和表名也补空格（specs/m8-feedback/task.md F8.1；tech-design §9.7「接受」「WHERE 补全的细节」）
# 接受后补不补空格的规则有单测；这里在真实终端里测 query 和 WHERE：↵ 接受之后不换行、不执行，再按一次 ↵ 才换行 / 执行；
# 值、函数、INSERT 列清单里的列不补空格。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 4)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }
has() { [[ "|$(names)" == *"|$1|"* ]] || { echo "  candidates: $(names)"; false; }; }
line() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
cc() { echo "$(e2e_flag cursor_y):$(( $(e2e_flag cursor_x) - 103 ))"; }      # 光标所在行（0 起）和它前面的字符数
fresh() { key Escape; key Escape; key g g; key d G; key i; }
settle() { wait_for 8 settled; }

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); ST=$D/state; SJ=$ST/sqlmux/state.json; trap 'e2e_stop; rm -rf "$D"' EXIT
start; key C-l; key i

# ---- query：用户的原话 —— delete from t_event where id ↵
e2e_type "delete from t_event where id"; sleep 0.6
check "输入 …where id：列表开着，有 id" has id
key Enter; sleep 0.4
check "↵：只接受 id，不换行（仍是一行），补一个空格，光标在空格后（29），列表关了" eval '[[ $(line 1) == "delete from t_event where id" && -z $(line 2) && $(cc) == 1:29 && -z $(pop) ]] || { echo "  [$(line 1)] [$(line 2)] $(cc)"; false; }'
key Enter; sleep 0.3
check "再按一次 ↵（列表已关）：换行，新行在第一列" eval '[[ $(line 1) == "delete from t_event where id" && $(cc) == 2:0 ]]'

# ---- 表名、列名补空格；函数、值、INSERT 列清单里的列不补
fresh; e2e_type "select * from t_us"; sleep 0.6; key Enter; sleep 0.3
check "接受表名 t_user：补一个空格，不换行" eval '[[ $(line 1) == "select * from t_user" && -z $(line 2) && $(cc) == 1:21 ]]'
fresh; e2e_type "select coun"; sleep 0.6; key Enter; sleep 0.3
check "接受函数 count：不补空格，不换行" eval '[[ $(line 1) == "select count" && -z $(line 2) && $(cc) == 1:12 ]]'
fresh; e2e_type "select 1 where x is nul"; sleep 0.6; key Enter; sleep 0.3
check "接受值 null：不补空格，不换行" eval '[[ $(line 1) == "select 1 where x is null" && -z $(line 2) && $(cc) == 1:24 ]]'
fresh; e2e_type "insert into t_event(oc"; sleep 0.8
check "insert into t_event(oc：只剩 occurred_at（有前缀没有「全部列」）" has occurred_at
key Enter; sleep 0.3
check "INSERT 列清单里的列 occurred_at：不补空格（后面接 , 或 )），不换行" eval '[[ $(line 1) == "insert into t_event(occurred_at)" && -z $(line 2) && $(cc) == 1:31 ]] || { echo "  [$(line 1)] $(cc)"; false; }'

# ---- 文字不变也算接受：打完整个 null、再按 ↵ 也不换行
fresh; e2e_type "select 1 where x is null"; sleep 0.6
check "打完 null：列表还开着" eval '[[ -n $(pop) ]]'
key Enter; sleep 0.3
check "↵：文字不变，只关列表，不换行" eval '[[ $(line 1) == "select 1 where x is null" && -z $(line 2) && -z $(pop) ]]'
key Enter; sleep 0.3
check "再 ↵：换行" eval '[[ $(cc) == 2:0 ]]'
key Escape; key Escape

# ---- WHERE：同样，↵ 接受，再 ↵ 才执行
e2e_stop; SOLO=1 start -S "$ST"; open_table t_order; settle
key /; clear_in; e2e_type "paid"; sleep 0.6
check "WHERE 输入 paid：列表开着，有 paid" has paid
key Enter; sleep 0.8
check "↵：只接受 paid（补一个空格），不执行，还在 WHERE 的 INSERT，列表关了" eval 'mode_is INSERT && [[ $(where_in) == paid && -z $(pop) && $(bar) == *"editing WHERE"* ]] || { echo "  [$(where_in)]"; false; }'
check "没有报错（没发出半截的 WHERE）" eval '! screen_has "ERROR"'
key Enter; settle
check "再 ↵：执行，回到表格（NORMAL），WHERE 还是 paid" eval 'mode_is NORMAL && [[ $(where_in) == paid ]] && settled'
check "记进历史的是去掉首尾空白的 paid（不是 paid + 空格，F8.1 审查）" eval 'h=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))[\"tables\"][\"doraemon/public.t_order\"][\"history\"][0][\"where\"] + \"|\")" "$SJ"); [[ $h == "paid|" ]] || { echo "  history[0]: [$h]"; false; }'
check "结果是 paid 为 true 的行（行数少于 6000）" eval '[[ $(cnt) -lt 6000 && $(cnt) -gt 0 ]]'
key /; clear_in; e2e_type "status = 'don"; sleep 0.6; key Enter; sleep 0.6
check "WHERE 取值：status = 'don ↵ 接受 'done'，不补空格，不执行" eval 'mode_is INSERT && [[ $(where_in) == "status = '"'done'"'" && -z $(pop) ]]'
key Enter; settle
check "再 ↵：执行" eval 'mode_is NORMAL && settled'

e2e_done
