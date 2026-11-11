# 01 已登记价卡与价卡草稿读不回成工作簿——改一个价要整张重录，编辑器只能新建，也导不出对客价格表

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 无。开工时随票立一篇 ADR：它接上 ADR-0349 决定八「已登记价卡反推成模板」一条，照 `docs/adr/README.md` 顶部「接上」也算部分停用，要挂前向指针
地盘：`internal/parcelpricing/adapters/pricecardtemplate`、`internal/parcelpricing/adapters/http`、`internal/parcelpricing/application`、`cmd/parcel-api` 端点表、`apps/admin-web/src/pages/pricing`

对应要求：Pricing「草稿编辑」。它也是票 [11](./11-quotation-is-sent-as-an-exported-rate-sheet.md) 的底座。

## 现象

取证钉 `11bfa1ffb`。

- [ADR-0349](../../../docs/adr/0349-the-price-card-import-template-advances-to-ppt-2-and-its-structured-workbook-is-a-second-carrier-read-through-the-same-path.md) 决定八：「模板只管『填进去』：一张已登记价卡（规范化文档）没有反向译成电子表格或结构化工作簿的路，编辑器要『打开已有价卡改一版』得另立。」
- 内容读口 `GET /pricing-price-cards/content` 交的是规范化文档，不是工作簿。
- 管理台 `card-form/workbook-to-form.ts` 的头注写着「编辑器只能新建」。把工作簿还原成表单的那一半已经写好，只用在验收里；表单表达不了的结构抛 `NotRepresentable`。
- 草稿批准前可以用同一版本号重新提交来替换（`PriceCardDraft` 的 `ResubmissionOf`），但重新提交的那一整份要操作者自己重做。

后果：供应商调一个价，运营要整张卡重录，或者一直留着当初那份 Excel。

## 要做的

1. **读口**：按方案标识与方案版本，把一版已登记价卡，或一份带内容的草稿，交回成结构化工作簿。表、列与封闭取值用 `pricecardtemplate` 里那一份单一定义（`sheets`、`cardFields`、`closedVocabulary`），不另写一套。
2. **往返不变**：读回的工作簿原样再导入，得到的规范化版本与内容摘要与原卡相同。
3. **表达不了的卡如实答**：经受控登记口进来、含导入模板还不支持的结构的卡，答一格业务结论并点名是哪一处，不交一份丢了内容的工作簿。
4. **管理台**：价卡详情加「改出新版」，编辑器用读回的工作簿开场，新版本号由操作者给，提交照旧走预览与草稿录入。同一读口另给电子表格载体的下载。
5. **不做**：逐格保存半成品草稿。草稿册只存整份；没过校验的只存问题清单。

## 三类

全部是机制。没有租户取值。

## 完成判据

- 演示租户的完整合成价卡读回再导入，内容摘要相同；有真库用例。
- 模板表达不了的卡答业务结论：不是 500，也不是残缺工作簿。
- 管理台能从一张已登记价卡改出新版并存成草稿；`apps/admin-web` 三道门过。
- `UC-PP-001`「价卡导入」一节与「给开发的交接」同步；随票的 ADR 给 ADR-0349 挂上指针。
