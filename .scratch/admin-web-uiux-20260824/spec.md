# admin-web 页面 UI/UX 全面开发（2026-08-24 三通道并行轮）

Category: enhancement
Status: in-progress

## 目标与现状

管理台 35 个导航模块条目：2 已接线（提交与撤回、委托查阅）、30 骨架、1 演示、2 规划占位（口岸与申报路径、合规规则库）。骨架页由多会话分批交付，列定义与未配置态文案质量参差，模板层缺筛选与行动作能力，工作台总览只有四档计数。

本轮目标：骨架页全量对齐各自 CONTEXT 词汇与查阅语义，补齐两个占位页，共享层做增量能力（结构化未配置态、筛选槽、行动作），追踪视图页在端点形状确认后接真。

**不做**：造假数据、把未确认参数写成默认值；不碰 `pages/shipment-request/**`（MCP-3 取消切片预留，其取消页接线与 liveIds 登记已被 MCP-3 声明）、`cmd/parcel-api/**`（MCP-4 占号）、`internal/**`（后端地盘）、`tests/bentocontract/**`（MCP-1 占号）。

## 地盘（本轮权威；越界前先在频道说，对方让位再动）

| 通道 | 地盘（均在 apps/admin-web/src/ 下） |
|---|---|
| MCP-7 | templates/**、components/**、domain/**、navigation.ts、page-registry.tsx、Layout.tsx、App.tsx、main.tsx、pages/Workbench.tsx、pages/UnwiredModule.tsx、pages/governance/**、pages/template-preview/** |
| MCP-8 | pages/party/**、pages/pricing/**、pages/network/** |
| MCP-9 | pages/operations/**、pages/customs/**、pages/visibility/**、pages/settlement/**、pages/collection/** |

各页面目录含其 index.ts。新页登记进 page-registry.tsx 与 liveIds 变更一律由 7 号落笔（送导出名即可）。

## 每页通用检查单（02/03 逐页执行，此处是唯一定义）

1. **词汇对照**：列定义、字段注释、状态词与主责 CONTEXT.md 对应小节逐条核对，用原词；缺依据的列删，有依据没上列的补；
2. **查阅语义**：搜索占位文案写清可搜什么；该上下文有明确查询维度的，把应有筛选字段先写成页内注释（等共享筛选槽落库再实装）；
3. **未配置态**：description 如实、带出处，不写「敬请期待」类空话；
4. **一致性与可达性**：列宽/对齐/mono 用法一致，total=0 时分页呈现正确，交互元素有 aria 语义；
5. **自查**：`pnpm exec tsc --noEmit` 过（在 apps/admin-web 下跑）。

## 协作纪律（docs/agents/parallel-sessions.md 在本轮的绑定）

- 只改自己地盘；共享层改动需求 send_to_session 7，由 7 号落；
- 全量 `pnpm build`（会写 dist）只由 7 号跑；各票自查用 `tsc --noEmit`；
- 逐文件 `git add` 自己地盘内文件，可自行 commit（提交信息中文、写明票号），不 push；
- 不跑全仓派生态命令（gofmt -w、go mod tidy、全树格式化等）；
- 注释一律中文、术语用文档原词、跨文件引用写文件名或符号名不写行号（AGENTS.md 红线）；
- 完成：票面 Status: resolved + 附提交 SHA 与验证结果，report_task 标 done，send_to_session 7 简报。

## 子票

- 01 共享层增量与治理区（7 号，in-progress）
- 02 主数据/计价/网络 13 页（8 号）
- 03 作业/关务/追踪/结算/代收 10+2 页与追踪接真（9 号）
- 04 外壳 hash 路由与两处过期未配置文案（3 号，补记，resolved）
