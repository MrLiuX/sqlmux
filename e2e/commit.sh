# Shared by f13.1.sh (PG) and f13.1b.sh (MySQL): F13.1 the connection goes right after the COMMIT is sent (e2e/commitcut.py, a proxy that
# drops both sides once a COMMIT has gone through). Table t_c(id pk, v): 1 one, 2 two, 3 three in the script's own database; $CUTDSN is the DSN through the proxy,
# $D/c/connections.toml has it as doraemon; DBVALS prints the values as stored.
setup_cut() {   # ENGINE URL-OF-OWN-DB
  read -r UH UP <<<"$(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p.hostname, p.port)' "$2")"
  CUTPORT=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
  python3 "$E2E_ROOT/e2e/commitcut.py" "$CUTPORT" "$UH" "$UP" & CUTPID=$!; sleep 0.5
  CUTDSN=$(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p._replace(netloc=f"{p.username}:{p.password}@127.0.0.1:{sys.argv[2]}").geturl())' "$2" "$CUTPORT")
  mkdir -p "$D/c"; printf '[[connection]]\nname = "doraemon"\nengine = "%s"\ndsn = "%s"\n' "$1" "$CUTDSN" >"$D/c/connections.toml"; chmod 600 "$D/c/connections.toml"
}
cut_stop() { [[ -n $CUTPID ]] && { kill "$CUTPID" 2>/dev/null; wait "$CUTPID" 2>/dev/null; }; CUTPID=; }
has_pending() { [[ $(qb | sed -E 's/.*COLS [0-9]+\/[0-9]+//; s/auto.*//' | tr -cd '0-9') == 1 ]]; }   # 查询条里 COLS 和行数统计之间的保存按钮：计数 1
toast_has() { e2e_cap | grep -qF -- "$1"; }

commit_body() {
  SOLO=1 start -C "$D/c"; open_table t_c; wait_for 8 settled
  key l; key i; sleep 0.3; e2e_type "X"; key Enter; sleep 0.5
  check "（准备）v 改成 X，保存计数 1" eval 'has_pending'
  e2e_keys C-s; wait_for 8 eval 'toast_has "提交结果未知"'
  check "C-s 时 COMMIT 发出去之后连接断了：toast「<连接名>：连接断开，提交结果未知：已刷新，请核对」（§8.2）" eval 'toast_has "连接断开，提交结果未知：已刷新，请核对"'
  check "不是「已回滚」：没有「已回滚」，错误栏里也没有" eval '! screen_has "已回滚"'
  sleep 3.5
  check "修改标记还在（保存计数 1），格子里还是 X——让用户刷新后自己核对" eval 'has_pending && e2e_plain | grep -qE "│ +1 │ +1 │ X"'
  check "库里是真的提交了（v = X，只写了一次）" eval '[[ $(DBVALS) == "X,two,three" ]]'
  key r; sleep 0.5
  check "r 放弃这条修改：修改标记清掉；表格在事务丢失时已经重载成库里的值，所以格子是 X（不是打开时的 one，§8.2：事务丢失同时重载）" eval 'e2e_plain | grep -qE "│ +1 │ +1 │ X" && ! has_pending'
  DBEXEC "update t_c set v = 'THREE' where id = 3"
  key R; wait_for 8 settled; sleep 0.8
  check "R 刷新：取回库里的真实值——第 1 行 X（那次提交其实成功了）、第 3 行 THREE（别处改的）" eval 'e2e_plain | grep -qE "│ +1 │ +1 │ X" && e2e_plain | grep -qE "│ +3 │ +3 │ THREE"'
  _e2e_kill

  # ---- query 里手写 begin; …; commit：断线也是「提交结果未知」，不是普通错误
  start -C "$D/c"; key C-l
  setsql "begin; update t_c set v = 'Q' where id = 2; commit"; key g g; key V G; key Enter     # 选区执行：整段（↵ 在 NORMAL 下只跑光标所在的那一条语句）
  sleep 1.5
  check "query 里 begin; update; commit，COMMIT 发出去后断线：toast「连接断开，提交结果未知：请先查一下数据」（§8.2），不是普通错误或「已回滚」" eval 'toast_has "连接断开，提交结果未知：请先查一下数据" && ! screen_has "已回滚"' || e2e_cap | tail -12
  check "库里是提交了的（id 2 是 Q）" eval '[[ $(DBVALS) == "X,Q,THREE" ]]'
}
