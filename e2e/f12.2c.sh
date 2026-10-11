#!/usr/bin/env bash
# F12.2 值编辑器：鼠标和大 JSON（PG）（specs/m12-grid/task.md F12.2；tech-design §10「值编辑器」）
# 鼠标：点浮窗里的字只动值编辑器的光标，不动底下表格的光标；在浮窗上滚，滚的是值编辑器，底下的表不动；点浮窗外不关、不切焦点。
# 大 JSON（约 1MB，15 万行）：打开要多久，按住 j 和打字有多慢——这几个只记数字（echo），上限放得很宽，不是性能验收。自建库 t_long。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/valeditor.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q >/dev/null <<'SQL'
create table t_long (id int primary key, doc jsonb);
insert into t_long select g, case when g = 1 then (select jsonb_agg(n) from generate_series(1, 80) n) else '{"a":1}'::jsonb end from generate_series(1, 100) g;
create table t_big (id int primary key, doc jsonb);
insert into t_big select 1, (select jsonb_agg(n) from generate_series(1, 150000) n);
SQL
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
cur() { echo "$(e2e_flag cursor_x),$(e2e_flag cursor_y)"; }
first_id() { e2e_text 35 159 $(( $(grid_y) + 1 )) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }
topline() { pop_text | sed -n 1p | grep -oE '^ *[0-9]+' | tr -d ' '; }

# ---- 鼠标
SOLO=1 start -C "$D/own"; open_table t_long; wait_for 8 settled
key l; key e; sleep 0.7
check "（准备）浮窗打开，80 个数字的数组格式化成 82 行，光标在 1 行；底下表格的光标在 1,2" eval 'pop_open && [[ $(topline) == 1 ]] && pos_is 1,2'
b=($(popbox)); F0=$(focused)
e2e_click $((b[0] + 12)) $((b[1] + 6)); sleep 0.4
check "点浮窗第 6 行：编辑器光标移到那一行（终端光标在第 $((b[1] + 6)) 行），浮窗还在" eval 'pop_open && [[ $(e2e_flag cursor_y) == $((b[1] + 5)) ]]'
for i in 1 2 3 4 5; do e2e_wheel $((b[0] + 12)) $((b[1] + 6)) down; sleep 0.1; done; sleep 0.4
check "在浮窗上滚 5 格：值编辑器滚了（第一行的行号是 16），浮窗还在" eval 'pop_open && [[ $(topline) == 16 ]] || { echo "  top: $(topline)"; false; }'
e2e_click 5 10; sleep 0.4
check "点浮窗外（侧栏）：浮窗不关，焦点还在原来的 pane" eval 'pop_open && [[ $(focused) == "$F0" ]]'
key Escape; sleep 0.5
check "esc 关浮窗之后：底下的表格没动——光标还是 1,2，第一行还是 id 1（点击和滚轮没有漏下去）" eval '! pop_open && pos_is 1,2 && [[ $(first_id) == 1 ]]'
_e2e_kill

# ---- 大 JSON：打开、按住 j、打字
SOLO=1 start -C "$D/own"; open_table t_big; wait_for 15 settled
key l; e2e_keys e
python3 - "$E2E_SOCK" <<'PY' >"$D/big.txt"
import re, subprocess, sys, time
sock = sys.argv[1]
def tmux(*a): return subprocess.run(["tmux", "-L", sock, *a], capture_output=True, text=True).stdout
t0 = time.monotonic()
while time.monotonic() - t0 < 30 and not re.search(r"(doc|id) · jsonb ─", tmux("capture-pane", "-p", "-t", "t")): time.sleep(0.02)
print("open_ms", int((time.monotonic() - t0) * 1000))
t0 = time.monotonic()
tmux("send-keys", "-t", "t", *(["j"] * 200))
while time.monotonic() - t0 < 60 and not re.search(r"\b201\s", tmux("capture-pane", "-p", "-t", "t")): time.sleep(0.01)
print("j200_ms", int((time.monotonic() - t0) * 1000))
tmux("send-keys", "-t", "t", "A")   # insert at the end of the line
time.sleep(0.5)
lat = []
for i in range(20):
    x0 = tmux("display", "-p", "-t", "t", "#{cursor_x}").strip()
    t0 = time.monotonic()
    tmux("send-keys", "-t", "t", "-l", "x")
    while time.monotonic() - t0 < 10 and tmux("display", "-p", "-t", "t", "#{cursor_x}").strip() == x0: time.sleep(0.005)
    lat.append(int((time.monotonic() - t0) * 1000))
    time.sleep(0.05)
lat.sort()
print("type_ms median", lat[len(lat) // 2], "max", lat[-1])
PY
cat "$D/big.txt" | sed 's/^/  大 JSON：/'
OPEN=$(awk '$1 == "open_ms" { print $2 }' "$D/big.txt"); J200=$(awk '$1 == "j200_ms" { print $2 }' "$D/big.txt"); TMED=$(awk '$1 == "type_ms" { print $3 }' "$D/big.txt")
check "大 JSON（150000 个数字，约 1MB）：浮窗 15 秒内打开（实测 ${OPEN}ms）" eval '[[ -n $OPEN ]] && (( OPEN < 15000 ))'
check "一口气 200 个 j 在 20 秒内排空（实测 ${J200}ms）" eval '[[ -n $J200 ]] && (( J200 < 20000 ))'
check "打字：一个字符从按下到光标动的中位数 < 1 秒（实测 ${TMED}ms）" eval '[[ -n $TMED ]] && (( TMED < 1000 ))'
e2e_done
