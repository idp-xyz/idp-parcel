# 05 第三期：身份核对读不动翻成`待识别实物已建立` + 身份核对态「未核对」——ADR-0186 决定二（草案 C1-T1）身份支的 5xx 由本票翻转

Category: enhancement
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）；开工前先看下面 Blocked by 里那项用户决定
Blocked by:
- 04 进 main（前提「挡住同一作业实物重复形成控制起点」）；02、03 经它已在 main。
- 用户决定：ADR-0199 越权风险点「重核只有信封与人工两个触发」那一条——PS 读不动期间落下的未核对行，若号码的关联早在读不动之前就已形成，由什么续上：落未核对行的同一笔事务给自己投一封重核意图 / 同一来源重投时对未核对行重核 / 明确接受只靠人工。2026-10-19 未裁。
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「三、身份读不动翻成『待识别 + 未核对』」与「核对态要不要放进设备响应」那一问；ADR-0199 决定五。票 01「做什么」点名的三条真库用例归本票。
地盘：
- internal/nodeoperations/application/receive_delivered_unit.go 及其测试
- internal/nodeoperations/ports/ports.go（身份核对态的封闭枚举）、ports/catalogue_read.go
- internal/nodeoperations/adapters/postgres/reception.go、review_catalogue.go、parcel_containment_view.go 及其测试；身份读口的 savepoint 装饰器（与 service_result_savepoint.go 同形，新文件或同文件由实施定）
- migrations/node_operations/（身份核对态一列：封闭两值、只有待识别带；既有待识别行回填「已核对」，推定依据写迁移头注）
- internal/nodeoperations/adapters/http/receive_delivered_unit.go、query_node_operations_records.go 及其测试
- cmd/parcel-api/assemble_reception.go、cmd/parcel-frontline-import/intake_import.go（两处装配给身份读口包 savepoint；前线导入明细）
- cmd/parcel-api 下新起真库用例文件
- apps/admin-web/src/pages/operations/**（待识别实物页显示身份核对态）
- docs/adr/0186-node-reception-records-physical-facts-when-the-service-result-read-fails.md（文末追加复核记录，决定原文不动）
- docs/adr/README.md（0186 一行标部分停用）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 翻转前提今天各自齐不齐（钉 main `91ea0883`）

ADR-0199 决定五列的前提，缺一个都不翻：

| 前提 | 今天 | 卡在 |
|---|---|---|
| 来源已保全 | 不齐。收寄记录没有号码格，`Handle` 丢弃规范化文档 | 票 02 |
| 「未核对」处处可见（设备答复、前线导入明细、复核目录与管理台、`ParcelContainmentView`） | 不齐。核对态只有服务结果一格（ADR-0186；复核目录由 operator-channel/43、管理台由 operator-channel/44 透出），身份核对态不存在；`ParcelContainmentView` 对未核对在袋件的答法 ADR-0199 已定、未实现 | 本票自带，与翻转同票落地，不单独先落 |
| NO 主动重核 | 不齐。没有重核机制，`AT-NO-026` 是机制候选；且「关联早于读不动形成」那一格在裁定的触发下只能人工续上 | 票 03；那一格另卡在上面的用户决定 |
| 挡住同一作业实物重复形成控制起点，不只限于身份这一支 | 不齐。收寄键只有（租户，来源），同一实物换来源再收又起一个控制起点；no-reception-identity/03 的锁只串行同一包裹的唯一识别那一支 | 票 04 |

## 做什么

照 ADR-0199 决定五：

1. **编排**：身份核对读口交回错误时，落`待识别实物已建立`：收寄与控制照常、来源已保全、候选为空、身份核对态「未核对」，不问服务结果、不交意图；HTTP 首答 201。同一实物在同一节点已有当前收寄时照票 04 并入，不因身份读不动绕过。
2. **身份读口经 savepoint 隔离**：两处生产装配都包，ctx 不在事务里时直通，写法同 `SavepointedServiceResultView`；`transactionalReception` 与前线导入装配的头注随之改写。
3. **存储**：身份核对态一列，封闭 `CHECKED` / `UNCHECKED`，只有待识别行带；既有待识别行回填 `CHECKED`，迁移头注写推定依据（翻转前身份读不动从不落行：ADR-0186 决定二；ADR-0186 之前读不动答零行的待确认：票 operator-channel/29 现象；ADR-0196 的冲突版来自读口答复了的那次送站），写法同迁移 0006。
4. **处处可见**：HTTP 答复带 `identityCheck`（信息格，不进 outcome 词表，首答与重放都带）；前线导入明细写「身份未核对：候选缺席不代表查无此号」，这一行照常记成功、不再记未决；复核目录待识别行带核对态；管理台待识别实物页显示；`ParcelContainmentView` 在同租户有未核对在袋件时，对没有已关联在袋实物的包裹答`不可归属`。
5. **重核接上未核对**：票 03 的重核对象本就含不带关联、不记冲突的待识别；本票补「未核对 → 零候选」时追加「待识别 · 已核对」那一格的用例；按上面那项用户决定接上自动续办（若决定是只靠人工，写进完成记录）。
6. **改随新答法变了的用例与注释**：ADR-0186 名下「身份核对读不动不形成答案」的用例（`TestDependencyFailuresFormNoAnswer` 里身份核对那一格，HTTP 与前线导入两侧的对应用例）改钉新答法；`ReceiveDeliveredUnitHandler.Handle` 与 `receive` 头注里「身份核对读不动…交回 error」改写。
7. **ADR-0186 复核记录**：决定二「身份核对读不动：不形成答案」与决定七「身份读口不隔离」自本票落地起部分停用（ADR-0199 决定五），决定二里「不拿空候选顶替」照旧、由核对态兑现；adr/README 的 0186 一行加「部分停用」注。
8. **AT 点名**：`AT-NO-051`。

## 不做

- 服务结果读不动那一支（ADR-0186 决定一）、拒收与仅扫描、零候选、多候选、身份唯一可读各支的 outcome。
- 方向依据代码：未核对行照待识别折（缺少·待识别），不另立原因（ADR-0199 决定五）。
- 一线作业端（票 01「不做」）。

## 完成判据

- [ ] 真库用例一（经生产装配）：PS 身份读口读不动期间，同一来源重投，收寄不多出一条：答已有结果，`recordedOutcome` 是 `PENDING_IDENTIFICATION`、`identityCheck` 是 `UNCHECKED`，册上一行、一个控制起点、意图为零。
- [ ] 真库用例二：PS 恢复后，重核落下关联，意图一封，业务时间是原收寄时刻。触发照上面那项用户决定；决定之前至少经人工发起跑通。
- [ ] 真库用例三：换来源重扫同一作业实物，不再形成第二个控制起点（并入未核对的那一条，答已有结果）。
- [ ] savepoint 用例：身份读口报 SQL 级错误时，收寄那笔事务照常提交、未核对行落下；ctx 不在事务里时直通（照 `service_result_savepoint_test` 的做法）。
- [ ] 零候选（`CHECKED`）与未核对（`UNCHECKED`）在 HTTP 答复、前线导入明细、复核目录、管理台上分得开；`ParcelContainmentView` 对未核对在袋件答`不可归属`，对已关联在袋的包裹仍答`在`（`AT-NO-051`）。
- [ ] 其余各支 outcome 不变，既有用例照旧绿。
- [ ] ADR-0186 文末复核记录已追加、adr/README 0186 一行已加部分停用注；`git diff` 对 ADR-0186 只有追加行。
- [ ] admin-web 三道门退 0（口径见 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境)）。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
