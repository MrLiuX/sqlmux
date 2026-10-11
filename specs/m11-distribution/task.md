# M11 分发 · 任务清单

- **状态**：**todo**（节点 1 F11.1–F11.3 passed 2026-10-11，b1cb1c6，全量 3075 项 0 败；真 tag 上的 release.yml、brew、npx 等待 `TAP_GITHUB_TOKEN` 后在 v0.2.1 上确认。原文：2026-10-10 用户定：go install 之外还要 Homebrew 和 npm / npx；brew 走源码编译的 formula）。M10 全部 passed 之后开工，第一个 GitHub Release 也等那时一起发（用户 2026-10-10 定：M10 之后发一个小版本，不随 F10.12 先发）。
- **目标**：打 `v*` tag 就自动出 GitHub Release（压缩包、校验和、说明），`brew install kackerx/tap/sqlmux` 和 `npx sqlmux` 都能用。
- **范围**：tech-design §15「发布与分发」、§4。
- **依赖**：M10 F10.12（module 路径）。
- **完成标准**：F11.1–F11.4 passed；在一台干净的 macOS 和一台 Linux 上，三种方式装出来的 `sqlmux --version` 都是那个 tag。
- **审查节点**：
  1. F11.3 之后：F11.1–F11.3 一批。
  2. F11.4 之后：F11.4 单独一批（用户 2026-10-11 提的结果区滚动卡顿，和分发无关，排在这里是因为它是当前里程碑）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F11.1 GoReleaser 与 release workflow · 状态：passed（b1cb1c6；e2e bbf964c）

- **依赖**：F10.12
- **涉及**：`.goreleaser.yaml`、`.github/workflows/release.yml`、`cmd/sqlmux`（`--version`）、`justfile`、README

**开发**（§15「发布与分发」）
- [x] `sqlmux --version` 打印版本、commit、日期，来自 ldflags（`main.version` 等三个变量）；没注入时用 Go 记进二进制的模块版本（`runtime/debug.ReadBuildInfo` 的 `Main.Version`：`go install …@v0.2.1` 装的就是 `v0.2.1`，checkout 里直接 `go build` 的是 Go 1.24 起带 commit 和时间的伪版本），再没有才是 `dev`；不取 `vcs.*`（proxy 装的没有，checkout 里的伪版本已经带了）。`just build` 自己注入 `git describe` 的值（审查时定）。
- [x] `.goreleaser.yaml`：`cmd/sqlmux`，`CGO_ENABLED=0`，darwin / linux × amd64 / arm64（不出 Windows，本来不支持），`-s -w` 加版本的 ldflags，`tar.gz` 归档（名字 `sqlmux_<version>_<os>_<arch>`），`checksums.txt`，changelog 按 commit 前缀分组（`F*:` 功能、`docs:`、`e2e:`、`prune:` 排除）；`release.draft = false`，说明用仓库里的模板（`.goreleaser` 的 `release.header` 是内联字符串（OSS 没有 `from_file`），这批不写，文字由 oss-maintainer 起草、用户过目后在第一个 GoReleaser tag 之前加）。
- [x] `.github/workflows/release.yml`：`push: tags: ['v*']`，先跑 lint / vet / 单测（复用 ci.yml 的 job 或 `workflow_call`），再 `goreleaser release --clean`；`permissions: contents: write`；`just release-check` 本地跑 `goreleaser check` 和 `goreleaser build --snapshot --clean`。
- [ ] README 的 Install 节：`go install github.com/kackerx/sqlmux/cmd/sqlmux@latest`（决策者 069c422 已写）、Release 页下载一句（压缩包名和 `checksums.txt`，worker 这批补）、brew 和 npm 两行（F11.2 / F11.3 落地后由决策者补）。
- [x] 细节（已定）：GoReleaser 用 OSS 版，不用 Pro 的功能；发布说明的额外文字由 oss-maintainer 起草、用户过目后才进 Release。

