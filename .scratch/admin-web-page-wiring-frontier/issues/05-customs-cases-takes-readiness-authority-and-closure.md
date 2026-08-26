# 关务案件页收三类：就绪判断、提交授权、关闭核对

Category: enhancement
Status: resolved

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

## 收口（2026-08-26 · MCP-1）

`0cfa6b9` 后端（伴生读端口 `CaseRegisterCatalogueRead`、真库适配器、`GET /customs-case-registers` 端点、`cmd/parcel-api` 装配与放行面枚举——含 `unwired_orchestration.go` 补方法，票 08 收口预告的那格应验），`88b8607` 种子第二单元（就绪仍有效、授权已撤销），`d9e90d3` 页面层，取证工具两修随收口笔另记。

**三类收进两个页签，接线签在前并作默认。** 就绪与授权并排一签（单元维成行，两栏各带依据/时间/现况三列，现况封闭三态：未登记／仍有效／已撤销:原因+时刻——「未登记」与「已撤销」是两格，后者原依据原样留在列上）；关闭义务一签（案件维目录连义务项逐行，空清单目录占一行写明）。两个接线签排在四个对象族骨架签之前——对象族列表端点未建，占位签留在默认位会与工作台「已接线」档位打架；端点建成接线时可回归对象层级排序（页面注释同句）。

**截点裁决照票面倾向落地**：端点不收 `cutoff`，全部义务项连同适用区间原样上列，判读归读者；按截点盘点是点读 `LoadObligationItems` 伺候的另一个调用面，查阅口收截点等于让目录读口长出判断语义（裁决写在 `query_case_registers.go` 文件头）。

**取证**

1. 端点（隔离读实例 `:19081`，以当前树重建重启后）：三 `registry` 各答 `200`——`readiness` 2 条全有效；`submission-authority` 2 条、`SYN-UNIT-CN-EXPORT-02` 带 `revokedBy:SYN-CAUSE-MANDATE-WITHDRAWN`（0006 自注那格实答得出）；`closure-obligation` 1 份目录 2 项（已终结 + 已承接指名 `SYN-BROKER-01`，`appliesUntil` 如实缺席）。未配置 403 由处理器测试与 `endpoints_test.go` 的 unwired 探针钉住。
2. 真库四条 `-v` 下 `PASS` 非 `SKIP`（`TestEmptyCaseRegistersAnswerEmptyLists` 单跑 0.24s 实跑）：空册答空、撤销如实、目录未登记与空清单可分辨、`limit` 非正即拒；义务父子 `json_agg` 相关子查询一条语句（笛卡尔积教训照票 07）。
3. 全仓 `go test -count=1 ./...` 81 包全绿（含真库，`customscompliance/adapters/postgres` 63.9s 实跑）。工作树当时含 MCP-2 在途的 `parcel-commercial` / `parcelshipment` 未提交件，一并编译测试通过，与本票无涉。
4. `seed.sh --reset` 零报错，`authority-revoke: REVOKED` 在列。
5. 页面层：默认页签八项该在的全 HIT（「仍有效」与「已撤销：SYN-CAUSE-MANDATE-WITHDRAWN」同屏，正是形状约束一要的两栏各态），关闭义务页签八项全 HIT（含「持续有效」与「关闭义务目录 1 份 · 义务项 2 项」），两产物未配置码全 miss。工作台已接线 16→17、关务分区 1/2，liveIds 单源派生。
6. **「未登记」态 DOM 里核不到**——种子两单元两册齐全，库里没有「一册有行、另一册没有」的单元；该词形是前端并排两册时的第三态，数据侧由真库撤销/租户用例钉住。「目录已登记而清单为空」同理（唯一种子目录带两项），由 `TestClosureObligationCataloguesSeparateUnregisteredFromEmpty` 钉住。

**取证工具两处修，都因为踩了才修**（`dom-dump-tab.mjs`）：

- **Radix Tabs 的切换挂在 mousedown 上，`element.click()` 只合成 click**——按钮被「点」了而页签纹丝不动，产物字节数与默认页签几乎一样、needle 全 miss，长得跟「页面没接上」一模一样。此前用它取证的页第二签都是普通 onClick 筛选片，所以没暴露。现按真实事件序补发 mousedown → mouseup → click，两类按钮都吃这一序。
- **`Browser.close` 返回时 Edge 未必已放开 profile 目录**，立即 `rmSync` 撞 `ENOTEMPTY`——产物已落盘而进程以 1 退出，`&&` 链上的 dom-check 被吞。带重试删，删不干净留给 /tmp 回收，不再让清理失败污染取证退出码。
