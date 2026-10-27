# 51 VE 的 ETA 客户可见性判断（AT-VE-187）没有建模，客户视图的 ETA 一维只能答待确认

Category: enhancement
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code51（基 origin/main `57fc82782`）。此前 needs-triage——建议 R2（上线后尽快），分诊后定级 R2 不变。
Blocked by: 无——分诊裁定判断引用候选 ETA 的版本号，不依赖 ETA 存储的形状，不等 [docs-consistency/code/07](./07-eta-store-overwrites-history-and-mixes-sources.md)（见完成记录代决 1）；07 仍待做，落地时候选 ETA 的选取要按来源维重定（越权风险点 6）
出处：code/06 完成记录越权风险点 1；code/06 隔离评审。

## 现象

UC-VE-003 要求「ETA 客户可见性判断」：针对客户账户、服务产品、合同、预测对象与目标里程碑范围，引用候选内部 ETA 版本，保存质量评价、门槛版本、规则版本、结论（客户可见 ETA / ETA 不可见 / ETA 可见性待判断）、理由、判断时点、版本与替代关系。代码里只有内部预测 `ETAPrediction`，没有这个判断对象，也没有形成它的编排。客户视图因此无从读，`domain.ETADimensionWithoutVisibilityJudgement` 在策略准许展示时只能答待确认。

## 证据

- 文档：UC-VE-003 结果语义「ETA 客户可见性判断」一行与主流程第 6、8 步；`AT-VE-187`、`AT-VE-170`。UC-VE-008 主流程第 4 步「读取 `UC-VE-003` 的客户可见性判断」；`AT-VE-158`「只有内部 ETA 或质量未达对客门槛 → 不展示 ETA，也不使用计划或承诺时间填充」。
- 代码：`internal/visibilityexception/domain/eta_visibility_gap.go` 只有 `ETAPrediction` 与 `VisibilityGap`；`application.FormETAHandler` 只形成预测与缺口；`domain.ETADimensionWithoutVisibilityJudgement` 头注写明判断落地时改为收那份判断。
- AT 盘点：`AT-VE-158`、`AT-VE-170`、`AT-VE-187` 都在 ④。

## 三类

- 机制：判断对象、它的版本与替代关系、按（客户账户，包裹，目标里程碑）取当前判断的读口，以及客户视图 ETA 一维按判断结论作答。
- 产品策略：无判断时答待确认还是不展示（code/06 代决答待确认，待 VE owner 裁，见 code/06 完成记录）。
- 租户取值：质量门槛（`PAR-VIS-03`）、合同与客户服务规则（`PAR-COM-07`）照参数登记册，未登记答未配置，结论落「ETA 可见性待判断」。

## 规模

L：领域对象与版本表迁移、形成编排（由 ETA 新版本、门槛或规则版本变化触发）、读口、`ETADimensionWithoutVisibilityJudgement` 改为收判断、用例钉住 `AT-VE-187`、`AT-VE-170`、`AT-VE-158`。

## 最小的诚实修法

1. 建判断对象，结论封闭三格；形成编排引用候选 ETA 版本（code/07 之后的版本表）。
2. 客户视图派生读当前判断：客户可见 ETA → 照策略；ETA 不可见 → 不展示（钉住 `AT-VE-158`）；可见性待判断或读不到 → 待确认。
3. ETA 判断换版本时触发客户视图重派生（CONTEXT「ETA 新版本……必须重新派生客户视图」）。

## 要裁的

- 判断按哪几维取当前版：客户视图只有一维 ETA，而判断按目标里程碑范围形成，要裁视图取哪一个里程碑的判断（例如下一个未达里程碑）。
- 无判断时答待确认还是不展示，随 code/06 的代决由 VE owner 一并裁。

## 完成记录（2026-10-26，分支 claude-code51）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 VE owner 作出，待 VE owner 复核。

### 分诊

