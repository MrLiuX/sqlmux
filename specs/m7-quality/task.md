# M7 工程质量 · 任务清单

- **状态**：**开发完成，等用户验收**（2026-10-06）。节点 1（F7.1、F7.2）passed（5b76c68）；节点 2（F7.3、F7.4）passed（491ea8b；e2e 090e2ed，全量 89 个脚本 2230 项全过）。用户验收时追加 F7.5（明文密码不再警告、不改权限），做完一起验收，通过后打 tag `m7`。CI 在 GitHub 上的第一次运行等推送。F7.3、F7.4 的细节照常开工前由 worker 提议。用户验收 M5 + M6 和 M7 开发并行：验收版是 1489851 编的，M7 的改动不影响它。
- **目标**：用 golangci-lint 守住代码质量和规范，每个 commit、每次审查都先过 lint；仓库有 CI；常规操作统一走 `just`；连接凭据不再需要明文落盘（F7.3）；没有连接也能启动，用表单新建（F7.4）。
- **范围**：tech-design §4（新增文件）、§15「静态检查」「CI」「常规操作走 just」；AGENTS.md 的 worker 和 reviewer 规则。
- **依赖**：M6 F6.1–F6.7 全部 passed（lint 的修改会碰到所有包，不和 M6 的修复撞车）。
- **完成标准**：
  - F7.1–F7.5 passed；
  - `just check` 在干净的仓库上全绿，CI 在 GitHub 上跑过一次绿的；
  - AGENTS.md 里的命令都改成 just 的入口。
- **审查节点**：
  1. F7.2 之后：F7.1、F7.2 一批。lint 规则本身由 reviewer 起草，所以这一批 reviewer 重点看的是修 lint 报告时有没有改变行为（同审 pruner 的 commit：测试期望一个都不改）；
  2. F7.4 之后：F7.3、F7.4 一批（凭据和新建连接表单，用户要验收）；
  3. F7.5 之后：F7.5 单独一批（用户 2026-10-07 追加，一个小改动）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F7.1 golangci-lint 与 CI · 状态：passed（5b76c68；e2e 5b76c68）

- **依赖**：M6 全部 passed
- **涉及**：仓库根 `.golangci.yml`、`.github/workflows/ci.yml`，以及 lint 报告涉及的所有包

**开发**
- [x] reviewer 起草 `.golangci.yml`（golangci-lint v2 格式；linter 清单和理由见 §15「静态检查」），发给 worker 和决策者；worker 原样提交。2026-10-05 已起草（v2.14.0），决策者接受全部取舍；398899a 上 131 条报告，其中生产代码的 20 条 errcheck 都是可以安全丢掉的，写成 `_ =`；真问题有 ineffassign 1、wastedassign 4、unparam 1、unused 1、7 处变量名遮住内置函数。
- [x] worker 在全仓库跑 `golangci-lint run ./...`，修到零报告：一类报告一个 commit（如 `lint: errorlint`），**不改行为**，测试期望一个都不改；确实该屏蔽的用带理由的 `//nolint`。修的过程中觉得某条规则不合适，提给 reviewer 和决策者，不自己改配置。
  - 细节（worker 2026-10-06 提议，已定）：第一个 commit 只放 `.golangci.yml`（它自己的 lint 是红的，之后一个 linter 一个 commit 修干净，最后一个 commit 是 ci.yml；「每个 commit lint 零报告」从 F7.1 落地后算起）。修法：生产的 errcheck 写 `_ =`，测试里的检查失败 `t.Fatal`；exhaustive 只在故意不全的 switch 上加带注释的空 `default:`，该列全成员的就列全；revive 遮住内置函数的改名、未用参数改 `_`；unparam 去掉没人用的返回值和常量参数，改调用点、期望不动；editor 里 vim 移植的三处 wastedassign 做最小改动并跑 nvim 差分；misspell 的 `"SEL\nECT"` 加 `//nolint:misspell`。
- [x] `.github/workflows/ci.yml`：push 和 PR 到 main 时跑 lint（golangci-lint-action）、vet、`go test ./...`、`go test -tags integration ./...`（PG 17、MySQL 8.4 用 services 起，用 `testdata/seed` 导入）。Go 版本从 go.mod 读。e2e 不进 CI。job 拆成 lint / test / integration 三个；本机跑不了 Actions，第一次真绿要等推送。
- [x] 本地工具：golangci-lint v2.14.0（`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0`；现在装的 v1.63.4 不支持 Go 1.26），版本钉进 justfile 和 CI。装到默认位置 `~/go/bin`，覆盖用户的 v1.63.4（用户 2026-10-05 定：用最新版直接覆盖）；钉的版本就是当时的最新版。