**验收**
- [x] 单测：`--version` 没注入、也没有模块版本时输出 `dev`（`go test` 的二进制）。
- [x] `goreleaser build --snapshot` 在本地出四个二进制，`./dist/…/sqlmux --version` 显示 snapshot 版本。
- [x] 打 tag 后 GitHub Release 有四个压缩包和 checksums.txt，workflow 绿（推送后由决策者确认）。

## F11.2 Homebrew tap：源码编译的 formula · 状态：passed（b1cb1c6；e2e bbf964c）

- **依赖**：F11.1
- **涉及**：新仓库 `kackerx/homebrew-tap`（`Formula/sqlmux.rb`）、`.github/workflows/release.yml`

**开发**（§15「发布与分发」）
- [x] 决策者用 kackerx 账号建 `kackerx/homebrew-tap`（2026-10-11 已建）并推一个只有 README 的初始 commit（空仓库上 actions/checkout 取不到默认分支，审查时发现；workflow 不为空仓库写分支）；formula 从 tag 的源码包编译：`depends_on "go" => :build`，`system "go", "build", *std_go_args(ldflags: …)`，`test` 跑 `sqlmux --version`；macOS 和 Linux 都能装。
- [x] release workflow 在 Release 建好后，用一个 PAT（仓库密钥 `TAP_GITHUB_TOKEN`）把新 tag 的 formula 整份生成（模板只在本仓库 `.github/scripts/tap-formula.sh` 一处，tap 里不手工放一份；worker 开工时定，tap 仓库本来是空的）直接提交到 tap 仓库（不走 PR）。
- [x] 细节（已定）：不用 GoReleaser 的 `homebrew_casks`（只管 macOS，没签名的二进制被 Gatekeeper 拦，要么交 Apple 年费要么 xattr 绕过）和已废弃的 `brews`；homebrew-core 等项目够知名再提交。

**验收**
- [ ] 干净的 macOS 和 Linux（docker 的 `homebrew/brew` 镜像）上 `brew install kackerx/tap/sqlmux`，`sqlmux --version` 是那个 tag；`brew test sqlmux` 过。

## F11.3 npm：壳包加平台子包 · 状态：passed（b1cb1c6；e2e bbf964c）

- **依赖**：F11.1
- **涉及**：`npm/`（壳包 `sqlmux` 和 `@kackerx/sqlmux-<os>-<arch>` 四个子包的 package.json 模板、壳包的 bin 脚本）、`.github/workflows/release.yml`

**开发**（§15「发布与分发」）
- [x] 照 esbuild / turbo 的做法：壳包 `sqlmux` 的 `optionalDependencies` 列出 `@kackerx/sqlmux-darwin-arm64`、`-darwin-x64`、`-linux-x64`、`-linux-arm64`（作用域 `@kackerx`，用户 2026-10-10 定），每个子包 `os` / `cpu` 字段限定平台、里面只有一个二进制；壳包的 `bin/sqlmux.js` 按 `process.platform` / `process.arch` `require.resolve` 子包里的二进制，`spawnSync` 带 `stdio: "inherit"` 执行，退出码透传，找不到子包时打印安装提示并退出 1。不用 postinstall 下载（pnpm 10 默认禁脚本，`--ignore-scripts` 会装出空包）。
- [x] release workflow 从 GoReleaser 的产物装好子包、把五个 package.json 的版本写成 tag、`npm publish --access public`（仓库密钥 `NPM_TOKEN` 已于 2026-10-10 存进 `kackerx/sqlmux`：npm 账号 `kackerx`，token 名 `sqlmux`，package write、绕过 2FA、不过期；作用域 `@kackerx` 就是用户名，不用建组织）。
- [x] 细节（已定）：Windows 不出子包；壳包不带 `engines` 之外的依赖。

