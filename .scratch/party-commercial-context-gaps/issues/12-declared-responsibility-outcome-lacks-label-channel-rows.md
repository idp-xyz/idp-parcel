# `DeclaredResponsibilityOutcome` 没有面单渠道服务的两行：终局规则声明得出网络服务四格，面单渠道的「非取消终局 / 终局失败」在词汇表里没有行，PS 适配器只能如实答「终局规则未配置」

Category: enhancement
Status: resolved——2026-09-10 20:0x 通道 4（task-eff393d5，分支 `mcp4-pcgaps12` 基远端 main `dede3c2e`，代码 tip `c5afe453`）：CONTEXT 一句先进、领域两值、迁移 `0031`、库 / 批文反查改走领域、PS 适配器逐格翻译并删短接，真库往返 PASS；完成记录与给评审的判断题见 Comments 末条；进 main 待推送方非作者评审后重放。此前 in-progress——2026-09-10 19:0x 通道 4 认领（task-eff393d5），分支 `mcp4-pcgaps12` 基远端 main `dede3c2e`；此前 ready-for-agent——2026-09-10 17:1x 通道 1 推送方代裁「要裁的」三条（用户经队列授权「你自决」，读法见 tasks.md 16:5x–17:0x 节；三条均属命名 / 文档落位 / 有判据的技术选型，见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无（lc/11 的 PS 半边 `JudgeLabelServiceFinalHandler` 已在 main `0e5a4ea`；本票是它「不在本票 · PC 半边」那一条）

## 缺口（取证于 `3f485e97`）