票面描述核实仍属实：`ETAPrediction` 之外没有判断对象，`ETADimensionWithoutVisibilityJudgement` 在策略准许展示时一律答待确认，`FormETAHandler` 在 `cmd/` 下仍没有调用方（所以今天生产上本就不形成 ETA）。唯一变了的前提是 Blocked by：code/07 仍是 ready-for-agent、没有落地。

范围裁定（最小完整切片）：判断对象与版本表、形成编排、规则册读口、客户视图 ETA 一维按判断作答、「ETA 预测新版本 → 判断 → 客户视图重派生」的派发链。不做：规则册的登记口、规则或门槛版本单独生效时的自动重判、停止展示边界的执行方、合同与服务产品两维（见越权风险点）。

### 做了什么

- 领域 `domain.ETAVisibilityJudgement`：结论封闭三格（`CustomerVisibleETA`、`ETANotVisible`、`ETAVisibilityPendingJudgement`），理由即质量评价（`QualityMetTheGate`、`QualityBelowTheGate`、`CustomerRuleNotConfigured`、`CustomerRuleInConflict`），保存候选 ETA 版本与来源、对客质量门槛版本（`PAR-VIS-03`）、合同与客户服务规则版本（`PAR-COM-07`）、判断时点、替代关系；结论为客户可见时另存获准对客表达（点值或时间范围）与停止展示边界。`AssessETAVisibility` 是纯函数，同一候选版本与同一规则依据得同一发现（`ETAVisibilityFinding` 可比较，应用层据此判重放）。结论与字段成对的不变量在构造、重建口与迁移 CHECK 三处各守一遍。
- 迁移 `0031_eta_visibility_judgement.sql`：`eta_visibility_judgement_version`（版本行只增不改写）、`eta_visibility_judgement_current`（当前标记，唯一可变处）、`eta_visibility_rule`（ETA 对客规则册）。
- 编排 `application.JudgeETAVisibilityHandler`：受理 → 取候选 ETA 当前版（没有就不判）→ 固定判断时点取适用规则（缺失与冲突落「待判断」，读不动才是未决）→ 作发现 → 重放答已有判断，否则首版或替代指回前版。只写判断，不碰内部 ETA、视图、通知。
- 客户视图：`DeriveCustomerViewDeps.ETAJudgements`；`domain.ETADimensionOver` 取代 `ETADimensionWithoutVisibilityJudgement`——策略是上限，判断为客户可见才展示（内容来处是那一版判断）、ETA 不可见不展示、待判断或无判断待确认。判断只在策略准许展示 ETA 时才读。`DeriveCustomerViewCommand.ETAJudgementChanged` 让判断变化触发的重派生绕过「同一投影版本重放」的短路；四维没变时答已有结果，不 Reaffirm、不换代。
- 派发链：`veinbox.ETAPredictionConsumer`（认 `visibility-exception.eta-prediction.formed`）、`parcelshipment.JudgeETAVisibilityOnPredictionAdapter`、`cmd/parcel-dispatch/routes_eta_visibility.go` 与 `assemble_eta_visibility.go`（ADR-0222 新特性文件；`assemble.go` 只改了既有消费者构造器 `tenantBoundCustomerViewDerive`）。锁序：投影行 → 判断 → 客户视图，写在 `ports.ETAVisibilityJudgementStore` 头注。
- 文档：CONTEXT 补「ETA 客户可见性判断」词条与三条规则句；UC-VE-003 补规则与现状说明、新立 `AT-VE-213`、`AT-VE-214`；UC-VE-008 新立 `AT-VE-215`、`AT-VE-216`、`AT-VE-217`，改写 `contentRef` 一句，`AT-VE-158` 的条件限定为判断答 ETA 不可见并改写现状注。
- 测试先行（编译失败或断言红）再实现：领域、编排（判断与视图派生）、真库（判断库、规则视图、迁移 CHECK、事务外拒跑）、消费门、适配器、派发真库（FormETA → 一拍 → 判断与视图同事务，投影版本不变）。

### 代决

