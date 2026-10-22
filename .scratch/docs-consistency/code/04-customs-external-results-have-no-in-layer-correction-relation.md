# 04 CC 外部结果没有层内更正关系：来源明确的更正照判同层冲突

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code04（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：外部结果接收已在生产装配（`cmd/parcel-api/assemble_external_results.go`）；监管核定税费换金额、处置决定换种类、放行收窄这类明确更正一到，就被判冲突，进不了当前判断，也到不了 VE。偏保守（不会误放），但核定税费的更正碰钱、且冲突今天无人可见、无处置入口。首发关务只接受控模拟与沙箱，真实程序的更正形态（`PAR-CUS` 一类）尚未给出，所以列 R2；接真实关务程序之前升 R1。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/cc.md](../audit/cc.md) M9（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

CC CONTEXT 与 UC-CC-006 要求「同一层次和业务事项的后续明确更正形成新结果及更正关系」，`AT-CC-156` 要求按新旧范围交集、差集分别判断。CC 没有层内更正关系：接收载荷与领域对象都没有「更正哪一份」，一致性核对只能判「后续事件」或「矛盾」。所以来源明确的更正（核定税费换金额、处置决定换种类、放行收窄）今天一律判同层冲突。

## 证据

- 文档：
  - CC CONTEXT Lifecycles「提交与外部监管结果」：「后续监管更正形成新的外部事实及当前判断，不覆盖原放行结果」。
  - UC-CC-006「来源权威与解释规则」：「同一层次和业务事项的后续明确更正形成新结果及更正关系；原结果保持历史」；`AT-CC-156`：「来源明确发布对旧结果的更正或替代版本……形成新来源事实及更正关系」。
- 代码与决定：
  - `internal/customscompliance/domain/external_result.go` 的 `CheckLayerConsistency`：其余四层一版一结论，异义即冲突；注释写更正「得由来源指明」，而载荷没有这一格。
  - ADR-0226 Context：「CC 没有层内更正关系」；越权风险点 5：「监管核定税费换金额、处置决定换种类照判冲突，要等层内更正关系」。票 [demo-chain-gaps/12](../../demo-chain-gaps/issues/12-successive-same-layer-customs-results-are-judged-conflicts.md)（已 resolved）的代决第 3 条同此。

## 三类

机制：更正关系的形状、一致性核对先认更正再判矛盾。产品策略：范围交集差集的判法（UC-CC-006 已定）。租户取值：真实程序怎样标「更正哪一份」由连接器按公开规范或沙箱给出，没给出的来源仍按今天判。

## 规模

M：接收载荷与 `ExternalResultRecord` 加「更正哪一份」、迁移另取号、`CheckLayerConsistency` 按更正关系分支、当前判断按交集差集派生；VE 侧派生沿用替代链。

## 最小的诚实修法

1. 接收载荷可选带「更正的来源标识」；带了且所指在册、同层同提交版本，就形成更正关系，不进矛盾判断。
2. 当前判断：交集按更正后的结果，旧结果未覆盖的范围保持原判断，新扩展范围独立判断（`AT-CC-156`）。范围今天只到申报单元一级（ADR-0226），交集差集先在单元一级退化处理，单元以下随 `PAR-CUS-01/02`。
3. 原结果保持历史，不改写。
4. 不带更正指向的照今天判（后续事件或矛盾），不从业务时间猜更正。

与票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md)（冲突关系）改同一组文件，宜同批或先后紧接。

## 要裁的

- 所指的那一份本身已被更正过时：按链取当前那一份，还是只许更正链尾。归 CC owner，作者可代决并写越权风险点。
- VE 收到更正后的外部结果时怎样替代原条目：沿用 VE 替代链（推荐）还是另立更正关系。

## 完成记录

2026-10-21，分支 `claude-code04`（基于 main `be6b5ad20`），用户授权「你作为业务和系统专家自决……解决所有的问题」。

### 做了什么

