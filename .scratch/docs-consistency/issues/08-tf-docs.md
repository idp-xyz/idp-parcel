# 08 TF（transport-fulfillment）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dc08。此前 ready-for-agent——2026-10-21 协调方立（文档自洽修订，见 [README](../README.md)「修订执行」）
Blocked by: 无（01 进 main 后开工；票 57、88、146+94 已进 main）
出处：[audit/tf.md](../audit/tf.md)（tf.md）；[02 的完成记录](./02-code-defects-from-the-audit.md)里「交修订票」一表中交给本票的条目；[01](./01-adr-suspension-chains.md) 的「交后续」一节里交给本票的条目。

## 要什么

1. 按 main 现状逐条复核审计报告里本组的 A、B 类条目（审计钉 `6b0074d0`；其后进 main 的票与文档自洽 01、02 可能已消解一些——消解的写「已由某票消解」并指 SHA）。
2. **A 类**：改文档。范围 docs/domain/transport-fulfillment/CONTEXT.md、docs/application/transport-fulfillment/，以及本组新立 ADR 与它所停用的 ADR 的 Status 行与 Links。目标照 README「目标」1、2、4、5：CONTEXT 写出今天生效的语言、不变量与生命周期（已被 ADR 部分停用的句子不再以现行口吻留着，改写为现行规则并引用 ADR）；UC 同一场景只有一套结果，结果名与代码一致；不用行号、计数、位置引用。
3. **B 类**：按用户授权（「你作为业务和系统专家自决」）代决，立新 ADR（号段 0263–0268），Status 写法照近期 ADR，写越权风险点，部分停用照 `docs/adr/README.md` 顶部规则（文档自洽 01 改过的「一件事只写一处」）；裁完 CONTEXT 与 UC 随之改。裁定若要求改代码，不在本票改：在 [code/](../code/) 下补一张代码票（编号接 code/ 现有最大号），或在 02 已立的对应代码票里写明裁向并解除其 Blocked by。
4. **交横切**：需要改 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`、别组 CONTEXT/UC 的，不在本票改，逐条写进本票「交横切」一节（位置、引文、改成什么、依据），由票 10 统一改。

## 不变式

- 已接受 ADR 正文不改；只动本组文档、本组新 ADR 及其停用对象的 Status 行与 Links。
- 不改代码；不改 GLOSSARY、CONTEXT-MAP 等横切文件。
- `go run ./tools/adr-index -check`（重生成后）退 0。

## 完成判据

- 审计报告本组每一条 A、B 都有去向（已改 / 已由某票消解 / 交横切 / 立代码票）。
- 完成记录里一张对照表：审计条目 → 去向。

## 完成记录（2026-10-21，分支 claude-dc08，基 main `3619f5c0e`）

审计 [audit/tf.md](../audit/tf.md) 41 条逐条按 main `3619f5c0e` 复核（审计钉 `6b0074d0`）。其间进 main、动过 TF 的票：88（ADR-0231，`d238b6023`）、146+94（ADR-0237、0238）、57+15（ADR-0232，进 main 记录 `311adf934`）、145（ADR-0239），以及文档自洽 01（`205eb2548`）、02。消解的写在下表，其余现象仍在。

### 审计条目 → 去向

| 条目 | 归类 | 去向 | 落在哪 |
|---|---|---|---|
| H1 `AT-TF-181` 重号、结果相反 | A | 已改 | UC-TF-004 删去「不改写首次开始」那一行；留下的那一行顺带改掉已被 ADR-0231 停用的「采用不入队」 |
| H2 委托取消写成只停尚未消耗的下游范围 | A | 已改 | TF CONTEXT「运输委托、订舱、容量与装载」取消一条与生命周期取消一条改为整份、不往下游传（ADR-0204 决定三）；UC-TF-004 业务规则、`AT-TF-092`、「给开发的交接」记 ADR-0204 越权风险点 9 与票 58、37 |
| H3 交出、交付门禁读数的现行规则只在 ADR-0213 补记里，「文末补记」指错 | A | 已改 | TF CONTEXT Language「交出门禁读数」「交付门禁读数」逐码写出停点与所属格、关务读不动的两种依据；「运输交接与实物控制」末条写出交出更正另读一份、「揽收、派送与交付」写出 POD 更正承继，两边互指；UC-TF-005、UC-TF-006 改引 CONTEXT；ADR-0213 Status 前段两处「文末」改小节名；TF 文档里全部「文末补记」改按小节名引 |
| M1 `待确认`时控制归谁 | B | 代决 [ADR-0263](../../../docs/adr/0263-a-pending-handover-keeps-the-previously-confirmed-control-and-transport-fulfillment-has-no-control-unconfirmed-result.md)；已改；产品文档一处交横切 X4 | TF CONTEXT「运输交接与实物控制」里交接逐对象判断、封签、更正的各条，生命周期「权威运输交接」里待确认与已交接的条目；UC-TF-005 交接规则、`AT-TF-055`、「给开发的交接」（`AT-TF-054` 等单元控制规则，票 80） |
| M2 确认订舱需要容量保障，代码没做 | B | 代决 [ADR-0264](../../../docs/adr/0264-capacity-assurance-is-required-on-load-assignments-only-and-neither-a-confirmed-booking-nor-a-departure-reads-capacity.md) 决定一、二；已改 | ADR-0203 Status 注记与 Links 挂部分停用（决定一一句里的确认订舱）；ADR-0202 注记与 Links 挂越权风险点 17 已答；TF CONTEXT 讲容量保障的条目；UC-TF-004 `AT-TF-041`；票 demo-chain-gaps/37 加更正（保障判据作废、改为逐订舱消费关系）；注释一处并入 code/02 |
| M3 需求快照过期挡出发 | B | 代决 ADR-0264 决定三；已改 | TF CONTEXT 需求快照一条去掉「出发门禁」；UC-TF-003 容量规则、`AT-TF-089` |
| M4 UC-TF-003 的发布意图 | A | 已改 | UC-TF-003 步 8、`AT-TF-037`（ADR-0201 决定七） |
| M5 `AT-TF-048` 保留两份来源 | A | 已改 | UC-TF-004 `AT-TF-048` |
| M6 `AT-TF-093` 伙伴更正无路可走 | A | 发生项无修订入口一半已由票 57 消解（发生项失效的生产入口 `9dac020ff`，ADR-0232 决定四）；其余已改 | UC-TF-004 `AT-TF-093`：定局之后答复是冲突，范围无效经运营决定失效、结算撤下预期成本 |
| M7 订舱漏了撤回 | A | 已改；GLOSSARY 交横切 X1 | TF CONTEXT Language「订舱」「承运接受」、生命周期订舱一条；UC-TF-004 目标、对象表 |
| M8 容量池多维 | A | 已改；GLOSSARY 交横切 X2 | TF CONTEXT Language「容量池」 |
| M9 预占不带成员范围 | A | 已改 | TF CONTEXT「每个对象支持部分结果」一条 |
| M10 成功尝试形成发生项、交付自动形成漏写 | A | 已改 | UC-TF-002 目标与边界；UC-TF-006 目标与边界、步 7 |
| M11 有效时间只认规则一种来源 | A | 已改 | TF CONTEXT 外部承运轨迹事实规则与生命周期 |
| M12 交出读数不展开集运单元 | A | 已改 | TF CONTEXT「交出门禁读数」写明按单元本身问（现行 ADR-0213 决定四）；UC-TF-005「给开发的交接」记复核为应修订、票 122 |
| M13 ADR-0149 缺 0230 指针、0216 决定四标题 | A | 0149 一半已由文档自洽 01 消解（`205eb2548`，B3·5）；0216 标题一半已改 | ADR-0216 注记被停条款补决定四标题里的「作业事实口」 |
| M14 ADR-0233 与 0204、0218 的张力 | B | 已由票 88 消解（ADR-0231，`d238b6023`）；残留引文问题交横切 X6 | 0204、0218 已挂 0231 指针；ADR-0233 越权风险点 1 里「委托快照是采购的权威」不是 ADR-0204 原句，0233 正文不改，归 PC 组 |
| M15 UC 现状描述与状态行过时 | A | 已改；立 code/43 | 七份 UC 状态行改现状并指「给开发的交接」；UC-TF-002 那句改写；UC-TF-001、UC-TF-007 记无生产入口 → [code/43](../code/43-regulatory-disposition-acceptance-and-alternate-journey-have-no-production-face.md) |
| M16 尝试逐对象结果没有待确认 | C | 既有代码票承接；文档加注记 | [code/01](../code/01-fulfillment-attempt-object-outcomes-have-no-pending-confirmation.md)；UC-TF-002、UC-TF-006「给开发的交接」记 `AT-TF-018`、`AT-TF-070` 的现状缺口；AT 盘点两行移入④ |
| M17 待确认更正为`已交接`不进段 | C | 既有代码票承接；文档加 AT 与注记 | 票 demo-chain-gaps/130；UC-TF-005 新立 `AT-TF-329` 并记现状缺口，UC-TF-006 `AT-TF-289` 括注写明过渡态；票 130 记下 |
| M18 `AT-TF-199` 扇出两路 | A | 已改 | UC-TF-006 `AT-TF-199`，不写路数 |
| M19 派送要求漏了 NR | A | 已改 | UC-TF-006 输入契约，引 CONTEXT「派送要求」 |
| M20 GLOSSARY「出发门禁读数」 | A | 交横切 X3 | — |
| L1 `AT-TF-029` 部分准备 | A | 已改 | UC-TF-003 `AT-TF-029`、结果契约（「部分准备」一行换成「准备未配置」）、步 7 |
| L2 `AT-TF-122` 缺前提 | A | 已改 | UC-TF-005 `AT-TF-122` |
| L3 `AT-TF-251` 两格并一格 | A | 已改 | UC-TF-005 `AT-TF-251` 分写 `HANDOVER_NOT_HANDED_OVER`、`NO_PARTICIPATION_ENDED`，号不拆（已被测试点名） |
| L4 委托生命周期与对象表 | A | 已改 | TF CONTEXT 生命周期委托一条；UC-TF-004 对象表补运输已开始、换版；「给开发的交接」记 ADR-0202 决定五的缺口 |
| L5 发生项种类超出四原因 | A | 已改 | TF CONTEXT 发生项一条；UC-TF-004「给开发的交接」记票 76 |
| L6 `AT-TF-192` 与 `AT-TF-281` | A | 已改 | UC-TF-006 `AT-TF-192` |
| L7 派送要求「按对象引用」 | A | 已改 | TF CONTEXT Language「派送要求」 |
| L8 监管承接「待补充」 | B | 代决 [ADR-0265](../../../docs/adr/0265-a-regulatory-disposition-supplement-required-is-an-answer-not-a-recorded-decision.md)；已改；接线并入 code/43 | TF CONTEXT 词条、规则、生命周期；UC-TF-001 目标、元数据、六层表、结果契约、步 3、`AT-TF-004`、新「给开发的交接」；代码头注与新口径一致，不改 |
| L9 代码注释与现行决定不符 | C | 既有代码票承接 | code/02（本票追加 ADR-0264 带出的一处） |
| L10 Go 注释行号 | C | 既有代码票承接 | code/02 |
| L11 同一次停用写两遍 | A | 已由文档自洽 01 消解（`205eb2548`） | — |
| L12 0217 停用方与「见 adr/README 该行」 | A | 已由文档自洽 01 消解（`205eb2548`） | — |
| L13 0202、0209 缺 0233 回答指针 | A | 已由文档自洽 01 消解（`205eb2548`，X-09） | — |
| L14 0202「不等答不受理」的读法 | A | 已改（并收 01「交后续」交给本票的那一条） | ADR-0202 注记里那半句加读法注：「不受理」读作不收下这份答复、答订舱不在册并记挡回，不是`未受理` |
| L15 0209 Status「文末」「决定六」 | A | 已改 | ADR-0209 Status 前段 |
| L16 0213 缺 0221 指针 | A | 已改（按部分停用处理，不是无条款停用） | ADR-0213 注记来向逐字引票 66 补记「参与的入场依据只带交接版本、不带交接范围。」一句、写适用场景，Links 加 0221；ADR-0221 正文没写这一笔，补在它 Status 前段 |
| L17 交付段不准备 | A | 已改 | TF CONTEXT「班次与运输资源」 |
| L18 「只能做骨架」旧口径 | A | 已改 | UC-TF-001 末节、UC-TF-002 与 UC-TF-005 首发条件、UC-TF-007 `AT-TF-081` |

去向计数（按主去向）：已改 27（A）；代决立 ADR 后已改 4（M1、M2、M3、L8）；已由某票消解 5（M13、M14、L11、L12、L13）；只交横切 1（M20，另 M1、M7、M8 各有一半交横切）；既有代码票承接 4（M16、M17、L9、L10）。新立代码票 1（code/43），code/02 追加一处，code/01 不改票面。

02「交修订票」交给本票的条目：tf M14 一条，按「修订票 07、08 复核时只补文档」复核——TF CONTEXT 与 UC 已随票 88 写进委托协议换版与未形成册，0204、0218 已挂 0231 指针，本票无需再补。01「交后续」交给本票的条目：L14 那一条，已办。票 57 动过的 ADR-0200、0204、0207、0208、0229 已由 01 补完，本票复核无遗留。

### 代决与越权风险点

用户授权「你作为业务和系统专家自决」，三份新 ADR 全是会话代决，Status 写明代谁、待谁复核，越权风险点见各记录文末。

- **ADR-0263（M1，代 TF owner，待 TF owner 复核，第 2、3 点同待 NO owner）**：权威运输交接结果封闭三格；待确认保持此前已确认的控制关系，证据已表明占有变化而身份认不出的那一种也是待确认；更正撤回控制由失效参与版本与 NO 的「控制来源链待重算」合起来表达；集运单元成员控制规则未立前单元交接不为成员形成参与或控制判断，封签异常时不为成员另立待确认/冲突。不停用任何已接受条款。越权风险点：待确认期间控制记在已无实物的一方；单元成员控制规则未立（票 80）；长期待确认没有信号。
- **ADR-0264（M2、M3，代 TF owner，待 TF owner 复核）**：确认订舱不挂容量保障，答复不读池、不要预占引用，接受量可超过池余量；保护只在装载分配；出发不读容量与需求快照，需求变了由运营重核预占与分配。部分停用 ADR-0203 决定一一句里的确认订舱，回答 ADR-0202 越权风险点 17；票 37 改范围。越权风险点：池上看不出承运方已接受多少；需求变了没人提醒、分配变化入口未接线；租户可能要按确认订舱挡超卖。
- **ADR-0265（L8，代 TF owner，待 TF owner 复核，第 1、3、4 点同时待 CC owner 会签）**：承接决定封闭三种；待补充是答复，不落承接记录、不交回执，续办是协作事项本身；今天只有缺移动授权一种、答复不带原因，接线后（code/43）带一个原因，封闭为缺移动授权、缺对象、缺动作、缺起止责任。不停用任何已接受条款。越权风险点：关务一侧要自己记住哪件事项在等补；本上下文看不到在等补的事项；一件事项部分缺授权只能整件待补或记作部分承接的原因；UC-CC-008 要关务自记这一格答复。

### 交横切（交 docs-consistency/10）

- **X1 GLOSSARY「订舱」**（M7）。引文：「订舱可以部分确认、候补、拒绝、取消或失效」。改成什么：答复封闭为接受、拒绝、失效、撤回、候补五值，部分确认是接受量少于申请量，`待确认`是尚无答复时的读法，取消是运营决定不是答复；判法引 TF CONTEXT「订舱」，不另写。依据：ADR-0202 决定五、TF CONTEXT「订舱」。同节「承运接受」的「针对运输委托或订舱」按 TF CONTEXT「承运接受」改为落在订舱上、委托一级与逐对象不另成对象。
- **X2 GLOSSARY「容量池」**（M8）。引文：「班次、运输资源或外部运输服务在明确范围和期间内可供预占与分配的多维运力范围。」与「容量池只启用适用的重量、体积、件数、托盘位、袋位或舱位等维度，不强制折算为一个“剩余运力”数字。」改成什么：一池是一个具体班次在一个维度上的运力（身份由班次与维度派生），一班的多维运力是一组池，引 TF CONTEXT「容量池」，不另写。依据：ADR-0201「补记：票 17 落地时的补充裁定」。
- **X3 GLOSSARY「出发门禁读数」**（M20）。引文：「一条出发落册时与它同一笔保存的、逐个在控对象向动作准入判断拉来的答复记录」。改成什么：收成一句定义，判法（集运单元按出发那一刻的直接成员逐件问、单元那一格取最重）指向 TF CONTEXT「出发门禁读数」，不另写。依据：ADR-0225。
- **X4 `docs/product/IDP-PARCEL-BUSINESS-PROCESS-AND-MODULE-GUIDE.md`**（M1）。三处：
  - 正文：「实物占有已经变化但身份或合法来源不明时，必须进入控制待确认/冲突，不能为了保持页面整齐而虚构原方仍有控制。」
  - 交接登记一步那一行：「只有已交接才转移控制和建立实际履约段；占有已变但身份/合法来源不明时记为控制待确认/冲突」。
  - 跨上下文交接表 `transport-fulfillment` → `node-operations` 那一行：「已交接时记录控制转移；拒收或待确认且占有未变时保持当前原控制（若有）；占有已变但身份/合法来源不明时记录控制待确认/冲突」。
  改成什么：照 ADR-0263 改为记待确认、保持此前已确认的控制关系，接收一侧认不出的实物归节点作业的待识别实物与身份冲突处置；第三处的「且占有未变」去掉。依据：ADR-0263 决定二。
- **X5 GLOSSARY「权威运输交接结果」与「监管运输处置承接决定」**（M1、L8）。前者引文「结果逐对象形成为已交接、已拒收或待确认；只有已交接转移控制，已拒收和待确认均保持此前已确认的控制关系。」口径已与 ADR-0263 一致，按目标 6 改为引 TF CONTEXT「运输交接与实物控制」；后者引文「逐范围形成为已承接、部分承接、待补充或拒绝」改为三种决定、待补充是答复不落决定。依据：ADR-0263、ADR-0265。
- **X6 ADR-0233 越权风险点 1 的引文**（M14 残留，PC 组地盘）。0233 把 ADR-0204 决定一引作「委托快照是采购的权威」，0204 原句是「服务方、协议快照与采购责任法人的权威仍是委托那一行」，且那一句已被 ADR-0231 部分停用。0233 正文不改；要不要在 0233 Status 注记加一条「引用更正」，交票 07 或 10 定。
- **X7 SCENARIOS 容量例子**（M2，ADR-0264 越权风险点 3）。引文：「申请量为 `500kg`，外部服务方只接受并确认 `350kg`，因此先形成 `350kg` 的有效容量预占；各数量保持独立。」改成什么：注明那一笔预占是运营人员按确认的量另作的一次容量预占决定，承运答复本身不预占、不读池；「因此」改掉。依据：ADR-0264 决定一、二，ADR-0203 决定三。

### 检查

- scratchpad 副本（`git archive claude-dc08` 解出）上 `go run ./tools/adr-index` 生成后 `-check` 退 0；三份新 ADR 在索引里带〔代决·待 TF owner 复核〕。生成的 `docs/adr/README.md` 不随本票提交。
- `go run ./tools/at-coverage-counts -check`：只报计数过期（退 1），计数由推送方重算。
- 改过的 `.md` 相对链接与锚点逐个解析，全部能解析。
- 新 ADR 与改过的 Status 行里的书名号引文对出处逐字核过（CONTEXT、UC 的旧句对 main `3619f5c0e`，并在三份新 ADR 的 Context 里锚了取证 SHA）；被停原句（0203 决定一一句、0213 票 66 补记一句、0216 决定四标题）在被停方正文逐字存在。
- 改过 Status 行的已接受 ADR（0202、0203、0209、0213、0216、0221）去掉 Status 行与 Links 后与 main 逐字节相同。
- 不改代码；没碰 PS（票 06）、PC+PP（票 07）的文件，也没碰 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`。


