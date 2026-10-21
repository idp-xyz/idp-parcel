# 08 CI 给 ADR 索引与 AT 盘点计数加 -check 门

Category: enhancement
Status: resolved——2026-10-20，见文末「裁定与完成记录」。此前 needs-triage
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

## 裁定与完成记录（2026-10-20）

用户 2026-10-20 把「需要本人处理」的三件交给本会话办（原话「需要我本人处理的，你能帮我处理掉吗」），这一格按此代决。

- **裁定：只挂 `push: main`，PR 上不跑。** 理由就是上面「先裁的一格」：两处生成物由推送方在 tip 上重生成，PR 与分支上按设计就不一致，在那里跑 `-check` 不是报不报的问题，红本身就失去了信号。挂在 main 推送上，守的正是推送方漏了重生成；本仓落 main 走推送方直推，main 的每次推送都会经过它。
- **不另立「PR 上只报不挡」那一档**：PR 上它的结论恒为「不一致」，报出来也没人读，只会把 CI 的红绿训练成噪声。
- **机制清点那一步不动**：它的约定写在它自己的注释里，本票不改；两条约定为何相反写在新步骤的注释里，读的人在同一屏看得到。
- **做了什么**：`ci.yml` 的 static job 在「Mechanism inventory is current」之后加一步，条件 `github.event_name == 'push' && github.ref == 'refs/heads/main'`，依次跑 `go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check`。两个工具在主模块里，根目录的 vet 与测试分片已覆盖其代码，不单列。
- **越权风险点**：一是红在推送之后而不是之前——推送方漏了重生成时 main 会有一个红的 run，要靠下一笔补生成物转绿；若 owner 要求事前拦，就得改成推送方本机跑 `-check`（进 main 步骤里本已有重生成，漏的概率低）。二是 `if:` 条件写死了分支名 `main`，日后改默认分支要一并改。
- **验证**：在本分支树上两条 `-check` 本机退 0；`ci.yml` 能被 YAML 解析。推送要带 `workflow` 权限的凭据，推上去后看 main 上第一次 run 的 static job。
