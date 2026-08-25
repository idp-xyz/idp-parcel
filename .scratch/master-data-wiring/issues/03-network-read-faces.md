# 03 network-routing 读面:网络目录 + 服务区域

Category: feature
Status: in-progress
Owner: MCP-4
Blocked by: 01(已 resolved 即不阻)

给管理台两页供数:network-catalog(网络目录)、service-areas(服务区域与覆盖)。
四件套的前三件,装配件(cmd/parcel-api)**不在本票**,归票 06 / MCP-2。

## 范围(写入足迹)

只写 `internal/networkrouting/` 下:

1. **domain**:租户级运营查阅作用域类型(形状按 ADR-0076 Decision 二、通例按 ADR-0077)。
2. **ports**:网络目录七族(node/connection/line/service-area/service-calendar/
   availability-adjustment/route-strategy,与 0008 七张版本表一一对应,族清单以
   `cmd/parcel-network-register` 的 kind 封闭集为准)的伴生列表读端口,租户在签名上;
   不拓宽既有 NetworkCatalog 写口接口。
3. **adapters/postgres**:读适配器,按族列版本行;真库测试至少覆盖「跨租户不可见」
   「空租户答空列表」「limit 边界」「族间不串」。
4. **adapters/http**:查询端点,建议单端点 GET /network-catalog 以 ?family= 必填分派
   七族(先例:tracking 端点一口三分派),未知族按坏请求拒在 Intake 之前;
   UnconfiguredIntake 同款;httptest 测试。

## 边界(本票特有)

- **0007 network_definition 登记册无写入方**(两表合流判给 ADR-0068 Decision 六的
  解析层设计),本票不读它、不试图给它造读面。service-areas 页读的是 0008 的
  service-area 族版本骨架。
- 服务区域地理覆盖、日历内容、策略规则正文的列还不存在(PAR-NET-14),读面只透版本
  骨架字段(code/version/生效区间等,以迁移 SQL 实际列为准),不发明内容列。

## 先读

spec.md → 267cb44 diff → ADR-0077/0076/0055/0068 →
`docs/domain/network-routing/CONTEXT.md` 查阅语义(上列字段用 CONTEXT 原词)→
0008 迁移 SQL 与 `cmd/parcel-network-register/main.go` 七族 payload 形状。

## 完成标准

同票 02:build/vet 零告警、本上下文 `-count=1` 全绿且真库 `-v` PASS、自己提交、
票面 resolved + SHA、回频道 2。

## Comments
