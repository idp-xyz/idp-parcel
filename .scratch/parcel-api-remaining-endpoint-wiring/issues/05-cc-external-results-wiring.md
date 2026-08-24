# CC 外部结果接收接线

Category: enhancement
Status: resolved

阻塞条件(非票号):工作树里 MCP-3 未提交批次先落提交,见父规格「排序约束」。
(2026-08-24 认领时核:该批次早已随 `463646b` 落库;本票实际等的是 MCP-5 的解释规则
版本维改造——ReceiveExternalResultDeps 加 Units/Cases 两依赖、读口换带辖区与评估
时点的新签名,其随 `0202a3d` 落库并于 MCP-5 释号广播后开工。)

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

## Comments

2026-08-24 随 `cd84409` 落库(MCP-9,承接 MCP-4 停机后的接线队)。缝数勘正:立票时按
五条列,MCP-5 版本维改造(`0202a3d`)给 Deps 加了 Units 与 Cases 两条辖区回指链读口,
实做七条,全部接真——本票没有「显式未配置」缝:结果登记册、提交索引、版本化解释
规则读口(租户随调用到达,无 VE 票 04 那个租户钉死问题)、单元与案件两读口、Outbox
意图交付、生产时钟。票面「登记册未就位就接显式未配置」的条件分支未触发:登记面已随
版本维改造就位,接真读口后「解释规则未配置→未决」由登记册内容如实作答(实例半边的
留白在册内不在缝上,ADR-0063)。

装配测试 assemble_external_results_test.go 对真库实跑 PASS,两验收钉都在:「找不到
原提交」留存不猜(真索引查过、原始响应带归属不上标记落库、不交意图、重放走已有记录
证事务提交);「解释规则未配置」如实未决(播种提交+单元+案件走通归属/时点/辖区链,
真规则读口对未登记组合答未配置,编排停在指名未决、携续办引用、零落库)。全仓验证按
cd84409 在临时 worktree 检出:gofmt 清、build/vet 退 0、go test -count=1 ./... 零
FAIL(DSN 已设,PG 包实跑非跳过)。

提交方式记一笔:落 cd84409 时暂存区正共居着另一会话的 bento 发射器批次(其冲突刚解、
整批在暂存),裸 commit 会复刻 0202a3d 卷带事故,故用路径限定提交(git commit --
<本票六文件>)只取自己的;对方整批原样留在暂存区,已广播提醒。
