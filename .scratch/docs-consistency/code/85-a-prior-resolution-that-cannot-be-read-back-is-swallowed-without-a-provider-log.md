# 85 第二、三阶段取回原解析读不回时，`loadPrior` 把 error 吞成未决、不记提供方日志，滚动发布时旧构建读不懂新快照无从察觉

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（线外，复核见 .scratch/launch-line-freeze-2026-10-27.md 第六节）：第二、三阶段取回原解析读不回时 `loadPrior` 吞 error——可观测性，不改结局代数；解冻条件：需要读出这一格时。此前 needs-triage——建议 R3（可观测性，不改结局代数）。2026-10-21 协调方据票 [docs-consistency/code/80](./80-the-commercial-resolution-snapshot-digest-carries-no-canonicalization-version.md) 隔离评审 N1 立（用户「你作为业务和系统专家自决」授权）。
Blocked by: 无
上下文：PC（参与方与商业），应用层。
出处：code/80 隔离评审 N1。

## 现象（读码，只记 `S`）

- `ValidateCommercialBasisHandler.loadPrior` 与 `FormJudgmentAsOfHandler.loadPrior` 调 `CommercialResolutionView.LoadResolution` 得到 error 时，直接答`解析未决`·`权威不可读`与 `JudgmentAsOfPending`，error 本身丢掉，也不记日志。
- code/80 之后，`LoadResolution` 在两种情形下会因构建版本而失败：读到本构建不认得的规范化版本（`domain.ErrCanonicalizationUnsupported`），或在 `PCR-1` 里读到不认得的键。滚动发布或回滚时，旧构建读新构建固定的解析正落在这里。
- PS 侧随之是`无法判定`或未决，经 `ErrAcceptanceChainUndecided` 回滚后重投；只要旧构建还在跑，同一封信就一直重投，日志里看不到是哪一份快照、因为什么读不回。

## 规则

- ADR-0029：取不到时按调用方的恢复动作分格，读不回落重试那一格。今天的归类合规，本票不改结局代数。
- 可观测性：提供方该记下读不回的原因（库错误、快照形状、未知规范化版本），供运维区分「依赖抖动」与「构建不认得这份快照、要换构建」。

## 三类

机制。产品策略：无。租户取值：无。

## 规模

S：两处 `loadPrior` 读不回时记一条提供方日志（带租户、解析标识与 error），答复不变；日志器的注入照 ADR-0222 的装配办法。

## 最小的诚实修法

1. 两个 handler 收一个日志器，由装配处给；没给时是装配期报错还是退到无操作日志器，领票时照 PC 应用层先例定。
2. `LoadResolution` 返回 error 时记一条，再照旧答未决；`ErrCanonicalizationUnsupported` 可单列一个字段，让「要换构建」一眼可分。
3. 用例：替身读口返回 error，断言结局不变、日志记了一条。

## 相关

- 票 code/80：解析快照接 `PCR-1`，代决 4 与越权风险点 2 写了读回那一路。
- ADR-0029、ADR-0014。
