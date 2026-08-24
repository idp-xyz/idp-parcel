# 05 取消页接线与 liveIds 登记（MCP-3）

Category: enhancement
Status: resolved

MCP-3 取消切片的预留兑现票：../spec.md「不做」节为本会话预留 `pages/shipment-request/**`
与取消页的 liveIds 登记。解锁事实是 `90c7742`——POST /shipment-requests/parcel-cancellations
真挂载（第十端点，UC-PS-006 取消编排接真，授权缝落 AUTHORITY_UNCONFIGURED 而非
UNAVAILABLE）。此前 CancelParcelPage 动作区的「接受后取消端点尚未建立」自此失真，本票
按页内注释既定模式照 withdraw 页对形状接线。

## 范围与落点（均在 apps/admin-web/src/ 下）

1. **api.ts**：收编取消端点形状——`CancellationOutcome` 七格词取
   `application.CancelParcelOutcome` 原字符串；`CancellationResponseBody` 与
   `internal/parcelshipment/adapters/http` 的 `cancellationResponse` 一一对应；
   `CancellationDraft` 只收客户可声明部分（原提交来源请求标识 + 委托标识双重指名、
   目标包裹、请求方引用、原因引用），来源信封与业务发生时间归接入适配器。取消没有
   自己的请求信封（原提交来源身份 + 包裹即幂等键），故不设「取消请求标识」字段。
2. **presentation.ts**：七格结果词表（三种已提交走向都是同等有效业务答案，不画成
   错误）+ 六格未决原因词表。`AUTHORITY_UNCONFIGURED` 是唯一未配置态：呈现为
   「授权规则未登记」，恢复动作是租户登记授权规则（PAR-COM-17 实例半边），不是错误
   也不是业务否定；词表措辞守住 UC-PS-006 明禁的双向默认。
3. **CancelParcelPage.tsx**：动作区从「端点尚未建立」占位换成对形状的表单与逐件
   结果面板。批量语义按 UC-PS-006 步骤 1：端点一次受理一件，页面把多件包裹逐件
   分发、逐件呈现，部分成功不以整单状态覆盖成员差异；不提供把已收寄包裹改回
   「已取消」的口子——边界后走向（待处置/拒绝）由服务端裁决，页面不预判。
   入口边界文案保留原有 CONTEXT.md 出处。
4. **index.ts**：CancelParcelPage 注释从「未配置骨架」转「已接线」，新增三个取消
   类型出口。
5. **page-registry.tsx**：liveIds 加 `'cancel-parcel'` 一行（只加自己行不动邻行，
   通道占号/释号广播各一次，无异议）。

## 地盘说明

`pages/shipment-request/**` 属 MCP-3 预留地盘（../spec.md「不做」节）；page-registry.tsx
属 7 号地盘，登记行按本轮改派纪律修订自落（先占号后动，改完即释）。后端
`cmd/parcel-api/**` 与 `internal/**` 本票零写入，只读取形状。

## 验证

- `pnpm exec tsc --noEmit`（apps/admin-web 下）绿；编辑文件零 lint。
- 提交 SHA：`73a2c7b`（代码笔）；本票与 spec.md 子票行随簿记笔另提交。
- 接线后请求走 /api 前缀经代理转发；端点 Intake 现挂「未配置即拒」，页面发请求会
  得到 403 + ACCESS_CHANNEL_NOT_CONFIGURED，ResultPanel 按未配置态呈现——这是
  ADR-0055 的诚实答案，不是接线缺陷。