- 领域：外部结果多一格来源内容「更正哪一份」（`ExternalResultSpec.Corrects`）；`CheckLayerConsistency` 先认更正关系再判矛盾，交回更正是否在同一范围上生效。所指那份须是同一提交版本、同一层的在册事实（否则 `ErrCorrectionOutsideSubmission` / `ErrCorrectionAcrossLayers`）；被生效更正取代的事实退出比对。
- 编排：命令带 `Corrects`；所指那份未在册答未决 `CORRECTED_RESULT_NOT_RECORDED`（带续办引用、什么也不落），所指是归属不上的留存、跨版本、跨层或指向自身答未受理；生效与否随记录（`ExternalResultRecord.CorrectionInForce`）落下并交给下游。
- 摘要：带更正指向的外部结果口写 `CCC-2`（ADR-0267 决定一：在已接的册正文里加键即换号），不带的照旧 `CCC-1` 逐字节不变；`CompareStoredExternalResultDigest` 认两版。带指向的命令没有无版本摘要可比。
- 迁移 `migrations/customs_compliance/0027_external_result_correction_relation.sql`：`corrects_source_id`、`correction_in_force` 两列记在更正一侧新行上，所更正那行不动；外键钉所指在册，CHECK 守形状，部分唯一索引守只许更正链尾。
- 读口：结果登记册的按键读与同层一致性读面连同更正关系读回；放行读口（动作准入与离港门禁读的那一口）把被生效更正取代的那份照样交出，带上取代时点（`ReleaseReading.SupersededAt`，取生效后继的业务发生时间），准入按所问时点取（评审 B1 后）。
- 接入：集成客户端载荷收 `correctsSourceId`。
- 文档：CC CONTEXT「外部结果与规则时效」加一条「来源明确的层内更正不是同层冲突」；UC-CC-006 来源权威一节、`AT-CC-156` 首发注、待决表「同层冲突裁决」与结果表「外部结果冲突」改为已补、改指 CONTEXT 那一条；AT 盘点把 `AT-CC-156` 从④移到①（总览计数未动，进 main 时重算）。ADR 正文未改。
- VE：只改 `internal/visibilityexception/adapters/customscompliance/derive_on_external_result.go` 的头注，派生行为不变，理由见代决第 9 条。

### 代决

