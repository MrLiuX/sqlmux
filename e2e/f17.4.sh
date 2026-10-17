#!/usr/bin/env bash
# F17.4 三处小 bug（specs/m17-polish-exec/task.md F17.4；tech-design §7.8、§6.7）
# a. 新建 session 时的「正在连接 <连接>…」toast 连上后就清掉（原来连上了还留着，切回原 session 也在）；
# b. session 的行写「1 个窗口」（单数）、「2 个窗口」，session 列表和命令面板的会话范围都一样；
# c. sqlmux keys --format toml 的注释里没有 F8.23、M5、§6.6 这类内部编号。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; kill $PX 2>/dev/null; rm -rf "$D"' EXIT
redis_start R || exit 1
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
# 一个拖了 2.5 秒才应答的 Redis（TCP 代理先睡 2.5 秒再转发）：新 session 要拨它，连接时间长一点，toast 才看得出来
PP=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
python3 - "$PP" "$RPORT" <<'PY' >/dev/null 2>&1 &
import asyncio, sys
port, up = int(sys.argv[1]), int(sys.argv[2])
async def pipe(r, w):
    try:
        while (d := await r.read(65536)): w.write(d); await w.drain()
    except Exception: pass
    finally: w.close()
async def handle(r, w):
    await asyncio.sleep(2.5)
    ur, uw = await asyncio.open_connection("127.0.0.1", up)
    await asyncio.gather(pipe(r, uw), pipe(ur, w))
async def main():
    s = await asyncio.start_server(handle, "127.0.0.1", port)
    async with s: await s.serve_forever()
asyncio.run(main())
PY
PX=$!; disown; sleep 0.7
mkdir -p "$D/c"
(umask 077; printf '[[connection]]\nname = "shop"\nengine = "sqlite"\ndsn = "%s"\n\n[[connection]]\nname = "slow"\nengine = "redis"\ndsn = "redis://127.0.0.1:%s/0"\n' "$DB" "$PP" >"$D/c/connections.toml")
sname() { e2e_plain | tail -2 | head -1 | sed 's/^ //; s/ ▾.*//'; }          # 状态栏 session 块里的名字
slist_open() { e2e_plain | grep -q "┌─ session"; }
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }   # 浮层（session 列表）的 X Y W H
srows() { local g; slist_open || return 0; g=($(box))   # 列表的每一行，3 个以上的空格压成两个，以 | 分隔
  e2e_rows $((g[0] + 2)) $((g[0] + g[2] - 2)) $((g[1] + 1)) $((g[1] + g[3] - 2)) $((g[0] + 2)) | awk -F'|' '{ sub(/ +$/, "", $1); gsub(/   +/, "  ", $1); printf "%s|", $1 }'; }
connecting() { e2e_plain | grep -q "正在连接"; }
quit_app() { e2e_type ":qa"; e2e_keys Enter; wait_for 8 exited; sleep 0.5; }

# ---- a. 新建 session：连上后「正在连接」的 toast 清掉
e2e_start -C "$D/c" "$E2E_BIN shop"; wait_for 8 flag_is alternate_on 1; sleep 2
e2e_keys Space; e2e_type s; sleep 0.5
check "（准备）session 列表：一行「shop  1 window」" eval '[[ $(srows) == "shop  1 个窗口|" ]] || { srows; false; }'
key n; sleep 0.4; key Enter
wait_for 8 eval '[[ $(sname) == 1 ]]'; sleep 1
check "attach 新 session 1，连上之后 toast「正在连接」没有了（新 session 不用等它拨完）" eval '! connecting || { e2e_plain | tail -3 | cut -c1-120; false; }'
sleep 2.5
check "再等 2.5 秒（慢的 slow 也该拨完了）：还是没有" eval '! connecting'
e2e_keys Space; e2e_type s; sleep 0.5; key k; key Enter; wait_for 6 eval '[[ $(sname) == shop ]]'; sleep 0.6
check "切回原来的 session shop：toast 也不在" eval '[[ $(sname) == shop ]] && ! connecting'

