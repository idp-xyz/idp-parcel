# 13 pilot-governance：硬风险检测形态与证据存储连接器

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分与暂缓重新定性划出的产品策略，PG 一张）；第 1、2 项待核
Blocked by: 无
地盘：pilot-governance 领域与应用层（暂停的检测侧），证据存储连接器适配器。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-GOV-05`、`PAR-GOV-11` 行内「〔ADR-0146 拆分〕」点名的部分；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二。

## 做什么

1. **待核：紧急暂停硬风险检测条件的内置形态**（`PAR-GOV-05`「检测条件」）。「仅限不可逆、违法、跨客户或事实完整性风险」是产品约束；其中跨客户与事实完整性这类不看任何租户就能判的检测是方法，归产品；阈值、最小范围与自动执行身份的取值留租户。核暂停登记口之外有无检测侧。
2. **待核：证据存储连接器**（`PAR-GOV-11`）。S3 兼容对象存储的连接器形态与 `idp-parcel/pilot` 命名空间约定归产品；实例引用与访问责任留租户。
3. **回放的差异分类**（票 02 暂缓清单：[ADR-0124](../../../docs/adr/0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md) 决定六把「批量驱动、范围选择与差异分类」一并判为 W02 实例半边）。缺失 / 额外 / 不同 / 迟到 / 无法解释这组分类怎样判是方法，归产品；回放范围与数据集仍是租户取值（`PAR-GOV-01`）。

## 不做

- 不替租户定检测阈值或值守人员；不建任何 MinIO 实例。

## 完成判据

- 每项要么有执行器（带测试），要么记下已有执行器的证据；登记册对应行同步收短。
