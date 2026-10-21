# 文档自洽修订（2026-10-21 起）

用户 2026-10-21 原话：「需要修改文档，确保自洽，正确」。本目录是这一轮的单一权威：口径、分组、进度都记在这里，审计报告与修订票落在本目录下。

## 为什么要做

2026-10-21 实测于 main `4384a85a`：`docs/adr/` 228 份，其中 91 份的 Status 行挂着被后来 ADR 部分停用的指针，ADR-0151 一份被 11 份后来的 ADR 改过；43 份是会话代决、越权风险点待 owner 复核。某个话题的现行规则散在一份底稿、几份部分停用与若干 Status 行注记里，作者读漏一处就写出矛盾——近两天的评审里，「文档与代码相反」「同一场景两套 AT 结果」「被停的句子仍在 CONTEXT 里当现行规则」各出现过数次。

## 目标

1. **CONTEXT 是现行规则的唯一出处**：每个上下文的 `CONTEXT.md` 写出今天生效的语言、不变量与生命周期，已被 ADR 部分停用的句子不再以现行口吻留在 CONTEXT 里；ADR 只作取舍的历史与理由，CONTEXT 引用它。
2. **UC 与 CONTEXT、ADR 一致**：同一场景只有一套结果；AT 行不与现行规则相反。
3. **ADR 停用链完整**：停用方的「对已接受记录的作用」与被停方的 Status 行前向指针两头都在、所引原句逐字存在；同一次停用不写两遍（infra-speed/07 并入本轮）；两份已接受 ADR 互相矛盾而无停用关系的，列出来裁。
4. **文档与代码一致**：文档写的结果码、状态名、端点、判法与代码一致。
5. **引用规范**：跨文件引用不用行号、不用「别处有几个」计数、不用「上一条」位置引用（AGENTS.md「改文档」）；链接全部能解析。
6. **GLOSSARY 单一权威**：跨上下文术语只在 GLOSSARY 定义一次，CONTEXT 引用；同一词在两处定义不同的列出来。

## 审计口径（每条发现必须写全）

- **类别**：上面 1–6 中的哪一条。
- **位置**：文件路径 + 引文（不用行号）。
- **证据**：与之相抵的另一处（文件 + 引文，或代码符号）。
- **归类**（决定怎么修）：
  - **A 文档错**：代码与已接受的现行决定一致、文档写错或过时 → 只改文档。
  - **B 要裁**：两份已接受的记录真矛盾、谁也没停用谁，或 CONTEXT 与 ADR 各执一词且代码两头都不完全对 → 立新 ADR 代决（用户授权「你作为业务和系统专家自决」），写越权风险点。
  - **C 代码错**：文档是对的现行规则、代码与它相反 → 不在本轮改代码，立代码票。
- **严重度**：高（会让作者照着写出错代码或让读者误判业务结果）/ 中 / 低（措辞、链接）。
- **建议修法**：一两句。

只读审计，不改仓库文件。已接受 ADR 的正文不改（AGENTS.md「不改写已接受 ADR 历史」），改的只是 Status 行、Links、索引注记与 CONTEXT、UC、GLOSSARY 等现行文档。

## 分组

| 组 | 范围 | 审计报告 | 修订 | 依赖（修订开工前须先进 main） |
|---|---|---|---|---|
| TF | transport-fulfillment | `audit/tf.md` | 待立 | 在途票 88、57、146+94 |
| CC | customs-compliance | `audit/cc.md` | 待立 | 无 |
| VE | visibility-exception | `audit/ve.md` | 待立 | 无 |
| SA+CR | settlement-accounting、collection-remittance | `audit/sa-cr.md` | 待立 | 在途票 57+15（改 SA） |
| PS | parcel-shipment | `audit/ps.md` | 待立 | 无 |
| PC+PP | party-commercial、parcel-pricing | `audit/pc-pp.md` | 待立 | 在途票 88（改 ADR-0233 Status 行） |
| NR+NO | network-routing、node-operations | `audit/nr-no.md` | 待立 | 无 |
| 横切 | CONTEXT-MAP、GLOSSARY、SCENARIOS、`docs/product/`、`docs/adr/` 停用链与 infra-speed/07、`docs/agents/` | `audit/cross.md` | 待立 | 在途票 146+94（改 GLOSSARY、CONTEXT-MAP） |

## 次序

审计并行、只读。修订按组各开一张票、各一个 worktree，组与组之间文件不相交的可并行；同一组内不并行。依赖列里的在途票进 main 之后，那一组才开修订。票 126（TF 运营决定记提交操作者）排在 TF 组修订之后。

## 审计结果（2026-10-21，钉 `6b0074d0`）

| 组 | 报告 | 条数 | 高 | A 文档错 | B 要裁 | C 代码错 |
|---|---|---|---|---|---|---|
| TF | [audit/tf.md](./audit/tf.md) | 41 | 3 | 32 | 5 | 4 |
| CC | [audit/cc.md](./audit/cc.md) | 24 | 3 | 18 | 2 | 4 |
| VE | [audit/ve.md](./audit/ve.md) | 22 | 2 | 9 | 3 | 10 |
| SA+CR | [audit/sa-cr.md](./audit/sa-cr.md) | 26 | 5 | 17 | 4 | 5 |
| PS | [audit/ps.md](./audit/ps.md)（取证：[ps-code](./audit/ps-code.md)、[ps-adr](./audit/ps-adr.md)、[ps-uc](./audit/ps-uc.md)） | 51 | 6 | 40 | 6 | 5 |
| PC+PP | [audit/pc-pp.md](./audit/pc-pp.md) | 24 | 5 | 20 | 2 | 2 |
| NR+NO | [audit/nr-no.md](./audit/nr-no.md) | 26 | 3 | 14 | 5 | 7 |
| 横切 | [audit/cross.md](./audit/cross.md) | 151 | 9 | 128 | 23 | 0 |
| **合计** | | **365** | **36** | **278** | **50** | **37** |

审计读的是 `6b0074d0`；其后进 main 的票（88、145、146+94、150、57+15 等）可能已改掉其中一些条目，修订开工时先按 main 现状逐条复核。

横切报告自注一处覆盖缺口：治理类 ADR（0022、0055、0072、0085、0100、0139、0140、0149、0150 一串）只逐对核了停用指针，没做话题级通读（两份都还现行的说法有没有互相打架）；修订 X1 开工时补上。
