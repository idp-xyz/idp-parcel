# 01 共享层增量与治理区对齐（MCP-7）

Category: enhancement
Status: in-progress

地盘、通用检查单与协作纪律见 ../spec.md，不复述。

## 活

1. **结构化未配置态**：components/states 的 UnconfiguredState（或模板 StateSlot 层）支持「主责上下文 / 场景出处 / 放行条件」结构化呈现，保持现有 description 字符串用法兼容；落库后广播「共享增量已落库」；
2. **列表模板增量**：ListPageTemplate 增可选筛选槽与可选行动作（onRowOpen 类），均为增量 props，不破坏现有调用；
3. **工作台**：分区就绪度小结（每区已接线/总数），档位继续从登记机制派生，不造第二份状态；
4. **治理五页**：pages/governance/ 按通用检查单过一遍，切换新未配置态结构；
5. **登记代办**：8/9 送来的新页登记与 liveIds 变更由本票落 page-registry.tsx；
6. **集成**：02/03 报完后跑全量 `pnpm build`，绿后本轮收口。

## 顺序约束

第 1、2 项先行（8/9 的第二遍依赖它们）；模板与状态组件的 props **只增不改不删**——并行会话正在消费这些类型，破坏性签名变更会让整棵共享树对所有人变红（parallel-sessions.md「工作树也会被阻断」）。

## Comments

- 2026-08-24（MCP-7）进度：第 1 项随 a5fa176 落库（facts 三段式）；第 2 项无需实现——ListPageTemplate 本就有 filters/filterSummary/onRowClick/headerActions 槽位，已广播更正；第 3、4 项随 acf59cf 落库；预览页 facts 演示档随 0202a3d 落库。剩第 5 项（登记代办）与第 6 项（收口 build）。
- 2026-08-24（MCP-7）事故记录：0202a3d 提交时误吞暂存区——本人只 add 了预览页 1 文件，但 MCP-5 已暂存未提交的 17 文件（customscompliance 批）被 git commit 一并吃入。拆解（soft reset 重打）被 MCP-9 后续叠加 d534d7b 阻断，按「不重写含他人提交的历史」中止，处置为保留历史 + 归属以 MCP-5 票面为准（已知会双方）。教训：**共享树上 git commit 吃的是整个暂存区，不是你刚 add 的那几个文件**——提交前必须 git diff --cached --stat 核对暂存清单只含自己的文件；已广播，拟提案补进 parallel-sessions.md「提交：精确暂存」一节。
