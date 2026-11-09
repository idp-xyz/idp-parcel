# 操作者与授予进管理台：操作者接入管理面、HTTP 写面与操作者页

Category: enhancement
Status: ready-for-human——2026-11-09 剩余子票全部冻结到首个租户上线之后（[上线线与冻结清单](../launch-line-freeze-2026-10-27.md)），本专项没有在做的票：01、02 已 resolved；03 同日补进冻结清单「未进那次分诊的」一表；解冻照冻结清单「二」的条件（首个租户真实用到，或用户点名），解冻时父票回 in-progress；用户答「按你的建议全部收掉吧，你是业务和系统专家，你来决定」，会话分诊。此前 in-progress
出处：用户 2026-11-04 问「现在我们很多的实现还在 cli 上，是吗」；盘点后建议按「访问身份 → 治理 → 结算 → 代收与清关 → 商业三小类」的顺序收口，用户答「按你的建议开始吧」。本规格是第一项。

现状：操作者册（ADR-0100 决定二）有写口 `OperatorRegistrar` 与按主体读口 `FindOperator`，HTTP 上只有自查（`/operator-binding`、`/operator-grants`，ADR-0315）与作业设备册查阅。登记操作者、授予、撤销只能跑 `parcel-access-register`；没有按租户列操作者的读口；没有管理台页面。给一个租户上线，第一步要配的东西全靠实施方跑命令行。

切片：横切运营面（操作者渠道，不属任何 PN），主责 `internal/accessidentity`（共享接入身份能力，不是限界上下文，无 CONTEXT.md）。权威：ADR-0100（操作者册的列由产品定、行归租户）、ADR-0085（写面进端点表，治理登记册另裁）、ADR-0315（自查读口只管呈现）、ADR-0101（载荷形状归产品定）。

## 决定（ADR-0363）

1. **新能力面 `OPERATOR_ACCESS_ADMINISTRATION`（操作者接入管理）**：可授；守操作者登记、授予登记、撤销登记三个写面与本租户操作者册的查阅读。它与预留的 `GOVERNANCE_REGISTRATION`（试点治理册，无租户维）不是一格：本面有租户维，管的是本租户自己的操作者。
2. **写面进端点表**：`POST /operator-registrations`、`POST /operator-grant-registrations`、`POST /operator-grant-revocations`；读面 `GET /operators`（本租户的操作者及其授予与撤销记录，外加产品词表：可授能力面、决定种类、作业范围种类，与本进程信任的发行方）。载荷形状照 CLI 的单项、去掉 `tenantId`（租户只取自信封）；未知键拒。答复沿用 `OperatorRegistrationOutcome` 代数。
3. **防锁死（机制）**：撤销若落在调用者自己此刻生效的操作者接入管理授予上，答 `REVOCATION_WOULD_LOCK_OUT_CALLER`，不写。
4. **首个管理授予经受控命令行登记（产品策略）**：租户的第一个操作者与它的管理授予由实施方经 `parcel-access-register` 登；之后在管理台做。CLI 保留为受控批量口（ADR-0100 决定六）。
5. **管理台「操作者与授予」页**（设置组）：工作台列操作者，右栏显绑定与授予（生效中 / 未到 / 已到期 / 已撤销），授予就地登记、撤销用对话框，「新增操作者」是登记视图；没有管理授予的人不摆入口（ADR-0315 同款）。读口答未授予时状态区显示本人的发行方与主体标识，便于交给管理员登记。
6. **演示租户**：`SYN-TENANT-01` 的配置操作者在种子里多一条操作者接入管理授予（S）。

机制：能力面、端点、读口、防锁死。产品策略：首个授予走 CLI、页面形态。租户取值：各租户的操作者与授予行（演示租户的那几行记 S）。

## 子票

| 票 | 内容 | 状态 |
|---|---|---|
| [01](./issues/01-access-administration-face-and-write-endpoints.md) | 后端：能力面、迁移、按租户读口、三个写端点与读端点、防锁死、测试、种子、ADR、参数登记册 | resolved（2026-11-04 进 main） |
| [02](./issues/02-operators-page.md) | 管理台：「操作者与授予」页、未授予状态区显主体标识 | resolved（2026-11-04 进 main） |
| [03](./issues/03-commercial-small-registrations.md) | 商业三小类（解析键、注册号类型、审批职责规则）的 HTTP 写面与登记面 | draft |
