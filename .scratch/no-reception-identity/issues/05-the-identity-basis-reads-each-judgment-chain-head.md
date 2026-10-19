# 05 方向依据的身份一格改读每条判断链的当前有效版本——不再对全部行取或

Category: enhancement
Status: resolved——2026-10-19 进 main（代码验证钉 `36960d4c`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（04 已进 main），分支 `claude-nri05`；与 no-reception-source/03 并行。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 04 拆出（代决，用户 2026-10-16 授权「你作为业务和系统专家你来自决」）
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

- [x] 领域用例：上列各格各一条，先红后绿。
- [x] 读口用例（`adapters/postgres`）：前版读回逐字一致；没有前版的旧行读回为缺席。
- [x] 真库用例：一码两物之后两件的身份一格各是缺少·{待识别, 身份冲突}（`AT-NO-061`）；no-reception-identity/01、03 与 no-directional-basis/01 的既有用例照旧绿，改钉的逐条列在完成记录。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-nri05`，基于 main `dee3a79c`）

### 落点

| 提交 | 内容 |
|---|---|
| `2be4d5ac` | 领域、读口与各层用例（下表） |
| 本笔 | AT 盘点追记八；本票完成记录 |

- `internal/nodeoperations/domain/directional_basis.go`：`IntakeIdentityObservation` 加 `Predecessor`（缺席即不接续任何版本）；`JudgeIdentityBasis` 先经新的 `judgmentChainHeads` 挑链头，再照原折法折，所依收寄只在带关联的链头里挑；头注改写，引 ADR-0210 决定五与 NO CONTEXT「判断链与当前有效版本」。
- `internal/nodeoperations/adapters/postgres/intake_identity_view.go`：`FindIntakeIdentities` 多读 `predecessor_version`，经 `versionFrom`（与收寄库读回前版同一个译法）交回；查询条件与索引不变。
- 机制清点在代码提交之后的干净工作树上重生成，与 main 无差，没有单独一笔。

### 各完成判据格对应的用例

| 格 | 用例 | 红时的现象 |
|---|---|---|
| 一码两物先到那件：冲突版指名原判断 → 缺少·{待识别, 身份冲突} | `domain` `TestIdentityBasisReadsEachJudgmentChainHead/一码两物先到那件：冲突版指名原判断为前版` | `MISSING [IDENTITY_CONFLICT]` |
| 第一期之前的形状：冲突版没有前版 → 缺少·身份冲突 | 同上 `/第一期之前的形状：冲突版没有前版` | 照旧绿（读法本就不变，钉回归） |
| 零候选待识别被带关联的重核版接续 → 成立，所依重核版 | 同上 `/零候选的待识别被带关联的重核版接续` | 照旧绿（ADR-0199 决定五的结论维持） |
| 两条链：一条成立、一条链头记冲突 → 缺少·身份冲突 | 同上 `/两条链：一条链头成立、一条链头记冲突` | 照旧绿 |
| 冲突版之后接带关联、不记冲突的版本 → 成立 | 同上 `/冲突版之后接一版带关联、不记冲突的处置版`（构造的观察，不依票 06；所依收寄须是处置版——来源标识排成被接续的原判断在同刻定序里占先） | `MISSING [IDENTITY_CONFLICT]` |
| 冲突版之后接不带关联、不记冲突的版本 → 缺少·待识别 | 同上 `/冲突版之后接一版不带关联、不记冲突的处置版` | `MISSING [IDENTITY_CONFLICT]` |
| 前版指向组外的行照常是链头 | 同上 `/前版指向组外的行` | 照旧绿 |
| 坏形状：同一前版被两行指名、一行指名自己 | `domain` `TestIdentityBasisRefusesMalformedJudgmentChains/同一前版被两行指名`、`/一行指名自己` | `err = <nil>` |
| 坏形状交到编排记`读不动` | `application` `TestDirectionalBasisUnreadableSourcesAreRecordedNotRaised/收寄册读回两行指名同一前版` | `IDENTITY answer = ESTABLISHED, want UNREADABLE` |
| 读口：前版逐字读回、没有前版的行读回缺席 | `adapters/postgres` `TestIntakeIdentitiesReadsThePredecessorOfEachVersion`（另把读回的一组交给领域，断言缺少·{待识别, 身份冲突}） | 冲突版 `Predecessor:` 为空 |
| 真库 `AT-NO-061` | `cmd/parcel-api` `TestTheWiredReceptionTurnsOneMarkOnTwoUnitsIntoAnIdentityConflict` | `SYN-UNIT-NRI01-A 的身份一格 = MISSING [IDENTITY_CONFLICT]（2 行）` |

**改钉的既有用例**（原只断言「含身份冲突」，改为逐个断言缺少·{待识别, 身份冲突}）：

- `cmd/parcel-api` `TestTheWiredReceptionTurnsOneMarkOnTwoUnitsIntoAnIdentityConflict`（票 no-reception-identity/01），头注加 `AT-NO-061` 点名；
- `cmd/parcel-api` `TestConcurrentDeliveriesOfOneMarkSerializeOnTheParcel`（票 no-reception-identity/03），红时 `SYN-UNIT-NRI03-A 的身份一格 = MISSING [IDENTITY_CONFLICT]（2 行）`。

全仓按原因集逐个断言、又会被新读法改变结论的既有用例，只有这两份；`application` 的 `TestOneMarkOnTwoUnitsFormsAnIdentityConflict` 仍只断言「含身份冲突」，照旧绿，它的观察拼装辅助 `identityReasonsOf` 补带前版（观察形状变了）。no-directional-basis/01 的既有用例（`judge_directional_basis_test.go`、`adapters/postgres/directional_basis_test.go`、`adapters/http` 目录用例）不改，照旧绿。

**ADR-0196 先到那件的原因集**：ADR-0199 第一期之前追加、没有前版的冲突版——原判断与冲突版各是链头，缺少·{身份冲突}，照旧；第一期之后追加、指名原判断的冲突版——链头只剩冲突版，缺少·{待识别, 身份冲突}。后到那件两种情形都是缺少·{待识别, 身份冲突}。

### 先红后绿

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri05-red.log`。第一轮只改测试、领域还没有前版一格，`go vet` 报 `Predecessor undefined`；第二轮领域只加这一格、折法与读口未改，上表「红时的现象」即这一轮的原话。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri05-green.log`。
- 门：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri05-gate.log`。