- `internal/partycommercial/domain/service_stage_content.go` 的 `DeclaredResponsibilityOutcome` 封闭四值：`EFFECTIVE_DELIVERY` / `RETURN_COMPLETED` / `SERVICE_TERMINATED` / `REGULATORY_DISPOSITION`，头注写「与 parcel-shipment 结果联合的四格语义对应」——四格全是网络服务的责任结果。核：`git show HEAD:internal/partycommercial/domain/service_stage_content.go`。
- PS 消费侧适配器 `internal/parcelshipment/adapters/partycommercial/service_stage_rules.go` 注释原句：「面单渠道服务的两格（非取消终局结果 / 终局失败结果）在提供方的声明词汇表里今天没有行」；那两格在适配器里 found=false，编排停在 `FINAL_RULE_UNCONFIGURED`。核：`git grep -n 面单渠道 -- internal/parcelshipment/adapters/partycommercial/service_stage_rules.go`。
- 于是首个面单渠道产品的租户**没有办法登记**「面单渠道非取消终局形成哪种终局类型 / 终局失败形成哪种」这两行声明；`JudgeLabelServiceFinalHandler`（PS，lc/11 落 `0e5a4ea`）对面单渠道包裹永远拿不到规则，只能未决。这是 [remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-3。
- `PublicationVocabulary`（`publication_vocabulary.go`）把 `outcome` 一栏按 `DeclaredResponsibilityOutcome.valid` 列封闭码，管理台表单据它供下拉（awf/20）——词汇表少两行，表单就选不出来。

## 语言从哪里来

- PC `CONTEXT.md` 词条「面单服务终局规则」：「接单规则包版本对面单渠道服务终局边界、适用结果作出的商业定义……规则必须明确授权角色、适用范围、关闭责任来源和硬限制前置条件。」Rules 节：「面单服务终局规则必须显式定义正常终局边界和取消权结束条件」。
- PS `CONTEXT.md` 词条「终局服务结果」：「网络服务和面单渠道服务可以具有不同终局结果，实际交付不是所有服务形态的统一完成条件」；Rules：「面单渠道服务在面单服务终局边界成立或形成有效取消结果时判断终局，不默认等待实际运输交付，也不把渠道受理成功直接当作终局」；生命周期：「`transport-fulfillment` 提供实际承运商首次有效收寄事件 → 面单渠道服务非取消终局结果」与「当前受控关闭已经生效且未被重开……→ 终局服务结果」。
- lc/11 `## Answer`「不在本票 · PC 半边」原句：「`DeclaredResponsibilityOutcome` 加面单渠道两行并在 `service_stage_rules.go` 补逐格翻译（删掉本票那一格）。PC 地盘，建议另立票；落地前面单渠道终局的采用一律停在 `FINAL_RULE_UNCONFIGURED`。」
- PS 侧的两格名从 `judge_label_service_final.go` 的结果代数取，不在本票另起名。

## 做法（按裁决落地；顺序即依赖）

1. **先改 CONTEXT 一句**：PC `CONTEXT.md` 词条「面单服务终局规则」或 Rules 那句「面单服务终局规则必须显式定义正常终局边界和取消权结束条件」——补一句声明形状：终局规则对面单渠道服务的**非取消终局**与**终局失败**两种责任结果各可声明一行终局类型；缺行在采用层读成「此产品下不形成终局」（与网络服务四格同一读法，`FinalizationDeclaration` 头注已有这句）。封闭集加格是领域语言改动，先改文再改码。
2. **领域**：`DeclaredResponsibilityOutcome` 加两值（名字照 PS 结果代数译成本上下文词，`String()` 给原词），`valid()` 上界随之；`closedSetNamed` / `DeclaredResponsibilityOutcomeNamed` 反查跟随；`FinalRuleContent` 构造门对「同一结果两行」的拒绝不变。
3. **规范化与库**：`publication_canonicalization_acceptance_rule_package.go` 的 `outcome` 反查与文档写法是否换号按 ADR-0014 判（只加封闭码、文档形不变 → 倾向不换号，票面记理由）；迁移 `party_commercial/0013` 的 `final_rule_*` CHECK 若钉了四个字面量，**新开序号**放宽（不改已施加迁移）。
4. **发布与批文**：`PublicationVocabulary` 的 `outcome` 一栏自然多两行；`cmd/parcel-commercial/translate.go` 的 `finalRuleDocument` 与 awf/12 接单规则包表单的下拉不改代码只多两个可选值——完成判据要核它们真列出来。
5. **PS 适配器**：`service_stage_rules.go` 补两格翻译、删「今天没有行」那句注释；`JudgeLabelServiceFinalHandler` 的 `FINAL_RULE_UNCONFIGURED` 对面单渠道从此只在租户真没登时出现。

## 红线

- 只加声明槽，不给任何默认终局类型；首个面单渠道产品的终局类型取值属实例半边（`PAR-COM-12` / `PAR-COM-17`），一行都不预填。
- 不在 PC 复制 PS 的终局生命周期（PC CONTEXT Rules：「具体请求、决定、权威截断、并发裁决、继续尝试判断和终局历史由 `parcel-shipment` 拥有，不在本上下文复制第二套生命周期」）——本票只多两个可声明的责任结果，不多任何判断。
- 不改已施加迁移；不改 lc/11 已 resolved 票面 Status。
- 面单渠道的**取消**结果不进这两行：取消不是终局规则声明的对象（PS CONTEXT「除已经形成的有效取消结果外……」）。

## 完成判据（非作者评审逐项对）

1. PC `CONTEXT.md` 那一句先于代码改动进 main，票面引其原句。
2. `DeclaredResponsibilityOutcome` 六值往返：`String()` / `DeclaredResponsibilityOutcomeNamed` / `closedSetNamed` 用例覆盖新两值；`NewFinalRuleContent` 对新两值各一行成立、同值两行仍拒。
3. 真库：`SaveFinalRule` / `LoadFinalRule` 对带新两行的正文往返，`0013` 的 CHECK 不再拒它们（若原 CHECK 钉字面量，新迁移序号在票面写明）。
4. 规范化：`PCC-*` 是否换号在票面写理由并有用例钉住旧文档仍按原版本重放。
5. `service_stage_rules.go`：面单渠道两格从 found=false 变成有行时能译回、没行时仍 found=false；「今天没有行」注释删去；`judge_label_service_final_test.go` 里靠「PC 无行」造未决的用例改为靠「租户未登」造。
6. `go build ./...` / `go vet ./...` 零信号；`internal/architecture` 两道棘轮不加宽；机制清点在 tip 重生成。
7. 管理台接单规则包表单（awf/12）与 `/publication-vocabulary` 读面（awf/20）实际列出两行——只核列出，不改前端。

## 地盘

`internal/partycommercial/domain/`、`internal/partycommercial/adapters/postgres/`（`stage_content_declaration.go` / `declaration_publication.go` 若 CHECK 要放宽）、`migrations/party_commercial/`（新序号）、`docs/domain/party-commercial/CONTEXT.md`、`internal/parcelshipment/adapters/partycommercial/service_stage_rules.go`（PS 消费侧一文件，跨地盘要在频道占号）。`cmd/parcel-commercial/translate.go` 预计零改动。

## 要裁的

1. **两行的名字**：PS 结果代数里「非取消终局结果 / 终局失败结果」译进 PC 封闭集叫什么原词（`LABEL_CHANNEL_FINAL` / `LABEL_CHANNEL_FAILED` 只是占位，本票不定）——归 PC owner，一句。
2. **CONTEXT 改哪一句**：加进词条「面单服务终局规则」正文，还是加进 Rules「必须显式定义正常终局边界和取消权结束条件」那条——归 PC owner，一句。
3. **规范化换不换号**：只加封闭码、文档形不变，倾向不换；若 owner 认为「可选值集合变了就是文档形变了」则按 ADR-0014 换号——归 PC owner。

**裁决（通道 1，推送方代裁；2026-09-10 17:1x；用户经队列授权「你自决」，读法见 tasks.md 16:5x–17:0x 节；三条经通道 4 分类均为 A 类——命名 / 同一份文档内的落位 / 有既定判据的技术选型，不动领域归属）**：

1. **两行的原词取 `LABEL_SERVICE_COMPLETED`（非取消终局结果）与 `LABEL_SERVICE_FAILED`（终局失败结果）**，Go 常量 `DeclaredLabelServiceCompleted` / `DeclaredLabelServiceFailed`。照既有四值的构词（`EFFECTIVE_DELIVERY` / `RETURN_COMPLETED` / `SERVICE_TERMINATED` / `REGULATORY_DISPOSITION`：大写蛇形、「主体 + 结果」名词），主体取 PS 词条「面单服务终局」的 `LABEL_SERVICE`，与 `LabelServiceFinalOutcome` 同一个词根；不用 `LABEL_CHANNEL`（渠道是通道，终局是服务的）。`service_stage_content.go` 上 `DeclaredResponsibilityOutcome` 头注「封闭四值」那句随改——改成不数格（AGENTS.md「计数与行号同构」），只说「与 PS 结果联合的各格语义对应」。越权风险点（供 PC owner 事后复核）：两串原词的措辞。
2. **CONTEXT 改词条正文**：加进 PC CONTEXT「面单服务终局规则」词条正文一句——规则可为其声明终局的责任结果含面单渠道服务的非取消终局与终局失败两格；Rules「必须显式定义正常终局边界和取消权结束条件」那条**不动**——它是不变式，两行是它的实例。
3. **不换号**：只加封闭码、文档形不变，判据 ADR-0126 决定一「加键不换号」的同一精神（可选值集合扩大不改变已发布文档的字节），同形先例 pc-gaps/09 加 `Validity()` 槽、awf/25（`852bf7c4`）加 `omitempty` 键都未换号；写一条测试钉「既有四值声明的文档摘要与本票前逐字节同」。

## 参照

- [lc/11](../../label-channel-service-first-release/issues/11-parcel-final-across-transactions.md)（`## Answer`「不在本票」）；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-3。
- ADR-0058（阶段内容归接单规则包版本）、ADR-0062、ADR-0088（面单渠道服务进首发）、ADR-0119（终局规则上的有效期声明槽——同一份正文上一次加槽的先例）、ADR-0014（规范化换号）。
- 同族先例：pc-gaps/09（`FinalRuleContent` 加 `Validity()` 槽，迁移 0026 / `d06192ed`）——形状、迁移与批文都照它。

## Comments

- 2026-09-10 · 通道 4：立票（task-9880bbc9）。起因是 remaining-work-dd5ed934 五-3 判「仍余且机制半边且无票」；本目录 spec 14:4x 刚按十一张全 resolved 收口，本票同笔把 spec 改回 in-progress 并记原因。未动代码。
- 2026-09-10 20:0x · 通道 4（task-eff393d5，分支 `mcp4-pcgaps12` 基远端 main `dede3c2e`）：**完成记录，转 resolved。** 逐笔（分支 SHA 只作此刻取证，推送方重放后以 main 上的为准）：`ba6e823f` docs Status → in-progress；`544bad5f` docs PC CONTEXT「面单服务终局规则」词条正文一句（做法 1）；`366a7178` feat 领域两值 + 四处用例（做法 2）；`217d153c` feat 迁移 `0031` + 库读口反查改走领域 + 真库往返用例（做法 3）；`e54f8d3d` feat PS 适配器逐格翻译、删短接、两处注释（做法 5）；`876831b7` feat 批文反查改走领域 + 用例（做法 4）；`1675f830` docs 清点在 `876831b7` 干净检出重生成（party_commercial 30 → 31）；`c5afe453` fix 自评三处（`0031` 末换行、三处注释去掉对别处格数的计数）。代码 tip = 分支 tip = `c5afe453`。
  **完成判据逐项**：
  1. CONTEXT 句先于代码进（`544bad5f` 早于 `366a7178`）。原句：「规则可为其声明终局的责任结果，除网络服务各格外，还含面单渠道服务的**非取消终局结果**与**终局失败结果**两格，每格各可声明一行终局类型；缺行在采用层读成「此产品下这种结果不形成终局」，与网络服务各格同一读法；已形成的有效取消结果不在可声明之列。」加在词条正文「本规则不代替针对具体包裹形成的继续尝试决定」之后、面单有效期那段之前；Rules「必须显式定义正常终局边界和取消权结束条件」那条一字未动（裁决 2）。
  2. 六值往返：`TestFinalRuleContentDeclaresLabelServiceOutcomes`（两值 `String()` 原词、`NewFinalRuleContent` 对新两值各一行成立、同值两行仍 `ErrConflictingFinalization`、`Declarations()` 顺序网络格在前面单格随后、只声明网络格时面单格缺行）；`TestClosedSetNamedLookupsRoundTripAndRefuseOutsiders` 的 `DeclaredResponsibilityOutcome` 行加两成员，集外词加 `LABEL_CHANNEL_FINAL`（票面被裁掉的占位）/ `LABEL_SERVICE_OUTCOME`（PS 原词）/ 小写形。Go 常量 `DeclaredLabelServiceCompleted` / `DeclaredLabelServiceFailed`，头注不再数格并写明原词来处与取消为何不在此列（裁决 1）。
  3. 真库：`0013` 的 `final_rule_declaration_closed_set` 确实钉了字面量，新迁移 **`0031_final_rule_declaration_label_service_outcomes.sql`** DROP 再 ADD 同名 CHECK 放宽到六个原词，`0013` 一字未动；`TestFinalRuleLabelServiceRowsRoundTripThroughTheDatabase` 经 `SaveFinalRule` → `LoadFinalRule` 带两行往返、网络格读回即缺行；集外词仍拒的既有用例不变。
  4. 不换号：仍 `PCC-1`，理由照裁决 3（只加可选值、文档字节不变，同 ADR-0126 决定一「加键不换号」精神与 pc-gaps/09 / awf/25 先例）；`TestLabelServiceOutcomesDoNotRenumberTheCanonicalization` 钉住基线 `dede3c2e` 上对 `fullRulePackageBody` 算得的字面摘要 `PCC-1:03bd7ee5…5cd305a1`（在 `dede3c2e` 的临时检出上用探针用例取得，本票后逐字节同），并证新两行进文档、`RehydratePublicationContent` 折回。批文 `translate.go` 的 `finalRuleDocument` 形不变。
  5. `service_stage_rules.go`：删 `IsLabelService` 短接与「今天没有行」那段注释，`declaredOutcomeFor` 加 `LabelServiceOutcome → DeclaredLabelServiceCompleted`、`LabelServiceFailure → DeclaredLabelServiceFailed`，default 仍 `ErrUntranslatableAnswer`（ADR-0025）；`TestFinalJudgmentTranslatesLabelServiceRows` 每格三段：有行译回带 `PAR-COM-17/<kind>`、缺行 `OUTCOME_NOT_FINAL_FOR_PRODUCT/LABEL_SERVICE_*` 且 found=true、租户未登 found=false。`judge_label_service_final_test.go` 那条「靠 PC 无行造未决」的用例——实测其夹具本就是 `rules.configured=false`（租户未登），只有 Covers 注释写成「PC 词汇表未声明」，故只改注释并点明缺行走 `FINAL_RULE_NOT_SATISFIED_YET`。
  6. `gofmt -l` 空、`go build ./...` / `go vet ./...` 全仓零信号；`internal/architecture` 全 ok，两份 `*_baseline.txt` 零改动；清点在干净检出重生成。
  7. 词表：`TestAcceptanceRulePackageVocabularyCarriesTheWordsTheFormTicketNames` 钉 `outcome` 六个字面串按声明顺序；管理台 `AcceptanceRulePackagePublicationForm` 责任结果一格是 `VocabularySet set="outcome"`、读 `/publication-vocabulary`，不内置枚举——**未开浏览器实核**，凭服务端词表用例 + 表单读法两处代码取证。`presentation.ts` 的 `finalOutcomeLabels` 没有这两行的中文，按「集外取值原样示出」显示英文原名；要中文标签另立 awf 票，不在本票。
  **验证强度**：隔离树 `mcp4-pcgaps12@876831b7` 带 DSN `-p 1 -count=1`：PC 五包 + 反向依赖（`cmd/*` 十一包、NR/PS/SA/TF/VE 的 `adapters/partycommercial`、PS `adapters/postgres`）+ `architecture` + `migrations` + `platform/migrate` + PS domain/application 共 27 包：26 ok / 0 FAIL / 1 无用例 / 0 cached，48 s；`-v` 下 PASS 3276 / SKIP 0 / FAIL 0；分包 PC adapters/postgres PASS 272、cmd/parcel-commercial 137、platform/migrate 5、PS adapters/partycommercial 176、PC domain 627，SKIP 均 0。修复笔 `c5afe453` 后复跑 migrations / platform/migrate / PC adapters/postgres 带 DSN：3 ok，PASS 280 / SKIP 0 / FAIL 0。未跑全量（推送方那一跑是 main 真值）。
  **偏离票面预测三处（供评审判，理由各附）**：① 票面「`translate.go` 预计零改动」——红测证明 `responsibilityOutcomeFrom` 自抄名单会把 `LABEL_SERVICE_FAILED` 当集外拒在触库前，改走 `pcdomain.DeclaredResponsibilityOutcomeNamed`，`TestFinalRulesTranslateLabelServiceOutcomes` 钉之；② `internal/parcelshipment/domain/final_outcome.go` 不在地盘，只改 `IsLabelService` 头注一句（原句断言提供方词汇表「今天只有网络服务四行的词」，本票后为假），签名不动；该方法此后无生产调用点，删不删归 PS，本票不动；③ 库读口 `declaredResponsibilityOutcomeFrom` 从「加两行」改成走领域反查（同文件另两个 `*From` 仍自抄名单，非阻断，未动）。
  **自评（/code-review 两轴串行自跑，子代理不可用）**：Standards 两条已修（`0031` 末换行；三处注释数了别处的格数）；Spec 无阻断。非阻断留票：`IsLabelService` 无调用点；PC postgres 另两个 `*From` 仍自抄名单；admin-web 两行无中文标签。
  **给评审的判断题**：(a) CONTEXT 句末「已形成的有效取消结果不在可声明之列」是否只是 PS CONTEXT「除已经形成的有效取消结果外」在提供方侧的同义复述，而非新增一条规则；(b) 面单格缺行从此走 `FINAL_RULE_NOT_SATISFIED_YET`（与网络格同一读法）而不再是 `FINAL_RULE_UNCONFIGURED`——对 UC-PS-004 `AT-PS-100`「保持终局未决」仍成立，但未决原因换了格，PS 侧文案要不要跟；(c) `0031` DROP/ADD 同名 CHECK 算不算「改已施加迁移」——我判不算（`0013` 一字未动，`0026` 放宽 anchor CHECK 的写法是先例）。
  **地盘外零改动**：`migrations.go` / `plan.go`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`apps/**`、`internal/architecture/*_baseline.txt`、ADR 正文、GLOSSARY。
