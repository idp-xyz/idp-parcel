# party-commercial 声明与发布无登记口,接受链六墙同根等一扇门

Category: enhancement
Status: needs-triage

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W04–W08、W11(声明面)、W12。

## 墙(六处哨兵,同根)

- `COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED` / `PC-ACCEPTANCE_CONTENT_NOT_CONFIGURED`(`parcelshipment/adapters/partycommercial/commercial_basis.go`)
- `ANCHOR_POLICY_NOT_CONFIGURED`(`partycommercial/domain/commercial_resolution.go`)
- `NOT_CONFIGURED`(`JudgmentAsOfOutcome`)、`REACHABILITY_AS_OF_NOT_CONFIGURED`、`FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`(`judgment_continuation.go`)
- `REJECTION/WITHDRAWAL/SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED`、`RULES_NOT_CONFIGURED`(`AuthorizationOutcome`)
- `CONTROL_POLICY_NOT_CONFIGURED`(`settlementaccounting/application/apply_pre_acceptance_control.go`)
- `ELIGIBILITY_UNDECIDED` 资格声明面 / `FINAL_UNDECIDED` 终局规则面(`adopt_network_intake.go`、`form_parcel_final.go`;装配点注释「不得为变绿去种 PAR-COM-16/17 声明行」)

## 现状:有装载无写入

party_commercial 迁移 0002–0014 的表与只读装载口全部就位,`resolve_commercial_basis` 解析用例真实可用;但**发布侧没有用例**:

- 六张声明表零 INSERT(非测试代码):`as_of_policy_declaration`、`acceptance_content_declaration`、`pre_acceptance_control_declaration`、`customer_contract_content`、`stage_content_declaration`、`acceptance_rule_package`。
- `commercial_version` / `commercial_resolution` / `authorization_grant` / `service_product_form` / `price_policy` / `settlement_policy` 有仓储级写入方,但无发布用例、无进程入口。
- 当前唯一填充路径是测试内隔离种子(`cmd/parcel-dispatch/syn_pc_seed_test.go`,SYN-RES-01,S 级)。

## 缺的最小机制件

1. 商业发布用例(publication):按批发布版本化声明(PAR-COM-14/15/16/17 的机制半边),写 `commercial_version` + 各 kind 声明表,带发布批准责任与有效区间;不可覆盖既有版本。
2. 消费方解析键来源(`ResolutionKeySource`)的实例登记面:范围/法人候选/锚点策略/必需依据种类(今天 nil 即显式未配置)。
3. 进程级登记口(端点或受控 CLI)。

另两条缝在本票范围外注明:控制金额缝(估价,依赖票 07)、控制作用域缝(ADR-0044 已让结算政策可观察,接通归本票)。

## 红线

- 声明内容全部属实例半边:本票只建发布机制,不种任何生产默认;隔离 S 种子继续只活在测试里。
- 一决策一处定义:发布用例引用 PAR-COM-* 登记册,不复制第二套参数口径。

## 参照

ADR-0027、ADR-0044、ADR-0058、ADR-0062;PAR-COM-14/15/16/17。
