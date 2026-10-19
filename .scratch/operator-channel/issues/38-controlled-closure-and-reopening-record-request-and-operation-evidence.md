# 38 受控关闭与重开的请求证据与操作证据落在决定上（新 ADR 部分停用 ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项的半句）

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-oc38`）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 1 评审遗留；用户授权代决）
Blocked by: 无（operator-channel/37 已进 main）
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)。被评的是 [15](./15-operation-decision-faces-take-operator-intake.md) 之六（代码 `0e3bb7df`），规则出处是 [ADR-0183](../../../docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md)。
出处：
- 票 15 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `0e3bb7df`」那一条的 Spec 非阻断 (1)；
- [label-channel/30](../../label-channel-service-first-release/issues/30-controlled-close-reopen-decision-write-face-ps-half.md)「裁决」22:0x ② 的越权风险点「重开决定应否保存货主新授权证据引用（本票只核不存，PS owner）」，以及同票评审「偏离 ④」。
地盘：
- docs/adr/0187-<slug>.md（新）
- docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md（只改 Status 行与 Links）
- docs/adr/0151-unassigned-command-faces-get-their-families.md（只改 Status 行与 Links）
- docs/adr/README.md
- docs/domain/parcel-shipment/CONTEXT.md
- docs/domain/GLOSSARY.md
- docs/domain/SCENARIOS.md
- internal/parcelshipment/domain/continued_attempt.go
- internal/parcelshipment/domain/continued_attempt_register.go
- internal/parcelshipment/domain/continued_attempt_test.go
- internal/parcelshipment/application/form_continued_attempt_decision.go
- internal/parcelshipment/application/form_continued_attempt_decision_test.go
- internal/parcelshipment/adapters/http/operator_decision_intake.go
- internal/parcelshipment/adapters/http/operator_decision_intake_test.go
- internal/parcelshipment/adapters/http/form_continued_attempt_decision.go
- internal/parcelshipment/adapters/postgres/continued_attempt_register.go
- internal/parcelshipment/adapters/postgres/continued_attempt_register_test.go
- internal/parcelshipment/adapters/partycommercial/continued_attempt_decision_authorization.go（只改 ContinuedAttemptDecisionRequestCoordinates 头注一句）
- cmd/parcel-api/endpoints.go（只改受控关闭与重开两行的行注）
- cmd/parcel-api/assemble_continued_attempt_decision_test.go
- docs/product/MECHANISM-INVENTORY.md（只在重生成有差时）
- .scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md（只追加 Comments 与补记后一句）
- .scratch/label-channel-service-first-release/issues/30-controlled-close-reopen-decision-write-face-ps-half.md（只追加 Comments）
- .scratch/operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md（评审结果 C5 去向）
- .scratch/operator-channel/issues/<本票>.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

新 ADR 用号 0187（本会话已占）。ADR-0183 越权风险点 1、2 的 owner 复核行不在本票，归 operator-channel/51。

## 现象

- 货主重授权证据过了门就不落册（`FormReopeningCommand` 头注「证据引用不落册——领域决定上没有它的格，它只在这一步作门」）。关闭载荷连证据格都没有。领域决定与快照文档里，既没有证据格，也没有提交操作者格。
- 册上的请求方因此无据可查。决定方是 PC 交回的运营角色（`AuthorizeContinuedAttemptDecision` 取 `authorized.Decider()`），不是具体的人，所以查不到是谁代录了货主指令。主动拒绝与授权处置两口却把操作者记成了证据（`operatorEvidence`），两边不对称。
- 冒用残余属实（ADR-0183 Context「货主的指令被冒用了两次」）。研判在票 28 代码上实测：货主指令关闭不要任何证据就能形成，重开写任意非空串就能过门。
- 依据：
  - PS CONTEXT Boundaries 把「包裹级关闭或重开请求证据」「请求方与实际决定方证据」划给 PS；
  - [pc-gaps/13](../../party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md) 裁决 ② 写明「具体请求与决定的证据归 PS（CONTEXT 原句）」，它所取候选 (a) 的代价句是「PS 存引用不判真伪」；
  - SCENARIOS 写「先保存请求方、请求原因和依据」；
  - 但 CONTEXT 定义与 Rules、PRODUCT-BASELINE 列举的必备项里都没有证据。
  所以本票是对必备项的扩充。它只增不减，与基线相容。

## 裁定

照簇 C5 终裁。要点：

1. **落什么、落在哪。** 落在决定事实上，不另立请求册。
   - 请求证据：请求方所凭依据的引用。货主指令关闭落货主那份指令的引用，重开落 `shipperAuthorizationEvidence`。去掉两端空白后逐字保存，不解析、不判真伪、不加种类格。它是引用不是内容：工单号、来电记录号、邮件标识一类，不记依据原文，也不记联系方式。
   - 操作证据：`OPERATOR/<发行方>#<sub>`，复用 http 的 `operatorEvidence`。只进审计，不顶替决定方、授权角色、授权依据快照或请求方；任何门、任何比对都不读它。领域上可以缺，操作者渠道一律填上。
