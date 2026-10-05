# M7 工程质量 · 任务清单

- **状态**：draft（用户 2026-10-05 提出）。M6 全部 passed 之后开工；`.golangci.yml` 的规则由 reviewer 在 M6 的审查队列清空后先起草，不等 M6 结束。开工前 worker 补充 F7.2 的清单，决策者确认后改为 todo。
- **目标**：用 golangci-lint 守住代码质量和规范，每个 commit、每次审查都先过 lint；仓库有 CI；常规操作统一走 `just`；连接凭据不再需要明文落盘（F7.3）；没有连接也能启动，用表单新建（F7.4）。
- **范围**：tech-design §4（新增文件）、§15「静态检查」「CI」「常规操作走 just」；AGENTS.md 的 worker 和 reviewer 规则。
- **依赖**：M6 F6.1–F6.7 全部 passed（lint 的修改会碰到所有包，不和 M6 的修复撞车）。
- **完成标准**：
  - F7.1–F7.4 passed；
  - `just check` 在干净的仓库上全绿，CI 在 GitHub 上跑过一次绿的；
  - AGENTS.md 里的命令都改成 just 的入口。
- **审查节点**：
  1. F7.2 之后：F7.1、F7.2 一批。lint 规则本身由 reviewer 起草，所以这一批 reviewer 重点看的是修 lint 报告时有没有改变行为（同审 pruner 的 commit：测试期望一个都不改）；
  2. F7.4 之后：F7.3、F7.4 一批（凭据和新建连接表单，用户要验收）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F7.1 golangci-lint 与 CI · 状态：draft

- **依赖**：M6 全部 passed
- **涉及**：仓库根 `.golangci.yml`、`.github/workflows/ci.yml`，以及 lint 报告涉及的所有包

**开发**
- [x] reviewer 起草 `.golangci.yml`（golangci-lint v2 格式；linter 清单和理由见 §15「静态检查」），发给 worker 和决策者；worker 原样提交。2026-10-05 已起草（v2.14.0），决策者接受全部取舍；398899a 上 131 条报告，其中生产代码的 20 条 errcheck 都是可以安全丢掉的，写成 `_ =`；真问题有 ineffassign 1、wastedassign 4、unparam 1、unused 1、7 处变量名遮住内置函数。
- [ ] worker 在全仓库跑 `golangci-lint run ./...`，修到零报告：一类报告一个 commit（如 `lint: errorlint`），**不改行为**，测试期望一个都不改；确实该屏蔽的用带理由的 `//nolint`。修的过程中觉得某条规则不合适，提给 reviewer 和决策者，不自己改配置。
- [ ] `.github/workflows/ci.yml`：push 和 PR 到 main 时跑 lint（golangci-lint-action）、vet、`go test ./...`、`go test -tags integration ./...`（PG 17、MySQL 8.4 用 services 起，用 `testdata/seed` 导入）。Go 版本从 go.mod 读。e2e 不进 CI。
- [ ] 本地工具：golangci-lint v2.14.0（`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0`；现在装的 v1.63.4 不支持 Go 1.26），版本钉进 justfile 和 CI。装到默认位置 `~/go/bin`，覆盖用户的 v1.63.4（用户 2026-10-05 定：用最新版直接覆盖）；钉的版本就是当时的最新版。

**验收**
- [ ] `golangci-lint run ./...` 零报告；`go test ./...`、集成测试、全量 e2e 都过且期望值没改（tester 对比 e2e 分支）。
- [ ] 在 GitHub 上跑过一次绿的 CI（推送要等用户验收打 tag 之后，所以这一项在 M6/M7 验收通过、推送后由决策者确认）。

## F7.2 justfile · 状态：draft

- **依赖**：F7.1（`just lint` 要用它的配置）
- **涉及**：仓库根 `justfile`、AGENTS.md、README.md

**开发**（清单由 worker 开工前补充；要有的入口见 §15「常规操作走 just」）
- [ ] `justfile`：build（到 `bin/`）、run、test、test-integration、e2e [脚本…]（只编译一次 sqlmux，各脚本共用）、lint、fmt、tidy、check（vet + test + lint + gofmt + tidy 检查）、golden（更新 golden）、db-up / db-down、keys（导出）、clean。
- [ ] AGENTS.md 和 README 里的命令改成 just 的入口（决策者改 AGENTS.md / README，worker 提供对应关系）。

