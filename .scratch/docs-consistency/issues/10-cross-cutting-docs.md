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

## 不变式

- 已接受 ADR 正文不改；不改代码。
- AGENTS.md 是人与 Agent 共用的入口，改它照 `/writing-for-agents` 的写法，不删现行规则、不重述 docs/ 的权威内容。
