# VE 运营追踪读口(追踪页接真裁决落地,方向 B)

Category: enhancement
Status: in-progress

## 背景与依据

- admin-web 全程追踪页接真卡在裁决闸「运营查阅作用域是否复用客户隔离读口」(记于 admin-web-uiux-20260824 票 03/04)。
- 取证简报:`.scratch/admin-web-uiux-20260824/tracking-scope-decision-brief.md`——复用客户读口是结构性漏报(覆盖:归属不可确定的包裹无客户视图行而投影存在;键形状:三元组点查无列表面;内容删减:披露规则过滤内部维度;探针合并:VIEW_NOT_FOUND 一格三义),故走 B:另建运营读口消费投影库。
- 授权来源:用户 2026-08-24 晚经 IDP 队列指示并行完成剩余工作;方向 B 系协调位(MCP-1)两次呈报建议后放行。若用户改口,本规格作废,已落 ADR 按 supersede 流程处理,不改写历史。
- 深度上限(取证简报第四节末条):PAR-INT-01(认证方式)未登记前,任何接线只到「前端发请求、如实渲染 403 未配置态」一档,与已接线的委托查阅页同档。本规格四票不改变该上限;真轨迹数据等渠道参数登记,属实例半边。

## 地盘(本轮权威;越界前先在频道说)

| 票 | 通道 | 足迹 |
|---|---|---|
| 01 | MCP-5 | docs/domain/visibility-exception/CONTEXT.md、docs/adr/(新 ADR 与 README 索引行) |
| 02 | MCP-3 | internal/visibilityexception/(ports、adapters/http、adapters/postgres)、cmd/parcel-api/(装配面,占号) |
| 03 | MCP-5 | apps/admin-web/src/pages/visibility/**、page-registry.tsx 的 liveIds 自落行(占号) |
| 04 | MCP-7 | 收口 build(提交态验证)、本规格状态收口 |

## 阻塞边

01 → 02 → 03 → 04。每票以前票报出的提交 SHA 为开工条件;等待期允许只读预研(读代码、列问题),不落笔、不占号。

## 纪律(沿用 admin-web-uiux-20260824 spec 与 docs/agents/parallel-sessions.md,此处只点名)

- 逐文件 add 自己地盘文件,提交前 git diff --cached --stat 核对暂存清单;提交信中文带票号;不 push。
- cmd/parcel-api 装配四文件(endpoints.go/endpoints_test.go/main.go/unwired_orchestration.go)是共享接线文件:占号→改→释号。原 MCP-4 的占号随其 crash 与 UI 轮收口失效,由票 02 接手,接手声明写在占号广播里。
- 真库用例以 -v 下 PASS/SKIP 判别,不以 ok 判别;验证一律 -count=1。
- adr/README.md 索引行只加自己行,不动邻行。
- 完成:票面 Status: resolved + SHA 与验证结果,report_task done,send_to_session 1 简报。

## 子票

- 01 CONTEXT 运营查阅语言 + ADR(5 号)
- 02 投影读口与端点施工(3 号,blocked on 01)
- 03 追踪页前端接线(5 号,blocked on 02)
- 04 集成收口(7 号,blocked on 03)
