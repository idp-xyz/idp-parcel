# 08 CI 给 ADR 索引与 AT 盘点计数加 -check 门

Category: enhancement
Status: needs-triage
Blocked by: [03](./03-adr-index-and-at-coverage-counts-are-generated.md) 进 main
地盘：`.github/workflows/ci.yml` 的 static job（要能推 workflow 文件的凭据，本机凭据推不上去）
出处：票 03 完成记录「越权风险点」第 5 条。

## 现状

`tools/adr-index` 与 `tools/at-coverage-counts` 都有 `-check`（生成结果与文件不同即非零退出），但 CI 里没有对应的一步。机制清点有（「Mechanism inventory is current」），所以推送方漏了重生成时 CI 会红；这两处漏了不会。

## 先裁的一格

机制清点的约定是各笔在自己那笔上重生成，所以 PR 上跑它的 -check 是对的。这两处相反：票**不**重生成（`docs/agents/parallel-sessions.md`「生成物不占号」），由推送方在 tip 上重生成——PR 上跑 -check 会让每张带新 ADR 或移了 AT 行的票都红。门挂在哪一种触发上（只 `push: main`，还是 PR 上也跑但只报不挡），先定。

## 要什么

- 按上面裁出的触发，在 static job 加一步跑两条 `-check`，形状照「Mechanism inventory is current」那一步。
- 两个工具在主模块里，根目录的 `go vet ./...` 与测试分片已经覆盖它们的代码，不必像机制清点那样单列 vet/test。
