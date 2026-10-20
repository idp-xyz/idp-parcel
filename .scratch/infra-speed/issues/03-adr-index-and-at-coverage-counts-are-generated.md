# 03 ADR 索引与 AT 盘点计数由工具生成，票不再手改

Category: enhancement
Status: ready-for-agent
Blocked by: 无
地盘：`tools/adr-index/`（新，形态照 `tools/mechanism-inventory`）、`docs/adr/README.md`、各 ADR 的 `Status` 行（只改这一行）、`.scratch/at-coverage-inventory.md` 的「总览」两张计数表、`docs/agents/workflow.md` 或 `parallel-sessions.md` 里进 main 步骤中「重生成机制清点」那一处（追加同类两步）
出处：用户 2026-10-19 同意「热点清单冲突」基础设施票。近两周提交频次：`docs/adr/README.md` 68、`.scratch/at-coverage-inventory.md` 41。前者每张带新 ADR 的票都往同一个列表末尾追加，两票同批必撞；部分停用注记两侧同改一行时，自动解法曾留下同一 ADR 的新旧两行（2026-10-19 进 main 时 0205、0206、0207、0209、0210、0217 各出过一次重复）。后者每张点名 AT 的票都改同两张计数表。

## 要什么

- **`docs/adr/README.md` 的两个列表（已接受 / 已被取代）由 `tools/adr-index` 从各 ADR 文件生成**：行文 `- [ADR-NNNN：<标题>](./<文件>)`，标题取 H1；部分停用与取代的注记取自该 ADR 的 `Status` 行。README 顶部的说明段仍手写，列表落在一对标记之间，工具只重写标记内。`-check` 模式：生成结果与文件不同即非零退出。
- **一次性迁移注记**：README 里现有每一条「｜**部分停用**……」与「→ 由 … 取代：……」注记，凡对应 ADR 的 `Status` 行尚未承载的，搬进那一行（`Status` 行在本仓被取代时本就会改写，见 README 顶部那段；正文一字不动）。搬完后第一次生成的列表与搬之前的 README 逐条对照：每个 ADR 一行、注记文字不丢不改，差异只许是格式归一；对照结果写进完成记录。
- **README 顶部「部分停用」规则改写**：被停条款与适用场景写在被停用 ADR 的 `Status` 行（README 由它生成），不再要求手改 README 行。别处凡写「在 docs/adr/README.md 该行写被停条款」的现行指引（`AGENTS.md`、`docs/agents/*.md`、各 workflow 提示不算）一并改指向；只追溯的旧票面不改。
- **AT 盘点的「总览」两张计数表由工具从同文件各分节的行数重算**（同一个工具加子命令，或 `tools/` 下另一个小工具，作者定）；票只移行，不碰计数。
- 进 main 步骤里在「重生成机制清点」旁加上这两步（生成 ADR 索引、重算 AT 计数），单独成笔，与机制清点同口径（「生成物不占号」）。

## 不变式

- 任何已接受 ADR 的正文不改；只动 `Status` 行。
- 生成器确定性：同输入同输出，排序按 ADR 号。
- 注释与文档中文；跨文件引用不用行号、不数别处。

## 验证

`go test ./tools/...`；`go run ./tools/adr-index -check` 在提交后的树上退 0；README 前后逐条对照（见上）。本票不动 `internal/` 与 `cmd/`，不必跑全量。
