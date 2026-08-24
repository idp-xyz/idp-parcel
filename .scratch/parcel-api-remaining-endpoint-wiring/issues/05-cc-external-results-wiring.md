# CC 外部结果接收接线

Category: enhancement
Status: ready-for-agent

阻塞条件(非票号):工作树里 MCP-3 未提交批次先落提交,见父规格「排序约束」。

## 要做什么

把 `POST /customs/external-results` 背后的 `unwiredResults` 换成真编排:新建 `cmd/parcel-api/assemble_external_results.go`,构造 `customsapp.NewReceiveExternalResultHandler`。

## 依赖缝逐条

`ReceiveExternalResultDeps` 的五条:

- `Results`(ports.ExternalResultStore)— CC postgres 外部结果登记册已存在(留存不猜形状入 CHECK),接真;
- `Submissions`(ports.SubmissionIndex)— 提交索引读面,查 CC postgres 现状后接真(编排对找不到原提交本就有「留存不猜」分支,不需要造索引);
- `Rules`(ports.InterpretationRuleView)— 解释规则视图。编排自带「解释规则未配置→未决」分支,登记册未就位就接显式未配置,让它走这条真实分支;
- `Downstream`(ports.ExternalResultHandoff)— 接 CC 的 Outbox handoff(意图投递失败不翻结果,重放重发同一份);
- `Clock` — 生产时钟。

## 验收

同票 01;另加:装配测试钉「解释规则未配置」分支如实未决、「找不到原提交」分支留存不猜。
