# sqlmux 开发计划（索引）

范围和设计以 [`tech-design.md`](tech-design.md) 为准。每个里程碑一个目录，目录里的 `task.md` 是这个里程碑的任务清单。协作流程、审查节点怎么走、谁改什么、里程碑怎么结束，全在 [`../AGENTS.md`](../AGENTS.md)，这里不重复。

| 里程碑 | 任务清单 | 目标 | 状态 |
|---|---|---|---|
| M0 骨架 | [m0-skeleton/task.md](m0-skeleton/task.md) | 界面骨架、按键系统、鼠标、命令面板、主题；不连数据库 | 完成（tag `m0`，7c1f353） |
| M1 浏览 | [m1-browse/task.md](m1-browse/task.md) | 连接 PG，浏览 schema 与表数据，命令面板的快速 SQL；只读 | 完成（tag `m1`，fc9cfc6） |
| M2 编辑 | [m2-edit/task.md](m2-edit/task.md) | 编辑单元格、选项浮层、保存与刷新 | 完成（用户 2026-10-02 与 M3、M4 一起验收，tag `m2`） |
| M3 console | [m3-console/task.md](m3-console/task.md) | vim 编辑器、执行、底部结果区、格式化、补全、schema 下拉；表格与 console 混放 tab | 完成（用户 2026-10-02 验收，tag `m3`） |
| M4 命令面板 | [m4-palette/task.md](m4-palette/task.md) | 快速 SQL 接上 M3、DDL 预览、面板里 tab 的层级 | 完成（用户 2026-10-02 验收，tag `m4`）；验收意见排进 M5 的 F5.5–F5.9 |
| M5 工作现场 | [m5-workspace/task.md](m5-workspace/task.md) | 一个 session 多个连接、多 session / window、只读与事务模式、MySQL；先做 M4 验收意见 F5.5–F5.9 | 完成（F5.1–F5.19 passed，完整回归在 d156ec7 上全过，2026-10-05；tag `m5`） |
| M6 配置 | [m6-config/task.md](m6-config/task.md) | 键位配置收尾、主题、持久化、界面中英文切换、布局文件（保存与恢复工作现场）、可以关掉 vim 模式 | 完成（F6.1–F6.7 passed，完整回归在 be5f6f3 上全过，2026-10-06；tag `m6`） |
| M7 工程质量 | [m7-quality/task.md](m7-quality/task.md) | golangci-lint 与 CI、justfile、凭据不落明文、分字段的新建连接表单与无连接启动（用户 2026-10-05 提出） | 完成（F7.1–F7.6 passed，ec4aaad，2026-10-07；tag `m7`；CI 2026-10-10 首次在 GitHub 上跑绿） |
| M8 第一轮使用反馈 | [m8-feedback/task.md](m8-feedback/task.md) | 用户照 WALKTHROUGH 走完 M5–M7 后的 20 条意见：补全和格式化的 bug、列宽、边框样式、默认排序、window / session / console 改名、连接的修改与删除 | 完成（F8.1–F8.23 passed，0c023d2，完整回归 118 个脚本全过，2026-10-10；tag `m8`） |
| M9 第二轮使用反馈 | [m9-feedback2/task.md](m9-feedback2/task.md) | 用户 2026-10-10 在 M8 验收期间提的 10 条：WHERE 历史 / 收藏的顺手操作、表格 `^` / `C-r` / 复制为 JSON、缩放提示、DDL 查看 | 完成（F9.1–F9.10 passed，93e9d8c，完整回归 130 个脚本全过，2026-10-10；tag `m9`，`v0.1.0` 在 ffbcec5；用户 10-10 定：先打 tag，之后自己慢慢验收） |
| M10 第三轮使用反馈 | [m10-feedback3/task.md](m10-feedback3/task.md) | 用户 2026-10-10 在 M9 审查期间提的 9 条：表格多选、flash 式跳转、树去掉 pane 层、连接分组、WHERE 下拉撑满、新建 session 先要名字、console 结果 500 行并翻页、COLS 调列顺序、侧栏去掉键位文字、删开着的 console；最后 console 改名 query（单独一个节点） | 完成（F10.1–F10.12 passed，08d9009，完整回归 2982 项全过，2026-10-11；tag `m10`、`v0.2.0`；用户照 M9 的办法先打 tag 再验收） |
| M11 分发 | [m11-distribution/task.md](m11-distribution/task.md) | GoReleaser 与 release workflow、`--version`、Homebrew tap（源码编译的 formula）、npm / npx（壳包加平台子包） | todo（2026-10-10 用户定；2026-10-11 M10 passed 后开工） |

## 发布

2026-10-10 推送：`m5`–`m9` 打在各自最后一个产品 commit 上，`v0.1.0` 打在 ffbcec5（m9 之后只多一个让 keychain 测试在 Linux 上过的 commit；93e9d8c 的首次 CI 是红的，tag 打在绿的那个），`go install` 还装不了：go.mod 的 module 是 `sqlmux` 而不是 `github.com/kackerx/sqlmux`，M10 F10.12 改掉后下一个 tag 才能 `go install github.com/kackerx/sqlmux/cmd/sqlmux@latest`；2026-10-11 M10 passed 后推送 `m10`、`v0.2.0`（de62155；M10 新功能，配置键 console → query 不兼容，所以升 minor），CI run 38096306240 绿，第一个 GitHub Release 发在 v0.2.0 上，只有源码包和说明，`go install …@v0.2.0` 验过；`kackerx/homebrew-tap` 仓库已建（空，M11 F11.2 填）；GoReleaser、Homebrew、npm 是 M11。每个里程碑的 tag 在用户验收通过后由 worker 打并推送（M5–M9 由用户 10-10 定为先打 tag 再慢慢验收）。

## 任务文件的格式

每个 `task.md` 开头写清五项：目标、范围（PRD 编号与 tech-design 章节）、依赖、完成标准、**审查节点**。之后每个 feature 一节：

- **标题行**：`## <ID> <名称> · 状态：<状态>`。状态为 passed 时，在后面注明 commit sha 和 e2e sha。
- **依赖**：必须先完成的 feature。
- **涉及**：会改动的包或文件，用来判断改动范围、避免冲突。
- **开发**：worker 按顺序实现的清单。
- **验收**：tester 的测试项，每项都要能用命令、golden 或 e2e 脚本验证。写着「e2e」的验收项里，属于布局、颜色、位置的，可以由渲染 golden 验证（`roles/tester.md`）。

**审查节点**：worker 按 feature 提交，reviewer 不逐个审，只在节点上整批审，通过后整批提测。节点由决策者按工作量定：小的里程碑只在最后一个 feature 之后设一个；大的里程碑在中间合适的位置多设几个，让一批的改动量保持在一次能审完的范围内。用户要求拉长节点、减少来回：一轮改进项或一个中等大小的里程碑只设一两个节点。

## 状态

| 状态 | 含义 |
|---|---|
| `draft` | 决策者起草，开发清单等 worker 补充 |
| `todo` | 决策者已确认，可以开工 |
| `passed（sha；e2e sha）` | 所在批次审查、测试都已通过，由决策者填写 |

不再有 doing、reviewing、testing、failed 这些中间状态：按批审查之后，它们只增加记账，不提供信息。
