# 09 报价的领域语言与用例落文

Category: enhancement
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: 票 [08](./08-adr-draft-quotation-review-numbering-and-acceptance.md)（ADR 接受之后才有规则可写）
地盘：`docs/domain/party-commercial/CONTEXT.md`、`docs/domain/parcel-pricing/CONTEXT.md`、`docs/domain/CONTEXT-MAP.md`、`docs/domain/GLOSSARY.md`、`docs/application/party-commercial/`、`docs/application/parcel-pricing/`、`docs/application/README.md`、`docs/product/PILOT-PARAMETER-REGISTER.md`

对应要求：Quotation 一行的前置。本票只动文档，是改现行规则的文档票，进 main 前要一份非作者评审。

## 要做的

照已接受的那篇 ADR 写，不在这里另作决定。下面按草案第二稿列，ADR 接受时有出入的以 ADR 为准。

1. **`party-commercial` 的 `CONTEXT.md`**
   - Language：报价（一条发出记录）、报价答复。写明报价不是商业版本、不是价格评价、不是逐票价格承诺，也没有自己的批准。
   - Rules：只有内容已固定的价格内容可以发出、发出不是送达、答复的门、状态全部读时派生、生效申请落在现有的发布上。
   - Lifecycles：报价一节，写明各格都是派生的结论。
   - Boundaries：拥有发出与答复；不拥有所引的价格内容，不拥有价卡的批准与发布。
   - 规则与实现的差距不挂在规则句上，写进新用例的「给开发的交接」；`CONTEXT.md` 头部那句固定指针已经在。
2. **`parcel-pricing` 的 `CONTEXT.md`**：Lifecycles「定价方案与价表版本」加已批准 → 已撤下；`UC-PP-001`「价卡导入」一节随之补。
3. **`CONTEXT-MAP.md`**：`party-commercial` 的「拥有」加报价；关系约束加 `parcel-pricing → party-commercial`（价卡草稿读口）一条；关系图加一条边。
4. **`GLOSSARY.md`**：加「报价」，所有者与避免使用的词写清。
5. **新用例**：`docs/application/party-commercial/` 下立一份报价用例，号取该目录的下一个。写输入、结果代数（按恢复动作分格）、失败边界、一致性与幂等、验收示例与 AT 号、「给开发的交接」。`docs/application/README.md` 登这份用例。
6. **参数登记册**：ADR 若采纳「发布以接受为前提」那道门，加租户取值一行，号取下一个，状态写待提供。
7. **未确认的业务选择**记成 `BD-*`，按三类归好：谁可以代录接受、有效期终点的业务时区。

## 完成判据

- 上面各处写成现行规则，句子里不挂票号。
- 新用例的每个结果都说得出调用方下一步做什么；AT 号在盘点里有行（盘点由推送方重算）。
- 非作者评审无阻断；评审记录写明钉的 SHA。
- 票 10 与票 11 的「Blocked by」里指向本票的那一条随之解除。

## Comments

- 2026-11-11：随草案第二稿改。去掉「报价版本」「价格生效申请」两个词条与「报价审批职责规则」那一行参数；加计价 `CONTEXT.md` 的草稿撤下。
