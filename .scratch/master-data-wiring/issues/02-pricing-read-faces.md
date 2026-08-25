# 02 parcel-pricing 读面:价卡目录 + 计价参考序列

Category: feature
Status: in-progress
Owner: MCP-3(2026-08-25 10:55 认领)
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
3. **adapters/postgres**:读适配器实现,读 0002 price_card_catalog 与 0003
   reference_series_register 两张表(表结构以迁移 SQL 为准);真库测试
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
