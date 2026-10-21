# CC（customs-compliance）文档自洽审计

- 读的版本：只读 worktree `/home/tops/workspace/idp-parcel-audit`，detached @ `6b0074d0`；在途分支未读。
- 口径：`.scratch/docs-consistency/README.md`「审计口径」；AGENTS.md「改文档」「写代码注释」。
- 范围：`docs/domain/customs-compliance/CONTEXT.md`；`docs/application/customs-compliance/` 全部 13 份 UC；CC 相关 ADR（核心读全文：0069、0070、0137、0197、0206、0226、0227、0236；按停用链核了 0149、0151、0205、0213、0219、0225 的 Status 行）；CONTEXT-MAP、GLOSSARY 的 CC 条目；PILOT-SCOPE 与参数登记册 `PAR-CUS-01..08`；`internal/customscompliance/**` 与 `cmd/**` 的 CC 装配。
- 链接：CONTEXT、13 份 UC 与上列 ADR 的全部相对链接与锚点用脚本解析过，**无断链**（目标 5 的链接部分无发现）。
- 共 24 条：高 3、中 12、低 9。按主类别：1 类 2 条、2 类 9 条、3 类 3 条、4 类 6 条、5 类 3 条、6 类 1 条。按归类：A 18 条、B 2 条（H3、M6）、C 4 条（M8、M9、L8、L9）。

---

## 高

### H1 UC-CC-005 `AT-CC-109`「就绪不存在」仍答门禁阻断，与代码和 ADR-0227 首发答法相反，且没有旁注

- **类别**：2（同一场景两套结果）、4（文档与代码）
- **位置**：`docs/application/customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md` 验收示例：「`AT-CC-109` | 执行请求到达时就绪不存在、已失效或与目标不一致 | 返回`提交前门禁已阻断`，不冻结版本、不消费授权、不占用凭证、不调用渠道」
- **证据**：
  - 代码 `internal/customscompliance/application/submit_declaration.go` `SubmitDeclarationHandler.Handle`：`LoadReadiness` 答 `!configured` → `DeclarationUndecided` + `ReadinessUnconfigured`（`DECLARATION_UNDECIDED` / `READINESS_UNCONFIGURED`），头注「就绪规则/资格目录是实例半边：未配置停在未决，不默认就绪」；只有 `!readiness.Effective()` 才答 `NOT_READY`。
  - ADR-0227 决定八：「授权确认时单元的就绪判断没登、提交执行时单元的提交授权没登，首发都答未决并指名未配置……提交编排早已这样答」。但它列出的替代范围只有「UC-CC-004 `AT-CC-105`……在「不存在」「已经失效」两格、UC-CC-005 `AT-CC-110`……在「不存在」一格」，漏了提交执行口上**就绪**没登的这一格，即 `AT-CC-109`。
  - 同一份 UC 的 `AT-CC-110` 已旁注「首发对「授权未登记」一格的答法见 `AT-CC-439`」，`AT-CC-109` 没有对应旁注，也没有一条 4xx 号 AT 写提交执行口上「就绪未登记 → 未决（就绪未配置）」。
  - 这就是票 psb/18 评审发现的 105/444、110/439 同一类，漏掉的第三格。
- **归类**：A（代码与 ADR-0227 决定八的原则一致；文档漏改）
- **严重度**：高（照 AT 写测试会断言「门禁阻断」，与生产答复相反；读者会把「租户还没登就绪」读成「门禁明确不成立」，续办方向错）
- **建议修法**：在 UC-CC-005 新增一条首发 AT（提交执行时单元没有就绪判断 → 未决并指名就绪未配置，不落单元、版本或尝试），`AT-CC-109` 的「不存在」一格改指向它（与 M2 一并改成一套结果）；ADR-0227 Status 行注明决定八的替代范围补上 `AT-CC-109`「不存在」一格。

### H2 UC-CC-006 仍写「同层冲突 → 依赖动作的门禁未决，未登记冲突门禁规则时不得放行」，与 ADR-0226、ADR-0206 和 `AT-CC-417`/`434`/`435` 相反

- **类别**：2（同一场景两套结果；UC 正文与现行 ADR 相反）
- **位置**：`docs/application/customs-compliance/UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md`
  - 「时间、乱序、更正与当前判断」：「是否将依赖该层的动作转为受控阻断，必须由已确认的冲突门禁规则形成新的有效判断；规则未登记时显式保持门禁判断未决，不能按来源到达先后或操作员偏好裁决。」
  - 结果表「外部结果冲突」禁止行为：「……也不得在未确认冲突门禁规则时自行阻断或放行业务动作」
  - `AT-CC-158`：「保持外部结果冲突……是否阻断依赖动作由已确认冲突门禁规则决定，规则未登记时门禁判断显式未决」
  - 「生产参数与待决风险」「同层冲突裁决」行：「门禁规则未登记时不得自行阻断或放行依赖动作」
- **证据**：
  - ADR-0226 决定三：「矛盾时冲突标在后到那份上，双方留存；当前判断以它到达时已在册的事实为准，后到一方不进当前判断」；并说 CC 动作准入的折法「与此一致，不必改」。
  - ADR-0206 决定五：「外部放行按层读……同层冲突的那份不算」。
  - 同一份 UC 的 `AT-CC-434`「先到的那份与当前判断不变，冲突那份不作为当前判断交出」、`AT-CC-435`「后到的那份形成外部结果冲突，不按最后到达改变当前判断」；UC-CC-013 `AT-CC-417`「答未决（只有部分放行）；同层冲突那份不算」。
  - 代码：`ports.ExternalResultStore.LoadForSubmission` / 放行读口排掉记了冲突的那份，`domain.JudgeActionAdmission` 只读未记冲突的放行。先到全部放行、后到回退的部分放行（冲突）时，准入照全部放行答 `ADMITTED`，没有任何「冲突门禁规则」或「未决」。代码里也没有冲突门禁规则这一册。
  - 同一场景（先全部放行、后到矛盾的放行），`AT-CC-158` 要求门禁未决，`AT-CC-435` 与 `AT-CC-417` 的判法给出「当前判断不变 → 可放行」。