2. **同在同缺。** 形成时，请求方与请求证据要么都在，要么都不在。
   - 领域谓词 `RequestEvidenceAccompaniesRequester` 一处定义。
   - 编排在问授权之前用它核命令，与缺项检查同一处。不合格答 `INPUT_NOT_ACCEPTED · DECISION_INVALID`：不问 PC、不签标识、不认重放。`Append` 再用它兜底。
   - 重建门只拒「有请求证据无请求方」，照收「有请求方无请求证据」。规则生效前或回滚写回的决定，都是 ADR-0028 说的「当时已经受过约束的事」。
   - **不加守卫迁移。** 研判稿的守卫要求「清空……重新形成」，违背 CONTEXT「不删除原关闭历史」「不得静默改写原决定历史」以及禁止回填；严格重建门在回滚后会卡死包裹。
3. **货主指令关闭必带证据。** `controlledClosurePayload` 加 `shipperInstructionEvidence`：
   - 责任来源为 `SHIPPER-INSTRUCTION` 时必带，全空白与没带同读；
   - `OPERATOR-ACTION`、`EXTERNAL-RESTRICTION` 带了即坏报文；
   - 两道都是纯载荷校验，排在 `locateParcel` 之前，答 400。
   重开载荷不变。
4. **与票 28 裁定 (b) 相容。**
   - 两份证据都不进重放比对，比对格以 ADR-0183「复核记录二」为准。
   - 重投答 `EXISTING_RESULT`，交回带原证据的原决定。
   - 写面回执随决定带回册上的 `requestEvidence`，只加这一格，调用方据此看出本次证据没落（ADR-0029）。
   - 补正证据不是重投；今天没有更正口。
5. **出新 ADR，复核记录不够。** 停用的两句只限受控关闭口与重开口，其余各条不变：
   - ADR-0183 决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」；
   - ADR-0151 决定二受控关闭与重开一项「不进命令的任何一格」。
   TF 各口要不要记提交操作者，仍归 TF owner。
6. **冒用残余在 PS 内不该堵、也堵不住。**
   - PS 做到的是请求有据可查、经手人可追；今天只能在库里查到，审计读面是后继。
   - 堵点在别处：客户渠道（ADR-0139）、授予（租户取值）、角色模型 psb/07（产品策略）、租户内控。
   - 关闭不加双人门；重开的双人控制若要，是 PC 授权规则里的租户取值。
7. **否决的候选**，写进新 ADR：
   - 只核不存；
   - 只落请求证据；
   - 提交操作者记在接入通道审计；
   - 另立请求册（留给 PS owner）；
   - 证据进重放比对；
   - 同内容、异证据答拒；
   - 证据带种类格；
   - 重建门同严，加守卫迁移；
   - 重开证据不得等于原关闭请求证据。

## 做什么

**第一笔：文档，先于代码。ADR 与改句同笔。**

1. 新 ADR：`docs/adr/0187-<slug>.md`。今天 origin/main 上的下一空号是 0186，落盘时重取。
   - Status：Accepted，写裁决能力边界，照 ADR-0183 的写法。
   - Context：引 PS CONTEXT 的拥有句、SCENARIOS「先保存请求方、请求原因和依据」、pc-gaps/13 裁决 ②、`FormReopeningCommand` 头注原句、ADR-0183 Context「货主的指令被冒用了两次」，以及核验探针的结论。
   - 候选与反方：即裁定第 7 条，各写一句为何否决。
   - Decision：
     - 一：请求证据落在决定上；
     - 二：形成门同在同缺，重建门只拒有证据无请求方；
     - 三：提交操作者进且只进操作证据一格；
     - 四：两份证据不进重放比对，回执带回请求证据；
     - 五：货主指令关闭必带 `shipperInstructionEvidence`；
     - 六：真伪与冒用残余的边界；
     - 七：部分停用两句及其适用场景。
   - Consequences：
     - 快照两键、无 DDL、无守卫；
     - 回滚只丢键、不卡包裹，所以授权映射接上后避免跨本版回滚；
     - 线格式变化；
     - `HasShipperAuthorizationEvidence` 改名为 `HasRequestEvidence`；
     - 管理台日后接这两口时：选「货主指令」须把该格做成必填，提示填引用、不填联系方式；回执为 `EXISTING_RESULT` 时提示本次未写入；
     - lc/30 的那条越权风险点与「偏离 ④」随之关闭。
   - 越权风险点：照簇 C5 终裁 Q1 的 overreachRisks。
