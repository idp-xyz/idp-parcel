# 08 补充口经首方渠道接通

Category: enhancement
Status: in-progress——2026-10-26 会话 `tops-20` 认领（06 已进 main），分支 `claude-fpa08`。此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Blocked by: 06
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

首方 `IntakeSupplement`、基准版本取自载荷带回的提交版本标识（取舍 E）、`endpoints_customer_supplement.go` 换真 Intake；他账户答找不到。

## 完成判据

- [ ] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [ ] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

M
