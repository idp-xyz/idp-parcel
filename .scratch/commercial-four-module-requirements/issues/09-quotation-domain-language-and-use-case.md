# 09 报价的领域语言与用例落文

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [08](./08-adr-draft-quotation-review-numbering-and-acceptance.md)（ADR 接受之后才有规则可写）
地盘：`docs/domain/party-commercial/CONTEXT.md`、`docs/domain/CONTEXT-MAP.md`、`docs/domain/GLOSSARY.md`、`docs/application/party-commercial/`、`docs/application/README.md`、`docs/product/PILOT-PARAMETER-REGISTER.md`

对应要求：Quotation 一行的前置。本票只动文档，是改现行规则的文档票，进 main 前要一份非作者评审。

## 要做的

照已接受的那篇 ADR 写，不在这里另作决定。

1. **`party-commercial` 的 `CONTEXT.md`**
   - Language：报价、报价版本、价格生效申请。写明报价不是商业版本、不是价格评价、不是逐票价格承诺。
   - Rules：状态封闭与读时派生的过期、内容固定在批准那一刻、所引价卡草稿摘要变了即不放行、批准按租户的报价审批职责规则、发出不是送达、接受的两个入口与门、价格生效申请不自己发布。
   - Lifecycles：报价一节。
   - Boundaries：拥有报价；不拥有所引的价卡草稿，不拥有价卡与合同各自的批准发布。
   - 规则与实现的差距不挂在规则句上，写进新用例的「给开发的交接」；`CONTEXT.md` 头部那句固定指针已经在。
2. **`CONTEXT-MAP.md`**：`party-commercial` 的「拥有」加报价；关系约束加 `parcel-pricing → party-commercial`（价卡草稿读口）一条；关系图加一条边。
3. **`GLOSSARY.md`**：加「报价」，所有者与避免使用的词写清。
4. **新用例**：`docs/application/party-commercial/` 下立一份报价用例，号取该目录的下一个。写输入、结果代数（按恢复动作分格）、失败边界、一致性与幂等、验收示例与 AT 号、「给开发的交接」。`docs/application/README.md` 登这份用例。
5. **参数登记册**：加租户取值「报价审批职责规则」一行，号取下一个；状态写待提供。
6. **未确认的业务选择**记成 `BD-*`，按三类归好：谁可以代录接受、有效期终点的业务时区、报价批准要不要看毛利。

## 完成判据

- 上面各处写成现行规则，句子里不挂票号。
- 新用例的每个结果都说得出调用方下一步做什么；AT 号在盘点里有行（盘点由推送方重算）。
- 非作者评审无阻断；评审记录写明钉的 SHA。
- 票 10 至 13 的「Blocked by」随之解除。
