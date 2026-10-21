# 01 ADR 停用链：两头都在、只写一遍、规则不再长出重复

Category: enhancement
Status: in-progress——2026-10-21 认领，分支 claude-dc01
Blocked by: 无（票 demo-chain-gaps/57 动的 ADR-0200、0204、0207、0208、0229 留到 57 进 main 后补一笔，不挡本票其余部分）
出处：[audit/cross.md](../audit/cross.md) 的停用链部分（X 前缀条目、附表 A、附表 B）与各组报告里落在 ADR Status 行与 Links 上的条目；并入 [infra-speed/07](../../infra-speed/issues/07-adr-status-lines-carry-a-suspension-twice.md)。

## 要什么

1. **补缺失的前向指针**：附表 A 结论里列出的、后来 ADR 用「改为、接上、扩成、按口解」之类词改掉前一份条款却没写停用的那约 15 处，以及 X-01（ADR-0181 ← 0182）、B2-01（ADR-0047 ← 0122、0127）等报告点名的缺口：被停方 Status 行补前向指针、索引注记写被停条款原句（逐字）、Links 双向；停用方正文若缺「对已接受记录的作用」，在它的 Status 行与 Links 写明（已接受 ADR 正文不改）。
2. **合并 Status 行里写两遍的停用**（infra-speed/07 的「要什么」与「不变式」照办）：附表 B 的来向两写、去向两写、接受出处两写、头部「Partially suspended by / Amended by」第三份写法，逐份合成一份、留在索引注记；合并前后两版并排贴进完成记录。
3. **改 `docs/adr/README.md` 顶部规则**（生成段之外的手写部分），让新 ADR 的停用只写一处，不再长出重复（X-02：2026-10-20、21 新接受的几次停用照样两写）；同步改 `tools/adr-index` 的 `-check`（若规则可机械校验，例如 Status 行标记前段不得再写被停条款原句），并补工具用例。
4. **补治理类 ADR 的话题级通读**：横切报告自注的覆盖缺口——0022、0055、0072、0085、0100、0139、0140、0149、0150 一串只逐对核了停用指针，没查两份都还现行的说法有没有互相打架。通读后：互相打架的列成 B 类，交给本票末尾「交后续」一节（不在本票裁），没有的写「已通读，无」。

## 不变式

- 只动 ADR 的 Status 行、索引注记与 Links，以及 `docs/adr/README.md` 顶部手写规则、`tools/adr-index`；ADR 正文一字不改；README 生成段只经 `go run ./tools/adr-index` 重生成。
- 合并不丢事实。
- 不碰 CONTEXT、UC、GLOSSARY、CONTEXT-MAP（那是 03–10 的地盘）。

## 完成判据

- 附表 A 每一对两头都在、原句逐字；附表 B 每一份只剩一处写法。
- `go run ./tools/adr-index -check` 退 0；README 重生成前后逐条对照，差异只许是合并与补指针带来的那几行。
- 票 57 进 main 后，那五份 ADR 补完同样的处理（可以是本票后续一笔）。
