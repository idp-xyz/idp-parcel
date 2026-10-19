# 01 同一外部条码出现在两件实物：第二件也唯一关联到同一包裹，不形成身份冲突（AT-NO-019 字面场景没有实现）

Category: bug
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 0 研判）
Blocked by: operator-channel/29 已进 main
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C2 第三张草案（AT-NO-019）
地盘：
- internal/nodeoperations/application/receive_delivered_unit.go
- internal/nodeoperations/application/receive_delivered_unit_test.go
- internal/nodeoperations/ports/ports.go
- internal/nodeoperations/adapters/postgres/reception.go
- internal/nodeoperations/adapters/postgres/reception_test.go
- cmd/parcel-api/（新的真库用例文件，不与 assemble_reception_identity_test.go 同文件）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 出处

票 ps-external-mark-relations/02 评审遗留（operator-channel/26 序 0）研判时，权威一致性核验发现的漏项。会话 `idp-parcel-9b`，钉 main `8d22729f`；本会话复跑探针证实。

## 现象

真库实测（经生产装配 `buildReceptionOrchestration`，合成夹具，证据只记 `S`）：某个号在册里只标识一件包裹。两件不同的作业实物（作业实物标识不同）先后贴着这个号送站，两件都答`节点收寄已形成`，都关联到同一件包裹，各自建立控制，都不记身份冲突。

落点：

- `ReceiveDeliveredUnitHandler.receive` 只按候选数分支，恰一个就关联。`default` 分支的注释写「一码多物或多候选」，实际只覆盖了多候选。
- `receive_delivered_unit_test.go` 的 `TestUnknownAndConflictedIdentitiesStayPending` 头注点名了 `AT-NO-019`，但它的 `conflicting candidates` 子用例测的是一件实物两个候选。AT 覆盖盘点据此把 `AT-NO-019` 记成了已钉住。

## 为什么是缺陷

NO CONTEXT「实物身份与事实完整性」原话：「同一外部条码出现在多个实物，或同一实物存在相互冲突的外部标识时，各作业实物继续独立，形成身份冲突并暂停方向性作业。」UC-NO-002 验收 `AT-NO-019` 也写着：「两件实物保持独立，形成身份冲突并暂停方向性作业」。

它还与 ps-external-mark-relations/05 的唯一误关联叠加：真主人那件实物后到，也认不出冲突。

no-directional-basis/01 当前裁定的身份判断是「恰好关联到一个正式包裹、且没有一行记身份冲突，为成立」。这一格不补，两件实物都会被判成身份成立。

## 要裁的问（NO owner；难逆转的进 ADR）

1. 「同一外部条码出现在多个实物」怎么认？同租户内，同一件包裹已有另一件作业实物的当前收寄关联，算不算？按号码认，还是按关联到的包裹认？要不要限于先到那件仍在节点控制下？同一作业实物的重投是重放，不在此列。
2. 先到那件怎么办？它的收寄已形成，可能已被 PS 采用。在只追加、不覆盖的约束下，是给它补一版身份冲突判断，还是只让后到那件答冲突、交 VE 协调？
3. 与方向性作业门（no-directional-basis/01）怎么衔接？

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

按关联到的包裹认「同一外部条码出现在多个实物」：同租户内这件包裹已有另一件作业实物的当前客户送站收寄，后到那件答身份冲突、建待识别实物，不论先到那件是否仍在节点控制下；同一作业实物的重投仍是重放。先到那件只追加一版身份冲突判断，原判断照留（只追加、不覆盖）；两件在 no-directional-basis/01 的身份判断里都落成「身份冲突」。已被 PS 采用的那次采用不回滚，交 VE 协调。难逆转，出 NO ADR（用号 0196，会话已占）。实施排在 operator-channel/29 之后（同改 receive_delivered_unit.go）。

## 做什么（裁后）

- 照裁定在收寄判断里认出一码两物并答身份冲突；原观察与原判断照留。
- `receive` 里 `default` 分支的注释照实写。
- `TestUnknownAndConflictedIdentitiesStayPending` 的 `AT-NO-019` 点名，改成只认它实际钉住的那一半（照 AT 覆盖盘点「半边覆盖在点名处注明」的口径）。新用例按 `AT-NO-019` 的字面场景点名；AT 覆盖盘点按它的产出方式更正。

## 不做

- 唯一误关联（ps-external-mark-relations/05）、方向性作业门本身（no-directional-basis/01）、身份后来被确认（`AT-NO-026`）。

## 完成判据

- [ ] 要裁的问已裁，写进本票；难逆转的进 ADR。
- [ ] 一码两物按裁定答身份冲突。要有编排层用例，外加一条经生产装配的真库用例（另起测试文件，不与 ps-external-mark-relations/04 新建的 `assemble_reception_identity_test.go` 同文件）；零候选、多候选、唯一命中的既有用例照旧绿。
- [ ] `AT-NO-019` 的点名与实际覆盖一致。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`internal/architecture` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
