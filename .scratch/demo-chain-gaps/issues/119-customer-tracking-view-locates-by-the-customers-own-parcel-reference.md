# 119 客户追踪视图不能按客户自己的包裹参考直接定位：首方契约只承诺包裹内部标识这一种查询标识

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-19 票 [101](./101-customer-tracking-view-locates-by-the-internal-parcel-id-only.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。协调方代决分诊为 C（暂不做：增强，挡不住演示与上线，要 VE、PS owner 裁定），分组以 [triage-2026-10-19](../triage-2026-10-19.md) 为准，票面状态照该表的约定不动。出自 [ADR-0223](../../../docs/adr/0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md) 决定四「不按客户包裹参考或版本化外部标识直接定位客户视图」一条。
Blocked by: 用户决定（VE、PS owner：VE 是否随视图存客户参考，或 PS 开一条跨上下文的标识映射读口；客户包裹参考在一个客户账户里是否唯一）
地盘：VE 查阅口的定位（`internal/visibilityexception/adapters/http` 的 `CustomerTrackingViewIntake` 与查询键、客户视图库）；若走跨上下文读口，另及 PS 外部标识关联册的读面（`internal/parcelshipment/adapters/postgres/external_identifier.go`）。

## 现象（钉票 101 的分支，只记 `S`）

- 票 101 起，首方提交答复逐件带回包裹内部标识（`parcels[].parcelId`），配客户包裹参考（ADR-0223 决定一）。客户系统自己记映射，就能凭这个号查 `GET /customer-tracking-view?parcel=<号>`。
- [UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md) 查询标识一行列了包裹内部引用、授权客户参考号与版本化外部标识。首方契约今天只承诺包裹内部引用。
- 客户系统若没留住提交答复（例如由别的系统提交、或补录历史件），只拿着自己的客户包裹参考，就查不到视图。它只能凭同一接入请求标识原样重发拿回答复；换过请求标识的，没有路。

## 三类

- 机制：授权客户参考号要在本客户账户名下解析到唯一包裹。零匹配与「别的账户名下有」同答 `VIEW_NOT_FOUND`（AT-VE-151 不可枚举），多匹配走 AT-VE-152 的冲突格；版本化外部标识同理，谱系归 PS。
- 产品策略：首方契约对外再承诺哪一种查询标识（ADR-0140 契约的一部分），以及查询串怎么区分标识种类（例如另一格 `customerParcelReference=`，而不是让 `parcel=` 猜）。
- 租户取值：无。

## 要裁的

- VE 是否在派生视图时把客户参考随视图存下（VE 自持一份，派生时从 PS 的外部标识关联取），还是查阅时跨上下文读 PS 的标识映射。前者多一份要随关联更正而更正的副本；后者查阅口多一条依赖。
- 客户包裹参考在一个客户账户里是否唯一。PS 外部标识关联按（分配方，适用范围，号码）认，一个号可能对上多件（客户复用了件号），这时答冲突还是列候选。

## 完成判据

- 客户系统只凭自己的客户包裹参考，就能查到本账户那一件的视图；别的账户的同名参考不泄露存在性，多匹配有明确的答复格。
- UC-VE-008 与首方契约写明授权客户参考号这种查询标识；ADR-0223 决定三加前向指针。

## 规模

M
