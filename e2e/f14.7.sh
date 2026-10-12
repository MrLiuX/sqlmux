#!/usr/bin/env bash
# F14.7 主题文件的未知键降级为警告（specs/m14-sqlite/task.md F14.7；同 F13.5）
# themes/<名字>.toml 里不认识的色键、[icon] 名字：忽略、启动后 toast「themes/<名字>.toml: 不认识的键 …」，不阻止启动，错误文案里没有 BurntSushi 的
# 「There is no token called」；颜色值写坏仍是启动错误；theme.switch 到这样的主题也给 toast。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/c/themes"
printf 'pane_bg = "#101820"\nbrand_new_token = "#ff00ff"\n' >"$D/c/themes/newer.toml"
printf 'theme = "newer"\n' >"$D/c/config.toml"
start -C "$D/c"; sleep 1.2
check "主题文件里多一个不认识的色键 brand_new_token：启动照常，toast「themes/newer.toml: 不认识的键 brand_new_token」" eval 'running && screen_has "themes/newer.toml: 不认识的键 brand_new_token"' || e2e_plain | head -3
check "没有旧的启动错误（There is no token called）" eval '! screen_has "no token called" && ! screen_has "[e2e-exit"'
check "认识的键照样生效：pane_bg #101820 是 pane 的底色" eval 'e2e_cap -e | grep -qF "48;2;16;24;32"'
_e2e_kill

printf 'pane_bg = "#101820"\n[icon]\nnosuchicon = "x"\n' >"$D/c/themes/newer.toml"
start -C "$D/c"; sleep 1.2
check "[icon] 里不认识的名字 nosuchicon：同样是 toast，不阻止启动" eval 'running && screen_has "themes/newer.toml: 不认识的键" && screen_has "nosuchicon"' || e2e_plain | head -3
_e2e_kill

printf 'pane_bg = "red"\n' >"$D/c/themes/newer.toml"
start -C "$D/c"; wait_for 5 screen_has '[e2e-exit'; sleep 0.3
check "颜色值写坏（pane_bg = \"red\"）仍是启动错误：退出（退出码 1），说出是哪一项" eval 'screen_has "[e2e-exit 1]" && screen_has "pane_bg"' || e2e_plain | head -6
_e2e_kill

printf 'pane_bg = "#101820"\nbrand_new_token = "#ff00ff"\n' >"$D/c/themes/newer.toml"; rm -f "$D/c/config.toml"
start -C "$D/c"; sleep 1.2
check "（准备）默认主题启动：没有不认识的键 toast" eval 'running && ! screen_has "不认识的键"'
pal ">theme"; sleep 0.4; key Enter; sleep 0.6
e2e_type newer; sleep 0.4; key Enter; sleep 1
check "theme.switch 到 newer：切换成功，toast「themes/newer.toml: 不认识的键 brand_new_token」" eval 'screen_has "themes/newer.toml: 不认识的键 brand_new_token"' || e2e_plain | tail -4 | cut -c1-200
e2e_done