**验收**
- [ ] 干净的 macOS 和 Linux 上 `npx sqlmux@<tag> --version` 和 `npm i -g sqlmux` 后 `sqlmux --version` 都是那个 tag；TUI 能正常打开和退出。
- [x] `pnpm dlx sqlmux --version` 同样（默认禁脚本也能用）。

## F11.4 结果区滚动卡顿 · 状态：todo

用户 2026-10-11：普通表格（一页 100 行）里滚动还行，结果区的表格（F10.4 起一页 500 行，截图里是 15 列、带中文和 JSON 的 mt_task）滚动时卡顿、阻塞、延迟明显；要求复查 F8.20 的优化是不是没盖到结果区。

决策者看代码后的判断（只是嫌疑，以 profile 为准）：`ui.Grid` 的列宽 `widths()` 每次 `layout()` 都把这一页每个单元格重新算一遍宽度（`Width(Cell(val))`），F8.20 的改法 C「列宽缓存」因为 A、B 已达标没做；100 行 × 几列的 ASCII 走快路径没事，500 行 × 15 列、含中文和 JSON 的值走 `ansi.StringWidth` 的字素路径，一个按键 `Draw`、`Scroll`、`View`、`cellRect` 又各自 `layout()` 一次，一次滚动是几万次字素宽度计算。表格 pane 的 LIMIT 100 只是把这个开销藏住了。

- **依赖**：F8.20、F10.4
- **涉及**：`internal/ui`（grid 的列宽）、`internal/app`（benchmark、result）

**开发**（§7.6「列宽」、§15「性能」）
- [ ] 先量：benchmark 加一个结果区的场景（默认布局，结果 tab 500 行 × 15 列，值里一半是中文和 100 字的 JSON，喂 `j` 按住 3 秒的按键事件和滚轮事件各一组，每个事件一次 Update + View），记下优化前每事件的耗时、分配，和 profile 的热点；也量一下同样数据在表格 pane 100 行时的数字作对照。
- [ ] 按热点改，先做改动最小的那一处。如果热点是列宽：每一页只算一次每列的「想要的宽度」（表头和这一页最宽的值，`maxColWidth` 封顶），缓存在持有数据的那个 tab 上（结果 tab 和表 tab 都是），数据换了（取回新页、翻页、刷新、转置、隐藏列、调列顺序、编辑单元格后的值变了）就失效；按 room 压缩那一步照旧每帧做，因为它取决于 pane 宽。一个按键里 `layout()` 仍算两次，热点消掉后已达标，不合并（审查时定；reviewer 复测 min of 3：结果区按键 7.44 → 0.60 ms、滚轮 8.26 → 0.62 ms，表格 0.63 → 0.65 ms，指针 0.49 → 0.53 ms）。
- [ ] 目标同 F8.20：结果区 500 行时每事件 < 1ms，按住 `j` 时光标行不落后于按键；表格 pane 的数字不变差。
- [ ] 优化前后的数字写进 §15「性能」（结果区一行），没达到就写明差距和原因。
- [ ] 细节（已定）：缓存用值快照比对判断失效，不在 tab 上挂失效事件（worker 送审时定，更不容易漏）；`layout()` 合并成一次没做，热点消掉后已经 < 1ms。不改一页 500 行的默认（F10.4），不做只渲染可见行之外的「只算可见行的列宽」（列宽要全页一致，不然滚动时列会跳）。

**验收**
- [ ] benchmark 在 `go test` 里能跑，§15 有优化前后的数字。
- [ ] 单测：列宽缓存在数据换了之后失效（翻页、刷新、转置、隐藏列各一例），没换时同一页两次 `layout()` 不重算（计数或 benchmark 的 allocs）。
- [ ] e2e：结果 tab 500 行（mt_task 或 seed 里凑够 500 行的表），tmux 里每秒 30 个 `j` 持续 3 秒，最后一个按键后 100ms 内光标在正确的行；期间 sqlmux 的 CPU 用 `ps -o cputime` 前后差算，不超过一个核的 20%（机器忙时放宽并写明）。
