# 领域类型零生产消费者普查（探针，扔弃件）

导出领域类型共 1127 个，其中零生产消费者 122 个。

| 上下文 | 导出领域类型 | 零消费者 | 其中有自身测试 |
|---|---|---|---|
| collectionremittance | 36 | 0 | 0 |
| customscompliance | 100 | 22 | 5 |
| networkrouting | 89 | 5 | 5 |
| nodeoperations | 37 | 0 | 0 |
| parcelpricing | 82 | 4 | 1 |
| parcelshipment | 223 | 24 | 13 |
| partycommercial | 134 | 13 | 8 |
| pilotgovernance | 31 | 0 | 0 |
| settlementaccounting | 186 | 8 | 6 |
| transportfulfillment | 113 | 33 | 23 |
| visibilityexception | 96 | 13 | 1 |

## 逐个点名


### customscompliance（22）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `AssessedDutyReference` | struct | 0 | internal/customscompliance/domain/duty_release.go |
| `ComplianceJudgment` | struct | 0 | internal/customscompliance/domain/compliance_judgment.go |
| `ComplianceJudgmentSpec` | struct | 2 | internal/customscompliance/domain/compliance_judgment.go |
| `ComplianceRuleVersionReference` | struct | 0 | internal/customscompliance/domain/compliance_judgment.go |
| `ComplianceTopicReference` | struct | 0 | internal/customscompliance/domain/compliance_judgment.go |
| `CredentialHolderReference` | struct | 0 | internal/customscompliance/domain/compliance_judgment.go |
| `CredentialID` | struct | 0 | internal/customscompliance/domain/compliance_judgment.go |
| `CustomsReleaseOutcome` | struct | 0 | internal/customscompliance/domain/duty_release.go |
| `DutyCollaborationSpec` | struct | 3 | internal/customscompliance/domain/duty_collaboration.go |
| `DutyCoverage` | 基础/别名 | 0 | internal/customscompliance/domain/duty_release.go |
| `DutyDelta` | 基础/别名 | 0 | internal/customscompliance/domain/duty_release.go |
| `DutyFactValidity` | 基础/别名 | 0 | internal/customscompliance/domain/duty_release.go |
| `DutyObligationKind` | 基础/别名 | 1 | internal/customscompliance/domain/duty_collaboration.go |
| `DutyPaymentCollaboration` | struct | 0 | internal/customscompliance/domain/duty_collaboration.go |
| `DutyPaymentVerification` | struct | 0 | internal/customscompliance/domain/duty_release.go |
| `ExternalFundsFactReference` | struct | 0 | internal/customscompliance/domain/duty_release.go |
| `JudgmentMode` | 基础/别名 | 1 | internal/customscompliance/domain/compliance_judgment.go |
| `LegalObligorReference` | struct | 1 | internal/customscompliance/domain/duty_collaboration.go |
| `PaymentRequirementSource` | struct | 0 | internal/customscompliance/domain/duty_collaboration.go |
| `RegulatoryCredential` | struct | 0 | internal/customscompliance/domain/compliance_judgment.go |
| `ReleaseKind` | 基础/别名 | 0 | internal/customscompliance/domain/duty_release.go |
| `ResponsibilityTargetReference` | struct | 0 | internal/customscompliance/domain/duty_collaboration.go |

### networkrouting（5）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `CriterionScore` | struct | 1 | internal/networkrouting/domain/route_ranking.go |
| `HardConstraintFindingSpec` | struct | 6 | internal/networkrouting/domain/hard_constraint.go |
| `PathExecutabilitySpec` | struct | 3 | internal/networkrouting/domain/logical_path.go |
| `RouteRequirementSpec` | struct | 3 | internal/networkrouting/domain/route_requirement.go |
| `ServiceAreaResolutionSpec` | struct | 4 | internal/networkrouting/domain/service_area.go |

### parcelpricing（4）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `PricingPlanID` | struct | 0 | internal/parcelpricing/domain/value_objects.go |
| `RateTableID` | struct | 0 | internal/parcelpricing/domain/value_objects.go |
| `ReferenceSeriesRegistrationSpec` | struct | 2 | internal/parcelpricing/domain/reference_series_register.go |
| `WeightPolicyID` | struct | 0 | internal/parcelpricing/domain/value_objects.go |

