# 04 customs-compliance 读面:合规规则库

Category: feature
Status: resolved
Owner: MCP-5(2026-08-25 10:57 认领,同日收口)
Resolution: 代码 2d5c8ff。全仓 go build 与本上下文 go vet 零告警,gofmt 无输出;
go test -count=1 ./internal/customscompliance/... 全绿(带 DSN 真库实跑,非 SKIP),
新增真库四用例 -v 下逐一 PASS(空租户空列表/键序与跨租户不可见/版本区间读回/limit 门禁)。
Blocked by: 01(已 resolved 即不阻)

给管理台一页供数:compliance-rules(合规规则库)。四件套的前三件,装配件不在本票,
归票 06 / MCP-2。

## 范围(写入足迹)

只写 `internal/customscompliance/` 下:

1. **domain**:租户级运营查阅作用域类型(形状按 ADR-0076 Decision 二、通例按 ADR-0077)。
2. **ports**:规则与配置登记册的伴生列表读端口,租户在签名上。**上列哪几本册子先做
   词汇对照**:页面出处是 CONTEXT「禁限运、归类、原产地、申报价值、监管凭证适用性等
   规则化合规判断及其规则版本、依据和决定方式」——案件要求规则(case requirement rule,
   W13)与解释规则(interpretation rule,ADR-0070)是规则库正身;五配置登记册里
   就绪/提交授权/关闭义务/门禁条件四本是**按申报单元的运行态**,更贴关务案件页(A 批),
   若你对照后判定它们不属本页,票面记下判断即可,不必硬塞。以 CONTEXT 与 ADR-0070
   为准,存疑回频道 2 问 MCP-2。
3. **adapters/postgres**:对应读适配器 + 真库测试(跨租户不可见/空租户空列表/limit)。
4. **adapters/http**:查询端点,建议 GET /customs-compliance-rules 以 ?registry= 分派
   (封闭集,未知值坏请求);UnconfiguredIntake 同款;httptest 测试。

## 先读

spec.md → 267cb44 diff → ADR-0077/0076/0055/0070 →
`docs/domain/customs-compliance/CONTEXT.md` 查阅语义 → 相关迁移 SQL 与
`cmd/parcel-customs-register` 的 kind 与 payload 形状、
`internal/customscompliance/application/register_case_requirement_rule.go`。

## 完成标准

同票 02:build/vet 零告警、本上下文 `-count=1` 全绿且真库 `-v` PASS、自己提交、
票面 resolved + SHA + 上列册子的对照结论、回频道 2。

## Comments

**词汇对照结论(2026-08-25,MCP-5)**——上列两本,不上列四本:

- **上列**:案件要求规则(`case_requirement_rule`,键:租户+辖区+方向+程序)与解释规则
  (`interpretation_rule`,键:租户+结果层+辖区+生效起点,ADR-0070 问一甲的版本化登记面)。
  两本按监管维度立键、登记的是规则内容与依据,ADR-0070 Decision 四称两者同为关务规则
  ——规则库正身,正对页面出处那句「规则化合规判断及其规则版本、依据和决定方式」。
- **不上列**:就绪(`readiness_judgment`,键:租户+申报单元)、提交授权
  (`submission_authority`,键:租户+申报单元)、关闭义务(`closure_obligation_catalog/item`,
  键:租户+案件)、门禁条件(`gate_condition_catalog/finding`,键:租户+范围+动作+边界)。
  四本按申报单元/案件/决定范围立键;CONTEXT 词条把它们定义为对明确申报范围或案件的
  判断与授权(「申报就绪判断」「提交授权」词条均以明确申报范围为对象),是案件处理的
  运行态,贴关务案件页(A 批),不属规则库页。硬塞进来会把「规则内容」与「某单元此刻
  的状态」装进同一个口,正是 ADR-0070 分记录侧/选择侧要拆开的那类混装。

**给票 06/07 的装配契约**:端点 `GET /customs-compliance-rules`,按 `?registry=` 封闭集
分派(`case-requirement` / `interpretation`,缺席与未知值 400,拒在 Intake 之前);
outcome 两格 `CASE_REQUIREMENT_RULES_LISTED` / `INTERPRETATION_RULES_LISTED`,空册答
`rules: []` 走 2xx;未配置一律 403 `ACCESS_CHANNEL_NOT_CONFIGURED`。装配件:
`postgres.NewRuleCatalogue(db)` + `customshttp.NewQueryComplianceRulesEndpoint(intake, reader)`,
`customshttp.UnconfiguredIntake{}` 已实现 `CatalogueQueryIntake`。行体字段:建案要求规则
`jurisdiction/direction/procedure/required/basis`;解释规则 `layer/jurisdiction/rule/
appliesFrom/appliesUntil(开放版缺席)`。
