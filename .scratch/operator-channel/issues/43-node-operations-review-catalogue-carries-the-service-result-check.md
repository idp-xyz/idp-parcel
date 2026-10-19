# 43 节点作业复核目录透出收寄的服务结果核对态，让「读不动」与「已查、无标记」在查阅答复里分得开

Category: enhancement
Status: in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（两道 Blocked by 都已进 main：operator-channel/29、no-directional-basis/01 偏离标记半边），分支 `claude-oc43`。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 2 评审遗留；用户授权代决）
Blocked by: operator-channel/29 进 main；no-directional-basis/01 偏离标记半边进 main（同改复核目录三份文件）
父票：operator-channel/29（收寄遇依赖读不动的主修复票）。
出处：
- 簇 C1 终裁 C1-Q2：「读不动」在人看得到的每一处都要可区分；
- 业务后果核验：审阅者看到的是「无服务标记」，如果那件其实已取消，就有误放风险。
地盘：
- internal/nodeoperations/ports/catalogue_read.go
- internal/nodeoperations/adapters/postgres/review_catalogue.go
- internal/nodeoperations/adapters/postgres/review_catalogue_test.go
- internal/nodeoperations/adapters/http/query_node_operations_records.go
- internal/nodeoperations/adapters/http/query_node_operations_records_test.go
- docs/product/MECHANISM-INVENTORY.md（如有变化则重生成）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

## 为什么

operator-channel/29 落地后，服务结果读不动的那次收寄会落成 INTAKE_FORMED：标记为空，核对态为「读不动」。

复核目录今天只转写 service_markers。这样一来，在查阅答复里，这一行与「已查、无取消、无终局」逐字相同。终裁探针实测过：原型不加核对态时，读不动那一行和可读基线那一行，在库里都是 markers=[]。

今天没有方向性门禁：no-directional-basis/01 的读面随一线作业端排期。所以复核页是运营唯一看得到限制的地方。

## 裁定

- ReceptionCatalogueRow 照行转写核对态，不重建领域对象，不形成判断。
- 只有 INTAKE_FORMED 行带这一格；PENDING_IDENTIFICATION 行不带。
- 节点作业查阅的答复，用同名字段透出。

## 做什么

- ports/catalogue_read.go：ReceptionCatalogueRow 加核对态字段，补头注。
- adapters/postgres/review_catalogue.go：查询带出这一列。
- adapters/http/query_node_operations_records.go：答复透出这一格。
- review_catalogue_test.go 与 query_node_operations_records_test.go 里用了词表外的 CANCELLED_BEFORE_ARRIVAL，改成由 ports 合成入口产出的取值。

## 不做

- 不改管理台，归operator-channel/44。
- 不改查阅 Intake，归 operator-channel/35。
- 不改收寄写侧，那在operator-channel/29 里。

## 完成判据

- [x] 真库：「已查、有标记」「已查、无标记」「读不动」三种行，转写结果各不相同；待识别行不带核对态。
- [x] HTTP 查阅用例：这一格在场、缺席，各有一例。
- [x] go build ./...、go vet ./... 退 0，gofmt -l . 无输出；带 DSN 跑 internal/nodeoperations/...、cmd/parcel-api、internal/architecture 全绿；机制清点没有未提交的差异。
- [ ] 走 Go 那条路（docs/agents/parallel-sessions.md）。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-oc43）

基线 main `04c6a71d`（含 operator-channel/29 的核对态列与 no-directional-basis/01 偏离标记半边）。

**落点**：`237bee61` 一笔，五份文件都在地盘内。
- `ports/catalogue_read.go`：`ReceptionCatalogueRow.ServiceResultCheck`，类型取写侧同一个枚举 `ports.ServiceResultCheck`，不另立一份串；头注写明只有形成行带、为什么不像 `Kind` 那样以串转写、仍是照行转写不形成判断。
- `adapters/postgres/review_catalogue.go`：`ListReceptions` 带出 `service_result_check`，经同包 `serviceResultCheckFrom`（按键读回 `FindByKey` 用的那一个）解码；集外取值让整页答不出，不折成「不带」。
- `adapters/http/query_node_operations_records.go`：收寄册每行透出 `serviceResultCheck`，取值 `CHECKED` / `UNREADABLE`（`ports.ServiceResultCheck.String()`），与收寄写侧答复 `receptionResponse` 同名同值；`omitempty`，待识别行整键不出现（不是空串、不是 null）。头注写了理由：同名同值让设备与管理台读同一个词；缺席表达「不适用」是本册 `controlReleased*` 与待识别册 `association` 的惯例；封闭词不折成布尔，理由同 `kind`。

