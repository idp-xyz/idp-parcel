# `case_requirement_rule` 有读口无写口，挡的是建案那一侧的墙

Category: enhancement
Status: needs-triage

从 [SYN-WALL-DOOR-AUDIT 票 06](../../syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md)（清单 W13）执行中分出。W13 点名五本册子，`case_requirement_rule` 是同一批迁移里的**第六本**，形状与那五本完全同类——有表、有只读视图、非测试代码零 `INSERT`、无登记用例——但它挡的墙不在 W13 的两堵之内，因此 W13 不扩，照实另立本票。

## 墙在哪：`EstablishCaseUndecided`，不是 W13 的那两堵

W13 的两堵墙是 `submit_declaration.go` 的 `DECLARATION_UNDECIDED` 与 `receive_external_result.go` 的 `RESULT_UNDECIDED`。这一本挡的是第三处，在 `application/establish_customs_case.go` 的 `EstablishCustomsCaseHandler.Handle` 里：

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

## 待 triage 的一格

`case_requirement_rule` 同样没有版本维（无法定生效区间、无适用时点列），而它也是一条**关务规则**。CONTEXT 硬句 191 适不适用于它，与
[解释规则版本维那票](../../cc-interpretation-rule-version-dimension/issues/01-interpretation-rule-has-no-version-dimension.md)
是同一个模型问题的两个实例，两票一并裁比分开裁省事。本票不预判。

## 本票不做的事

- 不扩 W13。那票已按「只做写口与仓储半边、五类配置」收窄执行中。
- 不替 `customs-compliance` 定所有权。
