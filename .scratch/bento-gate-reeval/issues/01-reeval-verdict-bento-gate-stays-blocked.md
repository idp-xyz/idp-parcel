# 重估结论:Bento 持久化闸门维持阻断——缺的仍是「适用 `PBC-*` 消费者证明」,缺口首次有了精确形状

Category: enhancement
Status: needs-triage

本票是死会话分支 `bento-gate-reeval` 取证现场的蒸馏产物。该现场由 `.scratch/dead-session-salvage/issues/02` 封存(提交 `2399ecd`,父提交 `49a2ab0` 已在 main 祖先),用户 2026-08-20 批复「先蒸馏后定」。封存提交自注「内容未逐行读」;本票是第一次逐行读的结论。五个原件(两个取证脚本、三份输出,合计约 147KB)**不进 main**,一律以分支 SHA `2399ecd` 为索引调取,如 `git show 2399ecd:.scratch/bento-gate-reeval/audit-pbc08-output.txt`。

## 证据索引(全部只读自 `2399ecd`)

| 文件 | 内容 |
|---|---|
| `audit-pbc08.ps1` | 审计脚本:对 `internal/` 下每个持久化写方法(直接调 `RequireExecutor`,或经 `outboxintent.EnqueueOnce` 写出)在同包测试里找「引用 `ErrTransactionRequired` 且调用该方法」的证据块,逐行输出 EVIDENCE/MISSING |
| `audit-pbc08-output.txt` | 上述审计的输出:155 个写方法逐行判定 + 汇总 |
| `dump-evidence-blocks.ps1` / `evidence-blocks.txt` | 全部含 `ErrTransactionRequired` 的测试块清单(测试名、夹具构造器、方法调用),供人工归属 |
| `fulltest-with-pg.txt` | 真实 PostgreSQL 上的全仓 `go test` 输出,全绿 |

取证代码基线:`49a2ab0`(死会话工作区仅有未跟踪的 `.scratch/bento-gate-reeval/`,代码即该提交;来源见 salvage 票 02)。本票复核时点:`b9f6cba`。

两点先说清,免得全绿被误读:

- `fulltest-with-pg.txt` 里各 `adapters/postgres` 包耗时 2–30 秒、`tests/bentocontract` 2.5 秒,证明 `IDP_PARCEL_POSTGRES_DSN` 已设、PG 用例实跑——`workflow.md` 点名的「本地全绿可能是跳过造成」的陷阱在这份证据里**不成立**,这是它的价值。
- 但简报「实现准入检查」明文「文档完成、测试通过或候选文件存在不能自动解锁」;全绿只说明实现与合同测试在真实 PG 16 上可跑,不是闸门证据。

## 重估结论:维持阻断

闸门四项通过条件(ADR-0009、不可变候选、空缓存下载、适用 `PBC-*` 消费者证明)中,前三项简报闸门表已记录在案(asOf 2026-08-11),本批取证不触及;**第四项仍缺**。九项 PBC 现状:

- `PBC-01`、`PBC-06`:已通过(简报闸门表口径,非本批证据)。
- `PBC-08`:静态半边(testkit、`pgx.Tx` 导入门禁)是 `internal/architecture` 常驻测试,全测含它;行为面——简报明文「每新增一个持久化适配器都要自带这条证明」——存在**真实缺口**,精确形状见下节。
- `PBC-02/03/04/05/07/09`:**零证据**。`RunRepositoryContract` 在代码里零调用(ADR-0031 的断言,本票于 `b9f6cba` 复核仍成立);`tests/bentocontract/` 至今只有 `candidate*`(PBC-01)与 `outbox_inbox_test.go`(PBC-06)。各适配器自己的回滚/事务模板测试(如 `TestJudgmentRollbackLeavesNothingBehind`、各 `*FollowsTheTransactionalTemplate`)是将来取证 PBC-03 的原料,但没有任何一项已按候选登记为 PBC 证明。

按 ADR-0026:实现存在不等于通过;九项对同一候选(`v0.1.0-rc.2` + checksum)全部通过前,不得登记 `B-06` 候选基线、不得宣称闸门已通过、不得改写开发主线横切缺口状态。**本次重估没有发现任何使闸门可解除的新事实,结论维持 ADR-0017/0026 的「保持阻断」。**

## PBC-08 行为面缺口的精确形状

审计脚本口径:155 个写方法中 84 个 MISSING;133 个适配器类型中 68 个全无证据。**该数字显著高估。** 脚本自注「Approximation notes (verified by sampling, see report)」,但那份抽样核实报告死会话没写出来——本票用 `evidence-blocks.txt` 交叉归属,把 84 行拆成三类:

