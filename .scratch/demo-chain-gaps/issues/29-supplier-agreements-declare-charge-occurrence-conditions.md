# 29 供应商协议声明收费发生条件

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)
地盘：PC `domain/publication_canonicalization_supplier_agreement.go`、`supplier_agreement.go`、发布用例、http 草稿载荷、postgres + 迁移 PC 新号；TF 登记时判条件；种子 `commercial/publish-batch.json` v2；ADR-0209、PC CONTEXT。
出处：[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「07 运输收费发生项」D07-1，「需要的 ADR」ADR-0209。

## 做什么

- PC 供应商协议正文加「收费发生条件」文法：逐原因声明计费与否，加数量形（方案 D07-1）；发布成新版本，旧版本不可变。
- TF 登记时按协议快照判条件，取代 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的登记者断言；没声明与声明不计费的答法照方案「需要的 ADR」ADR-0209。
- 演示干线协议以 v2 重新发布（方案 D07-1）。
- 起草 ADR-0209；PC CONTEXT 补收费发生条件。

## 完成判据

- 新摘要文法，v1 不变。
- 没声明答 `未配置`。
- 不计费的原因有业务格。
- 数量形定出数量与单位。
- 真库测试。
- 演示：用 v2 干线协议登实际履约，不再需要断言。

## 规模

L
