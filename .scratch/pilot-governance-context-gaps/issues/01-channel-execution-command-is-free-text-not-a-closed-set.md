# `channel_execution.command` 是自由文本——受控通道的命令集合只在 CLI 的字符串常量里封闭，库与领域都没有那个集合

Category: enhancement
Status: resolved
Blocked by: 无

本票**只写量到的事实与要裁的**，不写代码、不选形状。

## 事实（钉在 main `dc7d44e9`）

**库上：文本列，只拦空白。**

- `migrations/pilot_governance/0004_channel_execution.sql`：`command text NOT NULL`，唯一约束是
  `channel_execution_not_blank`（`btrim(command) <> ''` 与另四列同一条 CHECK）；没有 `CHECK (command IN (…))`，
  没有枚举类型，没有外键。`outcome` 列同样是只拦空白的文本。
- 头注写明设计意图：「受控通道执行留痕：治理登记身份双轨的第①轨」「只追加：每次执行各留一行……无 UPDATE
  路径，也不设幂等约束」。留痕的**事实性**（几次、谁、何时）写得很清楚，命令**取值**属不属封闭集一字未提。

**领域侧：没有类型。**

- `internal/pilotgovernance/domain` 里没有任何与通道执行或命令对应的类型；本上下文的封闭集都是 `uint8` iota
  枚举配 `String()`（`ExecutionStage`、`ReviewVerdict`、`NoGoDisposition`、`ScopeVersionRelationKind`、
  `RelationComparison`、`AdmissionSuspensionGround`），留痕这一件不在其中。
- 留痕的结构体在持久化适配器里：`internal/pilotgovernance/adapters/postgres/channel_executions.go` 的
  `ChannelExecution{Command string; RecordReference string; OSUser string; Hostname string; Outcome string; ExecutedAt}`，
  `complete()` 只核六件非空；`ChannelExecutions.Append` 走 `RequireExecutor`、原样 INSERT。它的头注把自己定位成
  「不是治理记录……这里一个内容字段都没有」。

**写入方与实际出现过的取值。**

- 唯一生产写入方是 `cmd/parcel-governance-register`：`traceExecution` 把 `command` 原样落列，而 `command` 是
  `args[0]`，在入口处按 `switch command { case commandAuthorityInterval, commandSuspend, commandResume: }` 校过——
  三个常量的值是 `"authority-interval"`、`"suspend"`、`"resume"`（同文件 const 块；头注写「首批只开……阶段评审
  与接管第二批」）。**所以集合今天是封闭的，但封闭在 CLI 的字符串常量与一个 switch 上**：换一个写入方、或直接
  写表，什么都拦不住。
- `outcome` 列的取值来自 `application` 层各结果枚举的 `String()`（`RegisterAuthorityIntervalResult.Outcome()`、
  `GovernIncidentResult.Outcome()`），且只在 `tracedIncidentOutcome` 圈定的「落册 / 已在册」四格与区间登记的
  两格时才落痕——集合在应用层封闭，到列上又成自由文本。
- 测试里出现过的取值：`internal/pilotgovernance/adapters/postgres/channel_executions_test.go` 用
  `Command: "suspend"`、`Outcome: "SUSPENSION_RECORDED"`；`cmd/parcel-governance-register/vertical_test.go` 经真
  CLI 跑三个子命令。**本票只量到这两处**，没有逐一核对 `vertical_test.go` 断言了哪些列值。

**同形的兄弟。**

- `migrations/visibility_exception/0020_channel_execution.sql` + `internal/visibilityexception/adapters/postgres/channel_executions.go`
  （票 12 双轨「经票 15 沿用」）是同一形状的复制品，写入方 `cmd/parcel-ve-register`。`cmd/parcel-customs-register/main.go`
  头注明写「不留 channel_execution 痕：那是票 12 对治理登记面的裁决，其留痕册属 pilot-governance」——它**没有**
  第三份拷贝。所以同形的是两份：pilotgovernance 与 visibilityexception；本票只量前一份，裁完之后后一份要不要跟，
  写在「开工前置」。

## 与本仓封闭集通例的差距

