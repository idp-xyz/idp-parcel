# 19 准入范围桥：操作者与集成客户端铸造器的 `AdmissionScope` 由「租户对照 + `AuthorityCoverage`」实现，接进生产装配，演示租户登 `SYN-` 对照

Category: enhancement
Status: needs-triage——2026-10-13 通道 4 立（通道 1 派单 `task-40698906`，照裁定 A：[15](./15-operation-decision-faces-take-operator-intake.md) 余下四口不换，桥另立）。桥的骨架有出处：ADR-0149 决定四第二条定了按（租户、对象范围、能力、事实类型）读生产权威区间，[14](./14-admission-scope-read-and-grade.md) 交了读口 `AuthorityCoverage` 与「不在准入范围」一格。没有出处的是对照本身——行住在哪、按什么键、本产品的权威串从哪来；第一件定了就要落迁移，见「要裁的问题」。
Blocked by: 无——要裁的是本票自己的形状，裁完转 ready-for-agent
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘（裁完按形状收窄）：`AdmissionScope` 的生产实现与对照的读口（落点随问题一）；`cmd/parcel-api` 的 `buildOperatorMinter`、`buildIntegrationClientMinter` 与 `main.go` 里建它们的那几行；演示种子 `scripts/demo-seeds/data/governance/` 的区间、对照的那份种子、`seed.sh` 相应一段与 `scripts/demo-seeds/README.md`；对照若是新的一类实例值，参数登记册增一行。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第二条、越权风险点 3；[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定三；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一；[ADR-0083](../../../docs/adr/0083-pilot-governance-read-face-carries-registry-dimensions-only.md) 决定一；14 判断项 1、2、4；`cmd/parcel-api/assemble_operator_decisions.go` 的 `buildOperatorMinter` 头注。

## 为什么

14 判断项 4 把桥定为「随第一个消费者一起立」。第一个消费者是 15 之四：`buildOperatorMinter` 取了 `UnconfiguredAdmissionScope{}`，头注写「对照登进来的那一笔，把这里换成『对照 + pilot-governance 的 AuthorityCoverage』桥」；[11](./11-integration-client-register-and-client-credentials.md) 的 `buildIntegrationClientMinter` 同样。桥是机制；各租户的对照、区间与权威串是租户取值，对照没登的租户答「不在准入范围」（ADR-0149 越权风险点 3）。

演示租户也只有这一条路：ADR-0150 决定一不许按 `SYN-` 前缀分支，演示动线要越过准入范围，只能给 `SYN-TENANT-01` 登一份普通的对照与区间。取证钉 `1e02d2ea`：全仓非测试代码里 `AdmissionScope` 的实现只有 `UnconfiguredAdmissionScope`；`NewAuthorityCoverage` 没有生产调用点；演示种子里没有能力为 `OPERATION_DECISION` 或 `EXTERNAL_FACT` 的区间。

## 挡在本票上的

- 15 余下四口——关段、建派送任务、有效时间判断、承运商首次有效收寄判断：换口同笔撤隔离放行（ADR-0150 决定三）之后，演示租户在这几口上答 `403 OUTSIDE_ADMISSION_SCOPE`，管理台有效时间判断页（`effective-time-judgment-api.ts` 的 `judgeEffectiveTime`）随之退成 403。关段、建派送任务与收寄判断另挡在 [20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md)。
- 15 之四已换的复核完成、主动拒绝、授权处置、装载分配、终止参与：设了发行方的生产装配里，授予齐备也停在 `403 OUTSIDE_ADMISSION_SCOPE`（`TestSwappedDecisionFacesAnswerFromTheOperatorChannel` 的 "granted, admission not registered" 一格钉着）。越过它，装载分配与终止参与答业务结果，委托侧三口到达编排、再停在各自的授权缺口（15 完成判据第三条）。
- [10](./10-device-register-and-operation-fact-capability-face.md) 余下那半（作业事实各口换口同笔撤隔离放行）：`WorkFactMinter` 同样收 `AdmissionScope`，请求带准入要求即判；`cmd/parcel-api` 今天还没有它的装配点，接上时取 `UnconfiguredAdmissionScope` 就与 15 余下四口同一处境。
- 11 已换的集成客户端各口（关务外部结果、监管凭证登记、外部资金事实登记与更正）：同一格，`TestIntegrationClientFacesAnswerTheChannelGrades` 的 "granted, admission not registered" 钉着。

登记册配置写面与目录查阅面不受影响：它们的请求不带准入要求，`MintOperator` 只在 `Admission` 非空时判。

## 要裁的问题

1. **对照的行住在哪、归哪个上下文。** ADR-0083 决定一否决给治理登记册加租户维（治理是产品实例级机制）；14 判断项 1 只说对照是「租户随试点登记的实例半边，由 `AdmissionScope` 的实现去换」，没说行在哪。parcel-shipment 生产归属的 `GovernanceScopeDirectory` 同样只有隔离实现，生产形态留空（`buildProductionOwnership` 头注）。候选：
   - 甲 · access-identity 另立一本对照册，与操作者册、设备册、集成客户端册同处：`access_identity` 下一号迁移，`parcel-access-register` 加子命令，行带生效区间与依据。
   - 乙 · pilot-governance 另立一张带租户的对照表：与 ADR-0083 决定一相抵，要另出记录部分停用它。
   - 丙 · 部署参数，同发行方那几件：不落库、不过受控登记。

   甲、乙落迁移，只进不退；丙每改一次要重启，也没有登记轨迹。
2. **对照按什么键。** `Admits` 只收（租户、能力与事实类型、时刻），看不到这一笔改的是哪个对象；演示种子里路由、计价、委托受理各是一个试点范围。拟按（租户、能力、事实类型）→ 一个对象范围。同一租户同一能力分属几个试点范围（比如按线路分批上线）就表达不了，要让 Intake 把对象交给 `Admits`——那是改 14 的接口，另立。
3. **本产品的权威串从哪来，PS 那一处跟不跟。** 14 判断项 2：`AuthorityCoverage` 的 selfAuthority 与 parcel-shipment 生产归属的 `SelfAuthority` 应取同一值。PS 那一格今天生产形态留空，隔离形态取 `isolatedSelfAuthority`（`SYN-AUTH/idp-parcel-pilot`）。拟取部署参数一件、两处同读；PS 那一处若随本票同笔接上，地盘就越进 PS 的装配（`buildProductionOwnership`）。

另有一格属产品策略，可随上面一并认：治理登记册上的能力与事实类型词表。两族认证方今天取 `OPERATION_DECISION` + 决定种类、`EXTERNAL_FACT` + 外部事实类型，15 判断项 3 记着「首个租户登记区间时要对齐」；演示区间照这两组常量登。

## 做什么（裁完照此拆笔）

1. `AdmissionScope` 的生产实现：按问题一、二读租户对照，换成 `AuthorityCoordinates` 问 `AuthorityCoverage.Covers`。对照没登答不在准入范围；对照或区间读不动答 `ErrAdmissionScopeUnavailable`，不折成不在准入范围（14 的分格）。
2. 生产装配：`buildOperatorMinter`、`buildIntegrationClientMinter` 改收这只实现，`main.go` 建它（区间读口接真库，selfAuthority 按问题三）。selfAuthority 没配时，各口答复与今天逐字节相同。10 接 `WorkFactMinter` 时取同一只。
3. 演示租户：`SYN-TENANT-01` 的对照，与 `OPERATION_DECISION`、`EXTERNAL_FACT` 下各种类的区间，经普通登记进种子，标识带 `SYN-` 前缀；演示部署配上 selfAuthority。取证要用的授予若种子里还没有，随本票补，只加自己那几条。
4. 问题一取甲或乙时，参数登记册增一行（某租户的对照，租户取值），格里只写尚无租户。

## 不做

- 任何真实租户的对照、区间与权威串（租户取值）。
- 15 余下四口的换口与撤放行、10 的换口（各在其票）。
- 改 `AdmissionScope` 接口让它看得到对象（问题二若裁为要，另立）。

## 完成判据

- 对照已登且区间覆盖 → 铸成；对照未登，或登了而区间不覆盖（别的权威、已关、未始、别的对象范围）→ 不在准入范围；对照读不动、区间读不动 → 依赖故障格，且不被「不在准入范围」认出（带测试）。
- 生产装配：selfAuthority 未配时，`TestSwappedDecisionFacesAnswerFromTheOperatorChannel` 与 `TestIntegrationClientFacesAnswerTheChannelGrades` 的准入那一格照旧；配了且对照与区间齐备时，装载分配与终止参与答业务结果（`cmd/parcel-api` 装配测试带 DSN）。
- 演示环境灌完种子后，带运营决定授予的合成操作者在装载分配或终止参与上不再答 `OUTSIDE_ADMISSION_SCOPE`（`scripts/demo-seeds/operator-channel/probe-operator-channel.sh` 那一套取一次证据，证据层级 `S`）。
