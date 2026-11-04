# ADR-0362：商业发布审批职责规则经 parcel-commercial 的受控命令行登记，一租户一条、撞键不覆盖

Status: Accepted（2026-11-04，Claude Code 会话代 owner 决，用户授权代决；票 approval-duty-rule-entry/01。裁决能力边界：读过 ADR-0126 决定三与五、ADR-0101 决定五与六、ADR-0085 决定四、`ApprovalDutyRuleRegistry` 端口与其 Postgres 实现、`parcel-commercial` 既有子命令的形状；越权风险点见文末。部分停用 [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定五「登记面归治理写面那一族按 ADR-0085 决定四另裁」一句。）<!-- 索引注记 -->｜**入口**：ADR-0126 立了审批职责规则的表与读口，登记入口留待治理写面那一族另裁，参数登记册 `PAR-COM-18` 一直是「待提供」，任何租户的批准门都只能答未配置。**取法**：登记入口就是 `parcel-commercial register-approval-duty-rule`，与本上下文其余受控登记同一族；文件形状归产品定，须否不同主体必填、等级可缺；一租户一条，同内容重放、异内容冲突不覆盖；治理 HTTP 写面仍不在本记录
Date: 2026-11-04

## Context

- ADR-0126 决定三让批准经待批准载体推进，批准门按租户的审批职责规则判；决定五把规则的登记入口留给治理写面那一族（ADR-0085 决定四）另裁。于是机制半边齐了——表 `party_commercial.publication_approval_duty_rule`、端口 `ApprovalDutyRuleRegistry`、Postgres 实现——却没有任何进程口调它，写口「只给装配与测试用」。
- 结果是每个租户的批准门都停在`未配置`：管理台右栏「批准」按下去，服务端如实答「租户还没有登记审批职责规则」。演示租户也是，审批与发布页因此演示不到成功路径。
- ADR-0085 决定四把治理登记册排在 HTTP 写面首批之外，理由是治理动作的操作者授权模型特殊。它排除的是 HTTP 写面，不是受控命令行：本上下文的其余登记（发布批、参与方身份、法人资料、收尾）都已经走 `parcel-commercial`，演示种子也从这条路灌。

## 候选与反方

- **等治理 HTTP 写面裁完再开口。** 否决：那一族何时裁没有时间表，而缺口今天就让所有租户的批准门关着；受控命令行是本上下文登记入口的既有形状，不是另立一条路。
- **直接写库（演示用 SQL）。** 否决：演示租户按真实租户对待（ADR-0150），它的取值要经产品的登记口进；写库绕过构造门与重放 / 冲突判据，演示出来的就不是产品。
- **给演示租户在代码里内置一条规则。** 否决：红线两个方向都禁替租户定治理规则（ADR-0101 决定六、ADR-0126 候选）。
- **受控命令行登记（采纳）。**

## Decision

**一、登记入口是 `parcel-commercial register-approval-duty-rule -input <file>`。**（机制）

- 文件形状归产品定（ADR-0101）：`tenantId`、`requiresDistinctSubjects`（必填——须否不同主体是规则的正文，缺席不读成「不要求」）、`requiredApproverLevel`（可缺，缺即不要求等级）；未知键拒。
- 一租户一条：首次落册退 0，同内容重放退 0，异内容撞键答冲突、退 2、册上原样——改规则今天仍没有入口，与载体册同一条纪律（ADR-0031）。
- 入口只做翻译与交仓储，不裁任何治理判断；批准门照旧按读到的规则判（ADR-0126 决定三）。

**二、规则的取值归租户，演示租户登自己的一条。**（产品策略 / 租户取值）

- 产品不随发布带任何规则；参数登记册 `PAR-COM-18` 对真实租户仍是「待提供」，只把入口记上。
- 演示租户 `SYN-TENANT-01` 在种子里登一条合成取值（单主体可批、不要求等级），证据记 `S`：它只有一个配置操作者账号，要求不同主体就没人能批。这是演示租户自己的治理取值，不是产品默认。

**三、治理 HTTP 写面不在本记录。** ADR-0085 决定四那一族裁定时，本入口照旧可用；两口共用同一份翻译与仓储判据。

## Consequences

- `cmd/parcel-commercial` 多一个子命令与两条用例（翻译；对库的重放与冲突）；演示种子多一步。
- 管理台审批与发布页在演示租户上能走完「批准 → 发布」；真实租户在登记规则之前照旧答未配置。
- 参数登记册 `PAR-COM-18` 的状态改写为「机制已有入口；真实租户待提供」。

## 越权风险点

1. 把登记入口定在受控命令行而不是治理 HTTP 写面：若 owner 认为治理参数必须经治理写面登记，本入口应随那一族的裁决收口为种子与运维专用。
2. 演示租户「单主体可批」的取值：它只服务于演示能走通，若 owner 认为演示也该示范双人审批，需另加一个批准者账号并改种子。

## Links

- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：决定三立批准门与规则；决定五把入口留待另裁——本记录部分停用那一句，0126 的 Status 行与 Links 已挂前向指针。
- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)：载荷形状归产品定；审批职责规则缺省朝拦。
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：决定四把治理登记册排在 HTTP 写面首批之外。
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：演示租户按真实租户对待。
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：仓储写落点是封闭代数，重放与冲突各一格。
