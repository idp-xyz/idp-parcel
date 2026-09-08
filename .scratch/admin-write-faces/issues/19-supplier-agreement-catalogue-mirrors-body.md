# 19 供应商协议目录读面镜像后端已透的正文：`contentRegistered` 显式布尔 + 0021 各键上列，「没有正文表可读」那几句退场

Category: enhancement
Status: ready-for-agent——形状已裁清（后端读口已透的键逐一镜像、显式布尔照合同页同款、行转写抽纯函数；本票无待裁问题）。由票 [11](./11-supplier-agreement-form.md) 的非作者评审 Spec 非阻断 (1) 拆出（MCP-6 2026-09-08 15:1x，通道 1 指派；锚 main `83970dcf`）。不是伞票 07 的子票——07 管的是各册的发布主路径，本票是读面
Blocked by: 无（11 已进 main `83970dcf`，写签在场）

## 缺什么

后端 `GET /commercial-supplier-agreements` 自票 party-commercial-context-gaps/03 起逐字段透出版本壳与 0021 正文：
`internal/partycommercial/adapters/http/query_commercial_relations.go` 的 `supplierAgreementBody` 带 `contentRegistered`
显式布尔，为真时在场 `supplier`、`legalEntity`、`purchasePlan`、`agreementScope`、`agreementEffectiveStartsAt`、
`agreementEffectiveEndsAt`（可缺）、`registeredAt`；方向**不透出**——那一处注释的原话是「领域恒为 BUY，库上不成列，
转写一个常量等于为同一件事立第二个口径」。

前端没跟上：`apps/admin-web/src/pages/party/api.ts` 的 `SupplierAgreementRecord` 止于 `publishedAt`，头注写着
「服务端没有正文表可读(见后端 supplierAgreementBody 注释),因此这里也没有对应字段」；`SupplierAgreementsPage.tsx`
的列注（「今天没有对应的正文表可读」）与页面描述（「尚无正文册可读，不上列」）说的是同一句——这几句在票 pc-gaps/03
落地那天就过期了，只是一直没人碰这页。票 11 落了写签之后它变得可见：操作者从「发布协议版本」发出去的正文，在同页
「协议目录」里**版本行可见、正文列不可见**（票 11 评审 Spec 非阻断 (1) 原话）。

## 要做的

- `party/api.ts`：`SupplierAgreementRecord` 镜像后端各键——`contentRegistered: boolean` 必在，正文各键可缺（与 Go 侧
  `omitempty` 同形）；过期头注改掉，判据照本文件 `CustomerContractRecord` 上那段 contentRegistered 注释写：**没登记正文**与
  **登记了正文但某键为空**都可能表现为键缺席，恢复动作相反，页面必须先看布尔。
- `SupplierAgreementsPage.tsx`：目录加正文列——正文（已登记 / 未登记）、供应商、责任法人、采购方案引用、协议范围、协议
  区间、登记时间。壳在正文缺是合法状态（壳可先入册、正文随发布登记），照合同页「未登记」如实显示；布尔为真而键缺是
  响应不合契约，点名而不显示成空（判据同 `policy-rows.ts` 里接受前财务控制策略册那一支）。列注与页面描述那两句过期话
  退场，换成读面此刻真实的说明。
- 行转写抽成纯函数 + node:test（写法照 `party/policy-rows.ts`，钉：只有壳的行、带正文的行、布尔为真键缺的坏行三例）。

## 边界

不动后端读口、不动 0021、不加端点、不加查询参数（过滤只在已取回数据上做，README「列表页上列通则」）；方向不上列——
后端刻意不透，前端转写常量等于第二个口径；写签（票 11 的 `SupplierAgreementPublicationForm`）不动；读面 403 那堵墙
照旧由 `catalogueViewState` 答，本票不碰。

## 完成判据

供应商协议页「协议目录」签对带正文的行显出正文各键、对只有壳的行显「未登记」；`apps/admin-web/src` 下「没有正文表可读」
「尚无正文册可读」一类的话对供应商协议零命中（用 ASCII 之外的针要显式 UTF-8 读，见 workflow.md 本机环境）；tsc /
run-tests 绿。
