# 75 交接、揽收、交付、外部承运追踪四路仍把集运单元当一件追踪包裹投

Category: enhancement
Status: in-progress——(a) 完工待评审与进 main，分支 claude-code75（基于 main `8d68c7bab`，见文末「完成记录」）；(b) Blocked by 80。此前 needs-triage——(a) R2（上线后尽快）：整袋交接是常态，追踪上每交接一袋就多一份不是包裹的追踪投影，违背 VE CONTEXT「集运、装载或同批关系不合并包裹身份和客户轨迹」。(b) 归后续。2026-10-21 协调方据票 demo-chain-gaps/123 隔离评审立（作者子代理代笔）。
Blocked by: (a) 无；(b) [demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md)（ADR-0263 决定四）
上下文：VE（全程追踪）。
出处：票 [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md) 完成记录越权风险点 3；其真库补派生用例里，袋经交接进段时交接那一封给袋立了一条交接条目。

## 现象

票 123 只改了移动事实那一路与补派生。其余四路按控制事实或轨迹事实的对象引用直接立追踪包裹，对象是集运单元时照旧把它当一件包裹投：

- `internal/visibilityexception/adapters/transportfulfillment/derive_on_transport_handover.go`（运输交接）
- `internal/visibilityexception/adapters/transportfulfillment/derive_on_offsite_pickup.go`（对象级场外揽收）
- `internal/visibilityexception/adapters/transportfulfillment/derive_on_effective_delivery.go`（有效交付）
- `internal/visibilityexception/adapters/transportfulfillment/derive_on_external_carrier_tracking.go`（外部承运轨迹）

证据只记 `S`（读码）。

## 拆两半

### (a) 不再把单元当追踪包裹投——R2，机制，现在就能做

- 四路在立追踪包裹之前，经票 123 的 `veports.ConsolidationMembershipView` 问一次「这一件那一刻是不是集运单元」（时点取那份事实的业务发生时间）；是单元就入账不派生，不为它立投影；读不动记未决，答复不一致记发布失败，未决名单各路自己登。
- 不在这一半把事实投到成员上：交接与交付对成员意味着控制转移，ADR-0263 决定四在单元控制规则立起来之前不为成员形成参与或控制判断，投影若先投给成员，追踪上就出现一份运输履约自己都没认的成员控制。
- 规则句补进 VE CONTEXT「追踪投影与里程碑」票 123 那一条旁边。存量袋投影不清理（同票 123 越权风险点 2）。

### (b) 把交接、交付投到成员上——Blocked by 票 80

- 票 80 定下成员随单元受控的派生规则之后，交接、交付按那条规则投到成员上；揽收与外部承运轨迹看那时的规则是否同样适用。

## 三类

(a) 机制。(b) 机制，口径随票 80。

## 既有票

- [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md)；[demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md)

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-code75，基于 main `8d68c7bab`）

(a) 已做；(b) 未做，仍 Blocked by [demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md)。

### 做了什么

- **VE 端口多一问**：`internal/visibilityexception/ports/consolidation_membership.go` 的 `ConsolidationMembershipView` 加 `IsUnitAt`，只答 `MembersAt` 答复里的 `Unit` 那一格、不认成员身份；错误契约同 `MembersAt`。VE 适配器 `internal/visibilityexception/adapters/nodeoperations/consolidation_membership.go` 两问共用一读（`read`），读的是票 123 已消费的同一口 NO `ConsolidationMembershipsAt`。NO 一个文件也没改。
- **四路认单元**：`internal/visibilityexception/adapters/transportfulfillment/consolidation_members.go` 新 `deriveUnlessConsolidationUnit`——命令上的对象在事实的业务发生时间那一刻是集运单元即入账不派生，否则照常交派生编排；失败分格与票 123 的展开共用 `membershipFailure`（读不动 `ErrConsolidationMembershipNotVisible`，答复译不回 `ErrConsolidationMembershipInconsistent`）。交接、场外揽收、有效交付、外部承运轨迹四个适配器各多一口成员关系视图（缺即构造拒），派生改走这一处。三处「不替它猜、原样当包裹引用」的旧注释改为现口径。
- **装配**：四个消费者构造从 `cmd/parcel-dispatch/assemble.go` 原样搬进新特性文件 `cmd/parcel-dispatch/assemble_control_fact_projection.go`，各接 `projectionConsolidationMembers`（票 123 的装配函数，头注改为不列调用方）。四份未决哨兵名单照 ADR-0222 决定二留在 `assemble.go` 原处，各加 `ErrConsolidationMembershipNotVisible`，`ErrConsolidationMembershipInconsistent` 写进各自「不在名单里」。未动 endpoints / main / routes；无迁移、无种子改动。
- **文档**：VE CONTEXT「追踪投影与里程碑」在票 123 那条规则后加一条（交接任一种判断、对象级场外揽收、有效交付、外部承运轨迹的对象那一刻是单元：不立投影、此刻也不投到成员上，理由引 ADR-0263 决定四；判据同移动事实展开成员；单元册读不动是未决），「入账不派生」的「事实没有可投的追踪对象」一格补这四类。UC-VE-002 立 `AT-VE-199`–`200`，`AT-VE-046` 的现状注补一句四路已补、投到成员上等票 80。AT 盘点只加两行与一条追记，总览计数未改。ADR 正文与 Status 行都没动。