- **归类**：A（ADR-0226、ADR-0206 已接受，代码照它们做；按 AGENTS「ADR 优先于用例」，UC 过时）
- **严重度**：高（作者照 UC 会给门禁或准入加一格「冲突即未决」，把已全部放行的货挡住；读者会误判冲突后包裹能否出发）
- **建议修法**：把上列四处改写为 ADR-0226 决定三的口径：冲突标在后到那份，当前判断与依赖动作的读法以到达时在册的事实为准；「按权威、更正或人工复核裁决冲突」与「冲突是否另行阻断依赖动作」留作待决项，指向 ADR-0226 越权风险点 7 与票 demo-chain-gaps/124。`AT-CC-158` 改成与 `AT-CC-435` 同一结果，或并入它。

### H3 代码里的「限制」是按监管结果解除的**监管限制**，CONTEXT、UC-CC-011 与 ADR-0206 却把它当**内部合规限制**用

- **类别**：4（核心对象的模型与判法与文档不一致）、1
- **位置**：
  - `docs/domain/customs-compliance/CONTEXT.md` Language「内部合规限制」：「运营企业依据贸易合规规则，对明确对象和动作形成的内部阻断决定……只有该限制的责任来源可以形成有效解除」；Rules：「外部放行不自动解除内部限制」「只有原限制责任来源在当前授权内可以形成解除」「部分解除保留未覆盖范围」；「限制适用性判断」「可以是有效限制、无内部限制、待确认或冲突」
  - 「动作准入判断」词条与 Rules：三源之一是「内部合规限制」，「有效限制一律阻断，外部放行不解内部限制」
  - `docs/application/customs-compliance/UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md` `AT-CC-350`：「只有限制责任来源据此形成独立解除决定后，内部限制才改变；外部结果本身不等于解除」；`AT-CC-355`（部分解除）
  - ADR-0206 决定五：「限制：动作时点有效且约束本动作的限制一律阻断……外部放行不解内部限制」；Consequences：「限制加按时点的 `InForceAt` 与 `JudgeActionAdmissibilityAt`」
- **证据**（代码）：
  - `internal/customscompliance/domain/regulatory_restriction.go`：类型是 `RegulatoryRestriction`，头注「作用于明确对象范围的监管限制」，挂 `Decision RegulatoryDecisionID`。解除只有 `ReleaseByRegulatoryOutcome(release RegulatoryReleaseReference, …)`，`RegulatoryReleaseReference` 头注「指名责任来源接受的监管结果——它是解除限制的唯一依据类型」。
  - 限制只有一个 `Scope`，没有覆盖项、责任来源、解除条件；解除是整条一次（`ErrRestrictionNotCurrent`），没有部分解除。`ActionAdmissibility` 是二值（可继续 / 被阻断），没有待确认、冲突两格。
  - `ports.RestrictionStore`：「保存监管限制并按范围盘出参与准入判断的限制」。准入判断读的就是这一册。
  - GLOSSARY「内部合规限制」写「避免使用：海关扣留」；「限制」条：「“内部合规限制”仅指运营企业依据贸易合规规则形成的内部业务决定；监管扣留……属于外部监管事实……不能互相改名或相互推导」。代码恰好把两者合成了一个对象。
  - ADR-0206 越权风险点 1：「限制今天没有生产写入（`ManageRestrictionHandler` 未接任何入口）」。`cmd/` 下确实没有装配 `NewManageRestrictionHandler`，所以这一错位今天不出生产事故，但 UC-CC-011 一开口就会撞上。
- **归类**：B。代码两头都对不上：把它当内部合规限制，解除路径（按监管结果）违反 CONTEXT；把它当监管限制（扣留派生），ADR-0206 与 CONTEXT「动作准入」写的三源之一「内部合规限制」就没有实现，UC-CC-011 的对象也没有模型。要先裁：准入第一源是内部合规限制、监管扣留派生的限制，还是两者都要。
- **严重度**：高（UC-CC-011 实施者会在现有类型上加「责任来源解除」，或照代码让外部结果解除内部限制，都违反 CONTEXT 硬句）
- **建议修法**：立新 ADR 代决准入第一源的构成与两种限制的模型分工（越权风险点：监管扣留要不要也进准入第一源），再按裁定改 CONTEXT「动作准入判断」与 UC-CC-011/UC-CC-013 的措辞，或立代码票把 `RegulatoryRestriction` 改成 CONTEXT 的内部合规限制模型（覆盖项、责任来源解除、部分解除、四格适用性）。UC-CC-011 顺带补一句：UC-CC-013 是限制的消费方，按动作时点读（`InForceAt`）。

---

## 中

### M1 UC-CC-004：就绪读不动有两套结果（`AT-CC-103` 与 `AT-CC-444`），结果表也没有 `AT-CC-444` 所用的格

