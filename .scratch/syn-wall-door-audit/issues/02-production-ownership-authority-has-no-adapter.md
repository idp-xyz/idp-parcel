# 生产归属权威端口无生产适配器,治理登记册配好也接不进提交链

Category: enhancement
Status: needs-triage

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W03。

## 墙

`OWNERSHIP_UNRESOLVED` / `ADMISSION_PAUSED`(`parcelshipment/application/submit_shipment_request.go` 的 `SubmitOutcome`)、`AUTHORITY_UNRESOLVED`(`parcelshipment/domain/production_ownership.go`)。UC-PS-001 第二步「生产归属」在 `DecideProductionOwnership` 处停摆。

## 现状:半座桥

- PS 侧:`ports.ProductionOwnershipAuthority` 全库只有接口定义,无任何生产适配器(仅测试替身)。
- 治理侧:pilot-governance 的 `authority_interval` 表、INSERT 写入方(`governance_records.go`)与登记用例 `record_stage_review`、`govern_incident` 都在——PAR-GOV-03 的登记机制半边基本齐。
- 缺口:①两者之间没有桥接适配器(按权威区间+准入控制回答一份 `AdmissionScope` 归谁);②治理登记用例未接任何进程入口(cmd 无治理端点/CLI)。

## 缺的最小机制件

1. PS→pilot-governance 桥接适配器:实现 `ProductionOwnershipAuthority`,读权威区间与暂停/恢复记录,译成 `ProductionOwnershipDecision`(含 `ADMISSION_PAUSED` 一格,注意 `blockedOutcome` 三分)。
2. 治理登记的进程级入口(端点或受控 CLI),让 PAR-GOV-03..07 的实例登记有路可走。

## 红线

- 无权威区间登记时如实答 `AUTHORITY_UNRESOLVED`,不得默认「本产品承接」。
- 治理记录不可覆盖(既有票已钉);本票不改治理语义,只接线。

## 参照

`docs/product/PILOT-PARAMETER-REGISTER.md` PAR-GOV-03..07;UC-PS-001;ADR-0017。
