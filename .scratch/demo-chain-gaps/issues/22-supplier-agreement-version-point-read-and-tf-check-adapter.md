# 22 供应商协议版本点读口与 TF 协议核对适配器

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：PC `ports/ports.go`（加窄口）、`adapters/postgres/supplier_agreement.go`；TF 新 `ports/supplier_agreement.go`、新 `adapters/partycommercial/supplier_agreement_source.go`。
出处：[02](./02-transport-commission-has-no-command-face.md)、[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「02 委托、订舱、承运答复」D02-4、D02-5，「07 运输收费发生项」D07-2，「跨票通则」X4。

## 做什么

- PC 新开一个窄读口，按（租户、协议对象、版本标签）答 Effective 版本壳加正文；不借管理台列表口（方案 D02-5）。
- TF 消费侧适配器把它译成核对结论与采购责任法人，判法照方案 D02-4；新端口另立文件，不改 TF `ports/ports.go`（方案 X4）。
- 本票只出读口与适配器，[23](./23-transport-commission-submission-face.md) 的委托核对与 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的采购上下文核对共用它（方案 D07-2）。

## 完成判据

- 按（租户、对象、版本标签）答版本壳与正文。
- 适配器译成：在册 / 未生效 / 已终止 / 服务方不符 / 法人。
- 真库测试覆盖 SYN 两份协议。
- 不动 cmd。

## 规模

S
