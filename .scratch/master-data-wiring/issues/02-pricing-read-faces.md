# 02 parcel-pricing 读面:价卡目录 + 计价参考序列

Category: feature
Status: resolved
Owner: MCP-6(接手;原主 MCP-3 认领 b6c7663 后 crash、零代码遗留,用户 2026-08-25 经队列 6 指示接续)
Blocked by: 01(已 resolved 即不阻)

给管理台两页供数:price-card-catalog(价卡目录)、reference-series(计价参考序列)。
四件套的前三件,装配件(cmd/parcel-api)**不在本票**,归票 06 / MCP-2。

## 范围(写入足迹)

只写 `internal/parcelpricing/` 下:

1. **domain**:租户级运营查阅作用域类型(形状按 ADR-0076 Decision 二、通例按 ADR-0077;
   参照 `internal/visibilityexception/domain/operations_query_scope.go`,类型名自定但
   语义同款:作用域引用+租户两维非零,无客户维)。
2. **ports**:价卡目录与参考序列的伴生列表读端口(租户在签名上;不拓宽既有登记写口
   接口——扩写侧接口会拆写侧替身,267cb44 提交信记过这条风险)。
3. **adapters/postgres**:读适配器实现,读 0002 的 price_card_version 与 0003 的
   reference_series_version 两张表(表结构以迁移 SQL 为准;两处表名都与迁移文件名不同,
   按表名写);真库测试
   (internal/platform/pgtest 先例见 visibilityexception 的 projection_list_test.go):
   至少覆盖「跨租户不可见」「空租户答空列表」「limit 边界」。
4. **adapters/http**:查询端点(参照
   `internal/visibilityexception/adapters/http/query_tracking_projections.go`):
   建议路径 GET /pricing-price-cards 与 GET /pricing-reference-series(最终路径归装配票,
   handler 构造器暴露即可);UnconfiguredIntake 同款(未配置一律 403
   ACCESS_CHANNEL_NOT_CONFIGURED,不读内容);outcome 按运营语义分格(列表成格 +
   坏请求),空目录如实答空;httptest 传输层测试。

## 先读

spec.md(共同纪律、深度上限)→ 267cb44 的 diff(四件套形状)→ ADR-0077/0076/0055 →
`docs/domain/parcel-pricing/CONTEXT.md` 里价卡目录与参考序列的查阅语义(哪些维度上列,
词汇对照;上列字段用 CONTEXT 原词)→ 迁移 SQL 与
`cmd/parcel-pricing-register` 的登记行形状(读面字段与登记字段对齐,不发明列)。

## 完成标准

`go build ./...`、`go vet ./...` 零告警;`go test -count=1` 本上下文全绿且真库用例
`-v` 下 PASS;自己提交(不含 cmd/parcel-api 改动),票面记 resolved + SHA + 哪种绿,
回频道 2 报告(格式见 spec 共同纪律)。

## Comments

- 2026-08-25 13:02 MCP-3 代收(经用户通道 3 授权调度本轮):实现由 MCP-6 完成于
  f4dad52(11:33),四件套前三件十文件全落(domain 作用域+测试、ports 两读口、
  postgres 读适配器+真库测试、http 两端点+UnconfiguredIntake+httptest)。MCP-3 于
  11:53-11:55 独立验证提交态(树净,工作树即 HEAD 5ad31bf):`go build ./...` 与
  `go vet ./...` 零告警;`go test -count=1 ./internal/parcelpricing/...` 全 ok;
  三个目录读面真库用例(TestPriceCardCatalogueTranscribesTheColumnFace /
  TestReferenceSeriesCatalogueTranscribesTheColumnFace /
  TestPricingCatalogueAppliesTheLimitAndRejectsNonPositive)在 DSN 已设下 `-v` 全
  `--- PASS` 非 SKIP——**绿(含真库)**;证据已于 12:00 前后发频道 2。MCP-6 自
  f4dad52 后无收口动作、截至 13:02 未响应,按授权代记 resolved,其实现一字未动。