### 代决（用户原话「你作为业务和系统专家自决……解决所有的问题」）

1. **单元的这几类事实入账不派生，不另留痕**：只留 inbox 那一行消费记录，不为它立已接受事实、不立投影，也不另建「待单元控制规则」的登记。依据：CONTEXT「集运、装载或同批关系不合并包裹身份和客户轨迹」，追踪对象是包裹；源事实仍在运输履约册上，(b) 落地时要回头投到成员上，从那里取得回来（见越权风险点 1）。节点作业对同一类交接也是「先认单元、入账不推进」（ADR-0214 决定二）。备选「记未决等票 80」否：等的不是会自己恢复的依赖，整封信会无限重投；备选「仍投到单元上、标成单元条目」否：追踪页多一件不是包裹的条目，还会给单元另立客户轨迹。
2. **交接三种判断一律同处理**：已拒收、待确认的单元交接同样不是哪件包裹的轨迹，不因判断不同而投到单元上。
3. **时点取派生命令的业务发生时间**：交接取 `JudgedAt`，揽收、交付取 `OccurredAt`，外部承运轨迹取源给的发生时间、不取判断过的有效时间——与进投影的业务发生时间那一格同一个，也与票 123「成员取事实发生那一刻」同口径。今天节点作业的单元册按实例存在判单元、与时点无关，时点只为日后按时段判单元时口径不漂。
4. **端口新加 `IsUnitAt`，不复用 `MembersAt`**：这几路此刻不投到成员上，`MembersAt` 会逐件读收寄、判身份链，单元里一件成员身份链判不下来就会让交接投影那一路记发布失败、收寄册读不动就会让它停在未决，而它要的答复与成员无关。判据仍只一处：节点作业同一本单元册，VE 适配器两问共用一读，失败分格共用 `membershipFailure`，同串的包裹号都按单元认。
5. **哨兵**：读不动进四路各自的未决名单；答复译不回是确定性的，不进（派发分区用例取各路名单变量本身）。
6. **装配搬家**：消费者构造搬出 `assemble.go`，以后改这四路只动特性文件；哨兵名单不搬，因 ADR-0222 决定二让既有名单留在原处（它们的头注彼此按位置互指）。`assemble.go` 因此有一段纯删除与四份名单的原地加项。
7. **删掉 `TestAConsolidationUnitLikeObjectIsUsedAsParcelReference`**：它钉的是旧口径「集运单元号原样当包裹引用、不得猜不得跳过」，与本票相反；单元号在节点作业册上不存在时仍当包裹投，由新用例 `TestAControlFactOnAParcelStillDerives` 覆盖。
8. **不立新 ADR**：本票是 CONTEXT 既有规则（不合并包裹身份与客户轨迹）在四路上的落地，不投成员的理由取 ADR-0263 决定四原文，判据取 ADR-0214 决定二、ADR-0225 决定四，读码判断不与已接受 ADR 相抵。

### 越权风险点

