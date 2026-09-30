# 04 冻结边界与已执行前缀的判定

Category: enhancement
Status: resolved——2026-10-01 进 main（`67602b37→efc1fee8`，补提交 `c6db4c51→df6c45e3`，清点 `255e2a6d`，ADR-0172）。Blocked by 03 已在 main
Blocked by: 03（路由策略版本的内容载体）
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「路由策略族」那一步
地盘：network-routing 领域与复核编排；路由策略版本内容（随 03 的载体扩，迁移号开工时预留）；network-routing [`CONTEXT.md`](../../../docs/domain/network-routing/CONTEXT.md) 相关句。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二、七；CONTEXT Language「路由冻结边界」与 Rules「改路只能改变尚未执行的剩余旅程」；[UC-NR-003](../../../docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md) `AT-NR-041`。

## 做什么

1. **冻结边界的判断形态**归产品，租户只选形态、填取值（ADR-0146 决定二）。本票裁首版形态并写进 CONTEXT，策略版本声明它。
2. **已执行前缀**：由计划段链与当前可控节点判出已执行的计划前缀与剩余旅程。可控节点只认节点收寄或权威运输交接确认的那一格，不凭扫描、位置或消息顺序推断（CONTEXT Language「当前可控节点」）。
3. **复核接上两者**：过冻结边界且原计划仍可执行时，轻微改善不触发改路（`AT-NR-041`）；硬约束失效照旧重判。取证于 `64b37f27`：复核编排里没有任何冻结判断，这是新增，不是改写。

## 不做

- 不定任何租户的冻结取值；改善阈值与自动改路条件归 05；与装载的并发归 06。

## 完成判据

- [x] 领域用例：前缀判定（可控节点在计划节点上 / 不在 / 尚无可控节点）与冻结判定各分格覆盖。
- [x] 复核用例覆盖 `AT-NR-041`。
- [x] 策略版本未声明冻结形态时，冻结判断答未配置，不当作「未冻结」。

## Comments

**评审 ← 通道 1 · 钉 `67602b37` · 2026-10-01**

- **阻断**：无。架构门禁 PASS。`TestFreezeStaysUnconfiguredWhenTheFormIsUndeclared`、`TestAFrozenPlanIsKeptWhenItIsStillExecutable`、`TestExecutedPrefixDoesNotInventANodeFromAScan` PASS。
- **结论：可重放**。全量在拣选后才红：两条既有「仍适用」收寄用例的证据没声明冻结形态，答了 `FREEZE_FORM_UNCONFIGURED`。退回原分支补提交，不改写 `67602b37`。

**补提交评审 ← 通道 1 · 钉 `c6db4c51`**

- 只改两份测试。`evidenceWithOpenFreeze` 声明 `REMAINING_SEGMENT_COUNT`，限额 0；计划一段、收寄在首节点，剩余段数 1，已配置且未越过。未声明路径未改回「未冻结」。
- **阻断**：无。

**进 main 记录（2026-10-01，通道 1）**

重放到 `c06bc7a7` 之上，零冲突：`67602b37→efc1fee8`，`c6db4c51→df6c45e3`，清点 `255e2a6d`。ADR 索引按编号把 0172 排到 0170、0171 之后。全量 `go test -p 1 -count=1 ./...`：135 ok / 0 FAIL。分支作封存出处。
