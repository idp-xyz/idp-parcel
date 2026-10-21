# 04 VE（visibility-exception）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dc04（见文末「完成记录」）。此前 in-progress——2026-10-21 认领；原 ready-for-agent——2026-10-21 协调方立（文档自洽修订，见 [README](../README.md)「修订执行」）
Blocked by: 无（01 进 main 后开工）
出处：[audit/ve.md](../audit/ve.md)（ve.md）；[02 的完成记录](./02-code-defects-from-the-audit.md)里「交修订票」一表中交给本票的条目；[01](./01-adr-suspension-chains.md) 的「交后续」一节里交给本票的条目。

## 要什么

1. 按 main 现状逐条复核审计报告里本组的 A、B 类条目（审计钉 `6b0074d0`；其后进 main 的票与文档自洽 01、02 可能已消解一些——消解的写「已由某票消解」并指 SHA）。
2. **A 类**：改文档。范围 docs/domain/visibility-exception/CONTEXT.md、docs/application/visibility-exception/，以及本组新立 ADR 与它所停用的 ADR 的 Status 行与 Links。目标照 README「目标」1、2、4、5：CONTEXT 写出今天生效的语言、不变量与生命周期（已被 ADR 部分停用的句子不再以现行口吻留着，改写为现行规则并引用 ADR）；UC 同一场景只有一套结果，结果名与代码一致；不用行号、计数、位置引用。
3. **B 类**：按用户授权（「你作为业务和系统专家自决」）代决，立新 ADR（号段 0245–0248），Status 写法照近期 ADR，写越权风险点，部分停用照 `docs/adr/README.md` 顶部规则（文档自洽 01 改过的「一件事只写一处」）；裁完 CONTEXT 与 UC 随之改。裁定若要求改代码，不在本票改：在 [code/](../code/) 下补一张代码票（编号接 code/ 现有最大号），或在 02 已立的对应代码票里写明裁向并解除其 Blocked by。
4. **交横切**：需要改 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`、别组 CONTEXT/UC 的，不在本票改，逐条写进本票「交横切」一节（位置、引文、改成什么、依据），由票 10 统一改。

## 不变式

- 已接受 ADR 正文不改；只动本组文档、本组新 ADR 及其停用对象的 Status 行与 Links。
- 不改代码；不改 GLOSSARY、CONTEXT-MAP 等横切文件。
- `go run ./tools/adr-index -check`（重生成后）退 0。

## 完成判据

- 审计报告本组每一条 A、B 都有去向（已改 / 已由某票消解 / 交横切 / 立代码票）。
- 完成记录里一张对照表：审计条目 → 去向。

## 交横切

不在本票地盘、交票 [10](./10-cross-cutting-docs.md) 统一改的。位置一律文件加引文，不用行号。

1. **`docs/domain/CONTEXT-MAP.md`**（VE-14）。`party-commercial / parcel-shipment → visibility-exception` 一条的「参与方与商业提供货主客户账户、责任法人、服务产品、客户合同、客户查询授权及适用异常服务规则」：去掉「客户查询授权」，另起半句「客户查询授权由共享身份能力的客户渠道册给出（ADR-0139 决定二、四；`PAR-INT-13`）」。依据：ADR-0139 决定二、四；代码 `adapters/accessidentity/customer_tracking_view_authenticator.go`；PC CONTEXT 里没有「查询授权」。UC-VE-008 的 `ACCOUNT` 行本票已拆成 `ACCOUNT`、`ATTRIBUTION`、`GRANT` 三行。
2. **GLOSSARY 补「客户归属」「客户全程追踪视图」两个词条**（VE-18）。前者 UC-CC-001（「保存客户归属和来源资料引用」、`AT-CC-437`）与 SCENARIOS 都在用，后者 CONTEXT-MAP 与多份 ADR 在用。词条正文取 VE CONTEXT Language 同名两条原文；同一笔把 VE CONTEXT 那两条改为一句定义加「跨上下文定义见 GLOSSARY」。这一处要动 VE CONTEXT，由票 10 在同一笔里做，或交回 VE 组。
3. **GLOSSARY「供应商或保险追偿」与 VE CONTEXT「追偿事项」同义两名**（VE-18）。GLOSSARY 词头改为「追偿事项」，「供应商或保险追偿」列为同义；「应追偿金额」一条「避免使用：追偿事项、追偿认可金额、已收回、追偿成功。」里的「追偿事项」改写为「（不要把金额称作）追偿事项」，免得读成「不要用追偿事项这个词」。依据：VE CONTEXT Language「追偿事项」、UC-VE-007。
4. **GLOSSARY「状态」一条**（VE-18）。「全程追踪状态是面向包裹旅程的派生投影，不覆盖源上下文状态。」改为「全程追踪投影是面向包裹旅程的派生投影，不覆盖源上下文状态；它按并行维度表达，不是一条统一状态机。」依据：VE CONTEXT「追踪摘要」词条「不是一条覆盖各维度的统一状态机」；「全程追踪状态」一词在任何地方都没有定义。
5. **事实类型词的单一出处**（VE-13）。GLOSSARY「外部承运轨迹事实」与 TF CONTEXT 同名词条各有一句「提供给 `visibility-exception` 时的事实类型词为 `external-carrier-tracking`。」：保留词，句末加「（全部类型词见 VE CONTEXT「事实类型词」）」。依据：本票在 VE CONTEXT 立了全部 26 个类型词的单一表（计数取证于 main `205eb254` 的 `adapters/registrationjson` 与各消费适配器）。ADR-0197 决定二、ADR-0205 文末补记里的词表是接受时的历史，正文不改。
6. **UC-TF-005 `AT-TF-157`–`161`**（VE-19）。五行里全程追踪自己的行为已迁入 UC-VE-002 `AT-VE-189`–`192` 与 UC-VE-004 `AT-VE-193`–`195`，对应：`AT-TF-157` → `AT-VE-189`；`AT-TF-158` → `AT-VE-193`；`AT-TF-159` → `AT-VE-190`；`AT-TF-160` → `AT-VE-191`（投影半边）与 `AT-VE-194`（信号规则半边）；`AT-TF-161` → `AT-VE-192`（投影半边）与 `AT-VE-195`（信号半边）。UC-TF-005 原行改为引用，或整行删去、只在表后留一句指向 VE 两份 UC；AT 盘点 ① 里 `AT-TF-157`–`161` 几行随之移行或删去。测试 Covers 改点新号归 [code/32](../code/32-ve-comments-carry-counts-non-verbatim-quotes-and-stale-at-pointers.md)。这一步也可由 TF 组（票 08）做，两边别重复。
7. **ADR-0077 缺一条前向指针**（VE-15 相邻）。ADR-0077 决定三「运营接入认证属接入渠道实例半边、未登记」与「一律 403 `ACCESS_CHANNEL_NOT_CONFIGURED`」已被 ADR-0100 决定四点名覆盖（「覆盖面是 ADR-0085 Decision 一那一族登记端点与 ADR-0077 那一族目录查阅端点」），ADR-0077 的 Status 行只有 `Accepted`。照 adr/README「部分停用」补指针：停用方是 ADR-0100，不是本票的 ADR-0247。
8. **ADR-0078 缺一条前向指针**（VE-15 相邻）。ADR-0078 决定一「`/customer-tracking-view` 明确排除」那段的理由「那正是 `PAR-INT-01` 拥有的实例半边语义」已被 ADR-0139 决定一收窄（首方渠道不归 `PAR-INT-01`）；ADR-0139 的部分停用清单没有列 ADR-0078。排除这一结论照旧成立（客户维是调用方自己的身份主张），所以挂的是「理由收窄」一类的注记，不是结论停用。

## 完成记录（2026-10-21，分支 claude-dc04）

### 复核

审计钉 `6b0074d0`，本票按 main `205eb254` 逐条复核。审计之后 VE 的 CONTEXT、8 份 UC、`internal/visibilityexception` 与 VE 演示种子都没有提交（`git log 6b0074d0..main` 对这些路径为空），22 条现象只有 VE-22 已被消解，其余全部仍在。02 已立的 VE 代码票（code/05–10）与追加进的既有票（demo-chain-gaps/117、123、124、135）逐一核对了票号与审计条目，对得上。`ve-customer-view-per-projection-version/01` 的裁定前提在代码里不成立，02 已补指针，缺陷归 code/05，本票不再动它。

### 审计条目 → 去向

| 条目 | 类 | 去向 |
|---|---|---|
| VE-01 客户视图原地覆盖、按投影换代 | C | 已有代码票 [code/05](../code/05-customer-tracking-view-overwrites-published-versions.md)。文档是现行规则（CONTEXT「原已发布版本保留」「不与内部投影版本一一对应」，ADR-0065 Consequences 末条），不改 |
| VE-02 ETA 维无判断也答展示 | C | 已有代码票 [code/06](../code/06-customer-view-shows-dimensions-without-a-visible-judgement.md)；它交来的文档半边已改：UC-VE-008「客户追踪视图」一行写明四维三态与 `contentRef` 指名披露策略允许展示的内容集、不是内容本身 |
| VE-03 被替代条目不在投影版本里 | C | 已有票 [demo-chain-gaps/117](../../demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md)。文档是现行规则（ADR-0065 决定七），不改；裁向写进 117：连同 CONTEXT 前半一起落 |
| VE-04 分叉先按业务时间裁决 | B | 代决 [ADR-0245](../../../docs/adr/0245-a-forked-supersession-chain-is-never-ordered-by-business-time-and-stays-pending-in-the-projection-version.md)；CONTEXT 分叉一条与 Lifecycles 一行、UC-VE-002 主流程第 5 步与 `AT-VE-043` 已改；ADR-0065 挂读法注；代码票 [code/30](../code/30-forked-supersession-is-judged-by-business-time-and-not-kept-pending.md) |
| VE-05 视图未形成与无权合成一格 | B | 代决 [ADR-0246](../../../docs/adr/0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md)；CONTEXT 账户核对一条与 Lifecycles 一行、UC-VE-008 元数据、结果语义、主流程第 8 步已改，新立 `AT-VE-188`；部分停用 ADR-0223 决定三一句（委托已接受、视图未形成、包裹唯一归属请求方时），同时回答其越权风险点 3，两头挂指针；代码票 [code/31](../code/31-customer-tracking-query-cannot-say-no-visible-result-for-its-own-parcel.md) |
| VE-06 `AT-VE-169` 重号 | A | 已改：UC-VE-003 的 ETA 那一行改号为 `AT-VE-187`（全仓 AT-VE 最大号 186 之后的下一个；该行没有任何代码引用）；UC-VE-008 那行保留 169（测试与十余处注释都点它）。AT 盘点 ④ 移行、① 补 169 |
| VE-07 CONTEXT 缺「入账不派生」 | A | 已改：CONTEXT「追踪投影与里程碑」立「入账不派生」一条，列三族情形（源上下文已判同层冲突的那一份、没有可投对象的移动事实、补派生时没有可补的移动事实），各引 ADR-0197、0205、0224、0226；Lifecycles 投影一节补一行；UC-VE-002 结果表补「入账不派生」一格 |
| VE-08 关务同层冲突不形成待确认与信号 | C | 已有票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md)；UC-VE-001 `AT-VE-006` 表后注明现状缺口、指向 124，预期结果不变 |
| VE-09 信号无事实依据与因果条件 | C | 已有票 [demo-chain-gaps/135](../../demo-chain-gaps/issues/135-departure-gate-signals-carry-causal-condition-and-fact-basis.md)；CONTEXT 不改；UC-VE-004 那段说明补上「与输入语义和 `AT-VE-065` 相反」并指向 135（135 落地时删去） |
| VE-10 袋被当成一件包裹投影 | C | 已有票 [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md)；UC-VE-002 表后注明 `AT-VE-046` 的现状缺口、指向 123 |
| VE-11 ETA 存储整行覆盖 | C | 已有代码票 [code/07](../code/07-eta-store-overwrites-history-and-mixes-sources.md)；文档不改 |
| VE-12 披露待授权装不下草稿 | C | 已有代码票 [code/08](../code/08-disclosure-awaiting-authorization-carries-no-draft.md)；文档不改 |
| VE-13 事实类型词无单一出处 | A | 已改：VE CONTEXT 立「事实类型词」一节（26 个词、源上下文、源事实及分格），「已接受事实必须携带事实类型」一条改为引用它、写明含义归源上下文；UC-VE-002 输入表引用。GLOSSARY 与 TF CONTEXT 的引用交横切第 5 条 |
| VE-14 「客户查询授权」归给 PC | A | 已改：UC-VE-008 权威依据 `ACCOUNT` 行拆成 `ACCOUNT`、`ATTRIBUTION`、`GRANT` 三行，授权指向 ADR-0139 与 `PAR-INT-13`。CONTEXT-MAP 那条边交横切第 1 条 |
| VE-15 ADR-0076 决定三与 ADR-0100 矛盾 | B | 代决 [ADR-0247](../../../docs/adr/0247-the-operations-tracking-read-is-admitted-through-the-operator-channel-family.md)：部分停用 ADR-0076 决定三两句与 Consequences 一句、ADR-0100 决定四标题句的覆盖面扩到运营查阅面，两头挂指针。代码已是这样，不立代码票。ADR-0077、ADR-0078 两条相邻指针交横切第 7、8 条 |
| VE-16 演示冲突信号登成 `CUSTOMS_HOLD` | C | 已有代码票 [code/09](../code/09-demo-conflict-signal-rule-is-registered-as-customs-hold.md)；文档不改 |
| VE-17 追偿动作缺「补充主张」 | C | 已有代码票 [code/10](../code/10-recovery-action-kinds-lack-supplementary-assertion.md)；有票就不在 UC-VE-007 写「首发只两种」，文档不改 |
| VE-18 GLOSSARY 缺词、异名、未定义词 | A | 交横切第 2、3、4 条（全在 GLOSSARY；第 2 条连带 VE CONTEXT 两条词条改为引用） |
| VE-19 VE 验收暂挂在 UC-TF-005 | A | 已改：UC-VE-002 新立 `AT-VE-189`–`192`、UC-VE-004 新立 `AT-VE-193`–`195`，两份 UC 的「暂挂」段落改为指向新号；AT 盘点 ① 加行。UC-TF-005 原行交横切第 6 条；测试 Covers 改点新号进 [code/32](../code/32-ve-comments-carry-counts-non-verbatim-quotes-and-stale-at-pointers.md) |
| VE-20 Go 注释里的计数 | A | 改的是代码注释，本票不改代码：立 [code/32](../code/32-ve-comments-carry-counts-non-verbatim-quotes-and-stale-at-pointers.md) |
| VE-21 Go 注释里的假引文与错指 | A | 同上，并进 code/32 |
| VE-22 Status 行把同一次停用写两遍 | A | 已由票 [01](./01-adr-suspension-chains.md) 消解（`3221ed51f`「合并 Status 行里写两遍的停用与接受出处」）：ADR-0078、ADR-0104 的那次部分停用今天只在索引注记里写一遍 |

去向计数：已改文档 5 条（VE-06、07、13、14、19，其中 13、14、19 另有交横切）；代决立 ADR 3 条（VE-04、05、15）；已有代码票、只核对或只补现状注 10 条（VE-01、02、03、08、09、10、11、12、16、17）；只交横切 1 条（VE-18）；A 类落在 Go 注释、立代码票 2 条（VE-20、21）；已由某票消解 1 条（VE-22）。

### 代决与越权风险点

用户授权「你作为业务和系统专家自决」，三份 ADR 都代 VE owner 决，Status 写「待 VE owner 复核」。号段 0245–0248 用了 0245–0247，0248 未用。

- **ADR-0245（VE-04）**：替代链分叉一律无法裁决，不进 `ResolveByBusinessTime` 或任何裁决函数。分叉以链头计。前身与各后继留在投影版本内、标明分叉关系、不参与派生，那一条事实流答信息待确认，并形成适用异常信号。分叉关系随投影版本存下。分叉不自动解开，走信号、案件与处置请求。越权风险点：
  - 永不让 VE 择一；
  - 源侧没有解开分叉的形状，待确认可能长期持续；
  - 以链头计比今天的检测宽，「再更正一次就恢复」不再成立；
  - 客户视图那一维的待确认依赖 code/06；
  - 跨源冲突仍没有发现机制。
- **ADR-0246（VE-05）**：客户查询的业务结果分三格，`CURRENT_VIEW` / `NO_VISIBLE_RESULT` / `VIEW_NOT_FOUND`。只在读不到视图时经 PS 归属反查口问一次「这件归不归你」。委托接受之前与标识冲突对外仍合并。部分停用 ADR-0223 决定三「在那之前，客户查自己的号也答 `VIEW_NOT_FOUND`，与不存在不分」一句，同时回答其越权风险点 3。越权风险点：
  - 查询面多一次 PS 读（待 PS owner 会签）；
  - 对归属方说出了「已归属」一位（见末条）；
  - 时延差不做常数时间；
  - 投影形不成时 `NO_VISIBLE_RESULT` 会长期不变；
  - 应赶在首方契约基线发布前落地；
  - 包裹所在委托已`已接受`的声明方查得 `VIEW_NOT_FOUND` 而非 `NO_VISIBLE_RESULT`，推得出「这件不唯一归我」（例如多重声明、归属转走、委托在视图形成前转为`已取消`）；包裹号 `parcels[].parcelId` 由服务端现签、只交回提交方，不构成枚举口；
  - `NO_VISIBLE_RESULT` 是首方客户得知委托已接受的第一条、也是唯一一条路（首方客户族今天没有读委托状态的口），这一条比副通道更重。
- **ADR-0247（VE-15）**：运营追踪查阅面归操作者渠道族，能力面「主数据与运营查阅读」，答复照 ADR-0100 决定四三格；ADR-0055 Decision 五两项未决对它不适用。越权风险点：
  - 追踪查阅与目录查阅共用一格授予（待接入身份 owner 会签）；
  - VE 其他运营查阅口按今天的装配一并归族；
  - 「两项未决不适用」是顺带作出的。

本票另有两处判断，不立 ADR：
- VE-03 照 ADR-0065 决定七写进票 117 的裁向。
- VE-13 把类型词表立在 VE CONTEXT，而不是审计备选里的 UC-VE-002 或各源 CONTEXT。理由是 CONTEXT 是现行语言的唯一出处，词是 VE 消费侧、租户可见的映射键。代价是源上下文增减语义分格时，要记得来 VE CONTEXT 改表。

### 代码票编号

03、04、05 三组并行时都从 code/26 起立票，撞号。协调方分配：03 用 code/26–27，05 用 28–29，本票用 **code/30–32**。本票原立的三张依次改为 30（分叉）、31（客户查询当前无可见结果）、32（VE 注释），文件名、票内标题与全部引用处（ADR-0245、ADR-0246、UC-VE-002、UC-VE-008、AT 盘点、票 117、本票）已一并改；全仓 grep 不再有本票的旧号。

### 检查

- `go run ./tools/adr-index`：在 scratchpad 的 `git archive HEAD` 副本上重生成后 `-check` 退 0；索引只多出 0245–0247 三行，0065、0076、0100、0223 四行的注记各多一段。分支上的 `docs/adr/README.md` 保持 main 的版本，落地时由协调方重生成。
- `go run ./tools/at-coverage-counts -check` 退 1（只报总览计数过期，没有格式错）：本票只移行、加行，总览计数由协调方进 main 时重算。
- 本分支改过的全部 `.md` 里的相对链接逐条解析，都能解析。
- 被停原句逐字核：0065、0076、0100、0223 新加注记里引被停记录的每一句，都在被停记录正文（去掉 Status 行）里逐字找到；报不中的只有别份记录的原词（ADR-0100 的能力面名「主数据与运营查阅读」）与注记自指的前文。
- `git diff main --stat`：只动了 VE CONTEXT、`docs/application/visibility-exception/` 下五份 UC、ADR-0245–0247 及 0065、0076、0100、0223 的 Status 行与 Links、AT 盘点、本票、code/30–32、票 demo-chain-gaps/117。没有动代码、种子、GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/` 与别组文件。
- 新写的 CONTEXT、UC 文字不用行号、跨文件计数与跨文件位置引用。CONTEXT 里两处同文件指称用的是引文（「多个有效事实冲突时」那一条、「冲突仍无法裁决时」那一条）。