### 门

在 `2be4d5ac` 上：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-frontline-import/... ./internal/architecture/... ./migrations/...` 全 ok。`JudgeIdentityBasis` 与 `IntakeIdentityObservation` 的用处只在 `internal/nodeoperations` 与 `cmd/parcel-api`，没有别的上下文读它。没跑全量 `./...`。

证据层级：`S`（夹具全为合成，`SYN-` 值）。

### 越出票面的地方

- **坏形状多一种：成环。** 票面列了两种；几行互相接续成环，库面（（租户，前版）唯一与「前版不是自己」）拦不住，全在环里时会被读成「无收寄记录」、与别的链混着时会被静默略过。照收寄库 `CurrentVersion` 走到环报错的同一待遇，一并交回 `ErrInvalidDirectionalBasis`，有用例 `TestIdentityBasisRefusesMalformedJudgmentChains/成环`。
- `judge_directional_basis_test.go` 不只改辅助：在既有读不动用例里加了一个子用例，钉「坏形状由编排记`读不动`」。
- 读口用例落在 `adapters/postgres/directional_basis_test.go`：既有的身份读口用例本就在那份文件里，没有另开文件。

### 没做

- `receive_delivered_unit.go` 里 `appendIdentityConflicts` 头注「方向依据的身份一格读到这一版就落成身份冲突」没改：那份文件是并行的 no-reception-source/03 的地盘。新读法下原因集仍含身份冲突，这句没有说错，只是没写全；`ports.IntakeIdentityView` 的头注同理不动，仍然成立。
- `AT-NO-061` 后半「零候选的待识别已被重核版接续 → 成立」只有领域面：真库面要等 no-reception-source/03 的重核编排进 main。
- 「走 Go 那条路」一格只做到分支与作者自验；非作者评审与推送方在干净检出上带 DSN 跑全量未做，该格不勾。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `e248c1c3`）

两轴合审，0 阻断。

- **Spec**：ADR-0210 决定五落到。`JudgeIdentityBasis` 先由 `judgmentChainHeads` 挑出组里没有后继的各行，再照原折法折；所依收寄只在带关联的链头里挑。前版指向组外的行照常是链头；第一期之前没有前版的旧冲突行自成一条链、照旧判冲突，不回填。ADR-0196 先到那件在一期之后落成缺少·{待识别, 身份冲突}，两份既有真库用例由「含身份冲突」收紧成逐件断言，是有意的结论变更。读口只多读前版一列，查询条件与索引不变。
- **作者多出的一格认可**：成环库面拦不住，与 `CurrentVersion` 走到环同一待遇报错不挑；编排把它记成身份一格读不动，不抛。
- **Standards**：头注写为什么（取或会让冲突版一落就再也解不开）。
- **非阻断（不另立票）**：`appendIdentityConflicts` 头注「方向依据的身份一格读到这一版就落成身份冲突」没写全（新读法下原因集还含待识别），那份文件归 no-reception-source/03 在途，随它或 no-reception-identity/06 顺手改；`AT-NO-061` 的「零候选被重核版接续」真库面等 no-reception-source/03 进 main。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-nri05` 直接叠在远端 main 之上快进推送；机制清点重生成零差，没有清点那一笔。
- 推送方全量一次：钉 `36960d4c` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 144 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