1. **不等 code/07。** 判断引用候选 ETA 的版本号（唯一标识）和来源口径，不重读被引用的预测，所以不依赖 ETA 库将来是否改成版本表；07 落地后候选的选取规则要重定（风险点 6）。理由：07 是 R3，等它等于这张 R2 一直不动。
2. **视图取哪个里程碑的判断（票面「要裁的」第一条）：适用客户服务规则指名唯一一个目标里程碑，判断线键为（租户，账户，包裹），里程碑是判断的属性不是键的一维。** 候选 ETA 的里程碑不是规则指名的那一个时不判（`AT-VE-214`），不拿别的里程碑的 ETA 顶替。理由：客户视图只有一维 ETA；「下一个未达里程碑」要里程碑有序，映射目录里没有序；谁的 ETA 是客户要的那一个本就是合同与客户服务规则（`PAR-COM-07`）的内容。规则缺失或冲突时没有目标可依，候选是哪个里程碑就记哪个。
3. **无判断仍答待确认，判断待判断答待确认，判断 ETA 不可见答不展示（票面「要裁的」第二条）。** 沿 code/06 代决 4：没人作过的判断不说成作过；`AT-VE-158` 的「不展示」对的是判断已形成、答不可见那一格，没有判断或判断待判断那一格另立 `AT-VE-217`，答待确认（评审 N2：原行条件「只有内部 ETA 或质量未达对客门槛」把两格混在一起，同一场景给了两套结果）。策略是上限，只收窄不放宽（`AT-VE-216`）。
4. **质量门槛表达为「哪些可信度依据达标」的集合，空集合不接受。** 内部 ETA 的可信度依据是不透明引用（`ConfidenceReference`），没有可比大小的数值；不替登记册发明一条序。`PAR-VIS-03` 里覆盖率、误差、新鲜度这几项没有建模（风险点 5）。
5. **规则缺失与冲突是确定的业务答案，落「ETA 可见性待判断」，不是错误也不是未决；读不动规则册才是未决。** 同一账户同一时点两条规则同时适用算冲突，不任选一条；适用按判断时点选（含起不含止），未闭区间同账户至多一条由唯一索引先挡。
6. **客户视图 ETA 一维展示时的内容来处是判断版本，不再是披露策略的内容集引用。** 否则 ETA 新版本只换判断、四维不变，视图不换代，CONTEXT「ETA 新版本……必须重新派生客户视图并形成追加更正或明确替代关系」落不了地。UC-VE-008 结果语义里 `contentRef` 一句已按此改写；客户 API 响应体的字段不变，只是 ETA 一维的 `contentRef` 取值变为判断版本标识。
7. **触发方式：消费既有的 `eta-prediction.formed` 事件，在同一事务里锁投影行、判断、重派生视图；不另立「判断已形成」事件。** 沿 `DeriveCustomerViewOnAcceptanceAdapter` 的先例。账户反查零行不判断（没有归属的包裹没有判断对象），多行落未决哨兵（`AT-VE-152` 不任选），无当前投影时只判断不派生（视图在第一份投影派生时读判断）。
8. **判断变化触发的重派生用命令上的 `ETAJudgementChanged` 标志绕过「同一投影版本重放」的短路。** 替代做法（把判断版本并进视图的确认依据）要改视图库的列与幂等键，波及面大于这张票。
9. **不立 ADR。** 没有改动任何已接受 ADR 的口径（ADR-0296 未用）；ADR-0065 与 ADR-0246 只是引用。
10. **迁移号 0031；AT：`AT-VE-052`、`AT-VE-158`、`AT-VE-170`、`AT-VE-187` 由④移入①，新立 `AT-VE-213`–`217`。** 盘点总览计数未手算，由推送方重生成。
11. **演示种子不改。** 规则册没有登记口，种子里写行不过是绕过治理；演示租户的 ETA 维仍答待确认，与实际一致。

### 越权风险点

