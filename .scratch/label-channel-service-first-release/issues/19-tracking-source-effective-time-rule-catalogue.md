# 19 轨迹源有效时间规则目录：没有规则时事实只能待判断，而今天没有地方登记规则

Category: enhancement
Status: resolved——四笔在分支 `mcp5-lc19-21`（基线 `a17bfac`）：`7b84ab5` 领域+目录+登记编排+迁移 0014、`d8e82ee` HTTP 登记口、`8793db1` parcel-api 装配、`12a733c` 机制清点；回填问题答复见「完成记录」；待 MCP-1 重放进 main，票面记的是分支上的 SHA
Blocked by: 无（`16` 已 resolved）

## 缺口

ADR-0102 决定三：有效时间由所有者显式判断，来源只有两种——就这一条显式给出，或依据该源**已登记并带版本**的
规则形成。票 `16` 把第二种做成端口 `ports.EffectiveTimeRules`，执行器无规则即把事实留为「有效时间待判断」、
不提供给 `visibility-exception`。**该端口没有生产实现**（机制清点「缺」名单已列），于是今天每一条被认领的外部
轨迹都停在待判断，客户可见面上是空白——ADR-0102 Consequences 说这是真话，但真话要有条出路。

规则本身是实例参数（`PAR-INT-02`，与账号、地址、状态词表同列）；**规则目录的机制**是产品的。

## 做什么

1. 定「有效时间规则」在本仓的形状：按（租户，轨迹源）登记、带版本、只追加；一版规则至少要能表达
   ADR-0102 Consequences 点名的那道判断——「该源报来的时间字段是事件发生时间还是源方处理时间」——以及
   有效时间相对哪一个时间、偏移多少。形状不预设太多：先装得下「有效时间＝发生时间」与「有效时间＝接收时间」
   两种最朴素的规则，且**两种都是显式登记的结果，不是默认值**。
2. `adapters/postgres` 落目录表与迁移，真库实跑；实现 `ports.EffectiveTimeRules`：有在用版本 →
   `EffectiveTimeRuleApplied` 带规则引用与版本；没有 → `EffectiveTimeRuleAbsent`。
3. 登记写面随实施票按 ADR-0101 裁。
4. **回填**：规则登记之后，此前留为待判断的事实怎么补判——是逐条走 `JudgeEffectiveTimeHandler` 的显式判断，
   还是新开一条「按新登记的规则重判该源全部待判断事实」的批量入口。本票要正面答这一问；答「不回填」也行，但要
   写明理由。

## 红线

- **没有规则时必须答 `EffectiveTimeRuleAbsent`**，任何实现都不得把「等于发生时间」当缺省（ADR-0102
  Alternatives 第二条否决的正是它）。
- 规则不解释状态词。状态词的含义是另一份登记（里程碑映射登记册的事，见 ADR-0102 Consequences 第四条）。
- 不填任何一家源的真实规则取值。

## 完成判据

目录有实现与真库测试；`EffectiveTimeRules` 有生产实现且机制清点「缺」名单里该口消失；`16` 的用例矩阵里
「有规则→认领并判断→交 VE」一格能在真实现上复现；回填问题有明确答复。`gofmt -l` 空、`go build`/`go vet`
退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

ADR-0102 决定三与 Consequences；票 `16` 完成记录「刻意留下的三格」第 2 条；
`internal/transportfulfillment/ports/external_tracking_fact.go` 的 `EffectiveTimeRules` 与
`EffectiveTimeRuleInput`；`domain.EffectiveTimeRuleReference`（规则引用必须带版本）。

## 完成记录（2026-09-04，MCP-5 起笔、通道 6 收口；四笔均在分支 `mcp5-lc19-21`，待 MCP-1 重放进 main）

