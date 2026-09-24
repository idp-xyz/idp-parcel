# 06 管理台试算页

Category: enhancement
Status: draft
Blocked by: 05
地盘：`apps/admin-web/src/pages/pricing/`（新页与纯逻辑）、`apps/admin-web/src/navigation.ts` 与 `page-registry.tsx`（登记一个模块）。
出处：[02 的裁决](./02-estimate-evaluation-entry.md)；行为以 [UC-PP-001](../../../docs/application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md) 为准。

## 要做的

1. 声明表单：主要范围、价格方向、计价基准时点、实重与单位、尺寸（可缺）、分区与邮编路线、结算币种（可缺）；一格不预填业务值。
2. 结果区：逐卡并列，不排序、不标首选；每卡示评价状态（五格原词）、金额与费用组成、版本清单与解释，输入不全的卡点名缺项。
   解释的排法可参考 spec 里记的原型「同址多注入」比较卡，规则以本仓为准。
3. 四态：操作者 Intake 接上之前端点答 403，页面如实呈现未配置，不拿演示数据顶替。

## 形态

diff 全在 `apps/admin-web/**` 与本票面，按 workflow「前端切片」在 `main` 上做；05 落地后转 ready-for-agent。