### 提交

分支 `claude-dc04`，已 rebase 到 main `73a2c4a7`，各笔见 `git log main..claude-dc04`：认领；VE 代码票；ADR 与指针；CONTEXT；UC；AT 盘点；票 117 裁向；本记录；完工状态；评审修订四笔与本记录更新。

## 评审记录

### 隔离评审一轮（2026-10-21）与作者回应

阻断：

- **B1 ADR-0246 实为部分停用 ADR-0223 决定三一句，不只是回答越权风险点 3**：已修。ADR-0246 前段改为「部分停用 ADR-0223 决定三一句，同时回答其越权风险点 3」，Context、决定五、Consequences 与 Links 同步。ADR-0223 的注记由「已答」改为部分停用，逐字引被停原句「在那之前，客户查自己的号也答 `VIEW_NOT_FOUND`，与不存在不分」，写适用场景：委托已接受、客户视图未形成、包裹唯一归属请求方。注记里写明接受之前不变、越权风险点 3 同时由它回答、其余不变；Links 同步。
- **B2 C 类「文档不迁就代码」的几处 UC 没有现状注**：已修，照 UC-VE-001、002 已有的写法逐处加，CONTEXT 不动。
  - UC-VE-008：`AT-VE-161` → code/05；`AT-VE-158` 与结果语义「客户追踪视图」一行 → code/06；「当前无可见结果」一行与 `AT-VE-188` → code/31。
  - UC-VE-002：`AT-VE-044` 与主流程第 5 步「被替代条目保留」→ 票 117，顺带注明 `AT-VE-192` 的回指前版在真库上同病；`AT-VE-043` → code/30。

