# 01 内部合规限制形成与解除没有生产入口

Category: enhancement
Status: resolved——2026-10-28 进 main（`2e24f5941`，launch-wiring 01–04 同批；全量 160 ok / 0 FAIL，种子退 0），见文末「进 main 记录」。此前 in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-lw-cc（代码最后一笔 9d06793c9）。此前：in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」），分支 claude-lw-cc
Blocked by: 无

## 做什么

把 `ManageInternalRestrictionHandler`（建立、决定）接成 `cmd/parcel-api` 的运营决定口：授予照现有运营决定种类的写法另立种类，租户与操作者取自认证信封，生产装配真库用例；演示租户授予种子照 `scripts/demo-seeds/data/access` 写法补。依据 UC-CC-011、ADR-0334（决定七、越权风险点 1）。

## 完成判据

- 持授予的运营人员经 `/v1` 能建立内部合规限制并作确认/解除决定；准入随之阻断或放开（真库用例）。
- 未授予答 403；重放答已有；租户取值不写死。


## 完成记录（2026-10-28，作者子代理，分支 claude-lw-cc）

起点 origin/main `2d0193336`；代码一笔 `9d06793c9`。

### 改了什么
- accessidentity：运营决定加 `INTERNAL_RESTRICTION_ESTABLISHMENT`、`INTERNAL_RESTRICTION_DECISION` 两种；迁移 `access_identity/0030` 重建决定种类 CHECK（取 0029 全集再加两词）。取号时 origin/main 到 0029，`origin/claude-lw-pid` 也只到 0029，未撞。
- customshttp（新文件 `internal_restriction_endpoints.go`）：`POST /customs-internal-restrictions`（建立）与 `POST /customs-internal-restriction-decisions`（确认或解除）。Intake 挂在既有 `OperatorDecisionIntake` 上，各按自己那一种决定认证；租户与决定方取认证信封，载荷封闭（自报租户、决定方、决定方式都是坏报文）。domain 补 `ParseGuardedAction`（只给新代码用，没去改另两处旧译表）。
- parcel-api（新文件）：`endpoints_internal_restrictions.go`、`assemble_internal_restrictions.go`、未接线占位；事务壳复用 `customsDecisionTransaction`，未决整笔回滚。共享装配文件（`assemble.go`、端点总表）零改动。
- 演示种子：access 26/27、governance 63/64、步骤 483（授予、准入对照、权威区间；不建任何限制）。
- UC-CC-011 状态行、UC-CC-013 实现缺口一句改成现状（内部合规限制写入面已接；监管限制仍无；决定方对来源的当前授权核验仍缺）。

### 代决与越权风险点
1. 授予粒度：建立与决定各一种（能建立的不因此能解除）。票面只说「另立种类」，粒度是代决。
2. 决定方式恒为人工决定（`ManualDecision`）：运营口的决定方就是认证出的操作者，规则化形成不经此口。
3. 授权引用（`authority`）由请求方在载荷里给，只被保存、不核验——ADR-0334 越权风险点 1 的后半（对 party-commercial 核决定方当前授权）仍未做，本票也不做。
4. 限制责任来源、依据、覆盖项、解除条件、证据全是租户取值，产品不代填、不设默认。
5. 未立新 ADR：ADR-0334 决定七已授权「新的运营决定授予与认证（ADR-0227 一路）」，本票照做。未立新 AT 号，测试 `Covers:` 引 UC-CC-011。
6. 越权风险：授权引用不核验意味着持「决定」授予的人可在载荷里填任何授权引用；领域只核「来源 = 限制的责任来源」。

### 判据对用例
- 持授予的运营人员经 `/v1` 建立限制并作确认/解除决定；准入随之阻断或放开：真库用例 `TestInternalRestrictionsAreEstablishedAndReleasedThroughTheProductionFaces` 用生产读适配器 `ListInternalByScope` 加 `JudgeInternalApplicability` 读到建立后「有效限制」、解除后放开（解除之前的动作时点仍读到有效，不回溯）、未解除的覆盖项仍待确认。
- 未授予答 403（含授予不相通：只持建立不能解除，反之亦然）；重放答已有（`EXISTING`、`DECISION_EXISTING`），同标识异内容答冲突，库里仍一份；非责任来源解除答 `NOT_RESPONSIBLE_SOURCE` 不落决定；无证据解除答 `DECISION_REJECTED`；租户取值不写死。

### 自验
`go build ./...`；`go vet` customscompliance、accessidentity、cmd/parcel-api；改过的 `.go` `gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./internal/customscompliance/...`、`./internal/accessidentity/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/parcel-api/...` 全绿；种子灌一次性库退 0 后删库；`at-coverage-counts -check` 无输出。

### 留给协调方
- 无需立票的新发现。进 main 时若另两位作者的 `access_identity/0030` 撞号，本份顺延；CHECK 的并集要把后进来的词补进去（0030 头注已写明约定）。

## 评审记录（2026-10-28，隔离子代理，钉 `f977c587c`，01–03 同一评审）

两轴无阻断。建立与决定各一种授予；决定方取自认证信封记 `DecidedBy`（ADR-0303）；授权引用只存不核、决定的 `source` 由请求方自报——ADR-0334 越权风险点 1 后半，UC 现状句如实。非阻断：`ParseGuardedAction` 与两处旧译表重复。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 与 launch-wiring 02、03、04 同批：隔离树上先 cherry-pick `claude-lw-cc`（tip `f977c587c`）、再 `claude-lw-no`（tip `272f98c15`）到 main `322b5695c`；`internal/accessidentity/operation_decision.go` 的 `decisionKinds` 文本冲突按并集解。access_identity 迁移 0030（01）、0031（04）、0032（02）、0033（03）依序施加，0033 的 CHECK 为全集（评审逐词核过）。
- 重生成 ADR 索引、AT 盘点计数与机制清点，有变化的各成一笔；两项 `-check` 退 0。
- 全量 `scripts/test-full.sh`：160 ok / 0 FAIL。种子灌一次性库退 0，用后删库。
- 推确切 SHA `2e24f5941` 到 main。
