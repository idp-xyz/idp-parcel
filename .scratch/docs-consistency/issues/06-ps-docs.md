# 06 PS（parcel-shipment）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dc06；此前 in-progress——2026-10-21 认领；原 ready-for-agent——2026-10-21 协调方立（文档自洽修订，见 [README](../README.md)「修订执行」）
Blocked by: 无（01 进 main 后开工）
出处：[audit/ps.md](../audit/ps.md)（ps.md（取证 ps-code、ps-adr、ps-uc））；[02 的完成记录](./02-code-defects-from-the-audit.md)里「交修订票」一表中交给本票的条目；[01](./01-adr-suspension-chains.md) 的「交后续」一节里交给本票的条目。

## 要什么

1. 按 main 现状逐条复核审计报告里本组的 A、B 类条目（审计钉 `6b0074d0`；其后进 main 的票与文档自洽 01、02 可能已消解一些——消解的写「已由某票消解」并指 SHA）。
2. **A 类**：改文档。范围 docs/domain/parcel-shipment/CONTEXT.md、docs/application/parcel-shipment/，以及本组新立 ADR 与它所停用的 ADR 的 Status 行与 Links。目标照 README「目标」1、2、4、5：CONTEXT 写出今天生效的语言、不变量与生命周期（已被 ADR 部分停用的句子不再以现行口吻留着，改写为现行规则并引用 ADR）；UC 同一场景只有一套结果，结果名与代码一致；不用行号、计数、位置引用。
3. **B 类**：按用户授权（「你作为业务和系统专家自决」）代决，立新 ADR（号段 0254–0259），Status 写法照近期 ADR，写越权风险点，部分停用照 `docs/adr/README.md` 顶部规则（文档自洽 01 改过的「一件事只写一处」）；裁完 CONTEXT 与 UC 随之改。裁定若要求改代码，不在本票改：在 [code/](../code/) 下补一张代码票（编号接 code/ 现有最大号），或在 02 已立的对应代码票里写明裁向并解除其 Blocked by。
4. **交横切**：需要改 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`、别组 CONTEXT/UC 的，不在本票改，逐条写进本票「交横切」一节（位置、引文、改成什么、依据），由票 10 统一改。

## 不变式

- 已接受 ADR 正文不改；只动本组文档、本组新 ADR 及其停用对象的 Status 行与 Links。
- 不改代码；不改 GLOSSARY、CONTEXT-MAP 等横切文件。
- `go run ./tools/adr-index -check`（重生成后）退 0。

## 完成判据

- 审计报告本组每一条 A、B 都有去向（已改 / 已由某票消解 / 交横切 / 立代码票）。
- 完成记录里一张对照表：审计条目 → 去向。

## 完成记录（2026-10-21，分支 claude-dc06，rebase 于 main `3619f5c0e`）

审计 [audit/ps.md](../audit/ps.md) 51 条逐条按 main 复核（审计钉 `6b0074d0`）。其间进 main 的票没有动 `internal/parcelshipment`、PS 的 CONTEXT 与 UC；文档自洽 01（resolved 于 `205eb2548`）改过 PS 一组 ADR 的 Status 行与 Links，消解了下表标明的几条。代码类条目复核时现象全部仍在。

### 审计条目 → 去向

| 条目 | 归类 | 去向 | 落在哪 |
|---|---|---|---|
| PS-01 新提交版本取代后旧占用不释放 | C | 既有代码票承接；文档加缺口注 | [code/20](../code/20-superseded-submission-version-keeps-its-control-occupation.md)；UC-PS-001「给开发的交接」`AT-PS-035` 一行 |
| PS-02 面单渠道取消不核承运商收寄与终局 | C | 既有代码票承接；文档加缺口注 | [code/21](../code/21-label-channel-parcel-cancellation-ignores-carrier-pickup-and-finals.md)（补裁向一节）；UC-PS-006 结果表与「给开发的交接」 |
| PS-03 收寄早于取消、取消先提交、收寄后到 | B | 代决 ADR-0254；已改；立代码票 | PS CONTEXT「形成取消决定前」一条；UC-PS-003 一致性一条、`AT-PS-045` 改写、新立 `AT-PS-107`；UC-PS-006 决定矩阵、并发一条、`AT-PS-080`、`AT-PS-081`；UC-PS-004 面单渠道一节、`AT-PS-095`、新立 `AT-PS-108`；[code/35](../code/35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md) |
| PS-04 撤回只释放资金冻结 | A | 已改；注释交代码票 | PS CONTEXT 撤回一条；UC-PS-005 包含、不执行、元数据、财务控制引用、步骤 6、规则、试点叠加、`AT-PS-070`/`071`/`073`/`074`，新立 `AT-PS-109`；BD 简报 BD-PS-003 补信用暴露；`releaseOccupation` 头注入 [code/38](../code/38-parcel-shipment-comments-follow-docs-consistency-06.md)；`docs/application/README.md` 一行交横切 X9 |
| PS-05 UC-PS-001 结果语义契约与提交编排对不上 | A | 已改 | UC-PS-001 结果语义契约拆成提交答复（按 `SubmitOutcome` 原名，含准入暂停、原委托不可见、关联不适用）与接受判断结果（`DECIDED`/`UNDECIDED`/`VERSION_SUPERSEDED`）；「本用例不执行」补提交编排不执行接受条件判断 |
| PS-06 `已提交`委托决定前重组无路 | B | 代决 ADR-0255；已改；立代码票 | ADR-0132 挂来向；PS CONTEXT 规则一条与两处生命周期；UC-PS-001 接受条件段与成员基线行；BD-PS-005、BD-PS-007 决定与示例；UC-PS-005 目标、规则；路径在真库上走不通的缺口立 [code/36](../code/36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md)；`OutcomeLinkIneligible` 头注入 code/38 |
| PS-07 客户面 `!found` 答 500 / 200 | C | 既有代码票承接；文档加缺口注 | [code/22](../code/22-customer-face-http-contract-misses-accepted-adrs.md)（补 06 复核一节）；UC-PS-002、UC-PS-005、UC-PS-006「给开发的交接」 |
| PS-08 越权写法不一 | A | 已改 | PS CONTEXT「统一不可见结果」词条与查询一条（可见范围内无动作授权答未获授权）；UC-PS-002 步骤 3、4、`AT-PS-019` 改写并拆出 `AT-PS-106`；UC-PS-003 `AT-PS-052`；UC-PS-005 步骤 2、`AT-PS-075`、新立 `AT-PS-110`；UC-PS-006 步骤 2 |
| PS-09 UC-PS-001 闸门口径过时 | A | 已改；注释交代码票；AGENTS.md 交横切 | UC-PS-001「开发拆分建议」末段改引 ADR-0017、0026、0146、0150；包头注入 code/38；交横切 X8 |
| PS-10 0017 与 0026 无停用关系 | B | 已由 docs-consistency/01 消解（`205eb2548`） | 0017 注记已挂 0026 的收窄与适用场景，0026 前段已写去向；不另立 ADR |
| PS-11 0081 决定六四格 | A | 已由 docs-consistency/01 消解（`205eb2548`） | 0081 注记逐格写 0158、0156、0179、0157，引原句 |
| PS-12 0081 决定三被四份收窄无指针 | A | 已改 | 0081 注记与 Links；0086、0094、0106 前段补去向；0132 前段补去向；0086、0094 Links 那一行补说明 |
| PS-13 0122 ← 0125、0125 ← 0132 | A | 已由 docs-consistency/01 消解（`205eb2548`） | 两份注记已挂部分停用；0122 判据摘要里「折叠在 PS 消费适配器」是它自己的原判，紧跟的部分停用指针写明已搬进领域 |
| PS-14 0055 决定三 `401` | A | 已由 docs-consistency/01 消解（`205eb2548`） | 0055 注记已挂各真渠道族解除 |
| PS-15 首方接入仍挂在 `PAR-INT-01` | A | 已改；Go 简报交横切 | UC-PS-001 输入契约后一句与末节一句；UC-PS-005 试点叠加 `PAR-INT-01` 一条；交横切 X10 |
| PS-16 采用判断取值四处不一 | A | 已改；GLOSSARY 交横切；注释交代码票 | PS CONTEXT 词条取一组值（已采用、待补充、待复核、冲突、未决，`待补充`与`待复核`续办方不同），生命周期两处与收件地点一条改引词条；UC-PS-002 结果表；交横切 X1；`SourceDataAdoptionOutcome` 头注入 code/38 |
| PS-17 「授权处置」另作他义 | A | 已改；TF 那句交横切 | UC-PS-006 `AT-PS-082`；PS CONTEXT 届满一句；BD-PS-001 同句；`cancel_parcel.go` 注释入 code/38；交横切 X7 |
| PS-18 UC-PS-002 结果分格与代码不对 | A | 已改 | UC-PS-002 结果语义契约按 `AmendmentOutcome` 重写，步骤 4、6、阶段节、元数据对齐 ADR-0116、0118、0120 |
| PS-19 UC-PS-003 交接与现状、闸门相反 | A | 已改 | UC-PS-003「给开发的交接」删现状句、闸门改引 ADR-0017、0026、0146，列缺口表 |
| PS-20 「按上一条」位置引用 | A | 已改 | UC-PS-003 改按「先合法形成者保留」引文与 `AT-PS-049`；UC-PS-004「上面的采用流程」改按节名（本票修订时自己新写的文字里也出现过两处位置引用，已在同一票内改掉，不在 main 上） |
| PS-21 技术未形成算不算未决 | B | 代决 ADR-0256；已改；不加 `NOT_FORMED`（02 交来的条件性 B）；评审轮补「重放须接着办」，代码半边归 code/39（提交）与 [code/44](../code/44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md)（撤回、资料修订） | UC-PS-001「尚未决定」、UC-PS-002 结果表与元数据、UC-PS-004、UC-PS-005 元数据；PS CONTEXT 资料版本生命周期 |
| PS-22 外部标识关联的`已失效` | B | 代决 ADR-0257；已改；不立代码票（02 交来的条件性 B，裁为不形成） | ADR-0184 挂回答、表述更正与决定一终态列举的部分停用（评审轮改）；PS CONTEXT 生命周期删「适用 → 已失效」、终态改两种、Rules 一条；`ExternalIdentifierChangeKind` 头注入 code/38 |
| PS-23 POD 失效表达不出 | C | 既有代码票承接；文档加缺口注 | [code/23](../code/23-final-outcome-rederivation-cannot-express-a-voided-delivery.md)；UC-PS-004「给开发的交接」 |
| PS-24 `/v1`、问题详情未落 | C | 既有代码票承接；文档加缺口注 | code/22；UC-PS-001「给开发的交接」 |
| PS-25 0140 注记「不带 detail」 | A | 已改 | 0140 注记对齐 Decision 三 |
| PS-26 CONTEXT 缺渠道择优决定 | A | 已改 | PS CONTEXT 开篇职责句、词条「渠道择优决定」、Rules 渠道选择一条（含与 NR 履约段伙伴择优的分界）、Boundaries 一条 |
| PS-27 生产归属术语无定义处 | A | 已改；GLOSSARY 与 CONTEXT-MAP 交横切 | PS CONTEXT 消费侧一条；BD-PS-004 统一为「准入控制」；交横切 X2 |
| PS-28 UC-PS-004/005/006 状态行过时 | A | 已改 | 三份状态行改为已实现部分加「给开发的交接」指针 |
| PS-29 Go 简报证据边界与布局 | A | 交横切 | X10 |
| PS-30 首方 `occurredAt` 取接收时刻 | B | 代决 ADR-0258；已改；不改代码 | PS CONTEXT 词条「来源发生时间」补一句；首方契约文档那一句随 code/22（补记一条） |
| PS-31 UC-PS-003/004/005 结果表缺格 | A | 已改 | UC-PS-003、UC-PS-004 结果表加代码列并补`请求未受理`；UC-PS-004 补面单服务终局判断结果码；UC-PS-005 补未获授权、来源冲突；UC-PS-006 新立结果语义契约 |
| PS-32 0081 决定二引的排除项不存在 | A | 已改 | 0081 注记「引用更正」；UC-PS-001「本用例不执行」补那一项 |
| PS-33 UC-PS-002 步骤 1 技术未形成 | A | 已改 | UC-PS-002 步骤 1（ADR-0256） |
| PS-34 BD-PS-009/010 仍列待确认 | A | 已改 | UC-PS-002 待确认表两行注明机制已决；权威依据表补 ADR-0116、0118、0120 |
| PS-35 `AT-PS-099`「保持未形成」 | A | 已改 | UC-PS-004 `AT-PS-099` |
| PS-36 「人工接受」 | A | 已改；产品与设计文档交横切 | UC-PS-005 规则一条；交横切 X12 |
| PS-37 撤回漏写声明撤除 | A | 已改 | UC-PS-005「本用例包含」与步骤 5 |
| PS-38 资格第 1 项自己比自己 | A | 已改 | UC-PS-003「适用资格判断」第 1 项 |
| PS-39 0061 ← 0188 无指针 | A | 已改 | 0061 注记补来向，Links 删「Status 与其余各条不变」 |
| PS-40 0045、0054 Links 死链 | A | 链接目标已由 docs-consistency/01 消解（`205eb2548`）；0045 那条说明已改 | 0045 Links |
| PS-41 同一次停用写两三遍 | A | 已由 docs-consistency/01 消解（`205eb2548`） | 头部停用行已并进注记，`adr-index -check` 校来向只在注记 |
| PS-42 `ResumePath` 计数过时 | A | 已改 | 0106 注记、0086 Links 去计数；0094 注记挂 0132 扩一格（写作部分停用 Decision 三「四格处置」），0132 前段补去向 |
| PS-43 0072 原句不逐字 | A | 已由 docs-consistency/01 消解（`205eb2548`） | 0072 注记两条都引了带括注的原句；0139 正文不改 |
| PS-44 0118 注记「今天」 | A | 已改 | 0118 注记改过去时并指 ps-port-remainder/05 |
| PS-45 节点交接复核读基线投影 | A | 已改；CONTEXT-MAP 交横切 | PS CONTEXT「地址要素」补那一路（ADR-0215）；交横切 X5 |
| PS-46 0185 不引原句 | A | 已改（Links 节已由 01 补） | 0185 前段补被停原句 |
| PS-47 Go 简报「六处」计数 | A | 交横切 | X10、X11 |
| PS-48 BD 简报次序写反 | A | 已改 | BD 简报「组合后的接单机制」第 1 条 |
| PS-49 申报测量 / 声明测量 | A | 已改；CONTEXT-MAP 交横切；注释交代码票 | PS CONTEXT 新立词条「声明测量」，资料版本词条改名；交横切 X6；代码注释入 code/38 |
| PS-50 歧义信号、范围相容只在 PS 定义 | A | 交横切 | X3 |
| PS-51 「账号授权的版本生命周期」 | A | 已改；GLOSSARY 交横切 | PS CONTEXT Boundaries PC 一条（ADR-0093）；交横切 X4 |

去向计数（按主去向）：已改 36（A 31，B 5——PS-03、06、21、22、30）；已由 docs-consistency/01 消解 7（PS-10、11、13、14、40、41、43，其中 PS-10 是 B）；交横切 3（PS-29、47、50；另有多条的 GLOSSARY、CONTEXT-MAP 或设计文档一半交横切，见 X1–X12）；既有代码票承接 5（C 类 PS-01、02、07、23、24，code/20–23）。新立代码票 6：code/35（PS-03 的代码半边）、code/36 与 code/39（修订时读码新发现，不在审计条目里）、code/37（审计顺带点出、未单列）、code/38（各条的注释半边）、code/44（评审轮 B2 立，撤回与资料修订重放短路）。

审计范围外顺带记下的「受控关闭与重开没有 UC 承载」：本票不补 UC，留给 PS owner 判要不要立。

### 代决与越权风险点

用户授权「你作为业务和系统专家自决」，五份新 ADR 都是会话代 PS owner 决，Status 写明待 PS owner 复核，越权风险点见各记录文末。

- **[ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md)（PS-03）**：取消与边界事实按两者的业务时间比（取消一侧是渠道声明的取消业务时间，不比提交时刻）；提交边界上已在册即不形成取消；取消已提交后才到的边界事实，晚于取消则取消在先，不晚于则是事实冲突，不采用也不抹取消，重投推不动、消费门入账，只由明确的业务决定解开；网络服务与面单渠道服务同用。无条款停用。越权风险点：比的是渠道声明的时间；冲突没有解开面、采用信封今天还会重投到耗尽；面单一侧改了 `AT-PS-095` 的适用面（知会 TF owner）；相等按冲突。
- **[ADR-0255](../../../docs/adr/0255-a-submitted-request-is-regrouped-only-by-withdrawing-it-and-submitting-linked-new-requests.md)（PS-06）**：决定前重组只有「撤回原委托 → 以 `WITHDRAWN_RESUBMISSION` 提交关联新委托」一条路；成员去向由新委托成员与回指表达；不开待决关联方向；受控补充只形成同一委托的新提交版本。部分停用 ADR-0132 决定一一句里「那是受控补充的一种结果」那一半。越权风险点：客户多一步、撤回后准入可能已暂停；成员去向不逐件登记；释放后重新占用；运营角色须持撤回授权。
- **[ADR-0256](../../../docs/adr/0256-technical-non-formation-is-a-reason-of-undecided-not-an-outcome-of-its-own.md)（PS-21）**：「技术未形成」不是独立结果——保全没完成即没有形成答案（5xx、原样重发），保全之后没落成的，有未决格的编排答未决并以原因码指名，提交编排不在此格、照样答 5xx；重放须接着办（评审轮改，今天只有受控补充编排做到，缺口归 code/39、code/44）。无条款停用。越权风险点：客户端要读原因码才分得出本方故障；未决统计要按原因拆。
- **[ADR-0257](../../../docs/adr/0257-a-lapsed-label-success-only-feeds-the-label-service-final-and-leaves-the-channel-parcel-number-relation-applicable.md)（PS-22）**：面单成功结果按有效期不可逆失效只进面单服务终局判断，渠道包裹件号关联照旧适用；外部标识关联只有已作废与已被替代两种终态；回答并更正 ADR-0184 决定七的理由，部分停用 ADR-0184 决定一一句里的终态列举（评审轮改）。越权风险点：身份核对照旧认一个面单已失效的号（知会 NO owner）；删去`已失效`一格。
- **[ADR-0258](../../../docs/adr/0258-the-first-party-family-declares-no-source-occurrence-time-and-records-the-receipt-instant.md)（PS-30）**：首方客户 API 不收来源方声明的发生时间，信封的来源发生时间记接收时刻；客户的业务时间只走客户请求生效时间；只限首方族。无条款停用。越权风险点：离线攒批的创建时刻不进系统；日后要收这一格须改本记录。

作者在 A 类上另作的判断（不另立 ADR，评审可挑）：

1. **PS 一组 ADR 停用链的补指针由本票做**（PS-12、39、42、44、46 与 0081、0140 的注记）。票面范围写的是「本组新立 ADR 与它所停用的 ADR」，这几条是审计列给本组的 A 类、01 的附表没有覆盖，交给只改共享文件的票 10 不合适，所以按「本组文档」处理；只动 Status 行与 Links，正文去掉 Status 行与 Links 后与 main 逐字节相同。ADR-0188 决定四写过「ADR-0061 的 Links 加前向指针，Status 与其余各条不变」，那是 01 改 README 规则之前的文档处理，本票按现行规则给 0061 的注记补了来向；0188 注记里那段去向复述没有动。
2. **0132 扩 0094 Decision 三「四格处置」一格写作部分停用**（README「扩成」同属部分停用），而不是审计建议的「补充」。
3. **采用判断取值删去`待下游判断`**：PS CONTEXT 词条原本是四值、生命周期五值；本票以词条为底加`待复核`，判`待下游判断`不是一个取值（每次采用判断变化都交下游重新判断），UC-PS-002 的「已记录但待下游判断」并入「已记录」。GLOSSARY 同名词条五值交横切 X1。
4. **「统一不可见」与「未获授权」分格写进 CONTEXT**：原词条把「调用者未获授权」并进统一不可见；改为作用域不覆盖才不可见，可见范围内无动作授权答未获授权，与各编排代码一致。

### 交横切（交 docs-consistency/10）

- **X1 GLOSSARY「当前客户资料版本采用判断」**（PS-16）。引文：「它可以表示已采用、待下游判断、待补充、冲突或未决，不是可手工覆盖的全局状态。」改成什么：取值改为 PS CONTEXT 同名词条那一组（已采用、待补充、待复核、冲突、未决），或只留定义一句、取值引 PS CONTEXT。依据：PS CONTEXT 词条；本票作者判断 3。
- **X2 GLOSSARY 与 CONTEXT-MAP：生产归属**（PS-27）。GLOSSARY 新立「生产归属」「准入控制」词条（权威身份与准入控制两个正交维度；准入暂停不是第四种权威身份、不是客户业务拒绝、不自动恢复），统一用「准入控制」；PS CONTEXT 已写消费侧一条，词条立起来后在那条加 GLOSSARY 链接（这一步是 PS 文件，票 10 可一并改或交回 PS）。`internal/pilotgovernance` 进不进 CONTEXT-MAP 由票 10 定。依据：UC-PS-001「首发试点叠加条件」、BD-PS-004、`domain.ProductionOwnershipDecision`。
- **X3 GLOSSARY：歧义信号、范围相容**（PS-50）。由票 10 定是否进 GLOSSARY；若进，以 PS CONTEXT 两词条现文为准，PS 与 NO 的 CONTEXT 改为引用。
- **X4 GLOSSARY「渠道账号使用授权」相关一条**（PS-51）。引文：「重新取得授权形成新的有效依据，不复活已撤销或到期的旧授权版本」。改成什么：「旧授权版本」改为「旧授权登记」一类不暗示商业版本的说法。依据：ADR-0093「渠道账号使用授权不是商业版本，走自己的修订式登记册」。
- **X5 CONTEXT-MAP `parcel-shipment → network-routing`**（PS-45）。引文：「网络与路由不建立读取小包托运地址的端口」。改成什么：前句加适用场景「接受前与小包托运信封触发的判断里」，与后句的节点交接复核例外接上。依据：ADR-0215 决定二；PS CONTEXT「地址要素」。
- **X6 CONTEXT-MAP「申报测量」**（PS-49）。引文：`parcel-pricing → parcel-shipment` 一条「计价按声明包裹身份读取申报测量与地址要素」，以及「提交版本申报」一条里的同词。改成什么：「声明测量」。依据：PS CONTEXT 新词条「声明测量」、ADR-0048 标题。
- **X7 TF CONTEXT「授权处置」**（PS-17）。引文：「容量缩减或新增分配使软容量超过阈值 → 形成超配缺口并进入授权处置」。改成什么：换一个不撞 PS「授权处置」（ADR-0132）的说法，例如「交授权角色裁决」，或由 GLOSSARY 消歧。依据：PS CONTEXT 词条「授权处置」。
- **X8 AGENTS.md「当前默认切片」**（PS-09）。引文：「范围：机制与产品策略已放行，租户取值与 Bento 持久化闸门仍阻断」。改成什么：持久化实现按 ADR-0026 可写，登记为发布基线仍在闸门后。依据：ADR-0017 注记（0026 收窄）、ADR-0026。
- **X9 `docs/application/README.md` UC-PS-005 一行**（PS-04）。引文：「撤回、接受、拒绝并发决定边界及冻结释放补偿已形成」。改成什么：「……及已成立占用的释放补偿已形成」。依据：PS CONTEXT 撤回一条、ADR-0125 决定四。
- **X10 首个消费者切片简报**（PS-15、29、47）：「`SourceRequestKey` 的真实组成由各接入适配器依据 `PAR-INT-01` 绑定」改为首方族由首方契约推导（ADR-0139 决定五）、租户既有渠道依据 `PAR-INT-01`；「当前仍没有可持久化业务切片或 Bento 候选依赖。」改过去时或删；布局块的 `parcel-migrate/`、`parcel-outbox/`、`internal/platform/` 下 `bootstrap/`、`observability/`、`tests/` 下 `bento_contract/` 按 `cmd/` 与 `internal/platform/` 实际目录改；「六处委托 `!found` 到`统一不可见结果`的映射」改为按符号指（各委托编排的 `!found` 分支），不写数。
- **X11 开发主线「七处 `!found`」**（PS-47）：「……因此共七处」「七处 `!found` 分支里六处旁边就地记下了该约束」两句的计数改为按符号指，或锚取证 SHA。
- **X12 「人工接受」**（PS-36）：`docs/product/PILOT-SCOPE.md`「撤回、自动或人工接受及主动拒绝竞争同一不可覆盖决」一句，`docs/design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md` 与 `docs/design/pn-02-real-parameter-evidence-and-development-handoff.md` 里「人工接受」各处，改为「接受」（人工复核完成本身不形成决定）。依据：PS CONTEXT 接受判断任务生命周期。

### 检查

- scratchpad 副本（`git archive claude-dc06` 解出）上 `go run ./tools/adr-index` 重生成后 `-check` 退 0。重生成的 `docs/adr/README.md` 不随本票提交（照 parallel-sessions「生成物不占号」）。
- `go run ./tools/at-coverage-counts -check`：只报计数过期（退 1），计数由推送方重算。
- 本票改过的 ADR 去掉 `Status` 行与 Links 节后与 main 逐字节相同（脚本比对，0 份不同）；Status 行新加的「……」引文逐个查被停方正文，均逐字存在（只有引 UC 节名与引别份 ADR 原句的两处不在本文件，属预期）。
- 改过的全部 `.md` 相对链接与锚点逐个解析，全部能解析。
- 新写的文字里没有行号、跨文件计数与「上一条」式位置引用（逐行扫过）。
- `git diff main --stat` 只含：PS CONTEXT；`docs/application/parcel-shipment/` 下 7 份；新 ADR 0254–0258；ADR 0045、0061、0081、0086、0094、0106、0118、0132、0140、0184、0185 的 Status 行与 Links；`.scratch/at-coverage-inventory.md`；`.scratch/docs-consistency/` 下本票与 code/21、22、35–39、44（评审轮立）。不改代码。

## 评审记录（2026-10-21，隔离评审第 1 轮 → 作者修订）

阻断两条、建议合入前修 5 条、顺手 8 条，全部照办；未采纳：无。

| 条 | 评审意见 | 处置 |
|---|---|---|
| B1 阻断 | 保全之后出错，ADR-0256 决定一第 2 格说答未决格，UC-PS-001 说答 5xx；代码 `SubmitOutcome` 没有未决格 | 照协调方裁定改 ADR-0256：决定一第 2 格写明提交编排不在内（ADR-0081 下提交只答归属与`已提交`，保全之后出错答 5xx，重发须接着办，code/39）；Context 补提交编排取证；标题、注记、越权风险点 3、Links 同步；UC-PS-001 那句保留，依据改指 ADR-0256 决定一 |
| B2 阻断 | 「重放走已有结果那条路」在提交、撤回、资料修订三只编排上走不通，只有受控补充编排接着办 | ADR-0256 决定一第 2 格续办改为「重放须接着办」并注明今天只有受控补充编排做到；Context 补三只编排的短路取证；Consequences 去掉「代码不改」改指代码票；立 [code/44](../code/44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md)（撤回、资料修订，含业务未决之后重放与资料修订注释误判，建议 R2，44 号已 `ls` 确认空着）；code/39 保持 R1、只管提交；UC-PS-002、UC-PS-005「给开发的交接」各补一行指 code/44 |
| 1 | 面单冲突三处写成「停在未决」 | code/21 裁向、code/35 修法第 4 条、UC-PS-006 规则一节改为「不形成终局、指名冲突（`LABEL_SERVICE_NOT_FINAL` 加一格原因）」，UC-PS-006 那句的「停在未决」只留给网络服务 |
| 2 | 第 1 格不比时间与「到达顺序不参与」相抵 | 照协调方裁定保留不对称：ADR-0254 决定一标题与正文、注记、CONTEXT 取消一条写明：两边都在册之后只比业务时间，谁先越过提交边界只决定走哪一格（CONTEXT 写作「走哪一条」），并写明理由；补越权风险点 5（同一对业务时间因到达先后不同而结果不同） |
| 3 | 地址要素「受理之后」「受理基线」 | CONTEXT 改为「接受之后」「接受基线」 |
| 4 | `AT-PS-079` 场景 | UC-PS-006 改为「取消到达提交边界时收寄已采用」；UC-PS-003 `AT-PS-045`「同 `AT-PS-079`」核过一致 |
| 5 | 已有结果不带原归属结果 | UC-PS-001「给开发的交接」补一行，并入 code/39 修法第 2、3 条 |
| 6 | 0254 第 1 格引的 CONTEXT 旧句已删 | 改引新句「此刻已在册的收寄或终局使取消不成立」 |
| 7 | 面单一侧时间的引法 | 改为引 ADR-0135 决定三原句「业务发生时间取自依据、不由本上下文铸；依据是外部承运轨迹事实时取它已判断的有效时间。」 |
| 8 | ADR-0257「只有两种终态」收窄了 ADR-0184 决定一 | 改为部分停用 0184 决定一那一句的终态列举，逐字引原句、写适用场景；0257 前段、「对已接受记录的作用」、Links，0184 注记（「不改」改为部分停用）与 Links 两头补齐 |
| 9 | UC-PS-006 步骤 2 未获授权 | 注明 `CANCELLATION_REFUSED` 即 CONTEXT「统一不可见结果」词条所说的未获授权那一格 |
| 10 | 未决原因码列全 | UC-PS-002 去掉「例如」、按 `JudgmentPendingReason.String()` 与编排调用点列全（补 `SHIPMENT_REQUEST_UNAVAILABLE`、`SOURCE_DATA_VERSION_IDENTITY_UNAVAILABLE`、`STALE_SHIPMENT_REQUEST_REVISION`）；UC-PS-005 同法列全（补 `SHIPMENT_REQUEST_UNAVAILABLE`） |
| 11 | UC-PS-005 首段与步骤 7 | 补未获授权与来源冲突两格 |
| 12 | 完成记录 PS-20 一行不实 | 按 main 实况改写 |
| 13 | code/35 书名号、0061「推进」 | code/35 证据去掉非原文书名号；0061 注记改写为「停用（ADR-0188 自称「推进」，即把门扩到这一格）」 |

修订后的检查（main 仍是 `3619f5c0e`，未再 rebase）：scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0；改过的 ADR 去掉 Status 行与 Links 后与 main 逐字节相同（11 份，0 份不同）；Status 行新引文逐字核过被停方正文（ADR-0184 决定一那句、0132、0081、0061、0185、0045），未命中的两处是引 UC 节名与引别份 ADR 原句，属预期；改过的 `.md` 相对链接与锚点全部能解析。

## 评审记录（第 2 轮，2026-10-21）

第 1 轮 B1、B2 评审确认已消。残留阻断一条、建议三条、顺手四条，全部照办：

| 条 | 评审意见 | 处置 |
|---|---|---|
| X1 阻断 | ADR-0256 越权风险点 3 引的「重发接着办」在 code/39 里没有 | 改为引 ADR-0256 决定一第 2 格原句「重发必须接着办」，落地指 code/39 修法第 2 条 |
| N1 | ADR-0256 把短路说过头：资料修订停在 `SOURCE_DATA_VERSION_NOT_HANDED_OFF` 之后重放能接着办 | 决定一第 2 格、注记、前段收窄为保全已落下而结果没落成的那几格，资料修订写明是版本落库之前停下的各格；Context 补资料修订保全自成一笔事务（`preservationBoundary`），业务未决之后重放同样短路，并写明 `SOURCE_DATA_VERSION_NOT_HANDED_OFF` 那一格不在其中 |
| N2 | UC-PS-006 把冲突格读成取消未决，与 UC-PS-003 `AT-PS-107` 两套写法 | 决定矩阵拆成「有效性待确认」与「事实冲突」两行；`AT-PS-081` 两种情形直写，后一种与 `AT-PS-107` 同一结果；`AT-PS-107` 补收寄后处置不形成，对齐 |
| N3 | UC-PS-001 交接左列与 code/39 证据指「未决结束」 | 两处改指 ADR-0256 决定一第 2 格「重发必须接着办」 |
| N4 | code/44 撤回的 `DECISION_NOT_RECORDED` 走不到保全照样提交 | 证据与修法第 4 条写明它整笔回滚答 5xx；短路的是业务未决、读不回与 `STALE_SHIPMENT_REQUEST_REVISION`；ADR-0256 Context 同口径 |
| N5 | code/44 撤回修法 | 改为按委托状态判，并写明 `domain.Withdrawal` 不带请求身份 |
| N6 | ADR-0184 注记的不变范围 | 补齐为「行只插不改」「适用状态的变化另起一行挂在原关联上，终态一次」与「不回到适用」，与 ADR-0257 对齐 |
| N7 | 票面三处 | 「检查」diff 清单补 code/44；第 1 轮评审记录第 2 条去掉非逐字的书名号；code/44 现象去掉书名号 |

检查（main 仍是 `3619f5c0e`）：scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0；改过的 ADR 去掉 Status 行与 Links 后与 main 逐字节相同；改动的 `.md` 链接全部能解析；新写的书名号引文逐字核过（ADR-0256 决定一原句、ADR-0184 决定一原句、`amend_customer_source_data.go` 那段注释）。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dc06` tip `8936be533` 在 main `3619f5c0e` 上快进；两轮隔离评审的阻断全部消掉（第二轮残留的 ADR-0256 越权风险点 3 引文已改为本记录原句）。
- 协调方随后在 tip 上单独提交：重生成 `docs/adr/README.md`、重算 AT 盘点总览计数；两者 `-check` 退 0。机制清点重生成无变化；`go build ./...` 干净。本票只动文档，未跑全量真库测试。
- 去向：代码票 code/35–39、code/44；code/39（来源保全之后建单失败、重发不再建单）建议 R1，排进文档轮之后的首批代码票。
