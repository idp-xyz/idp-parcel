# PS 生产归属目录：隔离写开关退场的前件

Category: enhancement
Status: needs-triage——2026-10-19 票 [00](./00-first-party-customer-intake.md) 立。
Blocked by: 无
出处：[ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md) 决定三「目录本身归 PS 生产归属那一票」与越权风险点「PS 生产归属的能力与事实类型词不在本记录内」；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一、三；[ADR-0141](../../../docs/adr/0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md) 决定四「不许写一个直接返回已确定的假目录」。

## 现状（票 00 落地后）

- 提交口挂首方客户渠道 Intake，隔离写开关不再放行任何命令面。写开关只剩 ADR-0091 决定二的「墙二」：给提交编排的生产归属适配器注入合成治理坐标（`IsolatedGovernanceScopeDirectory`）与合成自身权威串。
- 生产形态下 `buildProductionOwnership` 的 `Directory` 留空，`governanceScope` 先于读册早退，任何租户经首方口提交都停在 `OWNERSHIP_UNRESOLVED`。演示要走到`已提交`仍得设写开关。
- 首方 Intake 给编排的拟受理范围是（租户、客户账户）快照（`customerAccountAdmissionScope`，票 00 代决）；目录还没有一处把它换成治理坐标。

## 做什么

1. 立 PS 的生产归属目录（`pspilot.GovernanceScopeDirectory` 的生产实现），读册换坐标、不代拟：候选是读接入身份的准入范围对照册（ADR-0180 决定一、二）按（租户、能力、事实类型）取对象范围，试点范围版本由谁给要一并裁。
2. 定 PS 委托受理在治理登记册上的能力词与事实类型词（ADR-0180 越权风险点「PS 生产归属的能力与事实类型词不在本记录内」；决定四那一类，产品策略）。
3. 演示种子按定下的词另登区间与对照，`SYN-` 合成坐标随写开关退场。
4. 目录接上之后，写开关在 ADR-0150 决定三说的「最后一口」那一笔与读开关一起删（读开关归 operator-channel/53）；`isolatedWriteAdmission`、`IsolatedGovernanceScopeDirectory` 与两开关一致性校验同笔删。

## 完成判据

- 生产形态（不设写开关）下，演示租户经首方口提交一单答 `SUBMITTED`；对照或区间没登答 `OWNERSHIP_UNRESOLVED`（不代拟）。
- 证据只记 `S`。