2. `docs/adr/README.md`
   - 新增一行。
   - 0151 与 0183 两行按部分停用机制补上被停条款原句与**适用场景**，并写「其余各条不变」。
   - 0183 行另注：「复核记录二」的比较格不变，由新 ADR 决定四补明证据不在比较格内。
3. ADR-0183 与 ADR-0151 的 Status 行与 Links 各加一条前向指针，正文不改。
4. `docs/domain/parcel-shipment/CONTEXT.md`
   - 「面单继续尝试决定」定义句改为：「……受控关闭决定必须固定生效时间、权威业务截断边界、原因、请求方（如有）及其请求证据、实际决定方、关闭责任来源、授权角色和授权依据快照；重开决定必须填写原因、关联此前关闭并且只面向未来生效，有请求方时同样保存其请求证据。请求证据是请求方所凭依据的引用（如工单号、来电记录号、邮件标识），不是依据原文，也不记联系方式；本上下文如实保存、不判真伪。登录操作人可以作为操作证据另行记录，但不能替代实际决定方、授权角色和请求方。」
   - Rules「受控关闭决定以包裹为范围……」那条里，「请求方（如有）」改为「请求方（如有）及其请求证据」。
5. `docs/domain/GLOSSARY.md`「面单继续尝试决定」词条同步：加「及其请求证据」；「登录操作人不能替代实际决定方」改为「登录操作人可以作为操作证据另行记录，但不能替代实际决定方」。
6. `docs/domain/SCENARIOS.md` 面单场景「授权角色决定停止新的尝试时……」那一步：在「请求方」后加「及其请求证据」。
7. 不改：PRODUCT-BASELINE、PILOT-SCOPE、产品导览 §7.5。

**第二笔起：代码，叠在 C5-Q2 之后。**

8. domain
   - 新增 `ContinuedAttemptRequestEvidenceReference` 与 `ContinuedAttemptOperationEvidenceReference`。`ContinuedAttemptDecisionSpec` 与 `ContinuedAttemptDecision` 加这两格及访问器。
   - 导出谓词 `RequestEvidenceAccompaniesRequester`。
   - `Append` 先用这个谓词核，再交给内部追加；`decisionFrom` 只拒「有证据无请求方」。
   - `RehydrateContinuedAttemptRegister` 改走内部追加，不过形成门那一格；头注写明重建门为何照收「有请求方无请求证据」。
   - `ContinuedAttemptRequest` 头注补一句：请求证据与操作证据不在其中，并指向新 ADR 决定四。`RecordedRequest` 头注同指。
   - `ContinuedAttemptDecisionSpec.Requester` 头注「只有关闭这一格允许缺席」改口。
9. application
   - `FormControlledClosureCommand` 加 `RequestEvidence` 与 `OperationEvidence`；`FormReopeningCommand` 加 `OperationEvidence`。`ShipperAuthorizationEvidence` 同时就是重开的请求证据。
   - `formation` 另带证据一格，交给 `decisionSpecOf`。构造失败按 `DECISION_INVALID` 处理，不吞错。
   - `form` 的缺项检查加同在同缺。
   - `HasShipperAuthorizationEvidence` 改名为 `HasRequestEvidence`。
   - 改口：文件头注「登录操作人不进任何一格」；`FormReopeningCommand` 头注；`formation.request` 注释「……一处定义」。
10. http
    - `controlledClosurePayload` 加 `shipperInstructionEvidence`。两道纯载荷校验排在 `locateParcel` 之前。两条命令都填 `OperationEvidence: operatorEvidence(operator)`。
    - `continuedAttemptDecisionResponse` 加 `requestEvidence`（omitempty），只取 `decision.RequestEvidence()`；头注写明这一格的例外理由。
    - 改口：`ContinuedAttemptDecisionIntake` 头注、`OperatorDecisionIntake` 头注（同时去掉口数）、`IntakeControlledClosure` 头注，以及 operator_decision_intake_test.go 头注「提交操作者不进命令（决定五）」。
11. postgres：`continuedAttemptDecisionDoc` 加 `requestEvidence`、`operationEvidence` 两键（omitempty），并能读回；`spec` 里「四个可缺项」一句改成不带数的列举。
12. `adapters/partycommercial` 的 `ContinuedAttemptDecisionRequestCoordinates` 头注「货主只作 Requester 证据随查询进 PC」改为「货主只作请求方落在决定上，不进 PC」。原句早已不实，又与新术语「请求证据」相混。
13. `cmd/parcel-api/endpoints.go`：受控关闭与重开两行的行注「提交操作者不进命令」改为「提交操作者只作操作证据」。