建议：

- **N1**：已修。UC-VE-008「目标与边界」首段的结果集合改为新口径。「身份谱系待确认」只留在结果语义「信息待确认」一行（归维上，ADR-0246 决定四）；元数据「待判断结束」只留外部标识或客户归属冲突，其余待确认指向那一行。
- **N2**：已修。ADR-0246 补越权风险点 6，写出现条件（多重声明或归属转走，只对自己声明过的包裹成立）、不构成枚举口的理由（首方口件号由服务端现签、只交回提交方）与续办。决定五「标识冲突」的理由改为只对非声明方成立，并写明对声明方交出的只是「不唯一归我」这一位。
- **N3**：已修。CONTEXT「入账不派生」第二条里补派生那一句句首加「段不在册的那一条，」。
- **N4**：已修。`docs/adr/README.md` 恢复为 main 的版本；adr-index 的生成与核对改在 scratchpad 副本上做。
- **N5**：已修。code/31 的规模与修法列上 `scripts/demo-seeds/README.md`「之前查它答 `VIEW_NOT_FOUND`」一句，与 `docs/design/synthetic-demo-journey-script.md` 首方查询表两处同步。
- **N6**：已修。
  - CONTEXT 事实类型词表末句改为「本表之外的词不由任何源产出」。
  - ADR-0247「九天后」改「十天后」。
  - AT 盘点 `AT-VE-191`、`194` 的叠括号合一，同组 `AT-VE-189`–`195` 一并合。
  - ADR-0247 Context 的口列表删掉 `/visibility-catalogues`，另注一句它属 ADR-0077 目录族、已在 ADR-0100 决定四覆盖面内。
