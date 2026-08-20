# 2026-08-18 needs-triage 分诊决议全面复核报告

Category: chore
Status: draft

对[分诊决议 v1](./triage-resolution-2026-08-18.md)的全面复核。复核验证了 8 个票内引用
提交、基线最新数字、派发接线状态与评审处置标账，发现三处 v1 论断不成立，均已修正并
落回决议 v2。本文只记复核结论与证据，决议正文以决议文件 v2 为准。

## 取证基线

- 票内引用提交全部存在：`f994dc3`、`91af99c`、`c45d0d7`、`a5095ae`、`b46400e`、
  `cab9d1b`、`7e22f27`、`64bae12`（`git cat-file -t` 逐条验证）。
- 派发组合根已接通（`64bae12`/`cda3879` 基线定级）：`Dispatcher` 一拍 + 进程内直投，
  路由表仅一条（PS 接受决定 → NR）；消费者方向在扩（`60ea63c` 第二方向、
  `e5009d8` 第三方向勘察落档）。
- 基线「横切缺口」已换代：无生产实现端口 11（`cda3879` 盘于 `bfb2063`，13→11），
  总数 200、PG 适配器 132（`docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md`
  第 186 节起）。
- `outbox-partition-key/02`（POD 更正静默丢失）已修：`390d4ad` 让 POD 更正自成一份
  信封并与首登同分区——乙类修法已有先例落地。
- 评审 081701 七项处置标账在 `docs/review/081701.md`；#3（消费方向扩展）排在 #7 之后。
- NR 证据端口「未配置」格已由 ADR-0052/0053 落地（`3b84eeb`/`0f266be`）。

## 修正一：`port-inventory-r25` 无需入库，改 resolved

v1 建议「ready-for-human 确认入库」，不成立：

- `72fa500` 已把重盘计入基线，`cda3879` 又重算为「200 接口 / 无生产实现 11 个」
  （13→11）；r25 报告的「差 3 未能还原」已随数字换代消失。
- 动作已经发生过，r25 只是历史盘点记录，无任何入库动作可做。
- 处置：resolved，无待裁。

## 修正二：`outbox-partition-key/01` 紧迫性上升、清单收窄

- 票写于 `cab9d1b` 时的前提「没有派发器在跑」已过时：派发组合根已接通，消费者
  方向在扩。已接线的方向（PS 接受决定）恰是分区键做对的正例，无乱序风险；无租户
  流量意味着 outbox 无存量 PENDING——免费窗口仍在，但随消费扩展收窄。
- 清单按 `390d4ad` 修正：TF POD 更正已修；剩余甲类 PS×2 + SA 对账单作废、
  乙类 SA 重分摊/重派生与 PS 终局/网络收寄、丙类 SA 核销撤销。
- 第一步仍只改分区键不动 ID，同笔落 `internal/architecture` 门禁（比 Envelope
  复合字面量 `ID` 与 `PartitionKey` 是否同一表达式）+ 具名例外清单；执行者先重数
  当前逐事件分区键数目并验证无存量。

## 修正三：`nr-route-evidence-views` 范围收窄

- ③「显式未配置报错」已由 ADR-0052/0053 落地（`3b84eeb`/`0f266be`），不再属于
  待做机制半边。
- 剩余：①版本化网络目录 schema（节点/连接/线路/服务区域/日历截单/临时调整/
  路由策略各一表带有效区间）②按 asOf 选版 ④视图修订标识的产生与同版原子性。
- 前置决定：PS 客户地址 → NR 服务区域的提供路径未定（CONTEXT 明文有、`ports.go`
  无端口），先裁再动。

## 未被推翻的结论

- `product-version-closure/design-input.md` → resolved（注意整个目录未提交，闭合时
  同笔提交）。
- `supplier-expected-cost-correction/01` → ready-for-human（SA owner 裁原币语义；
  当前无应用层调用点，裁决窗口仍开）。
- `ve-milestone-mapping-key/01` → ready-for-human（类型码归属；建议与
  `ve-claim-eligibility-dimensions` (b) 同轮会审）。
- `ps-external-mark-relations/01`、`route-handoff-delivery-granularity/01` →
  wontfix（暂缓，恢复条件与复评点已写入决议 v2）。

## 待确认项

1. 三处修正是否认可。
2. 两张 wontfix 的「暂缓」定性。
3. `outbox-partition-key/01` 第一步是否现在就放给 agent（涉及 PS/SA/TF 三块地盘，
   需先占号协调）。

确认后按决议 v2 落各票状态。