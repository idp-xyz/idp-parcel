# 不会自愈的「未决」照样烧重投预算，烧完落 ABANDONED 且无人重驱

Category: bug
Status: ready-for-human（要先裁一句：`未决` 里那一格「等的是人去登记」算不算 `未决`）

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
