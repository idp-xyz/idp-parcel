# 04 第四期：同一作业实物在同一节点已有当前收寄时并入、不另起控制起点；仅扫描的待确认由后来的明确接收或拒收续办

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（02、03 进 main 后开工），分支 `claude-nrs04`，NO 迁移号 0009；与 no-reception-identity/06 并行。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）
Blocked by: 03 进 main。功能上只依 02（来源文档给出待确认与拒收行的作业实物，前版机制给出续办）；与 03 同改收寄编排、PG 适配器与迁移号，按票号串行落。
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「四、同一作业实物重复控制起点的挡法」与「同一作业实物按什么认」「仅扫描的待确认怎样续办」两问；ADR-0199 决定四。落地次序排在第三期（票 05）之前：翻转的前提里有一项就是本票的挡法。
地盘：
- internal/nodeoperations/application/receive_delivered_unit.go 及其测试
- internal/nodeoperations/ports/ports.go（记录种类加「并入已有收寄」、并入版本）
- internal/nodeoperations/adapters/postgres/reception.go 及其测试（按作业实物的咨询锁、找当前收寄）
- internal/nodeoperations/adapters/postgres/ 下读收寄册的各面：review_catalogue.go、parcel_containment_view.go、intake_identity_view.go 及其测试（只在需要按种类筛时改）
- migrations/node_operations/（记录种类 CHECK 扩一种、并入版本列、按（租户，作业实物，节点）的索引）
- internal/nodeoperations/adapters/http/receive_delivered_unit.go 及其测试
- cmd/parcel-frontline-import/intake_import.go
- cmd/parcel-api 下新起真库用例文件（并发用例另起一份）
- docs/adr/0196-one-mark-on-two-units-is-an-identity-conflict-judged-by-the-associated-parcel.md（文末追加复核记录，决定原文不动）
- docs/adr/README.md（0196 一行标部分停用）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（钉 main `91ea0883`，读码；核验探针在 `8d22729f` 实测过）

- 收寄键只有（租户，来源）。同一作业实物换个来源重扫，形成又一条收寄、又一个控制起点；探针实测同一作业实物两行、两个控制起点。UC-NO-002「一致性、幂等与并发」：「同一实物同一客户交付边界只能形成一个当前有效节点收寄判断和一个控制起点」。
- ADR-0196 决定一明说「同一件实物换个来源再收一次照常形成收寄」，用例 `TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict` 钉的正是这一格；ADR-0196「能力边界」里冲突形成之后先到那件换来源再收照常答`节点收寄已形成`、交发布意图。
- 仅扫描的`收寄判断待确认`之后来了明确接收或拒收，两条判断互不相认，待确认没有续办。

## 做什么

照 ADR-0199 决定四：

1. **按（租户，作业实物）串行**：认同一实物之前取事务级咨询锁，所有分支都取；唯一识别那一支随后再取（租户，包裹）锁，次序固定先实物后包裹。键的派生与 no-reception-identity/03 的锁键口径对齐。
2. **认当前收寄**：同一实物在同一节点，有一条判断链的当前有效版本是形成或待识别、且控制没有转出。
3. **并入**：有当前收寄时，三种观察都落一行「并入已有收寄」（来源文档照存、指名并入的那一版、不带收寄与控制、不交意图），答 `EXISTING_RESULT`，`recordedOutcome` / `currentVersion` 是那一版的。
4. **续办**：当前有效版本是待确认时，后来的明确接收、拒收或又一次仅扫描照常形成新判断，以那条待确认为前版；待确认原行不改。`节点收寄未形成`之后的再次交付不指名前版。
5. **读面不误读**：复核目录、方向依据身份一格的读口、`ParcelContainmentView`、`FindOtherUnitIntakes` 逐面核过「并入」行：不带收寄与控制的行本就读不到的，用例钉住；要按种类筛的，改。
6. **改用例**：`TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict` 里「同一件实物换个来源再收一次照常形成收寄」那一段，改钉并入（同一节点）；另补不同节点照常形成收寄。冲突形成之后先到那件换来源再收，改钉并入到那件的冲突版。
7. **ADR-0196 复核记录**：决定一末句与「能力边界」那一句自本票落地起部分停用，范围只限同一节点上有当前收寄的情形（ADR-0199 决定四）；adr/README 的 0196 一行加「部分停用」注，照 0184 一行的写法。
8. **AT 点名**：`AT-NO-049`、`AT-NO-050`。

## 不做

- 控制转出的生产接线（`ControlTransferAdapter.TransferOut` 今天只有测试调用；ADR-0199 能力边界：转出接上之前，离开节点再回来的实物会被并入旧收寄）。
- 作业实物标识在租户内是否唯一的核验（ADR-0199 越权风险点）。
- 身份核对读不动的翻转（票 05）。

## 完成判据

- [ ] 编排用例：同一实物在同一节点已有当前收寄（形成、待识别各一），换来源再来明确接收、明确拒收、仅扫描：各落一行并入、不建控制、不交意图，答已有结果带那一版的种类与当前有效版本，册上只有一个控制起点（`AT-NO-049`）。
- [ ] 编排用例：不同节点照常形成收寄；`节点收寄未形成`之后再来照常形成判断、不指名前版。
- [ ] 编排用例：仅扫描待确认之后的明确接收与拒收各一条，新判断以待确认为前版，待确认原行逐字不变；重投待确认的来源答已有结果，`recordedOutcome` 是 `RECEPTION_UNDECIDED`、`currentVersion` 是新判断的版本（`AT-NO-050`）。
- [ ] `TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict` 已按上面改钉；no-reception-identity/01、03 其余用例照旧绿。
- [ ] 真库并发用例（照 no-reception-identity/03 的做法，池外独立连接看 `pg_stat_activity` 的等锁，带超时）：同一实物两个来源两条连接并发送站，后到那次等在实物锁上，前者提交后它答已有结果，册上只有一个控制起点；与 03 的同包裹并发用例同跑不死锁。
- [ ] 读面用例：复核目录、方向依据身份一格、`ParcelContainmentView`、`FindOtherUnitIntakes` 各一条，「并入」行不被读成收寄或关联。
- [ ] ADR-0196 文末复核记录已追加、adr/README 0196 一行已加部分停用注；`git diff` 对 ADR-0196 只有追加行。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
