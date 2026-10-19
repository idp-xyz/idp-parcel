# 26 SA 消费发生项已形成信封，发起 BUY 评价（收 10 的 BUY 半）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：SA 新 `adapters/transportfulfillment/`、PC 范围读、新 inbox 消费者；触发册加费用项目（domain、ports、postgres、迁移 SA 新号）；`cmd/parcel-settlement-register`；`cmd/parcel-dispatch/assemble.go`(+_test)；种子 settlement BUY 行；ADR-0208（ADR-0164 加后继指针）、UC-SA-002、SA CONTEXT。
出处：[10](./10-demo-tenant-buy-and-sell-trigger-registers-are-empty.md)、[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「BUY 发起（接 08，收 10）」D08-1、D10-1，「跨票通则」X3、X6，「需要的 ADR」ADR-0208。

## 做什么

- SA 在 `cmd/parcel-dispatch` 消费 `transport-fulfillment.charge-occurrence.formed`，路由先登（生产方 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 后上线，方案 X3），经 SA 消费侧适配器按引用回读发生项（方案 D08-1）。
- 发生项各要素与供应商协议取自发生项本身；主要范围经 SA→PC 读口取协议版本的商业范围（方案 D08-1）。
- BUY 触发册每行加一格「费用项目」（租户取值）；未配置与未决分格照方案 D08-1、X6。
- 演示种子登 BUY 触发行与费用项目，取值照方案 D10-1。
- 起草 ADR-0208：部分停用 ADR-0164 决定一末句，范围照方案「需要的 ADR」；ADR-0164 Links 加后继指针。UC-SA-002、SA CONTEXT 补一句。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 路由先登（生产方未上线，ADR-0049 允许）。
- 无行或无费用项目答 `未配置` 并入账。
- 读不动登哨兵。
- 真库测试用手插的 TF 发生项行跑到 BUY 请求 `FIRED`。
- 端到端演示证据随 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)。

## 规模

M
