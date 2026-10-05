#!/usr/bin/env bash
# F5.18 结果 tab 的上限（specs/m5-workspace/task.md F5.18；tech-design §11「tab 的组织」「上限」）
# 日志之外最多 10 个结果 tab：超过时关掉最久没更新的（固定的也算），正在执行的不关；一次 12 个结果集只留最后 10 个。
# 「到达时间」的先后（先开始后跑完的长查询）需要两个 console 同时在跑，由单测覆盖。宽终端（300 列）让 11 个 tab 都放得下。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

RP=3
paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
done_run() { wait_for 20 eval '[[ $(bar) != *busy* ]]'; sleep 0.5; }
rtabs() { tabbar $RP | sed -E 's/ {3,}.*//; s/- │/ │/g; s/-$//'; }
rtabs_are() { [[ $(rtabs) == "$1" ]] || { echo "  ③ tabs: '$(rtabs)'"; echo "  want    '$1'"; false; }; }

start -x 300; key C-l
sql=; for i in $(seq 1 12); do sql+="select $i as n;"$'\n'; done
setsql "$sql"; key V; key G; key Enter; done_run
check "一次执行 12 个结果集：日志之外只留最后 10 个（#1·3 … #1·12），前两个被关掉" rtabs_are "1:日志 │ 2:console_1 #1·3* │ 3:console_1 #1·4 │ 4:console_1 #1·5 │ 5:console_1 #1·6 │ 6:console_1 #1·7 │ 7:console_1 #1·8 │ 8:console_1 #1·9 │ 9:console_1 #1·10 │ 10:console_1 #1·11 │ 11:console_1 #1·12"

# ---- 固定的也会被关：每次执行后固定，第 11 个出现时最早的（固定的）被关掉
e2e_stop; start -x 300; key C-l
for i in $(seq 1 10); do setsql "select $i as n"; key Enter; done_run; key C-j; key P; key C-k; done
check "（准备）10 个固定的结果 tab：#1 … #10" rtabs_are "1:日志 │ 2:console_1 #1 │ 3:console_1 #2 │ 4:console_1 #3 │ 5:console_1 #4 │ 6:console_1 #5 │ 7:console_1 #6 │ 8:console_1 #7 │ 9:console_1 #8 │ 10:console_1 #9 │ 11:console_1 #10*"
setsql "select 11 as n"; key Enter; done_run; key C-j; key P; key C-k
check "第 11 个：最早的 #1（固定的也算）被关掉，剩 #2 … #11，仍是 10 个" rtabs_are "1:日志 │ 2:console_1 #2 │ 3:console_1 #3 │ 4:console_1 #4 │ 5:console_1 #5 │ 6:console_1 #6 │ 7:console_1 #7 │ 8:console_1 #8 │ 9:console_1 #9 │ 10:console_1 #10 │ 11:console_1 #11*"

# ---- 正在执行的不关：第 12 个在执行中就占一个 tab，最早的关掉，它自己留着
setsql "select pg_sleep(3), 12 as n"; key Enter; sleep 1
check "执行中（占位 tab #12 在）：最早的 #2 已关掉，正在执行的没被关" rtabs_are "1:日志 │ 2:console_1 #3 │ 3:console_1 #4 │ 4:console_1 #5 │ 5:console_1 #6 │ 6:console_1 #7 │ 7:console_1 #8 │ 8:console_1 #9 │ 9:console_1 #10 │ 10:console_1 #11 │ 11:console_1 #12*"
done_run; sleep 0.5
check "执行完：#12 换成结果，还是 10 个" eval 'rtabs_are "1:日志 │ 2:console_1 #3 │ 3:console_1 #4 │ 4:console_1 #5 │ 5:console_1 #6 │ 6:console_1 #7 │ 7:console_1 #8 │ 8:console_1 #9 │ 9:console_1 #10 │ 10:console_1 #11 │ 11:console_1 #12*" && g=($(geom $RP)) && e2e_plain | grep -q " 12 "'

e2e_done
