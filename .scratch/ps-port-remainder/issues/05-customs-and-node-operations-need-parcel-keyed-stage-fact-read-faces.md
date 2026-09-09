# 关务与节点作业各缺一个按正式包裹键的阶段事实读面：`parcel-shipment` 的两只未接适配器等它们

Category: enhancement
Status: resolved——2026-09-07 通道 2 按 task-2f035050（owner 授权自决口径）一次落齐两半读面与 PS 接线，分支 `mcp2-psr05`（基线 `61344989`）：`13f3ba65` CC 读面 + 迁移 0017、`72b77de0` NO 读面 + 迁移 0004、`f91100c2` PS 两只真适配器 + parcel-api 装配接真；main 上的 SHA 以 MCP-1 重放广播为准。此前 draft——由 ADR-0118 决定四拆出（2026-09-07，通道 2）
Blocked by: 无

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
- 2026-09-07 · 通道 2（task-2f035050，owner 授权自决口径，取证锚 `main = 61344989`）：**裁决与完成记录**，分支 `mcp2-psr05`。两半各一次 `/domain-modeling`，形状归各 owner 上下文自己说，PS 只翻译。
  - **CC 半边（`13f3ba65`）**：`ports.ParcelDeclarationFactsView` 按（租户 + `DeclaredParcelReference`）交 `ParcelDeclarationFacts` 三件独立布尔事实——`MemberOfUnsubmittedUnit`（是至少一个尚无提交版本、且未被替代的申报单元的成员）、`InFixedSubmissionVersion`（至少一个已固定提交版本的组成快照含它）、`InClosedCase`（所在案件里至少一个当前已关闭）。三格分开交，不折阶段；**没有`不知道`格**——三本册子都是 CC 自己的，空册三格皆否是如实答案。三条读法：「已关闭」= 有关闭决定且无受控重开（生命周期「已关闭 → 重新打开」；今天一案一份关闭记录装不下第二个关闭期，重开过的读作未关闭，模型长出第二关闭期时读面随之改）；「所在案件」取案件建立时的直接包裹关联（`CaseParcelAssociation`，`customs_case.parcels`）与经当前申报单元的案件维两路之并；「尚无提交版本的单元」排掉被替代的（另一单元 `replaces_unit_id` 指向它，替代编排今天无写入方、读面先按列语义写好），已固定的版本按组成快照反查、永久保留不随替代消失。postgres 一条 SQL 三个 EXISTS；迁移 `0017` 只加索引（两张成员 jsonb 列与 `customs_case.parcels` 的 GIN jsonb_path_ops、`replaces_unit_id` 部分索引），0002 自注「无按成员检索的读面」到此不再成立、原文不改。不拓宽 `DeclarationUnitStore` / `DeclarationSubmissionStore` / `CaseClosureStore` 点读口（判据同 ADR-0077 Decision 五）。
  - **NO 半边（`72b77de0`）**：`ports.ParcelContainmentView` 按（租户 + `ParcelAssociationReference`）交封闭三值 `ParcelContainment`——`在`（至少一件已关联该包裹的作业实物此刻被未关闭单元直接包含）、`不可归属`（没有已关联实物在袋里，但某件仍待识别的实物候选里列着它、且那件此刻在袋里——候选不是归属，CONTEXT「候选尚未确认时不得据此执行方向性作业」；既不答在也不答不在）、`不在`（其余，**含 NO 从未把任何实物关联到它**——没有关联就没有可归属于它的装袋事实，是对自己册子的如实回答不是不知道；答成不知道会让每个尚未到站的包裹都判不出阶段）。正式包裹 → 作业实物的映射只有一处：收寄判断里识别成功建立的版本化关联（`reception.intake->>'association'`，与 PS `adopt_on_node_intake` 把该引用当 `DeclaredParcelID` 用是同一条约定）；`Identify` 路径今天无应用层调用方，`AddMember` 不核成员是否已识别，所以候选那一格真实存在。迁移 `0004` 只加索引（关联表达式 btree、候选 GIN）。不拓宽 `ContainmentIndex`。零值哨兵进枚举门禁。
  - **PS 侧（`f91100c2`）**：`adapters/customscompliance.CustomsStageView`（窄口 `ParcelDeclarationFactsLookup`，逐格布尔译三态）与 `adapters/nodeoperations.ConsolidationStageView`（窄口 `ParcelContainmentLookup`，全函数：在→在、不在→不在、不可归属→不知道、集外上抛 `ErrUntranslatableAnswer`）；读面调不通上抛让编排落 `SourceDataAmendmentStageFactUnavailable`。两只 `Unconnected*` 及其用例随读面立起退场。`cmd/parcel-api` 装配抽出 `buildAmendmentStageFactViews` 接真；**端口签名、领域判断、编排一行未动**。装配用例②改写为钉停点后移：授权过了阶段按真读面判出、停在矩阵未登记（`AWAITING_REVIEW`），记录壳从矩阵查询取阶段证三步（空册 → 已接受尚未收寄；NO 落收寄并装袋 → 已制签或已装袋；CC 再形成尚无版本的单元 → 关务资料形成中）。
  - **不取 ADR-0124、不改 CC/NO CONTEXT**：两面读法都是既有生命周期句与硬句的直接读法（重开、替代、候选不得当归属），形状可逆、无新词条；跨上下文归属已由 ADR-0118 决定三/四定下。预留号 0124 未消费，请 MCP-1 收回。越权风险点两条，单列：(1) CC「已关闭」把重开读作未关闭依赖「一案一份关闭记录」的今日形状，模型加第二关闭期时若无人改读面，重开再关的案件会被读成未关闭——已写在端口头注；(2) NO「从未关联 → 不在」是 NO 对自己册子的解释，若日后 TF 交接入站（`EstablishedByHandoverIn`）建立不经收寄判断的关联，读面要补那一路。
  - **验证**（见完工报）：gofmt 空、build/vet 0、含 DSN 全仓 `-p 1 -count=1`、探针一正一反、清点在干净检出重生成单独成笔。
- **owner 复核 2026-09-09 认可**（用户经 IDP 队列通道 1 授权代裁，两条逐条）：(1) CC「已关闭」读法依赖一案一份关闭记录——今日形状如此，端口头注已写明第二关闭期要改读面，够了；(2) NO「从未关联 → 不在」——NO 对自己册子的解释，TF 交接入站建立不经收寄判断的关联那天补一路，届时随那张票。
