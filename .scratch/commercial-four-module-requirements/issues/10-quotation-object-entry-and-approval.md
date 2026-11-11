# 10 报价对象、录入与批准

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [09](./09-quotation-domain-language-and-use-case.md)
地盘：`internal/partycommercial`（领域、应用、仓储、HTTP）、`internal/partycommercial/adapters/parcelpricing`（新增消费侧适配器）、`internal/parcelpricing`（草稿只读口）、`migrations/party_commercial`、`cmd/parcel-api`、`cmd/parcel-commercial`、`apps/admin-web/src/pages/party`

对应要求：Quotation「报价创建」「审批」。

## 现象

取证钉 `11bfa1ffb`：没有报价对象。在 `internal`、`cmd`、`apps/admin-web/src` 的非测试代码里检索 `quotation` 没有业务命中。

## 要做的

照 ADR 与票 09 落文后的用例实现。这里只列切片的边。

1. **领域**：报价与报价版本；状态封闭；过期读时派生，不存。录入即待批准；批准前同一报价版本重新录入即替换，批准后内容固定。
2. **引用价格内容**：报价按方案标识、方案版本、规范化版本与内容摘要引用一份已校验的销售价卡草稿。`party-commercial` 经消费侧适配器读 `parcel-pricing` 的草稿只读口；草稿不在、不是销售方向、还没过校验，或摘要与调用方看到的不同，各答各的一格。
3. **批准**：与录入分立；两个身份都从操作者信封来。按租户的报价审批职责规则裁，没登记答未配置、不放行。规则的登记入口照 [ADR-0362](../../../docs/adr/0362-the-commercial-publication-approval-duty-rule-is-registered-through-the-controlled-commercial-cli.md) 的形状走受控命令行；管理台上的登记面不在本票。
4. **绑定内容**：批准带调用方看到的内容摘要，对不上答已变更，不批（做法同 [ADR-0314](../../../docs/adr/0314-pending-publication-drafts-are-readable-back-by-the-operator-and-approval-and-publication-are-bound-to-the-content-digest-the-caller-saw.md)）。
5. **持久化与读面**：一本报价册，只增不改的版本行；列表与按身份读回。
6. **管理台**：报价列表、详情与录入；录入时从草稿册挑一份已校验的销售价卡草稿，相对方从往来单位里挑。
7. **演示种子**：演示租户登一条报价审批职责规则、一位 `SYN-` 相对方与一份 `SYN-` 报价。

## 三类

对象、状态与各道门是机制。报价审批职责规则的取值、每份报价的相对方、有效期与条款是租户取值。

## 完成判据

- 录入、替换、批准各有用例；自己批自己、没有所需授予、规则没登记各答各的一格。
- 所引草稿被替换之后，批准答所引价格已变更。
- 过了有效期终点的报价读出来是已过期，库里的状态没有变。
- 领域包不依赖 HTTP 或 `pgx`；计价一侧没有任何代码认得报价。
- 演示种子灌入退 0；证据记 `S`。
