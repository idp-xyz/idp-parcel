# 24 PC 授权规则或客户合同的版本收尾之后，授权授予与合同委派照旧生效

Category: bug
Status: ready-for-agent——R1（上线前必修）：答错且留错痕。收尾口（票 demo-chain-gaps/50 进 main，ADR-0233）对授权规则与合同都收；运营退役一版授权规则来收回某个等级的拒绝、关闭或重开权，这些权限实际上照旧生效。更重的一格：同起点重发 v2、退役 v1 之后，`Authorize` 取首个命中，命中的是已退役的 v1 授予，消费方保存的「所采用的授权规则版本」就是一版已退役的规则。第一次更正错登的授权规则就撞，没有绕法（只能把授予逐条改区间）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 文档自洽 07 已裁（[ADR-0261](../../../docs/adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)）：授予与委派**随所挂版本收尾**，多条命中答适用冲突。原 Blocked by：docs-consistency/07 先裁版本壳的截止与退役对授权授予与合同委派怎么作用。
出处：[audit/pc-pp.md](../audit/pc-pp.md) H2（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，PC 代码在此期间未改）。

## 现象

收尾口对任何商业对象种类都收（含授权规则），但授予与委派的装载只按各自的区间取行，不连退役册、不连区间更正册；授予还按（对象，版本标签）排序、取首个命中，退役的旧版排在前面。

## 证据

- 文档：PC CONTEXT「商业规则与政策版本」生命周期：「已生效 → 已到期、已退役或已替代：停止用于新的解析和判断……」，「本生命周期共同适用于接单规则包、接受前财务控制策略、商业价格政策、定价方案绑定、结算政策、信用政策和授权规则」。
- 决定：ADR-0233 越权风险点 6：「**合同委派、授权授予不随版本收尾**（PC owner）。同起点退役一版合同之后，挂在旧版上的委派照它自己的区间适用。」
- 代码：
  - `internal/partycommercial/adapters/registrationjson/version_closure.go` 经 `CommercialKindFrom` 取种类，`AuthorizationRuleObject` 也收。
  - `adapters/postgres/authorization_grant.go` 的 `LoadEffectiveGrants`：`effective_starts_at <= $3 AND (effective_ends_at IS NULL OR effective_ends_at > $3)`，`ORDER BY object_id, version_label`。
  - `adapters/postgres/contract_delegation.go` 的 `LoadEffectiveDelegations`：同样只看委派自己的区间。
  - `domain/authority_grant.go` 的 `Authorize` 取首个命中。

## 三类

机制：授予与委派的装载折进版本壳的收尾。产品策略：收尾对授予的作用（待 07 裁）。租户取值：哪一版退役、何时截止，由租户的运营决定给出。

## 规模

M：两个装载连上退役册与最新区间更正册（与 ADR-0233 给版本点读的折法同一口径）；`Authorize` 在多版并存时按收尾后仍有效的那一版取，并核「同一刻至多一版有效」；补同起点重发与截止两种用例。

## 最小的诚实修法

按 07 的裁定二选一：

- 若裁为「随版本收尾」（CONTEXT 照旧）：授予与委派的装载折进版本壳的截止与退役；同一授权规则对象多版并存时只取当刻有效的那一版，取不出唯一就答适用冲突，不取首个。
- 若裁为「按自身区间」（改归 B）：CONTEXT 照改；收尾口对 `AuthorizationRuleObject` 与带委派的合同版本给出明确答复（拒收，或收尾时提示须同步截止授予），并修掉「取首个命中」——那一格两种裁向都是错的。

## 要裁的

全部在 Blocked by 那一裁里；「取首个命中」不随裁向，必修。

## 裁向（2026-10-21，文档自洽 07）

[ADR-0261](../../../docs/adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md) 取上文「最小的诚实修法」第一支（随版本收尾），并把「取首个命中」那一格一并裁成适用冲突。照此实施：

1. `LoadEffectiveGrants` 装载的授予折进退役册（`party_commercial.commercial_version_retirement`）与壳上最新一条区间更正：版本已退役的授予不交出；有更正的，判用区间终点取更正终点与授予自身终点中较早的一个，只截终点（与 ADR-0233 决定五给自带区间的政策正文的折法同一口径，宜复用同一处折法）。范围加时刻有没有现行规则，按折后的授予判。
2. `LoadEffectiveDelegations` 对所挂客户合同版本同一折法。
3. `domain.Authorize`：折后命中多于一条授予即交回新的具名错误（适用冲突），不取首个；`resolveDecider`：命中多条委派而委派方不同同答适用冲突，委派方相同取那一个决定方。新错误要与 `ErrNotAuthorized`、`ErrAuthorityRulesNotConfigured` 分得开（按恢复动作分格，ADR-0029）。
4. `parcel-shipment` 的各授权适配器（主动拒绝、人工复核、资料修订、受控关闭与重开、撤回）把适用冲突译成各自的未决，不落`不允许`、不当作已授权。
5. 补用例：`AT-PC-042`、`AT-PC-059`、`AT-PC-060`、`AT-PC-061`（UC-PC-003），领域与真库各一，并在 `.scratch/at-coverage-inventory.md` 把这几行从④移入①。

