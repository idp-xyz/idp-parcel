# 23 party-commercial：时点语义格对参考配置引用的登记门

Category: enhancement
Status: ready-for-agent——2026-09-30 通道 1 按用户授权自决立（[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md) Comments「裁决 ← 通道 1」第 5 条）；派通道 2，与票 06 修复同一分支 `mcp2-submission-receipt-asof`、分笔提交、一起重放
Blocked by: ADR-0157（随票 06 修复同一分支起草）
地盘：party-commercial 规则包与价格政策发布翻译里时点语义格那一段及其测试（落在领域构造门还是翻译适配器，按分层门禁定）；party-commercial `CONTEXT.md` 那一句。不碰 parcel-shipment。
出处：票 06 Comments「评审 ← 通道 1 · 钉 `299cd954`」的阻断与其后的裁决。

## 做什么

1. 接单规则包的逐项时点策略语义格与价格政策汇率口径的时点语义格：值带 `REFCFG-1:` 前缀时，发布翻译经 `referenceconfig` 解析并确认该版已发布，不过就拒并给出明确错误，不落成一个到判断时才静默答`未配置`的引用。不带前缀的值照旧不透明。
2. ADR-0147 决定五的构造门测试：引用 `parcel-shipment/as-of-semantics/submission-receipt@1` 的一项经发布翻译形成领域对象；引用未发布版本、引用串形状不合的被拒。
3. 演示种子 `scripts/demo-seeds/data/commercial/publish-batch.json` 经同一翻译通过（含重算后的 `contentDigest`）：测试或一次性库 `seed.sh --reset` 实跑，二选一，证据写完成记录。
4. party-commercial `CONTEXT.md`：规则包那句补上价格政策汇率格，并写明登记时校验引用已发布。

## 不做

- 不在 PC 领域类型里列形态清单，PC 不把引用折成时刻。
- 不给计价侧加汇率时点的执行器。

## 完成判据

- 上面四项各有落点与证据。
- 全仓 `go build` 与 `go vet` 绿；受影响包及其反向依赖 `-count=1` 绿，`cmd/*` 带 DSN。
