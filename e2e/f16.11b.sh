#!/usr/bin/env bash
# F16.11b 补全候选的图标（PG）：视图、值（布尔 / 枚举 / now()）、null 关键字、列（specs/m16-redis/task.md F16.11；tech-design §7.7、§9.7）
# 值候选的图标是 value、色 string（默认不着色 = fg，主题里写了 string 就用它）；视图 view、null 是关键字图标；列 column（dim）。表 / 关键字 / 片段在 f16.11.sh（SQLite）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create type st as enum ('pending', 'done')" -c "create table t_v (id int primary key, s st, ok boolean, at timestamp, note text)" -c "create view v_v as select * from t_v" >/dev/null
mkdir -p "$D/own" "$D/th/themes"
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
cp "$D/own/connections.toml" "$D/th/connections.toml"; printf 'theme = "x"\n' >"$D/th/config.toml"; printf 'string = "#ff8800"\n' >"$D/th/themes/x.toml"
BOX() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
rows() { local g y; g=($(BOX)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $y; done | python3 -c '
import sys
for l in sys.stdin.read().split("\n"):
    l = l.strip()
    if not l: continue
    if "" <= l[0] <= "": print("U+%04x %s" % (ord(l[0]), l[1:].strip()))
    else: print("- " + l)'; }
icon_fg() { local g; g=($(BOX)); e2e_style $((g[0] + 2)) $((g[1] + $1)); }
has_row() { rows | grep -qE "$1" || { echo "  no row /$1/ in:"; rows | sed 's/^/    /'; false; }; }
newq() { key Escape; key Escape; e2e_type ggdG; sleep 0.3; key i; e2e_type "$1"; sleep 1.2; }

start -C "$D/own"; open_table t_v; wait_for 8 settled; key C-l; key C-l; key i
newq "select * from v_"
check "视图：v_v 前面是 view 图标（U+f06e），色同树里的视图（#7aa2f7）" eval 'has_row "^U\+f06e v_v " && [[ $(icon_fg 1) == "fg=#7aa2f7 "* ]]' || { rows; icon_fg 1; }
newq "select * from t_v where ok = "
check "布尔的值：true / false 前面是 value 图标（U+eb5d），默认色 = fg（string 默认不着色）" eval 'has_row "^U\+eb5d true" && has_row "^U\+eb5d false" && [[ $(icon_fg 1) == "fg=#c0caf5 "* ]]' || { rows; icon_fg 1; }
newq "update t_v set s = "
check "枚举的值：pending / done 是 value 图标；null 是关键字图标（U+eb62，色 keyword）" eval 'has_row "^U\+eb5d pending" && has_row "^U\+eb5d done" && has_row "^U\+eb62 null" && [[ $(icon_fg 3) == "fg=#bb9af7 "* ]]' || { rows; icon_fg 3; }
newq "update t_v set at = "
check "时间的值：now() / current_timestamp 是 value 图标" eval 'has_row "^U\+eb5d now\(\)" && has_row "^U\+eb5d current_timestamp"' || rows
newq "select * from t_v where "
check "条件里的列：五个都是 column 图标（U+eb5f），dim 色" eval '[[ $(rows | grep -c "^U+eb5f ") == 5 && $(icon_fg 2) == "fg=#565f89 "* ]]' || { rows; icon_fg 2; }
newq "update t_v set s = "; key Tab; key Enter; sleep 0.4
check "接受值候选：写进去的是 'pending'，没有图标字符" eval 'e2e_plain | python3 -c "
import re, sys
t = sys.stdin.read()
m = re.search(r\"set s = (\\S+)\", t)
sys.exit(0 if m and m.group(1) == \"\\x27pending\\x27\" else 1)"' || e2e_plain | grep -a "set s"
_e2e_kill

e2e_start -C "$D/th" "$E2E_BIN doraemon"; wait_for 8 flag_is alternate_on 1; sleep 2; open_table t_v; wait_for 8 settled; key C-l; key C-l; key i
newq "select * from t_v where ok = "
check "主题里 string = #ff8800：值候选的图标是橙色（颜色用 string）" eval '[[ $(icon_fg 1) == "fg=#ff8800 "* ]]' || { rows; icon_fg 1; }
e2e_done