**完成判据逐格**：
- 真库三种行各不相同、待识别不带：`TestReviewCatalogueTranscribesTheServiceResultCheck`（`internal/nodeoperations/adapters/postgres`）。四行经写侧适配器真落库：已查有标记、已查无标记（`ports.ParcelServiceResults{}.Markers()`）、读不动、待识别；逐行比标记与核对态，并断言四行的（kind，标记，核对态）两两不同形。
- HTTP 在场、缺席各一例：`TestReceptionRegistryCarriesTheServiceResultCheckOnFormedRowsOnly`。三条形成行分别答 `"CHECKED"`、`"CHECKED"`、`"UNREADABLE"`，算在场；已查无标记与读不动两行的 `serviceMarkers` 同为 `[]`；待识别行没有 `serviceResultCheck` 键，算缺席。另改 `TestReceptionRegistryTranscribesRows`：形成行的标记由 `ports.ParcelServiceResults{Cancellation: "SYN-CANCEL-1"}.Markers()` 合成，期望值由同一组标记编码得出，不再手写 `CANCELLED_BEFORE_ARRIVAL`。
- 「做什么」第 4 条：`CANCELLED_BEFORE_ARRIVAL` 字面量只在 `query_node_operations_records_test.go`，已改。`review_catalogue_test.go` 里没有这个字面量，它的标记来自共用夹具 `formedRecord`；那个夹具在 operator-channel/29 已经改用合成入口。新用例照样显式用合成入口给标记。

**先红后绿**：日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc43-red.log`（绿：同目录 `oc43-green.log`；门：同目录 `oc43-gate.log`；会话临时目录，不入库）。红分两段：
- 红 1，只写了用例：编译失败，`ReceptionCatalogueRow has no field or method ServiceResultCheck`。
- 红 2，只给端口加了字段、两个适配器都没改：落在行为上。真库用例报 `scan-3 与 scan-2 转写同形：INTAKE_FORMED|[]|`，三条形成行的核对态都转写成 `""`。HTTP 用例报三条形成行的 `serviceResultCheck` 都是「在场 false」。

随后改两个适配器，三条用例 `-v` 下都是 `PASS`。真库那条是 PASS，不是 SKIP。

**门**（钉 `237bee61`）：改动的 `.go` 与 `gofmt -l .` 均无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/nodeoperations/... ./cmd/parcel-api/... ./internal/architecture/...`，10 个有测试的包全 ok，0 FAIL。机制清点在提交后的干净树上重生成，零差异，没有另提一笔。没跑全仓全量，留给推送方。

**证据层级**：`S`（隔离合成，夹具用 `SYN-` 值）。

**偏离与说明**：
- 迁移 0006 的 CHECK 不拦「INTAKE_FORMED 不带核对态」，这是 operator-channel/29 评审的非阻断 (1)。万一有这样的行，目录与答复会照实转写成「不带」，与待识别行同形。生产写方恒带，既有行已回填，本票不另守。
- 集外取值那条错误路径没写用例：库面 CHECK 拦住了集外取值，要造这样的行得先拆约束，越出本票。

**越出地盘的改动**：无。

**未做**：
- 「走 Go 那条路」只做了作者自验那半。非作者评审、推送方在干净检出上带 DSN 跑全量，都待后续。
- 管理台展示归 operator-channel/44，本票未碰 `apps/admin-web`。

评审：待评审。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `acf42f95`）

两轴合审，0 阻断。

- **Spec**：裁定三条落到。目录行用写侧同一个枚举 `ports.ServiceResultCheck`，PG 读面复用 `serviceResultCheckFrom`，词表一处定义；只有 INTAKE_FORMED 行带，待识别行 `String()` 为空、`omitempty` 下整键缺席；HTTP 字段与写侧 `receptionResponse` 同名同值。真库四行两两不同形，HTTP 在场与缺席各一例。
- **Standards**：头注只写为什么（不折成布尔、缺席表不适用的惯例出处）；集外取值整页报错不折成「不带」，与按键读回同一纪律。
- **非阻断（不另立票）**：集外取值那条错误路径没有用例——库面 CHECK 先拦住了，要造只能拆约束，作者已记。INTAKE_FORMED 行核对态为 NULL 时会与待识别行同形，是 operator-channel/29 评审已记的非阻断，生产写方恒写、存量已回填。
- **与 no-reception-identity/01 的交叠**：ADR-0196 追加的冲突版是待识别行，不带核对态，目录上照常缺席。
