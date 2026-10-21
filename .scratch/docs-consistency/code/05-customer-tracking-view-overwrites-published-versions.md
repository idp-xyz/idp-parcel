# 05 VE 客户全程追踪视图的已发布版本被原地覆盖，且每个投影新版本都换一代视图

Category: bug
Status: ready-for-agent——R1（上线前必修）：对外发布物不留痕。客户视图派生已在 `cmd/parcel-dispatch` 生产装配里跑，客户读口在 `cmd/parcel-api` 上；第一天起每个投影新版本都改写一次客户视图那一行，已向客户发布过的版本读不回，`prior_version` 指着不存在的行。CONTEXT 明令「不得静默删除或改写历史披露」；没有绕法。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-01（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在，VE 代码在此期间未改）。

## 现象

1. **原地覆盖**：客户视图替代时 `UPDATE` 当前那一行，表主键是（租户，客户账户，包裹），前一版被改写，已发布的历史留不下。
2. **按投影版本换代**：只要投影版本变了就替代客户视图，四维内容没变也换一代，与「不与内部投影版本一一对应」相反。

## 证据

- 文档：
  - VE CONTEXT「客户可见性与通知」：「来源更正……必须重新派生客户视图并形成追加更正或明确替代关系。原已发布版本保留，但不再有效的内容不得继续显示为当前事实。」「客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影版本一一对应；投影重新派生本身不要求形成新的客户视图版本」「已经向客户发布的里程碑、ETA、异常说明或通知出现更正时，必须以追加更新或明确替代关系表达，不得静默删除或改写历史披露。」
  - UC-VE-008 `AT-VE-161`：「形成追加更正/替代视图，原版本保留且不再作为当前结果」。
  - ADR-0065 Context 否决过同一形状（「指针在，靶子不在」），决定二：「原版本的留存以存下来为准，不以可重放为准」；其 Consequences 写「客户视图那一层不变：CONTEXT 既有硬句……原样适用」。
- 代码：
  - `internal/visibilityexception/adapters/postgres/customer_view.go` 的 `CustomerViews.Save`：替代分支 `UPDATE visibility_exception.customer_view SET version = …`，乐观锁按旧版本号。
  - `migrations/visibility_exception/0001_customer_view.sql`：`PRIMARY KEY (tenant_id, customer_account_id, parcel_id)`。
  - `ports.CustomerViewStore` 注释「原版本由替代关系承担历史，库只管当前」；`domain.RehydrateCustomerView` 注释「库里只有当前版本」。
  - `application.DeriveCustomerViewHandler.Handle`：`current.BasedOn() == command.Projection.Version()` 才跳过，否则 `current.Supersede(...)`。
  - 测试：内存替身用例 `TestANewerProjectionSupersedesTheCurrentViewKeepingHistory` 绿；真库用例 `TestSupersedeAdvancesTheVersionWithOptimisticLock` 只验指回、不验留存。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：迁移另取号，把 `customer_view` 改成版本表加当前标记（照 `0016_tracking_projection_versions.sql` 的投影形状）；`Save`、读回、`FindCurrent` 随改；派生编排加「内容或披露依据变了才换代」；存量行迁成各自的首版。

## 最小的诚实修法

1. 客户视图版本只增不改写：每一版一行，当前版另有标记或由最大版本派生；替代写新行并回指前一版。
2. 只在四维内容或披露依据变化时形成新版本；投影换版本、四维不变，只更新「所依据的投影版本」，不换代——这一格怎么存见「要裁的」。
3. 真库用例补「替代后前一版仍读得回」。

## 要裁的

- 四维未变、投影换版本时，「所依据的投影版本」记在哪：当前版上原地更新这一列（它不是对客发布的内容），还是不记、读时现取。推荐前者，作者可代决并写越权风险点。
- 票 [ve-customer-view-per-projection-version/01](../../ve-customer-view-per-projection-version/issues/01-does-customer-view-keep-a-generation-per-projection-version.md) 裁「不需要按投影版本留代」时引的前提「CONTEXT『原发布历史保留』已保证历史不丢」在代码里不成立；那张票的结论（不按投影版本留代）本票照守，前提已在那张票上补注。
- 产品文档 `docs/product/IDP-PARCEL-BUSINESS-PROCESS-AND-MODULE-GUIDE.md`「投影和客户视图只增版本」一句与代码不符，本票落地后即成真，不必改文档。

与 [06](./06-customer-view-shows-dimensions-without-a-visible-judgement.md) 改同一个派生编排，宜先后紧接。
