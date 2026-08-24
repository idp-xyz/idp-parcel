# `case_requirement_rule` 有读口无写口，挡的是建案那一侧的墙

Category: enhancement
Status: resolved

从 [SYN-WALL-DOOR-AUDIT 票 06](../../syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md)（清单 W13）执行中分出。W13 点名五本册子，`case_requirement_rule` 是同一批迁移里的**第六本**，形状与那五本完全同类——有表、有只读视图、非测试代码零 `INSERT`、无登记用例——但它挡的墙不在 W13 的两堵之内，因此 W13 不扩，照实另立本票。

## 墙在哪：`EstablishCaseUndecided`，不是 W13 的那两堵

W13 的两堵墙是 `submit_declaration.go` 的 `DECLARATION_UNDECIDED` 与 `receive_external_result.go` 的 `RESULT_UNDECIDED`。这一本挡的是第三处，在 `application/establish_customs_case.go` 的 `EstablishCaseHandler.Handle` 里：

```go
judgment, configured, err := handler.deps.Requirement.JudgeCaseRequirement(
    ctx, command.TenantID, command.Jurisdiction, command.Direction, command.Procedure)
if err != nil || !configured {
    return EstablishCaseResult{outcome: EstablishCaseUndecided}, nil
}
```

空册即 `configured=false`，于是每一次建案请求都停在 `UNDECIDED`。**关务案件链的第一步就进不去**，W13 那五本册子登齐了也一样——那五本判的是已有案件之上的申报与结果。

## 现状：与 W13 同形

| | 状态 |
|---|---|
| 表 | 在——`migrations/customs_compliance/0007_interpretation_and_case_requirement_rules.sql`，主键四维（`tenant_id`、`jurisdiction_ref`、`direction`、`procedure_ref`） |
| 读口 | 在——`adapters/postgres/case_requirement_view.go` 的 `JudgeCaseRequirement`，实现 `ports.CaseRequirementView` |
| 写入方 | 零——非测试代码无 `INSERT`，只出现在 `case_requirement_view_test.go` |
| 登记口 | 零——`application/` 无登记用例 |

## 已经定死的边界，不要在本票里重开

- **`found=false` 是「规则未登记」，不是「不要求建案」。** 迁移与读口两处注释都写死了这条分界，`basis` 列 `NOT NULL` 是它的守门人：登记为「不要求」的那一行也说得出依据，于是「答否」与「没答」在数据上分得开，两者的续办动作完全不同（前者照常推进，后者等实例参数）。写口不得引入任何把未登记折成「不要求」的路径。
- 登记不可覆盖，沿用 W13 的写口纪律：`INSERT ... ON CONFLICT DO NOTHING`，同键已在册交回`已登记`，内容比对交给编排读回既有登记自己做。
- 规则正文属实例半边（PAR-CUS-01..07 实例值待提供是常态），登记册本身属机制半边；验证用脱敏合成配置，S 级只记 S。

## 那一格已裁：版本维移出本票范围

[ADR-0070](../../../docs/adr/0070-customs-rule-registries-split-recording-from-selection.md)（草案）裁了原列在此处的待 triage 一格。结论分两半：

- **版本维悬置。** `case_requirement_rule` 缺法定生效区间与适用时点两列，是否发作取决于建案会不会为过去的法律行为迟到发生——与[解释规则版本维那票](../../cc-interpretation-rule-version-dimension/issues/01-interpretation-rule-has-no-version-dimension.md)缺的是同一个模型决定，随那票一并悬置，**显式移出本票范围**。
- **写口不受它阻塞。** 写口属机制半边，上节那条 `found=false` 分界与不可覆盖纪律一条也不依赖版本维的裁决，因此本票转 `ready-for-agent` 先做。

另记一条 ADR-0070 顺带核出的事实，免得后来人当成本票的漏项：`domain.CustomsCase` 不存所采用的要求规则，`Basis()` 也只在「不适用」格给出。CONTEXT 对建案**没有**下过记录规则依据的义务（「案件身份与申报范围」一节只要求固定四维监管范围），所以这不是违规，也不在本票内补。

## 本票不做的事

- 不扩 W13。那票已按「只做写口与仓储半边、五类配置」收窄执行中。
- 不替 `customs-compliance` 定所有权。

## Comments

- 2026-08-24 · MCP-1：**三件交付，本票转 resolved。**
  * 写入方：`adapters/postgres/case_requirement_registry.go`——`ON CONFLICT DO NOTHING`
    不可覆盖、`RequireExecutor` 无环境事务即拒、集合外方向是错误不是未配置；真库用例
    四发（穿读口往返、不可覆盖、无事务拒、坏方向拒），「答否」也带依据落册那一格由
    往返用例钉住。
  * 登记用例：`application/register_case_requirement_rule.go`——受理门逐格拒（租户/
    辖区/方向/程序/依据，「不要求」也必须带依据）；冲突分界在编排：写口答`已登记`后
    读回逐字段比，同则`已存在`、异则`内容冲突`（required 翻面与换依据都算），绝不
    顶替；依赖故障折未决。独立 handler，不并入 W13 那五本的 handler（那票已按五类
    收窄结案）。
  * 进程级入口：并入 `cmd/parcel-customs-register` 为第十命令 `case-requirement`
    （票 06 B 半边刚落的同一口，四件就此齐全）。译装用 `*bool` 分辨「没给」与
    「给了 false」——required 缺席即拒，缺格不得静默落成「不要求建案」；方向封闭
    二向指名拒。
  * `found=false` 分界零触碰：读口原样，未登记仍是未决；写口没有任何把未登记折成
    「不要求」的路径。版本维照 ADR-0070 悬置，本票未动 0007 表结构。
  * 验证：写口/用例/CLI 三层新增 15 用例全绿；共享树全量 gofmt/build/vet 零信号、
    `go test -p 1 -count=1 ./...` 75 包 ok、0 FAIL（2026-08-24 12:53，真库 33.5s 实跑）；
    隔离树按提交 SHA 的全量验证随提交完成并记于提交信。
