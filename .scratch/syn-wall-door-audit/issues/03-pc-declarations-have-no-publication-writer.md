# party-commercial 声明与发布无登记口,接受链六墙同根等一扇门

Category: enhancement
Status: in-progress

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W04–W08、W11(声明面)、W12。

## 墙(六处哨兵,同根)

- `COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED` / `PC-ACCEPTANCE_CONTENT_NOT_CONFIGURED`(`parcelshipment/adapters/partycommercial/commercial_basis.go`)
- `ANCHOR_POLICY_NOT_CONFIGURED`(`partycommercial/domain/commercial_resolution.go`)
- `NOT_CONFIGURED`(`JudgmentAsOfOutcome`)、`REACHABILITY_AS_OF_NOT_CONFIGURED`、`FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`(`judgment_continuation.go`)
- `REJECTION/WITHDRAWAL/SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED`、`RULES_NOT_CONFIGURED`(`AuthorizationOutcome`)
- `CONTROL_POLICY_NOT_CONFIGURED`(`settlementaccounting/application/apply_pre_acceptance_control.go`)
- `ELIGIBILITY_UNDECIDED` 资格声明面 / `FINAL_UNDECIDED` 终局规则面(`adopt_network_intake.go`、`form_parcel_final.go`;装配点注释「不得为变绿去种 PAR-COM-16/17 声明行」)

## 现状:有装载无写入

party_commercial 迁移 0002–0014 的表与只读装载口全部就位,`resolve_commercial_basis` 解析用例真实可用;但**发布侧没有用例**:

- 六族声明表零 INSERT(非测试代码):`as_of_policy_declaration`、接受内容族、`pre_acceptance_control_declaration`、`customer_contract_content`、阶段内容族、`acceptance_rule_package`。

  > **2026-08-21 更正**：原文把两族写成了 `acceptance_content_declaration` 与 `stage_content_declaration` 两个表名，`migrations/party_commercial` 里**查无此表**——那是审计当时的族名简写。现行 schema 里接受内容族是 `acceptance_rule_content` / `acceptance_rule_check_group` / `pending_routing_permission`，阶段内容族是 `intake_qualification_content` / `intake_allowed_source` / `intake_qualification_ref` / `final_rule_content` / `final_rule_declaration` / `cancellation_authority_content` / `cancellation_authority_declaration`。判据背后的性质不变，核的时候按真表名核。
- `commercial_version` / `commercial_resolution` / `authorization_grant` / `service_product_form` / `price_policy` / `settlement_policy` 有仓储级写入方,但无发布用例、无进程入口。
- 当前唯一填充路径是测试内隔离种子(`cmd/parcel-dispatch/syn_pc_seed_test.go`,SYN-RES-01,S 级)。

## 缺的最小机制件

1. 商业发布用例(publication):按批发布版本化声明(PAR-COM-14/15/16/17 的机制半边),写 `commercial_version` + 各 kind 声明表,带发布批准责任与有效区间;不可覆盖既有版本。
2. 消费方解析键来源(`ResolutionKeySource`)的实例登记面:范围/法人候选/锚点策略/必需依据种类(今天 nil 即显式未配置)。
3. 进程级登记口(端点或受控 CLI)。

另两条缝在本票范围外注明:控制金额缝(估价,依赖票 07)、控制作用域缝(ADR-0044 已让结算政策可观察,接通归本票)。

## 红线

- 声明内容全部属实例半边:本票只建发布机制,不种任何生产默认;隔离 S 种子继续只活在测试里。
- 一决策一处定义:发布用例引用 PAR-COM-* 登记册,不复制第二套参数口径。

## 参照

ADR-0027、ADR-0044、ADR-0058、ADR-0062;PAR-COM-14/15/16/17。

## Comments

