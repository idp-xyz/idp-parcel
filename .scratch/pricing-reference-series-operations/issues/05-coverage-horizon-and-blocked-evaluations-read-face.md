# 覆盖地平线与被挂起评价联动的读面

Category: enhancement
Status: ready-for-agent——**但第 2 项的范围与票面所写不同，认领前先读 Comments**
Blocked by: 无（03 的代码已是主线祖先 `f62d619`）；第 2 项另有一道**未裁的领域改动**，见 Comments

## 要建什么

让缺口在当天被看见，而不是月底对账时。沿 ADR-0077 读面通例（独立查询端点、租户级作用域、未配置即拒、空册如实答空）：

1. **伴生读口 `ReferenceSeriesCoverageRead`**（`ports/catalogue_read.go` 同款，不拓宽写口）：按租户列出每条序列——种类、标识、在用版本引用、在用版本末期止点（开放则为空）、距 `now` 剩余覆盖时长、最近一次复核（时刻 / 结论）、未复核版本数。
2. **被挂起评价联动**：从评价库按待判断且原因码 ∈ {`REFERENCE_SERIES_UNRESOLVED`, `EXCHANGE_RATE_UNRESOLVED`} 计数并按序列种类分组——核 `ports/evaluation_read.go` 有没有可复用的列面；没有就在本票补最小读口，不扫 JSONB（ADR-0060 同款判据）。
3. **管理台**：在「计价参考序列」页顶部加覆盖摘要条（每序列一格：剩余覆盖天数、未复核版本、挂起评价数）；一键跳到该序列的登记 / 复核动作。
4. **告警阈值不在本票**：「剩余覆盖低于 N 天告警」「日环比超 X% 标黄」都是租户参数——机制只把数算出来摆在读面；阈值格留在配置里显式未决，页面文案说「阈值未配置」。

## 红线

- 读面不形成判断、不重建领域对象；只转写列面。
- 无租户时空列表是正常答案，不是故障。

## 验证

真库读口用例；`tsc --noEmit` 绿。

## Comments

- 2026-09-03 · MCP-1：**没动代码。第 2 项踩到两道票面没写的前提，第二道是领域改动，
  超出我拿到的授权，交回给 owner 裁。第 1、3、4 项不受影响。**

  票面第 2 项写的是「核 `ports/evaluation_read.go` 有没有可复用的列面；没有就在本票补最小
  读口，不扫 JSONB」。**「补一个最小读口」做不到**，理由有两层，取证如下（锚 `297d723`）。

  **第一层：那一列不存在。** `ports.EvaluationCatalogueRow` 只有 `Status`（五格封闭：
  COMPLETED/PENDING/CONFLICT/FAILED/UNRATABLE），没有原因码；`migrations/parcel_pricing/0001_evaluation.sql`
  的列也只有 `status`，**原因只活在 `snapshot` 那个 jsonb 里**，而票面自己禁止扫 JSONB。
  读口读不出一个不存在的列，所以这一项的真实前提是**先加列**，不是补读口。这一层人已授权。

  **第二层（真正的拦路者）：「按序列种类分组」在今天的领域里没有数据可分。**
  `domain.EvaluationIssue` 只有 `code` 与 `message` 两格（`evaluation.go`）。产出待判断那两格
  的地方是 `newEvaluationIssue("REFERENCE_SERIES_UNRESOLVED", seriesErr.Error())` 与
  `newEvaluationIssue("EXCHANGE_RATE_UNRESOLVED", …)`——**序列种类只以自由文本混在 message 里**。
  要按种类分组，就得给 `EvaluationIssue` 加一格结构化的种类，**那是领域模型改动**，不是加一列
  库面。按 AGENTS.md，难逆转的取舍走 ADR 或至少由 owner 裁；我拿到的授权是「加那一列」，
  不含这一条，所以停在这里。

  **还有一件形状问题，一并摆出来供裁**：一次评价带的是**一组** issue 不是一个
  （`withOutcome` 之前可能已追加过 `EXCLUDED_BY_RATE_CARD` 一类）。所以「加一列 reason_code」
  也不对形——要么 `text[]`，要么一张 `evaluation_issue` 子表（一行一条 issue）。子表是正规解，
  但它把本票从「补读口」变成「建表 + 改评价写侧 + 改领域」。

  **建议的拆法**（不是裁决，是给 owner 的选项）：

  - **05a**：第 1 项（`ReferenceSeriesCoverageRead` 覆盖地平线读口 + 真库用例 + 端点）。
    自足、不碰领域、不建表，随时可做。
  - **05b**：第 2 项。先裁两件——issue 要不要带结构化的序列种类（领域）、issue 落库用子表
    还是数组（库面）。裁完再动手。
  - 第 3 项（管理台摘要条）依赖 05a 与 05b；第 4 项（阈值）票面已明定不在本票。

  **这一格与今晚另外四件同族，写下来是因为它最难被撞见**：票面那句「没有就补最小读口」读起来
  像已经核过——它具体、有位置、给了退路。而**退路本身预设了那件事补得出来**。真去补的人会先
  写好读口再发现库面没有那一列，再发现领域没有那一格，两次都在已经动了手之后。
