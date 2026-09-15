# CC CONTEXT「税费付款核对」没有「未提供必须明确记录、规则要求而缺失保持未决」那句：付款人一格的代码注释、`0020` 头注与 sa-cc/12 裁决 2 引的都是「监管处置决定」词条——补句并改正引文归属

Category: chore
Status: in-progress——**2026-09-15 10:0x 通道 3 按通道 1 派单 task-7d92552e 认领**（`/implement`，纯 docs 零代码 diff；分支 `mcp3-sacc21` 基 main `3a21dab7`，隔离树 `$env:TEMP\idp-parcel-mcp3-sacc21`）。此前 ready-for-agent——**2026-09-14 22:1x 通道 1 按用户「你是业务和系统专家，自决」代裁（CC owner 口径），「要裁的」写入下方「裁决」节**：候选 A-1——与监管处置决定**同形复用**那句、落 Rules「税费、放行与案件闭环」段、主语是「付款人、金额、币种、业务时间等由来源提供或真实程序要求的维度」，并补第三格「程序尚未登记是否要求时核对不进行」；13 处引用**零改动**（取证量得 A-1 下引文与归属全部变准确）。纯 `docs/**` + 两票面一句，不动代码。此前 draft——2026-09-14 16:0x 通道 1（接管会话）立票（sa-cc/12 补评审 ← 通道 3 Standards 非阻断 ①；归 CC owner）。只写票面未动代码、未动 `docs/**`；取证锚 main `01974923`
Blocked by: 无（12 已进 main；要裁的已裁，见「裁决」）

## 缺口（取证于 `01974923`，逐符号名）

- `docs/domain/customs-compliance/CONTEXT.md`：「未提供或不适用必须明确记录，规则要求但缺失时保持未决」这句只出现在**监管处置决定**词条（数量 / 期限 / 条件 / 证据要求那几维）；**税费付款核对**词条只有「来源提供或真实程序要求的付款人、金额、币种、业务时间等维度」，Rules「税费、放行与案件闭环」段没有这句。
- 引它当作税费付款核对规则的地方（`git grep` 于 `01974923`）：`internal/customscompliance/domain/funds_fact_payer.go` 包头注；`internal/customscompliance/application/reconcile_duty_payment.go` `ReceiveFundsFact` / `DutyReconciliationReason` 头注；`reconcile_duty_payment_test.go` `TestThePayerDimensionIsJudgedByTheProcedureRule` 头注；`migrations/customs_compliance/0020_funds_fact_payer_may_be_unprovided_and_payer_rule.sql` 头注；`internal/customscompliance/adapters/http` `writeDutyReconciliationAnswer` 头注；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)「裁决」2 的 10:5x 改口（「CC CONTEXT 原词是『规则要求但缺失时保持未决』」）。
- 实现没有错：「来源提供**或**真实程序要求」已蕴含同一原则，三停格（要求而未提供 → 未决；不要求 → 明确记「未提供」；未登记 → 未决）与监管处置决定那句一致。错的是**归属**：付款人维的这条规则今天只活在票面与注释里，CONTEXT 上没有它——按 AGENTS.md「改领域语言、不变量 → 先改 CONTEXT，再改引用它的 UC / 代码」与「单一权威」，注释引的应是 CONTEXT 里真有的句子。

## 语言从哪里来

- CC `CONTEXT.md` 税费付款核对词条「来源提供或真实程序要求的付款人……维度」——「或」字是这条规则的种子，没有展开成「没提供怎么记、要求而没有怎么停」。
- 同文监管处置决定词条那句——CC owner 若认为两处是同一原则，补句时直接同形；若付款人维另有措辞，写付款人自己的。

## 做法（待裁后写实）

1. CC `CONTEXT.md` 税费付款核对词条或 Rules「税费、放行与案件闭环」段补一句（原词由 CC owner 定，见「要裁的」1），并在 `GLOSSARY` 若有对应词条处同步。
2. 上列各处注释与 `0020` 头注把引文归属改成新句所在词条；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)「裁决」2 原文不改写（历史），在其下追一句「引文归属见 sa-cc/21」。
3. 不改任何行为、不改测试断言；`go build` / `go vet` 零变化即可。

## 红线

- 只补语言不改规则：三停格与 12 裁决 2 的语义一字不动。
- 不动 `internal/**` 的非注释行；不新增 outcome / reason。
- 不引行号、不数别处。

## 完成判据（待裁后写实）

1. CC `CONTEXT.md` 税费付款核对处有那句（或 owner 定的同义句）；`git grep` 该句在 CONTEXT 命中 ≥ 2 处（监管处置决定 + 税费付款核对）或付款人专句命中 1 处。
2. 上列注释与 `0020` 头注的引文归属全部指向新句所在词条；`git diff --stat -- internal/ migrations/` 只含注释行（用 `git diff -w --ignore-blank-lines` 与 `go build` 零变化核）。
3. UC-CC-009（税费付款核对用例）若引到这一维，同步指向 CONTEXT 新句。

