# 05 party-commercial 读面:服务产品 + 商业规则与策略

Category: feature
Status: resolved
Owner: MCP-6
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

**2026-08-25 MCP-6 收口。代码 `ecf268e`,四件套前三件全落,装配件归票 06 未动。**

验证(哪种绿):本上下文 `go vet` 零告警、`go test ./internal/partycommercial/...
-count=1` 全绿,postgres 侧带 DSN 真库实跑,四条目录测试 `-v` 下 PASS 非 SKIP
(版本壳上列/租户隔离/limit/五册不串)。工作树含他会话在途文件(internal/architecture
未跟踪测试撞符号),全仓门禁改在临时 worktree 按 `ecf268e` 提交态复验:`go build ./...`
与 `go vet ./...` 零告警;验证树用毕已按纪律拆除(不加 --force)。

上列对照结论(票面授权的判断,以 CONTEXT 为准):

1. **服务产品页上列「已发布版本」而非表单行**——commercial_version(kind=1)左连
   service_product_form。CONTEXT 把目录对象定义为「服务产品版本:具有独立身份和
   适用范围的商业定义版本」,身份/范围/区间/状态都在版本壳上;ADR-0050 分工是
   版本答「有没有这份产品对象」、形态答「哪种服务形态」,且形态缺席合法——只列
   表单行会让未登形态的已发布产品从目录上消失。0008 迁移自注的装载方向(版本侧
   驱动、形态左连)同派。版本册草稿不入册,故上列的每一行都是已发布之后的状态,
   状态列如实透出(含 PUBLISHED 未生效)。
2. **商业策略页五册全部上列**,`?kind=` 封闭五值各答各的:接单规则包(0014,父子
   聚合出分类规则引用)、接受前财务控制(0007,拥有对象是**客户合同版本**,行内
   标识指名合同,`不适用`依据只在该格携带)、商业价格政策(0010,发布期保全的
   方案方向与转换照列)、结算政策(0011,六维平铺)、时点锚声明(0005,挂接单
   规则包版本,不存时点值故无时点值可列)。
3. **信用政策如实不列**:CommercialObjectKind 有 CreditPolicyObject(版本壳可入册),
   但 migrations/party_commercial 无信用政策正文表,页面无册可上;封闭集里不预留
   空格,正文册落库时按封闭集扩方法。
4. 发布批次/版本解析(commercial_publication 机制)未被当作目录正身:策略五册各读
   自己的正文表,不跨表拼版本壳;服务产品页读版本壳是因为它本身就是 CONTEXT 定义
   的目录对象,不是解析机制的复用。

新增面:domain.OperationsQueryScope(ADR-0077 Decision 二,自立不从 VE 导入);
ports.ServiceProductCatalogueRead + ports.CommercialPolicyCatalogueRead(伴生读口,
不拓宽 PublicationRegistry);postgres.OperationsCatalogue(一适配器实现两口);
commercialhttp 包:GET /commercial-service-products、GET /commercial-policies?kind=,
UnconfiguredIntake 未配置一律 403(PAR-INT-01),空目录 2xx 空数组(ADR-0077
Decision 四),kind 缺席/集外先于 Intake 按坏请求拒。
