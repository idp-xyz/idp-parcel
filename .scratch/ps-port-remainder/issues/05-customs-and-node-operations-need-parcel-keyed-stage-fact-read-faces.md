# 关务与节点作业各缺一个按正式包裹键的阶段事实读面：`parcel-shipment` 的两只未接适配器等它们

Category: enhancement
Status: draft——由 ADR-0118 决定四拆出（2026-09-07，通道 2）；两半分属 CC owner 与 NO owner，PS 侧只等接线，不在 PS 地盘动手
Blocked by: 无（各 owner 认领后各自开工；两半互不阻塞）

## 缺口

ADR-0118 让资料修订编排在问矩阵前先判「资料修订阶段」，六格里四格的事实来自邻接上下文：装袋归 `node-operations`，申报资料形成 / 已提交 / 案件已关闭归 `customs-compliance`。PS 侧读口已立（`ports.CustomsStageView`、`ports.ConsolidationStageView`），生产装配今天接的是两只**未接**适配器（`adapters/customscompliance.UnconnectedCustomsStageView`、`adapters/nodeoperations.UnconnectedConsolidationStageView`），一律答`不知道`——于是授权过了之后每次修订都停在 `SourceDataAmendmentStageUndetermined`，不问矩阵。停点如实，但要往前走得靠提供方各出一个读面。

取证于 `main = 08f54867`（逐符号名）：

- `customs-compliance`：`ports.CustomsCaseKey` 是（租户、管辖、方向、程序、义务范围），不含包裹；`domain.DeclarationUnit.Members()` 记 `DeclaredParcelReference`，但 `ports.DeclarationUnitStore` 只有 `Save` / `FindByID`，头注写明「案件→单元集」的反查「今天没有消费方，端口不预设方法」；`DeclarationSubmissionStore` 按（租户 + 单元 + 程序）键；`CaseClosureStore.FindByCase` 按案件标识。**没有任何一条路从正式包裹走到这三件事实。**
- `node-operations`：`ports.ContainmentIndex.CurrentParent(tenant, HandlingUnitID)` 答一件作业实物此刻被哪个未关闭单元直接包含；作业实物与正式包裹之间是识别成功后建立的版本化关联（`NodeIntake` 上的 `ParcelAssociationReference`），PS 的 `adapters/nodeoperations` 对待识别实物明确拒绝、不拿包裹标识冒充。**没有按正式包裹答「是否已装袋」的口。**

## 要提供方各立什么（形状建议，取舍归各 owner）

- **CC**：一个按（租户 + 正式包裹引用）的读面，答该包裹当前归属的申报单元有没有已形成尚未提交的版本、有没有已提交的版本、所在案件有没有关闭。三格分开交，不折成一个「关务阶段」——哪一格压过哪一格是 PS 的判断（`domain.JudgeAmendmentStage`）。读面存在而该包裹没有任何单元或案件时三格都答`不在`；`不知道`这一格留给 PS 的未接适配器，CC 读面自己不该答它。落点可以是 `ports` 上的一个 `*View` 加 postgres 适配器（申报单元成员列今天在库里，反查是一条带索引的查询），是否要新迁移由 CC owner 判。
- **NO**：一个按（租户 + 正式包裹引用）的读面，经版本化关联找到作业实物再问 `ContainmentIndex`，答「此刻在不在某个集运单元里」。关联缺席（待识别）时答什么要 NO 定：按 ADR-0118 的三态，「没有关联」不等于「没装袋」，更像`不知道`；但那是 NO 对自己事实的解释，PS 只消费。
- **PS 侧**（本目录，接线时做）：把装配点上那一只未接适配器换成读真读面的适配器，端口、编排、判断函数都不动；`cmd/parcel-api` 装配用例②（授权过了停在判不出阶段）随之改写为读面存在时的答复。

## 红线

- PS 不绕过已导出端口去读 CC/NO 的表；读面未立之前两只未接适配器保持答`不知道`，不改成`不在`。
- 读面只交事实，不交阶段；阶段的次序与并格规则在 ADR-0118 决定一，提供方不复制第二套。

## 参照

ADR-0118 决定三、四；`internal/parcelshipment/ports/amendment_stage.go`；`internal/parcelshipment/adapters/customscompliance/unconnected_customs_stage_view.go`；`internal/parcelshipment/adapters/nodeoperations/unconnected_consolidation_stage_view.go`；`internal/customscompliance/ports/ports.go` 的 `CustomsCaseKey` / `DeclarationUnitStore` 头注；`internal/nodeoperations/ports/ports.go` 的 `ContainmentIndex`；本目录 02。

## Comments

- 2026-09-07 · 通道 2：立票（draft）。PS 半边（读口 + 未接适配器 + 编排停点）已在分支 `mcp2-ps-ports` 落地，本票只记提供方的两条缺口，请 MCP-1 派或各 owner 认领。