1. **规则册（`PAR-VIS-03`、`PAR-COM-07` 的租户取值）没有登记口。** 表只能写库，空册答未配置、判断落待判断。租户把 ETA 登成展示、也上了 ETA 来源，ETA 维仍读到待确认，直到有登记口。这是这张票最大的遗留，建议另立票（受控登记口，照 `departure-gate-signal-rule` 先例走 `parcel-ve-register`，再补目录读面与管理台展示）；由协调方定是否立。
2. **判断只在 ETA 预测新版本到达时形成。** 门槛或客户服务规则版本单独生效不会对现存 ETA 重判（UC-VE-003 规则句要求「规则版本生效时必须形成新判断」，今天只做到 ETA 一侧）；规则换了目标里程碑后，旧里程碑的 ETA 再来只会被判「不在目标内」，前一条判断原样留着，直到新目标里程碑的 ETA 到达。
3. **停止展示边界只是登记册给的引用，没有执行方。** 判断仍是客户可见而边界已过时，视图仍会展示。生产放开 ETA 对客之前必须补（到点失效触发重判或重派生）。
4. **合同与服务产品两维并入货主客户账户一维。** 判断键和规则册都按账户；同一账户下多份合同或多个服务产品规则不同时，同一时点两条规则同时适用，落冲突待判断（保守，不会误判可见）。
5. **达标集合只覆盖「可信度依据」一维。** `PAR-VIS-03` 登记册列的覆盖率、误差、新鲜度等对客门槛没有建模，也没有「候选 ETA 过旧」的判断。
6. **code/07 未落地。** ETA 库仍是整行覆盖、键里没有来源维，承运商与运营企业两路预测互相覆盖；判断取的是「当前那一版」，可能是后写的那个来源。07 落地后要重定：候选按来源优先还是各自判断。
7. **整条链今天在生产上空转：** `FormETAHandler` 没有生产调用方，所以 `eta-prediction.formed` 不会出现。路由登了、真库用例（FormETA → 一拍 → 判断与视图）证明接上了，接 ETA 来源的那天它就是现成的下游。
8. **客户 API 的 ETA 一维 `contentRef` 取值变为判断版本标识**（代决 6）。字段与结构不变，客户端若把它当策略内容集引用解读会读错；首方客户渠道的对外契约文档里没有这个字段的取值语义，UC-VE-008 已改写。
9. **判断与视图派生同一事务。** 视图派生未决（披露策略读不动等）会回滚本拍的判断，重投从头再来，靠判断编排对同一候选与依据的幂等收口；头端阻塞是 ADR-0066 认下的代价，同投影与补派生两路。
10. **规则册 `eta_visibility_rule` 的归属可能与 ADR-0104 决定一、ADR-0136 决定二相冲（评审 N1）。** 0031 里 VE 自建了一本规则册：按（租户，账户）加判断时点选规则，一行带 `service_rule_version`、目标里程碑、对客表达、停止展示边界和质量门槛的达标集合。分开看：质量门槛（`PAR-VIS-03`）这一部分，ADR-0104 的 Status 写了「一本册是不是某类正文，看它的键是不是那类正文的键」，按这条可论为 VE 自己的判断参数；客户服务规则（`PAR-COM-07`：`service_rule_version`、目标里程碑、表达、停止展示边界）这一部分更像 PC 客户服务规则里追踪披露一项的内容——ADR-0104 的 Context 把客户服务规则可承载客户差异的项列为「六项（追踪披露、异常响应、客户更新、通知义务、索赔期限、最低材料）」，决定一写「VE 不得为这六项建自己的规则正文册」；而这本册的键（账户 + 判断时点 → 客户服务规则版本）正是 ADR-0104 论证 VE 两册不属规则正文时拿来对照的那一形（「不是（产品/合同版本 + 时点 → 规则版本）」）的近亲。ADR-0136 决定二写「VE 不持有合同维、不翻译合同标识、不自己再解析一次」：这本册按账户加判断时点自己选了一次规则版本，合同维并入账户（风险点 4），与接受时闭包固定、VE 读回指的路不同源。是否构成违反，取决于 ETA 对客规则整本归谁，本票不替 ADR owner 裁。判断保存的 `service_rule_version` 属 PC CONTEXT 要求 VE 做的「保存具体案件、通知和索赔实际采用的规则依据」（ADR-0104 Context 引），不在争议内。
    - **协调方裁定：本票不另立 ADR。** 理由：该表无写者、规则册生产上为空，且整条链因 `FormETAHandler` 没有生产调用方而空转（风险点 7），不存在生产暴露；归属要在「规则册登记口」那张票之前裁清（下节清单第 1 项）。

