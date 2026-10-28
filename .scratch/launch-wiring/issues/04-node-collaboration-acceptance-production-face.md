# 04 NO 监管协作承接编排无装配

Category: enhancement
Status: resolved——2026-10-28 进 main（`2e24f5941`，launch-wiring 01–04 同批；全量 160 ok / 0 FAIL，种子退 0），见文末「进 main 记录」。此前 in-progress——2026-10-28 作者完工，待进 main，分支 claude-lw-no（代码最后一笔 6923b87a5，含评审修订）。此前 in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」），分支 claude-lw-no
Blocked by: 无

## 做什么

把 `AcceptCollaborationHandler`（含 ADR-0333 待补充轮次）与执行事实登记接成生产入口（触发方照 UC-NO-001），授予与种子照现有运营决定口写法；生产装配真库用例。

## 完成判据

- 生产路径能承接、部分承接、待补充、拒绝，待补充后补充形成下一轮；待补充事项上登记执行事实被拒（真库用例）。

## 完成记录

- 起点 `2d0193336`；代码最后一笔 `3c7a26bb1`。
- 触发方：UC-NO-001 把承接交给「节点作业责任角色」、现场事实交给「节点现场执行角色」，故不做派发消费者（关务一侧没有协作事项的出站信封，无对端），做两个口：承接 `POST /node-operations/collaboration-acceptances` 挂「运营决定」能力面新增的 `NODE_COLLABORATION_ACCEPTANCE`；执行事实 `POST /node-operations/collaboration-executions` 挂「作业事实登记」能力面新增的事实类型 `NODE_COLLABORATION_EXECUTION`（设备加 NODE 作业范围，铸造侧判准入）。裁决见 ADR-0338。
- 改了什么：accessidentity 加决定种类与作业事实类型；迁移 access_identity/0031（取并行分支 0030 的两个词并入全集）；`RecordExecutionCommand` 加 `Node`，对不上承接决定的节点答未受理；nodeopshttp 两口与封闭载荷；noaccess 的作业事实认证加一方法；`cmd/parcel-api` 新文件 assemble/endpoints 各一对，两口各包事务；种子 `access/28`、`29`、`governance/65`、`66`、`steps/484`（含 README 一段）；ADR-0338；UC-NO-001 状态行与 `AT-NO-133`、`134`；AT 盘点两行加追记。共享装配文件（`endpoints.go`、`main.go`）零改动。
- 代决与越权风险点：见 ADR-0338 文末四点——事项存在与当前有效性不核；操作者不落库（审计只靠 `authority` 引用与请求日志）；承接后没有「作业任务」聚合，执行事实直接依附承接范围；取号。另有两处代决：承接的 `decidedAt` 取提交方填的业务时刻；`RecordExecutionCommand.Node` 必填（旧测试夹具已补）。
- 判据对用例：承接、部分承接、待补充、拒接四格与待补充后下一轮——真库用例 `TestTheNodeCollaborationFacesAcceptSupplementAndRecordExecutionAgainstARealDatabase` 覆盖待补充（201 第 1 轮）、补齐后下一轮（201 第 2 轮，库里两轮并存）、拒接，部分承接在编排单测 `TestOneDecisionPerCollaborationItem` 一侧；待补充事项上登记执行事实被拒（`ITEM_PENDING_SUPPLEMENT`、库里零事实）为真库断言。
- 自验（含 PG）：`go build ./...`、`go vet`、`gofmt -l` 无输出；`go test -count=1 -p 1 ./cmd/parcel-api/... ./internal/architecture/... ./migrations/... ./internal/accessidentity/... ./internal/nodeoperations/...` 全绿；种子灌一次性库退 0（已删库）；`at-coverage-counts -check` 只报生成物过期。
- 留给协调方：（1）进 main 时重生成 ADR 索引、AT 盘点计数、机制清点；（2）与 claude-lw-cc 撞了号：CC 取了迁移 0030、种子 access 26/27、governance 63/64、step 483，本票让位到 0031、28/29、65/66、484；0031 的 CHECK 全集已含 CC 的两个词，CC 若再改词表需同步；（3）不挡上线的发现：关务一侧无协作事项读口与承接/执行意图的消费者（承接意图 `node-operations.collaboration-acceptance.decided` 随决定入 Outbox 但无人消费），冻结清单应记一行。

## 评审修订

隔离评审（钉 `d60e058eb`）两轴无阻断，协调方要求补三项，已做：（1）access_identity 0031 取号不改，头注只讲本迁移加了什么；（2）操作者落库：NO 迁移 `0025` 给承接册加可空 `decided_by`、执行事实册加可空 `recorded_by`，端点从认证信封取「发行方#sub」写入，不进摘要、不回填、读回不带；真库用例断言两列落的操作者；ADR-0338 决定四与越权风险点 2 改现行；（3）ADR-0338 Consequences 补两份意图无订阅者、`dispatch.no_subscriber` 是认下的代价（同 ADR-0202 决定六先例）。自验同前：build、vet、gofmt、含 PG 的受影响包测试全绿。

## 评审记录（2026-10-28，隔离子代理）

- 第一轮（钉 `d60e058eb`）两轴无阻断：0031 CHECK 无漏词；节点防线合 UC-NO-001；无订阅者为响亮失败、每份信封自成分区。待处理：与 `claude-lw-cc` 的 0031 撞号（CC 让到 0032、0033）。协调方另要求操作者落库以与 TF（ADR-0303、ADR-0337）同口径。
- 第二轮（钉 `272f98c15`）：NO 迁移 0025 加 `decided_by`、`recorded_by`，可空不回填；操作者取自认证信封、只进审计不进摘要；ADR-0338 改现行并补无订阅者代价。阻断清零。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 与 launch-wiring 01、02、03 同批：隔离树上先 cherry-pick `claude-lw-cc`（tip `f977c587c`）、再 `claude-lw-no`（tip `272f98c15`）到 main `322b5695c`；`internal/accessidentity/operation_decision.go` 的 `decisionKinds` 文本冲突按并集解。access_identity 迁移 0030（01）、0031（04）、0032（02）、0033（03）依序施加，0033 的 CHECK 为全集（评审逐词核过）。
- 重生成 ADR 索引、AT 盘点计数与机制清点，有变化的各成一笔；两项 `-check` 退 0。
- 全量 `scripts/test-full.sh`：160 ok / 0 FAIL。种子灌一次性库退 0，用后删库。
- 推确切 SHA `2e24f5941` 到 main。
