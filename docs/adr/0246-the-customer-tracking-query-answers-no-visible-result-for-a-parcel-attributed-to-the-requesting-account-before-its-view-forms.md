# ADR-0246：客户追踪查询对已归属本账户、视图尚未形成的包裹答「当前无可见结果」——只在读不到视图时问一次归属，其余没有视图的情形对外仍是同一格

Status: Accepted（2026-10-21，Claude Code 会话代 VE owner 决（代决）：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/04，审计条目 [audit/ve.md](../../.scratch/docs-consistency/audit/ve.md) VE-05（归 B）。回答 [ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md) 越权风险点 3，不停用任何已接受记录的条款；ADR-0223 正文不改，只在其 Status 行与 Links 加前向指针。要改的代码另立票 docs-consistency/code/27。越权风险点见文末，待 VE owner 复核，第 1 点同时待 PS owner 会签。）<!-- 索引注记 -->｜**三格**：`CURRENT_VIEW`（有当前客户视图）；`NO_VISIBLE_RESULT`（读不到视图，而 `parcel-shipment` 的当前已接受委托把这件包裹唯一归属于请求方的货主客户账户）；`VIEW_NOT_FOUND`（其余一切读不到视图的情形：包裹不存在、归属别的账户、归属不可确定含委托接受之前、多份当前已接受委托同时声明）。**判据**：只在读不到视图时问一次客户归属，问的是「这件归不归你」；请求方账户只取自客户渠道册的绑定，答出区别的只有归属方本人，对别人仍是同一格同一副响应。归属读不动答未形成，与视图读不回同一格。**范围**：「当前无可见结果」只指视图尚未形成；视图已形成而各维待确认或不展示，照旧答当前视图、逐维如实。标识冲突与接受前的包裹对外不另分格。

Date: 2026-10-21

## Context

取证于 main `205eb254`，只记 `S`。

- UC-VE-008 的结果语义有一格「当前无可见结果」：「对象已授权，但当前没有满足对客展示条件的里程碑、ETA 或终局；不表示未收寄、未运输或遗失」。主流程第 8 步也说返回「当前视图、当前无可见结果、信息待确认、无权查询、标识冲突或未形成」。
- 代码只有封闭两格。`adapters/http/query_customer_tracking_view.go` 的业务结果是 `CURRENT_VIEW` / `VIEW_NOT_FOUND`，头注写「包裹不存在、不属于请求账户、或视图尚未形成，一律这一格」。端点只读客户视图库：库按（租户，客户账户，包裹）键，读不到就答 `VIEW_NOT_FOUND`。
- 视图在两种时候形成：投影派生之后按当前归属派生；客户归属确立时按当前投影补派生（UC-VE-008 `AT-VE-169`）。包裹已接受、初始路由投影还没形成时（或 `network-routing` 一直形不成投影时），归属方查自己的包裹，拿到的答复与无权查询相同。
- ADR-0223 越权风险点 3 记下了同一件事：「接受前查视图答 `VIEW_NOT_FOUND`，与不存在不分……要分开得在 VE 的答复代数里加一格，那会给枚举开口，归 VE owner；本记录不改。」
- ADR-0076 Context 四把这一格的三义合并称作「ADR-0029 纪律与 UC-VE-008 验收」。ADR-0029 的判据是按消费方的恢复动作分格：「稍后再查」与「核对你给的号」恰是两种恢复动作。
- CONTEXT 的约束是「对未授权对象的外部响应不得泄露对象是否存在或属于其他客户」。保护的是对**未授权**对象的作答。

## Decision

**一、客户追踪查询的业务结果是三格。**（产品策略）

- `CURRENT_VIEW`：读到这件包裹在请求方账户下的当前客户视图，照旧逐维如实转写。
- `NO_VISIBLE_RESULT`（当前无可见结果）：读不到视图，而 `parcel-shipment` 的当前已接受委托把这件包裹唯一归属于请求方的货主客户账户。续办是稍后再查，不表示未收寄、未运输或遗失。
- `VIEW_NOT_FOUND`：其余一切读不到视图的情形，对外同一格同一副响应，续办是核对查询标识。含：包裹不存在；归属另一个账户；归属不可确定，包括委托接受之前；多份当前已接受委托同时声明这件包裹（标识冲突）。
- 三格都是 HTTP 200，业务判别进 `outcome`（ADR-0022）；读不回、问不动照旧不进 `outcome`（决定二）。

**二、判据与次序。**（机制）

- 先读视图。读到即答当前视图，不再问归属；热路径不多一次读。
- 读不到视图，才经 `parcel-shipment` 的客户归属反查口问一次这件包裹的归属（VE 已有的 `ports.ParcelCustomerAccountView`，与视图派生同一口）。比的是「唯一归属账户等于请求方账户」这一位，答复里不带归属于谁。
- 反查答零份、答别的账户、答多于一份（具名错误），一律 `VIEW_NOT_FOUND`。
- 反查读不动，答未形成，与视图读不回同一格（UC-VE-008 `AT-VE-168`）；不降成 `VIEW_NOT_FOUND`，也不降成 `NO_VISIBLE_RESULT`。
- 查询仍不触发派生、披露或通知。反查只是读，不补派生视图：视图迟迟不形成的原因（投影未形成、派生搁置）归派生那一侧与运营追踪查阅。

**三、为什么这不泄露存在性。**

