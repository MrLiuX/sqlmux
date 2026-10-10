---
paths:
  - internal/editor/**
---
编辑器行为以 nvim 为准：改编辑器先往 `internal/editor/testdata/cases.txt` 加用例，`just generate` 调 nvim 生成期望结果，最后才改实现（tech-design §15）。