- **类别**：2、4
- **位置**：`docs/application/customs-compliance/UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md`
  - `AT-CC-103`：「读取权限、委派、政策或就绪依据时超时，或计算、持久化失败 | 返回`未形成授权结果`及失败阶段和续办引用；不伪装成拒绝、待人工或已授权」
  - `AT-CC-444`：「确认时单元的就绪判断尚未登记，或已失效，或读不动 | 分别返回未决并指名就绪未配置、不再就绪、未决并指名读不动」
  - 结果表「授权条件未决」的业务含义只说「当前权威角色、委派、权限矩阵或自动政策不足以确定谁能决定」；表里也没有「不再就绪」一格
- **证据**：`internal/customscompliance/application/confirm_submission_authorization.go`：`LoadReadiness` 出错 → `AUTHORIZATION_UNDECIDED` + `READINESS_UNAVAILABLE`；未登记 → `AUTHORIZATION_UNDECIDED` + `READINESS_UNCONFIGURED`；失效 → `NOT_READY`；授权册读不动 → `AUTHORIZATION_UNDECIDED` + `AUTHORITY_STORE_UNAVAILABLE`。代码与 `AT-CC-444` 一致，与 `AT-CC-103` 的「未形成授权结果」不一致。ADR-0227 越权风险点 9 也以「未决」称呼这几格。
- **归类**：A
- **严重度**：中
- **建议修法**：`AT-CC-103` 的「就绪依据读不动」一格改指 `AT-CC-444`（或写明首发答未决并指名读不动）。结果表「授权条件未决」的含义补「就绪未登记 / 依赖读不动」两种原因，并加「不再就绪」一格（`NOT_READY`）。

### M2 `AT-CC-105`/`AT-CC-444`、`AT-CC-110`/`AT-CC-439` 仍是同一场景两套结果（只加了旁注）

- **类别**：2
- **位置**：UC-CC-004 `AT-CC-105`「返回`请求未受理（授权前提不成立）`……（首发对「就绪未登记」「已失效」两格的答法见 `AT-CC-444`……）」；UC-CC-005 `AT-CC-110`「返回`提交前门禁已阻断`……（首发对「授权未登记」一格的答法见 `AT-CC-439`……）」
- **证据**：ADR-0227 决定八：「两行的正文不改，行旁注明首发答法见 `AT-CC-444` / `AT-CC-439` 与本条」。本轮目标 2「同一场景只有一套结果」与这一处理相抵：两行仍断言旧结果名，只靠括注提示读者另有答法。
- **归类**：A（业务答法已由 ADR-0227 决定八定；「正文不改」只是当时的文档处理方式）
- **严重度**：中（有旁注，误读概率低于 H1）
- **建议修法**：把 105 的「不存在」「已失效」两格、110 的「不存在」一格从行内移走，行内只留仍成立的格（105：与目标版本不一致；110：已撤销、已到期、目标不符、已被另一逻辑提交使用）。ADR-0227 Status 行注明决定八「两行的正文不改」由本轮修订取代，只改坐标，不动决定文字。与 H1 同一张票做。

### M3 UC-CC-001 结果表缺「包裹未接受」「未决（规则未配置）」两格；`AT-CC-016` 与 ADR-0227 决定二相反

- **类别**：2、4
- **位置**：`docs/application/customs-compliance/UC-CC-001-ESTABLISH-CUSTOMS-CASE.md` 结果语义契约（八格：已建立、部分范围已建立、已有案件、建案条件不足、未形成案件、请求冲突、请求未受理、不适用）；`AT-CC-016`「读取商业资格或监管规则时依赖超时 | 未形成案件并保留安全续办引用」；`AT-CC-438`「前者返回包裹未接受并逐件列出，后者返回未决并指名规则未配置」
- **证据**：
  - `internal/customscompliance/application/establish_customs_case.go` `EstablishCaseOutcome`：`ESTABLISHED`、`EXISTING`、`SCOPE_CONFLICT`、`NOT_REQUIRED`、`NOT_ACCEPTED`、`UNDECIDED`、`PARCEL_NOT_ACCEPTED`。未决原因有 `CASE_REQUIREMENT_UNAVAILABLE`、`CASE_REQUIREMENT_UNCONFIGURED`、`CASE_STORE_UNAVAILABLE`、`CASE_IDENTITY_UNAVAILABLE`、`ACCEPTED_PARCEL_SOURCE_UNAVAILABLE`。
  - ADR-0227 决定二：「建案未决的原因此后分格（规则未登记、规则读不动、案件册、标识签发、源上下文读不动），HTTP 以 200 带原因作答」。规则读不动今天答「未决 + 原因」，不是 UC 的「未形成案件」。
  - `AT-CC-438` 用到的「包裹未接受」「未决」两个结果名在结果表里不存在。
- **归类**：A
- **严重度**：中
- **建议修法**：结果表加「包裹未接受」（`PARCEL_NOT_ACCEPTED`）与「建案未决（带原因）」两格。写明「未形成案件」在首发即「未决 + 读不动类原因」，或把 `AT-CC-016` 改成未决并指名哪一口读不动。

### M4 UC-CC-005「提交已登记待发送」的定义与代码只形成的那一半不一致，UC 没有注明

