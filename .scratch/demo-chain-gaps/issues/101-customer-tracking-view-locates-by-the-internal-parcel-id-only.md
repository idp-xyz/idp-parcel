# 101 客户追踪视图只认正式包裹号，客户系统经首方渠道拿不到这个号

Category: enhancement
Status: ready-for-agent——2026-10-19 分诊为 A（挡演示/上线），批次见 [triage-2026-10-19](../triage-2026-10-19.md)。此前 needs-triage——2026-10-19 Claude Code 子代理立（票 [96](./96-customer-tracking-view-has-no-first-party-customer-intake.md) 实现时发现）。
Blocked by: 无
地盘：VE 查阅口的定位（`internal/visibilityexception/adapters/http` 的 `CustomerTrackingViewIntake` 与查询键），或 PS 首方提交答复带回包裹号（`internal/parcelshipment/adapters/http`）；二选一或都做，看裁决。
出处：[UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)「输入和结果语义」查询标识一行（「包裹内部引用、授权客户参考号或版本化外部标识；标识只用于定位候选，不是访问凭证」）与主流程步骤 2；[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四、五。

## 现象（钉 `79ad6122`，只记 `S`）

`GET /customer-tracking-view?parcel=<…>` 只认正式包裹号（`DPCL-…`）：VE 客户视图库以（租户，客户账户，包裹）为键，包裹就是这个号。客户系统经首方渠道能拿到的标识只有自己的客户包裹参考（`customerParcelReference`）和委托号——`POST /shipment-requests` 答 `201 SUBMITTED` 时只带 `shipmentRequestId`，不带包裹号。首方 API 上没有一条路让客户系统得知 `DPCL-…`。票 96 的演示是用操作者渠道 `GET /tracking-projections` 读出包裹号再交给客户令牌查的；真实客户系统做不到这一步。

## 三类

- 机制：查询标识按 UC-VE-008 三种之一定位候选——授权客户参考号要在本客户账户名下解析到唯一包裹，零匹配与「别的账户名下有」同答 `VIEW_NOT_FOUND`（AT-VE-151 不可枚举），多匹配走 AT-VE-152 的冲突格；版本化外部标识同理，谱系归 PS。或者：PS 首方提交答复带回逐包裹的正式包裹号，客户系统自己记映射。
- 产品策略：首方 API 对外承诺哪一种查询标识（ADR-0140 契约的一部分）。
- 租户取值：无。

## 要裁的

- 两条路不互斥。答复带包裹号改的是 PS 的答复形状（ADR-0140 契约），按客户参考定位要 VE 读 PS 的标识映射（跨上下文读口，或 VE 在派生视图时把客户参考随视图存下）。哪条先做、VE 是否存客户参考，归 VE / PS owner。

## 完成判据

- 客户系统只凭首方 API 上拿得到的标识，就能查到本账户包裹的视图；别的账户的同名参考不泄露存在性。
- 演示动线改走客户自己的标识，不再经操作者渠道读包裹号。

## 规模

M