## 地盘

`docs/domain/customs-compliance/CONTEXT.md`（一句）、`docs/domain/GLOSSARY.md`（若有）、`docs/application/customs-compliance/UC-CC-009-*`（若引）、`internal/customscompliance/{domain,application,adapters/http}` 注释行、`migrations/customs_compliance/0020_*` 头注、本票面与 12 票面一句。

## 要裁的

1. 那句写成什么、放在哪——归 CC owner：与监管处置决定同形（「未提供或不适用必须明确记录，规则要求但缺失时保持未决」直接复用于付款人维），还是付款人自己的措辞（如「付款人由来源提供或按真实程序登记的规则要求；来源未提供时明确记录为未提供，规则要求而未提供时核对保持未决，规则未登记时核对不进行」）。裁前注释保持现状。

## 裁决（2026-09-14 22:1x 通道 1 代裁，CC owner 口径；依据是通道 6 21:2x 取证条，钉 `bccb60a1`）

1. **那句写成什么、放在哪——候选 A-1：同形复用，落 Rules「税费、放行与案件闭环」段。** 在该段第 ② 条（「外部资金事实只有在能够按真实程序和适用范围关联到当前监管核定税费时……分别表达，不能实现为一组互斥总状态」）**之后**新增一条：「**付款人、金额、币种、业务时间等由来源提供或真实程序要求的维度：未提供或不适用必须明确记录，规则要求但缺失时保持未决；程序尚未登记是否要求某一维度时，核对不进行、保持未决，不替任何程序预设要求。**」为什么是这句、这里：(i) 它与监管处置决定条的原则**就是同一条**（12 三停格与处置维三格逐格对上，取证 5 已核），同形复用让读的人一眼认出同一纪律，不为付款人另造一套措辞；(ii) 主语取「付款人、金额、币种、业务时间等……维度」而不是付款人专句——税费付款核对词条本就把这四样并列为「来源提供或真实程序要求的」维度，规则该覆盖整组，不该只长付款人一格；(iii) 落 Rules 段而不是 Language 词条——13 处引用里 3 处自称「Rules 一句」，A-1 让 13 处**引文与归属全部变准确、零改动**（取证 5 实测；A-2 要改 3 处归属，B 要改 11 处）；(iv) 第三格「程序尚未登记 → 核对不进行、保持未决」今天只活在代码 `PayerRequirementNotConfigured` 与 12 裁决 2 里，CONTEXT 上没有——补进同一句，「单一权威」由此成立（放行门禁那一条 ④ 的「规则未配置」是门禁规则，与这里的付款人要求规则是两条规则，各自一句、不合并）。引号内那半句「未提供或不适用必须明确记录，规则要求但缺失时保持未决」**逐字保留**，`git grep` 于 CONTEXT 命中由 1 变 2（完成判据 1）。
2. **UC-CC-009「启动条件」那句**（「来源未提供且程序不要求的维度明确记录，不猜测补齐」——它今天只写了第二格）改为三格齐全并指向 CONTEXT 新句：「来源未提供且程序不要求的维度明确记录为未提供；程序要求而来源未提供时核对保持未决；程序尚未登记是否要求时核对不进行——见 CONTEXT Rules『税费、放行与案件闭环』」。UC 引 CONTEXT 是正常方向（UC-CC-008 对处置维已是这么写的）。
3. **归属更正两处（历史不改写，追一句）**：票 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)「语言从哪里来」第二条把「来源未提供且程序不要求的维度要『明确记录』」记为 CC CONTEXT——出处是 UC-CC-009「启动条件」，CONTEXT 零命中（取证 2）；12「裁决」2 的 10:5x 改口「CC CONTEXT 原词是『规则要求但缺失时保持未决』」——落地前那句只在监管处置决定条，本票落地后在税费段也有。两处各在 12 Comments 追一句「归属见 sa-cc/21 裁决 3」，原文不动。
4. **不做**：GLOSSARY「税费付款核对」词条不改（CONTEXT 是权威、词条要点已覆盖三态语义，付款人维不必在词条重复）；`internal/**` 与 `migrations/**` **零 diff**——13 处注释与 `0020` 头注在 A-1 下全部准确，作者逐处复核后在完成记录里列「13 处逐条核过、零改动」即可，不为改而改；`apps/admin-web` 零命中，不动。
5. **完成判据写实**：(1) `git grep -c '未提供或不适用必须明确记录，规则要求但缺失时保持未决' -- docs/domain/customs-compliance/CONTEXT.md` = 2（钉本笔）；新句在 Rules「税费、放行与案件闭环」段、位于「外部资金事实只有在……」那条之后。(2) `git diff --stat -- internal/ migrations/ apps/` 空；13 处引用逐条核过的清单在完成记录。(3) UC-CC-009「启动条件」含三格与 CONTEXT 指向。(4) 12 Comments 两句追加；本票完成记录同笔。
6. **能力边界**：裁的是措辞归属与落点；读过本票、12 裁决 2 与补评审 ①、通道 6 取证全文（CONTEXT 三段原文经引文）；**没读** CONTEXT 全文其余部分、UC-CC-009 全文、ADR-0137 正文。作者落句时若发现 Rules 段已有更贴的位置，以段内逻辑为准并写进判断项。