- 2026-08-20 · MCP-3：对 3324ecb 重核四件（只读）。**核心仍成立，范围收窄一角。**
  仓储表：在（票面已记，另 `party_commercial` 迁移现到 0014，74a6c35）。装载口：在，且比
  取证时厚——`commercial_publication.go`（26864d9，2026-08-18）落了 `ports.PublicationRegistry`
  持久化面：`LoadForScope` 单语句整册装载（版本+形态+价格政策+结算政策+区间更正），
  `SaveVersion` 写 `commercial_version`（撞键不覆盖、重放/冲突分格）——票面「缺的最小
  机制件」第 1 件的**仓储半边已被它覆盖**。写入方（应用用例）：仍零——`application/`
  只有 validate/resolve/form_judgment/adjudicate 四用例（最后触碰 6d6397f），无发布用例；
  六张声明表非测试 INSERT 仍为零（只在四个 `*_test.go`）。进程级登记口：仍零。
  `ResolutionKeySource` 只在 `parcelshipment/adapters/partycommercial/commercial_basis.go`，
  实例登记面仍缺（件 2 原样）。建议：ready-for-agent，票面第 1 件改写为「发布**用例**
  （经 PublicationRegistry 写版本册 + 各 kind 声明表）」，仓储半边引 26864d9 勿双做。
- 2026-08-20 MCP-1：采纳重核，Status → ready-for-agent。派 MCP-3 实现；仓储半边勿双做。
- 2026-08-21 · MCP-4（用户经队列明确授权代裁）：**两现场取舍裁定——以 MCP-3 现场为集成候选（结构裁定）。**
  取证（均只读）：两现场同基底 `d6453a7`（含 26864d9）；死会话现场已封存为 `db81745`（dead-session-salvage 票 02，提交信息自注「非集成候选」，含一个死在编辑半途的测试文件）；重叠面为真冲突——`domain/as_of_policy.go`、`domain/customer_contract.go`、`domain/service_stage_content.go`、`ports/ports.go` 两边各写一版，互差 79/79 行；MCP-3 版用例全程走 `ports.PublicationRegistry`（≥10 处引用），死会话版用例仅 1 处提及且另建六个独立写 adapter（写于「勿双做」收窄之前）；覆盖面上 MCP-3 版三件全有雏形（发布用例＋解析键实例登记面含迁移 0007＋`cmd/parcel-commercial`），死会话版仅第 1 件约七成＋第 2 件雏形。
  裁定四条：
  1. `wt-mcp3-pc-publication` 现场为集成候选，按本票面继续；domain/ports 的形状归它。
  2. `mcp1-pc-publication`（`db81745`）维持封存，降为参照采石场——不整体合并、不 cherry-pick。
  3. 收口前 MCP-3 须对照封存件做一次吸收扫描（六写口的判定＋`publish_commercial.go` 318 行＋275 行测试断言），B 版漏的按需以自己的写法吸收，扫描结果记回本票（无可吸收也记一句）。
  4. 封存分支去留不在本裁定内，随 dead-session-salvage 票 02 在关票时处置。
  能力边界：本裁定基于两现场文件清单与 diffstat、端口用法 grep、票面重核记录与封存提交自注，未逐行读两版用例的不变式与测试断言——因此只裁结构（谁当集成基座），不裁具体不变式；后者随实现归 MCP-3。
