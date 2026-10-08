#!/usr/bin/env bash
# F8.22 侧栏宽度配置 [tree] width（specs/m8-feedback/task.md F8.22；tech-design §7.8「schema 侧栏」、§14）
# 默认 32 列（含边框），配置的值夹在 16 到窗口宽度一半之间；布局文件里的 sidebar_width 优先；类型不对启动报配置错误。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkcfg() { rm -rf "$D/c"; mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/c/connections.toml"
  [[ -n $1 ]] && printf '%s\n' "$1" >"$D/c/config.toml"); [[ -d $D/keep/layouts ]] && cp -R "$D/keep/layouts" "$D/c/"; true; }   # 每次启动都是新的配置目录：存过的布局文件要带过去                 # [tree] 段的文字，空就不写 config.toml
sw() { geom 0 | awk '{ print $3 }'; }                                       # ⓪ 的宽度
startw() { mkcfg "$1"; e2e_start -x "${W:-160}" -C "$D/c" -S "$D/st" -D "$D/data" "$E2E_BIN $2"; wait_for 6 eval 'flag_is alternate_on 1 || exited'; sleep 1.2; }

startw ""
check "不配：侧栏 32 列（默认）" eval '[[ $(sw) == 32 ]] || { echo "  width $(sw)"; false; }'
startw $'[tree]\nwidth = 40'
check "[tree] width = 40：侧栏 40 列，① ② 的位置跟着往右" eval '[[ $(sw) == 40 && $(geom 1 | cut -d" " -f1) == 42 ]] || { echo "  width $(sw), ① at $(geom 1)"; false; }'
startw $'[tree]\nwidth = 8'
check "width = 8：夹到 16 列" eval '[[ $(sw) == 16 ]] || { echo "  width $(sw)"; false; }'
startw $'[tree]\nwidth = 200'
check "width = 200（160 列的窗口）：夹到窗口的一半，80 列" eval '[[ $(sw) == 80 ]] || { echo "  width $(sw)"; false; }'

# ---- 布局文件里的 sidebar_width 优先于配置
startw $'[tree]\nwidth = 40'
save_layout() { e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; for i in $(seq 20); do e2e_keys BSpace; done; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "$1"; key Enter; sleep 0.6; }
DRAGGED=
e2e_drag 41 20 51 20; sleep 0.5; DRAGGED=$(sw)
check "（准备）width = 40，拖侧栏右边的间隔（41 → 51 列）：侧栏变宽（$DRAGGED 列，拖过的记在 window 上）" eval '(( DRAGGED > 40 ))'
save_layout w
check "（准备）layout.save w：文件里记着 sidebar_width = $DRAGGED" eval 'grep -q "sidebar_width = $DRAGGED" "$E2E_TMP/config/sqlmux/layouts/w.toml" || { cat "$E2E_TMP/config/sqlmux/layouts/w.toml" 2>&1 | head -20; false; }'
mkdir -p "$D/keep"; cp -R "$E2E_TMP/config/sqlmux/layouts" "$D/keep/"
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
startw $'[tree]\nwidth = 44' "--layout w"
check "配置改成 width = 44，--layout w 打开：侧栏是布局里拖过的 ${DRAGGED} 列，不是配置的 44" eval '[[ $(sw) == "$DRAGGED" ]] || { echo "  width $(sw)"; false; }'

# ---- 类型不对：启动报配置错误（英文，读 config 之前就是英文）
mkcfg $'[tree]\nwidth = "wide"'
e2e_start -x 160 -C "$D/c" -S "$D/st" -D "$D/data" "$E2E_BIN"; sleep 1.5
check "width = \"wide\"：启动报错退出，错误里有 tree.width" eval 'e2e_plain | grep -qi "tree" && exited && e2e_plain | grep -qi "width" || { e2e_plain | head -5; false; }'
e2e_done
