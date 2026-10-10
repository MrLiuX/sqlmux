# M11 分发 · 任务清单

- **状态**：**todo**（2026-10-10 用户定：go install 之外还要 Homebrew 和 npm / npx；brew 走源码编译的 formula）。M10 passed 之后开工；F11.1 可以在 F10.12 落地、打 v0.1.1 时先做。
- **目标**：打 `v*` tag 就自动出 GitHub Release（压缩包、校验和、说明），`brew install kackerx/tap/sqlmux` 和 `npx sqlmux` 都能用。
- **范围**：tech-design §15「发布与分发」、§4。
- **依赖**：M10 F10.12（module 路径）。
- **完成标准**：F11.1–F11.3 passed；在一台干净的 macOS 和一台 Linux 上，三种方式装出来的 `sqlmux --version` 都是那个 tag。
- **审查节点**：
  1. F11.3 之后：F11.1–F11.3 一批。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F11.1 GoReleaser 与 release workflow · 状态：todo

- **依赖**：F10.12
- **涉及**：`.goreleaser.yaml`、`.github/workflows/release.yml`、`cmd/sqlmux`（`--version`）、`justfile`、README

**开发**（§15「发布与分发」）
- [ ] `sqlmux --version` 打印版本、commit、日期，来自 ldflags（`main.version` 等三个变量，没注入时是 `dev`）；`just build` 也注入 `git describe` 的值。
- [ ] `.goreleaser.yaml`：`cmd/sqlmux`，`CGO_ENABLED=0`，darwin / linux × amd64 / arm64（不出 Windows，本来不支持），`-s -w` 加版本的 ldflags，`tar.gz` 归档（名字 `sqlmux_<version>_<os>_<arch>`），`checksums.txt`，changelog 按 commit 前缀分组（`F*:` 功能、`docs:`、`e2e:`、`prune:` 排除）；`release.draft = false`，说明用仓库里的模板（`.goreleaser` 的 `release.header` 指向 README 的一句话加安装方式）。
- [ ] `.github/workflows/release.yml`：`push: tags: ['v*']`，先跑 lint / vet / 单测（复用 ci.yml 的 job 或 `workflow_call`），再 `goreleaser release --clean`；`permissions: contents: write`；`just release-check` 本地跑 `goreleaser check` 和 `goreleaser build --snapshot --clean`。
- [ ] README 的 Install 节：`go install github.com/kackerx/sqlmux/cmd/sqlmux@latest`、Release 页下载、brew 和 npm 两行（F11.2 / F11.3 落地后由决策者补）。
- [ ] 细节（已定）：GoReleaser 用 OSS 版，不用 Pro 的功能；发布说明的额外文字由 oss-maintainer 起草、用户过目后才进 Release。

**验收**
- [ ] 单测：`--version` 没注入时输出 `dev`。
- [ ] `goreleaser build --snapshot` 在本地出四个二进制，`./dist/…/sqlmux --version` 显示 snapshot 版本。
- [ ] 打 tag 后 GitHub Release 有四个压缩包和 checksums.txt，workflow 绿（推送后由决策者确认）。

## F11.2 Homebrew tap：源码编译的 formula · 状态：todo

- **依赖**：F11.1
- **涉及**：新仓库 `kackerx/homebrew-tap`（`Formula/sqlmux.rb`）、`.github/workflows/release.yml`

**开发**（§15「发布与分发」）
- [ ] 用户（或决策者用 kackerx 账号）建 `kackerx/homebrew-tap`；formula 从 tag 的源码包编译：`depends_on "go" => :build`，`system "go", "build", *std_go_args(ldflags: …)`，`test` 跑 `sqlmux --version`；macOS 和 Linux 都能装。
- [ ] release workflow 在 Release 建好后，用一个 PAT（仓库密钥 `TAP_GITHUB_TOKEN`）把 formula 的 `url` 和 `sha256` 改成新 tag 的，直接提交到 tap 仓库（不走 PR）。
- [ ] 细节（已定）：不用 GoReleaser 的 `homebrew_casks`（只管 macOS，没签名的二进制被 Gatekeeper 拦，要么交 Apple 年费要么 xattr 绕过）和已废弃的 `brews`；homebrew-core 等项目够知名再提交。

**验收**
- [ ] 干净的 macOS 和 Linux（docker 的 `homebrew/brew` 镜像）上 `brew install kackerx/tap/sqlmux`，`sqlmux --version` 是那个 tag；`brew test sqlmux` 过。

## F11.3 npm：壳包加平台子包 · 状态：todo

- **依赖**：F11.1
- **涉及**：`npm/`（壳包 `sqlmux` 和 `@sqlmux/<os>-<arch>` 四个子包的 package.json 模板、壳包的 bin 脚本）、`.github/workflows/release.yml`

**开发**（§15「发布与分发」）
- [ ] 照 esbuild / turbo 的做法：壳包 `sqlmux` 的 `optionalDependencies` 列出 `@kackerx/sqlmux-darwin-arm64`、`-darwin-x64`、`-linux-x64`、`-linux-arm64`（作用域 `@kackerx`，用户 2026-10-10 定），每个子包 `os` / `cpu` 字段限定平台、里面只有一个二进制；壳包的 `bin/sqlmux.js` 按 `process.platform` / `process.arch` `require.resolve` 子包里的二进制，`spawnSync` 带 `stdio: "inherit"` 执行，退出码透传，找不到子包时打印安装提示并退出 1。不用 postinstall 下载（pnpm 10 默认禁脚本，`--ignore-scripts` 会装出空包）。
- [ ] release workflow 从 GoReleaser 的产物装好子包、把五个 package.json 的版本写成 tag、`npm publish --access public`（仓库密钥 `NPM_TOKEN`：用户自己注册 npm 账号 `kackerx`，作用域就是用户名；生成只有 publish 权限的 granular token，由决策者 `gh secret set NPM_TOKEN` 存进仓库）。
- [ ] 细节（已定）：Windows 不出子包；壳包不带 `engines` 之外的依赖。

**验收**
- [ ] 干净的 macOS 和 Linux 上 `npx sqlmux@<tag> --version` 和 `npm i -g sqlmux` 后 `sqlmux --version` 都是那个 tag；TUI 能正常打开和退出。
- [ ] `pnpm dlx sqlmux --version` 同样（默认禁脚本也能用）。
