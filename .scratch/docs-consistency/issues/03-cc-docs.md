# 03 CC（customs-compliance）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dc03（见文末「完成记录」）。此前 in-progress——2026-10-21 认领；原 ready-for-agent——2026-10-21 协调方立（文档自洽修订，见 [README](../README.md)「修订执行」）
Blocked by: 无（01 进 main 后开工）
出处：[audit/cc.md](../audit/cc.md)（cc.md）；[02 的完成记录](./02-code-defects-from-the-audit.md)里「交修订票」一表中交给本票的条目；[01](./01-adr-suspension-chains.md) 的「交后续」一节里交给本票的条目。

## 要什么

1. 按 main 现状逐条复核审计报告里本组的 A、B 类条目（审计钉 `6b0074d0`；其后进 main 的票与文档自洽 01、02 可能已消解一些——消解的写「已由某票消解」并指 SHA）。
2. **A 类**：改文档。范围 docs/domain/customs-compliance/CONTEXT.md、docs/application/customs-compliance/，以及本组新立 ADR 与它所停用的 ADR 的 Status 行与 Links。目标照 README「目标」1、2、4、5：CONTEXT 写出今天生效的语言、不变量与生命周期（已被 ADR 部分停用的句子不再以现行口吻留着，改写为现行规则并引用 ADR）；UC 同一场景只有一套结果，结果名与代码一致；不用行号、计数、位置引用。
3. **B 类**：按用户授权（「你作为业务和系统专家自决」）代决，立新 ADR（号段 0241–0244），Status 写法照近期 ADR，写越权风险点，部分停用照 `docs/adr/README.md` 顶部规则（文档自洽 01 改过的「一件事只写一处」）；裁完 CONTEXT 与 UC 随之改。裁定若要求改代码，不在本票改：在 [code/](../code/) 下补一张代码票（编号接 code/ 现有最大号），或在 02 已立的对应代码票里写明裁向并解除其 Blocked by。
4. **交横切**：需要改 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`、别组 CONTEXT/UC 的，不在本票改，逐条写进本票「交横切」一节（位置、引文、改成什么、依据），由票 10 统一改。

## 不变式

- 已接受 ADR 正文不改；只动本组文档、本组新 ADR 及其停用对象的 Status 行与 Links。
- 不改代码；不改 GLOSSARY、CONTEXT-MAP 等横切文件。
- `go run ./tools/adr-index -check`（重生成后）退 0。

## 完成判据

- 审计报告本组每一条 A、B 都有去向（已改 / 已由某票消解 / 交横切 / 立代码票）。
- 完成记录里一张对照表：审计条目 → 去向。

## 完成记录（2026-10-21，分支 claude-dc03，基于 main `205eb254`）

审计 cc.md 24 条（钉 `6b0074d0`）逐条按 main `205eb254` 复核：其间进 main 的 CC 相关改动只有文档自洽 01（`3221ed51f` 合并 Status 行两写）与票 146、94 各给 ADR-0206 / 0213 的 Links 加一行，CC 领域代码、CONTEXT 与 UC 未变。除 L1 已由文档自洽 01 消解外，23 条现象仍在。

### 审计条目 → 去向

| 条目 | 类 | 去向 | 落点 |
|---|---|---|---|
| H1 `AT-CC-109`「就绪不存在」答门禁阻断 | A | 已改 | [ADR-0243](../../../docs/adr/0243-customs-first-release-answers-replace-the-uc-cells-they-answer-and-unavailable-dependencies-answer-undecided.md) 决定一把 ADR-0227 决定八的替代范围扩到这一格；UC-CC-005 新立 `AT-CC-448`（AT 盘点入①，钉 `TestReadinessAndAuthorityAreTwoSeparateTracks`），`AT-CC-109` 只留已失效与目标不一致两格 |
| H2 UC-CC-006「冲突门禁规则未登记即门禁未决」 | A | 已改 | UC-CC-006 时间与当前判断一节、结果表「外部结果冲突」、`AT-CC-158`、待决表「同层冲突裁决」四处改为 ADR-0226 判法：冲突不另给依赖动作加阻断或未决（ADR-0226 决定三、ADR-0206 决定五），按权威、更正或人工复核裁决要等处置入口（ADR-0226 越权风险点 7）；UC-CC-007 同一概念一并改 |
| H3 限制是监管限制还是内部合规限制 | B | 已改（代决）+ 立代码票 + 交横切 | [ADR-0241](../../../docs/adr/0241-action-admission-restrictions-are-regulatory-or-internal-and-only-regulatory-restrictions-exist-today.md)；CONTEXT 立「监管限制」、改「内部合规限制」「动作准入判断」与两节 Rules；UC-CC-011、UC-CC-013；[code/27](../code/27-customs-restriction-comments-call-the-regulatory-restriction-internal-and-internal-restrictions-have-no-model.md)；GLOSSARY 见「交横切」X1–X3 |
| M1 UC-CC-004 就绪读不动两套结果 | A | 已改 | ADR-0243 决定二；UC-CC-004 结果表加「授权前提未配置」「不再就绪」两格与首发结果码对照，`AT-CC-103` 去掉就绪一格 |
| M2 `AT-CC-105`/`444`、`110`/`439` 只加旁注 | A | 已改 | ADR-0243 决定三部分停用 ADR-0227 决定八「两行的正文不改……」一句（0227 注记与 Links 挂来向）；`AT-CC-105` 只留目标不一致、`AT-CC-110` 只留已撤销 / 到期 / 目标不符 / 已被使用，各注首发实现到哪 |
| M3 UC-CC-001 缺两格、`AT-CC-016` | A | 已改 | 结果表加「包裹未接受」「建案要求未配置」与首发结果码对照；`AT-CC-016` 写首发答未决并指名读不动的那一口 |
| M4 「已登记待发送」只形成一半 | A | 已改 | UC-CC-005 首发结果码对照写明一半；`AT-CC-107` 场景限定为已有发送通道，`AT-CC-125`、`126`、`127` 写首发答法 |
| M5 CONTEXT「首次实际发送时」 | A | 已改 + 立代码票 | CONTEXT Lifecycles「申报单元、就绪与提交」改为在固定提交版本时核、初始尝试为`已登记待发送`；代码里引原句的四处并进 [code/26](../code/26-cc-comments-cite-context-line-numbers-and-counts.md) |
| M6 技术回执是不是外部结果层 | B | 已改（代决） | [ADR-0242](../../../docs/adr/0242-a-technical-receipt-belongs-to-its-submission-attempt-not-to-the-external-result-register.md)；CONTEXT「技术回执」词条；UC-CC-005 阶段表、UC-CC-006 层次表与交接表、`AT-CC-140`、`141`、`144`、`153`；票 psb/10 Comments 记裁向 |
| M7 UC-CC-009 缺 ADR-0137、门禁结果格 | A | 已改 | 权威依据加 ADR-0137 决定三、四与 ADR-0236；外部资金事实只经 SA 采用信封；门禁结果按 CONTEXT 五格加未决；`AT-CC-260` |
| M8 关闭依据项缺「冲突」 | C | 代码票 code/03（核对） | 不走「并入未解决」，四值维持；code/03 末尾记复核 |
| M9 层内更正关系 | C | 代码票 code/04（核对） | UC-CC-006 待决表「同层冲突裁决」与「来源权威与解释规则」更正一条点名 code/04；`AT-CC-156` 加首发注（今天照判冲突、范围只到单元） |
| M10 ADR-0070 行号 | A | 已改 | 0070 注记挂「引用更正」，191 / 186 / 193 逐个对到 CONTEXT 引文；标题与正文不改 |
| M11 CC 注释行号 | A（代码） | 立代码票 | [code/26](../code/26-cc-comments-cite-context-line-numbers-and-counts.md)（R3） |
| M12 ADR-0206 R12 未落地 | A | 已改 + 交横切 | 0206 注记挂「复核记录 R12 未落地」读法注；0213 同根那一行属 TF 组，见 X6 |
| L1 CC ADR 的 Status 行两写 | A | 已由文档自洽 01 消解 | `3221ed51f`：0197、0206 来向只在注记，0226、0227、0236 注记不再重说接受出处 |
| L2 ADR-0137「三册同族一致」 | A | 已改 | 0137 注记挂来向（0149 决定一、三停用凭证那一册的适用，0236 手工口）、Links 加 0149；0149 前段补去向、Links 加 0137 |
| L3 UC-CC-013 已落地的票仍写待 | A | 已改 | 状态行、「本用例不执行」、待决两条 |
| L4 「七类」「五本」「PAR-CUS-01..07」 | A | 已改 + 立代码票 | UC-CC-005 改指 UC-CC-003「就绪门禁」表；CLI 包头注并进 code/26 |
| L5 GLOSSARY 与 CONTEXT 定义出入 | A | 交横切 | X5 |
| L6 凭证登记漏 CLI 一路 | A | 已改 | CONTEXT「监管凭证、限制与处置」 |
| L7 UC-CC-003 缺 ADR-0137 | A | 已改 | 权威依据加 `ADR-GATE`，待决行收窄 |
| L8 更正复用首报授权 | C | 既有票 psb/36（核对） | UC-CC-007 待决表加「后续动作的逐次授权」一行，指 ADR-0227 越权风险点 10 |
| L9 逐包裹范围 AT 无首发注 | C（02 交本票） | 已改 | UC-CC-006 `AT-CC-146`、`147` 与范围一节，UC-CC-009 `AT-CC-279` |

去向计数：已改 18（其中 3 条代决立 ADR），已由别票消解 1，交横切 1（另有 2 条已改的连带交横切），归代码票 4（新立 code/26；核对 code/03、code/04、psb/36；另有 2 条已改的连带进 code/26、1 条进新立的 code/27）。

### 代决与越权风险点

用户授权「你作为业务和系统专家自决」，代 CC owner 裁定；三份 ADR 的 Status 前段都写「代决」与「待 CC owner 复核」，工具派生〔代决·待 CC owner 复核〕。

- **ADR-0241**（H3）：限制分监管限制（凭已接受监管结果整条解除，即代码的 `RegulatoryRestriction`）与内部合规限制（UC-CC-011，覆盖项、限制责任来源解除、部分解除），互不改名互不解除；准入合成两种，内部合规限制待确认或冲突答未决；实现现状只读监管限制（缺口，首发范围仍要求内部合规限制）。部分停用 ADR-0206 决定四、决定五各一句。越权风险点：监管限制只能整条解除；待确认答未决依赖执行方不把未决当可走；准入今天不读内部合规限制（实现缺口）；监管限制无生产写入；「监管限制」一词进 GLOSSARY 交票 10。
- **ADR-0242**（M6）：技术回执按提交尝试归属、记在提交一侧，由申报发送通道接收，外部结果册不另加层；状态推进还是另记事实归 psb/10。不停用任何条款。越权风险点：报关服务方线下代发的技术回执无入口；技术回执怎样驱动「原提交结果仍待确认」；GLOSSARY 是否补一句交票 10。
- **ADR-0243**（H1、M1–M4 的文档处理）：提交执行时就绪没登答未决；授权确认、提交执行与 UC-CC-009 的税费付款、放行门禁两只核对编排读不动、写不进答未决并指名那一口（与建案同写法）；被替代的格移出原行。部分停用 ADR-0227 决定八「两行的正文不改……」一句，并把决定八「这替代……的结果名」一句扩到 `AT-CC-109`。越权风险点：「未决」一个结果码装两类续办，只靠原因分；写不进也答未决依赖整笔回滚；长期格留在原行。

范围外动到的两处，评审请留意：ADR-0149（横切的接入族 ADR）Status 前段与 Links 补了对 0137 的去向——停用链两头都在是 adr/README 的规则，只挂一头会留半条链；票 psb/10 的 Comments 追加了 ADR-0242 的裁向（02 允许「在已有代码票写明裁向」）。

### 交横切（交票 10）

| # | 位置 | 引文 | 改成什么 | 依据 |
|---|---|---|---|---|
| X1 | GLOSSARY「内部合规限制」 | 「所有者：`customs-compliance`；只有该限制的责任来源可以形成有效解除。」 | 补「它不是监管限制，两者互不改名、互不解除」；GLOSSARY 新增「监管限制」词条，定义引 CC CONTEXT「监管限制」 | ADR-0241 决定一 |
| X2 | GLOSSARY「限制」 | 「例如关务限制由 `customs-compliance` 拥有。」 | 补「关务限制分监管限制与内部合规限制两种」 | ADR-0241 决定一 |
| X3 | GLOSSARY「动作准入判断」 | 「再逐动作把内部合规限制、放行门禁核对和外部放行结果折成的封闭答复」；「门禁满足不答放行，外部放行不解内部限制。」 | 「把限制（监管限制与内部合规限制）、放行门禁核对和外部放行结果折成」；「外部放行不自动解除限制」 | ADR-0241 决定三；CC CONTEXT「动作准入判断」词条 |
| X4 | GLOSSARY「技术回执」（可选「申报提交尝试」） | 「所有者：`customs-compliance` 保存来源回执及其与提交尝试的关系」 | 同向，可补「按提交尝试归属、记在提交一侧，不是外部结果册的一层」 | ADR-0242 决定一 |
| X5 | GLOSSARY 四条（审计 L5） | 「限制适用性判断」：「判断至少区分有效限制、当前无内部限制、适用待确认、冲突和技术未形成」；「税费付款核对」：「银行、支付或财务系统提供的实际付款事实」；「申报就绪判断」：「对明确正式申报资料版本是否同时满足」；「监管核定税费」：「`settlement-accounting` 只依据有效责任形成关税代垫回收和服务费用」 | 依次对齐 CC CONTEXT：四格加「技术未形成另成一类结果」；「外部资金事实」；「对明确申报范围是否已经满足」；「依据实际代垫成立判断与有效责任形成客户代垫回收，服务费用另按独立费用规则形成」 | CC CONTEXT 同名词条与「税费、放行与案件闭环」节；GLOSSARY「客户代垫回收」 |
| X6 | ADR-0213 Status 行注记（TF 组） | 复核记录 R12「同 ADR-0206 的复核记录，见票 demo-chain-gaps/147。」 | 照 ADR-0206 注记新挂的「复核记录 R12 未落地」读法注：待票 147 落地并由新 ADR 部分停用 ADR-0206 决定二，落地前现行仍为决定二 | 审计 cc M12 |
| X7 | `docs/product/CUSTOMS-FIRST-RELEASE-DEVELOPMENT-BASELINE.md`「2. 内部合规限制已形成」 | 「`UC-CC-003`、`UC-CC-009` 和 `UC-CC-010` 消费该用例的范围化当前判断」 | 补 `UC-CC-013` 动作准入消费它；首发范围不变，实现现状（尚无模型与写入口）只记在 UC-CC-011 与 ADR-0241 决定四，不写进基线 | ADR-0241；UC-CC-011 |

### 检查

- `go run ./tools/adr-index`（重生成）后 `go run ./tools/adr-index -check` 退 0；新三份带〔代决·待 CC owner 复核〕。重生成的 README 没有提交（生成物在进 main 时统一做）。
- `go run ./tools/at-coverage-counts -check` 只报「生成物与文件不一致」（加了 `AT-CC-448` 一行，退 1），没有格式错；计数由协调方重算。
- 改过的 24 个文件里的全部相对链接与锚点用脚本解析，无断链。
- `git diff main --stat`：CC CONTEXT、`docs/application/customs-compliance/` 九份 UC、新 ADR-0241–0243、被停或补指针的 0070、0137、0149、0206、0227（Status 与 Links），`.scratch` 下本票、code/03、26、27、psb/10、AT 盘点；没有代码、GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`。