- **改号**：已修，见「完成记录」中「代码票编号」一节。
- **跨组约定（票 10 第 5 条）**：本票没有在 CONTEXT 的单条规则上挂票号；差距注都在 UC 的验收表后。

未采纳：无。

### 隔离评审二轮（2026-10-21）与作者回应

第一轮的 B1、B2、N1 与改号经核成立。本轮：

- **X1 ADR-0246 正文三处事实错**：已修。
  - (a) 决定五与越权风险点 6 的「件号由服务端现签」改为包裹号（`parcels[].parcelId`）。现签出处写 `CustomerSubmissionIntake.mintInternalIdentities` 与 ADR-0223 决定二，并注明按客户件号（`customerParcelReference`）定位今天不支持（ADR-0223 决定四、票 119）。code/31 Status 里的「件号」同改。
  - (b) 越权风险点 6 的「出现条件只有这两种」改为「例如」，补上第三种：委托在视图形成前由`已接受`转为`已取消`（`FindCurrentAcceptedByParcel` 只认已接受状态）。
  - (c) 越权风险点 2 如实写成：首方客户今天没有别的口能得知接受，`NO_VISIBLE_RESULT` 是第一条、也是唯一一条路，这条风险因此更重。依据是 `cmd/parcel-api` 里挂 `customerClientFace` 的各口没有读委托状态的，ADR-0223 越权风险点 4、决定四。决定五与越权风险点 6 的续办改为「稍后再查，或请租户客服核对」。
