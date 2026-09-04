# 19 轨迹源有效时间规则目录：没有规则时事实只能待判断，而今天没有地方登记规则

Category: enhancement
Status: in-progress——MCP-5（2026-09-04，基线 `a17bfac`，隔离 worktree 分支 `mcp5-lc19-21`）
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