## 评审记录（2026-10-21，隔离评审第 1 轮 → 作者修订）

阻断 4 条、建议 5 条、顺手 7 条，全部照办。

| 条 | 评审意见 | 处置 |
|---|---|---|
| B1 阻断 | 三份新 ADR 正文用「第二条」「两条」之类序号与计数引 TF CONTEXT | 0263 Context 与 Consequences、0264 Consequences、0265 Status 前段与 Consequences 一律改成「小节名 + 该条起头的原句」；顺带把 0264 引 UC-TF-004 的「步 4、5」改成按动作说、把「容量后来缩减……」的省略引文改成原句，0263 Context「与之相抵的有三处」去掉计数。新写文字再扫一遍，没有剩下的序号与计数 |
| B2 阻断 | UC-TF-004 结果表与步 4、5 还是旧口径，步 5 与 ADR-0264 决定二相抵 | 结果表按 `CommissionOutcome.String()` 逐码重写：`COMMISSION_SUBMITTED`、`BOOKING_SUBMITTED`、`BOOKING_ANSWERED`（五值，部分确认并入接受）、`COMMISSION_CANCELLED`/`BOOKING_CANCELLED`、`TRANSPORT_ALREADY_STARTED`、四个协议否定格、`COMMISSION_NOT_FOUND`/`COMMISSION_NOT_BOOKABLE`/`SCHEDULE_NOT_FOUND`/`BOOKING_NOT_FOUND`、`EXISTING_*` 四格、`*_CONFLICT` 三格、`INPUT_NOT_ACCEPTED`、`COMMISSION_UNDECIDED` 带五个原因码；「待补充/冲突」拆开、删掉待补充；表后写明带码各行属 `CommissionTransportHandler`。步 4 改为记承运答复五值、没有答复读作待确认、答复不读池；步 5 改为运营另作预占或释放（ADR-0203 决定三、ADR-0264 决定二），硬上限 `CAPACITY_EXCEEDED` 不可越过 |
| B3 阻断 | ADR-0265 决定二「这一格本身就说明缺的是移动授权」与决定四相抵 | 照协调方裁定两头都改：决定二改为今天只有缺授权一种、所以不带原因；决定四写明接线后待补充带一个原因，封闭为缺移动授权、缺对象、缺动作、缺起止责任，今天不带原因不算缺口；code/43 修法同步。（第 2 轮更正：第 1 轮只改了正文，索引注记里「这一格本身就说明缺的是移动授权」那句旧话第 2 轮才删、补进决定四的口径） |
| B4 阻断 | UC-TF-001 结果契约不全、没有码 | 逐码对齐 `AcceptDispositionOutcome.String()`：`DISPOSITION_DECIDED`（三种决定 `ACCEPTED`/`PARTIALLY_ACCEPTED`/`DECLINED`）、`SUPPLEMENT_REQUIRED`、`EXISTING_DECISION`、`DECISION_CONFLICT`、`NODE_ONLY_DISPOSITION`、`NOT_ACCEPTED`、`UNDECIDED`（`DISPOSITION_ACCEPTANCE_STORE_UNAVAILABLE`）；补「纯节点动作」「已有决定」两行；表后写明带码各行属 `AcceptRegulatoryDispositionHandler`，旅程、交接、履约与停止各行出自别的编排 |
| 建议 1 | CONTEXT「容量预占」仍写订舱、承运接受引用预占 | 改为只有装载分配经分配关系引用预占、实际装载与它比较，订舱与承运接受不引用（ADR-0264） |
| 建议 2 | 本分支改掉的 CONTEXT 原句还被 Go 注释逐字引着 | 并入 code/02 追加节，按符号定位：`SubmitBooking` 体内、`CancelCommission` 头注、`FormCarrierAcceptance` 头注、`AdmitsSuccessor` 头注、`ErrCancelledAfterTransportStart` 与 `ChargeOccurrenceReason` 两处；另核出两处同类一并列入——`CarrierAcceptanceOutcome` 头注引的旧订舱生命周期（评审所说 `FormCarrierAcceptance` 一带的生命周期引文实际在这里）、`CapacityPool` 头注「CONTEXT 说的多维运力范围」 |
| 建议 3 | ADR-0221 Links 里 0213 那条 | 改写为部分停用，写法照前段，去掉「文末」 |
| 建议 4 | 交横切 X4 漏两处、引文截断；X2、X5 前半没引文 | X4 列全产品指南三处并引全；X2 引 GLOSSARY「容量池」两句；X5 前半引 GLOSSARY「权威运输交接结果」那一句 |
| 建议 5 | UC-TF-005「当前实物方明确而来源链矛盾时保存控制冲突」 | 改成失效参与版本加节点作业记「控制来源链待重算」合起来的读法，不另存控制冲突（ADR-0263 决定三） |
| 顺手 6 | ADR-0264 与产品基线的关系 | 越权风险点补一条：PRODUCT-BASELINE「不得重复累计同一容量」、PILOT-ACCEPTANCE-MATRIX OPS-05「硬容量不可突破」不相抵，硬上限在预占上守（`domain.ErrCapacityExceeded`），接受量超过池只是承运方的陈述；SCENARIOS「先形成 `350kg` 的有效容量预占」写进交横切 X7 |
| 顺手 7 | ADR-0265 越权风险点 | 补两条：一件事项部分可承接部分缺授权时只能整件答待补充或记作部分承接的未承接原因；UC-CC-008「保存其接受、拒绝、待补充或部分接受结果」要靠关务自己记这一格答复，须 CC owner 会签 |
| 顺手 8 | ADR-0203 注记与票 37 标题 | 注记把决定一小标题「（产品策略；票 21 落装载分配，票 37 落确认订舱）」与 Consequences「票 21 在决定一、二之内落装载分配的容量关系；票 37 落确认订舱。」里的「票 37 落确认订舱」逐字点名一并作废；票 37 标题改为「逐订舱的消费关系」并注原题（文件名不改，免得断链） |
| 顺手 9 | UC-TF-003「已有结果」带「发布意图引用」 | 改为原准备结果与它引用的班次、容量；输入契约「请求与历史」与一致性一条里的发布意图同改 |
| 顺手 10 | UC-TF-004 交接表把「委托一级与逐对象的承运接受」列为缺口 | 删去那一行：CONTEXT 写的是规则（不另成对象），交接表不再当缺口列 |
| 顺手 11 | 「已拒绝」与「拒绝」混用 | CONTEXT 生命周期与「监管运输处置承接与履约」一条、UC-TF-001 目标、六层表与步 3 统一用「已拒绝」 |
| 顺手 12 | 新 ADR 的 Date 行 | 三份都在 Status 行与 Date 行之间空一行 |

