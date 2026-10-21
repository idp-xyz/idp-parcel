# ADR-0273：重投推不动、只等一次登记的格可以先入账、续办另票；ADR-0094 决定四「续办触发同笔落地」只约束接受判断链；续办由登记信封驱动，登记送不到信封时按租户登记的间隔到期重问

Status: Accepted（2026-10-21，Claude Code 会话代 PS、NR、TF、SA owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/11，出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) B3·1。无条款停用：[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 决定四的适用场景它自己的索引注记已写（「接受判断链的未决处置」），[ADR-0250](./0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md) 决定二已对路由复核照此读；本记录把这一读法写成全产品的判据，并给 ADR-0094 的索引注记加读法、给 [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 一处引文作引用更正。代码不改，不立代码票。越权风险点见文末，待 PS、NR、TF、SA owner 复核。）<!-- 索引注记 -->｜**入账**：重投改变不了结果、只等一次登记（或别的业务事实）才推得动的格，可以入账、确认消费、不烧重投预算，续办触发不要求同笔落地；前提是原因与对象在入账那一笔一起落库（原因没落库不得入账，ADR-0094 决定五护栏照用）。**要不要续办**：看补齐之后同一个已发生事实的答案会不会变——会变，就要有续办，续办落地之前那一格的缺口记在一张票上；不会变（答案绑在当时已采用的版本或已发生的事实上），入账即终局。**续办的形**：登记动作在本上下文、或有信封送到本上下文时，由登记信封驱动，不扫表；登记发生在别处、送不到信封时，可以按租户登记的重问间隔到期重问，间隔未登记答未配置，产品不内置扫描节拍。**接受判断链**：ADR-0094 决定四照旧——那一格（`等待运营登记`）没有触发不许开，不采用定时扫描

Date: 2026-10-21

## Context

- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 决定四：「**四、第三格必须与它的续办触发同笔落地，否则不许落地。**」理由：「只把回滚改成提交而不给触发，结果是把 `ABANDONED` 换成一个更安静的永久停滞」；续办方式：「不采用定时扫描重驱，理由与 ADR-0086 否决轮询那条相同」。它的索引注记写了分格判据的适用场景「接受判断链的未决处置」，决定四有没有越出小包托运，没有一处写明。
- 同一种恢复动作（等一次登记），各上下文的处置：
  - 入账、同笔不给续办触发：NR 路由复核的`未判`（[ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定二、六，续办另立票 demo-chain-gaps/154）；TF 运输准备的`未配置`（[ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)「准备结果记`未配置`、确认消费、不重投（ADR-0094 判据；X6）」，越权风险点 4「`未配置`结果不自动重备」，再准备另立票 demo-chain-gaps/36）；TF 实际发生项（[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 越权风险点 6「未配置、不计费、协议否定格、缺采购上下文都入账；补齐之后没有东西回头重跑」，后由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 以门铃补形成作答）；TF 派送（[ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md)「要求缺失也入账：所有者答「没有」，重投也不会变成「有」」，后由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 加待形成册与重问）。
  - 照 ADR-0094 做：SA、PP 的评价续办（[ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md)「先例：ADR-0094 决定四在小包托运那一侧立过「参数已登记」」），并以「没有登记时重驱只会再停一次，扫描周期也是一项租户取值」否了定时扫描。
  - ADR-0230 决定五却定了按间隔到期重问：「**五、重问节奏是租户取值：到期按租户登记的重问间隔算，产品不给默认。**」自动驱动另立票 demo-chain-gaps/152。读 ADR-0094 的人会以为这正是被否掉的定时扫描。
- ADR-0250 决定二：「**二、ADR-0094 决定四的「续办触发同笔落地」不约束路由复核。**」它只裁了路由复核，候选与反方里明写不采纳「另立一份记录把 ADR-0094 决定四定为跨上下文纪律、同时给路由复核补续办。」于是下一个上下文再立一格`未配置`时，作者面前仍是两份相反的先例。
- [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 写「ADR-0094 决定四写死「只把回滚改成提交而不给触发是被禁止的」，一格没有触发就不许开」，引号里那句 ADR-0094 里没有；它据此不开`等待资金到位`一格，结论在接受判断链之内。

## Decision

**一、ADR-0094 决定四只约束接受判断链。**（机制）

它说的第三格是小包托运接受判断任务的 `ResumePath` 一格（`等待运营登记`）：接受链上停住的那一次，此后不会再有别的触发来问，入账而没有续办，委托就永久停在`已提交`。这一格照旧——没有续办触发不许开，续办由登记信封驱动、不采用定时扫描。它不是全产品纪律；ADR-0250 对路由复核的读法是本条的一个实例。

**二、别的上下文里重投推不动、只等一次登记的格，可以先入账，续办触发不要求同笔。**（机制）

- 入格判据同 ADR-0094 决定二：重投改变不了结果，只有一次登记（或另一件业务事实到来）推得动。这样的格入账、确认消费、不烧重投预算，不让一个业务答案伪装成投递失败。
- 原因与对象在入账那一笔一起落库；原因没落库不得入账（ADR-0094 决定五、ADR-0235 同一护栏）。
- 续办触发可以晚于这一格落地，但缺口要记在一张票上；在续办落地之前，这一格不读成已了结。

**三、要不要续办，看补齐之后同一个已发生事实的答案会不会变。**（产品策略）

- 会变：答案取决于此刻的登记状态（网络配置、运输机会、派送要求、计价参数），补齐之后再问会得出别的答案。要有续办。
- 不会变：答案绑在当时已采用的版本或已发生的事实上，补齐登记也只作用于此后的新判断。同一路里两种格可以并存，要逐格判：TF 实际发生项里，按委托作答的那一版协议判的不计费不进未形成册、入账即了结；未配置、协议未生效或已终止、缺采购上下文进未形成册，采用成立或委托协议换版时补形成（ADR-0231）。入账即终局，不续办；要改它，走那一路自己的更正或人工登记。

**四、续办的形：登记送得到信封就由信封驱动，送不到才按租户登记的间隔到期重问。**（产品策略）

- 登记动作在本上下文、或已有信封送到本上下文时（ADR-0086 的「复核已完成」、ADR-0094 的「参数已登记」、ADR-0229 的「评价参数已登记」，ADR-0231 的门铃），续办由登记信封驱动，不扫表。理由同 ADR-0229：没有登记时重驱只会再停一次。
- 登记发生在别的上下文、没有信封送到本上下文时（ADR-0230 的派送要求由所有者补齐），可以按租户登记的重问间隔到期重问。间隔是租户取值，未登记答未配置；产品不内置扫描节拍（ADR-0230 候选与反方否决「产品内置按拍自动扫册，拍频由产品定。」）。到期重问的驱动多久醒一次是部署参数，不决定哪一件该问。
- 「不采用定时扫描」在本记录之下的意思是：登记送得到信封时不扫；不是一律不许按间隔重问。

## 候选与反方

- **ADR-0094 决定四定为全产品纪律：任何一格没有同笔的续办触发就不许入账。** 否决：ADR-0250 已量过这条路在路由复核上的代价——撤回`未判`改回重投，分区堵到 `ABANDONED`，丢掉这次复核还拖住后面的信封；TF 准备、发生项与派送同理。要求同笔落地等于让每个新格先等续办票，而停住的那一次在等的期间更不可见。
- **一律只认登记信封，不许按间隔重问。** 否决：派送要求由别的上下文的所有者补齐，没有信封送到 TF；只认信封，要么为这一路造一条跨上下文信封，要么让待形成册永远靠人按。前者是另一件事，可以以后做；在它之前，按租户登记的间隔重问是诚实的做法。
- **每个上下文自己裁，不立通则。** 否决：这正是审计看到的局面，两份相反的先例各有已接受记录撑腰。
- **产品内置一个默认重问间隔。** 否决：ADR-0230 决定五与候选与反方同一理由，问得多勤是租户的所有者与运营节拍的事。

## 对已接受记录的作用

- ADR-0094：无条款停用。决定一读的正是它索引注记的适用场景；它的索引注记加一条读法，指向本记录。
- ADR-0132：无条款停用。它所引的「只把回滚改成提交而不给触发是被禁止的」不是 ADR-0094 的原句，原句是决定四「第三格必须与它的续办触发同笔落地，否则不许落地。」与理由「只把回滚改成提交而不给触发，结果是把 `ABANDONED` 换成一个更安静的永久停滞」；它不开`等待资金到位`的结论在接受判断链之内，照决定一成立。它的索引注记加一条引用更正。
- ADR-0250：无条款停用。决定二是本记录决定一的一个实例；它的候选与反方不采纳的「定为跨上下文纪律」，本记录也不采纳。
- ADR-0229：无条款停用。SA、PP 的评价续办由登记信封驱动，是决定四里登记信封驱动的那一种形；它否掉的定时扫描是登记送得到信封时的扫描。
- ADR-0230：无条款停用。决定五按租户登记的间隔到期重问，是决定四里按租户登记的间隔到期重问的那一种形。
- ADR-0201、ADR-0216、ADR-0218、ADR-0235：无条款停用。各自入账的格照决定二成立，续办各有承接，见 Consequences 的对照。

## Consequences

- 已知各格对照：

  | 格 | 补齐之后答案变不变 | 续办 |
  |---|---|---|
  | NR 路由复核`未判`（ADR-0235、0250） | 变 | 票 demo-chain-gaps/154，由登记信封驱动（决定四） |
  | TF 运输准备`未配置`（ADR-0201） | 变 | 票 demo-chain-gaps/36（再准备） |
  | TF 派送要求缺失（ADR-0216、0230） | 变 | 待形成册与到期重问（ADR-0230），自动驱动票 demo-chain-gaps/152 |
  | TF 实际发生项未形成（ADR-0218） | 采用连线或协议换版到来时变 | ADR-0231 的门铃补形成 |
  | SA、PP 评价停办（ADR-0229） | 变 | 「评价参数已登记」续办 |
  | 小包托运`等待运营登记`（ADR-0094） | 变 | 「参数已登记」，同笔落地 |

- 此后新立一格`未配置`或`未判`的作者：先按决定一判它是不是接受判断链的那一格；不是，就按决定二入账，按决定三判要不要续办，按决定四选续办的形，并在记录里写明续办由哪张票承接。
- 不改 CONTEXT、UC：本记录不改任何一格的结果，只裁跨上下文的判据。
- ADR-0094、ADR-0132 的 Status 行与 Links 补指针。

## 越权风险点

1. **代各上下文 owner 定了跨上下文判据**（PS、NR、TF、SA owner）。某个上下文若要像接受判断链那样要求同笔续办，可以在自己的记录里收紧，不必推翻本记录。
2. **「答案变不变」由作者判**（各 owner）。决定三把一格归「入账即终局」，补齐之后就没有东西回头；判错了，那一格就是 ADR-0094 说的更安静的永久停滞。要求每一次归「不会变」都写明答案绑在哪一个已采用的版本或已发生的事实上。
3. **续办票落地之前停住的那几件只能靠人查**（NR、TF owner）。决定二允许先入账、续办另票；在票落地之前，补齐配置之后已入账的那几件不会自己重判，缓解只有原因与对象落库可查。
4. **按间隔重问可能问空**（TF owner）。登记送不到信封时，间隔短了是白问、长了是晚形成；这是租户登记间隔时要权衡的，产品不替它选。有了跨上下文的登记信封之后，宜改由信封驱动。

## Links

- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：决定四只约束接受判断链（本记录决定一）；决定二入格判据、决定五护栏照用
- [ADR-0250](./0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)：决定二对路由复核的同一读法
- [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md)：所引 ADR-0094 一句的引用更正
- [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md)：登记信封驱动的续办
- [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md)：决定五按租户登记的间隔到期重问
- [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md)：门铃补形成
- [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)、[ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md)、[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)、[ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)：先入账、续办另票的各格
- [ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md)：否决轮询那一条
- 票 `.scratch/docs-consistency/issues/11-unclaimed-cross-adr-contradictions.md`
