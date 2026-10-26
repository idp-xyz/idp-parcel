# 06 首方客户命令五口的共用底座

Category: enhancement
Status: in-progress——2026-10-26 会话 `tops-20`（Claude Code）立并认领。
Blocked by: 无
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

五口共用的一层：`CustomerClientMinter` 加撤回、取消、修订、补充、索赔五个 Mint 方法骨架与 `CustomerCapability` 五词；`RequestKeyDerivation` 五个推导（命令前缀，取舍 A；含 `DeriveWithdrawalRequestKey`）；一份 access_identity 迁移一次登五词；五口规范化形状与形状版本（ADR-0139 决定八）；提交回执返回提交版本标识（取舍 E，若今天没有）；演示租户五词的授予、准入范围对照与权威区间种子；立 ADR-0310 记取舍 A–F。各口领域 Intake 不在本票（07–11）。

## 完成判据

- [ ] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [ ] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

M
