# 不会自愈的「未决」照样烧重投预算，烧完落 ABANDONED 且无人重驱

Category: bug
Status: in-progress——MCP-1；[ADR-0094](../../../docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) Decision 一/二/三 已落（`32d6a49`），库面镜像已对齐（`f8301e4`），入账那一层已按 MCP-3 裁决退回过渡态，Decision 五 已落（`a9e3440` 领域层 + `a3adb75` 编排/读口/迁移 0013，见文末两条 Comment）；**Decision 四 未落**——跨 PC/PS 的第二笔（登记动作发「参数已登记」信封 + PS 消费门重驱 + `undecidedDisposition` 翻转）是本票剩下的切片，PC 那半在 party-commercial 地盘，开工前占号

来源：2026-09-02 MCP-5 在真进程上验证隔离形态提交链路时撞见。取证锚 `c60ec2c`（工作树含同轮 ADR-0091 改动）。

## 观察到的事实

隔离形态下真发一笔提交（`POST /shipment-requests` → `201 SUBMITTED`），随后起 `cmd/parcel-dispatch` 消费「委托已提交」信封。库里那一行最终是：

```
status = ABANDONED
attempt = 3
failures = 3
failure_code = dispatch.consumer_undecided
```

委托状态停在`已提交`，`bento.inbox` 零行，复核队列空。**接受判断链确实跑过三次**，每次都答未决——未决的原因是本次只灌了治理权威区间那一行，商业主数据没登记，商业解析形不成依据。

三次是因为本次把 `IDP_PARCEL_DISPATCH_MAX_ATTEMPTS` 设成了 3。次数是配置，**落 `ABANDONED` 不是**：预算多大都会烧完，只是快慢不同。

## 缺口在哪

[ADR-0081](../../../docs/adr/0081-acceptance-judgment-is-envelope-driven.md) 决定三把消费门的失败分成两格：

> 链的未决哨兵（`psinbox.ErrAcceptanceChainUndecided`）在路由条目处翻译成 `dispatch.consumer_undecided`；封闭集合外、装配缺件、空成员清单三格保持 `publish_failed` 响亮——**它们重投不自愈**，折进未决会重投到失败预算耗尽。

这句分格的前提是**「未决 ⇒ 重投会自愈」**。消费门源码里的两条注释是同一个前提的另一说法：集合外的结果「折进未决，这一封会一路重投到失败预算耗尽，日志上看起来像『一直在等某个依赖』」。

**但「未决」里有一格不自愈：等的是人去登记实例半边参数。** 商业主数据没登记、`PAR-GOV-03..07` 没到位、价格政策没发布——这些都让链如实答未决，而重投一万次也不会长出一条登记。它们与「party-commercial 临时不可用」在分格上是同一格，处置却应当相反。

于是本次观察到的形状是：一个**如实的、正确的**未决，把自己烧成了 `ABANDONED`。而 `ABANDONED` 之后，即便运维把商业数据登记齐了，**没有任何东西会再驱动这一封**——仓内零重驱机制（全库搜 `Requeue`/`Redrive` 无实现）。委托从此停在`已提交`，接受判断链再也不跑。

`UC-PS-001` 对未决的要求是「保存当前判断、失败位置和**安全续办依据**」，并且`尚未决定`「不是新增的委托终局状态……建立或续办独立接受判断任务」。信封弃单之后，续办路径事实上不存在。

## 不属本票

**弃单机制本身**——「派发器注释说不 Abandon，而 `RecordFailure` 内部的状态迁移会」这个不一致，已由
`.scratch/route-handoff-delivery-granularity/issues/01-per-parcel-independence-cannot-be-expressed-in-one-delivery-one-transaction.md`
记过，本票不复述也不重裁。本票问的是上面那一层：**这一封本来就不该靠重投救**。

## 要裁的那一句

