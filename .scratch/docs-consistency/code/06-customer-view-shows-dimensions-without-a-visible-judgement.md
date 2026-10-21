# 06 VE 客户视图的 ETA 维在没有任何客户可见 ETA 判断时也答 `SHOWN`，里程碑维同病较轻

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code05（基于 main `35d181d1c`，接在 code/05 之后）。原分级 R1（上线前必修）不变。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-02（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

客户视图的 ETA 一维只翻译披露策略的答复，不看有没有获准对客展示的 ETA 判断。终局一维有 `domain.FinalDimensionOver` 按投影兜底，ETA 一维没有。里程碑一维：投影里只有未归类条目时（例如只有初始路由那一份），策略照样答 `SHOWN`。

## 证据

- 文档：
  - VE CONTEXT「客户可见性与通知」：「客户全程追踪视图只基于当前有效的全程追踪投影、获准对客展示的 ETA、`parcel-shipment` 提供的客户服务终局，以及已经批准披露的客户可见异常形成。」
  - Lifecycles「客户全程追踪视图」：「某一维信息待确认或不可披露 → 只对该维返回待确认、暂不可用或不展示，不虚构里程碑、ETA、终局或异常说明。」
  - UC-VE-008 主流程第 4 步：「只有达到适用质量与合同门槛的 ETA 才进入视图」；`AT-VE-158`：「不展示 ETA，也不使用计划或承诺时间填充」。
- 代码与配置：
  - `internal/visibilityexception/application/derive_customer_view.go` 的 `dimensionsFrom`：`eta, err := dimensionFrom(answer.ETA)`，终局一维才过 `domain.FinalDimensionOver(final, projection)`。头注自承「ETA 一维要的是 `UC-VE-003` 的对客可见判断（`AT-VE-158`），那份判断不在投影里，不在这里补」。
  - `NewFormETAHandler` 在 `cmd/` 下无调用方。
  - 演示策略 `scripts/demo-seeds/data/visibility/07-disclosure-policy-v1.json` 与 `14-disclosure-policy-v2.json` 给 `SYN-ACCOUNT-01` 的 `eta` 都是 `SHOWN` / `SYN-VE-CONTENT-ETA-BAND-V1`。
  - HTTP 响应体 `dimensionBody` 只带 `contentRef`，不带 ETA 值。

## 三类

机制：维度派生按「有没有可见判断」兜底。产品策略：无。租户取值：披露策略照旧由租户登，不在本票改。

## 规模

S–M：ETA 维加一道与 `FinalDimensionOver` 同形的兜底（读 UC-VE-003 的对客可见判断；读不到即待确认）；里程碑维在投影没有已归类条目时答待确认；补 `AT-VE-158` 的用例。

## 最小的诚实修法

1. ETA 维：没有当前有效的客户可见 ETA 判断，就答待确认，不答展示。今天生产上没有 ETA，等于 ETA 维一律待确认——这是如实的答案。
2. 里程碑维：投影里没有已归类为标准追踪里程碑的条目时答待确认。
3. 两处都只收窄展示，不放宽：策略答不展示的维照旧不展示。

## 要裁的

- `contentRef` 在 UC-VE-008 结果语义里指什么（策略允许展示的内容集，不是内容本身）——属文档，交修订票 docs-consistency/04 写明。
- ETA 维兜底读 UC-VE-003 判断的端口形状：从 ETA 库按（包裹，里程碑）取当前对客可见的那一版。依赖 [07](./07-eta-store-overwrites-history-and-mixes-sources.md) 修好 ETA 存储之后读法才稳；在那之前读不到即待确认，本票不必等 07。

## 完成记录（2026-10-21，分支 claude-code05）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话代 VE owner 作出，待 VE owner 复核。

### 做了什么