1. **只许更正链尾**（票面「要裁的」第一条）。所指那份已被另一份生效更正取代时，两份更正都声称取代它，按到达先后或业务时间择一都是替来源判断谁更正了谁，所以后到那份记同层冲突，原有更正关系不变。不取「按链取当前那一份」：那等于把指名 A 的更正改读成指名 B。
2. **更正只解释它与所指那份之间的差别。** 与其余同层在册事实（含已记冲突的，照 ADR-0226 决定二）仍照同层判法比；被生效更正取代的事实只对业务发生时间不早于那份更正的后到结果退出比对——自那一刻起来源收回了它的说法，而在它仍算数那段时间里，它照旧挡住与它矛盾的后到结果（评审第二轮 B2 后）。这是 ADR-0226 决定二「与某一份……在册事实矛盾就记冲突」在更正出现后的读法：UC-CC-006 的`外部结果冲突`本就是「无法按权威、更正或时效规则裁决」的矛盾。
3. **记了冲突的更正不生效**，不取代任何结果：冲突的一方不得改变当前判断（CONTEXT「不得……按最后到达直接改变当前判断」）。所指那份本身记了冲突时，更正照样可以生效，但它仍要与那份当初与之矛盾的事实比，矛盾就记冲突——按更正裁决冲突仍等冲突处置入口（ADR-0226 越权风险点 7）。
4. **范围单元一级退化**：新旧范围相等即整份取代；不等则交集为空，关系照记、不取代，更正在自己的范围上照同层判法独立判断（`AT-CC-156`「新扩展范围独立校验」）。
5. **所指未在册答未决、不落**，不当作不带更正指向的结果去判（那会丢掉来源的更正语义，原结果后到时反被判冲突）；与解释规则未配置、辖区解析不出那几格同一做法。所指为留存、跨版本、跨层或自身是矛盾输入，未受理。
6. **更正指向是来源内容、进内容比对**：同一来源标识换了指向、丢了指向或补上指向都是来源响应冲突。按 ADR-0267 决定一换号到 `CCC-2`，按内容取能表达它的最低一版。
7. **「已被取代」是派生问答**：从后继行的生效标记派生，被取代行一列不动（接收记录只建立不改写）。生效标记是接收那一刻的判断，与冲突标记一样不重判。迁移给 `correction_in_force` 留了默认 false：不写更正指向的行本就没有更正关系；带指向的行由适配器显式写下。
8. **按时点读**（评审 B1，协调方裁定，与 ADR-0226 决定三「取业务发生时间不晚于所问时点的最后一份」一致）：被生效更正取代的那份在所问时点早于更正的业务发生时间时仍算数，不早于时由更正那份答；更正按它自己的业务发生时间算数，不继承原结果的时间。初版「任何时点都排掉」在原结果与更正之间那段时间两份都读不到，准入答「未收到放行」，已改。
9. **VE 不跟随更正关系**（票面「要裁的」第二条，未裁成）。推荐的「沿用 VE 替代链」要求更正与原结果落在同一事实引用下、以版本链接前身（VE CONTEXT「来源事实替代关系只能由源上下文给出，且只在同一源上下文、同一事实引用的版本之间成立」）；而 [ADR-0197](../../../docs/adr/0197-customs-external-results-enter-tracking-projection-per-layer.md) 决定三定了「事实引用 `external-result/<成员>/<来源标识>`……事实版本取结果关联的提交版本……所以不登前身」，ADR-0226 决定三又写「投影把每一份未记冲突的接收各派生成本层条目」。改引用或版本取法、或让 VE 对生效更正入账不派生，都与这两条相抵，需要新 ADR 才能定；按本票约束停下，不立 ADR。今天的行为：生效的更正不再记冲突，VE 把它当一份独立条目派生，原条目仍是当前有效条目。
10. 载荷字段名取 `correctsSourceId`（与 `sourceId` 同一词干）。

### 越权风险点（待 CC owner 复核；第 7 点同时待 VE owner）

1. **只许链尾偏保守**：推送与查询各送一份内容相同、都指名原结果的更正（两个来源标识）时，后到那份记冲突、不进当前判断。
2. **未被指名的同义副本挡住更正**：原结果经推送与查询各到一份（同义、都不记冲突），更正只指名其中一份时，与另一份矛盾，更正记冲突。不从语义相同推「这两份是同一结果」。
3. **已记冲突的分叉更正挡住此后的链尾更正**：比对含已记冲突的在册事实（ADR-0226 越权风险点 6 同一边）。
4. **取代时点取更正的业务发生时间**（代决第 8 条）：真实程序的更正若另有「自何时起生效」的语义（追溯到原结果的时点，或与业务发生时间不同的适用时间，`PAR-CUS-01/02`），取代时点要改取那一格。同层一致性比对按时点排除被取代的事实，但更正那份不论时点都在比对里：一份业务时间落在原结果与更正之间、与原结果一致的后到结果，同时要与更正那份比，构成回退就判冲突（如全部 t1、收窄成部分 t3，窗口里再到一份全部 t2），偏保守；与原结果矛盾的那一面照判冲突，不再漏。
5. **所指未在册不留存**：来源不重投，这份更正就停在通道侧；与其余几格未决同一风险。
6. **单元以下范围**：真实程序给出申报行或成员范围后，要补交集与差集的真判法（ADR-0226 越权风险点 2 同一处）。
7. **CC 当前判断与 VE 运营投影分叉**（代决第 9 条；评审 N1 改准）：放行收窄更正生效之后，CC 当前判断按部分放行，VE 运营投影里原全部放行那条仍当前有效并归入 `CUSTOMS_CLEARED`，另多一条部分放行条目；核定税费与处置决定的更正同样两条并存。客户视图今天不列里程碑明细，演示映射下改前改后客户面输出相同。现状钉在 `TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor`，VE 不加守卫；跟随更正关系要新 ADR，票 [code/84](./84-ve-follows-cc-external-result-correction-relations.md)。
8. **新扩展范围不另校验来源权威**：`AT-CC-156`「新扩展范围独立校验来源权威和适用性」在单元一级退化为与其余结果同一套接收校验，更正与原结果的权威角色不互比。
9. **评审 N5：约束违反折成可重试的`未决`**。登记册写入撞上更正关系的外键、形状 CHECK 或「至多一份生效的更正」唯一索引时照错误上抛，编排一侧与库不可用同归 `ResultStoreUnavailable`，答可重试的`未决`。这些都是写入方违了不变量，重试不会自愈，按 ADR-0029 本该按恢复动作另分一类；本票编排已在落册前用领域判过同一组形状，库里这几道约束只作兜底，撞上即是缺陷，故只记不改，协调方裁定。
10. **链上业务时间不单调**：更正的业务发生时间可以早于它所更正的那份（来源本可以改正原结果自己的时间），这时原结果的取代时点早于它自己的业务发生时间，按时点读它从不算数；比对一侧，业务时间落在两者之间的后到结果不再与原结果比。不改，协调方裁定。

