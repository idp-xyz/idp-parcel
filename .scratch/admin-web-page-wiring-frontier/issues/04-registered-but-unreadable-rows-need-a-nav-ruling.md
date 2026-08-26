# 已登记却无页可看的行——先裁导航归属，再谈接线

Category: enhancement
Status: resolved（裁决票：导航裁决已作出，见下方 2026-08-26 裁决节；产物是按该节拆出的接线子票 05–08，各自独立收口）

自[接线前沿盘点](../report.md)「两处不在页面清单上的发现」第一处。本票**不是接线票**：这些行的写入方与数据都在，成本与批 A 同级，卡的是「归到哪张页面」这个没人裁过的问题。

## 事实（演示库实测，锚 `7ce41e4` + 当前种子包）

关务种子灌进去的登记里，只有解释规则与建案要求两类被合规规则库页读到。以下**登记成功、库里有行、没有任何页面能看见**：

- `closure_obligation_catalog` 与 `closure_obligation_item`（结案义务目录与义务项）
- `gate_condition_catalog` 与 `gate_condition_finding`（放行门禁目录与门禁认定）
- `readiness_judgment`（备案判定）
- `submission_authority`（提交权威）

商业侧同类：阶段内容声明一族（`final_rule_declaration` 与正文、`cancellation_authority_declaration` 与正文、`intake_qualification_ref` 与正文、`intake_allowed_source`）在种子发布批里灌过，页面读不到。

## 为什么值得单列

这些行是**免费的接线燃料**：写入方在、数据在、租户维在（关务与商业两侧的表都带 `tenant_id`）。任何一张收下它们的页面都只差读面两件，与批 A 同级。

而它们今天看不见的原因不是机制缺件，是**页面版图与登记版图没对齐**——版图是按 `CONTEXT.md` 的所有权画的，登记是按受控入口的粒度开的，两者本来就不必一一对应，只是没人对过一次。

## 要裁什么

三选一，逐类裁（不同类结论可以不同）：

1. **并进现有页**——譬如放行门禁与结案义务并进 `customs-restrictions`（该页本就涵盖「放行门禁核对」）。
2. **新开页**——譬如备案判定与提交权威，它们与合规规则库同族但不是规则。
3. **不上页**——某些登记是流程内证据、不构成操作员要查阅的目录，如实记下理由即可，不留空页。

裁完再拆接线子票。**不要跳过这一步直接接**：先接后想会得到一批按登记表结构长出来的页面，那是让库表形状决定产品形状，与「页面向已提交的读面收敛」的方向相反（通则见 `apps/admin-web/README.md` 的列表页上列通则）。

## 裁决（MCP-2，2026-08-26）：一张新页都不开，七类全部有主

先纠一句本票自己的判断。上面写「页面版图与登记版图没对齐……只是没人对过一次」——**对过一次之后，结论是它们本来就是对齐的**。七类里每一类都能在既有页面的 `moduleInfoById[...].source` 原句或 `CONTEXT.md` 的定义段里找到归处，没有一类需要新页。不对齐的不是版图，是**读面**：页面早就认领了这些东西，只是没人去实现对应的查阅面。这个区别要紧——若照原判断去「重画版图」，会新开三四张按登记表长出来的页，而那正是本票自己警告的方向。

**关务六类，两页收完：**

| 登记 | 归处 | 依据（原句，不是我判的） |
|---|---|---|
| `gate_condition_catalog` / `gate_condition_finding` | `customs-restrictions` | 该页 `source` 原句结尾就是「**放行门禁核对**」，与 CONTEXT 116 的术语条同词。页面早认领过。 |
| `readiness_judgment` / `submission_authority` | `customs-cases` | 该页 `source` 含「申报单元……不可覆盖提交版本」；CONTEXT 32/35 把**申报就绪判断**与**提交授权**定义为提交版本的两个前件。 |
| `closure_obligation_catalog` / `closure_obligation_item` | `customs-cases` | CONTEXT 119/122「关务案件关闭核对」「关闭依据项」是案件级构件；该页就是「关务案件与申报」。 |

就绪判断与提交授权这一格有一条**接线时必须守住的形状**，现在写下来免得到时候合掉：CONTEXT 硬句 164「申报就绪判断与提交授权必须独立存在」，两者在页面上要分列两栏、各自带自己的撤销态，**不得合成一个「可提交」标记**。库上刻意分了两张表、注释写明「就绪还在、授权已撤销是真实且必须表达得出的一格」，页面把它们并成一格就把库的用心作废了。

**商业阶段内容声明一族要拆成两半，因为它们挂的对象不是同一个。** 0013 迁移的抬头写死了归属：收寄资格与终局规则归**接单规则包**（`object_kind=4`），取消授权目录归**授权规则**（`object_kind=9`）。

- 收寄资格（`intake_qualification_content` / `intake_allowed_source` / `intake_qualification_ref`）与终局规则（`final_rule_declaration` 及正文）→ **并进 `commercial-policies` 的「接单规则包」页签**，作为规则包正文的展开。该页签今天已经列 `acceptance_rule_package_rule` 的 `rules[]`，这两样是同一份规则包的其余正文面，不是第二种目录。
- 取消授权目录（`cancellation_authority_declaration` 及正文）→ **`commercial-policies` 加第六个页签「授权规则」**。这一格是本次勘察的意外发现：种子发布批里有三条 `AUTHORIZATION_RULE`，而该页的 `kind` 封闭集只有五格，**授权规则这一类商业对象在管理台上今天完全没有落脚点**——不是取消授权目录没页可去，是它的拥有对象没页可去。加页签而不是加页，判据同票 01 的那条：`/commercial-policies` 的 `kind` 分的是这一页里的页签。

**「不上页」这一档一个都没用上。** 七类全部是操作员要查阅的目录或案件构件，没有一类属「流程内证据、不构成查阅对象」。这一档留着，下次真遇到再用。

## 拆子票（按归处拆，不按登记表拆）

四张，各自独立可派，互不共享文件：

1. `customs-cases` 收三类（就绪判断、提交授权、关闭核对）——`customscompliance` 读面 + 端点 + 页面。最大的一张。
2. `customs-restrictions` 收门禁两表——同形，较小。
3. `commercial-policies` 接单规则包页签扩正文（收寄资格 + 终局规则）——**只动 `ListAcceptanceRulePackages` 与该页签**，不新增端点。
4. `commercial-policies` 加授权规则页签（含取消授权目录）——`kind` 封闭集加一格，前后端同改。

第 3、4 两张都碰 `internal/partycommercial/**` 与 `apps/admin-web/src/pages/party/**`，**要串行、不要并行派**；第 1、2 两张同理都碰 `customscompliance`。跨组（关务组 vs 商业组）可并行。
