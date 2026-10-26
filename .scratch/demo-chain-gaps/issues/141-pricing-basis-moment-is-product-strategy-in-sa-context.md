# 141 SA CONTEXT 与 GLOSSARY 仍要求价卡声明计价基准时点

Category: bug
Status: in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-dcgdocs tip `ac6f096b1`（本票无内容提交——两句已由 docs-consistency/09 `4c9b2c4cc`、10a `e35c903c5` 改完，只有票面那一笔；见文末「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，文档票同批（128、134、137、138、141、144），分支 `claude-dcgdocs`（worktree `idp-parcel-dcgdocs`）。此前 ready-for-agent——2026-10-20 代决复核立（文档），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R2；ADR-0190 越权风险点 8（ADR-0179 同此）

## 现象与修法

现象、证据与修法的完整写法以复核报告 R2 一条为准（单一权威），此处只记要点：

代决「是」：计价基准时点为产品策略、由内置时点给出；改 SA CONTEXT「每类……可执行价卡必须声明自己的计价基准时点」与 GLOSSARY 对应定义。

## 文档自洽复核（2026-10-21）

SA CONTEXT 的两处（Language「计价基准时点」与 Rules「计价、组合与精度」里计价基准时点那一条）已由 [docs-consistency/09](../../docs-consistency/issues/09-sa-cr-docs.md) 照本票改为产品内置的时点、由发起评价的那一路与评价目的给出、价卡与商业价格政策不另声明（审计 [sa-cr.md](../../docs-consistency/audit/sa-cr.md) M1）。GLOSSARY「计价基准时点」那一半交 [docs-consistency/10](../../docs-consistency/issues/10-cross-cutting-docs.md)（09 交横切 X1）；10 改完后本票即可收口。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcgdocs`，起点 `d3aa57fc3`）

### 提交

本票没有内容提交：两处改动在本批开工前已进 main；票面「文档自洽复核（2026-10-21）」一节等的 docs-consistency/10 已经完成。只有票面完成记录那一笔。

### 核对结果

- **SA CONTEXT**：Language「计价基准时点」与 Rules「计价、组合与精度」里那一条，都已写成产品内置的时点，由发起评价的那一路与评价目的给出，商业价格政策与可执行价卡不另声明（docs-consistency/09，`4c9b2c4cc`）。本票没动 SA CONTEXT，后段「客户赔付与外部追偿」那一块（code/12 的在途地盘）也没碰。
- **GLOSSARY**：「计价基准时点」词条已同句，所有者写作「内置时点的判断方法归产品」（docs-consistency/10a，`e35c903c5`）。
- 在 `docs/` 下（不含 `adr/` 与 `archive/`）grep 同时含「计价基准时点」与「声明」的句子：没有一句还要求价卡或商业价格政策声明它。参数登记册 `PAR-SET-02`、`PAR-SET-03` 两行也已写「不向价卡取证」。
- **ADR-0190**：文末「复核记录（2026-10-20…）」已有 R2 一条（「计价基准时点认定为产品策略、由内置时点给出；SA CONTEXT 与 GLOSSARY 的对应句随之改」），复核会话 `7270c63df` 写入。
- **ADR-0179**：它的越权风险点里没有这一格，决定三取的时点是「本次控制回显的判断时点，即规则包时点格采用的形态」，本身就是内置时点，与 R2 一致；没有条款被停用。

### 与报告相抵处

无。报告 R2 要改的两句，在本票开工前都已改完。

### 代决

1. ADR-0190 与 ADR-0179 的 Status、Links 都不加指针。按 adr/README「一件事只写一处」，「由 ADR-… 回答」只用于后来的记录作答；R2 出自复核报告，ADR-0190 已在文末复核记录里写明，2026-10-20 各份复核记录也都不往 Status 行抄。ADR-0179 既没有对应的风险点，也没有被停的条款。
2. 有一处没改，留给评审判断：UC-SA-002「输入语义契约」里「客户商业依据」一行，把「计价基准时点」列在 `party-commercial` 交的各项里；生产参数表 `PAR-SET-02` 一行也列着「计价基准」。按 ADR-0179 决定三，受理前估价的时点取自规则包时点格，所以这一行未必错，但读起来像是商业侧要声明这个时点。这两行不在 R2「改 SA CONTEXT 与 GLOSSARY 的对应句」之内，本票没动。

### 自验

无内容改动。`go run ./tools/adr-index -check` 退 0。
