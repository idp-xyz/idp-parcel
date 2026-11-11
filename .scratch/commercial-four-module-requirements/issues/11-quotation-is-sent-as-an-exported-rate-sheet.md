# 11 报价发出：导出对客价格表并登记发出

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [10](./10-quotation-object-entry-and-approval.md)、票 [01](./01-registered-price-card-reads-back-as-an-editable-workbook.md)
地盘：`internal/partycommercial`、`internal/parcelpricing/adapters/pricecardtemplate`（对客版式）、`cmd/parcel-api`、`apps/admin-web/src/pages/party`

对应要求：Quotation「发出」。

## 现象

取证钉 `11bfa1ffb`。

- 没有报价对象，也就没有发出。
- 价卡草稿读不回成工作簿，导不出文件（票 01）。
- 对客通知的出向连接器还没有实现：机制清点里 `visibilityexception.NotificationChannelGateway` 仍列在「无具体类型完整实现」之下；首方 webhook 的形态由 [ADR-0142](../../../docs/adr/0142-first-party-webhooks-are-a-product-owned-outbound-channel.md) 定。

## 要做的

1. **发出是一条登记的事实**：谁、何时、对哪个相对方、发出的是哪一版报价的内容摘要、经什么方式。只有已批准的报价版本能发出。
2. **首个方式：导出对客价格表。** 由报价所引的价卡草稿读回成工作簿，写成电子表格文件，带报价号、相对方、有效期与条款；文件由运营在系统外交给客户。
3. **对客版式不带内部信息**：源文件名与哈希、价格方向授权引用、规范化版本、录入人与批准人不出现在对客文件里。哪些格对客可见，在一处定义。
4. **导出的内容就是报价钉住的那份**：导出时所引草稿的摘要与报价上记的不同，答所引价格已变更，不导。
5. **发出不是送达**：读面分开显示已发出与客户的答复，不把导出当成客户已收到。
6. **不做**：邮件、webhook 等推送方式。出向连接器有实现之后另加。

## 要裁的

- 发出之后能不能再发一次同一版。建议可以，每次各记一条。
- 对客文件要不要进受限证据库留一份。留的话产品只存文件名、哈希与索引。

## 三类

登记与导出是机制。首个内置方式是导出文件，属产品策略。

## 完成判据

- 已批准的报价导出一份文件，价格与所引草稿逐格一致；未批准的不能发出。
- 对客文件里检索不到第 3 条列的那些内部信息；有用例。
- 所引草稿被替换之后导出答所引价格已变更。
- 管理台报价详情能导出并看到发出记录。