- 领域 `domain.ETADimensionWithoutVisibilityJudgement`：策略准许展示 ETA 时这一维答待确认、不带内容；策略不展示与待确认原样放行。
- 领域 `domain.MilestonesDimensionOver` 与 `TrackingProjection.CarriesClassifiedMilestone`：策略准许展示里程碑、投影里却没有一条已归类为标准追踪里程碑的条目时答待确认；有一条已归类即照策略展示；只收窄不放宽。
- 派生编排 `dimensionsFrom`：里程碑、ETA 两维与终局一维同样再过一道；头注改写，删去「那份判断不在投影里，不在这里补」。
- 测试：领域 `TestTheETADimensionPendsWithoutACustomerVisibleETAJudgement`、`TestTheMilestonesDimensionPendsUntilTheProjectionCarriesAClassifiedMilestone`；编排 `TestAPermittedETAPendsWithoutACustomerVisibleETAJudgement`、`TestPermittedMilestonesPendUntilTheProjectionCarriesAClassifiedMilestone`（里程碑维由待确认转展示时四维变了、换代指回前版，与 code/05 的换代判据衔接）。先写用例、确认红（编排用例答 `SHOWN`），再实现。首版用例与终局用例里「ETA 照策略展示」的期望随改。
- 文档：UC-VE-008 `AT-VE-158` 与结果语义「客户追踪视图」一行的现状缺口注改为已补，写明 ETA 一维要能答展示仍待 ETA 客户可见性判断落地（`AT-VE-187`）。

### 代决

1. **不接 `NewFormETAHandler`，也不加读 ETA 库的端口。** 「要裁的」第二格的端口形状不在本票定。理由：ETA 库里存的是内部预测（`ETAPrediction`），本上下文没有 `UC-VE-003` 的「ETA 客户可见性判断」这一对象（`AT-VE-187` 无实现）；从预测库取一份当成可见判断，正撞 UC-VE-008 主流程「不能使用计划时间或内部预测填充」。生产上本就从不形成 ETA，所以「ETA 维一律待确认」就是票面「最小的诚实修法」第 1 条说的如实答案。不动生产装配，接线棘轮不涉及。端口形状留到 ETA 客户可见性判断建模时一起定，那时 `ETADimensionWithoutVisibilityJudgement` 改为收那份判断，而不是另起一道。
2. **里程碑维的判据是「至少一条已归类条目」。** 按里程碑逐条筛「允许对客的公开里程碑」（`PAR-VIS-09` 普通追踪公开规则）不在本票：那份规则今天没有建模，视图也只带内容引用、不列里程碑本身。
3. **演示种子不改。** 演示策略给 `SYN-ACCOUNT-01` 的 `eta` 登的是展示，这是租户取值（准许展示），仍然成立；派生出的视图如今答待确认，是机制如实的答复。种子步骤里没有断言视图 ETA 态的地方。

### 越权风险点

1. 租户把 ETA 维登成展示，在 ETA 客户可见性判断落地之前也永远读到待确认；本票没有在登记那一侧加任何提示。今天仓里没有为「ETA 客户可见性判断建模并接入客户视图」立的票，宜由协调方立票（`AT-VE-170`、`AT-VE-187` 同在 ④）。
2. 里程碑维只看「有没有已归类条目」：某一类事实归进了里程碑、却不该对客（内部节点里程碑），这一维照样答展示。逐里程碑的对客过滤待 `PAR-VIS-09` 建模。
3. 已经发布过的、ETA 维为展示的视图，下一次派生时四维变了（ETA 转待确认），会换一代并指回前版；客户会看到一次「ETA 由展示转待确认」的替代。今天没有租户，演示库重灌即无存量。

### AT

- `AT-VE-158`：AT 盘点由 ④ 移入 ①，点名编排与领域两条用例。总览计数未动，由协调方进 main 时重算。

### 验证（分支 claude-code05）

同 code/05 完成记录的验证一节（同一分支、同一跑）：`go build ./...`、`go vet ./...`、`gofmt -l` 干净；`scripts/test-full.sh` 跑 `internal/visibilityexception/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-ve-register`、`internal/architecture/...`、`migrations/...`、`internal/platform/migrate/...`，21 个包 ok。