- 通例一：封闭集在领域包里是 `uint8` iota 枚举配 `String()`，`internal/architecture/enum_exhaustiveness_test.go`
  的 `TestEveryEnumConstantIsNamedByItsStringMethod` 守「新增取值必须同时补 `String()`」；消费侧 switch 不留
  `default` 吸收（ADR-0025 全函数、ADR-0031 封闭代数一族）。留痕的命令集合不在任何枚举里，门禁扫不到它。
- 通例二：库上 CHECK 是领域封闭集的「第二道镜像，不是唯一一道」（`0014_acceptance_rule_package.sql`、`0023` 一族
  的写法）。这里连第一道都没有，第二道自然也没有。
- 差距不是「少一个 CHECK」，是**这个集合归谁定**没有人说过：它是产品的受控通道命令集合（机制半边，封闭，
  随 CLI 子命令增删走迁移），还是租户可以自己扩的操作词汇（实例半边，列上只拦空白是对的）？两种读法下正确的
  形状相反。

## 可能的形状（列出，不选）

1. **机制半边**：领域包加 `ChannelCommand` 封闭枚举配 `String()`（进 enum 门禁），`ChannelExecution.Command`
   改用它；新迁移给 `command` 列加 `CHECK (command IN (…))` 作第二道镜像；第二批子命令（阶段评审、接管）进来
   时放宽 CHECK 走新迁移。`outcome` 列要不要同样收紧，取决于它引用的是哪几个应用层枚举的 `String()`。
2. **实例半边**：保持文本列，但把「为什么不封闭」写进迁移头注与适配器注释——今天两处都没说，读的人分不出
   是没想到还是有意的。
3. **折中**：列保持文本，领域侧只加一个「已知命令名」登记函数供 CLI 与测试共用，不加 CHECK——这实际上是把
   今天 CLI 常量搬一层，封闭性仍靠约定。

## 开工前置

- **先裁命令集是机制半边还是租户可扩。** 判据线索：CLI 头注说「首批只开三个、第二批阶段评审与接管」——那是
  产品按批开放的子命令，读起来像机制半边；但留痕表本意是「记事实」，事实列上加封闭集会让「一次未知命令的
  执行」变成写不进去而不是被记下来，这一格要 owner 说要哪一个。
- 裁定为机制半边时，是否值一篇 ADR：涉及一张只追加事实表要不要带业务封闭集，与 ADR-0031「写入结果是封闭
  代数」不是同一件事，可能只要在票面记裁决。
- 两份同形拷贝（pilotgovernance 与 visibilityexception 的 `channel_execution`）同改还是各自裁，先说一句；
  两边的命令集合本就不同（VE 那边是 `parcel-ve-register` 的子命令），同改也不是同一个集合。

## 边界

- 不动 `cmd/parcel-governance-register` 的子命令集合本身；不在本票加第二批子命令。
- 不改既有迁移 0004；要加 CHECK 走新迁移。

## Comments

- 2026-09-04 MCP-4：立票（draft），按 MCP-1 派单 task-62e8e262 只读取证。
- 2026-09-05 MCP-1：接手崩溃现场并完成本票。裁定命令集合属产品机制半边，不归租户扩；首批三个
  `parcel-governance-register` 子命令由 `pilotgovernance/domain.ChannelCommand` 封闭枚举拥有，CLI
  只从该集合解析，`channel_execution.command` 再由迁移 0006 加同值 `CHECK` 作库内第二道镜像。
  `outcome` 继续保持文本，因为它承载应用层多个结果枚举，不随命令集合一一对应。两份同形留痕表不合并：
  `visibility-exception` 的命令集合由其自己的登记入口拥有，留待该上下文单独裁定。
- 实施落点：新增 `internal/pilotgovernance/domain/channel_command.go` 及领域测试；将治理留痕适配器和
  CLI 改用 `ChannelCommand`；补齐 CLI/事务守卫测试调用面；新增
  `migrations/pilot_governance/0006_channel_execution_command_closed.sql`，并加入适配器旁路写入的双侧
  封闭测试。
- 验证：`gofmt`、`go vet ./...`、`go test -p 1 -count=1 ./...` 均通过；本机未设置
  `IDP_PARCEL_POSTGRES_DSN`，所有 PostgreSQL 集成用例按既有约定跳过，未将其记为真实库证据。
