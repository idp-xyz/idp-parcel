# 43 TF 监管运输处置承接与备用/退运旅程启动没有生产入口

Category: enhancement
Status: resolved——2026-10-28 进 main（`20fc84f1d`；全量 160 ok / 0 FAIL，种子退 0），见文末「进 main 记录」。此前 in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-code43（代码最后一笔 fd46dd609）。此前 in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code43。此前 needs-triage——R3（不急）：两条用例在首发主链之外（监管退运、备用伙伴切换、普通退运都按受控异常走），今天生产上没有人能走到这两口；后果是 UC-TF-001、UC-TF-007 的结果契约与 AT 只在领域与编排测试里成立。2026-10-21 文档自洽 08 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc08。
Blocked by: 无
出处：[audit/tf.md](../audit/tf.md) M15（UC 状态行过时、UC-TF-007 没有端点）与 L8（承接「待补充」，裁定见 [ADR-0265](../../../docs/adr/0265-a-regulatory-disposition-supplement-required-is-an-answer-not-a-recorded-decision.md)）；文档侧见 [08](../issues/08-tf-docs.md)。

## 现象

- `application.AcceptRegulatoryDispositionHandler`（UC-TF-001 承接决定：三种决定，待补充与纯节点动作两格答复，一事项一决定，回执意图）与 `application.StartAlternateJourneyHandler`（UC-TF-007 与 UC-TF-001 共用的独立旅程启动）都已实现、有测试，`cmd/` 下都没有接线。
- 读一侧已经接上：派发进程把旅程启动信封投给全程追踪投影（`cmd/parcel-dispatch/routes_exception_journey.go`），实际履约发生项按旅程册挂旅程（UC-TF-005 `AT-TF-252`）。生产上没有写入方。
- 承接编排只核移动授权；协作事项本体（载运对象、移动动作、起止责任、当前有效性）不读，UC-TF-001 `AT-TF-004` 除移动授权以外的几种缺口今天走不到待补充。
- 关务协作事项没有交进本上下文的路；反方向的承接回执信封 `transport-fulfillment.regulatory-acceptance.recorded` 也没有消费者（`.scratch/outbox-handoff-consumption-map/report.md` 里 `RegulatoryAcceptanceHandoff` 那一行记作「应有但未开」）。

## 三类

- 机制：两口的命令面与装配；承接编排读协作事项并核对象、动作、起止责任与当前有效性，不足时照 ADR-0265 答待补充、不落决定；回执交关务协作链的消费者。
- 产品策略：两口各归哪一族、哪一种决定（按 ADR-0151 定族判据；承接与旅程启动都是运营人员在管理台上作的生产决定，按此应归操作者渠道「运营决定」能力面，接线时立 ADR 定）。
- 租户取值：谁被授予、退运路径、备用伙伴、责任与证据（`PAR-NET-10`、`PAR-NET-11`、`PAR-CUS-01..05`），留空答`未配置`，演示租户以 `SYN-` 登记、证据记 `S`。

## 规模

M–L：两口命令面、装配、授予种类、协作事项读口（CC 一侧的提供方用例可能也要补），加 UC-TF-001、UC-TF-007 各 AT 的生产装配用例。

## 最小的诚实修法

1. 先立 ADR 定两口的族与决定种类，照 ADR-0151 与 ADR-0149 的判据。
2. 承接口接上 `AcceptRegulatoryDispositionHandler`；协作事项按引用向 CC 读（消费侧适配器，ADR-0025），对象、动作、起止责任、当前有效性不足时答待补充，答复带一个原因、指名缺哪一项（缺移动授权、缺对象、缺动作、缺起止责任，ADR-0265 决定四）；关务侧的消费者把待补充记进自己的协作事项状态（ADR-0265 越权风险点 1）。
3. 旅程启动口接上 `StartAlternateJourneyHandler`。
4. 两口都记提交操作者，照票 demo-chain-gaps/126 的口径。
5. 落地时改 UC-TF-001、UC-TF-007 的状态行与「给开发的交接」，补生产装配用例的 Covers。

## 要裁的

- 两口归哪一族、几种决定（见「三类」产品策略一条）。
- 关务协作事项交进来的形态：关务推一封信封，还是本上下文按事项引用去读。

## 完成记录（2026-10-28，Claude Code 作者子代理，会话 tops-20 派单，分支 `claude-code43`）

起点：`origin/main` `2afdd0b1e`，中途重放到 `282af38da`（含 code/37、code/27、psb22 已进 main 的部分）。

| 提交 | 内容 |
|---|---|
| WIP 一笔 | 底层：端口记录加操作者格、TF 迁移 0056、适配器读写、承接核对与旅程启动带操作者、tfhttp 载荷与端点、授予种类与迁移 access_identity/0029 |
| `fd46dd609` | `cmd/parcel-api` 装配与端点特性文件、各层用例、演示种子一节、ADR-0337、ADR-0265 前向指针、UC-TF-001/007 现行化、`AT-TF-367`–`370` 与 AT 盘点 |
| 完成记录那一笔 | 本节与 Status |

### 做了什么

