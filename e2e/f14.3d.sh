#!/usr/bin/env bash
# F14.3d MySQL 索引的定义能原样重建（F14.3 复审）（specs/m14-sqlite/task.md F14.3；tech-design §7.8「对象节点」、§8.4）：同 f14.3.sh，自建库：前缀长度、DESC、FULLTEXT、INVISIBLE、表达式、UNIQUE 都在 ↵ 出来的 create index 里。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT

e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
DB=sqlmux_e2e_$$
e2e_mysql "$DB" <<'SQL'
create table t (id int primary key, nm varchar(50), body text, n int, unique key ix_p (nm(5)), key ix_d (n desc, id), fulltext key ft_b (body), key ix_i (n) invisible, key ix_e ((n * 2)));
SQL
mkdir -p "$D/own"; printf '[tree]\nwidth = 70\n' >"$D/own/config.toml"; printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
tree() { e2e_plain | python3 -c "import sys; [print(l[:72].rstrip()) for l in list(sys.stdin)[:${1:-40}]]"; }
trow() { e2e_plain | python3 -c "
import sys
for i,l in enumerate(sys.stdin, 1):
    if sys.argv[1] in l[:72]: print(i); break" "$1"; }
tclick() { local y x; y=$(trow "$1"); [[ -n $y ]] || { echo "  no tree row '$1'"; return 1; }; x=$(e2e_plain | sed -n "${y}p" | python3 -c "import sys; l=sys.stdin.read(); print(max(l.find('▸'), l.find('▾')) + 1)"); e2e_click "$x" "$y"; sleep 0.5; }   # the arrow of the row that has TEXT
tleaf() { local y; y=$(trow "$1"); e2e_click 12 "$y"; sleep 0.6; }                                                                                    # the row's text: a leaf's ↵ is the click
thas() { tree 60 | grep -qF -- "$1"; }

title1() { e2e_plain | sed -n 1p | python3 -c "import sys; l = sys.stdin.read(); i = l.find('┌─ ①'); print(l[i:i + 40].strip() if i >= 0 else '')"; }
title1() { e2e_plain | sed -n 1p | python3 -c "import sys; l = sys.stdin.read(); i = l.find('┌─ ①'); print(l[i:i + 40].strip() if i >= 0 else '')"; }
Q='`'
ddl() { tleaf "$1"; sleep 0.8; e2e_plain | grep -a -iE 'create .*index' | head -1 | sed 's/^.*│ \(CREATE[^│]*[^ │]\) *│.*$/\1/'; key Escape; sleep 0.3; }
start -C "$D/own"; solo
tclick "t  "; sleep 0.5; tclick Indexes; sleep 0.3
check "Indexes (6) 的叶子：PRIMARY (id) UNIQUE、ft_b (body)、ix_d (n, id)、ix_i (n)、ix_p (nm) UNIQUE 和表达式那条" eval 'thas "Indexes (6)" && thas "PRIMARY (id) UNIQUE" && thas "ft_b (body)" && thas "ix_i (n)" && thas "ix_p (nm) UNIQUE"'
want="CREATE UNIQUE INDEX ${Q}ix_p${Q} ON ${Q}$DB${Q}.${Q}t${Q} (${Q}nm${Q}(5));"
check "前缀长度：ix_p 的定义是 ... (nm(5))，UNIQUE 也在" eval '[[ $(ddl "ix_p (") == "$want" ]]' || ddl "ix_p ("
want="CREATE INDEX ${Q}ix_d${Q} ON ${Q}$DB${Q}.${Q}t${Q} (${Q}n${Q} DESC, ${Q}id${Q});"
check "DESC：ix_d 的定义是 (n DESC, id)" eval '[[ $(ddl "ix_d (") == "$want" ]]' || ddl "ix_d ("
want="CREATE FULLTEXT INDEX ${Q}ft_b${Q} ON ${Q}$DB${Q}.${Q}t${Q} (${Q}body${Q});"
check "FULLTEXT：ft_b 的定义是 CREATE FULLTEXT INDEX …" eval '[[ $(ddl "ft_b (") == "$want" ]]' || ddl "ft_b ("
want="CREATE INDEX ${Q}ix_i${Q} ON ${Q}$DB${Q}.${Q}t${Q} (${Q}n${Q}) INVISIBLE;"
check "INVISIBLE：ix_i 的定义末尾有 INVISIBLE" eval '[[ $(ddl "ix_i (") == "$want" ]]' || ddl "ix_i ("
check "表达式：ix_e 的定义是 CREATE INDEX … ON db.t (表达式 n * 2)，不是列名清单" eval '[[ $(ddl "ix_e (") == "CREATE INDEX ${Q}ix_e${Q} ON ${Q}$DB${Q}.${Q}t${Q} ("*"${Q}n${Q} * 2"*");" ]]' || ddl "ix_e ("
e2e_done