- **类别**：2、4
- **位置**：UC-CC-005 结果表「提交已登记待发送 | 不可覆盖提交版本、授权使用、适用凭证占用、待发起尝试身份和持久化发送意图已经形成，确定尚未越过外部发送边界」；「提交尝试与发送阶段」同一格；`AT-CC-107`「……形成授权使用、适用凭证关系和提交尝试，并通过指定渠道发起发送」；`AT-CC-126`、`AT-CC-127`（续发同一发送意图）；`AT-CC-125`（本地持久化失败 → `未形成提交处理结果`）
- **证据**：
  - ADR-0227 决定四：「这一格今天只形成了 UC-CC-005「提交已登记待发送」的一半：尝试身份与目标登记了，授权使用关系没有建模，提交版本形成信封只有 VE 投影消费、不是发送意图」；越权风险点 7、10。
  - 代码 `domain.AttemptRegisteredPendingSend` 只表示尝试已登记、无发送时间。`submit_declaration.go` 在单元册或提交册写不进时答 `DECLARATION_UNDECIDED` + `UNIT_STORE_UNAVAILABLE` / `SUBMISSION_STORE_UNAVAILABLE`，不是「未形成提交处理结果」。
  - 同一个结果名在 UC 与代码里含义不同；CONTEXT「提交尝试」只说「尝试身份与目标已经登记」，与代码一致、比 UC 窄。
- **归类**：A
- **严重度**：中（作者会以为授权使用、发送意图已有，去依赖它们；ADR-0227 越权风险点 10 已点明更正、补充若照此接线会绕过逐次授权）
- **建议修法**：结果表与阶段表在「提交已登记待发送」格旁注首发只形成尝试身份与目标、无授权使用与发送意图，指 ADR-0227 决定四、越权风险点 7、10 与票 psb/10、psb/36。`AT-CC-107` 标为长期（有发送通道后），首发看 `AT-CC-441`。`AT-CC-125` 写明首发答未决并指名哪一册写不进。

### M5 CONTEXT 生命周期仍说「首次实际发送时」才形成提交版本，与 ADR-0227「已登记待发送」相抵

- **类别**：1
- **位置**：`docs/domain/customs-compliance/CONTEXT.md` Lifecycles「申报单元、就绪与提交」：「提交授权与就绪判断分别形成和失效。只有两者在逻辑提交首次实际发送时均有效，才形成新的提交版本和初始提交尝试」
- **证据**：同文件「提交与外部监管结果」首条已按 ADR-0227 改为「固定提交版本 → 首次尝试`已登记待发送`……只有发送通道越过发送边界，尝试才算已发起」。ADR-0227 决定四：「就绪与授权同时有效才固定提交版本；首次尝试记「已登记待发送」」。代码 `SubmitDeclarationHandler` 在登记那一刻核就绪与授权，随即固定版本，此时没有任何发送。UC-CC-005「最终提交门禁」也写「在形成提交版本前必须……一次性复核」。登记之后、发送之前就绪或授权失效怎么办，CONTEXT 没有说，ADR-0227 越权风险点 7 把它留给票 psb/10。
- **归类**：A
- **严重度**：中
- **建议修法**：改成「只有两者在固定提交版本时均有效，才形成新的提交版本与`已登记待发送`的初始尝试；越过发送边界前是否重核归发送通道（ADR-0227 越权风险点 7）」。

### M6 UC-CC-006 把「技术回执」当作本用例接收的外部结果层，代码与 ADR-0197 都没有这一层，也没有 ADR 裁过

- **类别**：4
- **位置**：UC-CC-006「外部结果层次」表首行「技术回执 | 申报渠道对连接、报文接收或技术校验的明确反馈 | 提交尝试及技术动作 | ……」；`AT-CC-140`「形成关联原提交尝试的技术回执」、`AT-CC-141`、`AT-CC-144`、`AT-CC-153`
- **证据**：
  - `internal/customscompliance/domain/external_result.go` `ResultLayer` 是「封闭六层……提交尝试与技术回执在提交对象上，这里是监管侧六层」：`REGULATORY_RECEIPT` 至 `DISPOSITION_DECISION`，没有技术回执。
  - 技术结果只是 `domain.AttemptResult` 的 `ACKNOWLEDGED`/`FAILED`/`PENDING_CONFIRMATION`，在形成尝试时就定了。ADR-0227 越权风险点 7：「尝试行今天没有 UPDATE」，越过边界时「是推进这一份尝试的状态、还是另记一笔发送事实……归票 10 先裁」。所以后到的技术回执今天无处可收。
  - ADR-0197 决定二的类型词表也只有监管侧六层加三种放行。
  - GLOSSARY「技术回执」写「`customs-compliance` 保存来源回执及其与提交尝试的关系」，与 UC-CC-006 同向。
- **归类**：B（技术回执是 UC-CC-006 的外部结果层，还是提交尝试的状态，没有 ADR 定；代码按后者、文档按前者）
- **严重度**：中
- **建议修法**：随票 psb/10 立 ADR 定技术回执的落点，再按裁定改 UC-CC-006「外部结果层次」与 `AT-CC-140/141/144/153`，或在 UC 注明首发技术回执无入口、指向该票。

### M7 UC-CC-009 没有带上 ADR-0137 决定三、四；门禁结果格与 CONTEXT 和代码不一致

- **类别**：2、4
- **位置**：`docs/application/customs-compliance/UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md`
  - 全文没有引用 ADR-0137，没有「税费付款」那一道的登记规则（三态接受集合、待确认与冲突不可登记为接受、未登记答「规则未配置」）。`AT-CC-260` 只说「形成条件未决」。
  - 输入表「外部资金事实 | 银行/支付/财务或程序认可来源」；应用流程步 6「银行/支付/财务系统 | 形成并提供……| 本用例接收稳定来源引用」。没有写外部资金事实只经 `settlement-accounting` 采用后由采用信封到 CC。
  - 结果表门禁三格是「放行门禁待满足 / 已满足 / 冲突或待确认」，元数据「门禁核对结束」写「待满足、部分满足、满足、冲突、不适用或待监管结果判断」