### 一、真缺 30 方法 / 22 类型(包内不存在任何可能调用该方法的 `ErrTransactionRequired` 测试块)

- **customscompliance,12 方法 / 8 类型**:`case_restriction_gate.go` 的 CustomsCases.Save、Restrictions.Save/Update、GateVerifications.Save;`declaration_submission.go` 的 Save;`disposition_verification.go` 的 Save;`follow_up_manifest_closure.go` 的 CaseClosures.Save、FollowUps.SaveRelation/SaveTarget/UpdateRelation、Manifests.Save/Update。包内聚合类负向块只有 `external_result_test.go` 一个,其夹具只建 ExternalResults。
- **visibilityexception,12 方法 / 9 类型**:AcceptedFacts.Save;Claims.Save、Recoveries.Save/AppendAction;DispositionRequests.Save/SaveSupersession;ETAs.Save、VisibilityGaps.Save、CustomerNotifications.Save;Projections.Save;SignalEpisodes.SaveHit/SaveRaised。包内聚合类负向块只有 `customer_view_test.go`;`claim_recovery_test.go` 在基线后虽有改动,至 `b9f6cba` 仍无 `ErrTransactionRequired`。
- **parcelpricing,1 方法**:Evaluations.Save。包内只有 handoff 负向块。
- **settlementaccounting 特名方法,5 方法 / 4 类型**:AllocationRuleApplicability.Register;ChargeConfirmationConditions.RecordBasis/RegisterCondition;OperationalBalances.RecordBalance;CreditStandings.RecordStanding。`evidence-blocks.txt` 全部块的调用清单里没有这些方法名。

### 二、待归属 53 行(块存在、调用同名方法,脚本认不出)

归属失败的机制:脚本要求测试块含 `new<TypeName>` 夹具或方法名包内唯一,而通行式样是**捆绑夹具**(`newJudgmentStores`、`newRouteStores`、`newCollaborationStores`、settlementaccounting 七个 `new*Stores`……)加非唯一的 `Save`/`Replace`。抽验已实证此类为假 MISSING:`adoption_cancellation_final_test.go` 的 `TestJudgmentWritesRefuseToRunOutsideATransaction` 对被标缺的 IntakeAdoptions、ParcelCancellations、FinalOutcomes 三个仓储逐一断言 `ErrTransactionRequired`(于 `b9f6cba` 复核);`consolidation_test.go` 同一块内 Save 与 Update 并存,只有包内唯一的 Update 被认出。networkrouting、nodeoperations、pilotgovernance、transportfulfillment 的其余 MISSING 行均有名称对得上的捆绑块,**但逐行定案未完成**——这正是死会话没写出的那份报告。

### 三、`platform/outboxintent.EnqueueOnce` 1 行

共享包自身无测试文件,但每个 Outbox handoff 负向块的 `ErrTransactionRequired` 都取道它,间接覆盖充分;要不要求自包证明属口径问题,随行动 2 一并裁。

另一面值得记录:**约 40 个 Outbox handoff 适配器全部有证据,无一缺口**——缺口整体落在聚合仓储一侧,且集中在后期成批长出的上下文。

## 差什么(解除路径,按依赖序)

1. **补真缺**:上列 30 方法的负向证据。简报明文该性质「随适配器逐个成立,没有静态门禁能替它把关」。
2. **定待归属**:按 `evidence-blocks.txt` 完成 53 行人工归属,把「84 缺」修成真实清单;顺手可改进 `audit-pbc08.ps1` 的归属逻辑(识别捆绑夹具),让它变成可复跑的门禁而不是一次性脚本。
3. **六项零证据 PBC**:`PBC-02/03` 先复核简报记录的建模阻塞(聚合版本字段、端口签名;ADR-0030/0031 已动过这一带)现状,再按简报「下一步」的取证序推进 `PBC-04/07`、`PBC-05`、`PBC-09`。
4. 九项对同一候选全过后,按完成门禁登记 `B-06`。**在那之前闸门结论一律照旧:保持阻断。**

## 未决(留用户 / MCP-1)

- **分支 `bento-gate-reeval` 去留**:蒸馏已完成,结论由本票承载,原件以 `2399ecd` 随时可调;删分支引用等用户单独确认(与 salvage 票 02 对 `b72d96e` 的口径一致)。
- 行动 1–3 是否开票排期。

## Comments

- 2026-08-20 MCP-4:按 MCP-1 派单(BENTO-GATE-DISTILL)只读蒸馏,未 checkout / cherry-pick 该分支,大输出未进 main;基线链核验:`2399ecd` 父即 `49a2ab0` 且为 main 祖先;`49a2ab0..b9f6cba` 间 internal 测试变动九个文件,均不新增 `ErrTransactionRequired` 覆盖,审计结论在当前树依然成立。