| 笔 | 内容 |
|---|---|
| `7b84ab5` | 领域：`EffectiveTimeRule` 一源一链、一版三件正文（`SourceTimeMeaning` ∈ {`EVENT_OCCURRENCE`, `SOURCE_PROCESSING`}、`EffectiveTimeAnchor` ∈ {`OCCURRED_AT`, `RECEIVED_AT`}、`Offset` 可零可负），换版回指前版、只追加；`ports.EffectiveTimeRuleRegistry`；postgres `EffectiveTimeRuleCatalogue` 兼 `ports.EffectiveTimeRules` 生产实现（无当前版答 `EffectiveTimeRuleAbsent`，有则 `EffectiveTimeRuleApplied` 带规则引用与版本）；`RegisterEffectiveTimeRuleHandler`（首登 / 换版 / 重放 / 内容冲突 / 未受理 / 未决六格）；迁移 `0014_effective_time_rule.sql` |
| `d8e82ee` | HTTP 登记口 `NewRegisterEffectiveTimeRuleEndpoint`：逐字段表单载荷（ADR-0101 决定八自裁形态：一源一链、低频）、严格解码（未知键与 `tenant` 键拒）、身份只来自 Intake 的操作者信封；`UnconfiguredIntake` 加同名格。MCP-5 在途件原样成笔 |
| `8793db1` | `cmd/parcel-api`：`/transport-fulfillment-effective-time-rule-registrations` 接真编排（事务边界归装配点）、探针行、unwired 占位；真库装配测试钉首登 / 重放 / 换版回指。通道 6 前会话在途件原样成笔 |
| `12a733c` | 机制清点在 `8793db1` 干净检出上重生成，`transportfulfillment.EffectiveTimeRules` 从基线口径与精确口径两份「缺」名单消失 |

**完成判据逐条**：目录有实现与真库测试——`effective_time_rule_catalogue_test.go` 九用例带 DSN 全 PASS；`EffectiveTimeRules` 有生产实现且清点「缺」名单里该口消失——`12a733c`；`16` 矩阵「有规则→认领并判断→交 VE」一格在真实现上复现——`TestTheAdoptionExecutorJudgesByTheRealRuleAndHandsOff`（真库，PASS）；回填问题——见下节；`gofmt -l` 空、`go build ./...` / `go vet` 退 0，无 DSN `go test -count=1 ./...` 全仓绿，带 DSN `-v` 于 `adapters/postgres` + `cmd/parcel-api` + `migrations` 190 PASS / 0 SKIP（本机门禁容器 55432）。

### 回填：显式触发的批量重判入口，不随登记自动发生；本票不做，立票 [`22`](./22-rejudge-pending-facts-by-registered-rule.md) 承接

规则登记之后，此前留为待判断的事实**按已登记规则批量重判**——每条走 `domain.JudgeEffectiveTimeByRule` 形成 `EffectiveTimeJudgedByRule` 基准、`ExternalCarrierTrackingFact.JudgeEffectiveTime` 换新版本回指前版、再交 VE，与收编时的规则路径是同一条判断逻辑。不是逐条走票 `21` 的显式判断：那是所有者「就这一条给出」的路，拿它替规则干活等于把 N 次人工判断当回填。

但它是**一条独立的、显式触发的入口**，不在登记同事务里自动发生。三条理由：

1. 登记是一次写，回填是 N 次换版加 N 次交接。并进同一事务会把一次配置动作的失败面变成 N 条事实的失败面，且失败时登记本身也被回滚——「规则登不上是因为某条历史事实交接失败」这个因果没人能解释。
2. 规则换版同样触发回填。若登记即自动重判，一次登错的版本会立刻把该源全部待判断事实改成错的有效时间并交给 VE 投影；显式触发让操作者先在票 `21` 的待判断读面上核对，再批量。
3. 今天生产上待判断事实集合恒空：`TrackingSource` 无实现，拉取节拍随第一家真源立（票 `20`）。回填入口此刻没有真实工作面，与 `20` 同批落最省；它需要的「按（租户，源）列待判断当前版」读口正是票 `21` 读面的端口。

能力边界：本答复读了 `adopt_tracking_material.go` 的规则分支、`judge_external_tracking_effective_time.go`、`external_carrier_tracking_fact.go` 的判断基准三格与 `external_tracking_rehydration.go`；**未读** VE 侧消费者对同一事实多个版本短时间先后到达的处理——回填会让 VE 在一批里收到同源大量新版本，那一侧是否有序无关是票 `22` 开工前要核的一问，已写进该票。
