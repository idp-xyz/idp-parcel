# 10 横切文档：GLOSSARY、CONTEXT-MAP、SCENARIOS、产品文档、协作文档

Category: enhancement
Status: ready-for-agent——2026-10-21 协调方立（文档自洽修订最后一张，见 [README](../README.md)「修订执行」）
Blocked by: 03、04、05、06、07、08、09 进 main（它们的「交横切」一节是本票的输入）
出处：[audit/cross.md](../audit/cross.md) 里 GLOSSARY（GL）、CONTEXT-MAP（CM）、SCENARIOS（SC）、产品基线诸件（PB）、试点三件（PL）、docs/agents 与 AGENTS.md（AG）各条，以及其中 B 类；03–09 各票的「交横切」一节；[01](./01-adr-suspension-chains.md)「交后续」交给本票的条目。

## 要什么

1. 按 main 现状逐条复核（03–09 进 main 后的现状）。
2. A 类改文档：GLOSSARY 单一权威（跨上下文术语只定义一次，各 CONTEXT 引用）；CONTEXT-MAP 的上下游与各 CONTEXT「拥有/不拥有」一致；SCENARIOS 不与现行规则相反；`docs/product/` 与 ADR 现行决定一致；`docs/agents/`、AGENTS.md 与现行做法一致（含 AG·H2：进 main 的步骤不能只写在单会话不读的地方）；20 条断链修好。
3. B 类代决立新 ADR（号段 0274–0278），裁完相关文档随之改；要改代码的在 [code/](../code/) 补票。
4. 03–09 交来的每一条都落实或写明不落的理由。

5. **跨组约定：CONTEXT 写成现行、代码尚未到的规则怎么让读者知道**（出自 05 的隔离评审）：不在 CONTEXT 的单条规则上挂票号（票号随票关闭而过时，也违背「只写一处」）。每份 `docs/domain/*/CONTEXT.md` 头部只放一句不含票号、各组同形的固定指针，例如「规则与实现的差距见各 UC『给开发的交接』」；各 UC 的「给开发的交接」未实现表与 AT 行的「未实现」注记由 03–09 各自维护。

## 不变式

- 已接受 ADR 正文不改；不改代码。
- AGENTS.md 是人与 Agent 共用的入口，改它照 `/writing-for-agents` 的写法，不删现行规则、不重述 docs/ 的权威内容。

## 分工（2026-10-21，协调方）

审计 cross.md 151 条加 03–09 交来的横切条目，按文件不相交拆两个作者、两条分支，先后或并行进 main：

| 半边 | 文件范围 | 新 ADR 号段 | 代码票号段 | 分支 |
|---|---|---|---|---|
| 10a 领域语言 | `docs/domain/GLOSSARY.md`、`docs/domain/CONTEXT-MAP.md`、`docs/domain/SCENARIOS.md`、各 `docs/domain/*/CONTEXT.md` 头部固定指针（及交来的 CONTEXT 级改动）、`docs/application/README.md`、新立 `docs/application/collection-remittance/UC-CR-001`（09 的 X5：迁 CR CONTEXT 交接表）、别组 UC 的交来改动（如 07 的 X11 指 PS 文档认`适用冲突`） | 0274–0276 | code/54–56 | claude-dc10a |
| 10b 产品与协作 | `docs/product/`（含参数登记册、PILOT-*、基线诸件）、`docs/design/`、`docs/agents/`、`AGENTS.md`、`docs/README.md`、已接受 ADR 的 Status 注记与 Links（交来的引用更正，如 08 的 X6：ADR-0233 越权风险点 1；07 的 X9：ADR-0146「第三条判据」） | 0277–0278 | code/57–59 | claude-dc10b |

两半边都要的条目（例如一处 GLOSSARY 改动牵动产品文档一句）由 10a 改领域语言、10b 照新词改产品文档；10b 开工时以 10a 已进 main 的词为准，若 10a 未进 main，就在返回里列出待对齐的句子。
