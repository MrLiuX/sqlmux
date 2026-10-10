#!/usr/bin/env bash
# F6.5 的审查补充（specs/m6-config/task.md F6.5；tech-design §14「布局文件」）：手写布局的容错（缺 connection / schema、同一张表写两次只留一个、
# pane 不在分割树里就忽略、limit ≤ 0 用默认值、超过 10000 夹到 10000、query 文件不存在就是空 query）、--layout last 打开 last.toml、
# 保存名 x.toml 写到 layouts/x.toml.toml。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/cfg/layouts" "$D/st" "$D/data"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/cfg/connections.toml")
qbar() { e2e_text 36 160 3; }
startl() { e2e_start -C "$D/cfg" -S "$D/st" -D "$D/data" "$E2E_BIN $*"; wait_for 6 eval 'flag_is alternate_on 1 || exited'; sleep 2; }
cat >"$D/cfg/layouts/hand.toml" <<'T'
version = 1
active = 0

[[session]]
  name = "hand"
  connection = "doraemon"
  active = 0

  [[session.window]]
    name = "w"
    focus = 1
    sidebar_open = true
    [session.window.layout]
      split = "h"
      ratio = 0.5
      [session.window.layout.a]
        pane = 1
      [session.window.layout.b]
        pane = 2

    [[session.window.pane]]
      id = 1
      current = 0
      [[session.window.pane.tab]]
        kind = "table"
        table = "t_order"
        limit = 0
      [[session.window.pane.tab]]
        kind = "table"
        schema = "public"
        table = "t_order"
        limit = 20000

    [[session.window.pane]]
      id = 2
      current = 0
      [[session.window.pane.tab]]
        kind = "query"
        n = 7

    [[session.window.pane]]
      id = 9
      current = 0
      [[session.window.pane.tab]]
        kind = "table"
        table = "t_user"
T
cat >"$D/cfg/layouts/clamp.toml" <<'T'
version = 1
[[session]]
  name = "c"
  connection = "doraemon"
  [[session.window]]
    name = "w"
    [session.window.layout]
      pane = 1
    [[session.window.pane]]
      id = 1
      [[session.window.pane.tab]]
        kind = "table"
        table = "t_order"
        limit = 20000
T
startl --layout hand
check "手写布局：session 叫 hand，window 叫 w，两个 pane（树 + ① + ②），pane 9 不在分割树里（pane 的 tab 一概不建）" eval '[[ $(e2e_panes | awk "\$1 != \"-\" { print \$1 }" | tr "\n" " ") == "0 1 2 " ]] && [[ $(bar) == *hand* && $(bar) == *"0: w"* ]]'
check "t_order 写了两次（一个没写 schema、一个写了 public）：只留一个 tab；query 没写 connection、没有文件：打开成空的 query_7" eval 'tabs_are 1 "1:t_order*" && tabs_are 2 "1:query_7*"'
check "limit = 0 用默认值 100" eval '[[ $(qbar) == *"LIMIT 100"* ]]'
e2e_stop
startl --layout clamp
check "limit = 20000 夹到 10000" eval 'e2e_plain | grep -q "LIMIT 10000"'
e2e_stop

# ---- 保存名：x.toml 只按名字取路径（写到 layouts/x.toml.toml）
startl doraemon
e2e_keys C-p; sleep 0.3; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "x.toml"; key Enter; sleep 0.6
check "保存名 x.toml：写到 layouts/x.toml.toml（只按名字取路径）" eval '[[ -f "$E2E_TMP/config/sqlmux/layouts/x.toml.toml" ]]'
cp "$E2E_TMP/config/sqlmux/layouts/x.toml.toml" "$D/saved.toml"
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.5
check "（准备）:qa 之后写了 last.toml" eval '[[ -f "$D/st/sqlmux/last.toml" ]]'
e2e_stop

# ---- --layout last 打开的是 last.toml
cp "$D/st/sqlmux/last.toml" "$D/last.copy"
startl --layout last
check "--layout last：打开 last.toml（和不带参数一样的 session / window）" eval 'running && [[ $(bar) == *doraemon* ]]'
e2e_stop

e2e_done
