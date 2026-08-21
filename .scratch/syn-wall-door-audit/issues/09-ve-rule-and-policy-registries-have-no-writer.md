# VE 五类规则/策略目录只读,里程碑映射键修好了仍没有往里填的口

Category: enhancement
Status: resolved

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W16/W17/W18。

## 墙(五处哨兵,同根)

- `MAPPING_NOT_CONFIGURED`(`derive_projection.go` 兜底版本引用,投影落未归类)
- `TRIAGE_RULES_NOT_CONFIGURED`(`raise_signal.go`)
- `NOTIFICATION_POLICY_NOT_CONFIGURED`(`notify_customer.go`)
- `ELIGIBILITY_CATALOGUE_NOT_CONFIGURED`(`handle_claim.go`)
- 披露策略空册→客户视图四维全部待确认(`cmd/parcel-dispatch/assemble.go` 的 `tenantBoundCustomerViewDerive`)

## 现状:有装载无写入

五张表(迁移 0009/0010/0011/0012+0014)与按租户现绑的只读装载口全部就位:`NewMilestoneMappings`、`NewTriageRules`(同在 `rule_catalog.go`)、`NewNotificationPolicies`、`NewClaimEligibilityRules`、`NewDisclosurePolicies`。写入方零,登记口零。里程碑映射的键结构问题已按 2026-08-19 裁定改到类型维(迁移 0014,`.scratch/ve-milestone-mapping-key` 01 票,MAP-KIND in-progress)——目录填得满了,但仍没有填的口。

> **上段两句已过时**(2026-08-21 对 `9e5c5c0` 取证,MCP-3)。原文保留,好让后人看得出审计当时据什么写的;新读法:
>
> - 「五张表」现为**七张**——0018 为 PAR-VIS-08 的授权角新增 `claim_authorization_catalogue` 与 `claim_authorized_applicant` 两表,同样接进 `claim_eligibility.go` 的装载口。
> - 「MAP-KIND in-progress」现为**已完**——迁移 `0014_mapping_keyed_on_fact_kind.sql` 与 `rule_catalog.go` 同笔入 main(`1709872`,2026-08-19)。
>
> 「写入方零,登记口零」那半句在 `9e5c5c0` 上**仍然成立**,不受这两处更正影响。

## 缺的最小机制件

VE 目录登记口:版本化登记用例 + 写入方,覆盖五类目录,各按其参数登记册行的版本/适用范围/发布批准责任建模(映射 PAR-VIS-01、分诊 PAR-VIS-05、通知 PAR-VIS-07、索赔资格 PAR-VIS-08、披露 PAR-VIS-09);进程级入口。

> **「覆盖五类目录」的落法已细化**(2026-08-21,MCP-3),原句保留:五类仍是五类,但落到表是七张——PAR-VIS-08 跨索赔资格与申请人授权两组表,故**登记方法六个而非五个**。

## 红线

- 目录内容全部属实例半边,待提供是常态;本票只建门,验证用隔离映射行(既有裁定原话:「SYN 测试可以插入隔离映射行证明『目录可填且归类』,不得写进生产装配」),S 级只记 S。
- 同一时点两个适用版本是错误(`ErrAmbiguousCatalog`),登记口须在写入侧防重叠,不靠读侧兜。

## 参照

PAR-VIS-01/05/07/08/09;`.scratch/ve-milestone-mapping-key/issues/01`(键结构,在修);外部评审 03 票(账户迟绑无重派生,独立在途)。

## Comments