- **1「已受理」改为「所在委托已`已接受`」**：已修，改了 UC-VE-008 `AT-VE-188`、ADR-0246 越权风险点 6、code/31 的现象与修法第 6 条演示表新行、AT 盘点 `AT-VE-188` 行。code/31 Status 的绕法同步去掉「按受理状态」。
- **2**：已修。UC-VE-008 `AT-VE-152` 句末加「；对外与无权查询同一答复（ADR-0246）」。
- **3**：已修。元数据「未形成结束」后半句改为「（唯一归属方答当前无可见结果，其余同无权查询）」；`AT-VE-168` 场景改为「视图读取、客户归属读取或授权判断失败」，与元数据用词对齐。
- **4**：已修。code/31 修法第 4 条补一句：`ports.ParcelCustomerAccountView` 头注与 `customer_account_lookup.go` 的「多于一个候选是未决/待确认」只管派生一侧，查询一侧答 `VIEW_NOT_FOUND`，头注要分清两侧。
- **5**：已修。`AT-VE-188` 在 UC-VE-008 验收表里挪到 `AT-VE-173` 之后。
- **6**：已修。ADR-0246 Consequences 改为指称被停那一句，逐字原句只留在 Links（与 Context 的取证引文）。
- **7**：已修。VE CONTEXT 头部加「规则与实现的差距见各 UC「给开发的交接」。」，与 NR、NO 两份 CONTEXT 逐字一致。

rebase 到 main（先 `f329f505`，再 `73a2c4a7`）时有冲突。`docs/adr/README.md` 取 main 的版本，分支上不提交重生成。AT 盘点文末追记段与 05 的追记段两段都留。

未采纳：无。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dc04` tip `f1477842f` 在 main `73a2c4a7` 上快进；两轮隔离评审的阻断全部消掉（第二轮 ADR-0246 三处事实错已改：包裹号而非件号、出现条件补已取消、首方客户没有别的口得知接受）。
- 协调方随后在 tip 上单独提交：重生成 `docs/adr/README.md`、重算 AT 盘点总览计数；两者 `-check` 退 0。机制清点重生成无变化；`go build ./...` 干净。本票只动文档，未跑全量真库测试。
- 去向：代码票 code/30、31、32；票 117 补了现状注与 `AT-VE-192` 同病一笔。
