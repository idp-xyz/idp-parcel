# VE 五类规则/策略目录只读,里程碑映射键修好了仍没有往里填的口

Category: enhancement
Status: needs-triage

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W16/W17/W18。

## 墙(五处哨兵,同根)

- `MAPPING_NOT_CONFIGURED`(`derive_projection.go` 兜底版本引用,投影落未归类)
- `TRIAGE_RULES_NOT_CONFIGURED`(`raise_signal.go`)
- `NOTIFICATION_POLICY_NOT_CONFIGURED`(`notify_customer.go`)
- `ELIGIBILITY_CATALOGUE_NOT_CONFIGURED`(`handle_claim.go`)
- 披露策略空册→客户视图四维全部待确认(`cmd/parcel-dispatch/assemble.go` 的 `tenantBoundCustomerViewDerive`)

## 现状:有装载无写入

五张表(迁移 0009/0010/0011/0012+0014)与按租户现绑的只读装载口全部就位:`NewMilestoneMappings`、`NewTriageRules`(同在 `rule_catalog.go`)、`NewNotificationPolicies`、`NewClaimEligibilityRules`、`NewDisclosurePolicies`。写入方零,登记口零。里程碑映射的键结构问题已按 2026-08-19 裁定改到类型维(迁移 0014,`.scratch/ve-milestone-mapping-key` 01 票,MAP-KIND in-progress)——目录填得满了,但仍没有填的口。

## 缺的最小机制件

VE 目录登记口:版本化登记用例 + 写入方,覆盖五类目录,各按其参数登记册行的版本/适用范围/发布批准责任建模(映射 PAR-VIS-01、分诊 PAR-VIS-05、通知 PAR-VIS-07、索赔资格 PAR-VIS-08、披露 PAR-VIS-09);进程级入口。

## 红线

- 目录内容全部属实例半边,待提供是常态;本票只建门,验证用隔离映射行(既有裁定原话:「SYN 测试可以插入隔离映射行证明『目录可填且归类』,不得写进生产装配」),S 级只记 S。
- 同一时点两个适用版本是错误(`ErrAmbiguousCatalog`),登记口须在写入侧防重叠,不靠读侧兜。

## 参照

PAR-VIS-01/05/07/08/09;`.scratch/ve-milestone-mapping-key/issues/01`(键结构,在修);外部评审 03 票(账户迟绑无重派生,独立在途)。