**记录**

14. 票 15：
    - Comments 记下 Spec 非阻断 (1) 的去向；
    - 文末「补记」之后追加一句：请求证据与操作证据不进重放身份（ADR-0151 决定三「逐口核」，凭新 ADR 决定四）；
    - 不改原句。
15. lc/30：Comments 记下那条越权风险点与「偏离 ④」的去向。
16. 票 26：「评审结果」记下 C5 的去向。

参考原型（只读取证、未提交）：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide/C5-final/c5-final-q1-on-q2.patch`。它含 C5-Q2，在 origin/main `c68e4342` 上 dry-run 干净。挂 overlay，带 DSN 跑 `./internal/parcelshipment/... ./cmd/parcel-api/ ./internal/architecture/...`，全 ok。

## 不做

- 不另立请求册（候选丁）。未授权的请求、渠道方或代理商的请求要不要先留痕，归 PS owner。
- 不做证据更正口；不做决定历史的审计读面（后继，归 label-channel/10 那一族读面）。
- 不动 PC 授权规则、accessidentity、TF；不改 admin-web，那边尚无消费方。
- 不加守卫迁移，不改 DDL。
- 不加证据种类格；不做「重开证据不得等于原关闭证据」这道收窄。
- 硬限制的「有效解除」依据（lc/30 判断题 (d)）不在本票，也不得借用请求证据格。
- 不在参数登记册立行。操作证据的保存期限与查阅权限是租户取值，交登记册 owner 判断要不要立。

## 完成判据

**文档**

- [ ] 新 ADR 已落盘，含候选与反方及部分停用的适用场景；adr/README 新行与 0151、0183 两行的标注齐全；0151、0183 的 Status 行与 Links 有前向指针，正文 `git diff` 只动这两处。
- [ ] CONTEXT、GLOSSARY、SCENARIOS 的改句在代码之前那一笔里。

**领域用例**

- [ ] 形成门：「有请求方无证据」「有证据无请求方」都拒，两者同缺照收。
- [ ] 重建门：照收「有请求方无证据」，拒「有证据无请求方」，后者答 `ErrInvalidRehydratedContinuedAttemptRegister`。
- [ ] `RecordedRequest` 同内容异证据照样认出。

**编排用例**

- [ ] 坏形状答 `DECISION_INVALID`：授权询问不增、`identities.next` 不变、册不写；与册上原关闭同内容时也不答 `EXISTING_RESULT`。
- [ ] 接入层形状（没带证据，请求方缺席）照旧答 `SHIPPER_REAUTHORIZATION_MISSING` 与 `RESPONSIBILITY_SOURCE_UNKNOWN`。
- [ ] 两份证据去两端空白后落在决定上；决定方与授权角色只取授权答复，永不等于操作证据。
- [ ] 换证据、换提交人原样重投，答 `EXISTING_RESULT` 并交回带原证据的原决定：不签标识、不写册、不发意图。

**Intake 用例**

- [ ] 货主指令没带或全空白的 `shipperInstructionEvidence` 答坏报文，另两种来源带了也答坏报文，而且都先于寻址（包裹查不到也是 400）。
- [ ] 两条命令的 `OperationEvidence` 都等于 `OPERATOR/` 加认证出的操作者。
- [ ] 载荷夹带 `operationEvidence` 之类的格，仍被封闭解码拒绝。

**真库用例**

- [ ] 两份证据往返不丢。
- [ ] 去掉两键（模拟旧版写回）后照样读得回，包裹判断不变。
- [ ] 有证据无请求方的行在重建门上报错。
- [ ] `TestClosureAndReopeningThroughTheOperatorChannelAgainstARealDatabase` 扩一段：请求证据来自载荷、操作证据来自信封、回执带回 `requestEvidence`；换证据重投答 `EXISTING_RESULT`，回执带回的是原证据。

**残留注释与迁移**

- [ ] `git grep -n '不进命令\|不进任何一格\|证据引用不落册\|只有关闭这一格允许缺席\|Requester 证据随查询进 PC' -- internal/parcelshipment cmd/parcel-api` 零命中。
- [ ] `git diff --stat origin/main -- migrations/` 为空。

**Go 门禁与流程**

- [ ] `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- [ ] 带 DSN 跑 `go test -p 1 -count=1 ./internal/parcelshipment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/...`，全 ok。
- [ ] 机制清点在本笔的干净检出上重生成，有差就单独成笔说明。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审（两轴分开）、推送方在干净检出上带 DSN 跑全量。
- [ ] 证据层级记 `S`。