## 评审记录

### 隔离评审（2026-10-21，阻断 1 条）与作者回应（rebase 于 main `f928c5ba`）

阻断：

1. **UC-CC-009 新造了同一场景两套结果**：「放行门禁核对未决」写进了「依赖读不动」，原有「未形成税费/门禁核对」与 `AT-CC-282` 仍把同一场景答成未形成。改：未决一行只留未配置与判不了（没有付款核对、三态待确认或冲突）；未形成一行与 `AT-CC-282` 写首发答 `UNDECIDED` 并指名读不动或写不进的那一口；结果表后补首发结果码对照（门禁核对与付款核对两只编排，`RECORDED` 五值结论、判不了与未配置的原因、读不动的原因各占一格）。ADR-0243 决定二扩到这两只编排，标题、注记、Consequences 随之改（ADR-0243 未进 main，就地改）。

建议与顺手：

2. UC-CC-006 时间一节与待决表「同层冲突裁决」删去「冲突本身是否另行阻断依赖动作，尚待决定（ADR-0226 越权风险点 7，票 124）」——已由 ADR-0226 决定三、ADR-0206 决定五定为不另加；越权风险点 7 只留按权威、更正或人工复核的裁决。
3. UC-CC-007「冲突门禁规则未确认时保持处理未决」改为照当前判断读、处置待入口。
4. ADR-0243「对已接受记录的作用」与 ADR-0227 注记逐字引出被扩的决定八「这替代 UC-CC-004 `AT-CC-105`……答 `NOT_AUTHORIZED`）」一句；逐字核过。
5. AT 盘点 `AT-CC-448` 行注明测试的 `Covers:` 尚未点名、补点名待 code/26；code/26 修法加这一项。行留在①（测试确实钉住这一格，只缺点名）。
6. ADR-0241 决定四、注记、越权风险点 3、Consequences 改称「实现现状 / 缺口」，点明 PILOT-SCOPE 与关务专项基线切片 1 仍要求内部合规限制、不在 ADR 里收窄；UC-CC-011、UC-CC-013、code/27 与交横切 X7 同此。
7. 按 `f928c5ba` 的跨组约定删去 CC CONTEXT「动作准入判断」末尾的实现缺口一条（UC-CC-011、013 已有）；CONTEXT 头部固定指针由票 10 统一加，本票不加。
8. 技术回执由发送通道一侧形成、UC-CC-006 只提供来源接受纪律：UC-CC-005 同步响应一段、`AT-CC-133` 与末段，UC-CC-006 交接表、`AT-CC-140`、`141`。
9. UC-CC-001 `AT-CC-438` 用「建案要求未配置」；UC-CC-006 `AT-CC-156` 与「来源权威与解释规则」更正一条加首发注（今天照判冲突、范围只到单元，缺口见 code/04）；UC-CC-004「请求未受理」去掉「当前就绪依据不可关联」，写明就绪没登属「授权前提未配置」。

