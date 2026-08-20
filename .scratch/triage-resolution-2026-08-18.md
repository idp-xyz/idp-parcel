# 2026-08-18 needs-triage 分诊决议（草稿 v2，待确认）

Category: chore
Status: draft

八张 `needs-triage` 票的去向决议。只定**去向与动作**，不替领域 owner 拍口径；
每张票需要谁裁什么、裁完做什么，按下表执行。族分析以
[ticket-families.md](./ticket-families.md) 为准，不重复。

## 取证基线

本文 v1 盘于 `37495cd` 之后的票面；v2 复核时重取以下事实（2026-08-18）：

- 派发组合根已接通（`64bae12`/`cda3879` 基线定级）：`Dispatcher` 一拍 + 进程内直投，
  路由表仅一条（PS 接受决定 → NR），**消费者方向在扩**（`60ea63c` 第二方向、`e5009d8`
  第三方向勘察落档）。
- 基线「横切缺口」已换代：无生产实现端口 **11**（`cda3879` 盘于 `bfb2063`，13→11），
  总数 200、PG 适配器 132。
- `outbox-partition-key/02`（POD 更正静默丢失）已修：`390d4ad` 让 POD 更正自成一份
  信封并与首登同分区——**乙类修法已有先例落地**。
- 评审 081701 七项处置标账在 `docs/review/081701.md`；#3（消费方向扩展）排在 #7 之后。
- NR 证据端口「未配置」格已由 ADR-0052/0053 落地（`3b84eeb`/`0f266be`）。
- 本表引用的票内提交（`f994dc3`、`91af99c`、`c45d0d7`、`a5095ae`、`b46400e`、`cab9d1b`）均存在。

## 决议表

| 票 | 性质 | 建议去向 | 谁裁 / 裁什么 | 触发条件 |
|---|---|---|---|---|
| `product-version-closure/design-input.md` | 备料，使命已完成 | **resolved** | 无待裁 | 无——design.md 已验收、五项已裁；注意整个 `.scratch/product-version-closure/` 目录尚未提交，闭合时同笔提交 |
| `port-inventory-r25/report.md` | 盘点数据，**已被吸收** | **resolved** | 无待裁 | v1 曾建议 ready-for-human 入库，复核撤销：`72fa500` 已把重盘计入基线、`cda3879` 重算为 11/200，「差 3」问题随数字换代消失，无需任何入库动作 |
| `outbox-partition-key/01` | 真缺陷 | **ready-for-agent（第一步）** | 无需人裁：票内已定两步走 | 第一步：只改分区键不动 ID。**清单按 `390d4ad` 之后修正**——TF POD 更正已修；剩余甲类 PS×2 + SA 对账单作废、乙类 SA 重分摊/重派生与 PS 终局/网络收寄、丙类 SA 核销撤销。同笔落 `internal/architecture` 门禁（比 Envelope 复合字面量 `ID` 与 `PartitionKey` 是否同一表达式）+ 具名例外清单。执行者先重数当前逐事件分区键数目（46 个 handoff 的最新状态，以 `62abc62` 清点与 `390d4ad` 之后为准）并验证 outbox 表无存量 PENDING |
| `supplier-expected-cost-correction/01` | 真分歧 | **ready-for-human** | SA 领域 owner：裁「同币种纠错后原币金额应是什么」，两扇门向哪边对齐；裁后随 UC-SA-002 编排落地（当前无生产调用点，代价最低） | owner 裁定后改票 |
| `ve-milestone-mapping-key/01` | 结构性填不满 | **ready-for-human** | VE 领域 owner + 各源上下文 owner：裁「源事实类型码归谁拥有」——是各源上下文已接受事实的一部分，还是 VE 消费侧归纳。答案定领域侧/端口侧方向。建议与 `ve-claim-eligibility-dimensions` 的 (b) 切块同一轮会审（同族同源，避免两轮仲裁） | owner 裁定后改票或直接实现 |
| `nr-route-evidence-views/01` | 机制半边可做（已收窄） | **ready-for-agent** | 实施者按票内判据自决「先建目录 schema 是否返工」 | **范围按 `3b84eeb`/`0f266be` 修正**：③显式未配置报错已由 ADR-0052/0053 落地，剩余①版本化网络目录 schema（节点/连接/线路/服务区域/日历截单/临时调整/路由策略各一表带有效区间）②按 asOf 选版 ④视图修订标识的产生与同版原子性。前置决定：PS 客户地址 → NR 服务区域的提供路径未定（CONTEXT 明文有、`ports.go` 无端口），先裁再动，别让实施者撞上 |
| `ps-external-mark-relations/01` | 未建模子域 | **wontfix（暂缓）** | PS 领域 owner / 产品 owner：随 PS 面单交易**子域建模开始**时恢复（比「切片立项」更精确） | 恢复条件满足时复评（评审复评点：081701 #3 消费方向扩展之后） |
| `route-handoff-delivery-granularity/01` | 已选 A，解法未排期 | **wontfix（暂缓）** | NR/产品 owner：出现第二个多成员意图或 NR 未决重试需求时恢复；当前 A 的取舍与理由已由 `f994dc3` 代码与本票互为出处 | 恢复条件满足时复评 |

## 与评审 081701 的衔接

#3（消费方向扩展）排在 #7 之后，`outbox-partition-key` 第一步与 NR 机制半边都先于
消费扩展完成最划算：分区键改动的免费窗口随消费者接线而收窄，版本化目录 schema 是
NR-003 复核消费方向的取数地基。

## 待确认项

1. v2 修正的三处（r25 改 resolved、outbox 清单按 390d4ad 修正、NR 范围收窄）是否认可。
2. 两张 wontfix 的「暂缓」定性。
3. `outbox-partition-key/01` 第一步是否现在就放给 agent（涉及 PS/SA/TF 三块地盘，需先占号协调）。