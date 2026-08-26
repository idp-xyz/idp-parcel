# VE 索赔材料归集面机制未建——ClaimEvidenceView 只能答「无从查起」

Category: enhancement
Status: ready-for-agent

自[接线票 04](../../parcel-api-remaining-endpoint-wiring/issues/04-ve-claims-wiring.md)
Comments 里的既知事实提出立票；[第二十六轮重盘](../../mechanism-reinventory-r26/report.md)
第三节点名为端口计数看不见的机制余量之二（`ClaimEvidenceView` 判据 A 记缺、判据 B 由
`cmd/parcel-api` 的显式未配置桩顶记「有」）。

## 事实

- `veports.ClaimEvidenceView.ReceivedMaterials(tenant, batch, item)` 只答**已收讫材料的
  要求引用**；「最低材料要求是什么」在另一个端口，两边相减才核得出缺什么（端口注释）。
- VE 领域面已有证据项模型（收到不等于采信、披露锚定原件），但**材料归集的登记面（表 +
  写入口 + 读适配器）未建**——桩按端口自设 known=false 格如实答「归集无从查起」，绝不造
  空清单（那会把差集变成整份清单、替一个不存在的归集面给客户立限期补充义务）。

## 裁定（承用户 08-26「自决」委托，与既有先例对齐）

归集面第一形态走**受控 CLI 登记**，与治理登记（票 12 裁定：执行者身份双轨、通道技术身份
入口自取 + 决定人显式必填）与 VE 目录登记（票 15 同款）同形——材料实物经租户的客服/作业
面收到后由操作者登记收讫事实，不是业务端点面（客户自助提交材料属 PAR-INT-01 之后的渠道
工作，本票不预造）。登记行按（租户、索赔批次、索赔项、材料要求引用、收讫时间）五件成行，
只登收讫事实不登内容实体（敏感材料实体外置，仓库只登脱敏引用——AGENTS 红线「敏感实例
外置」）；同五件重登幂等；不设删除，撤销另立撤销行（与「作废只改适用关系不删历史」的
仓内纪律同形）。

## 要做什么

- VE 迁移：材料收讫登记表（五件成行 + 撤销行）；
- VE postgres 读适配器实现 `ClaimEvidenceView`（读收讫减撤销的现存集）；写口给受控 CLI；
- `parcel-ve-register` 添登记子命令（沿既有 CLI 的身份与拒默认纪律）；
- `parcel-api` 换掉 `unconfiguredClaimEvidence` 接真读。

## 完成标准

- 归集面空时 known=true + 零件（「查过了，一件都没收到」的有效事实——注意这与今天桩的
  known=false 语义不同，接真后差集成立、限期补充可以推进）；未建/读不通才走各自的格。
- 登记-读回-撤销三态真库测试；全仓真库套件绿；判据 A 复点该口从缺转有。
