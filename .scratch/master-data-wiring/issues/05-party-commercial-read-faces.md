# 05 party-commercial 读面:服务产品 + 商业规则与策略

Category: feature
Status: in-progress
Owner: MCP-6(已认领)
Blocked by: 01(已 resolved 即不阻)

给管理台两页供数:service-products(服务产品与渠道)、commercial-policies
(商业规则与策略)。四件套的前三件,装配件不在本票,归票 06 / MCP-2。

## 范围(写入足迹)

只写 `internal/partycommercial/` 下:

1. **domain**:租户级运营查阅作用域类型(形状按 ADR-0076 Decision 二、通例按 ADR-0077)。
2. **ports**:两个伴生列表读端口,租户在签名上,不拓宽既有写口:
   - 服务产品:service_product_form(0008)的版本行上列。
   - 商业策略:接单规则包(0014)/接受前财务控制(0007)/价格政策(0010)/
     结算政策(0011)/as-of 政策(0005)——具体哪几张上列、各透哪些字段,先对照
     `docs/domain/party-commercial/CONTEXT.md`「接单规则包、接受前财务控制策略、
     商业价格政策、结算政策与信用政策」的查阅语义;信用政策若无独立表,如实不列,
     票面记下。
3. **adapters/postgres**:读适配器 + 真库测试(跨租户不可见/空租户空列表/limit/
   策略种类间不串)。
4. **adapters/http**:查询端点,建议 GET /commercial-service-products 与
   GET /commercial-policies 以 ?kind= 分派策略种类(封闭集);UnconfiguredIntake 同款;
   httptest 测试。

## 边界(本票特有)

- 发布批次与版本解析(commercial_publication/commercial_version)是发布机制,
  不是本两页的目录正身;若 CONTEXT 对照后你判定某页应上列「已发布版本」而非表单行,
  票面记判断并按它做——以 CONTEXT 为准,存疑回频道 2。
- 渠道产品目录无存储(CommercialObjectKind 封闭集合无此格,spec 已记),不属本票。

## 先读

spec.md → 267cb44 diff → ADR-0077/0076/0055 →
`docs/domain/party-commercial/CONTEXT.md` → 上列各表迁移 SQL 与
`cmd/parcel-commercial` 的发布输入形状。

## 完成标准

同票 02:build/vet 零告警、本上下文 `-count=1` 全绿且真库 `-v` PASS、自己提交、
票面 resolved + SHA + 上列对照结论、回频道 2。

## Comments