### parcelshipment（24）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `AuthoritativeCutoffBoundary` | struct | 1 | internal/parcelshipment/domain/continued_attempt.go |
| `ChannelCandidateCost` | struct | 7 | internal/parcelshipment/domain/channel_candidate_cost.go |
| `ChannelCandidateID` | struct | 0 | internal/parcelshipment/domain/channel_candidate_cost.go |
| `ChannelCostCurrency` | struct | 0 | internal/parcelshipment/domain/channel_candidate_cost.go |
| `ChannelCostUnavailability` | 基础/别名 | 1 | internal/parcelshipment/domain/channel_candidate_cost.go |
| `ClosureResponsibilitySourceReference` | struct | 0 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptAuthorityRoleReference` | struct | 1 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptAuthoritySnapshot` | struct | 1 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptDecision` | struct | 0 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptDecisionID` | struct | 0 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptDecisionKind` | 基础/别名 | 0 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptDecisionSpec` | struct | 15 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptJudgment` | 基础/别名 | 0 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptReasonReference` | struct | 1 | internal/parcelshipment/domain/continued_attempt.go |
| `ContinuedAttemptRegister` | struct | 1 | internal/parcelshipment/domain/continued_attempt_register.go |
| `HandoffAttemptID` | struct | 0 | internal/parcelshipment/domain/production_handoff.go |
| `HandoffObservation` | 基础/别名 | 4 | internal/parcelshipment/domain/production_handoff.go |
| `HandoffQueryReference` | struct | 3 | internal/parcelshipment/domain/production_handoff.go |
| `HandoffUnresolvedReason` | 基础/别名 | 1 | internal/parcelshipment/domain/production_handoff.go |
| `RehydrateContinuedAttemptRegisterSpec` | struct | 1 | internal/parcelshipment/domain/continued_attempt_register.go |
| `SafeHandoffAssessment` | struct | 0 | internal/parcelshipment/domain/production_handoff.go |
| `SafeHandoffAssessmentSpec` | struct | 21 | internal/parcelshipment/domain/production_handoff.go |
| `SafeHandoffStatus` | 基础/别名 | 0 | internal/parcelshipment/domain/production_handoff.go |
| `SubmissionBatchCandidate` | struct | 0 | internal/parcelshipment/domain/submission_batch_candidate.go |

### partycommercial（13）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `ChannelAccountBusinessStanding` | 基础/别名 | 0 | internal/partycommercial/domain/channel_account_use_authorization.go |
| `ChannelAccountID` | struct | 1 | internal/partycommercial/domain/channel_account_use_authorization.go |
| `ChannelAccountRevocationBasisReference` | struct | 0 | internal/partycommercial/domain/channel_account_use_authorization.go |
| `ChannelAccountTechnicalStanding` | 基础/别名 | 0 | internal/partycommercial/domain/channel_account_use_authorization.go |
| `ChannelAccountUseAuthorization` | struct | 1 | internal/partycommercial/domain/channel_account_use_authorization.go |
| `ChannelAccountUseAuthorizationStatus` | 基础/别名 | 0 | internal/partycommercial/domain/channel_account_use_authorization.go |
| `ChargeTypeReference` | struct | 0 | internal/partycommercial/domain/credit_policy.go |
| `CreditBasis` | struct | 1 | internal/partycommercial/domain/credit_policy.go |
| `CreditPolicy` | struct | 4 | internal/partycommercial/domain/credit_policy.go |
| `CreditPolicyQuery` | struct | 1 | internal/partycommercial/domain/credit_policy.go |
| `ProductChannelMapping` | struct | 2 | internal/partycommercial/domain/service_product.go |
| `Resolution` | struct | 1 | internal/partycommercial/domain/commercial_resolution.go |
| `SupplierAgreement` | struct | 2 | internal/partycommercial/domain/supplier_agreement.go |

### settlementaccounting（8）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `AuditedPayable` | struct | 2 | internal/settlementaccounting/domain/supplier_bill.go |
| `CreditNoteID` | struct | 0 | internal/settlementaccounting/domain/supplier_bill.go |
| `CreditNoteVersion` | struct | 0 | internal/settlementaccounting/domain/supplier_bill.go |
| `CreditReasonReference` | struct | 1 | internal/settlementaccounting/domain/supplier_bill.go |
| `PayableID` | struct | 1 | internal/settlementaccounting/domain/supplier_bill.go |
| `SupplierCreditNote` | struct | 1 | internal/settlementaccounting/domain/supplier_bill.go |
| `SupplierCreditNoteSpec` | struct | 7 | internal/settlementaccounting/domain/supplier_bill.go |
| `SupplierExpectedCostSpec` | struct | 4 | internal/settlementaccounting/domain/supplier_expected_cost.go |

