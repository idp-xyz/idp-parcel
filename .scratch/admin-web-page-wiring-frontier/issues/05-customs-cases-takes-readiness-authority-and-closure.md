# 关务案件页收三类：就绪判断、提交授权、关闭核对

Category: enhancement
Status: ready-for-agent

自[票 04 的导航裁决](./04-registered-but-unreadable-rows-need-a-nav-ruling.md)。三类都归 `customs-cases`（关务案件与申报），页面早在 `moduleInfoById` 的主责句里认领过，缺的只是查阅面。

## 事实（实读代码，锚 `dc611a3`）

1. 表与数据都在，且**带租户维**：`readiness_judgment`、`submission_authority`（0006，主键 `(tenant_id, unit_id)`）、`closure_obligation_catalog` / `closure_obligation_item`（0008，主键含 `case_ref`）。种子 `data/customs/` 灌过就绪、授权、义务目录与两项义务。
2. **点读适配器已在**：`ReadinessView.LoadReadiness`、`SubmissionAuthorityView.LoadSubmissionAuthority`（各按 `(tenant, unit)` 取一条）、`ObligationInventoryView.LoadObligationItems`（按 `(tenant, case, cutoffAt)` 取一份清单）。三个都交回领域对象、都是为编排而设的点读——**不是**目录上列。
3. `customscompliance/adapters/http` 里只有 `query_compliance_rules.go` 一条查阅端点，配套 `unconfigured_intake.go` 与 `isolated_read_intake.go` 都已在。
4. 写入方 `parcel-customs-register` 的三个子命令（`readiness-register`、`authority-grant`、`obligation-catalog` / `obligation-item`）都已在 `seed.sh` 里。

## 要做什么

照 `ADR-0077` 的伴生列表读端口通例（形状样板取 `partycommercial` 的 `CommercialRelationCatalogueRead`，票 01 刚落的那份）：

- `ports` 加**伴生**列表读端口，**不拓宽 `LoadReadiness` / `LoadSubmissionAuthority` / `LoadObligationItems` 三个既有点读口**——扩它们会拆全部编排侧测试替身，而且点读与上列本就不是一个调用面。
- `adapters/postgres` 加列表适配器；关闭义务那一格父子两表要**一条语句取回**（`ReadExecutor` 不保证两条语句同一快照）。
- `adapters/http` 加查询处理器，复用既有 `unconfigured_intake` 与 `isolated_read_intake`，不新立准入。
- `cmd/parcel-api` 装配 + `isolated_read_test.go` 放行面枚举加行（入格判据见票 01 交付节：按 ADR-0078 Decision 一那三条，不改 ADR）。
- `apps/admin-web` 的 `CustomsCasesPage` 接真 + `liveIds` 加一行。

## 三条形状约束（先写下来，接线时容易合掉）

1. **就绪判断与提交授权必须分列两栏，各带自己的撤销态。** CONTEXT 硬句 164「申报就绪判断与提交授权必须独立存在」；0006 迁移刻意分两张表，自注写着「就绪还在、授权已撤销是真实且必须表达得出的一格」。页面上合成一个「可提交」标记就把这份用心作废了。
2. **撤销不是删除**：`revoked_at IS NULL` = 仍有效，有值 = 已失效且原依据原样留着。读面要能答出「有过、已失效」，不得把失效谎报成未配置（0006 自注原话）。
3. **关闭义务的目录行与明细行是两条独立信息。** 0008 自注：目录未登记 → 未决（绝不是「没有义务所以可关」）；目录登记了而清单为空 → 才是如实的空。判据与票 01 的 `contentRegistered` 同形，页面要给这两态不同的说法。

义务项盘点按业务截点进行（`applies_from` / `applies_until`）。目录上列取什么截点是本票内要裁的一小格：倾向**不下推截点参数**（那要改端点契约），上列全部义务项并把区间原样列出，由读者自己看。裁决写进处理器注释。

## 完成标准

- 页面答 `200` + 非空册（三类都要有数据可见）；未启用隔离读准入时答 `403`，与既有查阅端点同签名。
- 真库测试钉住：租户隔离、目录未登记与空清单可分辨、撤销态如实、`limit` 非正即拒。
- 全仓 `go test -count=1 ./...` 绿（含真库，单跑一个真库用例看 `-v` 下是 `PASS` 不是 `SKIP`）。
- 页面层取证照票 01 的脚本走（`dom-dump.sh` / `dom-check.sh`，改 needle 即可）。
