# 43 节点作业复核目录透出收寄的服务结果核对态，让「读不动」与「已查、无标记」在查阅答复里分得开

Category: enhancement
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 2 评审遗留；用户授权代决）
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

- [ ] 真库：「已查、有标记」「已查、无标记」「读不动」三种行，转写结果各不相同；待识别行不带核对态。
- [ ] HTTP 查阅用例：这一格在场、缺席，各有一例。
- [ ] go build ./...、go vet ./... 退 0，gofmt -l . 无输出；带 DSN 跑 internal/nodeoperations/...、cmd/parcel-api、internal/architecture 全绿；机制清点没有未提交的差异。
- [ ] 走 Go 那条路（docs/agents/parallel-sessions.md）。
