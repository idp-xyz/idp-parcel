# 09 customs-compliance 十六册迁到 ADR-0144

Category: enhancement
Status: ready-for-agent——迁移编号预留 customs-compliance `0033`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/customscompliance/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/customs_compliance` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/customs-case-records` | `CustomsCaseRecordRead.ListCustomsCases` | `c.case_id`（另有按案件的窄口参数） |
| `/customs-submission-records` | `SubmissionRecordRead.ListSubmissionVersions` | `unit_id, procedure_ref, fixed_at, version_id` |
| `/customs-external-result-records` | `ExternalResultViewRead.ListExternalResults` | `recorded_at, source_id` |
| `/customs-action-admission-judgements` | `ActionAdmissionJudgementRead.ListActionAdmissionJudgements` | `judged_at DESC, judgement_id` |
| `/customs-unadmitted-objects` | `UnadmittedObjectRead.ListUnadmittedObjects` | `action_at, object_ref`，带计数与 `truncated` |
| `/customs-compliance-rules`（两种） | `RuleCatalogueRead.ListCaseRequirementRules` / `ListInterpretationRules` | `jurisdiction_ref, direction, procedure_ref` / `result_layer, jurisdiction_ref, applies_from DESC` |
| `/customs-case-registers`（三种） | `CaseRegisterCatalogueRead.ListReadinessJudgments` / `ListSubmissionAuthorities` / `ListClosureObligations` | `unit_id` / `unit_id` / `case_ref` |
| `/customs-gate-conditions` | `GateConditionCatalogueRead.ListGateConditions` | `scope_ref, action, boundary_ref` |
| `/customs-ports-paths`（两种） | `PortsPathsCatalogueRead.ListCandidatePorts` / `ListDeclarationPaths` | `port_ref, applies_from DESC` / `path_ref, applies_from DESC` |
| `/customs-credentials` | `CredentialCatalogueRead.ListCredentials` | `credential_id` |
| `/customs-duty-collaborations` | `DutyCollaborationCatalogueRead.ListDutyCollaborations` | `scope_ref, duty_ref` |
| `/customs-duty-verifications` | `DutyVerificationCatalogueRead.ListDutyVerifications` | `duty_ref, funds_ref, scope_ref, verified_at, version_digest` |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 今天首列与决胜键方向不一致的（`applies_from DESC` 那几册）照取舍 2 改为同向，判断项点名。
- 按案件、按版本的窄口参数登为筛选维或选择器，不留成未声明的键（决定四「未知参数键即拒」）。
- 未准入对象带计数，照取舍 5。
