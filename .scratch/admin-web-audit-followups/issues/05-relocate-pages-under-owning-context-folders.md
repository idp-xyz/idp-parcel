# 05 四张页面归位到所属上下文目录

Category: enhancement
Status: resolved（2026-09-03，MCP-2，`0ba050d`）
Blocked by: 无

## 缺什么

`apps/admin-web/src/pages/governance/` 今天装着五张页，`navigation.ts` 的 `moduleInfoById` 给
它们的主责上下文却分属三处：

| 页 | `moduleInfoById.owner` | 现目录 | 应属目录 |
|---|---|---|---|
| `ReconciliationPage` | 结算与经营核算 | governance | `settlement/` |
| `SettlementApplicationPage` | 结算与经营核算 | governance | `settlement/` |
| `ExceptionTriagePage` | 全程追踪与异常 | governance | `visibility/` |
| `AcceptanceReviewPage` | 小包托运 | governance | `shipment-request/` |
| `StageAdmissionPage` | 试点治理 | governance | 留在 `governance/` |

目录是 README 里「治理与复核类页面」那一批多会话并行时的历史分组，与现在按上下文分目录的
其余页面不一致。读代码的人按上下文找页会找不到。

## 做什么

移动文件、改各桶导出 `index.ts` 与 `page-registry.tsx` 的 import 路径；页面内容一字不改。
`page-registry.tsx` 是共享接线文件（第三类），只改 import 行、不动登记表与 `liveIds`。

`governance/api.ts` 只服务 `StageAdmissionPage`，留原地。

## 完成判据

`tsc --noEmit` 无输出；`pnpm test` 仍绿；`git log --follow` 能追到移动前历史（用 `git mv`）。

## Comments

- 2026-09-03 · MCP-2：**落于 `0ba050d`，转 resolved。** 四页 `git mv` 到表中目录，`StageAdmissionPage`
  与 `governance/api.ts` 留原地；目标目录逐条对过 `navigation.ts` 的 `moduleInfoById.owner`。

  **票面「页面内容一字不改」实际怎么守**：四页各自随目录必须改的相对 import 改了
  （`'../settlement/api'` → `'./api'` 这一类），行为、栏目与文案一字未动——不改就编不过，那条
  写的是内容不是字节。`ReconciliationPage` 页内那句「本页与收付款核销页历史上落在
  pages/governance/」照旧留着：它说的是历史，搬后仍真。

  **票面没点名但一并改真的三处**：`settlement/api.ts` 头注原先拿「两页落在 governance、从此处
  引入」当四端点同住一文件的理由，搬后为假，改成理由本身（四页同上下文、镜像同一个 Go 包）；
  各桶 `index.ts` 自注里「异常分诊在 governance，不在本目录」「两页历史上落在 governance，
  不迁」两句同样为假，随导出行一并改；`apps/admin-web/README.md`「现状」里 `src/pages/governance/`
  那一句原列五页，改为只列阶段决定并说明其余按主责上下文归位。留旧注释比多改三处更贵：
  它们会把下一个读者引回一个已不存在的目录。

  **评审两轴**（基线 `f62d619`）：Standards 拿住两处——新注释里对别处页面的装饰性计数
  「四页」（AGENTS.md「改文档」），与 `governance/index.ts` 把已接 `GET /governance-registers`
  的阶段决定页说成只呈现未配置态；两处均已修。Spec 无缺项，超范围即上一段三处，理由如上。

  **验证**：共享树上 `tsc --noEmit` 无输出、`pnpm test` 16/16、`go test ./internal/architecture/
  -run AdminWeb -count=1` 绿（管理台路径 ⊆ 端点表的门禁，搬迁不改路径字面量）；detached worktree
  检出 `0ba050d`（`node_modules` 以目录联接借共享树那份）三项同结果；四个新路径
  `git log --follow` 均穿过搬迁提交追到各自的创建提交（`ad595d8` 管理台首发，或 `64dcce4`
  收付款核销独立成页那一笔）。提交时树上另有 `pricing-reference-series-operations`
  票 03 与 `scripts/demo-seeds` 两份 JSON 的在途改动，属另一会话，按 pathspec 提交未带走。