- 请求方的货主客户账户只来自客户渠道册上的绑定（ADR-0139 决定四），请求里自报账户按坏请求拒绝（`AT-VE-171`）。能拿到 `NO_VISIBLE_RESULT` 的，只有这件包裹的当前归属方本人；他本来就有权知道这件包裹存在。
- 对任何别的账户，存在而归属他人、存在而归属未定、根本不存在，三者仍同一格同一副响应，读的次数也相同：每一次读不到视图的查询都问一次归属，不论结果。

**四、「当前无可见结果」只指视图尚未形成。**

- 视图已形成而四维全答待确认或不展示，照旧答 `CURRENT_VIEW`，逐维如实。同一场景不出两种结果。
- UC-VE-008 的「信息待确认」落在维上（逐维三态），不是查询级的一格。

**五、不另分的两种。**（产品策略）

- **委托接受之前**：CONTEXT 的客户归属由当前已接受委托给出，接受之前本上下文没有归属可核，答 `VIEW_NOT_FOUND`。客户系统手里有首方提交答复给的件号（ADR-0223），这时的续办是读 `parcel-shipment` 的提交与受理状态，不在本口。
- **标识冲突**：内部照 CONTEXT 处理——不择一、不形成视图、按待确认处理。对外答 `VIEW_NOT_FOUND`：说出「冲突」，就等于告诉请求方另有账户也声明了这件包裹，正撞「不得泄露……属于其他客户」。UC-VE-008 主流程里的「标识冲突」是本上下文内部的判断，不是对外的一格。按外部标识定位时的多候选（`AT-VE-152`）推后到票 119 的客户参考定位，不在本记录。

三类：决定一、五是**产品策略**（内置一种答法）；决定二是**机制**；客户渠道册上谁绑哪个账户、谁有查阅授予是**租户取值**（`PAR-INT-13`），演示租户以 `SYN-` 登记，证据只记 `S`。

## Consequences

- 代码票 [docs-consistency/code/27](../../.scratch/docs-consistency/code/27-customer-tracking-query-cannot-say-no-visible-result-for-its-own-parcel.md)：
  - 端点加 `NO_VISIBLE_RESULT` 一格，读不到视图时经归属反查口判。
  - 头注「一律这一格」改为本记录的三格。
  - 首方契约描述（ADR-0140）同步加这个取值。
- 首方 API 尚未发布兼容性基线（ADR-0140 Decision 五），今天加一个 `outcome` 取值不破坏任何集成方。
- 文档：UC-VE-008 结果语义「当前无可见结果」一行写明判据，主流程第 8 步与元数据随之改，另立验收行。

## Alternatives considered

- **维持合并，在 UC 里收回「当前无可见结果」对这一情形的承诺。** 否决。按 ADR-0029，「稍后再查」与「核对你给的号」是两种恢复动作，合在一格，客户系统只能对每个新号盲目重试，或把一次正常的派生延迟当成号错。合并保护的是对未授权对象的作答，这里的请求方恰是已授权的归属方，合并对他不保护任何东西。
- **归属确立时先形成一份四维全待确认的空视图，让查询照旧只读视图库。** 否决。CONTEXT「客户全程追踪视图只基于当前有效的全程追踪投影……形成」，视图版本要指名所依据的投影；没有投影也立一版视图，是为一个读需求改写披露对象的形成条件（ADR-0076 否决「给客户视图库补全景行」的同一条理由）。
- **委托接受之前也分开：按提交声明认归属。** 否决。接受之前声明的包裹不是正式包裹（PS CONTEXT），委托可能被拒绝或撤回；VE 若按提交声明认归属，就是在 CONTEXT 的客户归属之外另造一种归属。
- **标识冲突对外答一格「待确认」。** 否决，理由见决定五。

## 越权风险点

1. **查询时多一次对 `parcel-shipment` 的读**（同时待 PS owner 会签）。只在读不到视图时发生。客户系统对新号高频轮询时，这一读的量与轮询量同阶。反查口今天服务于派生，不是为查询面设计的。
2. **对归属方说出了「已归属」这一位。** 委托接受前后的差别、接受后投影形成前的那段时间，归属方都看得出来。这本是他可得的信息（`parcel-shipment` 的受理答复），但查追踪口也能得到它，算一条副通道。
3. **读次数相同不等于时延相同。** 反查答「是你的」与答「不是你的」可能快慢不同。差别与网络抖动同阶，本记录不加常数时间。
4. **`NO_VISIBLE_RESULT` 可能长期不变。** `network-routing` 一直形不成初始路由投影时，归属方会一直读到它。原因只在运营追踪查阅看得见，客户这一侧没有更多说明。
5. **首方契约加一个取值。** 基线发布前加是无痛的；基线之后再改就是破坏性变更，所以应在首个基线前落地。

## Links

- [ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md)：越权风险点 3 由本记录回答——接受之后、视图形成之前分开，接受之前维持合并
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：状态码只回答有没有形成答案
- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：决定四，客户账户只取自客户渠道册的绑定
- [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)：Decision 五，兼容性基线
- [ADR-0076](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md)：运营查阅分开作答的先例；Alternatives「给客户视图库补全景行」
- [visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)：「客户归属」词条；「客户可见性与通知」账户核对与存在性一条
- [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)：结果语义、主流程第 8 步、`AT-VE-168`、`AT-VE-169`、`AT-VE-171`
- [docs-consistency/code/27](../../.scratch/docs-consistency/code/27-customer-tracking-query-cannot-say-no-visible-result-for-its-own-parcel.md)：实现票
