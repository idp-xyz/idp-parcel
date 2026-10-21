# 36 customs-compliance：提交授权业务角色面的其余各格——拒绝、撤销、再次申请与授权目标绑定

Category: enhancement
Status: needs-triage——2026-10-20 Claude Code 会话随 [18](./18-customs-case-and-declaration-submission-entry.md) 分诊立（用户「你作为业务和系统专家自决……解决所有的问题」授权）
Blocked by: 无
地盘：`internal/customscompliance`（授权确认编排、提交授权册与其迁移、提交受理口）、`cmd/parcel-api` 的授权确认特性文件；`cmd/parcel-customs-register` 的授权命令。
出处：[18](./18-customs-case-and-declaration-submission-entry.md) 分诊裁定末段；[ADR-0227](../../../docs/adr/0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md) 越权风险点 1、3、4、5、6；[UC-CC-004](../../../docs/application/customs-compliance/UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md) 结果契约。

## 现状

18 落了 UC-CC-004 的首发人工确认一格：持授权确认授予的操作者对一个申报单元确认，单元就绪须当前有效，形成提交授权，确认人与所凭决定种类写进授权依据。其余各格没有：

- 拒绝、撤销、待人工决定与再次申请。提交授权册一单元一行，撤销后同一单元再确认只能答原授权已失效。
- 授权目标只绑申报单元。UC-CC-004「每项授权必须绑定关务案件、申报单元、组成版本、正式资料版本、拟提交动作和指定申报渠道」——今天组成、资料、渠道在提交那一刻才进提交版本，授权不知道它们。
- 提交受理口不核单元组成是否都在所属案件的包裹关联里（UC-CC-002 组单元那一步没有生产入口）。
- 授权登记 CLI（`parcel-customs-register authority-grant`）仍能直接写授权行，写进去的授权依据不是业务角色确认，提交执行却同样认它。
- 授权依据只记确认人与决定种类，不记那一笔授予的标识：操作者信封不带授予号。

## 要裁的

1. 授权册是否改成逐次决定一行（请求、批准、拒绝、撤销各成一笔），提交执行按「当前有效且未被使用」取一份。
2. 授权目标绑定到组成、资料、渠道的哪几维；提交执行时目标不一致答什么。
3. 授权登记 CLI 退场、只留给非生产环境，还是登进去的行另带一格来源，让提交执行分得出业务角色确认与运维登记。
4. 是否让信封带出授予号，写进授权依据。

## 完成判据

- UC-CC-004 结果契约里拒绝、撤销、不再有效与再次申请各有执行器与真库用例；提交执行只认业务角色确认形成的授权，或有书面裁定说明 CLI 一路为何保留。