### 上线前必做清单（本票未做）

1. **裁清 ETA 对客规则册归属（质量门槛归 VE、客户服务规则取自 PC，还是整本归 PC）——先于登记口。** 见越权风险点 10。
2. 规则册受控登记口与目录读面、管理台展示（照 `departure-gate-signal-rule` 先例走 `parcel-ve-register`）。判断对象（版本表）与规则册今天都没有运营读面，一并在这一项补。
3. ETA 来源接入：`FormETAHandler` 的生产调用方。
4. 停止展示边界的执行方（到点失效触发重判或重派生）。
5. 门槛或规则版本单独变化时的重判：现只在 ETA 预测新版本到达时形成判断，且账户反查零行时事件被入账丢弃，之后委托接受补派生不会对现存 ETA 重判。
6. code/07 落地后重定候选选取（按来源优先还是各自判断）。
7. 判断事务读到无投影时只判断不派生，与第一份投影并发提交会停在待确认，直到下一份投影或下一版 ETA 才自愈（窗口小）。

### 验证（分支 claude-code51）

- `go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出。
- `scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-ve-register/... ./internal/architecture/... ./migrations/... ./internal/platform/migrate/...`：22 个包，21 个 ok、`ports` 无测试文件（含枚举取值与 String 门禁、标识前缀门禁、接线与类型可达棘轮）。新用例 `-count=3` 全绿。
- 变异 12 处，均被杀、逐一从备份还原并 `cmp` 一致：重放短路不绕过、达标集合恒真、判断幂等失效、策略不再是上限、策略不准展示仍读判断、适配器漏 `ETAJudgementChanged`、判断变化而四维不变仍走 Reaffirm、锁序后移（落在写之后）、当前标记乐观锁失效、规则冲突时任选一条、待判断 CHECK 放宽、当前判断读取不带租户。评审修订轮另做 4 处，见「评审记录与修订轮」：`qualifies` 只比第一个成员、`qualifies` 只比最后一个成员、重放判据只比（结论，候选 ETA 版本）、删当前标记到版本表的外键；加上对未改的 0031 跑规则册逐元 CHECK 新用例（空白元素被放行），都由新用例杀掉。
- 迁移 0031：一次性库灌 `scripts/demo-seeds/seed.sh` 成功（三张新表与逐元检查函数在，规则册空；评审修订后重灌一次），用后已删库。
- `go run ./tools/adr-index -check` 通过；`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，分桶表无缺行）。

### 评审记录与修订轮