**验收**
- [ ] `just --list` 列出全部入口，每个都能跑；`just check` 在干净仓库上全绿；`just e2e f0.1 f0.2` 只编译一次。

## F7.3 凭据：环境变量展开、表单里的密码走钥匙串 · 状态：draft

用户 2026-10-05 提出：密码明文放在 connections.toml 里不放心；DataGrip 输入密码只显示 `*`。现状：已有 `password_cmd` / `password_env` / 明文 `password` / pgpass 四种来源，明文且文件可被其他用户读时 toast 警告；表单没有密码字段，只能填 dsn 和 password_cmd。别的项目：lazysql 把带密码的 URL 明文写在配置里；pgtui、pgcli 用 keyring 存进系统钥匙串；psql / usql 靠 `PGPASSWORD` 和 `~/.pgpass`；DBeaver 用固定密钥加密（形同明文）；DataGrip 存进 IDE 的密码库（系统钥匙串或 KeePass），输入框打 `*`。

- **依赖**：F5.1（表单）
- **涉及**：`internal/config`（`${VAR}` 展开、写 password_cmd）、`internal/app`（表单的密码字段）、`internal/ui`（输入框的掩码画法）

**开发**（§13「凭据」的三条新内容）
- [ ] `dsn`、`password` 里的 `${VAR}` 用 `os.Expand` 展开，只认 `${}`，没设置报错；启动时的连接报错退出，其他连接标失败、toast（同 `password_env`）。
- [ ] 表单加 `password` 字段，输入画 `*`（`ui.Input` 加一个掩码开关，光标位置照常）；保存时按平台存进钥匙串并写 `password_cmd`，没有钥匙串工具时写明文 + 0600 + toast。dsn 里带了密码（`user:pass@`）照旧交给驱动，不拆。
- [ ] README 的配置一节写清四种来源、`${VAR}`、钥匙串（决策者改 README）。

**验收**
- [ ] 单测：`${VAR}` 展开、`$VAR` 不展开、缺变量报错带变量名；表单保存后 connections.toml 里没有明文密码、有 password_cmd（用假的 `security` / `secret-tool` 放在 PATH 前面，e2e 的做法）；两者都没有时写明文并有 toast。
- [ ] e2e：表单里输入密码显示 `*`；保存后能连上（假钥匙串命令返回密码）；`dsn = "${X}"` 启动时 X 没设置的报错文字。

## F7.4 新建连接表单重做：分字段、无连接时启动、侧栏 ＋ 按钮 · 状态：draft

用户 2026-10-05 提出：没有配置连接时启动直接给表单；表单按 host / port / user / password / database 分字段填，持久化到 connections.toml；界面上加一个新建连接的按钮；表单居中、好看点。

- **依赖**：F7.3（密码字段和钥匙串）、F5.1
- **涉及**：`internal/app`（表单、启动时没有连接的工作区、侧栏按钮）、`internal/ui`（表单的绘制：分段开关、折叠的「高级」、按钮）、`internal/config`（拼 dsn）、`cmd/sqlmux`（没有连接不再退出）

**开发**（§7.8「新建连接表单」「新建连接的入口」，§14「启动时找不到连接」）
- [ ] 表单重做：字段 name / engine / host / port / user / password / database / read_only，折叠的「高级」里是 dsn 和 password_cmd；样式照 §7.8；保存时拼 dsn，文件格式不变。
- [ ] 没有 connections.toml 或没有连接时照常进界面：空树一行提示，自动打开表单；保存第一个连接后连上并展开；`esc` 关掉表单也能用。`sqlmux <名字>` 找不到照旧报错退出。
- [ ] 侧栏第一行右端 `＋` 按钮打开表单；空树那一行可点击。
- [ ] 细节（engine 切换时 port 的默认值怎么跟、「高级」展开时上面字段怎么变灰、sslmode 要不要给选项）开工前 worker 提议。

**验收**
- [ ] golden：表单（postgres、mysql 两种 engine，一个字段出错，高级展开）、空树加提示行、侧栏 `＋`。
- [ ] 单测：拼 dsn（PG / MySQL、带特殊字符的 user 和 database 要转义）、校验、engine 切换 port 跟随。
- [ ] e2e：`XDG_CONFIG_HOME` 指向空目录启动，界面出来、表单打开；填好保存，connections.toml 里多出一条、树里连上并展开、能打开表；点 `＋` 再开一次表单，`esc` 关掉。
