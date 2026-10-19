# 管理台的委托提交页没有首方客户端凭据：提交答 401

Category: enhancement
Status: needs-triage——2026-10-19 票 [00](./00-first-party-customer-intake.md) 立。
Blocked by: 无
出处：[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定三「不给操作者面开首方客户渠道……『操作者代客提交』是另一个用例，需要它自己的授权与来源标记」与决定六「不做客户人员的浏览器登录（authorization code + PKCE）那一族」；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三。

## 现状（票 00 落地后）

- `apps/admin-web/src/pages/shipment-request/api.ts` 的提交仍 `POST /shipment-requests`，带的是操作者令牌。提交口只认首方客户族的令牌，操作者令牌受众不对，答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`。
- 此前这一页靠隔离写开关的注入式 Intake 落单；那条路随首方 Intake 撤下（ADR-0150 决定三）。演示提交改由 `scripts/demo-seeds/submit-one-shipment.sh` 以合成货主系统的令牌走首方口。

## 待裁

1. 管理台要不要保留「提交委托」：保留就是「操作者代客提交」用例——要它自己的授权（操作者册上的能力面）、来源标记（代客的来源维）与端点，不复用首方口（ADR-0139 决定三）。
2. 不保留：页面改为说明提交走客户系统，或只留草案预览；前端切片照 workflow.md「前端切片」一人在 main 上直接做。

## 完成判据

- 页面不再打出一枪必然 401 的提交；所选那一支的用例与端点有测试。