- 2026-08-21 · MCP-1（接手实现，用户经队列授权「你现在是主控制方」「开工」）：**集成候选已封存、已跑绿、已合入当前 main**，并按上条裁定第 3 条做了吸收扫描。

  **一、现场先抢救。** 裁定作出时 MCP-3 现场还是未提交状态躺在 `D:/tops/wt-mcp3-pc-publication`（14 个路径、4274 行，分支相对 main 0 ahead），一次 `git worktree remove --force` 即全部无声消失。按 dead-session-salvage 票 02 的「先封存再拆」口径原样提交为 `c8abe94`，不做取舍不改一行。

  **二、跑绿暴露三处门禁违规，逐条修（`364606f`）。** 封存提交自注「不验证、不保证可编译」，实测编译得过但门禁三红：
  1. `commercial_resolution_keys_test.go` 四处在事务回调里调 `t.Fatal`/`t.Fatalf`（`TestNoTransactionClosureCarriesAGoexitAssertion`）——`runtime.Goexit` 让回调永不返回，提交与回滚两条分支都被跳过。闭包改成只做 IO 回 error。
  2. `commercial_resolution_keys.go` 落在 `adapters/postgres` 却 import `partycommercial/domain`（`TestBusinessModulesDoNotReachIntoEachOther`）。
  3. 整个文件搬去 `adapters/partycommercial` 之后立刻撞第二条：`pgx` 只许在持久化适配器里碰（`TestBusinessPackagesDoNotTouchTheDriverDirectly`）。

  **2 与 3 互相夹住**：一个既拼 SQL 又造 `ClosureResolutionKey` 的类型在本仓架构下两头违规，立不住。只能拆两半，中间过一个只有基本类型的行——`adapters/postgres/commercial_resolution_key_store.go` 搬运字符串行，`adapters/partycommercial/commercial_resolution_keys.go` 做校验与 `pcdomain` 翻译。

  **三、吸收扫描结果（裁定第 3 条要求记回）。**
  - **位置：封存件是对的，已吸收。** `db81745` 把解析键登记面放在 `adapters/partycommercial`（137 行、无 SQL、无迁移），本分支放在 `adapters/postgres`（272 行、含 SQL 与迁移 0007）。两份各对一半：封存件位置对、本分支持久化对。上面那一拆同时吸收了封存件的位置判断。
  - **六写口的形状：不吸收，记差异。** 封存件把 `Save*` 挂在既有读仓储同一个类型上（如 `AcceptanceContentDeclarations` 同时有 `LoadAcceptanceRuleContent` 与 `SaveAcceptanceRuleContent`），六种声明各自读写同处；本分支把六种的写集中在一个 `declaration_publication.go`。集中式的代价是每种声明的读写列映射分居两文件、有漂移余地；但裁定已把结构归本分支，且集中式那份 790 行加 639 行测试已成体系，重排收益不抵风险。**记此备查，不改。**
  - **测试断言：找到一条真缺口，尚未补。** 封存件的 `TestPublicationBatchKeepsSavedProductWhenContractConflicts` 断言发布批**逐项独立成败**（AT-PC-011）：同批两项，产品落库、合同撞内容冲突，断言 `len(registry.saved)` 仍为 1——「合同冲突把已合法产品从写入面撤走了（全量回滚）」。本分支在 `cmd/parcel-commercial/main.go` 两处与 `publish_commercial_authority.go` 一处引用 AT-PC-011，**但测试里没有任何批内部分落点的断言**（十三个测试逐个看过，最近的是 AT-PC-010 的未决格）。**声称有、没测过**——收口前须以本分支的写法补一条同义断言。
  - 其余：本分支十三个测试覆盖面显著大于封存件四个（重放/冲突分格、计划态与生效态、未确认角色未决、声明随属主版本发布、错属主种类拒收、合同内容与壳引用一致、登记册读不回即阻断，外加适配器层往返/重放冲突/无事务拒三条），无其他可吸收项。

  **四、AT-PC-011 那条已补（`TestAConflictingItemDoesNotRetractAnEarlierSavedItem`）。** 以本分支的单对象形状重写：同批第一项全新对象落库、第二项撞册上正文冲突，断言 `savedVersions` 仍恰为第一项。**但只补上了一半，另一半照实记**——该用例走登记册替身、没有事务，守得住「冲突项不入册也不动前项、两次调用间处理器不留共同状态」，**守不住事务边界**：哪天有人把 `cmd/parcel-commercial` 那个逐项各起事务的循环整个包进一个事务，前项就会被后项带走，而现有用例一条都不会红。要堵这一格得有一条对真库跑 `runPublish` 的用例，今天没有——`cmd/parcel-commercial` 只有 `translate_test.go` 的纯翻译测试。**列为收口前的待办。**

  **五、另记一处未被门禁抓到的同形隐患。** `internal/partycommercial/adapters/postgres/declaration_publication_test.go` 的 `mustSaveDeclaration` 也在事务回调里调 `t.Fatalf`（outcome 不符那一格），与本轮第 1 条修的是同一个缺陷形状，但 `TestNoTransactionClosureCarriesAGoexitAssertion` 没有报它——门禁能顺着 `mustWithinTransaction` 那个助手追进去，却没追 `mustWithinPublicationTransaction`。只在断言失败时才发作，因此绿着看不见。**本轮不改**（不在票 03 范围，且改法与门禁能力边界要一起看），记此备查。

  **六、对票面判据逐条核（附一处票面自身的表名更正）。**
  - 票面「六张声明表零 INSERT（非测试代码）」这条判据的表名有两个不存在：`acceptance_content_declaration` 与 `stage_content_declaration` 在 `migrations/party_commercial` 里**查无此表**，是审计当时的族名简写。现行 schema 把这两族拆得更细——接受内容族是 `acceptance_rule_content` / `acceptance_rule_check_group` / `pending_routing_permission`，阶段内容族是 `intake_qualification_content` / `intake_allowed_source` / `intake_qualification_ref` / `final_rule_content` / `final_rule_declaration` / `cancellation_authority_content` / `cancellation_authority_declaration`。**判据背后的性质仍成立且现已满足**：`declaration_publication.go` 非测试写入覆盖十六张表，六族全有生产写入方，零 INSERT 的现状已不成立。
  - 件 2（`ResolutionKeySource` 实例登记面）：生产实现在 `internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`（唯一非测试 `ResolutionKeySource` 实现），持久化半边在 `adapters/postgres/commercial_resolution_key_store.go`，表由 `migrations/parcel_shipment/0007_commercial_resolution_key.sql` 建（主线 0001–0006，0007 空号不撞）。
  - 件 3（进程级登记口）：`cmd/parcel-commercial`，两个子命令 `publish` 与 `register-resolution-key`。
  - 票面另六个对象（`commercial_version` / `commercial_resolution` / `authorization_grant` / `service_product_form` / price_policy / settlement_policy）原本就有仓储级写入方、缺的是发布用例与进程入口，两者现由 `publish_commercial_authority.go` 与上述 CLI 补齐。

  **七、`runPublish` 真库用例已补（`TestPublishBatchKeepsEarlierItemWhenALaterItemConflicts`），但只补到该补的地方，边界照实交代。** 端到端跑进程口：先发一份 `rules-conflict`，再发一批两项（全新 `rules-fresh` 在前、同对象异正文在后），断言退出码是 `exitAttention`、两个对象各恰一行版本。这条不可能空过——后一项撞得出 `CONTENT_CONFLICT`，前提就是前一次的行**真的已提交**到库里。它同时是件 3 进程口的首份真库证据。

  **它仍守不住事务边界，这一格确认无守门人。** 原以为它能钉住「逐项各起事务」，实测推翻：**冲突不是错误**——把 `runPublish` 的循环整个包进一个事务，第二项照样判冲突、事务照样提交，该用例依旧绿。真要钉住那个结构，得让后一项以技术失败收场再看前一项还在不在，而技术失败今天只来自基础设施故障，从批文里造不出来。注释里已写明，不留一条名不副实的守门人。

  **八、当前状态。** 票面正文那两个不存在的表名已就地更正（加更正框，不改写原句）。分支 `mcp3-pc-publication` 已合入 `main`（`ac04366`），合并无冲突；分支 HEAD 上 `gofmt` 干净、`go build`/`go vet` 全过、`go test -count=1 ./...` 全绿，且是含真库的绿。三件均有实现、判据均可核、吸收扫描已毕并已吸收。**可转 resolved 并推已验 SHA，等一句放行。**