## 参照

[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（裁决 2、完成记录、15:47 补评审 Standards ①）；CC `CONTEXT.md` 税费付款核对与监管处置决定两词条；AGENTS.md「改文档」第一条与「单一权威」红线。

## Comments

- 2026-09-14 16:0x · 通道 1（接管会话）：立票（sa-cc/12 补评审 ← 通道 3 Standards 非阻断 ①：「建议 CC owner 在 CONTEXT 税费付款核对 Rules 补一句再被引（另立文档票，不改代码）」）。只写票面，未动代码与 `docs/**`。能力边界：`git grep` 核过那句在 CONTEXT 只命中监管处置决定一处、在代码命中 `funds_fact_payer.go` / `reconcile_duty_payment.go` / 其测试三处；`0020` 头注与 HTTP 头注两处按评审原文列入，未逐字重读。
- **2026-09-14 21:2x · 取证 ← 通道 6 · 钉 `bccb60a1`**（task `edcfe85c`；隔离树 `$env:TEMP\idp-parcel-mcp6-sacc21` 切 `mcp6-sacc21-evidence` 基 `bccb60a1`；只读 `docs/**` 与代码，只写本条；不裁措辞、不写建议）。下列引文全部逐字取自 `bccb60a1`，定位一律用词条名 / 小节名 / 符号名；所有计数锚 `bccb60a1`。
  - **1 · CC CONTEXT 原文**（`docs/domain/customs-compliance/CONTEXT.md`；该目录 `git ls-files` 只有这一份文件）
    - **监管处置决定** · Language 词条全文：「监管机构针对明确对象或范围及依据作出的隔离、移交、销毁、退运、没收或其他处置决定。数量、期限、条件和证据要求只在来源实际提供或适用程序要求时构成决定维度；未提供或不适用必须明确记录。它是监管机构的决定，不等于物理处置已经执行或完成。」——词条里只有「未提供或不适用必须明确记录」，**没有**「规则要求但缺失时保持未决」。
    - **监管处置决定** · Rules「监管凭证、限制与处置」段那一条及前后各一句。前一句：「只有作用于当前对象和拟执行动作的全部阻断性限制均已解除，相应动作才可继续。尚未放行或存在方向性限制时，必须阻断其明确约束的出库、装载出发、跨关务区域移动或交付，但不因此阻止接收、隔离、测量、查验协作或已经授权的处置执行。」**本句**：「监管处置决定必须具有可唯一关联的决定身份、来源、法律动作语义和明确适用对象或范围。数量、期限、条件和证据要求只在来源实际提供或适用程序要求时作为核对维度；未提供或不适用必须明确记录，规则要求但缺失时保持未决。实际隔离、移交、开封、重封、销毁、退运装载或其他物理执行由执行事实证明，不能由决定本身推导。」后一句：「监管处置允许形成部分执行事实、执行失败事实、数量差异和再次执行事实。关务侧必须将事实与决定及适用证据规则逐范围比较，分别形成处置执行核对已覆盖、部分覆盖、差异、事实冲突或证据不足；仓库或外部执行方标记完成不能自动关闭执行待办、监管义务或关务案件。」
    - **税费付款核对** · Language 词条全文：「`customs-compliance` 将监管核定税费与银行、支付或财务系统提供的外部资金事实，按明确申报范围、法定义务以及来源提供或真实程序要求的付款人、金额、币种、业务时间等维度进行的版本化比较判断。它分别判断覆盖状态、差额和事实有效性，不形成实际付款、客户回收或监管放行。」——**没有任何一句讲「未提供怎么记」或「要求而缺失怎么停」**。「付款人」一词全 CONTEXT 只此一处（`git grep -c '付款人' -- docs/domain/customs-compliance/CONTEXT.md` = 1）；Rules 与 Lifecycles 里出现的是「实际付款方」（关务参与方角色快照词条、Rules「参与方、资料与合规判断」段、Rules「税费、放行与案件闭环」段首条）。
    - Rules「税费、放行与案件闭环」段提到税费付款核对或付款方的每一句：①「监管核定税费、实际付款、付款失败、资金退回或付款撤销、监管补缴、税费付款核对、运营企业向客户形成的代垫回收，以及监管放行必须分别保存并由各自责任方拥有。法定义务人、实际付款方和最终承担费用的客户可以不同，不能互相推导。」②「外部资金事实只有在能够按真实程序和适用范围关联到当前监管核定税费时，才能参与税费付款核对。金额相同、同一包裹、同一客户或同一时间不能单独证明付款覆盖；部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断。覆盖状态（无覆盖、部分覆盖、已覆盖）、差额状态（无差额、不足、超额或待确认）和有效性状态（有效、失效、冲突或待确认）分别表达，不能实现为一组互斥总状态。」③「放行门禁核对必须绑定当前有效的监管程序、明确申报范围、拟执行动作、适用监管边界、税费付款核对、限制、处置和其他前置条件判断。门禁满足不生成放行，也不能复用于其他动作或监管边界；门禁未满足也不能删除已经接收的放行结果。放行结果仍由 `UC-CC-006` 接收和解释。」④「「税费付款」这一道门禁怎么读税费付款核对，是按监管程序登记进来的规则——对覆盖、差额、有效性三态各自的接受集合，或声明不构成前置条件；待确认与冲突不可登记为接受；未登记规则时该道门禁答「规则未配置」，不取任何默认折法（ADR-0137）。」——④ 是该段唯一一句「未登记 → 规则未配置」，主语是放行门禁那道规则，不是付款人维。该段其余各条不提核对与付款人。
    - 外部资金事实相关（CONTEXT 无「集成」小节，下列取自 Rules 同段、Lifecycles「税费付款与放行门禁」、Boundaries and ownership）：Rules 同段「监管核定及法定义务由本上下文拥有，真实付款事实由银行或支付系统拥有，实际代垫成立判断和客户代垫回收由 `settlement-accounting` 通过 UC-SA-001 形成；关务服务费用另按独立费用规则形成。各层金额关系遵循 ADR-0007，不得共同维护一个可覆盖结果。」；Lifecycles「外部资金事实接入 → 税费付款核对：按真实程序逐范围分别形成覆盖状态、差额状态和有效性状态；资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态。」；Lifecycles「已接受监管核定税费，或真实程序明确当前范围无需付款 → 形成税费付款协作事项：固定当前税费义务依据、法定义务范围、付款要求来源、责任交接目标和核对入口；没有税费结果或规则依据时保持未决，不形成支付指令或实际付款事实。」（税费族在 CONTEXT 里**唯一**的「保持未决」，主语是协作事项、条件是「没有税费结果或规则依据」）；Boundaries「`settlement-accounting` 通过 `UC-SA-001` 拥有实际代垫成立判断、客户关税代垫回收及调整……；银行、支付或财务系统拥有实际付款、付款失败、追加付款、资金退回、付款撤销和外部资金事实更正。……本上下文只拥有监管核定、法定义务、税费付款核对和放行门禁判断及其与外部资金事实的可追溯关系。」
    - **`git grep -n '明确记录\|保持未决' -- docs/domain/customs-compliance/` 于 `bccb60a1`：4 命中，全在 CONTEXT.md**（数本身是论点）：① Language **关务执行协作事项**「……以及来源实际提供或适用程序要求的数量、期限、条件和执行/核对要求；未提供或不适用的内容必须明确记录，不能猜测补齐。」② Language **监管处置决定**「……未提供或不适用必须明确记录。」③ Rules「监管凭证、限制与处置」**监管处置决定**条「……未提供或不适用必须明确记录，规则要求但缺失时保持未决。」④ Lifecycles「税费付款与放行门禁」**税费付款协作事项**条「……没有税费结果或规则依据时保持未决……」。同时含两半的只有 ③；「规则要求但缺失」在 `docs/domain/` 只命中 ③（`git grep -c` = 1）；「未提供」全 CONTEXT 4 命中（上列 ①②③ + Rules「申报就绪、授权与提交」段关务执行协作事项条「未提供或不适用的字段不能使用默认值补齐」），**税费族与凭证族零命中**。
  - **2 · GLOSSARY / UC-CC-009**
    - `docs/domain/GLOSSARY.md`：**付款人**——零命中（`git grep -n '付款人' -- docs/domain/GLOSSARY.md` 无输出）。**外部资金事实**——无词条（`git grep -n -E '^#+ .*资金'` 只命中「### 资金退回」「### 资金冻结」），作为普通词出现在监管核定税费、监管补缴要求、客户代垫回收、实际代垫成立判断等词条正文。**税费付款核对**——有词条，全文：「`customs-compliance` 将监管核定税费与银行、支付或财务系统提供的实际付款事实，按明确范围和真实程序要求形成的版本化比较判断。」要点三条：「分别判断覆盖状态（无覆盖、部分覆盖、已覆盖）、差额状态（无差额、不足、超额或待确认）和有效性状态（有效、失效、冲突或待确认），不能实现为一组互斥总状态，也不形成实际付款、客户代垫回收或监管放行。」「金额相同、同一包裹、同一客户或同一时间不能单独证明付款覆盖；资金退回、付款撤销和更正形成新的判断并保留原事实。」「避免使用：支付成功、已缴税、放行完成。」——**词条不提付款人维**；「明确记录」「保持未决」全 GLOSSARY 零命中。GLOSSARY「监管处置决定」词条写「数量、期限、条件和证据要求只在来源实际提供或适用程序要求时构成决定与核对维度」，没有「明确记录」半句。
    - `docs/application/customs-compliance/UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md`：「付款人」13 行命中（`git grep -c`）。**直接讲付款人维「未提供怎么记」的只有一句**，在「启动条件」：「实际付款事实来自银行、支付、财务或真实程序认可的合格来源，并保留来源身份、外部业务引用、付款人、金额、币种、业务时间和当前有效性。来源未提供且程序不要求的维度明确记录，不猜测补齐。」同节前一句：「真实程序是否要求付款、付款是否构成当前拟执行动作的放行前置条件、允许的付款人/范围/币种及覆盖判断规则分别来自登记依据；无法确定时可以形成未决协作事项，但不得假设统一"先税后放"或"先放后税"。」「输入语义契约」表「付款义务与门禁关系规则」行：「当前范围是否需要付款、付款是否参与当前拟执行动作的门禁，以及允许范围/付款人/币种和覆盖条件 ｜ 两项判断分别形成；未登记时保持未决，不使用全局默认顺序」。其余命中——「输入语义契约」表「外部资金事实」行、「结果语义契约」表「外部资金事实已接收」「税费付款冲突」行、「税费付款与放行核对规则」「付款金额相同、付款人在案件角色中出现、同一客户、同一包裹、同一申报单元、最近业务时间或人工选中都不能单独构成权威关联」、「应用流程」步 7「按税费版本、外部引用、范围、付款人、金额、币种、业务时间和真实规则关联」、「一致性、幂等与并发」「同一请求身份携带不同税费版本、范围、付款人、金额、币种或条件时形成请求冲突」、「首发试点约束」、「验收示例」`AT-CC-261` / `AT-CC-266`（后者「不把付款人改写为法定义务人」）、「生产参数与待决风险」表「税费—付款关联矩阵」行——都把付款人当一个维度点名，**没有一句讲「程序要求而来源未提供 → 保持未决」**（「规则要求」全文零命中；「保持未决」只命中上引「未登记时保持未决」，主语是规则未登记）。
    - 顺带核出的归属事实：票 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)「语言从哪里来」第二条写「CC `CONTEXT.md`（sa-cc/03 票面引）：『来源未提供且程序不要求的维度要「明确记录」』」——`git grep '来源未提供且程序不要求' -- docs/` 于 `bccb60a1` **只命中 UC-CC-009「启动条件」那一句，CONTEXT.md 零命中**；这句出处是 UC 不是 CONTEXT。`.scratch/tasks.md` sa-cc/03 裁决摘录同样写成「CC 放宽（『来源未提供且程序不要求的维度要明确记录』）」未标出处。另：UC-CC-008「启动条件」有处置维的平行句「来源未提供且规则不要求的数量、期限、条件或证据字段明确记录为未提供/不适用；规则要求但缺失时进入未决，不使用同单、同袋、同客户或最近作业补齐」——与 CONTEXT 监管处置决定条同形，是 UC 引 CONTEXT 的正常方向；「规则要求但缺失」全仓 `docs/` 只命中 CONTEXT 这一条与 UC-CC-008 这一句。
  - **3 · 引用那句的每一处**。`git grep -n '明确记录\|保持未决\|规则要求但缺失\|规则要求而缺失' -- internal/customscompliance/ migrations/customs_compliance/ apps/admin-web/src/pages/customs/` 于 `bccb60a1`：**24 行命中 / 15 文件；`apps/admin-web/src/pages/customs/` 9 个文件零命中**（该目录只在 `api.ts` 两处提到「同付款人都不算」）。按符号归并：**付款人维 13 处**（票面列 6，多出 7）+ **非付款人维 8 处**（引的是处置 / 凭证 / 协作事项自己的句子，不在本票缺口内，列出只为把 grep 兑平）。
    - 付款人维 13 处（文件 · 符号 · 它引的原句 · 它自称的归属 · 票面有无）：
      1. `internal/customscompliance/domain/funds_fact_payer.go` · **包头注**：「CONTEXT「税费付款核对」把付款人定为「来源提供或真实程序要求的」维度——两种来处都成立时才必备；Rules 一句「未提供或不适用必须明确记录，规则要求但缺失时保持未决」给出三格的答案……」· 自称 **CONTEXT「税费付款核对」+「Rules 一句」**，两者紧连、读作税费付款核对的 Rules 句；未点名监管处置决定 · 票面已列。
      2. 同文件 · `FundsPayerNotProvided` 函数注：「形成「来源未提供」那一格——CONTEXT 要「明确记录」的就是它。」· 自称 CONTEXT，无词条 · 票面未列。
      3. `internal/customscompliance/domain/funds_fact_payer_test.go` · **文件头注**：「CC CONTEXT「来源提供或真实程序要求的付款人」与 Rules「未提供或不适用必须明确记录，规则要求但缺失时保持未决」。」· 自称 CC CONTEXT + Rules，与税费付款核对原句并列；未点名词条 · 票面未列。
      4. `internal/customscompliance/application/reconcile_duty_payment.go` · `DutyReconciliationReason` 类型头注：「付款人那两格（票 sa-cc/12 裁决 2，CC CONTEXT「规则要求但缺失时保持未决」）……」· 自称 CC CONTEXT（经裁决 2），无词条 · 票面已列。
      5. 同文件 · `ReceiveFundsFact` 方法头注：「付款人是「来源提供或真实程序要求的」维度（CONTEXT「税费付款核对」）——来源未提供不拒收，登记里显式记「未提供」（CONTEXT「未提供或不适用必须明确记录」）」· 前半正确归税费付款核对；后半只写 CONTEXT，无词条，紧随前半 · 票面已列。
      6. 同文件 · `VerifyPayment` 方法头注：「……未决 PayerRequiredNotProvided（等来源补事实，CC CONTEXT「规则要求但缺失时保持未决」——不是「不适用」，不适用是「这条维度与本程序无关」，与「该有而没有」是两格）」· 自称 CC CONTEXT，无词条 · 票面未列。
      7. `internal/customscompliance/application/reconcile_duty_payment_test.go` · `TestAFundsFactWithoutAPayerIsReceivedWithThePayerRecordedAsNotProvided` 头注：「登记里付款人显式「未提供」（CONTEXT「未提供或不适用必须明确记录」）」· 自称 CONTEXT，无词条 · 票面未列。
      8. 同文件 · `TestThePayerDimensionIsJudgedByTheProcedureRule` 头注：「裁决 2 的三停格（CC CONTEXT「未提供或不适用必须明确记录，规则要求但缺失时保持未决」）。付款人那一维按命令所指监管程序的登记规则判」· 自称 CC CONTEXT，无词条 · 票面已列。
      9. `internal/customscompliance/adapters/http/register_credential_and_duty.go` · `writeDutyReconciliationAnswer` 头注：「后两格是票 sa-cc/12 裁决 2 的停格，CC CONTEXT「规则要求但缺失时保持未决」」· 自称 CC CONTEXT，无词条 · 票面已列（票面只写目录与符号名，文件是 `register_credential_and_duty.go`）。
      10. `internal/customscompliance/adapters/postgres/duty_payment_reconciliation.go` · `RegisterFundsFact` 方法头注：「NULL 在这一列的唯一含义就是 CONTEXT 要「明确记录」的那个「未提供」，不是缺省」· 自称 CONTEXT，无词条 · 票面未列。
      11. `internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go` · `TestAFactWithoutAPayerIsReceivedWithThePayerRecordedAsNotProvided` 头注：「登记里付款人显式记为「未提供」（CONTEXT「未提供或不适用必须明确记录」）」· 自称 CONTEXT，无词条 · 票面未列。
      12. `internal/customscompliance/ports/ports.go` · `ExternalFundsFactRegistration` 类型头注：「Payer 是领域上的一格：来源提供了引用，或来源显式未提供——CONTEXT「未提供或不适用必须明确记录」，「未提供」进登记册也进核对」· 自称 CONTEXT，无词条 · 票面未列。
      13. `migrations/customs_compliance/0020_funds_fact_payer_may_be_unprovided_and_payer_rule.sql` · **文件头注**：「CC CONTEXT「税费付款核对」把付款人定为「来源提供或真实程序要求的」维度，Rules 一句「未提供或不适用必须明确记录，规则要求但缺失时保持未决」——所以「未提供」是一格要记下来的值……」· 自称 **CC CONTEXT「税费付款核对」+「Rules 一句」**，同 1 · 票面已列。
    - 代码之外第 14 处：票 12「裁决」2 的 10:5x 改口「CC CONTEXT 原词是『规则要求但缺失时保持未决』」——自称 CC CONTEXT，无词条；票面已列。
    - 引文逐字核对（对 CONTEXT）：13 处引的字串分四种。(a) 全句「未提供或不适用必须明确记录，规则要求但缺失时保持未决」——1、3、8、13：CONTEXT 逐字命中 **1 处**（Rules 监管处置决定条）；(b) 后半「规则要求但缺失时保持未决」——4、6、9：同上 1 处；(c) 前半「未提供或不适用必须明确记录」——5、7、11、12：CONTEXT 逐字命中 **2 处**（Language 监管处置决定词条 + Rules 监管处置决定条；`git grep -c` = 2）；(d) 只引「明确记录」两字——2、10：CONTEXT 3 处（关务执行协作事项 / 监管处置决定 ×2）。**13 处没有一处的引文落在税费付款核对词条或 Rules「税费、放行与案件闭环」段。**
    - 非付款人维 8 处（引的是别的维度自己的句子，归属核过）：`internal/customscompliance/domain/compliance_judgment.go` `RegisterCredential` 头注「零表示来源未提供次数额度——未提供必须明确记录，不猜测补齐（与监管决定的数量维度同一条纪律）」（凭证额度；自陈是借处置那条纪律，未称 CONTEXT 对凭证有此句——CONTEXT 监管凭证词条与 Rules 确无「未提供」句）；`internal/customscompliance/application/register_credential_test.go` `TestRegisteringACredentialWithoutAUsesQuotaRecordsItAsNotProvided` 头注「额度未提供必须明确记录为未提供，不猜测补齐」（无归属）；`internal/customscompliance/domain/disposition_verification.go` `RequiredQuantity` 类型注「必须明确记录，不能猜测补齐（CONTEXT 硬句）」（处置数量维，对应 Language 关务执行协作事项 / 监管处置决定）；`internal/customscompliance/domain/disposition_verification_test.go` Covers 注「CC CONTEXT「数量、期限、条件……未提供或不适用的内容必须明确记录，不能猜测补齐」」（逐字对上 Language 关务执行协作事项）；`internal/customscompliance/domain/duty_collaboration.go` `FormDutyCollaboration` 头注与 `internal/customscompliance/domain/duty_collaboration_test.go` Covers 注「编排据以保持未决」（协作事项「缺少税费结果」格，对应 Lifecycles 税费付款协作事项条与 Language「缺少税费结果不能被解释为无需付款」）；`internal/customscompliance/application/reconcile_duty_payment_test.go` 文件头注「「缺少税费结果」保持未决」与 `TestAMissingDutyResultKeepsTheCollaborationUndecided` 头注「编排保持未决……（UC-CC-009 启动条件那句）」（同上，协作事项）。
  - **4 · 三停格的代码原词**
    - `internal/customscompliance/application/reconcile_duty_payment.go` `DutyReconciliationReason`：付款人维只占**两个** reason 常量——`PayerRequiredNotProvided`（String `"PAYER_REQUIRED_NOT_PROVIDED"`）与 `PayerRequirementNotConfigured`（`"PAYER_REQUIREMENT_NOT_CONFIGURED"`）；第三格「程序不要求 → 照常形成」不是 reason，落 outcome `DutyVerificationFormed`（`"DUTY_VERIFICATION_FORMED"`）。另有依赖故障格 `PayerRequirementViewUnavailable`（`"PAYER_REQUIREMENT_VIEW_UNAVAILABLE"`），头注明说「规则读口自己答不出与「规则未配置」也是两格：前者重投会变，后者不会」。类型头注原词：「付款人那两格（票 sa-cc/12 裁决 2，CC CONTEXT「规则要求但缺失时保持未决」）：PayerRequiredNotProvided 等的是**来源补事实**——真实程序要求付款人而这条事实的来源没给；PayerRequirementNotConfigured 等的是**登记方补规则**——这个程序还没登「要不要付款人」，核对不进行、不取任何默认。两格恢复动作不同，所以是两个词。」`VerifyPayment` 方法体三格落点：`!configured → dutyUndecided(PayerRequirementNotConfigured)`；`errors.Is(err, domain.ErrFundsPayerRequired) → dutyUndecided(PayerRequiredNotProvided)`；`Admit` 放行 → `domain.VerifyDutyPayment` 形成。
    - `internal/customscompliance/domain/funds_fact_payer.go` 对「未提供」的命名：类型 `FundsPayer`（头注「两格：来源提供了一条付款人引用；或来源显式未提供」）；构造 `FundsPayerNotProvided()`（注「形成「来源未提供」那一格」）与 `ProvidedFundsPayer(reference)`（注「形成「来源提供」那一格；空白引用不是提供」）；内部枚举 `fundsPayerNotProvided` / `fundsPayerProvided` / `fundsPayerKindInvalid`；方法 `Provided()`（注「报告来源有没有提供付款人」）；哨兵 `ErrInvalidFundsPayer`（注「两格都不是（零值）。它是调用方编程错误，不是「未提供」——「未提供」要显式说出来，零值悄悄当成它，入向登记就分不出「来源明说没有」与「有人忘了填」」）。规则侧：`PayerRequirement` 封闭二值 `PayerRequired`（`"REQUIRED"`）/ `PayerNotRequired`（`"NOT_REQUIRED"`），头注「没有默认：未登记不是任何一格，由读口的 found=false 表达（编排答「规则未配置」，ADR-0137 决定三同一停点的形）」；哨兵 `ErrFundsPayerRequired`（英文文本 `"the customs procedure requires a payer the source did not provide"`，注「三停格里「程序要求付款人而来源未提供」那一格的哨兵：核对停在未决并点名缺付款人，恢复动作是来源补事实——与「规则未配置」（恢复动作是登记方补规则）不是同一格」）；`Admit` 头注「要求而未提供 → ErrFundsPayerRequired（未决，点名缺付款人）；不要求 → 提供与否都放行，「未提供」原样带着进核对；要求且提供 → 放行」。代码自己对三格的措辞因此是：事实侧**「来源未提供」/「显式未提供」**；停格一**「（程序）要求而（来源）未提供」**；停格二**「不要求 → 「未提供」原样带着进核对 / 核对照常」**；停格三**「规则未配置」/「未登记」**。
  - **5 · 两候选各自的实测后果**（只列不选。「改归属」= 只动注释里点名的词条 / 小节；「改引文」= 引号内字串也得换）
    - **候选 A · 与监管处置决定同形复用**（「未提供或不适用必须明确记录，规则要求但缺失时保持未决」原句进税费付款核对处）：13 处引号内字串全部仍在 CONTEXT 逐字可寻，**没有一处要改引文**。归属那一半随落点分两种：(A-1) 补进 Rules「税费、放行与案件闭环」段——1 `funds_fact_payer.go` 包头注、3 `funds_fact_payer_test.go` 头注、13 `0020` 头注三处写的「Rules 一句」变为准确，其余 10 处只写「CONTEXT」的也变为准确，票 12 裁决 2 改口「CC CONTEXT 原词是……」同样成立——**13 处零改动**；(A-2) 补进 Language 税费付款核对词条——1、3、13 三处「Rules」字样与落点不合，需改归属；其余 10 处零改动。两种落点下同一句在 CONTEXT 出现两处（监管处置决定 + 税费付款核对），`git grep` 该句命中 1 → 2，对上票面完成判据 1「命中 ≥ 2 处」那半。
    - **候选 B · 付款人专句**（以票面「要裁的」示例句「付款人由来源提供或按真实程序登记的规则要求；来源未提供时明确记录为未提供，规则要求而未提供时核对保持未决，规则未登记时核对不进行」为量尺；措辞归 owner，字串一变下列命中随之变）：引「规则要求但缺失时保持未决」的 4、6、9 与引全句的 1、3、8、13 共 **7 处要改引文**（示例句里没有「规则要求但缺失」四字连排；不改则 `git grep '规则要求但缺失' -- internal/ migrations/` 继续命中这 7 处而 CONTEXT 里能对上的仍只有监管处置决定条）；引前半「未提供或不适用必须明确记录」的 5、7、11、12 **4 处**：该字串在 CONTEXT 仍逐字存在但落在监管处置决定——要指向付款人专句得改引文，要保留原引文得把归属改成监管处置决定并说明是借句；只引「明确记录」两字的 2、10 **2 处**：示例句含「明确记录为未提供」，引文不必动，归属补词条名即可。票 12 裁决 2 改口那句在 B 下仍是错归属，票面做法 2 已定不改写、追一句。B 另带一个 A 没有的事实：示例句把第三格「规则未登记时核对不进行」一并写进 CONTEXT，而今天 CONTEXT 里「未登记 → 规则未配置 / 保持未决」只在 Rules 放行门禁那一条与 Lifecycles 协作事项条出现，付款人维这一格今天只活在代码 `PayerRequirementNotConfigured` 与票 12 裁决 2。
    - 两候选共有的事实：UC-CC-009「启动条件」那句「来源未提供且程序不要求的维度明确记录，不猜测补齐」已用付款人族的语气写了第二格（不要求 → 记「未提供」），没写第一、第三格；票面完成判据 3「UC-CC-009 若引到这一维，同步指向 CONTEXT 新句」对应的就是这一句所在小节。
  - **能力边界**：全部取证只读 `bccb60a1` 隔离检出，未跑 `go build` / `go test`、未占 55432；引文由 Read 工具逐字取出（本机 PowerShell 管道把 UTF-8 打成乱码，`git grep` 的定位与计数不受影响，但引文一律以 Read 结果为准）；`apps/admin-web` 只 grep 了派单指定的 `src/pages/customs/`，其它目录未查；GLOSSARY 只查了「付款人」「税费付款核对」「外部资金事实」三词与「明确记录」「保持未决」，未通读；未核 ADR-0137 正文对付款人维的措辞；票 03 票面原文未读（只读了 `.scratch/tasks.md` 里的裁决摘录）。不改 Status，不动票面其它字。