### transportfulfillment（33）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `ActualFulfillmentSegment` | struct | 1 | internal/transportfulfillment/domain/actual_fulfillment_segment.go |
| `ChargeOccurrenceReason` | 基础/别名 | 2 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `ChargeOccurrenceReference` | struct | 0 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `DispatchTask` | struct | 2 | internal/transportfulfillment/domain/dispatch_task.go |
| `DispatchTaskKind` | 基础/别名 | 1 | internal/transportfulfillment/domain/dispatch_task.go |
| `DispatchTaskSpec` | struct | 9 | internal/transportfulfillment/domain/dispatch_task.go |
| `FulfillmentParticipation` | struct | 0 | internal/transportfulfillment/domain/actual_fulfillment_segment.go |
| `FulfillmentSegmentReference` | struct | 0 | internal/transportfulfillment/domain/actual_fulfillment_segment.go |
| `GateClearanceReference` | struct | 0 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `HandoverScopeSummary` | struct | 0 | internal/transportfulfillment/domain/transport_handover.go |
| `LoadAssignment` | struct | 1 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `LoadAssignmentSpec` | struct | 7 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `LoadAssignmentVersion` | struct | 1 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `MovementFactKind` | 基础/别名 | 4 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `MovementFactReference` | struct | 0 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `MovementFactSpec` | struct | 2 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `MovementFactVersion` | struct | 0 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `MovementLocationReference` | struct | 0 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `MovementSourceReference` | struct | 0 | internal/transportfulfillment/domain/load_assignment_movement.go |
| `OccurrenceBasisReference` | struct | 2 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `OccurrenceRevisionKind` | 基础/别名 | 1 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `OccurrenceScopeReference` | struct | 0 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `OccurrenceValidityVersion` | struct | 1 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `ParticipationEndKind` | 基础/别名 | 2 | internal/transportfulfillment/domain/actual_fulfillment_segment.go |
| `ParticipationEntryKind` | 基础/别名 | 1 | internal/transportfulfillment/domain/actual_fulfillment_segment.go |
| `PlannedSegmentReference` | struct | 11 | internal/transportfulfillment/domain/actual_fulfillment_segment.go |
| `ProcurementLegalEntityReference` | struct | 1 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `ServiceConditionReference` | struct | 1 | internal/transportfulfillment/domain/dispatch_task.go |
| `TaskClosureBasisReference` | struct | 2 | internal/transportfulfillment/domain/dispatch_task.go |
| `TaskState` | 基础/别名 | 2 | internal/transportfulfillment/domain/dispatch_task.go |
| `TransportChargeOccurrence` | struct | 1 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `TransportChargeOccurrenceSpec` | struct | 14 | internal/transportfulfillment/domain/transport_charge_occurrence.go |
| `TransportMovementFact` | struct | 1 | internal/transportfulfillment/domain/load_assignment_movement.go |

### visibilityexception（13）

| 类型 | 形态 | 自身测试提及 | 声明处 |
|---|---|---|---|
| `CasePhase` | 基础/别名 | 0 | internal/visibilityexception/domain/exception_case.go |
| `ConflictJudgment` | struct | 0 | internal/visibilityexception/domain/fact_conflict.go |
| `ConflictResolutionBasis` | 基础/别名 | 0 | internal/visibilityexception/domain/fact_conflict.go |
| `EvidenceAppraisal` | 基础/别名 | 0 | internal/visibilityexception/domain/evidence.go |
| `EvidenceContentDigest` | struct | 0 | internal/visibilityexception/domain/evidence.go |
| `EvidenceDisclosureVersion` | struct | 0 | internal/visibilityexception/domain/evidence.go |
| `EvidenceItem` | struct | 1 | internal/visibilityexception/domain/evidence.go |
| `EvidenceItemID` | struct | 0 | internal/visibilityexception/domain/evidence.go |
| `EvidenceProviderReference` | struct | 0 | internal/visibilityexception/domain/evidence.go |
| `ExceptionCase` | struct | 0 | internal/visibilityexception/domain/exception_case.go |
| `ExceptionSignal` | struct | 0 | internal/visibilityexception/domain/fact_conflict.go |
| `ImpactScopeReference` | struct | 0 | internal/visibilityexception/domain/exception_case.go |
| `ResponsibleTeamReference` | struct | 0 | internal/visibilityexception/domain/exception_case.go |