**验收**
- [x] `golangci-lint run ./...` 零报告；`go test ./...`、集成测试、全量 e2e 都过且期望值没改（tester 对比 e2e 分支）。
- [ ] 在 GitHub 上跑过一次绿的 CI（推送要等用户验收打 tag 之后，所以这一项在 M6/M7 验收通过、推送后由决策者确认）。

## F7.2 justfile · 状态：passed（5b76c68；e2e 5b76c68）

- **依赖**：F7.1（`just lint` 要用它的配置）
- **涉及**：仓库根 `justfile`、AGENTS.md、README.md

**开发**（worker 2026-10-06 的清单，决策者确认）
- [x] `justfile`（每条就是现在手敲的命令，不加新逻辑）：`build`（到 `bin/`）、`run *args`、`test`、`test-integration`（`SQLMUX_TEST_PG` / `SQLMUX_TEST_MYSQL` 用 `env()` 给 compose 的默认值，用户设了不覆盖）、`e2e *scripts`（先编译到临时目录一次，`E2E_BIN` / `E2E_NOBUILD=1` 逐个跑，不带参数跑全部，一个失败不中断、最后汇总并退出非 0，跑完删二进制）、`lint`（钉 v2.14.0，版本不对报错并给 `go install` 命令）、`fmt`、`tidy`、`check`（vet 含 integration tag + test + test-integration + lint + gofmt 为空 + tidy 后无改动，任一失败即停；只有一个 check，集成测试失败时末尾印一行「if the databases are not up: just db-up」——条件句不算误导，几十条 connection refused 后面要有个出口（审查时定，worker 自查删掉后加回），不另加 check-fast）、`golden *pkgs`（默认 app、ui，自动把 XDG 三个目录指到临时目录、PATH 前放假 pbcopy）、`generate`（nvim 差分）、`db-up` / `db-down` / `db-reset`（`down -v` 再 up，改 seed 后用）、`keys *args`、`clean`；默认配方是 `--list`。
- [x] CI 的 job 拆开跑（lint / test / integration），justfile 不管 CI。
- [x] AGENTS.md 和 README 里的命令改成 just 的入口（决策者改）：vet / test / gofmt / tidy / lint → `just check`；全量 e2e → `just e2e`；更新 golden → `just golden`；`docker compose up -d postgres` → `just db-up`；`go generate ./internal/editor` → `just generate`。

**验收**
- [x] `just --list` 列出全部入口，每个都能跑；`just check` 在干净仓库上全绿；`just e2e f0.1 f0.2` 只编译一次。

## F7.3 凭据：环境变量展开、表单里的密码走钥匙串 · 状态：passed（491ea8b；e2e 090e2ed）

用户 2026-10-05 提出：密码明文放在 connections.toml 里不放心；DataGrip 输入密码只显示 `*`。现状：已有 `password_cmd` / `password_env` / 明文 `password` / pgpass 四种来源，明文且文件可被其他用户读时 toast 警告；表单没有密码字段，只能填 dsn 和 password_cmd。别的项目：lazysql 把带密码的 URL 明文写在配置里；pgtui、pgcli 用 keyring 存进系统钥匙串；psql / usql 靠 `PGPASSWORD` 和 `~/.pgpass`；DBeaver 用固定密钥加密（形同明文）；DataGrip 存进 IDE 的密码库（系统钥匙串或 KeePass），输入框打 `*`。

- **依赖**：F5.1（表单）
- **涉及**：`internal/config`（`${VAR}` 展开、写 password_cmd）、`internal/app`（表单的密码字段）、`internal/ui`（输入框的掩码画法）

**开发**（§13「凭据」的三条新内容）
- [x] `dsn`、`password` 里的 `${VAR}` 在建连前展开（正则，不用 `os.Expand`），只认 `${}`，没设置报错；启动时的连接报错退出，其他连接标失败、toast（同 `password_env`）。
- [x] 表单加 `password` 字段，输入画 `*`（`ui.Input` 加一个掩码开关，光标位置照常）；保存时按平台存进钥匙串并写 `password_cmd`，没有钥匙串工具时写明文 + 0600 + toast。dsn 里带了密码（`user:pass@`）照旧交给驱动，不拆。
  - 细节（worker 提议，已定）：展开用正则 `\$\{([^}]+)\}` + `os.LookupEnv`，不用 `os.Expand`（它连 `$VAR` 一起展开），不做 `${X:-默认}`；变量没设置或为空都报错；在 `dial` 里 `Secret()` 之前展开，启动连接的错误照旧打印退出。掩码在 `ui.FormField` 上（`Mask`），按字素画 `*`。`config.StorePassword` 按平台包装 `security` / `secret-tool`，名字里的 `'` 转义；macOS 的 `security -w <密码>` 会短暂出现在进程参数里，`ponytail:` 标出。密码留空什么都不存。**钥匙串工具在但执行失败**（锁着、被拒）：不保存、不降级成明文，错误（stderr 第一行）写在表单的错误行，表单留着让用户重试或清掉密码（决策者定）。