# ---- b. 单数
e2e_keys Space; e2e_type s; sleep 0.5
check "session 列表：两个 session 都是「1 个窗口」（不是「1 windows」，中文写「个窗口」）" eval '[[ $(srows) == "shop  1 个窗口|1  1 个窗口|" ]] || { srows; false; }'
key Escape; sleep 0.3
e2e_keys C-p; sleep 0.3; e2e_type '$'; sleep 0.6
check "命令面板的会话范围：也是「1 个窗口」" eval 'e2e_plain | grep -qE "shop +1 个窗口" && ! e2e_plain | grep -q "1 windows" || { e2e_plain | sed -n 6,16p | cut -c20-130; false; }'
key Escape; sleep 0.3
key Space; e2e_type c; sleep 0.8
e2e_keys Space; e2e_type s; sleep 0.5
check "shop 开了第二个 window：「2 个窗口」（复数照旧），1 个的还是「1 个窗口」" eval '[[ $(srows) == "shop  2 个窗口|1  1 个窗口|" ]] || { srows; false; }'
key Escape; sleep 0.3
quit_app
_e2e_kill

# ---- c. keys 的注释里没有内部编号
"$E2E_BIN" keys --format toml >"$D/keys.toml" 2>"$D/keys.err"
check "sqlmux keys --format toml 有内容（几百行）" eval '[[ $(wc -l <"$D/keys.toml") -gt 100 ]] || { head -3 "$D/keys.err"; false; }'
check "注释里没有 (F8.23) / (M5) 这类括号编号，也没有 §" eval '! grep -nE "\((F|M)[0-9]|§" "$D/keys.toml" || { grep -nE "\((F|M)[0-9]|§" "$D/keys.toml" | head; false; }'
check "也没有行内的 F<数字>.<数字> 编号（F3.38 这种）" eval '! grep -nE "\bF[0-9]+\.[0-9]+" "$D/keys.toml" || { grep -nE "\bF[0-9]+\.[0-9]+" "$D/keys.toml" | head; false; }'
# ---- 英文界面：窗口数是 "1 window" / "2 windows"；keys 的 toml 和 md 两种格式、两种语言里都没有内部编号
mkdir -p "$D/en/sqlmux" "$D/zh/sqlmux"; printf 'language = "en"\n' >"$D/en/sqlmux/config.toml"; printf 'language = "zh"\n' >"$D/zh/sqlmux/config.toml"
for lang in en zh; do for fmt in toml md; do
  XDG_CONFIG_HOME="$D/$lang" "$E2E_BIN" keys --format $fmt >"$D/keys.$lang.$fmt" 2>/dev/null
  check "keys --format $fmt（$lang）：有内容、没有 (F8.23) / (M5) / § 这类编号" eval '[[ $(wc -l <"$D/keys.'$lang'.'$fmt'") -gt 50 ]] && ! grep -nE "\((F|M)[0-9]|§" "$D/keys.'$lang'.'$fmt'"' || { head -3 "$D/keys.$lang.$fmt"; grep -nE "\((F|M)[0-9]|§" "$D/keys.$lang.$fmt" | head -5; false; }
done; done
mkdir -p "$D/enc"; cp "$D/c/connections.toml" "$D/enc/connections.toml"; printf 'language = "en"\n' >"$D/enc/config.toml"
E2E_NOLANG=1 e2e_start -C "$D/enc" "$E2E_BIN shop"; wait_for 8 flag_is alternate_on 1; sleep 2
e2e_keys Space; e2e_type s; sleep 0.5
check "英文界面：session 列表一行 \"shop  1 window\"" eval '[[ $(srows) == "shop  1 window|" ]] || { srows; false; }'
key Escape; sleep 0.3
key Space; e2e_type c; sleep 0.8
e2e_keys Space; e2e_type s; sleep 0.5
check "开了第二个 window：\"shop  2 windows\"" eval '[[ $(srows) == "shop  2 windows|" ]] || { srows; false; }'
key Escape; sleep 0.3
quit_app
_e2e_kill
e2e_done
