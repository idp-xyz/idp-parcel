# 22 按已登记规则批量重判待判断事实：规则登记之后的回填入口

Category: enhancement
Status: draft——2026-09-24 按 ADR-0146 重新定性（同票 [20](./20-tracking-pull-beat.md)，定性表见 [product-strategy-boundary/02](../../product-strategy-boundary/issues/02-split-parameter-register-and-retriage-deferrals.md)「票：未 resolved 的」）：工作面随票 20 对合成源的参考连接器出现，不再等第一家真源。此前：机制形状已在票 `19` 完成记录裁定（显式触发、不随登记自动发生、复用规则判断路径）；随第一家真源（票 `20`）一起立实施，之前没有真实工作面（通道 6 2026-09-04 立票，只写票面未动代码）
Blocked by: 20（工作面：待判断事实在拉取节拍落地前恒空）

## 缺口

票 [`19`](./19-tracking-source-effective-time-rule-catalogue.md) 落了有效时间规则目录，收编执行器自此对**新到**的素材按规则形成有效时间；
而规则登记（或换版）**之前**已认领、留为「有效时间待判断」的事实，今天只有票 [`21`](./21-effective-time-judgment-online-face.md)
的逐条显式判断一条路。那条路是所有者「就这一条给出」（ADR-0102 决定三第一种来源），拿它替规则干活是把 N 次人工判断当回填。
票 `19` 完成记录已答：回填是一条独立的、显式触发的批量入口，理由三条写在那里，本票不复述。

## 做什么

1. **入口**：受控批量口，一次按（租户，轨迹源）触发；形状照 `cmd/parcel-*-register` 家族或管理台端点，随票 `20` 定——两者
   择一时以 ADR-0101 决定八「结构与频率」判：它是低频运维动作，逐字段表单足够。
2. **每条事实**：取该源当前版规则（`ports.EffectiveTimeRules`）→ `EffectiveTimeRuleApplied` 则
   `domain.JudgeEffectiveTimeByRule(rule, at)` → `ExternalCarrierTrackingFact.JudgeEffectiveTime(judgment, version)` 换新版本回指前版
   → `Save` → 交 VE（`ExternalTrackingFactHandoff`），与 `adopt_tracking_material.go` 规则分支同一条判断逻辑；`EffectiveTimeRuleAbsent`
   则该源无规则，**整批答未受理**，一条不动。
3. **幂等**：当前版已按同一规则引用（源 + 版本）判过的事实不再换版，与 `JudgeEffectiveTimeHandler` 的 `ALREADY_JUDGED_AS_GIVEN` 同形，
   判等键是规则引用而不是有效时间值。
4. **读口**：「按（租户，源）列有效时间待判断的当前版」复用票 `21` 读面的端口，不另开。
5. **结果**：逐条结果清单 + 汇总；倾向每条各自成笔事务（一条交接失败不吞掉其余），失败条带续办引用——是否如此归实施时裁，票面记下倾向与理由即可。

## 红线

- 无规则不得默认「等于发生时间」（ADR-0102 Alternatives 第二条）；整批答未受理，不逐条猜。
- 不随规则登记同事务自动触发（理由见票 `19` 完成记录「回填」节）。
- 不写任何真实源的规则取值与事实；SYN 夹具只记 `S`。

## 开工前要核

- VE 侧消费者（`visibilityexception/adapters/veconsume` 一族）对同一事实多个版本在一批里先后到达是否有序无关；若不是，回填要按事实分组、
  每条只交最后一版，或在 VE 侧补幂等——归哪一侧改在开工前问 VE owner。
- 票 `20` 落地后待判断事实的真实规模量级，决定批量口要不要分页 / 分批提交。

## 完成判据

入口有实现与真库测试；一批里「有规则→全部待判断事实换版并交 VE」「无规则→整批未受理」「已按同版规则判过→不再换版」三格各有用例；
`gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

票 `19` 完成记录「回填」节；ADR-0102 决定三；`internal/transportfulfillment/application/adopt_tracking_material.go` 规则分支；
`internal/transportfulfillment/application/judge_external_tracking_effective_time.go`。

## Comments

- 2026-09-04 · 通道 6：立票。起因是票 `19` 完成判据要求正面答回填问题，答复是「显式批量入口、另立票」，故此票承接。**只写票面，未动代码。**
- 2026-09-24 · 通道 4：Status 行落 product-strategy-boundary/02 的既有定性（用户同日令「避免以后的 agent 开发时老说没有真实的外部环境」）；正文与阻塞边未改。