- [x] README 的配置一节写清四种来源、`${VAR}`、钥匙串（决策者改 README）。

**验收**
- [x] 单测：`${VAR}` 展开、`$VAR` 不展开、缺变量报错带变量名；表单保存后 connections.toml 里没有明文密码、有 password_cmd（用假的 `security` / `secret-tool` 放在 PATH 前面，e2e 的做法）；两者都没有时写明文并有 toast。
- [x] e2e：表单里输入密码显示 `*`；保存后能连上（假钥匙串命令返回密码）；`dsn = "${X}"` 启动时 X 没设置的报错文字。

## F7.4 新建连接表单重做：分字段、无连接时启动、侧栏 ＋ 按钮 · 状态：passed（491ea8b；e2e 090e2ed）

用户 2026-10-05 提出：没有配置连接时启动直接给表单；表单按 host / port / user / password / database 分字段填，持久化到 connections.toml；界面上加一个新建连接的按钮；表单居中、好看点。

- **依赖**：F7.3（密码字段和钥匙串）、F5.1
- **涉及**：`internal/app`（表单、启动时没有连接的工作区、侧栏按钮）、`internal/ui`（表单的绘制：分段开关、折叠的「高级」、按钮）、`internal/config`（拼 dsn）、`cmd/sqlmux`（没有连接不再退出）

**开发**（§7.8「新建连接表单」「新建连接的入口」，§14「启动时找不到连接」）
- [x] 表单重做：字段 name / engine / host / port / user / password / database / read_only，折叠的「高级」里是 dsn 和 password_cmd；每行「Nerd Font 图标 + 字段名 + 输入框」，图标清单见 §7.8 / §7.7，ascii 模式有替代字符；样式照 §7.8；保存时拼 dsn，文件格式不变。
- [x] 没有 connections.toml 或没有连接时照常进界面：空树一行提示，自动打开表单；保存第一个连接后连上并展开；`esc` 关掉表单也能用。`sqlmux <名字>` 找不到照旧报错退出。
- [x] 侧栏第一行右端 `＋` 按钮打开表单；空树那一行可点击。
- [x] 细节（worker 2026-10-06 提议，已定，写进 §7.8 / §7.7 / §14）：`config.NoConnections(err)`（不导出的错误类型加判断函数，不用哨兵：报错带路径、运行时翻译，哨兵拼不出来），`main.go` 没给连接名、没 `--layout` 时遇到它就 `app.OpenEmpty()`；零连接的 session 里 `a.conn()` 返回 nil，需要连接的命令在入口 toast，加一个「零连接下每条命令都跑一遍不 panic」的测试；保存第一个连接后连上、展开、建 console_1；「高级」dsn 非空时 host / port / user / database 变灰，password 不灰；port 预填并跟 engine 直到用户改过；dsn 用 `net/url` 拼，不做 sslmode 选项；ascii 图标 tag `#` host `@` port `:` user `~` database `=` advanced `+` link `&`；`last.toml` 在零连接时忽略；不加占位提示字。F7.3 让 e2e f5.1 的表单 13 项失效，连同 F7.4 的重做一起由 tester 改。

**验收**
- [x] golden：表单（postgres、mysql 两种 engine，一个字段出错，高级展开）、空树加提示行、侧栏 `＋`。
- [x] 单测：拼 dsn（PG / MySQL、带特殊字符的 user 和 database 要转义）、校验、engine 切换 port 跟随。
- [x] e2e：`XDG_CONFIG_HOME` 指向空目录启动，界面出来、表单打开；填好保存，connections.toml 里多出一条、树里连上并展开、能打开表；点 `＋` 再开一次表单，`esc` 关掉。

## F7.5 明文密码是用户自己负责的兜底：不警告、不改权限 · 状态：todo

用户 2026-10-07 定：手写 connections.toml 里的明文密码要能直接用，程序不要替用户 chmod、不要每次启动提醒；不手写的用户走表单，密码进钥匙串。

- **依赖**：F7.3
- **涉及**：`internal/config`（去掉权限检查和 chmod）、`internal/app`（启动时的那条 toast）、README

**开发**（§13「凭据」）
- [ ] 去掉「connections.toml 里有明文密码，且其他用户可读，建议 chmod 600」的检查和 toast（含 en 表里的条目）。
- [ ] F7.3 的明文兜底不再 `chmod 0600` 已有文件；新建文件照旧 0600。toast 文字不变。
- [ ] README 配置一节：四种密码来源、`${VAR}`、钥匙串按平台的情况、明文能用但安全自负（决策者改）。

**验收**
- [ ] 单测：0644 的文件带明文密码加载后没有警告；F7.3 的明文兜底后文件权限不变。
- [ ] e2e：f7.3 里「明文 + 0600」那项改成权限不变；启动时没有 chmod 的 toast。
