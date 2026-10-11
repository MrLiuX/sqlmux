#!/usr/bin/env bash
# F11.2 Homebrew tap：源码编译的 formula（specs/m11-distribution/task.md F11.2；tech-design §15「发布与分发」）
# .github/scripts/tap-formula.sh 出的 formula（release.yml 的 tap job 用同一个脚本）：结构检查，
# 然后在 docker 的 homebrew/brew（Linux）里用本树 git archive 的源码包做 file:// 的 url，brew install --build-from-source、brew test、brew audit。
# 真 tag 的 `brew install kackerx/tap/sqlmux`（tap 仓库里要有 formula）和干净 macOS 由决策者在 tag 上确认。需要 docker；镜像 homebrew/brew 只有 amd64，在 Apple 芯片上走模拟，要几分钟。
. "$(dirname "$0")/lib.sh"
D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-brew.XXXXXX"); trap 'rm -rf "$D"' EXIT
v=9.9.9 TAG=v9.9.9
cd "$E2E_ROOT"
git archive --format=tar.gz --prefix=sqlmux-$v/ -o "$D/$TAG.tar.gz" HEAD
SHA=$(shasum -a 256 "$D/$TAG.tar.gz" | cut -d' ' -f1)
sh .github/scripts/tap-formula.sh "$TAG" "$SHA" >"$D/formula.rb"

# ---- formula 的结构
check "url 是 GitHub 的 tag 源码包、sha256 是传进去的、license 和仓库的 LICENSE 一致（MIT）" eval 'grep -q "url \"https://github.com/kackerx/sqlmux/archive/refs/tags/$TAG.tar.gz\"" "$D/formula.rb" && grep -q "sha256 \"$SHA\"" "$D/formula.rb" && grep -q "license \"MIT\"" "$D/formula.rb" && head -1 LICENSE | grep -q "^MIT License"'
check "从源码编译：depends_on go => :build、std_go_args、编 ./cmd/sqlmux；ldflags 注入版本（v#{version}）" eval 'grep -q "depends_on \"go\" => :build" "$D/formula.rb" && grep -q "std_go_args" "$D/formula.rb" && grep -q "./cmd/sqlmux" "$D/formula.rb" && grep -q "main.version=v#{version}" "$D/formula.rb"'
check "没有 bottle、没有 on_macos / on_linux 分支（macOS 和 Linux 同一份）" eval '! grep -q "bottle\|on_macos\|on_linux\|on_arm\|on_intel" "$D/formula.rb"'
check "formula 的 class 名 Sqlmux、有 desc / homepage、没有 head、test 跑 --version" eval 'grep -q "^class Sqlmux < Formula" "$D/formula.rb" && grep -q "^  desc " "$D/formula.rb" && grep -q "homepage \"https://github.com/kackerx/sqlmux\"" "$D/formula.rb" && ! grep -q "^  head " "$D/formula.rb" && grep -q "assert_match \"sqlmux v#{version}\"" "$D/formula.rb"'
check "不同 tag / sha 出不同 formula，同样的输入出同样的（脚本没有别的输入）" eval 'sh .github/scripts/tap-formula.sh v1.0.0 abc | grep -q "v1.0.0.tar.gz" && [[ $(sh .github/scripts/tap-formula.sh $TAG $SHA | shasum) == $(shasum <"$D/formula.rb") ]]'
check "没参数时脚本报错退出（set -u），不输出半截 formula" eval 'out=$(sh .github/scripts/tap-formula.sh 2>/dev/null); [[ $? -ne 0 && -z $out ]]'

# ---- Linux 上 brew 真装
command -v docker >/dev/null && docker info >/dev/null 2>&1 || { echo "FAIL docker 不可用"; E2E_FAIL=$((E2E_FAIL + 1)); e2e_done; exit; }
sed "s|^  url .*|  url \"file:///work/$TAG.tar.gz\"|" "$D/formula.rb" >"$D/sqlmux.rb"
cat >"$D/run.sh" <<'SH'
set -e
export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_ENV_HINTS=1
brew tap-new --no-git local/test >/dev/null
cp /work/sqlmux.rb "$(brew --repository local/test)/Formula/sqlmux.rb"
echo "== install"; brew install --build-from-source local/test/sqlmux
echo "== version"; sqlmux --version
echo "== test"; brew test local/test/sqlmux
echo "== audit"; brew audit --strict local/test/sqlmux || echo "audit exit $?"
echo "== list"; brew list --versions sqlmux; ls -l "$(brew --prefix)/bin/sqlmux"
echo "== uninstall"; brew uninstall sqlmux; [ ! -e "$(brew --prefix)/bin/sqlmux" ] && echo "removed"
SH
chmod -R a+rX "$D"
timeout 1500 docker run --rm --platform linux/amd64 -v "$D:/work:ro" homebrew/brew sh /work/run.sh >"$D/brew.log" 2>&1; rc=$?
sec() { sed -n "/^== $1\$/,/^== /p" "$D/brew.log" | sed '1d;/^== /d'; }
check "docker homebrew/brew：brew install --build-from-source 成功（退出码 0）" eval '[[ $rc -eq 0 ]] || { tail -25 "$D/brew.log"; false; }'
check "sqlmux --version 是 'sqlmux v$v (…)'（ldflags 注入了 tag 的版本）" eval 'sec version | grep -q "^sqlmux v$v ("  || { sec version; false; }'
check "brew test 通过" eval 'sec test | grep -qi "error\|fail" && { sec test; false; } || grep -q "^== test" "$D/brew.log"'
check "brew audit --strict 没有问题（也是对 formula 语法的检查：macOS 自带的 ruby 2.6 读不了 std_go_args(ldflags:)）" eval '! sec audit | grep -q "audit exit" || { sec audit; false; }'
check "安装在 brew 的 prefix 里、版本 ${v}，uninstall 后命令没了" eval 'sec list | grep -q "^sqlmux $v" && sec uninstall | grep -q "^removed$" && [[ $rc -eq 0 ]]'
e2e_done
