# NO 收寄登记接线

Category: enhancement
Status: ready-for-agent

阻塞条件(非票号):工作树里 MCP-3 未提交批次先落提交,见父规格「排序约束」。

## 要做什么

把 `POST /node-operations/receptions` 背后的 `unwiredReception` 换成真编排:新建 `cmd/parcel-api/assemble_reception.go`,构造 `nodeopsapp.NewReceiveDeliveredUnitHandler` 并由 `main` 交入 `assembleBusinessEndpoints`。

## 依赖缝逐条

`ReceiveDeliveredUnitDeps` 的五条:

- `Receptions`(ports.ReceptionStore)— NO postgres 收寄库已存在,接真;
- `Versions`(ports.IntakeIdentityFactory)— 标识签发类端口已清零缺口,接既有实现;
- `Downstream`(ports.NodeIntakeHandoff)— 接 NO 的 Outbox handoff 适配器(发布意图与收寄同一提交);
- `Identity`(ports.ParcelIdentityView)— 跨上下文身份视图。先查端口盘点与 NO adapters 现状:有生产适配器则接真;没有则接显式未配置,让身份核对如实走「待识别」分支,不造采信映射;
- `Clock` — 生产时钟。

## 验收

- 装配测试对真库实跑(缺 DSN 诚实跳过,CI 有 DSN 必跑);
- 端点表测试仍与装配点互为对照;
- 未配置 Intake 仍拒在编排之前(端点行为对外不变,变化只在被调到时的答案来源);
- gofmt / vet / 全仓 test 绿;本票单独成 commit。