代码票号：本票占 code/26–27，未再立新票。

检查（修后）：`go run ./tools/adr-index` 重生成后 `-check` 退 0（README 未提交）；`at-coverage-counts -check` 只报计数过期；改动的 24 个文件相对链接与锚点全部解析；0206、0227、0137 注记与 ADR-0243 里的被停原句、0070 注记里的 CONTEXT 引文逐字核过。

### 第二轮隔离评审（2026-10-21，阻断 2 条）与作者回应

阻断：

- **B1 `DUTY_VERIFICATION_HANDOFF_UNAVAILABLE` 挂错了编排。** 它只由资金事实新版本的重派编排（`rederive_duty_verifications.go`）给出；人重核（`VerifyPayment` → `handOffVerification`）交接失败照答 `DUTY_VERIFICATION_FORMED` / `EXISTING_DUTY_VERIFICATION` 并带 `CONT-DUTY-VERIFICATION/…` 与原始错误，门禁核对（`handOffGate`）交接失败照答 `RECORDED` / `EXISTING_RESULT` 并带 `CONT-GATE/…`。改：UC-CC-009 对照表把这个码单列为重派编排一行（整笔回滚、消费门重投），表后补「人重核与门禁核对的交接失败照答已形成并带续办引用，不算未形成」；元数据「未形成结束」把事件发布限定在重派路上；ADR-0243 决定二与注记同改。
- **B2 `AT-CC-110` 写了「已到期答 NOT_AUTHORIZED」**，CC 领域没有到期字段（`SubmissionAuthorization.Effective()` 即 `revokedAt.IsZero()`）。改为「首发已撤销答 NOT_AUTHORIZED；授权没有到期时间、只绑申报单元、授权使用未建模，已到期、目标不符、已被使用三格未实现」；UC-CC-005、UC-CC-004 首发结果码对照同步；到期缺口登进 code/26「附带登记」一节（实施时宜挪进 psb/36）。

