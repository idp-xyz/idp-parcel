# 06 管理台试算页

Category: enhancement
Status: resolved——2026-09-25 通道 3；完成记录见文末
Blocked by: 05（resolved）
地盘：`apps/admin-web/src/pages/pricing/`（新页与纯逻辑）、`apps/admin-web/src/navigation.ts` 与 `page-registry.tsx`（登记一个模块）。
出处：[02 的裁决](./02-estimate-evaluation-entry.md)；行为以 [UC-PP-001](../../../docs/application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md) 为准。

## 要做的

1. 声明表单：主要范围、价格方向、计价基准时点、实重与单位、尺寸（可缺）、分区与邮编路线、结算币种（可缺）；一格不预填业务值。
2. 结果区：逐卡并列，不排序、不标首选；每卡示评价状态（五格原词）、金额与费用组成、版本清单与解释，输入不全的卡点名缺项。
   解释的排法可参考 spec 里记的原型「同址多注入」比较卡，规则以本仓为准。
3. 四态：操作者 Intake 接上之前端点答 403，页面如实呈现未配置，不拿演示数据顶替。

## 形态

diff 全在 `apps/admin-web/**` 与本票面，按 workflow「前端切片」在 `main` 上做；05 落地后转 ready-for-agent。

## 完成记录（2026-09-25，通道 3）

代码在 `fd70fcd5`（共享树 `main`，前端切片），本票面与 spec 表随收口一笔提。

- **要做的 1**：新页 `PricingEstimatePage` 挂导航「计价」组，登记为已接线模块。声明表单逐格对应 `POST /pricing-estimates` 的载荷；`emptyEstimateDraft()` 每格为空，价格方向与各单位也不预选。主要范围只把价卡目录里出现过的范围作输入提示，不预选。编码层 `estimate-form.ts`：墙钟时刻按显示时区换 RFC 3339，可缺的格留空即不进载荷，尺寸三边加单位、邮编路线始发加目的都不许半填。
- **要做的 2**：结果区逐卡并列，不排序、不标首选。已评价的卡示评价状态原词、方向与计价目的、证据层级、合计（评价未完成时写明没有合计，不以零金额顶替）、费用行、问题项；解释、版本清单与评价号收在可展开处。输入不全的卡点名缺项。适用冲突只列候选版本，写明交价卡治理责任方裁、本页不挑。
- **要做的 3**：五个答复分支都有独立呈现。未配置那一格如实说明接入渠道未接上，不是「没有价格」，也不拿演示数据顶替。
- **门**：`tsc -b --noEmit` 退 0；`run-tests` 454/454；`vite build` 退 0。

### 验证（浏览器，证据 `S`）

用 Windows Chrome 无头模式对 `vite build` 产物截图，经本机代理把 `/api` 转给 parcel-api：

- **真后端**：parcel-api 取共享树 `68f89611` 构建，连演示种子库。真实 `POST /pricing-estimates` 答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`，页面呈现未配置说明。`68f89611..76511c67` 未碰 `cmd/parcel-api/` 与计价上下文，结论在当前 tip 同样成立。
- **已形成与适用冲突**：操作者 Intake 未接上，今天没有真路径能让端点答出这两态，所以用代理注入。注入体不是手写口径：用一个一次性测试（跑完即删、未入库）借票 05 的 HTTP 适配器测试夹具，让真适配器 `NewEstimateEndpoint` 序列化出来；已形成那份与原先的注入体程序比对严格相等。因此页面的字段名与 Go 端 JSON 标签对得上。其中评价来自 `pptest.Evaluate`（领域 `EvaluatePricing`），不是经编排连库的端到端结果。
- **验证中发现并修掉**：证据一格原先误用参考序列的「证据等级」词表（可复核 / 断言强度），改为按验收矩阵原词新补的 `evidenceLevelLabels`（真实生产 / 历史数据回放 / 受控模拟）。这笔修正含在 `fd70fcd5` 里。
- **没在浏览器里走的**：调用方问题、未形成答复、传输失败三格只由穷举 `switch` 的类型检查兜住，未注入截图。
- **披露**：为绕过 OIDC 登录，只在 `/tmp` 的产物副本里注入了会话桩脚本，未入库；这一版管理台本来就不把令牌发给 `/api`。截图留在本机临时目录，未入库。

### 评审

diff 只碰业务页与 `src/` 根下的导航和页面登记，没碰 `templates/`、`shell/`、`components/` 等共享面，按 workflow「前端切片」做业务页自查。没有其他会话可做非作者评审，本记录不算非作者评审。
