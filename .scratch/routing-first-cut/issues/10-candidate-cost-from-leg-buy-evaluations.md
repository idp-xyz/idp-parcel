# 10 候选成本：计划履约段的 BUY 评价经 parcel-pricing 合成候选成本

Category: enhancement
Status: resolved · 已进 main——2026-10-08 纯快进 `53422925..f01a6077`（`f5257626`/`5b0d46f2`/`e4c1242c`/`5f09dde5`/`e9fef1ed`/`743927d8` + 清点 `f01a6077`，SHA 不换）。分支 `mcp3-rfc10` 作封存出处。此前：in-progress——2026-10-08 TraeCode 会话认领（用户令「开始接下一张票」；阻塞 02、03、09 均已在 main）。此前：ready-for-agent
Blocked by: 02、03、09
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「接路由证据取数侧」那一步（成本），也是「首个内置排序策略」的事实来源
地盘：network-routing 的出向端口与 parcel-pricing 消费方适配器（NR 侧）；目录线路版本上指向 BUY 价卡的引用列（新迁移号开工时预留）。parcel-pricing 若需新口，先在频道与 PP 地盘的主人约。
出处：02 的 ADR（成本口径）；[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-NET-16` 已确认的机制句；[`label-channel-service-first-release/13`](../../label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md)（PP 批量评价口，与「PP 只答算不算得出」的分工）。

## 做什么

1. 目录线路版本带它的 BUY 价卡引用：形状归产品，引用哪一份卡是租户取值。
2. 取数侧对每个候选逐段向 PP 取 BUY 评价，按 02 的口径合成候选成本，交 03 定的成本事实形状；任一段待判断或不可计价，该候选成本即不可得。
3. 与 03 合起来：合成目录上，初始路由按成本单维选出唯一候选并形成计划；两候选成本相同则交冲突。

## 不做

- 不在 NR 复制计价规则；不定任何价卡内容。

## 完成判据

- [ ] 真库或进程级用例：两候选成本不同 → 形成计划（`S`）；成本相同 → 冲突；一段不可计价 → 该候选出局。
- [ ] 取数侧的成本缺口不折成零。

## 裁决（2026-10-08，用户代 PP / PC owner 拍板；据此地盘从「仅 NR 侧」扩为 NR + PP + PC）

- **比较币种换算随本票落地。** ADR-0148 越权风险点 1 的 PP 侧延伸——换算从结算币种延伸到比较币种、逐段保留原币 / 结算币种 / 所引汇率序列版本与比较币种金额——由本票实现；口径仍由价格政策声明、比较币种仍是路由策略版本上的租户取值。「与 PP 主人约」落地为这次用户裁决。
- **自营段随本票落地。** 越权风险点 2：内部价格政策承载自营段（含集团内另一法人承运段）的内部标准成本，经 PP 同一评价机制出价；引用形状同时承载外包 BUY 价卡与内部政策价卡两形。

## 完成记录（2026-10-08，TraeCode 会话，分支 `mcp3-rfc10` 基 `53422925`）

**落点**

| 笔 | 内容 |
|---|---|
| `f5257626` | 票面：认领 + 两项裁决 + 实施方案 |
| `5b0d46f2` | parcel-pricing：比较币种换算（`WithComparisonCurrency` / `ComparisonAmount` 全精度 / `ComparisonStep` / ISO 4217 `MinorUnitScale`；摘要与快照各加 comparison 节） |
| `e4c1242c` | party-commercial：`InternalCostPolicyView` 点读口 + 真库六用例 |
| `5f09dde5` | network-routing：迁移 0016（策略版本比较币种/所引政策两列 + `line_cost_basis` 册）；证据端口透出比较声明；同笔补 04 折叠缺口（冻结两列随快照折回） |
| `e9fef1ed` | network-routing：取数侧（`adapters/parcelpricing/cost_source.go`）+ 内部政策解析桥（`adapters/partycommercial`）+ 编排接线与两格未决 + 计划 jsonb 出处留痕 + 装配根 |

**完成判据**

- ✅ 判断路径用例（进程级，取数侧交付事实、选择排序交 03 已有用例）：两候选成本不同各自出价（5→500、8→800 minor）→ 领域按成本单维选出唯一候选形成计划路径；不可计价（卡在判断时点不适用 → 评价冲突）→ 该候选整体 UNPRICEABLE 出局；并列交冲突由 03 的 `RankRouteCandidates` 用例钉。
- ✅ 缺口不折零：没挂依据、政策没登正文、方案不在册都如实 PENDING；比较币种异于卡币种时取全精度比较金额（10 USD × 7.2 = 72，取证 `TestAComparisonCurrencyReachesThroughTheLegs`）；两段 5.005 先求和后取整 = 1001 minor，逐段先取整会得 1002（`TestLegAmountsAreSummedBeforeTheSingleRounding` 的对照事实）。
- ✅ 出处留痕：选中候选逐段（评价标识 + 方案引用 + 内部段政策引用 + 全精度比较金额）随计划 jsonb 一并落，旧计划读回空列表照常成立。

**门（钉 `e9fef1ed`，带 DSN，本机 CI=true 且 55432 门禁库可达）**：`go build ./...` 0；`go vet ./...` 0；`gofmt -l` 空；`go test ./internal/architecture/ ./internal/networkrouting/... ./cmd/... -count=1` 全绿（含 PBC-08 事务门禁——新写口补了 `TestLineCostBasesRefuseToRunOutsideATransaction`）。全仓 `-p 1` 全量留给重放台跑实。

**判断项**

1. **比较口径的取整模式内置 HALF_UP。** ADR-0148 决定四.2 把汇率口径归价格政策，没点名合成取整模式；按行业惯例 HALF_UP 内置在取数侧（`minorUnitsOnce`），将来要变口径属价格政策声明的延伸，不写死在任何租户行。
2. **没登比较币种的退路用评价总价合成。** 总价已过卡的取整点，精度上比比较币种路径松弛一格；两段全同币种时合成照常、异币种如实停下 → PENDING。退税路的金额出处照实记在出处里。
3. **策略版本登记的 `ComparisonPricePolicy` 尚无人消费。** 本票把这一格落进目录与证据、随计划留痕；实际汇率读数仍按方案绑定的汇率序列取值（quoted basis 引用政策工件），「策略版本指名的比较价格政策」到取数侧的消费是下一张票（PP 校准口径读口）的事——记为已知缺口，不为它编默认政策。
4. **自营段方向核：内部政策解析桥按被引方向拒译**（引用指到 SELL/BUY 政策即响亮错误）；内部价卡必是 `INTERNAL` 方向 + `INTERNAL_PRICE` 用途的方案。
5. **04 折叠缺口同笔修复**：`freeze_form`/`freeze_remaining_segments` 此前只写不读（快照折回恒未声明），随 0016 一并补 SELECT/行模型/重建三处与回归用例。计入口径：这是 04 落地余下的缺陷，不是本票新引入。

**未验 / 边界**：演示租户端到端（rfc/11 与 demo-seeds 的实例半边）；计价输入取数路径仍为「未配置」哨兵（消费方实例半边，编不得）；真租户价卡与政策随登记册证据才升 `R`——本票全部取证为 `S`。评审：本会话是唯一执行方，推送方自审（不算非作者评审），单据按并行会话口径留白。

## Comments

### 进 main 记录（2026-10-08，TraeCode 会话代推送方）

- **纯快进，不 cherry-pick、SHA 不换**：`origin/main` = `f01a6077`（`53422925` 之上六笔码/票面 + 清点一笔）；分支 `mcp3-rfc10` 是 main 祖先，作封存出处。
- **门**：清点在 `/tmp/idp-replay-rfc10`（`743927d8` 的干净检出）上重生成；全量 `go test -p 1 -count=1 ./...` 带 DSN **136 ok / 0 FAIL / 0 失败行**（17:5x–18:0x 一轮）。
- **清点**：networkrouting 生产 65→67、测试 62→64；partycommercial 生产 153→154、测试 161→162、HTTP 39→40；合计 1145→1148 生产、1055→1058 测试、第四列 285→286；消费缝 networkrouting→parcelpricing 新 +1、networkrouting→partycommercial 3→4；network_routing 迁移 15→16。
- **评审**：本会话是唯一执行方——推送方自审（完成记录「判断项」五条即自审所得），**不算非作者评审**，按并行会话口径照实留白。
- **远端分支**：`origin/mcp3-rfc10` 停在其已进 main 的祖先上，按下述「收尾」删；本地改名 `merged/mcp3-rfc10` 指针留档。

### 收尾

- 远端 `mcp3-rfc10` 已删（其内容全在 main，SHA 对照零差）；本地指针改名 `merged/mcp3-rfc10`；重放树 `/tmp/idp-replay-rfc10` 拆。

## 实施方案（2026-10-08 认领时草定，随实现修订；已完成项见完成记录）

1. **PP · 比较币种换算（内部延伸，不新开对外口）**：`PricingInputSnapshot` 增 `WithComparisonCurrency`（与 `WithSettlementCurrency` 同款）；评价成形在结算换算之后，若声明比较币种且不等于卡币种，取同一 EXCHANGE_RATE 序列读数做第二笔换算（卡币→比较币），存 `comparison *ConversionStep`；比较金额**不**过卡的 `AmountRoundingAfterConversion`——它留给 NR 求和后取整一次。比较币种未声明 = 老行为，等于卡币种 = 无第二笔。`Currency` 增 ISO 4217 最小币单位（按公开基准内置，只录价卡出现过的币种，未录的如实答否 → 该段待判断）。摘要 canonical doc 增 comparison 节；解释行补一句；postgres 评价存储相应扩展 + 迁移。测试：同一读数两笔换算、等于卡币种无步、未声明向后兼容、摘要含 comparison。
2. **PC · 内部成本政策读口（只读新口）**：外部段引 PC 价格政策（InternalDirection），NR 取数侧要一个口把它解成绑定的 PP 方案引用（含 standing 三格）。先在仓库里核商业政策读侧有没有近似口，没有则页 `ports.InternalCostPolicyView` + postgres 适配器 + 真库测试。
3. **NR · 路由策略载体 + 目录（新迁移 `network_routing` 0016）**：策略版本加「比较币种」与所引价格政策两项（ADR-0148 Consequences：「随 /03 载体或 /10 接成本时补」）；线路版本段连各带一条成本依据引用：外包 BUY 价卡（方案 id/version）或内部政策（policy ref）两形；登记 / 读回 / 清单照既有目录写口同款。
4. **NR · 取数侧适配器（`internal/networkrouting/adapters/parcelpricing/`）**：逐候选逐段解引用 → 外包直接得方案，内部经 PC 口得方案 → 组装计价输入（实例半边口，`未配置`即停下，不编输入）→ `EvaluatePricingAcrossPlans` 批量评价 → 逐段译三态 → `Money.Add` 不舍入求和 → 按比较币种最小币单位取整**一次** → 合成 `CandidateCostFact`。段级明细（评价标识 + 方案引用 + 政策引用）随证据留存。策略版本没登比较币种时退回同币种合成，异币种由既有 `RankingCurrenciesDiffer` 未决格接。
5. **接线与验证**：`InitialRouteEvidence.CandidateCosts` 在证据装配处填上（组合点按装配器现状定）；新导出工厂补 `production_wiring_baseline`；真库用例三格（不同 → 计划；相同 → 冲突；一段不可计价 → 出局）+ 缺口不折零；全仓门禁后完成记录、评审、重放进 main。