- 独立评审（2026-10-26）：可合入，无阻断；下列为合入前建议修的项。
- N1（规则册归属，疑与 ADR-0104 决定一、ADR-0136 决定二相冲）：协调方裁定本票不另立 ADR，如实写入越权风险点 10，先裁归属列为上线前必做清单第 1 项。
- N2（`AT-VE-158` 同一场景两套结果）：已修。`AT-VE-158` 的条件限定为判断答 ETA 不可见（质量未达对客门槛），结果不变；没有判断或判断待判断那一格拆出，新立 `AT-VE-217`（UC-VE-008），答待确认。UC-VE-008 现状注、AT 盘点分桶表与追记同步，三处代码注释的 `Covers` 指向 `AT-VE-217`。
- N3（三处存活变异）：已补用例，每处先看对变异变红。
  1. 达标集合多成员：领域 `TestAnyMemberOfTheQualifyingSetMeetsTheGate`（集合 {A, B}：B 判可见、C 判不可见）；`qualifies` 改成只比第一个成员、只比最后一个成员，各红一例。
  2. 同结论、规则换版：编排 `TestAChangedRuleBasisSupersedesTheJudgementEvenWhenTheConclusionStaysTheSame`（同候选版本、同结论，门槛版本、客户服务规则版本、对客表达、停止展示边界各变一项应换代；不可见的判断本不带表达与停止边界，这两项变化与依据没变都答已有判断）；重放判据改成只比（结论，候选 ETA 版本），新用例红六例，原有两个相关用例仍绿。
  3. 当前标记的外键：真库 `TestTheCurrentMarkerCanOnlyNameAStoredVersionOfItsOwnJudgementLine`（改指不存在的版本、另一件包裹的标记指本件的版本、没有版本行的新标记，都应被库拒）；删外键后三例红。
- N4（规则册 CHECK 漏数组元素）：已修。0031 加 IMMUTABLE 函数 `eta_qualifying_confidences_are_present` 逐元挡空白与 NULL（写法沿 `parcel_shipment` 0006 的 `declared_parcel_ids_have_no_blank`），CHECK 调它；真库 `TestTheRuleTableRefusesABlankOrNullQualifyingConfidence`，对未改的 0031 红（空格、空串、制表符、好元素带空白元素四例被放行）。0031 未进 main，直接改本文件。
- 信息项：`routes_eta_visibility.go` 里「`FormETAHandler` 尚无生产调用方」的现状陈述改为不依赖现状的说法；判断对象与规则册没有运营读面，并入上线前必做清单第 2 项；哨兵名单 `veETAVisibilityUndecidedSentinels` 删一项无用例变红，只影响失败码归类，属信息级，未补。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code51` 由修订代理 rebase 到 origin/main `57fc82782` 后快进，tip `a6125ae98`。迁移 `visibility_exception/0031_eta_visibility_judgement`（判断版本表、当前标记表、规则册）；不立 ADR（0296 未用）。AT-VE-213–217（217 为修订轮拆出）。
- 隔离评审（Sonnet）可合入，无阻断；核实了判断并入派生短路的锁序（投影行 → 判断 → 视图）、乐观锁、事务外拒跑、ADR-0222 装配合规（新增 `routes_eta_visibility.go` 与 `assemble_eta_visibility.go`）、`contentRef` 在已接受 ADR 里无定义故语义变化不动 ADR。评审自做 7 处变异被杀；存活的三条（门槛集合多成员、同结论换规则版、当前标记外键）与 N4（规则册 CHECK 漏数组元素）由 Sonnet 修订代理补用例与迁移约束，N2（AT-VE-158 同场景两套结果）拆出 AT-VE-217 一行。
- 协调方裁定评审 N1（规则册 `eta_visibility_rule` 与 ADR-0104 决定一、ADR-0136 的归属可能相冲）：本票不另立 ADR——该表无写者、规则册生产上为空，且整条链因 `FormETAHandler` 无生产调用方而空转，不存在生产暴露；归属裁定列为「上线前必做清单」第 1 项，先于登记口。
- 协调方单独提交重算的 AT 计数与重生成的机制清点（无新 ADR，索引不动）；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净；种子灌一次性库退 0，三张新表在。无 admin-web 改动。
- 去向：无新票。**这条链在生产上空转，ETA 一维对客仍答待确认，直到「上线前必做清单」（票面）逐项做完**：1. 裁清规则册归属（先于登记口）；2. 规则册受控登记口与目录读面/管理台；3. ETA 来源接入（`FormETAHandler` 的生产调用方）；4. 停止展示边界执行方；5. 门槛/规则版本单独变化的重判；6. code/07 落地后重定候选选取；7. 判断事务读到无投影时的残留竞态（窗口小、会自愈）。
