# T1 门族五票对当前 main 重核：platform/PS/PP（01/02/07/08/10）

执行：MCP-2（派工 T1-REVERIFY，2026-08-20）
基线对：票面取证 `49a2ab0` → 本次重核 `3324ecb`（开工时 origin/main tip）
方法：按 [report.md](./report.md) 的四件逐票核——配置仓储 / 装载口 / 写入方 / 进程级登记口；只读代码与票面，不改代码、不改 Status。逐票证据（文件与 SHA 点名）在各票 Comments 的「2026-08-20 对 3324ecb 重核」条，本文只汇总，不复制第二套。

## 五票结论

| 票 | 墙 | 49a2ab0 判定 | 3324ecb 重核 | 变动 |
|---|---|---|---|---|
| [01](./issues/01-access-channel-registry-and-first-real-intake.md) | W01/W02 接入渠道 | 无门（四件全缺） | **原样**：八端点仍全桩，全库无渠道登记册表 | 区间 `cmd/parcel-api` 零提交 |
| [02](./issues/02-production-ownership-authority-has-no-adapter.md) | W03 生产归属 | 无门（桥缺） | **原样**：PS 侧仍仅接口+测试替身、无 PG 引用；治理侧表/写入方/用例俱在、无进程入口 | 区间两笔只改治理 handoff 信封（分区/ID），不动四件 |
| [07](./issues/07-pricing-plan-and-rate-table-no-version-repository.md) | W14 价卡 | 无门（四件全缺） | **原样**：迁移仍只有 `0001_evaluation.sql`，端口仍三口，用例未接进程 | 区间 parcel-pricing 零提交 |
| [08](./issues/08-pricing-reference-series-register-missing.md) | W15 参考序列 | 无门（四件全缺） | **原样**：序列仍纯域内对象，无表/口/写入方/入口 | 同上 |
| [10](./issues/10-intake-qualification-evidence-source-unimplemented.md) | W11 证据面 | 证据四件全缺 | **结论原样，票面一处漏记**（见下） | 装配点两处仍显式未配置 |

**五票全部可按票面开工**；无一票因代码演进而失效或缩水。

## 票面与代码的矛盾（仅一处）

票 10 写「端口与『诚实无门』实现在」，漏了 `KnownPrefixIntakeQualificationEvidence`
（按引用前缀把证明路由到已登记权威口，`7cb39b6`，08-18 落地，早于审计基线）——组合缝
当时已在，只是没有任何真源接在缝后，也没装进两处装配点。对开工的影响：证据门的落点
应读作「给既有 KnownPrefix 缝接真源并装配」，不是「另起证据口」。缺件范围不变。
已在票 10 Comments 照实更正。

## 07 特核答复

**PP 仍是全库唯一「配置仓储本体都缺」的上下文。** 对照组：同区间 NR 已长出版本化网络
目录七表骨架（`3b9f212`，ADR-0068），其余上下文的配置表俱在（缺的是写入方/登记口层）；
仅 PP 的价卡与参考序列两族连表都没有，评价入参今天只能由测试构造。

## 区间扫描附注（49a2ab0..3324ecb 与五票相关路径）

- `cmd/parcel-api`、`internal/parcelpricing`：零提交。
- `internal/pilotgovernance`：`81b50f2`/`3b37b5a`（OUTBOX-PK-STEP2 信封分区与 ID），与门四件正交。
- `internal/parcelshipment`：`11057dc`（VE-008 只读口），与五票无关。
- `migrations`：新增 NR `0008_network_catalog.sql`（票 04 地界）与 SA `0012`（无关）。

本文不提实现方案；Status 变更留给 MCP-1。