### 未决

- VE 跟随更正关系要新 ADR，已立票 [code/84](./84-ve-follows-cc-external-result-correction-relations.md)（R2 建议）。
- 票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md)（冲突关系）改同一组文件：它要立的冲突关系宜与本票的更正关系同一形状（记在后到一侧、只建立不改写）。
- AT 盘点总览计数与机制清点进 main 时重算，本分支未重生成。

### 被权限拦下的改动

无。

### 测试

- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh -count=1 ./internal/customscompliance/... ./internal/visibilityexception/adapters/customscompliance/ ./internal/networkrouting/adapters/customscompliance/ ./internal/parcelshipment/adapters/customscompliance/ ./internal/settlementaccounting/adapters/customscompliance/ ./internal/transportfulfillment/adapters/customscompliance/ ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-customs-register/ ./internal/architecture/... ./migrations/...`：20 包 ok，无失败。
- 新用例 `-count=3`：`scripts/test-full.sh -count=3 -run '<新用例名>' ./internal/customscompliance/... ./cmd/parcel-api/`，全 ok。
- `go build ./...`、`go vet ./...` 干净，改动的 `.go` 文件 `gofmt -l` 无输出。
- 种子未受影响（`scripts/demo-seeds` 不写外部结果），未灌一次性库。

## 评审记录

2026-10-21 隔离评审（协调方转述）：变异四个全杀，CCC-2 换号、链尾与并发、迁移都核过。修订在 `claude-code04` 上，先 `git rebase main`（main `32dc1d98d`，无冲突）。

| 条目 | 处理 |
|---|---|
| B1（阻断）放行读口不论时点排掉被更正的原结果，原结果与更正之间两份都读不到 | 已改，协调方裁定按时点读：`domain.ReleaseReading` 加 `SupersededAt`；视图改为左连生效后继（`correction_in_force`）、带出后继的业务发生时间；领域在 `SupersededAt` 非零且不晚于所问时点时跳过那份；更正不继承原结果的时间。补 `TestASupersededReleaseCountsOnlyBeforeItsCorrectionOccurred`（收窄、所问时点在两者之间→原结果答放行；收窄、不早于更正→只剩部分放行答未决；带指向的放宽、所问时点在两者之间→只有部分放行答未决；另一格取代时点恰为所问时点）。读口真库用例改为 `TestTheReleaseViewCarriesWhenACorrectionSupersededARelease`，生产装配用例改断取代时点。代决 8、越权风险点 4 改写 |
| N1 越权风险点 7 改准；VE 补现状钉；立 code/84 | 已改：风险点 7 改为 CC 当前判断与 VE 运营投影分叉、客户面输出不变；`internal/visibilityexception/adapters/customscompliance/derive_on_external_result_correction_test.go::TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor` 钉住生效更正作为独立条目派生、不带 Supersedes，VE 不加守卫；立 [code/84](./84-ve-follows-cc-external-result-correction-relations.md)（号先查过各分支未占），「要裁的」列九件 |
| N2 「今天判冲突」两处、「上一用例」 | 已改：前两处改为「不带更正指向时判同层冲突」，后一处点名 `TestCommandPayloadsRoundTripAsCCC1` |
| N3 `ReleaseOutcomeView` 端口头注与 `domain.ReleaseReading` 头注 | 已改，随 B1 写全被取代那份照交、带取代时点、按时点由领域判 |
| N4 全空白的 `Corrects` | 已改：领域判 `ErrInvalidExternalResult`；编排在归属判断之前答未受理（否则归属不上那一格会把它当留存收下）；领域与编排各补一格用例 |
| N5 | 只记不改：越权风险点 9 |

修订后测试：`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/customscompliance/... ./internal/visibilityexception/adapters/customscompliance/ ./internal/transportfulfillment/adapters/customscompliance/ ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/...` 16 包 ok；新用例 `-count=3`（含 VE 现状钉）全 ok；`go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出。迁移 0027 修订中未改，未灌种子库。

