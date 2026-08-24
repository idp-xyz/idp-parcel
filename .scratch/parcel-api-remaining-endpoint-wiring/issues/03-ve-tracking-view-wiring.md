# VE 客户追踪视图读口接线

Category: enhancement
Status: ready-for-agent

阻塞条件(非票号):工作树里 MCP-3 未提交批次先落提交,见父规格「排序约束」。

## 要做什么

把 `GET /customer-tracking-view` 背后的 `unwiredTrackingViews` 换成真库读适配器:VE postgres `NewCustomerViews` 已存在且带 `FindCurrent`(按租户+账户+包裹取当前视图版本),由 `main` 构造交入装配点。

这一笔与委托查阅读口(已接真)同形:读面不是编排,查阅不触发判断或披露,直接接存储读面。预计是五票里最薄的一笔——若读口方法形状与端点消费的接口有出入,在装配处以显式适配收拢,不改领域包。

## 验收

- 装配测试对真库实跑:至少覆盖「查得到当前版本」「查不到如实 404 统一不可见」「库错误落 5xx NO_ANSWER_FORMED 不伪装成查无」三格;
- 其余同票 01:端点表对照、Intake 仍在前、gofmt / vet / 全仓 test 绿、单独成 commit。