- **证据**：
  - CONTEXT「税费、放行与案件闭环」：「「税费付款」这一道门禁怎么读税费付款核对，是按监管程序登记进来的规则……未登记规则时该道门禁答「规则未配置」」。
  - ADR-0137 决定四：「外部资金事实进入本产品只有 `settlement-accounting` 采用这一口；`customs-compliance` 不开第二个铸造或补录入口」。CONTEXT-MAP `settlement-accounting → customs-compliance` 同句。
  - 代码：`application/verify_release_gate.go` 未决原因 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`、`DUTY_PAYMENT_FINDING_REGISTERED_BESIDE_RULE`。`domain.GateConclusion` 是五值 `UNMET`/`PARTIALLY_MET`/`MET`/`CONFLICTING`/`NOT_APPLICABLE`，与 CONTEXT「放行门禁核对」一致。代码没有「待确认」或「待监管结果」一格，资金事实入口是 `adapters/inbox/external_funds_fact_consumer.go`（SA 采用信封）。
- **归类**：A
- **严重度**：中（照 UC-009 实施，可能在 CC 侧另开资金事实入口，正是 ADR-0137 决定四禁的第二铸造点）
- **建议修法**：UC-CC-009 权威依据加 ADR-0137 决定三、四（与 ADR-0236 手工采用口）。输入表与步 6 改为「经 SA 采用信封接收的已采用事实引用」。门禁结果改为 CONTEXT 五格，另列「规则未配置 / 未决」。删去「待监管结果」。

### M8 关闭依据项：代码只有三值，CONTEXT、GLOSSARY、UC-CC-010 是四值（缺「冲突」）

- **类别**：4
- **位置**：CONTEXT Language「关闭依据项」：「并分别表达已终结、已移交、未解决或冲突」；Rules：「任一未解决或冲突项都阻止关闭」；GLOSSARY「关闭依据项」同；UC-CC-010 目标与边界、输入表「已终结、已移交、未解决或冲突结论」
- **证据**：`internal/customscompliance/domain/case_closure.go` `ObligationItemState`：「关闭依据项的封闭三值：已终结、已被有权接收方有效承接、未解决」，只有 `CONCLUDED`/`HANDED_OVER`/`UNRESOLVED`。头注引 CONTEXT「任一未解决或冲突项都阻止关闭」，却没有冲突这一格。`NewCloseCustomsCaseHandler` 在 `cmd/` 未装配。
- **归类**：C（文档是现行规则，代码少一格）
- **严重度**：中（编排未接线，今天不出生产事故）
- **建议修法**：立代码票给 `ObligationItemState` 加「冲突」一格（阻止关闭、带冲突依据），或由 owner 裁定冲突并入未解决后改 CONTEXT 与 GLOSSARY。

### M9 层内更正：文档写「后续明确更正形成新结果及当前判断」，代码没有更正关系，矛盾的更正会被判成冲突

- **类别**：4
- **位置**：CONTEXT Lifecycles「提交与外部监管结果」：「后续监管更正形成新的外部事实及当前判断，不覆盖原放行结果」；UC-CC-006「来源权威与解释规则」：「同一层次和业务事项的后续明确更正形成新结果及更正关系」；`AT-CC-156`
- **证据**：ADR-0226 Context：「CC 没有层内更正关系：来源更正在 CC 是另一次接收、另一个来源标识」；越权风险点 5：「要等层内更正关系落地才分得开」。代码 `domain/external_result.go` 的一致性判断注释「要么是矛盾、要么是更正，而更正得由来源指明，不能从业务时间猜」，但接收载荷与领域对象都没有更正指向。所以来源明确的放行收窄更正今天照判冲突、进不了当前判断，与 `AT-CC-156` 相反。
- **归类**：C（文档是长期规则，机制缺）
- **严重度**：中
- **建议修法**：立代码票补层内更正关系（接收载荷带「更正哪一份」，一致性判断据此先认更正、再判矛盾），可与票 demo-chain-gaps/124 的冲突关系同做。修好之前，在 UC-CC-006 那一条与 `AT-CC-156` 旁注首发尚无更正关系、按 ADR-0226 判冲突。

### M10 ADR-0070 标题与正文用 CONTEXT 行号（「硬句 191」「186 所列」），现在都指错了

- **类别**：5
- **位置**：`docs/adr/0070-customs-rule-registries-split-recording-from-selection.md` 标题「解释规则的选择侧违反硬句 191」；Context「同节的选择侧那句（即硬句 191）」；Decision 一「硬句 191 只管选择侧」；Decision 二「`OccurredAt` 是 186 所列的「业务发生或适用时间」」；它引的代码注释「……与适用时点（191）」
- **证据**：本意是 CONTEXT「外部结果与规则时效」的「关务规则版本必须记录适用辖区、法定生效区间和规则声明的适用时点……」与「每项外部结果必须保存来源身份……业务发生或适用时间……」两句。按行号数，191 现在落在「撤销重报至少包含撤销原提交和建立替代申报两个相关但独立的动作」，186 落在「每个逻辑申报目标首次实际对外发送前都必须形成不可覆盖的提交版本……」。ADR 索引里的标题同样带着这个坐标。
- **归类**：A
- **严重度**：中（标题本身就是一个指错的引用）
- **建议修法**：照 ADR-0027 的先例只改坐标：正文里的「硬句 191」「186」换成引文或「外部结果与规则时效」节的句首引文；Status 行注明「只改坐标，未动任何决定文字」；标题里的「硬句 191」换成「规则时效硬句」一类不带数字的称呼，重生成索引。

### M11 CC 领域代码注释里的 CONTEXT 行号全部指错

- **类别**：5（AGENTS「Go 注释里的跨文件引用同受此约束」）
- **位置**（以符号定位）：
  - `internal/customscompliance/domain/compliance_judgment.go` `ComplianceJudgment` 头注「自动与人工一视同仁（159）」，`Supersede` 头注「谁都删不掉谁（159）」
  - `domain/follow_up_target.go` `ReplacementRelation` 头注「……已被有效替代（175）」
  - `domain/external_result.go` `SourceAuthorityRole` 头注「……才能形成监管事实（186）」，`ExternalResult` 头注「……改变不了原提交的业务判断（188）」
  - `domain/case_closure.go` `ClosureObligationItem` 头注「……都不能证明责任已经移交（219）」，`CustomsCaseClosure` 头注「单个案件不存在部分关闭（218）……重开保留原关闭记录（233）」
- **证据**：按当前 CONTEXT 行号数，159 是「外部监管舱单更正、撤销……」，175 是「任一端国家/地区缺码时作答状态未知……」，186 是「每个逻辑申报目标首次实际对外发送前……」，188 是「同一次提交的通信超时……」，218、219 是两条监管凭证规则，233 是「实际付款成功只能触发对关务条件……」，没有一处是注释的本意。`go build`、`go vet` 与测试都不会报（AGENTS 原话）。
- **归类**：A
- **严重度**：中
- **建议修法**：逐处换成 CONTEXT 原句引文（如「CONTEXT「人工判断不能删除自动判断」」「CONTEXT「单个案件不存在部分关闭」」）。CC 修订票里顺带 `grep '（[12][0-9][0-9]）'` 全包扫一遍。

### M12 ADR-0206 复核记录 R12 写着「改答未配置……不再不列」，规则其实未变，Status 行没有提示

- **类别**：3、1
- **位置**：`docs/adr/0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md` 末节「复核记录」：「**R12**：对已登记过至少一个节点关务区域的租户，不在册对象起讫有一端未登区域时，跨区那一道改答「未配置」并指名那个未登的节点，不再不列……落地与对决定二的部分停用见票 demo-chain-gaps/147。」ADR-0213 复核记录 R12 同根。
- **证据**：票 `.scratch/demo-chain-gaps/issues/147-cross-zone-check-answers-unconfigured-naming-the-unregistered-node.md` Status 是 `ready-for-agent`，没有落地，也还没有部分停用决定二的新 ADR。现行规则仍是 CONTEXT「动作准入判断」：「只有对象确实带着跨区的门，跨关务区域移动才答未配置……否则这一道不列」，UC-CC-013 `AT-CC-422` 与代码 `domain.JudgeActionAdmission`（`CrossZoneUndetermined` 分支）相同。ADR-0206 Status 行与索引注记都没有提 R12，只读 Status 的人看不到它，读到正文末尾的人会以为规则已改。
- **归类**：A
- **严重度**：中
- **建议修法**：ADR-0206 与 ADR-0213 Status 行各加一句「复核记录 R12 的改判待票 147 落地并由新 ADR 部分停用决定二；落地前现行仍为决定二」。票 147 落地时按 R12 改 CONTEXT「动作准入判断」与 `AT-CC-422`。

---

## 低

### L1 CC ADR 的 Status 行里，同一条前向指针或接受出处写了两遍（infra-speed/07 同类）

- **类别**：3
- **位置**：
  - ADR-0197：标记前段「越权风险点 5 后半……已由 ADR-0226 回答」与索引注记「｜**同层后续已答**（2026-10-21）……」
  - ADR-0206：标记前段「Consequences「外包干线交接……」……已由 ADR-0213 回答」「越权风险点 8……已由 ADR-0225 回答」与索引注记「｜**挂法已答**」「｜**展开成员（出发）已答**」
  - ADR-0226、ADR-0227、ADR-0236：标记前段的接受出处「Accepted（2026-10-21，Claude Code 会话代决：用户原话……」与索引注记「**已接受**（2026-10-21，用户原话……」重说
- **证据**：`.scratch/infra-speed/issues/07-adr-status-lines-carry-a-suspension-twice.md`「要什么」：同一次停用合成一份、留在索引注记里；接受出处重说的同此处理或判为不合。停用链两头都在，所引原句在被停方正文里逐字存在（已核：0197 越权风险点 5、0206 Consequences、越权风险点 8 与决定六、0149 决定一、0151 决定四）。
- **归类**：A
- **严重度**：低
- **建议修法**：并入 infra-speed/07 的做法，逐份合并，不丢日期、原句与「其余各条不变」。

### L2 ADR-0137 Consequences「三册同族一致」已被 ADR-0149 与 ADR-0236 改掉，Status 行没有指针

- **类别**：3
- **位置**：ADR-0137 Consequences：「票 07 步一 CLI 三个子命令；步二端点 + 管理台照 ADR-0085 决定四做齐（三册同族一致）」
- **证据**：`cmd/parcel-api/endpoints_customs_registration.go`：`/customs-regulatory-credential-registrations` 挂 `integrationClientFace(accessidentity.FactRegulatoryCredential)`（ADR-0149 决定三），协作与付款核对两口挂 `registryWriteFace()`。手工凭证口按 ADR-0236 挂运营决定面。三册已不同族。ADR-0137 Status 行没有指向 ADR-0149 或 ADR-0236。
- **归类**：A
- **严重度**：低（Consequences 不是 Decision，但读者会据它找错族）
- **建议修法**：ADR-0137 Status 行加一句：凭证登记口的族已由 ADR-0149 决定三改为集成客户端族，手工口由 ADR-0236 加在运营决定面；协作与付款核对两册仍是登记写面。

### L3 UC-CC-013 里几处已落地的票仍写成「待」

- **类别**：2
- **位置**：`docs/application/customs-compliance/UC-CC-013-JUDGE-ACTION-ADMISSION.md` 状态行「生产消费方接线随运输履约出发登记一票」；「本用例不执行」：「……归动作执行方（ADR-0205，票 demo-chain-gaps/32 起草）」；「生产参数与待决风险」：「逐件建案等 psb/18：此前演示包裹不在册，演示动线上不出现放行」
- **证据**：票 demo-chain-gaps/32 Status「resolved——2026-10-19 进 main」；交出、交付读数由 ADR-0213 决定四、五（票 65、66）接上；票 psb/18 Status「resolved——2026-10-21 进 main」，ADR-0227 已给出逐件建案口，演示脚本 `declare-one-parcel.sh` 走通三口。
- **归类**：A
- **严重度**：低
- **建议修法**：状态行改为「生产消费方：出发（ADR-0205）、交出与交付（ADR-0213）已接线」；去掉「起草」；把 psb/18 那条改成「逐件建案口已由 ADR-0227 提供；默认演示动线是否出现放行，取决于是否灌外部放行结果」。

### L4 计数与区间型引用：UC-CC-005「七类」、CLI 头注「五本登记册」「PAR-CUS-01..07」

- **类别**：5
- **位置**：UC-CC-005「最终提交门禁」：「`UC-CC-003` 唯一拥有申报就绪的七类业务评估规则」；`cmd/parcel-customs-register/main.go` 包头注「关务案件配置面五本登记册的受控进程入口」「配置内容属实例半边（PAR-CUS-01..07 待提供）」
- **证据**：UC-005 的「七类」数的是 UC-CC-003「就绪门禁」表的行（另一个文件），是 AGENTS「改文档」禁的「别处有几个」。CLI 的命令表 `translate.go` `allCommands` 今天有就绪、授权、解释规则、义务、门禁、建案要求、口岸、申报路径、凭证、税费协作、付款核对、节点区域十余册，不是五本；`node-zone` 子命令对应的是 `PAR-CUS-08`。
- **归类**：A
- **严重度**：低
- **建议修法**：UC-005 改为「`UC-CC-003`「就绪门禁」表所列的业务评估规则」；CLI 头注去掉「五本」，区间改为指名参数登记册「关务」节，不写编号区间。

### L5 GLOSSARY 与 CONTEXT 对同一词的定义有出入

- **类别**：6
- **位置与证据**：
  - 「限制适用性判断」：GLOSSARY「判断至少区分有效限制、当前无内部限制、适用待确认、冲突和技术未形成」（五格）；CONTEXT Language「它可以是有效限制、无内部限制、待确认或冲突」（四格，技术未形成在 Rules 里作为另一类结果）。
  - 「税费付款核对」：GLOSSARY「将监管核定税费与银行、支付或财务系统提供的**实际付款事实**」；CONTEXT「……提供的**外部资金事实**」（含付款失败、退回、撤销）。
  - 「申报就绪判断」：GLOSSARY「对明确**正式申报资料版本**是否同时满足……」；CONTEXT「对明确**申报范围**是否已经满足……」。
  - 「监管核定税费」：GLOSSARY 要点「`settlement-accounting` 只依据有效责任形成关税代垫回收和服务费用」；CONTEXT「实际代垫成立判断和客户代垫回收由 `settlement-accounting` 通过 UC-SA-001 形成」，GLOSSARY 自己的「客户代垫回收」也写要先有实际代垫成立判断。「只依据有效责任」与此相抵。
- **归类**：A
- **严重度**：低
- **建议修法**：交横切组按目标 6 处理，CC 术语以 CONTEXT 为准，GLOSSARY 改为引用或对齐措辞。最后一条改为「依据实际代垫成立判断与有效责任形成客户代垫回收」。

### L6 CONTEXT 写监管凭证「两条路」登记，漏了受控 CLI 一路

- **类别**：1、4
- **位置**：CONTEXT「监管凭证、限制与处置」：「凭证版本可以由外部系统经集成客户端送来，也可以由持监管凭证登记授予的租户人员照凭证原件人工登记；两条路登的是同一种不可变版本」
- **证据**：`cmd/parcel-customs-register` 的 `regulatory-credential` 子命令同样登凭证版本；ADR-0236 决定三：「集成客户端族与受控登记 CLI 两路不记提交方，列为 NULL」。
- **归类**：A
- **严重度**：低
- **建议修法**：补一句：运维受控 CLI 同登同一种版本，不记登记人。

### L7 UC-CC-003 没有引 ADR-0137，「重新评估触发」仍列为全待确认

- **类别**：2
- **位置**：UC-CC-003 全文无 ADR-0137；「生产参数与待决风险」「判断失效与重新评估触发 | 待确认各来源变化的权威通知、轮询或业务复核覆盖范围」
- **证据**：ADR-0137 决定一（逐门禁判断各成一册、就绪按不可变引用绑定，凭证门禁为第一册）、决定二（评估请求驱动，来源变化只让既有判断失效、不后台重算）。CONTEXT「申报就绪、授权与提交」已写入这两条。「重新评估由什么触发」已定为请求驱动，只剩「失效怎样获知」待定（ADR-0137 越权风险点 2）。
- **归类**：A
- **严重度**：低
- **建议修法**：UC-CC-003 权威依据加 ADR-0137 决定一、二；待决行收窄为「来源变化怎样使既有判断失效、由谁发起下一次评估请求」。

### L8 `CorrectDeclarationHandler` 复用首报那份授权，与 UC 的逐动作授权相反（未接线、已有票）

- **类别**：4
- **位置**：UC-CC-004 `AT-CC-095`「后续动作分别请求新授权，不能复用首次申报授权」；UC-CC-005 `AT-CC-137`；UC-CC-007 `AT-CC-217`
- **证据**：`internal/customscompliance/application/correct_declaration.go` 按单元读 `SubmissionAuthorityView.LoadSubmissionAuthority`，读到的就是首报用掉的那一份。ADR-0227 越权风险点 10 已点明，票 psb/36 承接。`cmd/parcel-dispatch/customs_no_automatic_submission_test.go` 守着它不在生产装配里出现。
- **归类**：C（已有票 psb/36，不另立）
- **严重度**：低（无生产入口）
- **建议修法**：维持「票 36 落地前不得开更正、补充生产口」。CC 修订票里在 UC-CC-007 待决项点一句指向 ADR-0227 越权风险点 10。

### L9 逐包裹、逐申报行范围的 AT 行未注明首发结果范围只到申报单元一级

- **类别**：4
- **位置**：UC-CC-006 `AT-CC-146`（只查验 P2）、`AT-CC-147`（P1 附条件放行、P2 未放行）；UC-CC-009 `AT-CC-279`（只放行 P1）；「明确范围、部分结果与跨客户隔离」节
- **证据**：ADR-0226「范围：今天只有申报单元一级，包含关系退化为相等」；ADR-0197 越权风险点 3「CC 今天说不出部分放行具体放了哪几件」。接收编排要求结果范围就是在册申报单元标识。
- **归类**：C（文档是长期规则，机制只到单元）
- **严重度**：低（ADR 已承认）
- **建议修法**：在这几行或该节加首发注：结果范围只到申报单元一级，单元以下范围等真实程序给出 `PAR-CUS-01/02`「部分范围」后补（ADR-0226 越权风险点 2）。

---

## 已核无问题的要点

| 要点 | 核对结果 |
|---|---|
| 全部相对链接与锚点（CONTEXT、13 份 UC、核心 CC ADR） | 全部可解析 |
| ADR 引用的 AT 号（0069 的 327/337，0137 的 056，0226 的 431–436，0227 的 437–445，0236 的 446/447） | 全部存在且场景对得上 |
| 文档里的英文结果码、原因码（`PARCEL_NOT_ACCEPTED`、`READINESS_UNCONFIGURED`、`AUTHORITY_UNCONFIGURED`、`NOT_READY`、`NOT_AUTHORIZED`、`AUTHORIZATION_EXISTING`、`AUTHORIZATION_NO_LONGER_EFFECTIVE`、`REGISTERED_PENDING_SEND`、`OPERATOR_NOT_GRANTED` 等） | 在 `internal/`、`cmd/` 中都存在 |
| 端点路径（`/customs-cases`、`/customs-submission-authorizations`、`/customs-declaration-submissions`、`/customs-node-zone-registrations`、`/customs/external-results`、`/customs-regulatory-credential-manual-registrations`）与 CLI `node-zone`、`authority-grant` | 与 `cmd/parcel-api` 端点表、`parcel-customs-register` 命令表一致 |
| 动作准入五格、压制次序、依据种类、跨区待定判法、不在册对象的答法（CONTEXT、ADR-0206、UC-CC-013、GLOSSARY） | 与 `domain/action_admission.go`、`domain/node_customs_zone.go` 一致（R12 未落地，见 M12） |
| 关务适用性判断三格与理由、成因各格 | 与 `domain/customs_applicability.go` 一致 |
| 放行门禁核对五值、税费付款核对覆盖、差额、有效性三态取值 | CONTEXT、GLOSSARY 与 `domain/duty_release.go` 一致（UC-CC-009 的偏差见 M7） |
| 同层后续与矛盾判法（CONTEXT「同层后续事件不是同层冲突」一条、UC-CC-006 `AT-CC-431–436`） | 与 ADR-0226 及 `domain/external_result.go` 一致（UC-CC-006 旧条目见 H2） |
| 提交尝试四值与发送时间配对（CONTEXT「提交尝试」、GLOSSARY「申报提交尝试」） | 与 `domain.AttemptResult`、`AttemptSendTimeMatches` 一致 |
| ADR-0236 → 0149/0151 部分停用链 | 两头都在，被停原句在 0149 决定一与 0151 决定四里逐字存在 |
| ADR-0226 ↔ 0197、0213/0219/0225 → 0206 前向指针 | 两头都在、所引原句存在（只有重复问题，见 L1） |
| CONTEXT-MAP CC 各边（含 SA→CC「唯一采用口」、TF→CC 准入拉取） | 与 CONTEXT、ADR-0137、ADR-0206 一致 |
| PILOT-SCOPE 关务条款；参数登记册 `PAR-CUS-01..08` | 与 CONTEXT、ADR-0206、ADR-0227 一致，无冲突 |
| 两份已接受 CC ADR 互相矛盾而无停用关系 | 未发现（0137 决定四与 0236 不冲突：手工采用仍在 SA；0197 决定五与 0226 不冲突） |
