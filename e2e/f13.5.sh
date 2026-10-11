#!/usr/bin/env bash
# F13.5 配置文件拼错的键不再静默忽略（specs/m13-quality/task.md F13.5；tech-design §14）
# config.toml 顶层写 max_rows、connections.toml 的连接里写 readonly：启动后 toast 列出键名，不阻止启动；全是认识的键时没有这条 toast。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
conn() { (umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n%s' "$SQLMUX_TEST_PG" "$1" >"$D/c/connections.toml"); }
mkdir -p "$D/c"

# ---- 认识的键：没有警告 toast
conn ""; printf '[query]\nmax_rows = 10\n' >"$D/c/config.toml"
start -C "$D/c"; sleep 1.5
check "config.toml 里是认识的键（[query] max_rows）：没有「不认识的键」" eval 'running && ! screen_has "不认识的键"'

# ---- config.toml 顶层的 max_rows（应该在 [query] 下）
conn ""; printf 'max_rows = 10\n' >"$D/c/config.toml"
start -C "$D/c"; sleep 0.8
check "config.toml 顶层写 max_rows：启动照常（没有退出），toast 是「config.toml: 不认识的键 max_rows」" eval 'running && ! screen_has "[e2e-exit" && screen_has "config.toml: 不认识的键 max_rows"'

# ---- connections.toml 里拼错 readonly（应该是 read_only）
conn 'readonly = true
'; rm -f "$D/c/config.toml"
start -C "$D/c"; sleep 0.8
check "connections.toml 里 readonly：启动照常，toast 点名键和连接（connections.toml: 不认识的键 readonly（connection \"doraemon\"））" eval 'running && screen_has "connections.toml: 不认识的键 readonly" && screen_has "doraemon"'
sleep 3.5
open_table t_user; wait_for 8 settled
key d d; sleep 0.5
check "readonly 拼错不会变成只读：dd 标删，没有「只读」提示，保存计数出现" eval '! screen_has "只读" && ! screen_has "read-only"'

# 对照：拼对的 read_only = true 才是只读（dd 有「只读」提示），说明上面那条不是空检查
conn 'read_only = true
'; rm -f "$D/c/config.toml"
start -C "$D/c"; open_table t_user; wait_for 8 settled
key d d; sleep 0.5
check "（对照）read_only = true：dd 标删被拒，提示只读" eval 'screen_has "只读" || screen_has "read-only"'

# ---- 两个文件同时拼错：各一条键名都在
conn 'readonly = true
'; printf 'max_rows = 10\n' >"$D/c/config.toml"
start -C "$D/c"; sleep 0.8
check "两个文件都有拼错的键：键名都能在 toast 里看到" eval 'screen_has "max_rows" && screen_has "readonly"'
e2e_done
