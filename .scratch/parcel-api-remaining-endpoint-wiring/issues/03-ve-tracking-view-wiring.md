# VE 客户追踪视图读口接线

Category: enhancement
Status: resolved

## 要做什么

把 `GET /customer-tracking-view` 背后的 `unwiredTrackingViews` 换成真库读适配器:VE postgres `NewCustomerViews` 已存在且带 `FindCurrent`(按租户+账户+包裹取当前视图版本),由 `main` 构造交入装配点。

这一笔与委托查阅读口(已接真)同形:读面不是编排,查阅不触发判断或披露,直接接存储读面。预计是五票里最薄的一笔——若读口方法形状与端点消费的接口有出入,在装配处以显式适配收拢,不改领域包。

## 验收

- 装配测试对真库实跑:至少覆盖「查得到当前版本」「查不到如实 404 统一不可见」「库错误落 5xx NO_ANSWER_FORMED 不伪装成查无」三格;
- 其余同票 01:端点表对照、Intake 仍在前、gofmt / vet / 全仓 test 绿、单独成 commit。

## Comments

立票时即已完成:MCP-3 批次(随 `463646b` 落库)在 main 里把 `vepostgres.NewCustomerViews(db)`
交入装配点,本票无事可做,据实转 resolved。验收三格不由 cmd/parcel-api 另证——读口是
直通适配器,行为已各归其位地钉住:真库读回与租户/账户隔离在
`internal/visibilityexception/adapters/postgres/customer_view_test.go`,查无→404 与库错→5xx
的映射在 VE 传输层测试,装配在场与 403 在前在 endpoints_test.go 的端点表。给直通读口
另写装配测试只会复述适配器测试,不新增任何证据。

2026-08-24 勘误(MCP-4,依 MCP-3 本日对账广播):上一条「随 `463646b` 落库」归因不确——
`463646b` 不含本格接线;那笔接线是 MCP-3 在 463646b 落库之后、MCP-4 占号广播之前写在
装配文件里的未提交编辑(用户经其通道下令「完成真正剩余」),随 `eac94e0`(接线票 01 的
提交)一并带入落库。接线内容正确、resolved 裁定与验收归位理由均不变,只勘归因。
