#!/usr/bin/env bash
# F8.20 性能：全量移动上报下悬停行跟着指针，CPU 不到一个核的 10%（specs/m8-feedback/task.md F8.20；tech-design §2、§15「性能」）
# tmux 里每秒 100 个 SGR 移动事件（1003 模式）在表格的行上上下扫 3 秒（e2e/mouse_feed.py 按绝对时钟发，不因 tmux 调用慢而掉速），
# 最后一个事件后 100ms 内底色在指针所在的行；整个期间用 ps 的累计 CPU 时间前后差算 sqlmux 占一个核的百分比（不用 %cpu：
# macOS 上它是衰减平均）。机器忙时（1 分钟负载超过核数）阈值放宽到 20%，写在断言标题里。再测指针停在同一格上：事件照来，CPU 也低。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

HOVER=#292e42
cpu_s() { ps -o time= -p "$1" | awk -F'[:.]' '{ printf "%.2f", $1 * 60 + $2 + $3 / 100 }'; }   # 累计 CPU 秒（0:12.34）
spid() { ps -axo pid=,command= | awk -v b="$E2E_BIN" '$2 == b { print $1; exit }'; }
LOAD=$(sysctl -n vm.loadavg | awk '{ print $2 }'); NCPU=$(sysctl -n hw.ncpu)
if awk -v l="$LOAD" -v n="$NCPU" 'BEGIN { exit !(l > n) }'; then LIMIT=20 NOTE="机器忙（load $LOAD > $NCPU 核），放宽到 20%"; else LIMIT=10 NOTE="load $LOAD"; fi
bg_of() { e2e_style 60 "$1" | grep -o 'bg=#[0-9a-f]*'; }
hover_rows() { local y; for y in $(seq $((GY + 1)) $((GY + 30))); do [[ $(bg_of "$y") == "bg=$HOVER" ]] && echo "$y"; done | tr '\n' ' '; }
feed() { python3 "$E2E_ROOT/e2e/mouse_feed.py" "$E2E_SOCK" "$1" "$2" "$3" "$4" "$5"; }   # X YMIN YMAX SECS RATE

SOLO= start; open_table t_order; wait_for 8 settled; sleep 1
GY=$(grid_y); PID=$(spid)
BASE=$(hover_rows)                                            # 指针没动时就有一行是这个底色（当前行）：之后只看它之外多出来的
check "（准备）表格在 ①，找到 sqlmux 的进程（pid ${PID}），数据行从第 $((GY + 1)) 行起" eval '[[ -n $GY && -n $PID && $PID =~ ^[0-9]+$ ]]'
sleep 1

# ---- 扫过：每秒 100 个事件 × 3 秒，指针在 30 行之间来回
c0=$(cpu_s "$PID"); t0=$(date +%s.%N)
read -r SENT LAST ELAPSED < <(feed 60 $((GY + 1)) $((GY + 30)) 3 100)
sleep 0.1
c1=$(cpu_s "$PID"); t1=$(date +%s.%N)
PCT=$(awk -v a="$c0" -v b="$c1" -v t0="$t0" -v t1="$t1" 'BEGIN { printf "%.1f", (b - a) / (t1 - t0) * 100 }')
check "发出 $SENT 个事件 / ${ELAPSED}s（每秒约 100 个）" eval 'awk -v s="$SENT" -v e="$ELAPSED" '"'"'BEGIN { exit !(s / e >= 90) }'"'"''
check "最后一个事件（第 $LAST 行）后 100ms 内悬停底色在这一行，只在这一行" eval 'h=$(hover_rows); w=$(echo $BASE $LAST | tr " " "\n" | sort -nu | tr "\n" " "); [[ $h == "$w" ]] || { echo "  hover rows: [$h] want [$w] (base [$BASE])"; false; }'
check "扫过期间 sqlmux 占 CPU ${PCT}%（一个核的 ${LIMIT}% 以内；${NOTE}）" eval 'awk -v p="$PCT" -v l="$LIMIT" '"'"'BEGIN { exit !(p < l) }'"'"''

# ---- 指针停在同一格上，事件照样每秒 100 个：不做多余的工作
sleep 0.5
c0=$(cpu_s "$PID"); t0=$(date +%s.%N)
read -r SENT2 LAST2 ELAPSED2 < <(feed 60 $((GY + 10)) $((GY + 10)) 3 100)
sleep 0.1
c1=$(cpu_s "$PID"); t1=$(date +%s.%N)
PCT2=$(awk -v a="$c0" -v b="$c1" -v t0="$t0" -v t1="$t1" 'BEGIN { printf "%.1f", (b - a) / (t1 - t0) * 100 }')
check "指针停在同一格上 $SENT2 个事件：悬停行不变（第 $LAST2 行）" eval 'h=$(hover_rows); w=$(echo $BASE $LAST2 | tr " " "\n" | sort -nu | tr "\n" " "); [[ $h == "$w" ]] || { echo "  hover rows: [$h] want [$w]"; false; }'
check "停在同一格上 sqlmux 占 CPU ${PCT2}%（一个核的 ${LIMIT}% 以内；比扫过时还低或相当）" eval 'awk -v p="$PCT2" -v l="$LIMIT" '"'"'BEGIN { exit !(p < l) }'"'"''
echo "  （记录）load ${LOAD}；扫过 ${PCT}%，不动 ${PCT2}%"

# ---- 闲置：没有事件时几乎不占 CPU
sleep 0.5
c0=$(cpu_s "$PID"); t0=$(date +%s.%N); sleep 3; c1=$(cpu_s "$PID"); t1=$(date +%s.%N)
PCT3=$(awk -v a="$c0" -v b="$c1" -v t0="$t0" -v t1="$t1" 'BEGIN { printf "%.1f", (b - a) / (t1 - t0) * 100 }')
check "闲置 3 秒 sqlmux 占 CPU ${PCT3}%（1% 以内）" eval 'awk -v p="$PCT3" '"'"'BEGIN { exit !(p < 1) }'"'"''
e2e_done