建议与顺手：

1. UC-CC-009「目标与边界」首段与可观测性「分别统计」一条改成五种结论加「放行门禁核对未决」，统计补部分满足、不适用、未决。
2. 元数据「门禁核对结束」补判不了的情形（没有付款核对、三态待确认或冲突）也答未决。
3. 对照表补全：门禁核对 `NOT_ACCEPTED`（缺租户、范围、边界或动作不在词表；读回认定折不出结论，加现状注）、付款核对 `NOT_ACCEPTED` 与 `DUTY_VERIFICATION_FORMED` / `EXISTING_DUTY_VERIFICATION`；表头写明只列结论、未受理、未决与未形成的分格；付款核对未决原因列全，去掉「如」。`foldRefused` 答未受理照裁定立 [code/34](../code/34-release-gate-verification-answers-not-accepted-when-recorded-findings-do-not-fold.md)（R3，34 未被占）。
4. ADR-0243：决定二补一句准入按 ADR-0206 决定五保留读不动、本记录写法不延伸到准入（共用 `assessReleaseGate`、编排不同）；Links 补 UC-CC-009、ADR-0137、ADR-0206；Context 补评审发现；越权风险点 2 逐口点明「写不进什么都没留下」所依的代码。
5. ADR-0243 前段与「对已接受记录的作用」、ADR-0227 Links 改为「决定八两句（一停一扩）」。
6. ADR-0241「对已接受记录的作用」与 ADR-0206 注记里的书名号引文改为 0206 正文逐字「动作时点有效且约束本动作的限制一律`阻断`，来源是限制标识」。
7. ADR-0242 的「监管侧六层」「不加第七层」改为「监管侧各层」「不另加层」（标题与文件名本无计数；所引 `ResultLayer` 代码注释的原文照留）；本票面、psb/10 Comments 同此。
8. CC CONTEXT Lifecycles「税费付款与放行门禁」末条改为失败、迟到、冲突分别保存，技术未形成分别答复、不留下半份核对。

代码票号：本票占 code/26–27 与 code/34。

检查（修后，rebase 于 main `f329f505`，AT 盘点追记一处与 05 的追记相邻冲突、两条并存）：scratchpad 副本上 `go run ./tools/adr-index` 重生成后 `-check` 退 0；`at-coverage-counts -check` 只报计数过期；改动的 25 个 .md 相对链接与锚点全部解析；0206、0227、0137 注记与 ADR-0241、0243 里的被停原句逐字核过。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dc03` tip `2636bcfe8` 在 main `f329f505` 上快进；两轮隔离评审的阻断全部消掉，第二轮修订由协调方对代码抽核（`DutyVerificationHandoffUnavailable` 只由重派编排给出、`SubmissionAuthorization` 无到期字段）。
- 协调方随后在 tip 上单独提交：重生成 `docs/adr/README.md`、重算 AT 盘点总览计数；两者 `-check` 退 0。机制清点重生成无变化；`go build ./...` 干净。本票只动文档，未跑全量真库测试。
- 去向：代码票 code/26、27、34。
