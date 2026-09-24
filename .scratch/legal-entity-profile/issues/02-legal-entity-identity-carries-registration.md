# 02 责任法人身份登记加注册国家 / 地区与终身注册号

Category: enhancement
Status: ready-for-agent
Blocked by: 01
地盘：party-commercial 责任法人身份登记的领域、应用、postgres 与 http 适配器（含 `query_party_identities.go` 的 `groupLegalEntityBody`），`migrations/` 下
party-commercial 模块的新迁移，演示种子。
出处：[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定一、二；CONTEXT Rules 同句。

## 做什么

1. 身份登记加注册国家 / 地区与终身注册号（可多个，每个带类型）；新登记缺一拒登，号经 01 的校验（身份层）。
2. **不作变更，录错走更正**（决定二）：修订若改了这两格，必须是携带更正依据的内容更正修订；领域上没有「改号」这一种修订。
3. 既有修订不改写：迁移只加格，历史行的两格为空；自本票起的新登记与新修订必须带两格。演示种子补合成值。
4. 读口答复加两格；登记失败的理由散文照既有 `writeProblemWithDetail` 的用法交出。

## 不做

- 不建法人资料（归 03）；不动管理台（归 04）。

## 完成判据

- 真库用例：缺国家、缺号、号不属该国身份层类型、格式不符各拒一条；更正修订改号须带依据，不带即拒；历史修订读回时两格为空且不报错。
- 端点用例：答复带两格。