- 两口接上生产入口，各挂「运营决定」能力面一种决定：`POST /transport-fulfillment-regulatory-disposition-acceptances`（`REGULATORY_DISPOSITION_ACCEPTANCE`）、`POST /transport-fulfillment-alternate-journey-starts`（`ALTERNATE_JOURNEY_START`）。事务壳对`未决`整笔回滚；承接回执与旅程双链意图同笔入 Outbox。
- 承接口核协作事项本体：载荷带 `itemObjects`、`movementAction`、`origin`、`destination`，缺动作、缺对象、缺起止责任答带原因的待补充（加上领域门的缺移动授权共四值），承接对象不得超出事项指名的对象；动作与起止都空仍是纯节点动作。
- 两口都记提交操作者（TF 迁移 0056 的 `decided_by`、`started_by`），待遇照 ADR-0303。
- 演示租户授予、准入对照、权威区间一节（`steps/482`，数据 access 24/25、governance 61/62），灌一次性库验过。

### 代决与越权风险点（ADR-0337 文末同号）

1. **事项本体由提交方抄录，不读关务**（要裁的第二问）。关务一侧没有协作事项聚合、读口或出站信封，两种形态都无对端；抄录是持授予者的断言，不是读回的事实。待 TF、CC owner。
2. **当前有效性不核**，没有来源。
3. **旅程启动口不核监管来路旅程有对应承接决定**，成员也不核在已承接对象内；最小补法是按依据引用查承接册。本票未做，待 TF owner 另定。
4. 关务一侧记住等补事项、消费承接回执两半仍未建（ADR-0265 越权风险点 1、4）。
5. 两种决定分开授予（要裁的第一问）；owner 若要合并为一种须新记录。
6. 取号：ADR-0337、access_identity 0029、TF 0056、种子 24/25/61/62/482 取自 `282af38da` 后的下一个空号。

### 判据 → 用例

| 判据 | 用例 |
|---|---|
| 承接口生产入口、授予、落决定与回执意图、重放、冲突（`AT-TF-367`） | `cmd/parcel-api/assemble_regulatory_disposition_and_alternate_journey_test.go::TestTheRegulatoryDispositionAcceptanceFaceRecordsTheDecisionAndItsOperatorAgainstARealDatabase`；传输面 `adapters/http/regulatory_disposition_and_alternate_journey_test.go`；授予往返 `accessidentity/adapters/postgres/regulatory_disposition_and_alternate_journey_grant_test.go` |
| 待补充带原因、承接不超出事项（`AT-TF-368`） | `application/accept_regulatory_disposition_test.go::TestAnIncompleteCollaborationItemIsSupplementRequiredNamingWhatIsMissing`、`::TestAnAcceptanceCannotWidenTheItemsObjectScope`；真库同上一条 |
| 旅程启动口生产入口（`AT-TF-369`） | `cmd/parcel-api/...::TestTheAlternateJourneyStartFaceRecordsTheJourneyAndItsOperatorAgainstARealDatabase` |
| 操作者只进审计（`AT-TF-370`） | 两条编排用例、`adapters/postgres/regulatory_and_journey_operator_evidence_test.go`、两条真库用例 |

### 自验

含 PG（`127.0.0.1:55432`）：`go build ./...`；`go vet` 受影响包；`gofmt -l` 无输出；`go test -count=1 -p 1` 跑 `./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/...` 全绿；种子灌一次性库（建库、`seed.sh`、删库）零报错，两种授予读回。`go run ./tools/adr-index -check`、`at-coverage-counts -check` 只报生成物过期。

### 留给协调方

- 进 main 时重生成 ADR 索引与 AT 盘点计数；ADR-0337、迁移 0029/0056 与种子 482 号如被并行分支占用由推送方重排。
- 越权风险点 3（监管来路旅程不核承接决定）是否挡上线，请协调方记入冻结清单：持旅程启动授予者可开监管来路旅程，流向关务处置执行链。

## 评审记录（2026-10-28，隔离子代理，钉 `e8fe09fe4`）

- 两轴无阻断。两口授予（access_identity 0029 为 0028 全集加两词）、身份取自认证信封、幂等与冲突、未决整笔回滚、回执与双链意图同笔入 Outbox、只有落决定才 201，均核过并有真库用例。
- 协作事项由运营人员抄录、承接口核它：不越 UC-TF-001 边界（CC 无读口），ADR-0337 越权风险点 1、2 写明，撤下条件清楚；ADR-0265 部分停用写法合规。
- 作者自报缺口：旅程启动口不核监管来路旅程有对应承接决定、成员不核在已承接对象内——与 UC-TF-001 结果契约及 UC-TF-007「监管退运由 UC-TF-001 承接、不能经本用例绕过」相抵，但 TF CONTEXT 无硬不变量；须单独授予才走得到、操作者落 `started_by`、下游 CC 消费者未建，评为不挡上线，记入冻结清单。最小补法 S–M（启动编排读承接册），规模取决于 TF owner 定旅程依据引用与协作事项引用是否同一。
- 非阻断：UC-TF-001 AT 表 367、368、370 插在 011 与 012 之间，次序略乱；「当前有效性不核」只在文档披露。已发布迁移无改动；种子只给演示租户。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-code43` tip `e8fe09fe4` 作出处；在隔离树上 cherry-pick 到 main `10fc66c1c`，无冲突。
- 重生成 ADR 索引、AT 盘点计数与机制清点，有变化的各成一笔；两项 `-check` 退 0。
- 全量 `scripts/test-full.sh`：160 ok / 0 FAIL。种子灌一次性库退 0，用后删库。
- 推确切 SHA `20fc84f1d` 到 main。

