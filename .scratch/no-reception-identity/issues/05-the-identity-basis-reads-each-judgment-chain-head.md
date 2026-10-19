# 05 方向依据的身份一格改读每条判断链的当前有效版本——不再对全部行取或

Category: enhancement
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 04 拆出（代决，用户 2026-10-16 授权「你作为业务和系统专家你来自决」）
Blocked by: 04 进 main（ADR-0210、NO CONTEXT「判断链与当前有效版本」与 UC-NO-004 先落）。不依 no-reception-source/03：本票只碰身份读口与方向依据领域件，与它的地盘不交叉，可以先行或并行。
父票：[04](./04-identity-conflicts-have-a-disposition-and-release.md)
出处：票 04 裁定里「方向依据的身份一格改读每条判断链的当前有效版本，不再对全部行取或」那一点；[ADR-0210](../../../docs/adr/0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五与「与已接受决定的关系」。
地盘：
- internal/nodeoperations/domain/directional_basis.go 及其测试（`IntakeIdentityObservation` 加前版，`JudgeIdentityBasis` 只折链头）
- internal/nodeoperations/adapters/postgres/intake_identity_view.go 及其测试（多读前版一列）
- internal/nodeoperations/application/ 下组观察的测试辅助（`judge_directional_basis_test.go`、`receive_delivered_unit_test.go`），只在观察形状变了时改
- internal/nodeoperations/adapters/http/query_node_operations_records_test.go，只在观察形状变了时改
- cmd/parcel-api/reception_one_mark_two_units_test.go、reception_one_mark_concurrent_test.go（先到那件的原因集）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（钉 main `4f11563c`，读码）

- `JudgeIdentityBasis` 对这件实物在册上的全部带收寄的行取或：任一行记身份冲突，或带关联的行指向不同的正式包裹，就判缺少·身份冲突（头注原句「有行记了身份冲突，或带关联的行指向不同的正式包裹」）。冲突行一落，这一格就再也解不开；票 06 的处置版接在冲突版后面也解不开。
- `IntakeIdentities.FindIntakeIdentities` 只读 `source_id`、`intake` 与 `identity_conflict`，不读前版。
- 收寄表的判断版本与前版两列（迁移 0007）已经在：形成与待识别两格的判断版本就是收寄结果版本，前版由库面（租户，前版）唯一守住。

## 做什么

照 ADR-0210 决定五：

1. **领域**：`IntakeIdentityObservation` 加前版（缺席即这一版不接续任何版本）。`JudgeIdentityBasis` 先挑链头：被同一组里另一行指名为前版的行不再参与；再照原折法折——一行都没有为缺少·无收寄记录，链头里有一版记冲突、或各链头的关联指向不同包裹为缺少·身份冲突，没有一版链头带关联为缺少·待识别，其余成立、所依收寄取带关联的链头里业务时间最晚的那一版。头注改写，引 ADR-0210 决定五与 NO CONTEXT「判断链与当前有效版本」。
2. **坏形状**：同一前版被组里两行指名（库面已拦，读回见到即说明有绕过编排的写入）、一行指名自己，都交回 `ErrInvalidDirectionalBasis`，编排照旧记成`读不动`。前版指向组外的行（例如续办接在一条待确认之后）不算坏形状，那一版照常是链头。
3. **读口**：`FindIntakeIdentities` 多读前版一列，经领域构造器交回。查询条件不变（只读带收寄的行），索引不变。
4. **用例**（表驱动加真库）：
   - 一码两物先到那件：原判断加一版指名它为前版的冲突版 → 缺少·{待识别, 身份冲突}；
   - 第一期之前的形状：原判断加一版没有前版的冲突版 → 缺少·身份冲突，照旧；
   - 零候选的待识别被一版带关联的重核版接续 → 成立，所依收寄是重核版；
   - 两条链，一条链头成立、一条链头记冲突 → 缺少·身份冲突；
   - 冲突版之后接一版带关联、不记冲突的版本（模拟票 06 的处置版，用构造的观察，不依票 06）→ 成立；接一版不带关联、不记冲突的版本 → 缺少·待识别；
   - 坏形状两种 → `ErrInvalidDirectionalBasis`。
   - 真库（cmd/parcel-api，经生产装配）：一码两物之后读先到那件，身份一格是缺少·{待识别, 身份冲突}，后到那件仍是缺少·{待识别, 身份冲突}（`AT-NO-061`）。既有两份一码两物用例只断言「含身份冲突」，照旧绿；若有按原因集逐个断言的用例，照新读法改钉，并在完成记录里列出。
5. **AT 点名**：`AT-NO-061`；`.scratch/at-coverage-inventory.md` 照它的扫描口径更正（UC-NO-004 新增的 `AT-NO-053` 至 `AT-NO-061` 一并入表，未钉的入②；ps-external-mark-relations/05 新增的 `AT-NO-052` 也还没入表，顺带照口径补上）。
6. ADR-0196 与 ADR-0199 的复核记录、adr/README 的标注已由票 04 写好，写明自本票进 main 起生效；本票不再改 ADR。

## 不做

- 处置命令口、处置版的写入、`FindOtherUnitIntakes` 与 `ParcelContainmentView` 的处置截断、重核的例外格（票 06）。
- 已落册的方向依据记录重判（NO CONTEXT「事后不补判、不改记」）。
- 给第一期之前没有前版的冲突版补前版（ADR-0199「不回填」）。

## 完成判据

- [ ] 领域用例：上列各格各一条，先红后绿。
- [ ] 读口用例（`adapters/postgres`）：前版读回逐字一致；没有前版的旧行读回为缺席。
- [ ] 真库用例：一码两物之后两件的身份一格各是缺少·{待识别, 身份冲突}（`AT-NO-061`）；no-reception-identity/01、03 与 no-directional-basis/01 的既有用例照旧绿，改钉的逐条列在完成记录。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