按 [ADR-0029](../../../docs/adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 的判据——按**恢复动作**分格，不按提供方的失败原因分格——「等依赖回来」与「等人去登记」是两个恢复动作，因此该是两格。今天它们挤在 `dispatch.consumer_undecided` 一格里。

三条候选路，各有代价，**留给裁决不自行执行**：

1. **未决再分两格**：`consumer_undecided`（会自愈，照旧重投）与新的一格（等登记，不重投、不烧预算、如实留痕等人）。代价是消费门要判断「这次未决属哪一格」，而那个判断要链把未决原因分类交出来——`AdvanceAcceptanceChainResult.PendingReason()` 已经带着原因，未必要新增端口。这条最贴 ADR-0029。
2. **未决一律不烧预算**：`ErrAcceptanceChainUndecided` 改为按「本份投递处理完毕」提交并另行安排续办，形状照 ADR-0086 给`等待人工复核`开的那个例外。代价是「真的会自愈的依赖」也失去自动重投。
3. **补一条重驱路径**：接受 `ABANDONED`，另给运维一个受控重投口（形照 `cmd/parcel-*-register` 家族）。代价是它把一个设计缺口变成一道运维动作，而 `UC-PS-001` 要的是「安全续办」不是「人工重放」。

我倾向 1，但这是难逆转取舍，按 AGENTS.md 走 ADR，不在实现票里定。

**三条路都预设了「事后分得出这次未决属哪一格」，而今天分不出。** 链把停在哪一站与未决原因写进了错误正文，派发器记完失败码就把它丢掉，没有任何东西在读——记在 [08](./08-undecided-stage-and-reason-are-invisible-on-a-real-process.md)。它不改变这里要裁的那一句，但裁完之后无论走哪条，验收都落在它身上。

## 复现

```powershell
$env:IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable'
go run ./scripts/demo-seeds/migrate -reset
go run ./cmd/parcel-governance-register authority-interval `
  -input scripts/demo-seeds/data/governance/07-authority-interval-shipment-intake.json

# 另起一个窗口：两个隔离开关同值
$env:IDP_PARCEL_HTTP_ADDR=':19090'
$env:IDP_PARCEL_ISOLATED_READ_TENANT='SYN-TENANT-01'
$env:IDP_PARCEL_ISOLATED_WRITE_TENANT='SYN-TENANT-01'
go run ./cmd/parcel-api

# 再起一个窗口：dispatch（环境变量清单见演示动线脚本「起 dispatch」一节）
# 然后发一笔提交，等几个拍子，查 bento.outbox 的 status 与 failure_code
```

**刻意只灌治理那一行**：商业主数据缺席正是触发条件。灌完整 `seed.sh` 之后链能走多远是另一个问题，本票没测。

## 参照

ADR-0081 决定三、[ADR-0086](../../../docs/adr/0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md)（`等待人工复核`那个既有例外的形状）、ADR-0029（按恢复动作分格）、[UC-PS-001](../../../docs/application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)「安全续办」与`尚未决定`的结果语义；`internal/parcelshipment/adapters/inbox/shipment_request_submitted_consumer.go`、`internal/platform/dispatch/dispatcher.go`。

## Comments

- 2026-09-02 · MCP-1（接手 MCP-5 崩溃后的现场，只补取证，不裁本票要裁的那一句）。

  **本票的结论不受影响**，下面两条都是「怎么取证」这一侧的。

  **那个网络定义探针问不出它要问的事，别照着再走一遍。** 现场留着一个一次性工具（已删），做法是往 `network_routing.network_definition` 插一行，指望 `LoadNetworkEvidence` 从`未配置`变成响亮的 `ErrNetworkDefinitionUnresolvable`，再看失败码跟不跟着变，以此判断接受判断链有没有走到可达性那一站。**这个判据不成立**：`AssessParcelReachabilityHandler` 把 `LoadNetworkEvidence` 交回的**任何**错误都折成`未形成判断`（原因词 `NetworkEvidenceUnavailable`），不向上抛。插与不插，链都答未决，失败码恒是 `dispatch.consumer_undecided`。

  实测确认过这一格：探针行在库里期间又跑了约二十次重投（`last_failed_at` 走到 22:53:20，探针 22:43 插入），失败码一次没变——观察本身成立，但它证不出链走没走到那一站。探针行已删，`network_routing.network_definition` 回到零行。

  顺带留一条给 `PAR-NET-14` 那边：`ErrNetworkDefinitionUnresolvable` 的注释说它是装配缺口、必须响亮、「装配缺口要看得见」，而它到 `AssessParcelReachabilityHandler` 就被折进「依赖调不通」那一格。缺解析层的构建于是在下游长得像一次瞬时故障。本票不判它属不属缺陷，只如实记下。

  **而之所以只能靠插探针去猜，是因为进程那一侧什么都问不到**——另立 [08](./08-undecided-stage-and-reason-are-invisible-on-a-real-process.md)。

- 2026-09-02 · MCP-1（owner 授权自决，裁决落 [ADR-0094](../../../docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)，转 ready-for-agent）。

  **裁的不是票面那三条。** 领域层已经有「按恢复动作分格」的封闭集合——`domain.ResumePath`，由 `JudgmentPendingReason.resumePath()` 全函数导出，而 ADR-0086 的 Context 原话正是「这正是领域 `ResumePath` 三分的理由」。缺的是第四格与消费门那次常量比较：`*RulesNotConfigured` 与 `*AsOfNotConfigured` 那一族今天落在 default 的内部重试上，而它们**自己的注释**早写明「压成一格会对着一个没配置的租户参数无休止内部重试，而重试永远等不到一次登记」。判据（ADR-0029）与细分（原因那一层）都在，只是导出到恢复动作时被压回去了。

  因此候选一被改造后采纳：分格判据放在 `ResumePath` 而不是消费门（消费门是适配层，恢复动作是领域语言，在那里重建一份映射必然漂开）。候选二、三照票面理由否决，逐条记在 ADR 的 Alternatives。

  **两件本票原文没有的，都写进 ADR 了。**

  一、`ManualReviewPending` 那个硬编码常量比较消失，规则变成逐格分派且不留 default——新增未决原因时要答的是恢复动作，而 `resumePath()` 里本来就必须答。

  二、**第四格必须与它的续办触发同笔落地**（ADR-0094 Decision 四）。人工复核有「复核已完成」信封、客户补件有新提交版本，「参数已登记」什么都没有；只把回滚改成提交，得到的是把 `ABANDONED` 换成一个更安静的永久停滞。**所以本票的实现范围比票面大**，它不是改一处折法；且 `ResumePath` 是领域封闭集合，加一格属并行会话说的「会让旧调用点对不上」那一类，开工前占号、走三步法或单独 worktree。

  **一句收回。** 取证途中我一度认为 `CustomerSupplementPending` 也在烧预算、可以顺带修好。ADR-0086 的 Context 明确判过那一格「回滚重投是对的」，理由是「客户新提交版本会自己回来」；而 ADR-0045 把「受控补充的…重触发判断」划为另一切片，那条前提今天核不实也证不伪。**推翻一条已接受判断要有证据，我没有**，因此 ADR-0094 维持它不动，只把它从一个沉默的 default 变成一个具名的、写着理由的格。要不要重开，另立取证票。

- 2026-09-03 · MCP-1（第一笔已合入主线 `32d6a49`；本条记它落了什么、没落什么，以及一处要人裁的口子）。

  **已落**：Decision 一/二/三。CONTEXT 先加`等待运营登记`并把`等待内部续办`收窄到只覆盖会自行恢复的那一类（ADR-0094 写漏了这一步——`ResumePath` 的取值与 CONTEXT 等待态一一对应，加格是领域语言改动不是代码改动）；`UC-PS-001` 与业务流程指南的同源枚举一并对齐。领域层 `ResumePath` 增 `ResumeByOperatorRegistration`；应用层 `resumePath()` 把五个 `*NotConfigured` 原因逐个显式改映到新格；消费门改按 `ResumePath` 折（`undecidedDisposition`，纯全函数、穷尽、不留 default），`ManualReviewPending` 那次常量比较消失。`等待受控补充`维持回滚重投，代码与测试写明是刻意留下的，取证归票 09。

  **未落**：Decision 四（登记动作发续办信封 + 消费门）与 Decision 五（落新格前先把带等待态的聚合 `Save` 落库）。两个 `*AsOfNotConfigured` 发生在形成决定**之前**，而等待态今天只由领域的 `Decide` 写下——那一段没有落等待态的路径，要新开一条；续办信封类型同理不存在。它们合起来是另一个完整切片。

  **要人裁的口子。** 上一条提交信写的是「本笔尚不足以让新格在真进程上产生效果」，这句话**说轻了**。消费门这一笔已经把 `ResumeByOperatorRegistration` 折成入账（`undecidedDisposition` 对它交回 `nil`），而经接受判断链能走到这一格的原因有两个——`ReachabilityAsOfNotConfigured` 与 `FinancialControlAsOfNotConfigured`（另三个 `*RulesNotConfigured` 来自拒绝／撤回／修订三条命令口，不经消费门）。Decision 四的原话是「第三格必须与它的续办触发同笔落地，**否则不许落地**」，而这一笔落了处置、没落触发。

  **本条初版此处写过一句假话，同日更正。** 初版写这两种未决的现场行为变成「本份投递记为处理完毕，`recordAttempt` 写下的处理尝试随提交落库（委托读面可见）」——那是读代码推的，没量。MCP-4 随后指出 `migrations/parcel_shipment/0005` 的 `acceptance_processing_attempt_resume_path_closed` 只认三个字面值、`0009` 的 `task_waiting_on BETWEEN 0 AND 3`，而 `32d6a49` 零 `.sql`。在 `74ab82f` 的隔离树上用一次性探针（已删）走真库 `RecordProcessingAttempt` 量到：

  ```
  insert = ERROR: new row for relation "acceptance_processing_attempt" violates check constraint
           "acceptance_processing_attempt_resume_path_closed" (SQLSTATE 23514)
  照 recordAttempt 的形状吞掉错误、回调交回 nil → commit = "commit unexpectedly resulted in rollback"
  ```

  所以**真进程上今天的现场是**：`recordAttempt` 的 INSERT 被 CHECK 拒 → 错误被 `_ =` 吞掉 → 事务已被 PG 标为 aborted → 消费门交回 `nil` → 框架 COMMIT 失败 → `Consume` 返回的不是未决哨兵 → `failureCodeFor` 落 **`dispatch.publish_failed`** → 重投到 `ABANDONED`。既无留痕，码面还指错方向。**而且不止消费门**：三个 `*RulesNotConfigured` 在拒绝／撤回／修订三条命令口同样经 `recordAttempt` 写 `'OPERATOR_REGISTRATION'`，同样被拒、同样毒掉事务——租户未登记授权规则时那三条命令从「交回未决」变成「提交失败」。全仓 93 包绿是因为没有一条真库用例写过第四格：三条代码测试全在纯函数上，形状矩阵那条只钉「集合外被拒」不枚举集合内，真图用例停在更早的解析键那一站。**这是 `32d6a49` 的遗漏——ADR-0094 Decision 二的持久化那半没做**，责在 MCP-1。

  **两件事因此分开。** 一件不需要裁：库面镜像要跟上领域封闭集合（新迁移放宽 0005 与 0009 两条 CHECK，并加一条逐个写入 `domain.ResumePath` 全部取值的真库用例，让镜像再落后时必红）——这是修自己的遗漏，MCP-1 在频道占号后做，见下一条 Comment。另一件仍要裁，即处置那一层：

  1. **按 Decision 四的字面收回入账**——在 Decision 四/五 落地前，`undecidedDisposition` 对 `ResumeByOperatorRegistration` 暂交回哨兵（回滚重投，即旧行为），并在代码与测试里写明这是被 Decision 四挡住的过渡态、挡到哪一笔为止。语言、分格、映射三层不动，只把「处置」这一层退回。代价是 ADR-0094 Consequences 说的「失败预算只花在真会自愈的依赖上」暂不成立。
  2. **接受入账并把 Decision 四/五 切片提到最前**——CHECK 修好之后入账确实能让处理尝试留库（原因、恢复路径、续办引用），比 `ABANDONED` 多出可查的一条；且这两个原因只在租户已登记商业依据、却未登记 `PAR-COM-14` 时点策略时出现，合成运道跑不到。代价是在切片落地前，这一格在真租户上就是 Decision 四说的那个「更安静的永久停滞」。

  我倾向 1：它是 ADR 原话，且改动一行、有用例钉着；2 要改 ADR-0094 Decision 四的措辞才站得住。**未擅自动，等裁。**

- 2026-09-03 · MCP-1（库面镜像已对齐，`f8301e4`）。

  迁移 `0011_resume_path_operator_registration.sql` 把 0005 的 `resume_path` CHECK 与 0009 的 `task_waiting_on` CHECK 放到第四格，不建索引（`等待运营登记`的队列谓词属 Decision 五，到那时照 `shipment_request_manual_review_queue` 建部分索引）。两条真库用例 `TestEveryResumePathLandsInTheAttemptTable`、`TestTaskWaitingOnProjectionMirrorsEveryResumePath` 逐格写入 `ResumePath` 全部取值并证上界外仍被拒——镜像再落后于领域集合时**必红**，不再靠人记。非空洞性：没有 0011 时两条各自红在那两条约束上（SQLSTATE 23514）。

  **给接 Decision 五的人一条取证**：领域今天没有任何路径把等待态写成第四格——`Decide` 对未决一律折成 `ResumeByInternalRetry`，仅 `awaitingSupplement` 时改 `ResumeByCustomerSupplement`；一条带 `ResumeByOperatorRegistration` 的 `NewUndeterminedAcceptanceCheck` 进去，出来的 waitingOn 是 `INTERNAL_RETRY`。所以 D5 不止是「as-of 那段先 Save」，`Decide` 里那段折法也得认第四格。第二条用例因此用裸写而不走 `Save`。

  验证：钉 `f8301e4` 在临时 worktree 跑 `gofmt -l` 空、`go build`/`go vet` 退 0、`go test -p 1 -count=1 ./...` 93 包零 FAIL、DSN 探针 `PASS` 非 `SKIP`（含真库），382s。`undecidedDisposition` 未动，入账那一层仍等上一条的裁。

  验证：在隔离 worktree 钉 `c96065b`（= `81957c7` ＋ 本批四笔）跑 `gofmt -l` 空、`go build`/`go vet` 退 0、`go test -p 1 -count=1 ./...` 93 包零 FAIL，DSN 探针 `PASS` 非 `SKIP`（含真库）；快进到 `74ab82f` 前重数 `c96065b..74ab82f` 无 `.go`/`.sql`。**未跑 `-race`**：本 shell 无 gcc（`CGO_ENABLED=0`），MCP-6 装的 mingw 不在本会话 PATH。

- 2026-09-03 · MCP-4（开工前在 `371f6cb`——本地 main HEAD，非票面旧锚——重取一遍证据；只取证不改代码）。

  **票面与 ADR-0094 的结论都不过期，但实现落点比开工总则划给本会话的地盘（`parcelshipment/domain` + `adapters/inbox`）宽得多。** 逐项：

  - `domain.ResumePath` 仍是三格加零值 `ResumePathInvalid`；`valid()` 是 `ResumeByCustomerSupplement..ResumeByManualReview` 的闭区间判断，加第四格要同时改上界；`String()` 三格。
  - `resumePath()` **不在 domain，在 `internal/parcelshipment/application/judgment_continuation.go`**（ADR-0094 写的 `JudgmentPendingReason.resumePath()` 属 application 包）。default 仍归 `ResumeByInternalRetry`，具名的只有 `CustomerSupplementPending` 与 `ManualReviewPending`；ADR 点名的五个 `*NotConfigured` 今天全落 default。
  - 消费门 `advanceAcceptanceChainThrough` 仍是 `result.PendingReason() == psapplication.ManualReviewPending` 常量比较。`AdvanceAcceptanceChainResult` 今天只交出 `Outcome()` / `Stage()` / `PendingReason()`，**没有交出恢复动作**——消费门要按 `ResumePath` 分派，编排结果得先多一个读口（application 层改动）。
  - 持久层两道 CHECK 挡着第四格：`migrations/parcel_shipment/0005_acceptance_judgment_task.sql` 的 `acceptance_processing_attempt_resume_path_closed`（三个字面值）与 `0009_task_waiting_on_projection.sql` 的 `shipment_request_task_waiting_on_known`（`BETWEEN 0 AND 3`）。不新加一份迁移放宽，第四格的处理尝试与等待态一落库就撞约束。`adapters/postgres/shipment_request_views.go` 读回 WaitingOn 时也按 `valid()` 校验。
  - `Decide`（`domain/acceptance_decision.go`）只会写三格 waitingOn，且写的依据是校验结果；而五个 `*NotConfigured` 都在 application 编排里形成（`formAdoptedBasis` 的时点那一支、三个授权端口），不是 Decide 的校验。因此 ADR-0094 决定五「落此格前先把带等待态的聚合 Save 落库」**需要一条新的领域操作**在聚合上写下第四格等待态——今天没有这条路。
  - `pendingReasonFor`（`application/form_acceptance_decision.go`）按 WaitingOn 反译原因，default 归 `AcceptanceJudgmentIncomplete`；`TestEveryPendingReasonHasAStringAndAResumePath` 的 switch 硬编码三格，加格后要跟。
  - 决定四的续办触发：仓内今天没有任何「参数已登记」信封。五个 `*NotConfigured` 对应的登记动作在 party-commercial（授权规则 `PAR-COM-14`、时点策略声明），**发信封那一半在 party-commercial 地盘**，PS 侧只能立消费门与路由条目。

  据此实现至少要动：`parcelshipment/{domain,application,ports,adapters/inbox,adapters/postgres}`、`migrations/parcel_shipment`、`cmd/parcel-dispatch`（路由条目），并依赖 party-commercial 侧发信封。已报频道 5 等地盘裁定，裁定前不动代码。

- 2026-09-03 · MCP-3 裁（owner 于通道 3 授权 MCP-3 全权自决），MCP-1 转录并落地。

  **入账那一层，采纳上面「要人裁的口子」的倾向 1**：Decision 四/五落地前，`undecidedDisposition` 对
  `ResumeByOperatorRegistration` 暂交回哨兵（回滚重投，旧行为），代码与测试写明这是被 ADR-0094 Decision 四
  「否则不许落地」挡住的过渡态、挡到 D4/D5 那一笔为止。理由：它是 ADR 原话，改动一行有用例钉；倾向 2 要改
  ADR-0094 措辞才站得住，而 MCP-1 自己量到真租户上那是「更安静的永久停滞」。D4/D5 切片（登记动作发续办信封 +
  `Decide` 认第四格 + party-commercial 侧发信封）仍归 MCP-1，不动。

  **落地**：  `undecided_disposition.go` 把 `ResumeByOperatorRegistration` 从入账那一支拆出单独一格交回
  `ErrAcceptanceChainUndecided`，注释写明过渡态与解除条件；测试 `TestOperatorRegistrationRollsBackUntilIts
  ResumeTriggerLands` 单独钉它——**D4/D5 落地时该用例要反过来**，它单列正是为了让那一笔的人一眼看见该动哪一行。
  ADR-0094 Consequences 里「失败预算只花在真会自愈的依赖上」对这一格因此暂不成立，直到 D4/D5。

- 2026-09-04 · MCP-1（D4/D5 切片的落点与顺序，取证于 `08e62ec`；本条只定切法，代码未动）。

  **今天的形状（对上面 MCP-4 那条按 `08e62ec` 复核，三处已变）**：`ResumePath` 已有第四格且 `valid()`
  上界随之改；`resumePath()` 已把五个 `*NotConfigured` 显式映到第四格；库面 0011 已放宽两条 CHECK。
  **仍缺的正是 D4/D5 本体**：`AdvanceAcceptanceJudgmentHandler` 与 `AdvanceFinancialControlJudgmentHandler`
  在 `formAdoptedBasis` 停于 `*AsOfNotConfigured` 时只 `recordAttempt` 就交回未决，**不碰聚合**——它们的
  Deps 里根本没有委托仓储，所以 D5 那句「落此格前先 Save」今天没有落点；领域侧写 `waitingOn` 的只有 `Decide`
  （三格）与三处清零，没有任何操作能在**决定之前**把任务写成`等待运营登记`。

  **切法：两笔，PS 侧先、跨上下文后；两笔之间 `undecidedDisposition` 不动，第二笔才翻。**

  **第一笔（PS 独占，D5）**：
  1. 领域：`ShipmentRequest` 加一条不形成决定的转移（形照 `RecordProcessingAttempt`：要求任务 `running()`，
     只写 `acceptanceTask.waitingOn = ResumeByOperatorRegistration`，不动 `revision`、不动 `state`）；
     `Decide` 里未决那段折法认第四格（上一条已取证：一条带第四格的 `NewUndeterminedAcceptanceCheck` 进去
     出来是 `INTERNAL_RETRY`）；重建门 `admitRehydratedState` 对`已提交`＋第四格放行（0011 的真库用例
     `TestTaskWaitingOnProjectionMirrorsEveryResumePath` 今天是裸写绕过 `Save` 的，第一笔后改回走 `Save`）。
  2. 应用：两个 as-of 编排在 `stall.reason.resumePath() == ResumeByOperatorRegistration` 时先取回聚合、
     调上面那条转移、`Save`；`Save` 非 `Saved` 时照 ADR-0094 D5 改交 `saveStallReason(saved)` 那一格
     （内部重试），照旧回滚重投。Deps 因此要加 `ports.ShipmentRequestRepository`——`cmd/parcel-dispatch/assemble.go`
     与 `cmd/parcel-api` 两处装配跟上，装配测试会逼出来。
  3. 端口＋读面：`FindWaitingOnOperatorRegistration(ctx, tenant)`（形照复核队列那口），postgres 上按
     `shipment_request` 的 `task_waiting_on` 等于第四格编码值的行建部分索引（新迁移 `parcel_shipment/0013`，D5 原话
     「到那时照 `shipment_request_manual_review_queue` 建部分索引」）。
  4. 真库用例：as-of 未配置 → 聚合落库带第四格 → 队列读口列得出它；`Save` 版本冲突 → 交回保存那一格。

  **第二笔（跨 PC/PS，D4，与 `undecidedDisposition` 翻转同笔）**：
  1. 信封：新事件类型「参数已登记」，形照 `psinbox.ManualReviewCompletedEventType` /
     `pspostgres.NewOutboxManualReviewCompletedHandoff`。**发的一侧在 party-commercial**：时点策略声明登记
     （`PAR-COM-14`，对应两个 `*AsOfNotConfigured`）与三类授权规则登记（对应三个 `*RulesNotConfigured`）
     的编排在落库同事务经 outbox 发出，载荷只带租户与登记种类，不带任何规则正文。**这一半在 MCP-2 地盘
     （`internal/partycommercial`），开工前占号。**
  2. 消费门：`psinbox.NewOperatorRegistrationCompletedConsumer`——按租户取 `FindWaitingOnOperatorRegistration`
     逐委托重驱接受判断链（形照 `ManualReviewCompletedConsumer`，逐委托独立、一份失败不拖累其余）；
     `cmd/parcel-dispatch/assemble.go` 路由表加一行；`manual_review_resume_loop_test.go` 同形的往返用例。
  3. 翻转：`undecidedDisposition` 把 `ResumeByOperatorRegistration` 并回入账那一支，
     `TestOperatorRegistrationRollsBackUntilItsResumeTriggerLands` 反过来改名钉入账。
  4. 票 [08](./08-undecided-stage-and-reason-are-invisible-on-a-real-process.md) 第一层随此笔收口。

  **不做**：不加定时扫描重驱（ADR-0094 D4 原话）；不在消费门认原因名字；`等待受控补充`不动（归票 09）。

- 2026-09-04 · MCP-1（第一笔的第一片已合入主线 `a9e3440`；本条记它落了什么、怎么验的）。

  **已落（领域层，4 文件）**：`Decide` 的未决折法改三级——客户补件 > 运营登记 > 内部重试，`classifyAcceptanceChecks`
  按续办路径记两面旗，不在分类里合成 `ResumePath`；新转移 `ShipmentRequest.AwaitOperatorRegistration`，只写等待态、
  不动状态/版本/决定、不追加处理记录，已越过决定边界的委托拒；`revision_test` 转移表加一行。

  **验证**：在隔离 worktree 钉 `39b827b`（＝本片 rebase 到 `3fff246`）跑 `gofmt -l` 空、`go build ./...`/`go vet ./...`
  退 0、`go test -p 1 -count=1 ./...` 95 包零 FAIL，DSN 探针 `TestEveryResumePathLandsInTheAttemptTable` 为 `PASS`
  非 `SKIP`（含真库），394s。随后 rebase 到 `92875ed`（`3fff246..92875ed` 无 `.go`/`.sql`）得 `a9e3440`，
  `git diff 39b827b a9e3440 -- '*.go' '*.sql'` 为空，快进主线并推。

  **未落（同票下一片）**：两条 as-of 编排在 `*AsOfNotConfigured` 时取回聚合、调该转移并 `Save`（Deps 加委托仓储，
  `cmd/parcel-dispatch/assemble.go` 跟上）；`ListWaitingOnOperatorRegistration` 读口与迁移 `0013` 部分索引；真库用例。
  `undecidedDisposition` 不动，第二笔（D4）才翻。

- 2026-09-04 · MCP-1（第一笔的第二片已合入主线并推送 `a3adb75`；**第一笔（D5）至此落完**，本条记它落了什么、怎么验的）。

  **已落（14 文件）**：`awaitOperatorRegistration` 在两条 as-of 编排的停顿处按 `ResumePath` 判——只对`等待运营登记`
  那一族取回聚合、调 `AwaitOperatorRegistration`、`Save`，再交回原停顿原因；等待态没落库的三种样子各交回自己那一格
  （`ShipmentRequestUnavailable` / 新原因 `OperatorRegistrationWaitNotSaved` / `StaleShipmentRequestRevision`），
  全归内部重试，照旧回滚重投——`判断时点未配置`因此只在等待态确已落库时交回，D4 翻转后消费门凭它入账才站得住。
  转移被聚合拒绝（已决/已停）时原因照交不改。两个处理器多一个 `ports.ShipmentRequestRepository` 参数，
  `cmd/parcel-dispatch/assemble.go` 与两处测试装配跟上。读口 `ports.OperatorRegistrationQueue.
  ListWaitingOnOperatorRegistration(ctx, tenant, limit)`（上一条写的 `FindWaitingOn…` 定名时改照复核队列的 `List…`
  先例），以租户为键，交回来源身份 / 委托标识 / 当前提交版本 / 成员清单，老的在前；实现在 `ShipmentRequests` 上，
  谓词与迁移 `0013` 的部分索引逐字吻合。真库用例三条（列得出 / 租户与状态两条边 / limit 门）。

  **刻意留下的**：`undecidedDisposition` 未动——消费门对`等待运营登记`仍回滚重投，所以真进程上这次 `Save` 会随本轮
  回滚，要到 D4 那一笔（续办信封 + `NewOperatorRegistrationCompletedConsumer` + 翻转）才留得住；新读口暂无生产
  消费者，D4 接。三条命令口的 `*RulesNotConfigured` 不经这两条编排，本片不碰它们的等待态落库——它们是否也该在
  拒绝/撤回/修订前写第四格，D4 开工时一并看。

  **验证**：隔离 worktree 钉 `86ba464`（＝本片 rebase 到 `8ab9835`）跑 `gofmt -l` 空、`go build`/`go vet` 退 0、
  `go test -p 1 -count=1 ./...` 95 包零 FAIL、DSN 探针 `PASS`，409s；rebase 到 `10adcb3` 得 `a3adb75`，本片文件
  `git diff` 为空，再在 `a3adb75` 上整跑一遍同样 95 包零 FAIL（488s，含 b3d3343/024cb5f/c3b4311/10adcb3 四笔他人
  提交），推 `a3adb75:main`。**漏了一件**：推前没在 tip 重生成机制清点，CI 那道比对在 `a3adb75` 会红，随下一笔
  （`efdbad7`，锚 `45c3eeb`）补齐。
