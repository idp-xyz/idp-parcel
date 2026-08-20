# 账户关系迟于事件建立时，客户视图没有重派生触发器

Category: enhancement
Status: resolved

来自外部评估（基线 `49a2ab0`），协调岗已核实。

## 现象

`DeriveCustomerViewOnProjectionAdapter.HandleDerivedTrackingProjection`（
`internal/visibilityexception/adapters/parcelshipment`）按已裁的三格代数落地：账户反查
零行时不派生视图、不发明账户，交回 nil，消费门把该投影派生信封入账。重试仅由**下一个
投影版本信封**触发——每个投影版本各入队一封，账户到位后的下一个源事实会带来重试机会。

于是存在一格空档：某包裹的**末次**源事实已入账、其后才建立可反查的已接受委托（账户维
才填得上）时，不再有新的投影版本信封，该包裹的客户视图永远不会派生。投影本身照旧存在
（UC-VE-002 侧完好），缺的只是客户视图（UC-VE-008 侧）。

## 与已有裁决的关系

`.scratch/ve-parcel-to-party-lookup/report.md` 的三格代数只裁了「当下反查不到时不发明
账户、不落『无轨迹』」，没有裁「事后取得回时如何补」。本票不推翻裁决，只补这个空档。

## 未定问题（先裁再动，可能要 /grill-with-docs）

触发器从哪来，候选方向牵动面各不相同：

- **a) 事件驱动补派生**：VE 消费 PS 的委托接受/采认类事实，对该客户名下包裹的当前投影
  重试客户视图派生。要先对照 CONTEXT 与按消费清点确认该信封型是否可登记（ADR-0049
  「接得住才登记」）。
- **b) 运维重放口**：提供手工触发某（租户+包裹）重走派生的接口，不加自动链路。
- **c) 接受为已知边界**：首发不补，把「账户迟到的包裹无客户视图」写进 UC-VE-008 的
  已知边界，试点前重估。

## 今天的影响

无租户、无生产流量，零实际影响；首发试点前要有答案，因为「先有包裹流转、后补客户账户
绑定」在运营上是真实序。

## Comments

- 2026-08-20 MCP-1：外部评估四项可操作发现之一（其第 5 条后半），核实属实后立票。
- 2026-08-20 MCP-1：**清账置 resolved**——「未定问题」已由用户拍板走方向 a（见本目录
  [03 号决定文档](./03-rederive-route-decision.md)，已 resolved），实现落 ve-008 票族并合入
  main：`004e338`（客户归属确立进客户视图触发，进 CONTEXT 与 UC）+ `a771bc3`（接受决定经
  FanOut 补派生，UC-VE-008/AT-VE-169）。方向 b 的运维重放口另立
  [`ve-008-late-account-rederive/issues/04`](../../ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md)
  阻于 PAR-INT-01。本票状态此前停在死会话冻结时刻，由完成度评估快照（2026-08-20）点名后核实清账。
