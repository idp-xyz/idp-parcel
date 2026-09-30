# ADR-0156：可达性资格的闭包标识从本轮已采用的商业解析回指

Status: Accepted（2026-09-30。本项是机制：AGENTS 红线「机制与产品策略开发方现在就做」。通道 2 按 ADR-0064 的同一形补可达性这一条，不造解析号，不填租户截点。用户在本通道对「下一刀做 psb/16 第 1 项」的答复是检查队列，不是本记录的接受依据。）
Date: 2026-09-30

## Context

接受判断链在时点能形成之后停在 `REACHABILITY_JUDGMENT_NOT_FORMED`。装配把 `ReachabilityClosureIdentity` 留空，资格视图答未配置，判断就不形成。

ADR-0064 已经否决过同一种空：判断键上没有解析标识，不等于任何地方都取不到。那份记录只改了初始路由。可达性发生在接受决定形成之前，不能去读 `AcceptanceDecision().Basis()`。本轮的权威在更早一处：`formAdoptedBasis` 在询问可达性之前已经把唯一解出的商业解析记下。那次解析的标识就是闭包标识。

留着「范围 → 解析」映射，或从判断键另造一个解析号，都是第二套解析。ADR-0064 对初始路由否决的就是这两条。

## Decision

**一、闭包标识是本轮已采用的商业解析，作为命令附加字段，不是判断维。**

路径固定为：parcel-shipment 的接受判断编排把 `adopted.snapshot` 的解析标识放进可达性请求 → 消费方适配器译成 network-routing 的 `CommercialResolutionReference` → `AssessParcelReachabilityCommand` 携带 → `CommercialEligibilityView.AssessNetworkEligibility(key, resolution)` 与判断键并列传入 → 按租户与标识 `LoadResolution` → 既有形态翻译表（ADR-0050）不动。

不把解析标识塞进 `ReachabilityJudgmentKey`，不从判断键发明闭包键，不建判断键到解析的登记表，不代拟标识。

**二、空引用、闭包读不回，是依赖不可用。** 引用为零、或按它取不到闭包，答服务产品不可观察，编排形成`未形成判断`。不把缺席读成「不要求判断」。

**三、租户不一致不上抛成未配置。** 按调用方租户取回的闭包，解析键却指着另一个租户，与初始路由同一哨兵：这是写坏的闭包，不是等一份映射。

**四、本记录不解除后面的实例闸门。** 不默认网络证据，不填财务控制的接受时点、账户目录与控制金额，不造演示网络。资格一旦要求判断，证据与关务仍按各自的未配置作答。

**五、换解析不是同一次请求的重放。** 解析标识不进 `SameJudgmentScope`，判断键不因此拆开。请求关联把标识带在委托、提交版本与包裹之后。同一解析再来，命中原关联，交回原判断。换一个解析是另一次请求，提供方另存一份，不把旧判断交回当这一轮的答案。

新商业版本要成为接受用的另一份判断，靠的是提交版本或判断时点，不是把解析塞进判断键。提交版本在判断键里，也在消费方判断账的主键里：新版本即使时点相同也是自己的一行。同一版本上时点更晚的重判参与决定，旧时点那一份留在库里。同一版本、同一时点上后到的另一份标识，消费方仍按重放保留先到者——那是迁移 `0018` 的主键，本记录不改它。0064 能把「已接受解析固定不变」当理由，是因为初始路由发生在接受之后；可达性发生在接受之前，每轮 `formAdoptedBasis` 会重解，所以关联必须带上这一轮的标识，不能只靠判断键。

## Consequences

- `NewCommercialEligibility` 只持提供方只读口，与 `NewRoutingApplicability` 同形。生产装配不再传入 nil 身份映射。
- 停用 ADR-0064 后果里「可达性那条链的 `ReachabilityClosureIdentity` 不变」这一句。该记录的决定一至四，以及初始路由那条回指，不变。
- 同判断键、同解析的重放仍交回原判断。换解析不命中那条重放：关联不同。解析标识仍不参与 `SameJudgmentScope`。

## Alternatives considered

- **继续装配 nil 身份映射。** 否决：机制缺口。本轮解析已经记下，留空只是假装还要一份试点映射。
- **按判断键查一张范围到解析的登记表。** 否决：第二套解析。权威已经在本轮采用记录上。
- **等接受决定形成后再回指。** 否决：可达性是接受决定的输入，那时决定还不存在。

## 裁决能力边界

读过 ADR-0064 全文、UC-NR-002 里「新商业版本形成新判断版本」那一句、`SameJudgmentScope`、`formAdoptedBasis` 与消费方判断账主键（迁移 `0018`）。没读：计价侧、结算账户登记册、面单择优链。

## 越权风险点

1. 请求关联带上解析标识，判断键仍不带。归 NR、PS owner 复核：重校必须回指形成时记下的那份关联，不能按不含解析的派生另查一次。
2. 同一版本、同一时点上消费方仍只留先到者。归 PS owner 复核：这不是把新解析记成第二份接受判断。

## Links

- [ADR-0064：初始路由适用性闭包标识从已接受解析标识回指](./0064-initial-route-applicability-closure-from-accepted-resolution.md)：同一形；本记录只停用其后果中可达性那一句
- [ADR-0050：服务产品形态随解析闭包可观察](./0050-service-product-form-observable-through-the-resolution-closure.md)：翻译表不动
- [ADR-0027：跨上下文多步协议的中间状态由提供方按解析标识保留](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)