未采纳：无。AT 盘点与 06、07 的冲突照协调方说由落地时取并集，本轮没再动盘点。

修订后的检查：main 仍是 `3619f5c0e`，无需 rebase。scratchpad 副本（`git archive claude-dc08`）上 `go run ./tools/adr-index` 后 `-check` 退 0；改动的 `.md` 相对链接与锚点全部解析；三份新 ADR 的书名号引文对出处逐字核过（旧句对 main `3619f5c0e`）；被停原句（0203 决定一一句、0213 票 66 补记一句、0216 决定四标题）在被停方正文逐字存在；0202、0203、0209、0213、0216、0221 去掉 Status 行与 Links 后与 main 逐字节相同；code/02 追加节里引的注释原句在 Go 源里逐字存在（`CarrierAcceptanceOutcome` 那一句在注释里折了行，引的是它所引的 CONTEXT 旧句）；`go run ./tools/at-coverage-counts -check` 只报计数过期。

## 评审记录（第 2 轮，2026-10-21）

第 1 轮 B1、B2、B4 评审确认已消；B3 只改了正文。本轮阻断 2 条、建议 4 条、顺手 7 条，全部照办：

| 条 | 评审意见 | 处置 |
|---|---|---|
| X-B1 阻断 | ADR-0265 索引注记还留着「这一格本身就说明缺的是移动授权」，决定四没写进去 | 注记改为：今天只有缺移动授权一种、答复不带原因；接线后（code/43）待补充带一个原因，封闭为缺移动授权、缺对象、缺动作、缺起止责任；越权风险点摘要补上后两条。第 1 轮 B3 行加更正 |
| X-B2 阻断 | UC-TF-001「缺路由」答待补充，与 CONTEXT「已承接但无当前有效路线 → 履约准备待形成」相抵，也不在四种原因里 | UC-TF-001 元数据「待补充结束」、结果表 `SUPPLEMENT_REQUIRED` 行、`AT-TF-004` 去掉路由，地点并进起止责任；元数据注明没有当前有效路由不在此列；步 4 与输入契约「路由计划」行改为无计划时承接继续有效、履约准备待形成。CONTEXT「监管运输处置承接与履约」里待补充那一组改为缺当前有效移动授权或载运对象、运输动作、起止责任（含起始控制与目标责任边界）不足，路线缺席指向本节「当前无有效路线或运输机会时」那一条；ADR-0265 里定位这一句的引文随之改为该条起头的原句 |
| 建议 1 | 会签点 | ADR-0265 前段改为第 1、3、4 点同时待 CC owner 会签；ADR-0263 前段补第 3 点另待 VE owner 会签；票面同步 |
| 建议 2 | 测试注释四处 | 并入 code/02 追加节：`TestADeclineCarriesItsBasisAndNoObjects`、`TestAPartialAcceptanceKeepsBothSides`（引的是 UC-TF-001 结果契约旧句）、`TestCancellationOnlyReachesUnstartedCommissions`、`TestCancellationOnlyReachesUnstartedIntents`（行号写法加旧句） |
| 建议 3 | UC-TF-004 元数据与 `AT-TF-039` | 「待补充结束」改为「未成结束」，指向协议否定格、引用不在册或不可订与没有答复读作待确认；`AT-TF-039` 改为委托已提交、订舱读作待确认 |
| 建议 4 | ADR-0264 Context 的符号 | 改为 `application.DepartureGateReader.Read` 与端口 `ports.DepartureGateView.ReadDepartureGate` |
| 顺手 5 | 步 5 的码出处 | 点明 `CAPACITY_EXCEEDED` 属 `application.PrepareTransportOpportunityHandler.Reserve` 的 `OpportunityOutcome` |
| 顺手 6 | 0264 与 0203 对称 | 0264「对已接受记录的作用」补一条：0203 小标题与 Consequences 里的「票 37 落确认订舱」一并作废；0203 Links 里 ADR-0202 那一条照新口径改 |
| 顺手 7 | UC-TF-005 引 CONTEXT 原句 | 改引「当前实物方已明确但控制来源链冲突」 |
| 顺手 8 | 「已拒绝」 | UC-TF-001 目标首段与元数据「已拒绝结束」统一 |
| 顺手 9 | `AT-TF-043` 与 `AT-TF-139` 对不上 | UC-TF-004「给开发的交接」补一行：今天池只有硬上限，没有软超配阈值与审批的输入格，`AT-TF-043` 走不到，软超配尚未立票 |
| 顺手 10 | 去向表 M1、M2 行的序号与计数 | 改按条目内容说 |
| 顺手 11 | CONTEXT 数量转换一条 | 改为各量各自保存、只沿明确关系转换，预占由运营另作、确认订舱不自动转成预占（ADR-0264） |

未采纳：无。

修订后：已 `git rebase main`（main `544e41de1`，06、07 已进），AT 盘点 ④ 节与文末追记的冲突取并集，06、07 与本票的行与追记都保留。scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0，重生成的索引里 ADR-0265 那一行不再有「这一格本身就说明缺的是移动授权」；改动的 `.md` 链接全部解析；新写的书名号引文逐字核过；0202、0203、0209、0213、0216、0221 去掉 Status 行与 Links 后与 main 逐字节相同；`at-coverage-counts -check` 只报计数过期。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dc08` tip `ae7e340d6` 在 main `544e41de1` 上快进；两轮隔离评审的阻断全部消掉，第二轮修订由协调方抽核（ADR-0265 索引注记已无旧理由；UC-TF-001 缺路由不再答待补充）。
- 协调方随后在 tip 上单独提交：重生成 `docs/adr/README.md`、重算 AT 盘点总览计数；两者 `-check` 退 0。机制清点重生成无变化；`go build ./...` 干净。本票只动文档，未跑全量真库测试。
- 去向：代码票 code/43；code/02 追加注释清单；票 37 改范围；交横切 X1–X7 交票 10。票 demo-chain-gaps/126 可开工。