1. **已入账的单元事实，票 80 之后不会自己投到成员上**（VE owner，票 80）：(b) 落地时要么在单元控制规则形成成员参与的那一刻补派生（形状仿 ADR-0224），要么回扫运输履约册上对象是单元的交接、交付；本票没有留痕表，回扫靠 TF 册与 inbox。
2. **存量单元投影不清理**（VE owner，同票 123 越权风险点 2）：本票之前已把单元当包裹投下的交接、揽收、交付、外部轨迹条目照留，运营按袋号查追踪仍看得到旧条目。
3. **每封交接、揽收、交付、外部轨迹信多一次节点作业单元册读**（运维）：那一本读不动时，连纯包裹的这四路也整封未决——说不出是不是单元就不能当包裹投。这四路排在投影名次、同一封信上的主业务各路仍照跑（FanOut 收齐各路错误），整封信重投时已入账的各路按 inbox 跳过。
4. **成员拿不到单元的交接、交付、外部轨迹**（VE、TF owner）：整袋交接、按主单给轨迹的外部承运商，成员追踪上这一段在 (b) 之前是空的；单元经交接进段时移动事实已由补派生投到成员上（票 123），于是成员追踪上可能有出发、没有之前那一次交接条目。揽收与外部承运轨迹是否随票 80 的规则投到成员上，票面 (b) 已列为到时再看。
5. **包裹号与单元号同串时按单元认**（NO、VE owner，同 ADR-0214 越权风险点 1）：那件包裹自己的交接、揽收、交付、外部轨迹也不进追踪。
6. **小包托运终局那一路不在本票地盘**（PS owner）：PS 采用有效交付、采用场外揽收与面单终局判断各路仍把单元号原样当包裹反查（反查不中各自落未找到），与 VE 这边入账不派生口径不同，二者不矛盾，但单元交付在两边留下的痕迹不同。
7. **`assemble.go` 被改**（协调方）：一段连续删除与四份名单各加一行，与并行会话改同一文件的别处不重叠，进 main 时若有文本冲突按两边取并集。

### AT

- UC-VE-002 新立：`AT-VE-199`（四路对象是单元入账不派生、不投成员、只问是不是单元、时点取业务发生时间；是包裹照旧）、`AT-VE-200`（问是不是单元读不动未决、答复不一致发布失败）。
- 用例：`internal/visibilityexception/adapters/transportfulfillment/control_fact_consolidation_test.go`（交接三种判断、揽收、交付、外部轨迹六条道各跑单元、包裹、两种失败，单元那一问的替身一问成员即失败；四个构造缺视图即拒）；`internal/visibilityexception/adapters/nodeoperations/consolidation_membership_test.go::TestIsUnitAtAnswersFromTheUnitBookWithoutJudgingMembers`（收寄册一问就报错时 `IsUnitAt` 仍答得出）；真库 `cmd/parcel-dispatch/control_fact_consolidation_projection_test.go`——四路各经生产消费者构造：对象是节点作业册上的一袋，inbox 一行、袋无已接受事实无投影；对象是包裹照旧一份事实一个条目；派发分区四路各两格。票 123 的 `TestABagHandedIntoTheSegmentAfterItsDepartureBackfillsEveryMember` 加两格：同一封交接信经生产路由一拍，交接投影那一路入账、袋上没有任何条目。证据一律 `S`。
- 变异：把「是单元即入账」去掉时，适配器层 `TestAControlFactOnAConsolidationUnitIsAccountedForWithoutDeriving` 六条道全红、真库 `TestAControlFactOnABagIsAccountedForWithoutATrackingProjection` 四路全红（本地验过，未提交）。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；三笔代码提交中前两笔各自 `git archive` 到 scratchpad 单独 `go build ./...` 与 `go vet`（VE、parcel-dispatch）通过。
- `TMPDIR=… scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-ve-register ./internal/architecture/... ./migrations/...`：20 个包 ok、0 FAIL。
- 新用例另跑 `-count=3`：VE 两个适配器包与 `cmd/parcel-dispatch`（`-run 'ControlFact|Bag'`）ok。
- 未重生成 AT 计数与机制清点（`at-coverage-counts -check` 此刻如预期报计数不一致，留给进 main 时重算）；ADR 索引不受影响（没动 ADR）。
