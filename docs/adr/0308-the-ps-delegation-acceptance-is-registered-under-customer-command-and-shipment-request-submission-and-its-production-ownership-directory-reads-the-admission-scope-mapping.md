# ADR-0308：PS 委托受理在治理登记册上登在能力 `CUSTOMER_COMMAND`、事实类型 `SHIPMENT_REQUEST_SUBMISSION` 之下；生产归属目录读准入范围对照册换坐标、不代拟，试点范围版本取对照的对象范围本身

Status: Accepted（2026-10-26，Claude Code 会话代 PS owner 决：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 first-party-api/04，ADR-0180 越权风险点 6 与决定四把这一格交给「PS 生产归属那一票」。裁决能力边界：读过 [ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md) 全文，票 first-party-api/04 与 00 的完成记录（拟受理范围快照一节），ADR-0150 决定三、ADR-0141 决定四的出处句（经票面转引）；代码按符号取：`ProductionOwnershipAdapter` 的 `governanceScope` 与 `GovernanceScopeDirectory`、`IsolatedGovernanceScopeDirectory`、`buildProductionOwnership`、`AdmissionScopeMappingReader`、`ResolveAdmissionObjectScope`、`checkAdmissionRequirement`、`CustomerCommandAdmissionCapability` 与 `customerCapabilitiesAdmittedAtMint`；演示种子 `data/governance/07`、`08` 与 `data/access/03`。没读：ADR-0139 全文（只信 `customer_client_pre_advice.go` 注释对决定八的转述）、ADR-0025 全文、`pilot_governance` 暂停相关迁移。拿不准的列在文末「越权风险点」，待 PS owner 复核）<!-- 索引注记 -->｜**词**：能力 `CUSTOMER_COMMAND` + 事实类型 `SHIPMENT_REQUEST_SUBMISSION`，产品词、不带 `SYN-`，定义在 `accessidentity.ShipmentRequestSubmissionAdmission` 一处。**目录**：读对照册、不代拟，查不到答缺席（编排照旧答`权威未确定`），读不动答错误；试点范围版本取对照的对象范围本身，不另设来源。
Date: 2026-10-26

## Context

**生产形态下首方口一单也提交不进来。** 提交口挂首方客户渠道 Intake 之后，演示租户之外的任何租户经 `POST /v1/shipment-requests` 提交都停在 `OWNERSHIP_UNRESOLVED`：`buildProductionOwnership` 在生产形态下不给 `Directory`，`governanceScope` 在目录缺席时先于读册早退。只有设了隔离写开关，注入式合成目录才交出 `SYN-CAP/shipment-intake`、`SYN-FACT/shipment-request` 这组合成坐标。ADR-0150 决定三要写开关随最后一口换完退场，退场之前必须有生产目录接手这一格。

**要裁的是三格。** 一，PS 委托受理在治理登记册上用什么能力词与事实类型词（ADR-0180 越权风险点 6，决定四那一类）；二，目录从哪里读、读不到怎么答；三，目录交出的坐标里「试点范围版本」（给暂停决定定位）由谁给。

## 候选与反方

### 能力与事实类型词

- **另立一个 PS 自己的能力词**（如 `SHIPMENT_INTAKE` + `SHIPMENT_REQUEST`）。反方：首方客户族的生产写口已经定下同族取法——能力 `CUSTOMER_COMMAND`、事实类型取客户面能力词（`CustomerCommandAdmissionCapability` 头注：「每一口在治理登记册上就是一个独立的能力，阶段治理无从按『客户业务命令面』一并登区间」）。委托提交是同一族的第一口，另立一个词等于让这一族分裂成两种取法，阶段治理（`PAR-GOV-03` 一类）要按两套词分别登区间。否决。
- **沿用隔离形态的 `SYN-CAP/shipment-intake` 与 `SYN-FACT/shipment-request`。** 反方：带 `SYN-` 的是合成实例标识，不是产品词（ADR-0180 决定四第二项）；生产词带它，等于把过渡期的合成坐标固化成生产默认（红线「租户取值留给租户」的反面）。否决。
- **`CUSTOMER_COMMAND` + `SHIPMENT_REQUEST_SUBMISSION`。** 即决定一。

### 目录读哪里、读不到怎么答

- **在目录里代拟**（按租户、客户账户拼一个对象范围）。反方：代拟的后果不是查不到，而是查到别人那一行（`GovernanceScopeDirectory` 头注）。否决。
- **从部署参数读。** 反方：ADR-0180 候选丙已否决——对照是租户取值，需要依据、生效区间与历史。否决。
- **读接入身份的准入范围对照册。** 即决定二。读不动与查不到分格：把故障读成缺席，一次依赖故障就变成一次`权威未确定`，两者的运维动作不同（ADR-0029）。

### 试点范围版本由谁给

- **另设一个来源**（部署参数，或对照册多一列）。反方：权威区间的对象范围与暂停决定指向的试点范围版本在治理登记册里是同一个引用（演示种子里两者同值，隔离形态的合成坐标亦然）；多一个来源，就多一处能与对照错开的地方，错开时暂停会停在别的范围上而不报错（ADR-0180 越权风险点 3 的同类）。
- **取对照的对象范围本身。** 即决定三。

## Decision

**一、词。** PS 委托受理在治理登记册上的能力是 `CUSTOMER_COMMAND`，事实类型是 `SHIPMENT_REQUEST_SUBMISSION`（即客户面能力 `CustomerCapabilityShipmentRequestSubmission` 的值）。它们是产品词，不带 `SYN-`；演示区间照原词登，带 `SYN-` 的只有对象范围与权威串。词在 `internal/accessidentity` 的 `ShipmentRequestSubmissionAdmission` 一处定义，生产目录与对照登记（`checkAdmissionRequirement`）都引它——抄成两份，改了一份，登在旧词下的区间对另一边无声失效（ADR-0180 决定四同一理由）。

