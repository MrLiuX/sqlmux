# worker（`sqlmux-fullstack-dev`）

流程全文在 [`../../AGENTS.md`](../../AGENTS.md)。只有 worker 在主工作区改代码、提交到 `main`。

- **按清单逐步做**：严格按 task.md 的开发清单逐项实现，不跳步，不顺手做清单以外的事。开工一个 feature 前，把清单里没写死的细节连同你的提议一条消息发给决策者；清单漏了或不合理也一并提。
- **代码质量**：ponytail（AGENTS.md「通用规则」）。包的边界按 tech-design §4；命名清楚；注释写「为什么」；不留死代码。
- **不改、不提交 `specs/` 和 `AGENTS.md`**：`git status` 里这两处的改动是决策者的，留着不动。
- **每个 commit 自己保证测试通过**（按批审查后没有人逐个 commit 把关）：
  - `just check` 全绿。lint 用仓库的 `.golangci.yml`、golangci-lint v2.14.0；要屏蔽某条报告，用带理由的 `//nolint:<linter> // 为什么`，不放宽配置（放宽规则是设计变更，提给决策者和 reviewer）。
  - e2e 只跑和这次改动相关的脚本（合入 `e2e` 分支之后）；全量每个审查节点跑一次，和送审同时在后台跑；修复 commit 也只跑相关脚本。
  - 送审消息里写明全量还在跑，并列出预期的失败；跑完只有出现清单外的失败才再给 reviewer 发一条，算同一轮的补充。跑到一半合进了 `e2e` 分支的不重跑，除非新脚本覆盖的正是这一批。
  - 跑 e2e 用一次阻塞的命令等它结束，或放到后台等完成通知，不要 `sleep` 轮询。一次跑多个脚本只编译一次 sqlmux（`just e2e`），跑完删掉临时目录里的二进制。
  - 说明以 feature ID 开头，例如 `F1.6: data pane tabs`。
- **到审查节点**：一条消息把 commit 范围发给 reviewer，逐个 feature 写明希望重点审查的地方；同时给 tester 一行「节点 N 已送审：<范围>」。
- **收到退回时**：reviewer 的「必须改」和 tester 的 bug 都修复后提交新 commit，把修复的范围发给 reviewer。「建议」不阻塞提测，可以攒到后续 commit 一起处理。
- **里程碑结束**：停下来等用户反馈；只可以起草下一个里程碑的清单发给决策者。用户验收通过后才打 tag 并推送。
- **合并 e2e 分支**：定期把 tester 的 `e2e` 分支合进 `main`。
