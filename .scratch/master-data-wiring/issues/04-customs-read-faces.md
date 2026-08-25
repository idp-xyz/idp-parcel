# 04 customs-compliance 读面:合规规则库

Category: feature
Status: in-progress
Owner: MCP-5(2026-08-25 10:57 认领)
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