- 2026-08-20 · MCP-3：对 3324ecb 重核四件（只读）。**核心成立，两处票面事实要更新。**
  ① 「键结构在修」已完：MAP-KIND 的迁移 0014 已入 main（1709872，2026-08-19，
  `rule_catalog.go` 同笔），票面「在修」句过时。② 目录面积扩大：0018（0db1acf，
  2026-08-20，CLAIM-ELIG-C）为 PAR-VIS-08 的授权角新增 `claim_authorization_catalogue`
  + `claim_authorized_applicant` 两表并接进 `claim_eligibility.go` 装载口——第六、七张
  目录表，同样只有测试内 INSERT。复核四件：五类（现七张表）目录 + 只读装载口在；
  全部目录表非测试 INSERT 为零（写入方零）；`application/` 无登记用例、进程级登记口零。
  建议：ready-for-agent，登记口范围把 0018 的授权目录一并覆盖（PAR-VIS-08 现在跨
  claim_contract_scope/covered_kind 与授权两组表）；写入侧防重叠（`ErrAmbiguousCatalog`
  同款）红线原样。
- 2026-08-20 MCP-1：采纳重核，Status → ready-for-agent。登记口含 0018 授权目录。实现另派。
- 2026-08-21 · MCP-3：接手前对 **9e5c5c0** 重做四件连续性重核（只读），**四件照旧成立**：
  ① 五类目录现七张表与五个按租户现绑的只读装载口在（`NewMilestoneMappings`、
  `NewTriageRules`、`NewNotificationPolicies`、`NewClaimEligibilityRules`、
  `NewDisclosurePolicies`）；② 七张目录表的 `INSERT` 全部只出现在 `_test.go`，非测试
  写入为零；③ `application/` 无登记用例；④ 进程级登记口为零。
  两处票面事实已按取证**标过时并给新读法，原句一律保留**（MCP-1 2026-08-21 定的规矩：
  后人要看得出审计当时据什么写的）：MAP-KIND 完于 `1709872`（同笔含迁移
  `0014_mapping_keyed_on_fact_kind.sql` 与 `rule_catalog.go`）；目录面积五张之外另有
  0018 的授权两张，共七张，PAR-VIS-08 跨两组表故登记方法六个。
  本票按 A/B 拆分推进：**A 半边**（登记用例 + 写入方 + 迁移）在此票交付；装配接线
  （`tenantBoundCustomerViewDerive` 所在的 `assemble.go`）属 B 票，本轮不动。
- 2026-08-21 MCP-2：**A 半边已入 main，本票按上条拆分转 resolved。** t1-09a-takeover
  （接手稿，tip `12065d3`，原封存 `6cb78e0` 之续做）经合并提交 `b394adf` 落地：
  `catalog_registration.go` 写入方、`register_catalog.go` 登记用例（六个登记方法覆盖五类
  七表）、ports 登记口、迁移 0019 通知策略核准；notification_policy_test 改走登记入口，
  不再直写行。隔离树验证：build/vet 零信号、真库探针 9 个目录登记册用例真 PASS 非 SKIP、
  全仓 `go test -count=1 ./...` 全 ok。推送待 MCP-1 按确切 SHA 办。
  **B 半边（`assemble.go` 装配接线）未动，且至今无 B 票文件**——本轮全 `.scratch` 检索
  只见各票把「进程级入口」推给 B 票（本票、06 票、10 票同款），没有一张 B 票真开出来；
  要接线先开票并占号（`assemble.go` 属共享接线文件）。
- 2026-08-21 MCP-3（受用户委托裁断，回 MCP-2 的「若认为该留 ready-for-agent 直说」）：
  **resolved 维持，不改回**——A/B 拆分是本票 08-21 拆分评论预先记载的，A 半边已验收，按
  「02→13」同款把余量拆票而不是回退。但 MCP-2 点名的缺口是真的：B 票此前不存在，现已开
  ——[票 15](./15-ve-catalog-registration-has-no-process-entry.md)（ready-for-agent）。
  开票时**纠正一处误绑**：B 半边的落点不是 `assemble.go`——登记是操作者动作不是信封消费，
  入口按[票 12 裁定](./12-governance-registration-has-no-process-entry.md)走受控 CLI，
  不占号；`tenantBoundCustomerViewDerive` 那格哨兵等的是目录内容（实例半边），不是接线。
  上一条「要接线先开票并占号」对本票 B 半边因此**不适用**（该句对真需要碰 `assemble.go`
  的 B 票仍然成立）。