### 第二轮（2026-10-21）

B1 修法正确完整，两处变异都杀掉，merge-tree 干净。N5 原文由协调方补进越权风险点 9（`c1ad44751`）。

| 条目 | 处理 |
|---|---|
| B2（阻断）同层一致性比对不论时点排除被取代那份，O 仍算数的 [t_O, t_C) 里与 O 矛盾、不带更正指向的 X 不记冲突 | 已改：`CheckLayerConsistency` 的被取代集合带取代时点（生效后继的业务发生时间），只在新到结果的业务发生时间不早于它时跳过；只许链尾照旧只看有没有后继。补 `TestASupersededFactStillJudgesLaterArrivalsInsideItsOwnWindow`（评审两格判冲突，X 在 t_C 之后一格不判）。代决 2、越权风险点 4、`ports.ExternalResultStore` 与 `LayerFact` 头注、CONTEXT「来源明确的层内更正不是同层冲突」一条随改 |
| N1 迁移 0027 注释 | 已改两处注释（仅本分支，未进 main） |
| N2 code/84 出处 | 改用粗体标题指 |
| N3 code/84 Blocked by 与 Status | Blocked by 只留 code/04；新 ADR 的起草与接受写成票内一步、由协调方代决、ADR 号向编号分配方取；Status `ready-for-agent`，R2 |
| N4 code/84 停用范围 | 照改 |
| 读口不对称 | 已改：放行读口取后继的 `source_id` 判存在，后继在而缺业务时间整笔报错；补 `TestTheReleaseViewRefusesASuccessorWithoutAnOccurrenceTime` |
| 链上时间不单调 | 只记不改：越权风险点 10 |
| 用例头注 | 「（所更正那份退出当前判断）」改为「（自更正的业务时间起退出当前判断）」 |

第二轮测试：`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/customscompliance/... ./internal/visibilityexception/... ./internal/transportfulfillment/adapters/customscompliance/ ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/...` 30 包 ok；新用例 `-count=3` 全 ok；`go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出。迁移 0027 改了注释，在一次性库 `c04seed` 上随 `scripts/demo-seeds/seed.sh` 灌通（租户 SYN-TENANT-01 各上下文全部落库，两列在），用后已删库。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-code04` 由协调方 rebase 到 main `142338122`（`git rebase main` 退 0）后快进，tip `8805b3480` 之上的 rebase 结果；两轮隔离评审：第一轮 B1（放行读口不按时点）、第二轮 B2（同层比对不按时点排除被取代那份）均已修，B2 修法照评审给的形状落，协调方核过 diff 后未再起第三轮。
- 协调方单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通（迁移 CC 0027），用后删库。
- 去向：code/84（VE 跟随 CC 外部结果更正关系，需新 ADR，R2）。
