# M7 工程质量 · 任务清单

- **状态**：draft（用户 2026-10-05 提出）。M6 全部 passed 之后开工；`.golangci.yml` 的规则由 reviewer 在 M6 的审查队列清空后先起草，不等 M6 结束。开工前 worker 补充 F7.2 的清单，决策者确认后改为 todo。
- **目标**：用 golangci-lint 守住代码质量和规范，每个 commit、每次审查都先过 lint；仓库有 CI；常规操作统一走 `just`。
- **范围**：tech-design §4（新增文件）、§15「静态检查」「CI」「常规操作走 just」；AGENTS.md 的 worker 和 reviewer 规则。
- **依赖**：M6 F6.1–F6.7 全部 passed（lint 的修改会碰到所有包，不和 M6 的修复撞车）。
- **完成标准**：
  - F7.1、F7.2 passed；
  - `just check` 在干净的仓库上全绿，CI 在 GitHub 上跑过一次绿的；
  - AGENTS.md 里的命令都改成 just 的入口。
- **审查节点**：
  1. F7.2 之后：F7.1、F7.2 一批。lint 规则本身由 reviewer 起草，所以这一批 reviewer 重点看的是修 lint 报告时有没有改变行为（同审 pruner 的 commit：测试期望一个都不改）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F7.1 golangci-lint 与 CI · 状态：draft

- **依赖**：M6 全部 passed
- **涉及**：仓库根 `.golangci.yml`、`.github/workflows/ci.yml`，以及 lint 报告涉及的所有包

**开发**
- [ ] reviewer 起草 `.golangci.yml`（golangci-lint v2 格式；linter 清单和理由见 §15「静态检查」），发给 worker 和决策者；worker 原样提交。
- [ ] worker 在全仓库跑 `golangci-lint run ./...`，修到零报告：一类报告一个 commit（如 `lint: errorlint`），**不改行为**，测试期望一个都不改；确实该屏蔽的用带理由的 `//nolint`。修的过程中觉得某条规则不合适，提给 reviewer 和决策者，不自己改配置。
- [ ] `.github/workflows/ci.yml`：push 和 PR 到 main 时跑 lint（golangci-lint-action）、vet、`go test ./...`、`go test -tags integration ./...`（PG 17、MySQL 8.4 用 services 起，用 `testdata/seed` 导入）。Go 版本从 go.mod 读。e2e 不进 CI。
- [ ] 本地工具：golangci-lint 升到 v2（现在装的 v1.63.4 不支持 Go 1.26），版本写进 justfile 的注释和 CI。

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
