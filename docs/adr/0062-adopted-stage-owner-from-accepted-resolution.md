# ADR-0062: 采用规则版本从已接受解析标识回指提供方持有的闭包

Status: Accepted
Date: 2026-08-18

## Context

收寄采用停在 `ELIGIBILITY_UNCONFIGURED`，一半是因为 `AdoptedStageOwner` 还不会从已接受快照回指闭包。这是机制缺口，不是缺表。

[ADR-0058](./0058-stage-content-owned-by-rule-objects.md) 第三条写「消费侧装配不得从 `SourceIdentity` 发明采用版本」——身份键不含规则对象，这条仍对。但它没写另一条已经存在的回指：已接受委托的 `Decision.Basis.ResolutionID()` 正是 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 留给消费方的那份引用，提供方按标识持有闭包。把「身份上没有」读成「任何地方都取不到」，装配就只能永远装 `UnconfiguredAdoptedStageOwner`，而闭包里的 `ACCEPTANCE_RULE_PACKAGE` 已经在接受时固定过了。

[ADR-0061](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md) 已把已接受重建门打开，按来源身份取回委托能拿到决定。缺的是消费侧把解析标识译回提供方闭包、再取出采用版本这一段。

拆 `RulePackage` 字符串当版本键是有损的：对象号与版本号都可以含 `/`，没有无损分节。`NewCommercialDraft` 在消费侧造一份 owner 等于替租户发明采用版本。两者都不是本记录允许的路。

## Decision

**一、采用版本的权威来源是已接受委托上的解析标识，经提供方闭包取出。**

路径固定为：`SourceIdentity` → 取回已接受委托（ADR-0061）→ `AcceptanceDecision().Basis().ResolutionID()` → 译成提供方 `ResolutionID` 与租户 → `CommercialResolutionView.LoadResolution` → `Closure.AdoptedFor(ACCEPTANCE_RULE_PACKAGE).Version()`。

不从 `SourceIdentity` 发明，不拆快照上的 `RulePackage` 字符串，不在消费侧 `NewCommercialDraft`。

**二、提供方只读口从写口拆出，形状同发布册的 F-2。** `CommercialResolutionView` 只含 `LoadResolution`；`CommercialResolutionStore` 内嵌它再叠加 `Save`。parcel-shipment 的采用 owner 只持只读口，不得持 `Save`。

**三、分格按恢复动作，不得把永久损坏折成未配置。**

| 情况 | 答案 |
|---|---|
| 委托找不到、尚未已接受、或没有接受决定 | `found=false`：采用版本尚未固定 |
| `LoadResolution` `found=false` | `found=false`：提供方还没有这份闭包 |
| 读失败 / 重建失败 | `error`：要查库或适配器，不是等配置 |
| 取回的闭包租户与调用方不一致 | `error`（[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)）：解析标识不是能力凭证 |
| 闭包在场却未采用 `ACCEPTANCE_RULE_PACKAGE` | `error`：接受流的装配缺陷，与 `adoptedResolution` 同精神 |
| 闭包未采用 `AUTHORIZATION_RULE` | `found=false`：今天必需依据由消费方键源决定，缺席是真话，不发明授权规则 |

**四、本记录不解除实例闸门。** 没有写入提供方解析库的 `ResolutionID` 仍答 `found=false`。取消请求方的格映射仍属实例半边，本记录不解。

## Consequences

- 生产装配把 `UnconfiguredAdoptedStageOwner` 换成按解析标识回指的实现；两条采用消费链共用这一份。
- SYN 夹具的 `ResolutionID=SYN-RES-01` 若未写入 PC 解析库，资格视图继续答未配置——那是真话，不要为了让纵向变绿去种规则包。
- ADR-0058 第三条正文不改写：禁止从身份发明仍然成立；本记录只补「可以从已接受快照回指闭包」这一条路径。

## Alternatives considered

- **拆快照 `RulePackage` 字符串当商业版本键。** 否决：有损，且对象号/版本号可含 `/`。
- **消费侧 `NewCommercialDraft` 造一份 owner。** 否决：那是发明采用版本，正是 ADR-0058 第三条要挡的。
- **闭包未采用规则包时答 `found=false`。** 否决：接受流的唯一闭包必须带接单规则包；缺席是装配缺陷，折成未配置会让运维去等一份不会到来的配置。
- **继续装配 `UnconfiguredAdoptedStageOwner`，等实例参数。** 否决：实例参数是规则包正文与解析行，回指路径是机制；机制不做，实例来了也接不上。

## Links

- [ADR-0058：阶段内容声明按拥有规则对象归属](./0058-stage-content-owned-by-rule-objects.md)：第三条禁止从身份发明；本记录补回指路径
- [ADR-0027：跨上下文多步协议的中间状态由提供方按解析标识保留](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)：消费方只回指标识
- [ADR-0061：已接受委托重建门按快照表达能力开门](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md)：取回委托能拿到决定
- [ADR-0003：集团租户边界](./0003-group-tenant-legal-entity-customer-account.md)：取回后租户必须一致
- [ADR-0025：跨上下文适配器落在消费方](./0025-cross-context-adapters-live-on-the-consumer-side.md)
- [ADR-0064：初始路由适用性闭包标识从已接受解析标识回指](./0064-initial-route-applicability-closure-from-accepted-resolution.md)：同一份标识，适用性走闭包里的服务产品，不改本记录的采用版本路径