**二、对照册收下这一对词，铸造侧仍不判它。** 委托提交不在铸造侧判准入范围（ADR-0139 决定八，生产归属由编排判），所以这一对词不进 `customerCapabilitiesAdmittedAtMint`，铸造方法不会问它；`checkAdmissionRequirement` 单独认下它，登记子命令才登得进来。对照册对它只是换坐标的行，不是准入判据。

**三、生产目录读对照册、不代拟。** `ProductionGovernanceScopeDirectory`（`internal/parcelshipment/adapters/accessidentity`，ADR-0025 的消费方侧位置）实现 `pspilot.GovernanceScopeDirectory`：从拟受理范围认回租户（`AdmissionScope.CustomerAccount`，不另解析引用串），按（租户、`CUSTOMER_COMMAND`、`SHIPMENT_REQUEST_SUBMISSION`）读对照，经 `ResolveAdmissionObjectScope` 取判断时刻生效的那一行。

- 认不出租户、册上没有行、行未到生效时刻或已结束：答缺席（`false`，无错误），编排照旧答`权威未确定`。
- 对照读不动，或同键两行同时生效（绕过登记口的直插）：答错误，不折成缺席、不任选一行。
- 对象范围上此刻有没有本产品的区间、暂停是否生效，仍由 `ProductionOwnershipAdapter` 读治理登记册判，目录不管。

**四、试点范围版本取对照的对象范围本身**（`NewGovernanceScopeOfObjectScope`）。不设第二个来源。

**五、装配。** `buildProductionOwnership` 在不设写开关时装生产目录；设了写开关仍装注入式合成目录（过渡期，写开关随 operator-channel/53 退场，ADR-0150 决定三），两形态各有用例。权威串未设时适配器在读目录之前就早退（ADR-0180 决定三，不变）。合成坐标（`SYN-CAP/…`）与写开关代码本记录不删。

**六、演示种子。** 演示租户在这一对词下登一行对照（`data/access/17`）与本产品的一段权威区间（`data/governance/52`），对象范围沿用委托受理那一个试点范围 `SYN-PILOT-SCOPE/shipment-intake@v1`、权威串 `SYN-AUTH/idp-parcel-pilot`。不设写开关、设权威串时，演示租户经首方口提交答 `SUBMITTED`。

## 对已接受记录的作用

不部分停用任何已接受记录。ADR-0180 越权风险点 6 留的这一格由本记录回答，该记录正文不改，只在 Status 索引注记与 Links 加前向指针；决定四第一项「运营决定」「外部事实」两族的词与本记录无关。

## Consequences

- 生产形态下，登了对照与区间的租户经首方口提交不再停在`权威未确定`；对照或区间少一样，仍答`权威未确定`，不代拟。
- 写开关（读开关归 operator-channel/53）的退场前件齐了：目录有了生产实现，剩下是删 `isolatedWriteAdmission`、`IsolatedGovernanceScopeDirectory` 与两开关一致性校验，以及种子 `data/governance/07` 的合成区间。
- 登记指引（参数登记册「准入范围对照」一行）要写明：委托提交的对照登的是该租户全部委托受理落入的对象范围，且对照行存在不等于铸造侧会判它。

## 越权风险点

1. **键的粒度是租户，不是客户账户。** 一个租户下所有客户账户共用一行对照、同一个对象范围。ADR-0180 越权风险点 2 的同一问题落到委托受理：要按客户账户分批上线，得让对照键带账户，另立。
2. **「试点范围版本 = 对象范围」是一条假设。** 今天登记册里两者同值；治理侧日后若让暂停指向与区间对象范围不同的引用，本记录决定四要改，改法是另给来源（对照册多一列或登记册里登一条映射），目录的接口不变。
3. **设了写开关时，开关租户之外的租户即使登了对照也答`权威未确定`。** 过渡期两形态互斥：设开关装注入式目录，只答开关租户。这是对 ADR-0091 决定六的保守读法，随写开关退场自然消失；若演示部署要两者并用，得另出一个组合目录，本记录不做。
4. **`CUSTOMER_COMMAND` 下混着两类口。** 铸造侧判准入的预报、交接确认口与本记录这个不在铸造侧判的口，用同一个能力词、同一本对照册。登记者可能以为登了对照就会被铸造侧判。边界靠 `customerCapabilitiesAdmittedAtMint` 与 `ShipmentRequestSubmissionAdmission` 两处分开表达，没有第三道校验。
5. **权威串是部署参数，目录对它不知情。** 生产部署若没设 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY`，对照与区间登得再齐也答`权威未确定`，表现与「没登」相同（ADR-0180 决定三已定，此处只提醒上线核对）。
6. **没读的部分。** ADR-0139 全文、ADR-0025 全文、暂停相关迁移；与它们原文有出入处以原文为准，本记录让位。

## Links

- [ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)：越权风险点 6 与决定四，本记录回答它
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：决定三，写开关退场的时机与前件
- [ADR-0141](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md)：决定四「不许写一个直接返回已确定的假目录」，生产目录读册换坐标、归属仍由适配器按区间判
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：决定二，词表归产品策略
- 票 [first-party-api/04](../../.scratch/first-party-api/issues/04-ps-production-ownership-directory-before-the-write-switch-retires.md)
