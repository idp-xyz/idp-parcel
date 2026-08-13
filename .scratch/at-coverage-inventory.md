# AT 覆盖度盘点（机制半边索引）

盘点基线：本文件所在提交（2026-08-13，点名票后第二次重盘；前两盘 `136f79b`、`0cddbba` 同日）。产出方式与口径见下；本文件是 PN-08 候选包索引的机制半边，不是证据登记册。变化摘要见文末[重盘变化摘要](#重盘变化摘要0cddbba)。

## 方法与口径

- ① **已钉住**：精确统计——扫描全仓 `*_test.go` 中注释与测试体内的 `AT-*` 引用（Covers 注释为主），归属到所在测试函数。
- ② **机制候选**：启发式——未被测试引用，且属于机制面较深的上下文（PS/PC/NO/NR/TF/SA），场景行文不含实例/闸门关键词。其中一部分机制已存在只差 Covers 注释，另一部分需要小块新测试；逐条领票时再定。
- ③ **实例半边/闸门阻断**：启发式关键词分桶（真实参数、税率、时区、账期、模拟隔离、批量接入、查询/权限/审计面、文件回调通道），原因标注在行内；依 ADR-0017，接入与持久化面尚在闸门后。
- ④ **机制未落地**：未被测试引用且属于起步期上下文（CC/VE 为主）——不是②「已有机制缺注释」，也未必是③实例半边；等后续切片。
- ②③④ 为启发式分桶，抽样复核过 CC/SA/TF 各十余条；单条归类若与领票时的判断冲突，以领票时对照 UC 原文为准。
- **VE/CC 的①列为零是引用口径所致**：两包既有测试的 Covers 注释引 CONTEXT 硬句原文而非 AT 号（实测 `internal/visibilityexception`、`internal/customscompliance` 全部 `*_test.go` 无一处 `AT-*` 字样），不代表两包无测试或无覆盖；它们的④行里有一部分实际已被硬句测试钉住，逐条领票时按 UC 原文比对。
- **`0cddbba` 重盘补充：硬句引用口径已扩展到首盘之后新落的全部编排批**——VE 五个编排（投影派生、客户视图、信号分诊、客户通知、处置请求）、CC 四个编排（申报提交、外部结果接收、处置核验、案件关闭）、PG 阶段评审、PP 计价评价编排、SA 费用确认、TF 交付生效、NO 关务协作领域件的新测试同引硬句不点 AT 号（增量 48 个测试文件仅 2 处新增 `AT-*` 点名，且都点在已在册行上）。①因此低估的范围从 VE/CC 扩大到上述新编排面；对应 ②④ 行的高估同步扩大，逐条领票时仍按 UC 原文比对，不据本表宣称未覆盖。
- **点名票后修正（第二次重盘）**：VE 五编排与 NO 关务协作的测试已逐条对 UC 验收表补 AT 点名（宁缺勿滥——语义整条对上才点，半边覆盖在点名处注明「××半边」；点不上的继续引硬句），VE ①由 0 升至 16、NO ①由 10 升至 13。**SA 费用确认与 CC 放行门禁两组测试对表后零点名**：其所钉语义（确认条件定格、五值折叠、条件指纹换版、目录未登记未决）落在各自 UC 的结果契约与 CONTEXT 硬句上，验收表无一一对应行——不硬点。VE/CC 其余包的硬句低估口径仍然成立。
- **第二轮点名票后修正**：CC 六编排（建案/舱单/后续动作/处置核对/案件关闭/内部限制）、TF 三登记编排、NO 合箱也已对表点名，**CC 的①列不再为零**（0→20）；PP 与 PG 在 `docs/application` 无 AT 清单（PP 无 UC 文档、PG 走 PN-08 验收矩阵 GOV-*，两处 GOV 机制半边已在 PG 测试注释点名但不入本表）；PC 声明两测试与 SA 费用确认维持零点（无对应行）。硬句低估口径对其余未点名面继续成立。

## 总览

| 桶 | 数量 |
|---|---|
| ① 已钉住 | 284 |
| ② 机制候选 | 227 |
| ③ 实例/闸门 | 147 |
| ④ 机制未落地 | 425 |
| 合计 | 1083 |

| 前缀 | 总数 | ① | ② | ③ | ④ |
|---|---|---|---|---|---|
| CC | 410 | 20 | 0 | 89 | 301 |
| NO | 46 | 16 | 23 | 7 | 0 |
| NR | 53 | 24 | 28 | 1 | 0 |
| PC | 36 | 35 | 1 | 0 | 0 |
| PS | 93 | 66 | 25 | 2 | 0 |
| SA | 178 | 66 | 90 | 22 | 0 |
| TF | 97 | 30 | 60 | 7 | 0 |
| VE | 170 | 27 | 0 | 19 | 124 |

## ① 已钉住（AT → 测试）

| AT | 测试（文件::函数） |
|---|---|
| `AT-CC-008` | internal/customscompliance/application/establish_customs_case_test.go::TestCaseRequirementGapsAndRefusalsStayDistinct |
| `AT-CC-009` | internal/customscompliance/application/establish_customs_case_test.go::TestOneCasePerRegulatoryScopeAndNoSilentScopeGrowth |
| `AT-CC-010` | internal/customscompliance/application/establish_customs_case_test.go::TestOneCasePerRegulatoryScopeAndNoSilentScopeGrowth |
| `AT-CC-201` | internal/customscompliance/application/manage_follow_up_test.go::TestFollowUpTargetsProposalsAndEffectsStayDisciplined |
| `AT-CC-202` | internal/customscompliance/application/manage_follow_up_test.go::TestFollowUpTargetsProposalsAndEffectsStayDisciplined |
| `AT-CC-208` | internal/customscompliance/application/manage_follow_up_test.go::TestFollowUpTargetsProposalsAndEffectsStayDisciplined |
| `AT-CC-233` | internal/customscompliance/application/verify_disposition_test.go::TestVerificationIsIdempotentPerFactSetAndVersionsAccrue |
| `AT-CC-237` | internal/customscompliance/application/verify_disposition_test.go::TestVerificationIsIdempotentPerFactSetAndVersionsAccrue |
| `AT-CC-250` | internal/customscompliance/application/verify_disposition_test.go::TestFactFailuresStallAndIntentsRetry |
| `AT-CC-307` | internal/customscompliance/application/close_customs_case_test.go::TestClosureIsBlockedItemByItemAndClosesOnceWhole |
| `AT-CC-308` | internal/customscompliance/application/close_customs_case_test.go::TestClosureIsBlockedItemByItemAndClosesOnceWhole |
| `AT-CC-345` | internal/customscompliance/application/manage_restriction_test.go::TestRestrictionsEstablishOnceAndReleaseIdempotently |
| `AT-CC-346` | internal/customscompliance/application/manage_restriction_test.go::TestRestrictionsEstablishOnceAndReleaseIdempotently |
| `AT-CC-347` | internal/customscompliance/application/manage_restriction_test.go::TestActionJudgmentListsEveryBlockerUntilAllRelease |
| `AT-CC-348` | internal/customscompliance/application/manage_restriction_test.go::TestActionJudgmentListsEveryBlockerUntilAllRelease |
| `AT-CC-370` | internal/customscompliance/application/manage_restriction_test.go::TestRestrictionsEstablishOnceAndReleaseIdempotently |
| `AT-CC-380` | internal/customscompliance/application/receive_manifest_test.go::TestManifestsAssociateOnlyOnUniqueMatch |
| `AT-CC-381` | internal/customscompliance/application/receive_manifest_test.go::TestManifestsAssociateOnlyOnUniqueMatch |
| `AT-CC-387` | internal/customscompliance/application/receive_manifest_test.go::TestManifestsAssociateOnlyOnUniqueMatch |
| `AT-CC-389` | internal/customscompliance/application/receive_manifest_test.go::TestRevisionsAdvanceVersionsAndRematchAssociations |
| `AT-NO-008` | internal/nodeoperations/application/accept_collaboration_test.go::TestExecutionFactsAreRecordedWithinTheDecision；internal/nodeoperations/application/accept_collaboration_test.go::TestOneDecisionPerCollaborationItem |
| `AT-NO-009` | internal/nodeoperations/application/accept_collaboration_test.go::TestOneDecisionPerCollaborationItem |
| `AT-NO-012` | internal/nodeoperations/application/accept_collaboration_test.go::TestExecutionFactsAreRecordedWithinTheDecision |
| `AT-NO-015` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestAnExplicitReceptionFormsIntakeAndControl |
| `AT-NO-016` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestReplayConflictAndFailedIntentStayDisciplined |
| `AT-NO-017` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestReplayConflictAndFailedIntentStayDisciplined |
| `AT-NO-018` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestUnknownAndConflictedIdentitiesStayPending |
| `AT-NO-019` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestUnknownAndConflictedIdentitiesStayPending |
| `AT-NO-020` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestAnExplicitReceptionFormsIntakeAndControl |
| `AT-NO-023` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestScanOnlyAndRefusalDoNotEstablishControl |
| `AT-NO-024` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestScanOnlyAndRefusalDoNotEstablishControl |
| `AT-NO-025` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestABatchKeepsPerUnitResultsIndependent |
| `AT-NO-027` | internal/nodeoperations/application/receive_delivered_unit_test.go::TestReplayConflictAndFailedIntentStayDisciplined |
| `AT-NO-035` | internal/nodeoperations/application/consolidate_parcels_test.go::TestOneDirectParentIsEnforcedAcrossUnits |
| `AT-NO-036` | internal/nodeoperations/application/consolidate_parcels_test.go::TestSealingFreezesTheSnapshotAndClosureIsFinal |
| `AT-NO-040` | internal/nodeoperations/application/consolidate_parcels_test.go::TestSealingFreezesTheSnapshotAndClosureIsFinal |
| `AT-NR-001` | internal/networkrouting/application/create_initial_route_test.go::TestThreeParcelsKeepThreeIndependentResults；internal/networkrouting/domain/initial_route_plan_test.go::TestAnInitialRoutePlanCarriesItsFullSelectionBasis；internal/networkrouting/domain/route_ranking_test.go::TestSelectionFollowsTheStrategysDeclaredCriterionOrder |
| `AT-NR-002` | internal/networkrouting/application/create_initial_route_test.go::TestTwoParcelsFormTwoIndependentPlans |
| `AT-NR-003` | internal/networkrouting/application/create_initial_route_test.go::TestAReplayReturnsExistingResultsAndAConflictOverwritesNothing |
| `AT-NR-004` | internal/networkrouting/application/create_initial_route_test.go::TestAConcurrentLoserReadsBackTheWinner |
| `AT-NR-005` | internal/networkrouting/application/create_initial_route_test.go::TestThreeParcelsKeepThreeIndependentResults；internal/networkrouting/domain/initial_route_plan_test.go::TestNoCurrentRouteDemandsAClosedFullyEliminatedSpace |
| `AT-NR-006` | internal/networkrouting/application/create_initial_route_test.go::TestUnknownCustomsEligibilityStaysUndecidedNotNoRoute |
| `AT-NR-009` | internal/networkrouting/application/create_initial_route_test.go::TestAWaybillOnlyServiceIsNotApplicableWithItsBasis |
| `AT-NR-010` | internal/networkrouting/application/create_initial_route_test.go::TestAPreCommitRevisionChangeRejudgesWithTheNewEvidence；internal/networkrouting/application/create_initial_route_test.go::TestARollingViewKeepsTheJudgmentUndecided |
| `AT-NR-011` | internal/networkrouting/application/create_initial_route_test.go::TestAFailedIntentDeliveryIsRetriedWithoutASecondPlan |
| `AT-NR-012` | internal/networkrouting/application/create_initial_route_test.go::TestThreeParcelsKeepThreeIndependentResults |
| `AT-NR-014` | internal/networkrouting/domain/initial_route_plan_test.go::TestAPlanRefusesABrokenSelectionOrChain |
| `AT-NR-016` | internal/networkrouting/domain/reachability_test.go::TestOneQualifiedCandidateIsReachable |
| `AT-NR-017` | internal/networkrouting/domain/reachability_test.go::TestAllCandidatesEliminatedIsUnreachable |
| `AT-NR-018` | internal/networkrouting/domain/service_area_test.go::TestServiceAreaEvaluationFoldsTheMatrixRows |
| `AT-NR-019` | internal/networkrouting/application/assess_parcel_reachability_test.go::TestUnavailableNetworkEvidenceIsNotFormedRatherThanInsufficientEvidence |
| `AT-NR-020` | internal/networkrouting/application/assess_parcel_reachability_test.go::TestSameScopeRetryReturnsTheExistingJudgmentWithoutReassessing |
| `AT-NR-021` | internal/networkrouting/application/assess_parcel_reachability_test.go::TestSameCorrelationWithADifferentScopeIsAConflictAndDoesNotOverwrite |
| `AT-NR-023` | internal/networkrouting/domain/reachability_test.go::TestAllCandidatesEliminatedIsUnreachable；internal/networkrouting/domain/service_area_test.go::TestServiceAreaEvaluationFoldsTheMatrixRows |
| `AT-NR-024` | internal/networkrouting/domain/reachability_test.go::TestPartiallyEliminatedWithUnknownRemainderIsInsufficient |
| `AT-NR-025` | internal/networkrouting/domain/reachability_test.go::TestCandidateScopedGapsElsewhereDoNotBlockReachable；internal/networkrouting/domain/service_area_test.go::TestServiceAreaEvaluationDrivesTheThreeValuedConclusion |
| `AT-NR-026` | internal/networkrouting/application/assess_parcel_reachability_test.go::TestACommittedRequirementFlowsThroughTheAssessment；internal/networkrouting/domain/route_requirement_test.go::TestAPlainPreferenceEliminatesNothing |
| `AT-NR-028` | internal/networkrouting/application/assess_parcel_reachability_test.go::TestAConcurrentWinnerIsReadBackRatherThanOverwritten；internal/networkrouting/application/assess_parcel_reachability_test.go::TestFormedJudgmentTakesItsJudgmentTimeFromTheClockNotTheAsOf |
| `AT-NR-030` | internal/networkrouting/application/assess_parcel_reachability_test.go::TestAFormedJudgmentHandsOffOneIntentClaimedByItsCorrelation；internal/networkrouting/application/assess_parcel_reachability_test.go::TestFormedJudgmentTakesItsJudgmentTimeFromTheClockNotTheAsOf |
| `AT-NR-031` | internal/networkrouting/domain/hard_constraint_test.go::TestACustomsRestrictionEliminatesOnlyItsCandidate |
| `AT-PC-001` | internal/partycommercial/domain/party_relationship_test.go::TestLegalEntityReferenceAndCustomerPartyAreFormedSeparately |
| `AT-PC-002` | internal/partycommercial/domain/commercial_registry_test.go::TestRegisteringTheSameContentTwiceIsAReplay |
| `AT-PC-003` | internal/partycommercial/domain/commercial_registry_test.go::TestSameVersionWithChangedContentConflicts |
| `AT-PC-004` | internal/partycommercial/domain/commercial_registry_test.go::TestNewVersionCoexistsWithTheOneItReplaces；internal/partycommercial/domain/commercial_version_lifecycle_test.go::TestPublishedVersionTakesEffectOnlyAtItsBoundary |
| `AT-PC-005` | internal/partycommercial/domain/commercial_version_test.go::TestPublicationWaitsWhenNamedReferenceIsUnpublished |
| `AT-PC-006` | internal/partycommercial/domain/commercial_resolution_test.go::TestMissingAnchorPolicyIsPendingRatherThanDefaultingToNow；internal/partycommercial/domain/commercial_resolution_test.go::TestZeroMultipleAndUnavailableAreDistinctOutcomes |
| `AT-PC-007` | internal/partycommercial/domain/commercial_resolution_test.go::TestSupersedingContractKeepsHistoryAndSelectsSuccessorUniquely；internal/partycommercial/domain/commercial_resolution_test.go::TestZeroMultipleAndUnavailableAreDistinctOutcomes |
| `AT-PC-008` | internal/partycommercial/domain/commercial_resolution_test.go::TestEndedVersionsLeaveTheCandidateSet |
| `AT-PC-009` | internal/partycommercial/domain/channel_account_use_authorization_test.go::TestTechnicallyAvailableAccountDoesNotPublishWithoutBusinessAuthorization |
| `AT-PC-010` | internal/partycommercial/domain/commercial_version_test.go::TestPublicationWaitsWhenApprovalRoleIsUnconfirmed |
| `AT-PC-011` | internal/partycommercial/domain/publication_batch_test.go::TestPublicationBatchKeepsLegalProductWhenContractConflicts |
| `AT-PC-012` | internal/partycommercial/domain/commercial_version_test.go::TestPublishedCommercialVersionRefusesInPlaceRevision |
| `AT-PC-013` | internal/partycommercial/domain/validity_correction_test.go::TestValidityCorrectionKeepsOriginalAndAdvancesViewRevision |
| `AT-PC-014` | internal/partycommercial/application/form_judgment_as_of_test.go::TestAResolutionNamedByAnotherTenantIsNotAccepted；internal/partycommercial/application/validate_commercial_basis_test.go::TestARevalidationNamedByAnotherTenantIsNotAccepted；internal/partycommercial/domain/commercial_registry_test.go::TestCrossTenantSameObjectVersionNeitherReplaysNorLeaks；internal/partycommercial/domain/party_relationship_test.go::TestCustomerAccountRejectsCrossTenantPartyBinding |
| `AT-PC-015` | internal/partycommercial/domain/service_product_test.go::TestNoServiceProductCanTakeAnIndependentWaybillChannelForm |
| `AT-PC-017` | internal/partycommercial/domain/commercial_resolution_test.go::TestUniqueCandidateResolvesWithItsAdoptedVersion |
| `AT-PC-018` | internal/partycommercial/application/form_judgment_as_of_test.go::TestAJudgmentWithNoDeclaredPolicyStopsInsteadOfDefaultingToNow；internal/partycommercial/domain/commercial_resolution_test.go::TestMissingAnchorPolicyIsPendingRatherThanDefaultingToNow |
| `AT-PC-019` | internal/partycommercial/domain/commercial_resolution_test.go::TestALapsedContractIsNotRevivedByAnEarlierBusinessTime |
| `AT-PC-020` | internal/parcelshipment/adapters/partycommercial/commercial_basis_test.go::TestEveryFirstPhaseAnswerLandsOnItsOwnApplicability；internal/parcelshipment/application/form_acceptance_decision_test.go::TestNoApplicableCommercialBasisRejectsRatherThanStalling；internal/parcelshipment/domain/acceptance_decision_test.go::TestARejectionStandsWithoutACommercialBasis；internal/partycommercial/domain/commercial_resolution_test.go::TestZeroMultipleAndUnavailableAreDistinctOutcomes |
| `AT-PC-021` | internal/parcelshipment/adapters/partycommercial/commercial_basis_test.go::TestEveryFirstPhaseAnswerLandsOnItsOwnApplicability；internal/partycommercial/domain/commercial_resolution_test.go::TestZeroMultipleAndUnavailableAreDistinctOutcomes |
| `AT-PC-022` | internal/partycommercial/domain/commercial_registry_test.go::TestSameVersionReboundToAnotherReferenceConflicts；internal/partycommercial/domain/commercial_version_test.go::TestPublicationWaitsWhenNamedReferenceIsUnpublished；internal/partycommercial/domain/reference_closure_test.go::TestAClosureAdoptsTheRulePackageTheContractNames；internal/partycommercial/domain/reference_closure_test.go::TestAClosureRefusesARulePackageTheContractDoesNotName |
| `AT-PC-023` | internal/partycommercial/application/form_judgment_as_of_test.go::TestEachJudgmentFormsItsOwnAsOfRatherThanSharingOneGlobalTime；internal/partycommercial/domain/as_of_policy_test.go::TestRulePackageDeclaresIndependentAsOfPerJudgment |
| `AT-PC-024` | internal/parcelshipment/adapters/partycommercial/commercial_basis_test.go::TestARevalidationConfirmsTheStandingResolution；internal/partycommercial/application/validate_commercial_basis_test.go::TestAnUnchangedViewLetsThePriorResolutionStandWithItsOriginalIdentity；internal/partycommercial/domain/commercial_resolution_staleness_test.go::TestUnchangedAuthorityViewKeepsThePriorResolutionUsable；internal/partycommercial/domain/commercial_resolution_test.go::TestRepeatedResolutionIsStableUntilTheViewRevisionChanges |
| `AT-PC-025` | internal/parcelshipment/application/form_acceptance_decision_test.go::TestADecisionStopsWhenAReachabilityJudgmentWasSuperseded；internal/parcelshipment/application/form_acceptance_decision_test.go::TestAnAcceptedDecisionSurvivesRetiredBasisOnReplay；internal/partycommercial/application/validate_commercial_basis_test.go::TestAnUnchangedViewLetsThePriorResolutionStandWithItsOriginalIdentity；internal/partycommercial/domain/commercial_resolution_test.go::TestEndedVersionsLeaveTheCandidateSet |
| `AT-PC-026` | internal/parcelshipment/application/form_acceptance_decision_test.go::TestADecisionRevalidatesTheBasisItsJudgmentsWereFormedUnder；internal/parcelshipment/application/form_acceptance_decision_test.go::TestAnAcceptedDecisionSurvivesRetiredBasisOnReplay；internal/parcelshipment/application/form_acceptance_decision_test.go::TestARejectionOnLostBasisStillReleasesAnExistingFreeze；internal/parcelshipment/application/form_acceptance_decision_test.go::TestASupersededBasisIsResolvedAgainInsteadOfRetried；internal/partycommercial/application/validate_commercial_basis_test.go::TestARevalidationSeesANewCandidateAndReportsTheResolutionStale；internal/partycommercial/domain/commercial_resolution_staleness_test.go::TestReResolutionAfterAChangeYieldsANewResolutionIdentity；internal/partycommercial/domain/commercial_resolution_test.go::TestEndedVersionsLeaveTheCandidateSet |
| `AT-PC-027` | internal/parcelshipment/application/form_acceptance_decision_test.go::TestAnUndeterminedCommercialResolutionStaysUndecided；internal/partycommercial/application/validate_commercial_basis_test.go::TestAnUnreadableAuthorityLeavesTheRevalidationPendingRatherThanStale；internal/partycommercial/domain/commercial_resolution_test.go::TestZeroMultipleAndUnavailableAreDistinctOutcomes |
| `AT-PC-028` | internal/partycommercial/application/form_judgment_as_of_test.go::TestAResolutionNamedByAnotherCustomerIsNotAccepted；internal/partycommercial/application/form_judgment_as_of_test.go::TestASecondPhaseProbeCannotTellAMissingResolutionFromOneOwnedByAnotherCustomer；internal/partycommercial/application/validate_commercial_basis_test.go::TestAProbeCannotTellAMissingResolutionFromOneOwnedByAnotherCustomer；internal/partycommercial/application/validate_commercial_basis_test.go::TestARevalidationNamedByAnotherCustomerIsNotAccepted；internal/partycommercial/application/validate_commercial_basis_test.go::TestARevalidationNamedByAnotherTenantIsNotAccepted；internal/partycommercial/domain/party_relationship_test.go::TestCustomerAccountRejectsCrossTenantPartyBinding |
| `AT-PC-029` | internal/partycommercial/domain/commercial_resolution_test.go::TestAResolutionResultHasNowhereToPutAnAcceptanceVerdict |
| `AT-PC-030` | internal/partycommercial/domain/service_product_test.go::TestNoServiceProductCanTakeAnIndependentWaybillChannelForm |
| `AT-PC-031` | internal/partycommercial/domain/settlement_basis_resolution_test.go::TestSettlementBasisCarriesMethodAndScopeAndKeepsDisjointChargeScopesApart；internal/partycommercial/domain/settlement_policy_test.go::TestPrepaidAndTermsCoexistAcrossNonOverlappingScopes |
| `AT-PC-032` | internal/partycommercial/domain/reference_closure_test.go::TestConflictOutranksAMissingBasis；internal/partycommercial/domain/settlement_basis_resolution_test.go::TestSettlementBasisConflictsWhenOneChargeScopeIsHitByBothMethods；internal/partycommercial/domain/settlement_policy_test.go::TestSameScopeHitByBothMethodsIsAConflict |
| `AT-PC-033` | internal/partycommercial/domain/price_policy_test.go::TestSellPolicyCannotBindBuyPlanWithoutDeclaredConversion |
| `AT-PC-034` | internal/partycommercial/domain/price_policy_test.go::TestApprovedBindingKeepsPolicyWhileUnpublishedPlanStaysPending |
| `AT-PC-035` | internal/partycommercial/domain/price_policy_test.go::TestEachDirectionResolvesItsOwnPolicy；internal/partycommercial/domain/reference_closure_test.go::TestClosureAdoptsIndependentPricePoliciesPerDirection |
| `AT-PC-036` | internal/partycommercial/domain/price_policy_test.go::TestAPolicyBoundToAnUnusablePlanDoesNotResolve |
| `AT-PS-001` | internal/parcelshipment/domain/acceptance_decision_test.go::TestAllChecksPassingAcceptsAndFixesBaselineAndCommitment |
| `AT-PS-002` | internal/parcelshipment/domain/acceptance_decision_test.go::TestOneFailingMemberBlocksTheWholeSubmissionVersion |
| `AT-PS-003` | internal/parcelshipment/application/submit_shipment_request_test.go::TestSubmitReplayReturnsExistingResultWithoutASecondRequest |
| `AT-PS-004` | internal/parcelshipment/application/submit_shipment_request_test.go::TestSubmitConflictPreservesTheOriginalWithoutBuilding |
| `AT-PS-005` | internal/parcelshipment/application/form_acceptance_decision_test.go::TestOneUnreachableMemberRejectsTheWholeSubmissionVersion |
| `AT-PS-006` | internal/parcelshipment/application/form_acceptance_decision_test.go::TestAnInsufficientEvidenceJudgmentWaitsOnTheCustomerNotAnInternalRetry；internal/parcelshipment/application/form_acceptance_decision_test.go::TestInsufficientEvidenceLeavesTheRequestSubmittedRatherThanRejected |
| `AT-PS-007` | internal/parcelshipment/adapters/partycommercial/commercial_basis_test.go::TestAUniquelyResolvedClosureBecomesAnAdoptedSnapshot；internal/parcelshipment/domain/judgment_translation_test.go::TestPendingRoutingAllowancePassesAnUnreachableParcel；internal/partycommercial/domain/acceptance_content_test.go::TestPendingRoutingPermissionBelongsToTheServiceProductAndCarriesItsBasis |
| `AT-PS-008` | internal/parcelshipment/application/advance_acceptance_judgment_test.go::TestAnUnavailableAuthorityBecomesPendingAndNeverAJudgement；internal/parcelshipment/application/advance_acceptance_judgment_test.go::TestAnUnavailableCommercialResolverIsPendingUnderItsOwnReason |
| `AT-PS-010` | internal/parcelshipment/application/submit_shipment_request_test.go::TestSubmitFormsNoRequestWhenThisProductLacksAuthority |
| `AT-PS-012` | internal/parcelshipment/adapters/http/submit_shipment_request_test.go::TestSubmitLetsTheApplicationFormInputNotAcceptedRatherThanRejectingItAtTheEdge |
| `AT-PS-013` | internal/parcelshipment/application/form_acceptance_decision_test.go::TestADecisionStopsWhenAReachabilityJudgmentWasSuperseded；internal/parcelshipment/application/form_acceptance_decision_test.go::TestAFormedDecisionHandsOffOneIntentClaimedByTheDecision；internal/parcelshipment/application/form_acceptance_decision_test.go::TestAnUndeliveredDecisionHandoffKeepsTheAcceptanceWithAResumableIntent；internal/parcelshipment/application/form_acceptance_decision_test.go::TestAReplayResendsTheSameDecisionIntentWithoutDecidingAgain |
| `AT-PS-014` | internal/parcelshipment/application/amend_customer_source_data_test.go::TestAnAuthorizedAmendmentFormsAVersionAndDerivesItsAdoption |
| `AT-PS-015` | internal/parcelshipment/domain/customer_source_data_test.go::TestCustomerSourceDataVersionsAppendWithoutOverwritingBaselineOrPriorVersions |
| `AT-PS-016` | internal/parcelshipment/application/amend_customer_source_data_test.go::TestARepeatedAmendmentReturnsTheOriginalVersionWithoutFormingASecond |
| `AT-PS-017` | internal/parcelshipment/application/amend_customer_source_data_test.go::TestAnAmendmentSourceConflictNeitherOverwritesNorFormsASecondVersion |
| `AT-PS-020` | internal/parcelshipment/application/amend_customer_source_data_test.go::TestAnExplicitClearReachesTheRuleMatrixAsItsOwnAction；internal/parcelshipment/application/amend_customer_source_data_test.go::TestUnconfiguredAmendmentRulesStallRatherThanRefuseTheCustomer；internal/parcelshipment/domain/customer_source_data_test.go::TestAmendmentIntentMustCohereWithItsBasis |
| `AT-PS-023` | internal/parcelshipment/application/amend_customer_source_data_test.go::TestAnAmendmentNamingAParcelOutsideTheAcceptanceBaselineIsRejectedRatherThanErroring；internal/parcelshipment/application/amend_customer_source_data_test.go::TestAnAmendmentToARequestThatIsNotYetAcceptedIsNotReportedAsOutsideTheBaseline；internal/parcelshipment/application/amend_customer_source_data_test.go::TestAnExplicitClearReachesTheRuleMatrixAsItsOwnAction；internal/parcelshipment/domain/customer_source_data_test.go::TestACustomerSourceDataVersionCannotReachAParcelOutsideTheAcceptanceBaseline |
| `AT-PS-031` | internal/parcelshipment/application/amend_customer_source_data_test.go::TestAnAmendmentLostToAConcurrentWriterIsUndecidedNotRecorded；internal/parcelshipment/application/amend_customer_source_data_test.go::TestARetryAfterAFailedHandoffResendsTheSameIntentWithoutFormingASecondVersion；internal/parcelshipment/application/amend_customer_source_data_test.go::TestUnconfiguredAmendmentRulesStallRatherThanRefuseTheCustomer |
| `AT-PS-033` | internal/parcelshipment/application/form_acceptance_decision_test.go::TestAllAdoptedJudgmentsPassingFormsAnAcceptance |
| `AT-PS-034` | internal/parcelshipment/application/reject_shipment_request_test.go::TestAnAuthorizedOperatorFormsAnActiveRejection；internal/parcelshipment/domain/acceptance_task_test.go::TestAManualReviewCompletionWithoutAuthorityOrEvidenceCannotBeBuilt |
| `AT-PS-035` | internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control_test.go::TestARestrictedFreezeCarriesItsLimitBasisAndAnIdentity；internal/parcelshipment/application/form_acceptance_decision_test.go::TestAFailedReleaseKeepsTheRejectionAndLeavesCompensationPending；internal/parcelshipment/application/form_acceptance_decision_test.go::TestAnAcceptanceDoesNotReleaseTheFreeze；internal/parcelshipment/application/form_acceptance_decision_test.go::TestARejectionOnLostBasisStillReleasesAnExistingFreeze；internal/parcelshipment/application/form_acceptance_decision_test.go::TestARejectionReleasesTheFreezeByItsOriginalAssociation；internal/parcelshipment/application/reject_shipment_request_test.go::TestAFailedReleaseKeepsTheActiveRejectionAndLeavesCompensationPending；internal/parcelshipment/application/reject_shipment_request_test.go::TestAnActiveRejectionReleasesTheFreeze；internal/parcelshipment/application/reject_shipment_request_test.go::TestAnActiveRejectionWithUnreadableJudgmentsSendsNoRelease |
| `AT-PS-036` | internal/parcelshipment/application/form_new_submission_version_test.go::TestASupplementOnASubmittedRequestFormsANewVersion；internal/parcelshipment/application/submit_shipment_request_test.go::TestALinkedSubmissionIsBornCarryingItsProvenance；internal/parcelshipment/domain/rehydration_test.go::TestRehydrationCarriesThePriorRequestLink；internal/parcelshipment/domain/request_link_test.go::TestALinkedRequestIsBornPointingAtItsTerminalPrior；internal/parcelshipment/domain/submission_supersession_test.go::TestASupplementFormsANewSubmissionVersionKeepingHistory |
| `AT-PS-037` | internal/networkrouting/application/assess_parcel_reachability_test.go::TestAFormedJudgmentRetainsTheEvidenceViewRevision；internal/networkrouting/application/validate_reachability_judgment_test.go::TestAValidationConfirmsOrSupersedesByTheViewRevision；internal/parcelshipment/adapters/networkrouting/reachability_test.go::TestARevalidationTracksTheEvidenceViewRevision；internal/parcelshipment/application/form_acceptance_decision_test.go::TestADecisionStopsWhenAReachabilityJudgmentWasSuperseded；internal/parcelshipment/domain/submission_supersession_test.go::TestANewSubmissionVersionCannotFormAfterTheDecisionBoundary |
| `AT-PS-038` | internal/parcelshipment/adapters/nodeoperations/intake_source_test.go::TestANodeIntakeFlowsThroughToACommitment；internal/parcelshipment/application/adopt_network_intake_test.go::TestAQualifiedIntakeFormsTheCommitmentAtThePhysicalTime；internal/parcelshipment/domain/network_intake_test.go::TestBothIntakeSourcesFormTheSameCommitmentSemantics |
| `AT-PS-039` | internal/parcelshipment/adapters/transportfulfillment/pickup_source_test.go::TestAnOffsitePickupFlowsThroughToACommitment；internal/parcelshipment/domain/network_intake_test.go::TestBothIntakeSourcesFormTheSameCommitmentSemantics |
| `AT-PS-040` | internal/parcelshipment/application/adopt_network_intake_test.go::TestOneParcelsIntakeNeitherWaitsForNorCoversItsSiblings |
| `AT-PS-041` | internal/parcelshipment/application/adopt_network_intake_test.go::TestAReplayReturnsTheOriginalAndAConflictOverwritesNothing |
| `AT-PS-042` | internal/parcelshipment/application/adopt_network_intake_test.go::TestAReplayReturnsTheOriginalAndAConflictOverwritesNothing |
| `AT-PS-043` | internal/parcelshipment/domain/network_intake_test.go::TestAnIntakeSourceRefusesHintsAndHalfShapes |
| `AT-PS-044` | internal/parcelshipment/application/adopt_network_intake_test.go::TestTheCancellationBoundaryJudgesByBusinessTime |
| `AT-PS-047` | internal/parcelshipment/adapters/partycommercial/service_stage_rules_test.go::TestIntakeEligibilityTranslatesTheDeclaration；internal/parcelshipment/application/adopt_network_intake_test.go::TestEligibilityGapsStallWithoutDefaultingToACommitment |
| `AT-PS-048` | internal/parcelshipment/domain/network_intake_test.go::TestACommitmentAdjustsOnlyByAReasonedNewVersion |
| `AT-PS-049` | internal/parcelshipment/application/adopt_network_intake_test.go::TestASecondSourceKindDoesNotStartASecondResponsibility |
| `AT-PS-050` | internal/parcelshipment/domain/network_intake_test.go::TestACommitmentAdjustsOnlyByAReasonedNewVersion |
| `AT-PS-051` | internal/parcelshipment/application/adopt_network_intake_test.go::TestAFailedIntakeIntentIsRetriedWithoutASecondCommitment |
| `AT-PS-052` | internal/parcelshipment/application/adopt_network_intake_test.go::TestForeignOrUnacceptedTargetsRefuseWithoutLeaking |
| `AT-PS-053` | internal/parcelshipment/adapters/transportfulfillment/delivery_outcome_test.go::TestAnEffectiveDeliveryFlowsThroughToAFinalOutcome；internal/parcelshipment/application/form_parcel_final_test.go::TestAQualifiedDeliveryFormsTheFinalAndDerivesCompletion；internal/parcelshipment/domain/final_outcome_test.go::TestAFinalOutcomeDemandsItsRuleVersionAndKeepsItsSource |
| `AT-PS-054` | internal/parcelshipment/application/form_parcel_final_test.go::TestACancelledSiblingCountsTowardCompleteness；internal/parcelshipment/application/form_parcel_final_test.go::TestAQualifiedDeliveryFormsTheFinalAndDerivesCompletion；internal/parcelshipment/domain/final_outcome_test.go::TestCompletionIsDerivedPerParcelNotEdited |
| `AT-PS-055` | internal/parcelshipment/domain/final_outcome_test.go::TestAResponsibilityOutcomeDemandsBothDecisionAndExecution |
| `AT-PS-056` | internal/parcelshipment/application/form_parcel_final_test.go::TestUnconfiguredOrUnsatisfiedRulesStallWithoutDefaulting |
| `AT-PS-057` | internal/parcelshipment/domain/final_outcome_test.go::TestAResponsibilityOutcomeDemandsBothDecisionAndExecution |
| `AT-PS-061` | internal/parcelshipment/application/form_parcel_final_test.go::TestReplayConflictAndFailedIntentStayDisciplinedForFinals |
| `AT-PS-062` | internal/parcelshipment/application/form_parcel_final_test.go::TestReplayConflictAndFailedIntentStayDisciplinedForFinals |
| `AT-PS-063` | internal/parcelshipment/application/form_parcel_final_test.go::TestSourceRevisionRederivesWhileForeignSourcesAreRefused；internal/parcelshipment/domain/final_outcome_test.go::TestRederivationFormsANewVersionWithoutErasingHistory |
| `AT-PS-064` | internal/parcelshipment/application/form_parcel_final_test.go::TestSourceRevisionRederivesWhileForeignSourcesAreRefused |
| `AT-PS-065` | internal/parcelshipment/application/form_parcel_final_test.go::TestReplayConflictAndFailedIntentStayDisciplinedForFinals |
| `AT-PS-066` | internal/parcelshipment/application/form_parcel_final_test.go::TestForeignProbesAndHalfShapesAreRefusedUniformly |
| `AT-PS-067` | internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAnAuthorizedCustomerWithdrawsAPendingRequest；internal/parcelshipment/domain/withdrawal_test.go::TestACustomerWithdrawsAStillUndecidedRequest |
| `AT-PS-068` | internal/parcelshipment/application/withdraw_shipment_request_test.go::TestARepeatedWithdrawalReturnsTheOriginalWithdrawalAndItsCompensation；internal/parcelshipment/application/withdraw_shipment_request_test.go::TestARepeatedWithdrawalSourceReturnsTheOriginalHandling |
| `AT-PS-069` | internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAWithdrawalSourceConflictNeitherOverwritesNorWithdrawsAgain |
| `AT-PS-070` | internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAWithdrawalReleasesTheFreezeByItsOriginalAssociation；internal/parcelshipment/domain/withdrawal_test.go::TestADecisionCannotFormAfterAWithdrawalWon |
| `AT-PS-071` | internal/parcelshipment/application/reject_shipment_request_test.go::TestUnconfiguredRejectionRulesStallRatherThanRefuseTheOperator；internal/parcelshipment/application/withdraw_shipment_request_test.go::TestALateWithdrawalAfterAnAcceptanceKeepsItsLawfulFreeze；internal/parcelshipment/domain/withdrawal_test.go::TestAWithdrawalCannotOverwriteADecisionThatAlreadyCrossedTheBoundary |
| `AT-PS-072` | internal/parcelshipment/application/withdraw_shipment_request_test.go::TestALateWithdrawalReadsTheDecisionThatAlreadyWon；internal/parcelshipment/domain/withdrawal_test.go::TestAWithdrawalCannotOverwriteADecisionThatAlreadyCrossedTheBoundary |
| `AT-PS-073` | internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAFailedReleaseKeepsTheWithdrawalAndLeavesCompensationPending |
| `AT-PS-074` | internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control_test.go::TestAReleaseByTheRecordedResultIdentityFreesTheOriginalFreeze；internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAWithdrawalWithoutAnyFreezeSendsNoReleaseAndLeavesNoCompensation；internal/settlementaccounting/application/release_pre_acceptance_control_test.go::TestAReleaseForARequestThatNeverFrozeIsNothingToRelease |
| `AT-PS-075` | internal/parcelshipment/application/submit_shipment_request_test.go::TestAClaimOnAnInvisiblePriorIsUniformlyNotFound；internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAnUnauthorizedWithdrawalFormsNothing；internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAWithdrawalForAnUnknownRequestIsUniformlyInvisible |
| `AT-PS-076` | internal/parcelshipment/application/submit_shipment_request_test.go::TestALinkedSubmissionIsBornCarryingItsProvenance；internal/parcelshipment/application/withdraw_shipment_request_test.go::TestAWithdrawnRequestIsNeverRestoredInPlace；internal/parcelshipment/domain/rehydration_test.go::TestRehydrationCarriesThePriorRequestLink；internal/parcelshipment/domain/request_link_test.go::TestALinkedRequestIsBornPointingAtItsTerminalPrior |
| `AT-PS-077` | internal/parcelshipment/application/cancel_parcel_test.go::TestALawfulCancellationBeforeIntakeFormsTheFinalResult；internal/parcelshipment/domain/parcel_cancellation_test.go::TestACancellationFormsOnlyBeforeTheIntakeBoundary |
| `AT-PS-078` | internal/parcelshipment/application/cancel_parcel_test.go::TestAPartialBatchDoesNotRollBackItsSuccessfulMembers |
| `AT-PS-079` | internal/parcelshipment/application/adopt_network_intake_test.go::TestTheCancellationBoundaryJudgesByBusinessTime；internal/parcelshipment/application/cancel_parcel_test.go::TestACrossedBoundaryTurnsIntoDispositionPending；internal/parcelshipment/domain/parcel_cancellation_test.go::TestACrossedIntakeBoundaryRefusesCancellation |
| `AT-PS-080` | internal/parcelshipment/application/adopt_network_intake_test.go::TestTheCancellationBoundaryJudgesByBusinessTime |
| `AT-PS-081` | internal/parcelshipment/application/adopt_network_intake_test.go::TestTheCancellationBoundaryJudgesByBusinessTime |
| `AT-PS-082` | internal/parcelshipment/application/cancel_parcel_test.go::TestACrossedBoundaryTurnsIntoDispositionPending；internal/parcelshipment/domain/parcel_cancellation_test.go::TestACrossedIntakeBoundaryRefusesCancellation |
| `AT-PS-090` | internal/parcelshipment/application/cancel_parcel_test.go::TestForeignProbesAndFailedIntentsStayDisciplined |
| `AT-PS-091` | internal/parcelshipment/domain/final_outcome_test.go::TestAResponsibilityOutcomeDemandsBothDecisionAndExecution |
| `AT-PS-092` | internal/customscompliance/domain/disposition_verification_test.go::TestVerificationDemandsFactsAndComparesProvidedQuantities |
| `AT-SA-002` | internal/settlementaccounting/application/assess_advance_recovery_test.go::TestAnAssessmentRegistersOncePerIdentity；internal/settlementaccounting/domain/advance_recovery_test.go::TestAdvanceAssessmentJudgesWithoutRecomputingOrFabricating |
| `AT-SA-003` | internal/settlementaccounting/domain/advance_recovery_test.go::TestAdvanceAssessmentJudgesWithoutRecomputingOrFabricating |
| `AT-SA-004` | internal/settlementaccounting/domain/advance_recovery_test.go::TestAdvanceAssessmentJudgesWithoutRecomputingOrFabricating |
| `AT-SA-005` | internal/settlementaccounting/domain/advance_recovery_test.go::TestRecoveryNeedsAnEstablishedAdvanceAndContract |
| `AT-SA-006` | internal/settlementaccounting/domain/advance_recovery_test.go::TestRecoveryNeedsAnEstablishedAdvanceAndContract |
| `AT-SA-044` | internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control_test.go::TestAHeldFreezeBecomesAFormedHeldResult |
| `AT-SA-045` | internal/settlementaccounting/application/apply_pre_acceptance_control_test.go::TestTermsExposureReplayConflictAndUnavailableStanding；internal/settlementaccounting/application/release_pre_acceptance_control_test.go::TestARepeatedReleaseReturnsTheOriginalAnswerWithItsOriginalTime |
| `AT-SA-046` | internal/settlementaccounting/application/release_pre_acceptance_control_test.go::TestAHeldFreezeIsReleasedByItsOriginalAssociation |
| `AT-SA-049` | internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control_test.go::TestAnExplicitNoControlCarriesItsCommercialBasis；internal/settlementaccounting/application/apply_pre_acceptance_control_test.go::TestContractWithoutPreAcceptanceControlIsNotApplicable |
| `AT-SA-051` | internal/settlementaccounting/application/apply_pre_acceptance_control_test.go::TestSameRequestIdentityWithADifferentAmountIsAConflict |
| `AT-SA-054` | internal/settlementaccounting/domain/supplier_expected_cost_test.go::TestACorrectionAppendsWithoutRewritingTheOriginal |
| `AT-SA-056` | internal/settlementaccounting/domain/customer_charge_test.go::TestAdjustmentsDemandTheirSemanticKind |
| `AT-SA-059` | internal/settlementaccounting/domain/customer_statement_test.go::TestACutOffDraftAdmitsOnlyConfirmedCharges |
| `AT-SA-060` | internal/settlementaccounting/domain/customer_statement_test.go::TestACutOffDraftAdmitsOnlyConfirmedCharges |
| `AT-SA-063` | internal/settlementaccounting/domain/customer_statement_test.go::TestACutOffDraftAdmitsOnlyConfirmedCharges |
| `AT-SA-064` | internal/settlementaccounting/domain/customer_statement_test.go::TestLateChargesAndAdjustmentsGoToSubsequentPeriods |
| `AT-SA-065` | internal/settlementaccounting/domain/customer_statement_test.go::TestAPublishedStatementIsASealedSnapshot |
| `AT-SA-067` | internal/settlementaccounting/domain/customer_statement_test.go::TestAPublishedStatementIsASealedSnapshot |
| `AT-SA-069` | internal/settlementaccounting/domain/customer_statement_test.go::TestAPublishedStatementIsASealedSnapshot |
| `AT-SA-070` | internal/settlementaccounting/domain/customer_statement_test.go::TestADisputeIsIndependentAndScoped |
| `AT-SA-071` | internal/settlementaccounting/domain/customer_statement_test.go::TestADisputeIsIndependentAndScoped |
| `AT-SA-072` | internal/settlementaccounting/domain/customer_statement_test.go::TestADisputeIsIndependentAndScoped |
| `AT-SA-073` | internal/settlementaccounting/domain/customer_statement_test.go::TestADisputeIsIndependentAndScoped |
| `AT-SA-076` | internal/settlementaccounting/domain/customer_statement_test.go::TestLateChargesAndAdjustmentsGoToSubsequentPeriods |
| `AT-SA-077` | internal/settlementaccounting/domain/customer_statement_test.go::TestLateChargesAndAdjustmentsGoToSubsequentPeriods |
| `AT-SA-079` | internal/settlementaccounting/domain/supplier_bill_test.go::TestABillClaimArrivalIsNotAPayable |
| `AT-SA-082` | internal/settlementaccounting/application/receive_supplier_bill_test.go::TestABillIsReceivedWithPerLineMatches；internal/settlementaccounting/domain/supplier_bill_test.go::TestLineMatchingClassifiesDifferencesPerLine |
| `AT-SA-083` | internal/settlementaccounting/application/receive_supplier_bill_test.go::TestABillIsReceivedWithPerLineMatches；internal/settlementaccounting/domain/supplier_bill_test.go::TestLineMatchingClassifiesDifferencesPerLine |
| `AT-SA-086` | internal/settlementaccounting/domain/supplier_bill_test.go::TestLineMatchingClassifiesDifferencesPerLine；internal/settlementaccounting/domain/supplier_bill_test.go::TestOnlyAuditedMatchesBecomePayables |
| `AT-SA-087` | internal/settlementaccounting/application/receive_supplier_bill_test.go::TestABillIsReceivedWithPerLineMatches |
| `AT-SA-089` | internal/settlementaccounting/domain/supplier_bill_test.go::TestOnlyAuditedMatchesBecomePayables |
| `AT-SA-090` | internal/settlementaccounting/domain/supplier_bill_test.go::TestACreditNoteSupplementsWithoutRewriting |
| `AT-SA-092` | internal/settlementaccounting/application/receive_supplier_bill_test.go::TestReplayConflictAndIntentRecovery |
| `AT-SA-093` | internal/settlementaccounting/application/receive_supplier_bill_test.go::TestReplayConflictAndIntentRecovery |
| `AT-SA-096` | internal/settlementaccounting/application/receive_supplier_bill_test.go::TestUndecidedAndNotAcceptedSplitByRecovery；internal/settlementaccounting/domain/supplier_bill_test.go::TestLineMatchingClassifiesDifferencesPerLine |
| `AT-SA-098` | internal/settlementaccounting/application/receive_supplier_bill_test.go::TestReplayConflictAndIntentRecovery |
| `AT-SA-100` | internal/settlementaccounting/domain/supplier_bill_test.go::TestABillClaimArrivalIsNotAPayable |
| `AT-SA-101` | internal/settlementaccounting/domain/external_funds_test.go::TestAnAdoptedFundsFactIsAReferenceNotABalance |
| `AT-SA-102` | internal/settlementaccounting/domain/external_funds_test.go::TestAnAdoptedFundsFactIsAReferenceNotABalance |
| `AT-SA-104` | internal/settlementaccounting/domain/external_funds_test.go::TestMappingNeedsExplicitBasisBeyondCoincidence |
| `AT-SA-106` | internal/settlementaccounting/domain/external_funds_test.go::TestApplicationConservesAmountsPerAllocation |
| `AT-SA-107` | internal/settlementaccounting/domain/external_funds_test.go::TestApplicationConservesAmountsPerAllocation |
| `AT-SA-111` | internal/settlementaccounting/domain/external_funds_test.go::TestMappingNeedsExplicitBasisBeyondCoincidence |
| `AT-SA-112` | internal/settlementaccounting/domain/external_funds_test.go::TestReversalAppendsWithoutDeletingTheOriginal |
| `AT-SA-113` | internal/settlementaccounting/domain/external_funds_test.go::TestReversalAppendsWithoutDeletingTheOriginal |
| `AT-SA-114` | internal/settlementaccounting/domain/external_funds_test.go::TestAnAdoptedFundsFactIsAReferenceNotABalance |
| `AT-SA-122` | internal/settlementaccounting/domain/cost_allocation_test.go::TestAllocationNeedsAVersionedRule |
| `AT-SA-123` | internal/settlementaccounting/domain/cost_allocation_test.go::TestAllocationNeedsAVersionedRule |
| `AT-SA-124` | internal/settlementaccounting/domain/cost_allocation_test.go::TestAllocationNeedsAVersionedRule |
| `AT-SA-126` | internal/settlementaccounting/domain/cost_allocation_test.go::TestReallocationKeepsTheOriginalVersion |
| `AT-SA-137` | internal/settlementaccounting/domain/cost_allocation_test.go::TestOperatingResultIsDerivedNotEdited |
| `AT-SA-138` | internal/settlementaccounting/domain/cost_allocation_test.go::TestReallocationKeepsTheOriginalVersion |
| `AT-SA-144` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestClaimAmountsFormOnlyFromResponsibilityConclusions |
| `AT-SA-147` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestClaimAmountsFormOnlyFromResponsibilityConclusions |
| `AT-SA-148` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestClaimAmountsFormOnlyFromResponsibilityConclusions；internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestRecoveryReceivableIsIndependentOfAcknowledgement |
| `AT-SA-149` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestRecoveryReceivableIsIndependentOfAcknowledgement |
| `AT-SA-150` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestAcknowledgementCoversOnlyTheAcceptedRange |
| `AT-SA-152` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestAdjustmentsAppendWithoutRewritingAmounts |
| `AT-SA-153` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestClaimAmountsFormOnlyFromResponsibilityConclusions |
| `AT-SA-155` | internal/settlementaccounting/domain/claim_recovery_amount_test.go::TestAdjustmentsAppendWithoutRewritingAmounts |
| `AT-SA-167` | internal/settlementaccounting/domain/external_funds_test.go::TestApplicationConservesAmountsPerAllocation；internal/settlementaccounting/domain/external_funds_test.go::TestReversalAppendsWithoutDeletingTheOriginal |
| `AT-SA-168` | internal/settlementaccounting/domain/external_funds_test.go::TestApplicationConservesAmountsPerAllocation |
| `AT-SA-170` | internal/settlementaccounting/domain/external_funds_test.go::TestMappingNeedsExplicitBasisBeyondCoincidence |
| `AT-SA-176` | internal/settlementaccounting/domain/supplier_expected_cost_test.go::TestACrossCurrencyCostAdoptsTheEvaluationsConversion |
| `AT-SA-177` | internal/settlementaccounting/domain/supplier_expected_cost_test.go::TestAMissingConversionStepStopsTheCost |
| `AT-SA-178` | internal/settlementaccounting/domain/supplier_expected_cost_test.go::TestACorrectionAppendsWithoutRewritingTheOriginal |
| `AT-TF-002` | internal/transportfulfillment/application/accept_regulatory_disposition_test.go::TestANodeOnlyCollaborationBuildsNoTransportObject |
| `AT-TF-003` | internal/transportfulfillment/application/accept_regulatory_disposition_test.go::TestDetentionWithoutMovementAuthorityIsSupplementRequired |
| `AT-TF-004` | internal/transportfulfillment/application/accept_regulatory_disposition_test.go::TestDetentionWithoutMovementAuthorityIsSupplementRequired |
| `AT-TF-006` | internal/transportfulfillment/application/accept_regulatory_disposition_test.go::TestAPartialAcceptanceKeepsBothSides |
| `AT-TF-010` | internal/transportfulfillment/application/accept_regulatory_disposition_test.go::TestOneDecisionPerDispositionItem |
| `AT-TF-013` | internal/transportfulfillment/application/perform_offsite_pickup_test.go::TestAMixedAttemptRecordsPerObjectResults |
| `AT-TF-015` | internal/transportfulfillment/application/perform_offsite_pickup_test.go::TestAMixedAttemptRecordsPerObjectResults |
| `AT-TF-016` | internal/transportfulfillment/application/perform_offsite_pickup_test.go::TestAnAllFailedAttemptBuildsNoSegmentAndHandsOffNothing；internal/transportfulfillment/application/register_offsite_pickup_test.go::TestAFailedVisitHasNothingToRegister |
| `AT-TF-019` | internal/transportfulfillment/application/perform_offsite_pickup_test.go::TestAReplayReturnsTheOriginalAndResendsTheSameIntent；internal/transportfulfillment/application/register_offsite_pickup_test.go::TestAPickupRegistrationIsIdempotentPerObjectAttempt |
| `AT-TF-020` | internal/transportfulfillment/application/perform_offsite_pickup_test.go::TestAConflictingSourceKeepsTheOriginal；internal/transportfulfillment/application/register_offsite_pickup_test.go::TestAPickupRegistrationIsIdempotentPerObjectAttempt |
| `AT-TF-021` | internal/transportfulfillment/application/perform_offsite_pickup_test.go::TestARescheduledSecondAttemptSucceedsWithoutTouchingTheFirst |
| `AT-TF-024` | internal/transportfulfillment/application/perform_offsite_pickup_test.go::TestAReplayReturnsTheOriginalAndResendsTheSameIntent；internal/transportfulfillment/application/register_offsite_pickup_test.go::TestPickupRegistrationRecoveryDiscipline |
| `AT-TF-027` | internal/transportfulfillment/application/prepare_transport_opportunity_test.go::TestAScheduleEstablishesOnce；internal/transportfulfillment/domain/transport_schedule_capacity_test.go::TestAScheduleCarriesNoCapacityOrBooking |
| `AT-TF-031` | internal/transportfulfillment/application/prepare_transport_opportunity_test.go::TestOverReservationIsRefusedWithTheRemainder；internal/transportfulfillment/domain/transport_schedule_capacity_test.go::TestReservationsDrawDownFinitePoolCapacity |
| `AT-TF-051` | internal/transportfulfillment/domain/transport_handover_test.go::TestAHandedOverObjectCarriesBothEvidencesAndTransfersControl |
| `AT-TF-052` | internal/transportfulfillment/domain/transport_handover_test.go::TestBatchConclusionsDeriveOnlyFromObjectResults |
| `AT-TF-053` | internal/transportfulfillment/domain/transport_handover_test.go::TestRefusedAndUnconfirmedDoNotTransferControlOut |
| `AT-TF-061` | internal/transportfulfillment/application/register_transport_handover_test.go::TestAHandoverVersionRegistersOnce |
| `AT-TF-062` | internal/transportfulfillment/application/register_transport_handover_test.go::TestAHandoverCorrectionRegistersTheNewVersion；internal/transportfulfillment/domain/transport_handover_test.go::TestAHandoverCorrectionFormsANewVersionWithoutOverwriting |
| `AT-TF-064` | internal/transportfulfillment/domain/effective_delivery_test.go::TestARefusalEndsOnlyItsOwnObject |
| `AT-TF-065` | internal/transportfulfillment/domain/effective_delivery_test.go::TestARefusalEndsOnlyItsOwnObject；internal/transportfulfillment/domain/effective_delivery_test.go::TestFailedDeliveryOutcomesCannotBecomeAnEffectiveDelivery |
| `AT-TF-066` | internal/transportfulfillment/domain/effective_delivery_test.go::TestASecondAttemptDeliversAfterAFirstFailure |
| `AT-TF-067` | internal/transportfulfillment/domain/effective_delivery_test.go::TestADeliveredObjectFormsAnEffectiveDeliveryWithItsPOD |
| `AT-TF-069` | internal/transportfulfillment/application/register_effective_delivery_test.go::TestDeliveryRecoveryDiscipline |
| `AT-TF-072` | internal/transportfulfillment/application/register_effective_delivery_test.go::TestACorrectionSupersedesWithTheVersionChain；internal/transportfulfillment/domain/effective_delivery_test.go::TestAPODCorrectionFormsANewVersionWithoutOverwriting |
| `AT-TF-073` | internal/transportfulfillment/application/register_effective_delivery_test.go::TestDeliveryRecoveryDiscipline |
| `AT-TF-078` | internal/transportfulfillment/domain/alternate_journey_test.go::TestRegulatoryReturnIsMarkedByItsBasisKind |
| `AT-TF-079` | internal/transportfulfillment/domain/alternate_journey_test.go::TestAnAlternateJourneyIsLinkedButIndependent |
| `AT-TF-080` | internal/transportfulfillment/application/start_alternate_journey_test.go::TestARegulatoryJourneyFeedsBothChains；internal/transportfulfillment/domain/alternate_journey_test.go::TestRegulatoryReturnIsMarkedByItsBasisKind |
| `AT-TF-094` | internal/transportfulfillment/domain/transport_charge_occurrence_test.go::TestFailedAttemptOccurrenceTakesItsFactFromTheResult |
| `AT-VE-038` | internal/visibilityexception/application/derive_projection_test.go::TestFactsDeriveAndRederiveTheProjection |
| `AT-VE-039` | internal/visibilityexception/application/derive_projection_test.go::TestReplayConflictAndUnconfiguredMappingStayHonest |
| `AT-VE-040` | internal/visibilityexception/application/derive_projection_test.go::TestReplayConflictAndUnconfiguredMappingStayHonest |
| `AT-VE-042` | internal/visibilityexception/application/derive_projection_test.go::TestReplayConflictAndUnconfiguredMappingStayHonest |
| `AT-VE-044` | internal/visibilityexception/application/derive_projection_test.go::TestFactsDeriveAndRederiveTheProjection |
| `AT-VE-049` | internal/visibilityexception/application/form_eta_test.go::TestSufficientFactsFormAVersionedETA |
| `AT-VE-050` | internal/visibilityexception/application/form_eta_test.go::TestACommandMissingAnEssentialIsNotAcceptedAsAnETA |
| `AT-VE-053` | internal/visibilityexception/application/form_eta_test.go::TestAGapIsNotFormedBeforeTheWindowElapses |
| `AT-VE-054` | internal/visibilityexception/application/form_eta_test.go::TestAnElapsedWindowFormsTheGapWithoutConcludingLossOrStall |
| `AT-VE-057` | internal/visibilityexception/application/form_eta_test.go::TestAGapIsIdempotentPerWindowRuleVersion |
| `AT-VE-062` | internal/visibilityexception/application/raise_signal_test.go::TestAFirstHitOpensAnEpisodeAndConcludesTriage |
| `AT-VE-064` | internal/visibilityexception/application/raise_signal_test.go::TestARepeatHitOnAnActiveEpisodeUpdatesItWithoutASecondEpisodeOrTriage |
| `AT-VE-065` | internal/visibilityexception/application/raise_signal_test.go::TestAHitAfterRecoveryReopensALinkedEpisodeAndRetriages |
| `AT-VE-079` | internal/visibilityexception/application/send_disposition_request_test.go::TestAnActiveCaseSendsARequestWithItsEssentials |
| `AT-VE-092` | internal/visibilityexception/application/send_disposition_request_test.go::TestAnExpiredWindowRefusesALateAcceptanceButStillRecordsARefusal |
| `AT-VE-099` | internal/visibilityexception/application/notify_customer_test.go::TestANonDiscloseConclusionCannotGenerateANotification |
| `AT-VE-100` | internal/visibilityexception/application/notify_customer_test.go::TestANonDiscloseConclusionCannotGenerateANotification |
| `AT-VE-106` | internal/visibilityexception/application/notify_customer_test.go::TestAFailedChannelSubmissionIsRecordedAndKeptForRetry；internal/visibilityexception/application/notify_customer_test.go::TestARetryAfterFailureAppendsANewMilestoneKeepingTheFailedOne |
| `AT-VE-114` | internal/visibilityexception/application/handle_claim_test.go::TestClaimsInOneBatchAreReceivedItemByItem |
| `AT-VE-127` | internal/visibilityexception/application/handle_claim_test.go::TestAReviewWithinTheWindowFormsANewConclusionVersionKeepingThePrior |
| `AT-VE-129` | internal/visibilityexception/application/handle_claim_test.go::TestAReviewAfterTheWindowIsRefusedKeepingTheOriginalConclusion |
| `AT-VE-131` | internal/visibilityexception/application/handle_claim_test.go::TestARecoveryMatterOpensIndependentlyOfAnyClaim |
| `AT-VE-133` | internal/visibilityexception/application/handle_claim_test.go::TestRecoveryActionsKeepEveryAttemptPerKind |
| `AT-VE-136` | internal/visibilityexception/application/handle_claim_test.go::TestRecoveryActionsKeepEveryAttemptPerKind |
| `AT-VE-156` | internal/visibilityexception/application/derive_customer_view_test.go::TestFirstQualifyingProjectionPublishesAnAccountIsolatedView |
| `AT-VE-157` | internal/visibilityexception/application/derive_customer_view_test.go::TestANonDisclosableDimensionIsWithheldAloneWithoutHidingTheOthers |
| `AT-VE-161` | internal/visibilityexception/application/derive_customer_view_test.go::TestANewerProjectionSupersedesTheCurrentViewKeepingHistory |

## ② 机制候选（补 Covers 或小块测试）

| AT | 场景 | UC |
|---|---|---|
| `AT-NO-001` | 当前有效查验协作事项明确指向节点控制的 P1 及受控开封、呈验和重封动作 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-002` | 请求覆盖 P1、P2，但节点只控制并能合法处理 P1 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-003` | 请求缺少明确对象、动作、节点或必要授权依据 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-006` | 处置要求销毁 100 件，现场只确认并实际销毁 80 件 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-007` | 节点完成退运装载和节点侧交出扫描 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-010` | 协作事项在执行前取消或替代 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-011` | 协作事项在部分不可逆执行后取消或缩小范围 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-013` | 跨客户合报事项只允许节点处理 C2/P2 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-014` | 事实已保存，但任务派生或发布发生技术失败 | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-021` | 已终局包裹仍实际到站 | UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md |
| `AT-NO-026` | 身份后来被 `parcel-shipment` 确认 | UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md |
| `AT-NO-029` | 已控制包裹完成扫描、称重和量方 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-030` | 新测量使路由资格变化 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-032` | 新路由指令生效 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-033` | 三个包裹中两个兼容、一个不兼容 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-034` | 作业批次包含多个包裹 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-038` | 封签连续且相符的封闭单元被有效控制 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-039` | 集运单元嵌套形成循环 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-041` | 客户资料值与节点实测不同 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-042` | 已取消或终局包裹仍在节点 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-043` | 同一来源事实重复或内容冲突 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-044` | 路由变化与旧指令下分拣并发 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-045` | 节点完成备货、点验和物理装载 | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NR-007` | 产品明确允许待路由，委托已接受但当前仍无可行候选 | UC-NR-001-CREATE-INITIAL-ROUTE.md |
| `AT-NR-008` | 候选参考某班次可用性和容量风险后被选中 | UC-NR-001-CREATE-INITIAL-ROUTE.md |
| `AT-NR-013` | 客户声明偏好某线路，但合同和产品未承诺该偏好 | UC-NR-001-CREATE-INITIAL-ROUTE.md |
| `AT-NR-015` | 初始计划时间窗口与接受时预计承诺存在差异 | UC-NR-001-CREATE-INITIAL-ROUTE.md |
| `AT-NR-022` | 同一委托三个包裹分别具备合格候选、全部被排除和地址资料缺口 | UC-NR-002-ASSESS-PARCEL-REACHABILITY.md |
| `AT-NR-027` | 某次班次容量申请、订舱或装载失败 | UC-NR-002-ASSESS-PARCEL-REACHABILITY.md |
| `AT-NR-029` | 服务产品明确允许待路由，当前判断为不可达 | UC-NR-002-ASSESS-PARCEL-REACHABILITY.md |
| `AT-NR-032` | 标准网络服务得到不可达结果 | UC-NR-002-ASSESS-PARCEL-REACHABILITY.md |
| `AT-NR-033` | 客户送站形成有效网络收寄，实际节点与初始计划一致 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-034` | 场外揽收地点与计划接货范围一致，尚未到节点 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-035` | 有效网络收寄发生在非计划但已确认控制的节点 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-036` | 当前有效实测仍满足原计划硬限制 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-037` | 当前有效实测使原线路不再合格，存在替代候选且允许自动改路 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-038` | 原计划失效且没有替代候选 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-039` | 先前无路由，收寄后出现合格候选 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-040` | 只有普通扫描或卸载，没有控制依据 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-041` | 存在更优候选但已过冻结边界且原计划仍可执行 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-042` | 候选可行但不满足自动改路条件 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-043` | 改路会改变关务区域或报关服务方 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-044` | 单次班次或装载分配失败但线路仍有可行机会 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-045` | 同一触发重复处理 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-046` | 同一触发身份携带不同实测版本 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-047` | 路由复核与实际装载并发 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-048` | 结果提交成功但发布失败 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-050` | 硬限制已证明原计划失效，但替代候选来源暂时不可用 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-051` | 当前计划适用性证据本身冲突 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-052` | 原计划失效，候选评估确认没有合格候选 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-NR-053` | 原计划失效，候选评估未决后补齐并找到合格候选 | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-PC-016` | 发布提交成功但事件投递失败 | UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md |
| `AT-PS-009` | 接受后客户合同或产品发布新版本 | UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md |
| `AT-PS-011` | 接受成功后检查下游对象 | UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md |
| `AT-PS-018` | 无法识别目标委托、包裹或资料范围 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-019` | 请求方无客户、委托或字段范围授权 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-021` | 过期基础版本与当前版本在同一字段重叠 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-024` | 请求改变客户账户、责任法人、合同、服务产品或共享服务范围 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-025` | 客户重量/尺寸更正与节点实测冲突 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-026` | 已制签、收寄、装袋或运输后客户更正 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-027` | 关务尚未提交时形成客户资料更正 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-028` | 关务已提交后形成客户资料更正 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-029` | 关务案件已关闭或服务已完成后收到更正 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-030` | 业务有效时间早于系统接收时间的迟到更正 | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-045` | 收寄事实先发生，取消消息后到 | UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md |
| `AT-PS-046` | 包裹已经终局或被身份谱系替代后到站 | UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md |
| `AT-PS-058` | 普通退运旅程开始 | UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md |
| `AT-PS-059` | 退运完成满足合同终局规则 | UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md |
| `AT-PS-060` | 被拆分替代的来源包裹收到迟到交付结果 | UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md |
| `AT-PS-083` | 已收寄且普通退运决定、责任和路径齐备 | UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md |
| `AT-PS-084` | 监管退运要求到达 | UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md |
| `AT-PS-085` | 只有拒收或无路由信号 | UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md |
| `AT-PS-086` | 包裹已有终局后请求取消 | UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md |
| `AT-PS-087` | 取消包裹已有面单交易 | UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md |
| `AT-PS-088` | 处置请求已发出但运输方拒绝承接 | UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md |
| `AT-PS-089` | 收寄后服务终止执行结果满足合同终局规则 | UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md |
| `AT-PS-093` | 异常案件认定可能遗失，但权威服务责任结果尚未形成 | UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md |
| `AT-SA-001` | 合法关务交接到达 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-007` | 运营企业付款但合同明确由其最终承担 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-008` | 付款事实完整但客户责任版本不明确 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-009` | 税费 100、合格运营付款 80 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-010` | 税费 100、运营付款 120 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-011` | 税费、付款或结算账户币种不同且无有效换算依据 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-012` | 一笔付款有权威依据分配到多个税费义务 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-013` | 一笔付款被多个义务认领且无分配依据 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-015` | 合同存在已确认的比例、限额或免赔规则 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-016` | 实际代垫成立且产品另有代垫服务费 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-017` | 监管税费上调且尚无追加付款 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-019` | 收到监管税费退回决定但没有资金退回事实 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-020` | 合格资金退回事实关联原代垫 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-021` | 原付款被外部系统撤销或更正 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-022` | 相同交接和来源版本重复到达 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-023` | 相同请求身份携带不同客户、税费或金额 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-024` | 税费更正与资金更正并发到达 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-025` | 同一付款被两个客户并发认领 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-026` | 回收已形成但结果事件首次发布失败 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-027` | 对账单发布后收到迟到税费或资金更正 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-029` | 关务案件已关闭后收到合法结算输入 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-030` | 结算调整改变客户应收但未产生新监管义务 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-032` | 技术处理在结果提交前失败 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-034` | 检查本用例直接形成或修改的对象 | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-035` | 有有效测量、合同和客户价格规则 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-036` | 客户规则与供应商规则使用不同重量 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-037` | 只有客户声明重量，没有合格实测 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-038` | 实测迟到但业务有效时间早于截单 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-039` | 同一范围命中两个互斥基础价 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-040` | 附加费、折扣、最低收费同时适用 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-041` | 周期最低消费覆盖多个包裹 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-042` | 普通客户费用确认后出现有授权的计价更正 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-043` | 渠道交易作废但计价纠错或商业让利条件尚未成立 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-047` | 接受已成立但事件投递失败 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-048` | 最终费用高于冻结 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-050` | 规则或币种版本缺失 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-052` | 同一测量通过两个来源重复到达 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-053` | 客户计费规则更正但供应商规则未变 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-055` | 费用确认与截单并发 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-061` | 结算归属日与包裹签收日不同 | UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md |
| `AT-SA-068` | 发布请求重复 | UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md |
| `AT-SA-074` | 客户未回复对账单 | UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md |
| `AT-SA-075` | 客户后续付款已到账 | UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md |
| `AT-SA-078` | 检查本用例形成的对象 | UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md |
| `AT-SA-080` | 账单格式/账期无法识别 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-081` | 预期成本存在但供应商未提交主张 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-084` | 一条主张对应多个实际履约段 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-085` | 多条主张对应一个履约段 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-088` | 审核人员无金额授权 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-091` | 迟到账单属于已关闭账期 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-094` | 同一金额被两个客户并发认领 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-095` | 履约事实被更正 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-097` | 供应商已收到运营方付款通知 | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-103` | 付款指示明确指定一张对账单 | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-105` | 无付款指示存在两个同额候选 | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-108` | 付款金额不足 | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-109` | 付款金额超出明确费用范围 | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-110` | 付款币种与账户币种不同且无有效换算 | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-115` | 运营方收到付款通知但财务未确认 | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-118` | 核销结果发布失败 | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-125` | 分摊产生最小货币单位尾差 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-127` | 供应商审核应付已确认 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-128` | 内部法人提供节点服务，内部应收与内部应付方向账户均有效 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-129` | 内部协议或任一法人间方向结算账户缺失、冲突或不匹配 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-130` | 客户成本分析需要包裹归因 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-131` | 经营口径为预估，供应商账单尚未到达 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-132` | 经营口径分别为已确认和已结算 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-133` | 客户赔付金额已形成、应追偿金额尚未形成 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-135` | 跨币种来源无换算依据 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-136` | 汇兑差额产生 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-139` | 两个规则同时适用且互斥 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-142` | 指标发布失败 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-143` | 检查本用例形成的对象 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-145` | 一个索赔批次含多个索赔项 | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-146` | 客户责任只覆盖部分对象 | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-151` | VE 记录对方拒绝或追偿失权 | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-154` | 客户赔付金额已形成但运营企业尚未向客户支付 | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-156` | 同一责任范围被两个索赔项认领 | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-157` | 责任方、客户或币种不一致 | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-159` | 结果发布失败 | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-161` | 外部订舱发生项及采购价格规则有效 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-162` | 原订舱取消产生取消发生项，随后备用伙伴新旅程成功 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-163` | 两次外包派送尝试分别形成发生项 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-164` | 运输履约使原发生项失效，供应商账单尚未到达 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-166` | 上述应付和贷项已有 `UC-SA-005` 明确运营核销分配 | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-171` | 同一货主的业务 A 唯一命中预付政策和账户，业务 B 唯一命中账期政策和账户 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-172` | 同一金额或接受前控制范围同时命中预付和账期政策 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-173` | 相同输入快照、方向、目的和版本清单重复请求 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-174` | SELL 评价完成但价格费用代码没有唯一费用项目映射 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-175` | 节点实测重量超出价卡明确排除范围，`parcel-pricing` 返回不可计价 | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-TF-001` | 当前有效监管退运决定、明确对象及新路由均有效 | UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md |
| `AT-TF-005` | 运输协作已承接且完成订舱/承运接受，但尚未交接 | UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md |
| `AT-TF-007` | 节点已装载并扫描交出，但双方证据尚不足 | UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md |
| `AT-TF-008` | 退运旅程出发或到达 | UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md |
| `AT-TF-009` | 协作在出发前取消，或在部分对象出发后缩小范围 | UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md |
| `AT-TF-011` | 运输已到达监管指定地点 | UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md |
| `AT-TF-012` | 事实已保存但派生或发布技术失败 | UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md |
| `AT-TF-014` | 外包伙伴提供符合当前规则的对象级接收证据 | UC-TF-002-PERFORM-OFFSITE-PICKUP.md |
| `AT-TF-017` | 包装不合格且执行方明确拒收 | UC-TF-002-PERFORM-OFFSITE-PICKUP.md |
| `AT-TF-018` | 只有车辆到场和司机扫描 | UC-TF-002-PERFORM-OFFSITE-PICKUP.md |
| `AT-TF-022` | 一个对象已取消且取消先于实际接收 | UC-TF-002-PERFORM-OFFSITE-PICKUP.md |
| `AT-TF-023` | 揽收先实际发生，取消消息后到 | UC-TF-002-PERFORM-OFFSITE-PICKUP.md |
| `AT-TF-025` | 场外揽收成功但尚未到节点 | UC-TF-002-PERFORM-OFFSITE-PICKUP.md |
| `AT-TF-028` | 当前路由有效但没有合格班次 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-029` | 计划段只有部分对象准备好 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-030` | 重量和袋位容量同时适用 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-032` | 商业软容量允许受控超配 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-033` | 出发前资源替换 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-034` | 出发后发生资源故障 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-035` | 同一准备请求重复到达 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-036` | 计划段版本与节点准备范围冲突 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-037` | 运输准备保存成功但发布失败 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-039` | 运输委托提交但承运方尚未响应 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-040` | 承运方只接受部分数量 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-041` | 订舱确认但无容量预占依据 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-042` | 物理硬容量不足 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-043` | 软容量超配获授权 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-044` | 装载分配已经形成但节点尚未装载 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-045` | 舱单成员变化 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-046` | 签约服务商与实际承运商不同 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-047` | 外部响应重复且内容相同 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-048` | 外部响应冲突 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-049` | 委托取消与承运接受同时发生 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-050` | 结果已保存但发布失败 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-054` | 封签连续的集运单元整体交接 | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-055` | 封签破损但单元被接收 | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-056` | 渠道品牌已知但实际承运商未知 | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-058` | 班次出发后资源故障 | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-059` | 运输中断但没有控制终止证据 | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-060` | 目的节点接收证据充分 | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-068` | 执行方上传照片但方式未获允许 | UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md |
| `AT-TF-070` | 本人签收和地址来源冲突 | UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md |
| `AT-TF-071` | 多包裹部分交付后启动退运决定 | UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md |
| `AT-TF-075` | 主伙伴不可继续，系统有备用候选 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-076` | 授权角色确认备用伙伴 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-077` | 原实际段已经出发 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-082` | 新伙伴只接受部分对象 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-083` | 同一采用决定重复提交 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-084` | 决定更正与新旅程开始并发 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-086` | 同一成员的重量、体积和件数均参与容量 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-087` | 预占后节点新测量使体积增加 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-088` | 集运单元成员在舱单冻结前变化 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-089` | 出发门禁仍引用过期需求快照 | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-090` | 订舱已经确认且采用协议把该动作列为成本发生依据 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-091` | 订舱后取消被接受，但协议允许存在取消费用 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-092` | 取消到达时服务已经开始 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-093` | 伙伴更正原订舱结果并证明原发生范围无效 | UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md |
| `AT-TF-095` | 外包尾程第一次无人失败、第二次交付成功，协议允许失败尝试费 | UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md |
| `AT-TF-096` | 原旅程订舱后取消并切换备用伙伴，新旅程成功 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-097` | 原伙伴后来对原旅程给出费用贷记 | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |

## ③ 实例半边/闸门阻断

| AT | 场景 | 阻断原因 | UC |
|---|---|---|---|
| `AT-CC-005` | 同一固定范围内包含不同客户账户的包裹 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-011` | 两个处理并发尝试提交同一建案请求 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-025` | 不同客户包裹满足受控跨客户合报全部兼容条件 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-043` | 案件、客户账户或请求主体不具备资料准备范围 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-054` | 跨客户单元全部成员仍满足受控合报条件 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-057` | 只有凭证附件，或凭证身份不明、范围不符、到期、撤销或额度耗尽 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-064` | 新规则发布后，仅部分进行中范围依法适用 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-072` | 批量请求包含单元 U1、U2，U1 满足条件而 U2 存在阻断 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-073` | 真实程序的必要字段或口岸/渠道组合尚无权威规则，无法确定门禁 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-076` | 跨客户单元被无权主体请求评估或查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-080` | 具有授权请求权限但没有当前目标批准权限的操作员发起授权请求 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-082` | 委派已过期、范围不足，或只有批准权限却尝试拒绝/撤销 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-087` | 有当前拒绝权限的角色明确拒绝本次目标 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-093` | 决定形成后，决定方角色或授权政策发生变化 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-094` | 适用规则允许撤销，有撤销权限的角色在授权使用前明确撤销 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-100` | 人工或自动决定过程中目标版本、就绪或权限依据发生变化 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-101` | 批量请求包含 U1、U2，U1 获得授权而 U2 被拒绝或待决定 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-102` | 跨客户申报单元进入人工决定，或客户侧尝试查询授权 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-103` | 读取权限、委派、政策或就绪依据时超时，或计算、持久化失败 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-112` | 操作员有批准权限或可以登录渠道账号，但没有当前范围的实际提交权限 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-115` | 真实参数指定在冻结边界占用凭证，且当前版本所需次数或额度可用 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-119` | 跨客户合报单元形成提交版本 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-129` | 请求可能已经发送，但通信超时、连接中断或响应丢失 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-131` | 存在结果待确认尝试时，操作员再次点击提交或任务重跑 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-132` | 对结果待确认的原提交发起渠道查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-134` | 渠道返回重复、已存在或含义不能确定的响应 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-135` | 批量包含 U1、U2，U1 已发起而 U2 因凭证或授权阻断 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-141` | 申报渠道返回明确技术失败，但来源未证明监管侧没有收到业务申报 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-153` | 一个批量响应包含 U1 技术成功、U2 监管接收和无法关联的 U3 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-154` | 相同来源消息身份、版本和内容经同步、回调或查询重复到达 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-160` | 已关联响应包含真实程序尚未登记的结果代码 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-162` | 来源身份认证失败、租户不匹配或内容完整性校验失败 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-164` | 跨客户合报结果被客户 C1 或一般客服查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-165` | A1 通信超时后按原外部业务关联发起查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-166` | 查询返回“无记录”，但真实渠道未证明原尝试不存在可继续有效的外部申报身份或再次发送不会重复 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-167` | 查询调用失败、超时、渠道不可用或没有查询能力 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-168` | 查询响应明确返回原提交的监管接收或业务受理，或返回截至某时点的相同状态快照 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-169` | 合格来源和真实规则共同证明 A1 未形成可继续有效的外部申报身份，且再次发送同一版本不会造成重复申报 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-170` | 原提交仍待确认时，操作员要求换外部标识重新申报 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-171` | 分层结果已完整提交，但结果事件首次投递失败 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-187` | 真实程序明确补充保留原案件和原申报单元身份 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-188` | 真实程序明确更正保留原案件和原申报单元身份 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-190` | 来源称“改单”，但真实程序尚未确认是否保留原外部申报身份 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-194` | 真实程序明确必须取得某项撤销外部结果后才允许发送重报 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-200` | 替代目标和新单元已经建立，但尚未取得程序要求的外部结果 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-203` | 替代关系生效后查询原申报历史 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-204` | 监管更正只覆盖跨客户申报单元中客户 C2 的 P2 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-206` | 两项监管要求可由一个更正动作共同满足，且真实规则明确证明范围兼容 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-212` | 批量包含 U1 可更正、U2 动作未决、U3 请求越权 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-213` | 操作员有提交授权或实际提交权限，但没有后续动作决定权限 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-222` | `UC-CC-006` 已接受范围明确的监管扣留决定 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-223` | `UC-CC-006` 已接受销毁、移交、退运、没收或其他处置决定 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-229` | 扣留事项收到节点隔离和控制证据 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-230` | 操作员请求“已隔离所以解除扣留”但没有权威解除结果 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-236` | 本次物理执行范围已核对覆盖，但真实程序还要求监管最终确认 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-247` | 操作员有提交授权或节点登录权限，但没有执行协作/处置核对权限 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-248` | 节点在没有当前有效监管授权时开封或隔离 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-252` | 批量包含核对已覆盖、核对部分覆盖、差异、待交接和越权范围 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-253` | 监管决定要求扣留 P1 并将其转移到另一保管位置，但没有独立处置决定 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-254` | 同一来源消息同时包含对 P1 的扣留要求和独立销毁决定 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-257` | 本次执行范围已核对覆盖后，`UC-CC-006` 接受程序要求的监管最终确认 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-258` | `UC-CC-006` 已接受范围明确的监管核定税费 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-259` | 已接受监管税费结果或真实程序规则明确当前范围无需付款 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-260` | 已有税费核定，但真实程序的付款条件或放行顺序尚未登记 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-266` | 实际付款方与案件预期安排不同，真实程序是否允许尚未确定 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-267` | 同一付款事实通过回调、文件和查询重复到达 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-280` | 外部放行早于付款事实到达，且真实程序允许该顺序 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-294` | 当前税费仍需后续付款，但真实程序明确付款不阻断 P1 的当前出库动作 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-295` | 当前范围没有监管税费消息，但真实程序提供了可追溯的明确无需付款依据 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-298` | 一项申报结果仍待确认 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-304` | 真实程序允许一项剩余责任后续处理，来源方向明确接收方发起移交 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-305` | 接收方在有效权限和完整范围内接受责任并提供续办引用 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-318` | 操作员有案件编辑权限但没有相关重开授权 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-333` | 越权主体请求关闭、查看迟到事实、重开或建立后续案件 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-334` | 跨客户合报案件执行关闭或续办 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-339` | 已登记来源依据当前规则和合格事实，对明确 P1、动作 A1 和范围 R1 形成内部限制 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-344` | 限制覆盖 P1 的出库动作，但规则未覆盖接收、隔离、测量或已授权处置执行 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-352` | 非原限制责任来源请求解除有效限制 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-353` | 原限制责任来源、当前授权和解除条件满足证据均明确，拟解除范围为 R1/A1 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-369` | 批量请求含多个独立限制，其中部分成功、部分待确认或未受理 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-374` | 无权主体跨客户、法人或案件请求形成、解除、查询或导出限制 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-386` | 只有承运商页面文案或人工备注声称监管已受理 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-393` | 同一外部监管舱单覆盖 C1/P1 与 C2/P2 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-394` | 当前签约方、渠道品牌、代理商和实际承运商身份不同 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-398` | 人工发现原关联错误并具有修正权限和充分证据 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-399` | 无权用户尝试把引用关联到其他客户案件 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-400` | 一个批量来源中部分舱单合法、部分未决或冲突 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-404` | 承运商无法提供查询能力或当前状态 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-409` | 来源属于未授权法人、程序、方向或客户范围 | 查询/权限/审计面（接入与持久化闸门后） | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-NO-004` | 操作员有节点登录权限但没有当前有效开封、销毁或移交授权 | 查询/权限/审计面（接入与持久化闸门后） | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-005` | 节点依扣留协作将 P1 隔离并保持控制 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md |
| `AT-NO-022` | 包裹无当前有效路由 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md |
| `AT-NO-028` | 跨客户查询未知实物或收寄结果 | 查询/权限/审计面（接入与持久化闸门后） | UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md |
| `AT-NO-031` | 包裹没有当前有效路由 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-037` | 封签破损或与记录不符 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NO-046` | 跨客户集运隔离模拟 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md |
| `AT-NR-049` | 跨租户查询路由复核 | 查询/权限/审计面（接入与持久化闸门后） | UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md |
| `AT-PS-022` | 多包裹批量请求中 P1 合法、P2 待复核、P3 越权 | 查询/权限/审计面（接入与持久化闸门后） | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-PS-032` | 跨客户查询、导出和批量更正 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md |
| `AT-SA-014` | 跨客户合报税费已权威分配给 C1 和 C2 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-018` | 监管税费下调但尚无真实资金退回 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-028` | 后续收到客户真实付款并完成核销 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-031` | 操作员无目标客户或责任法人的结算权限 | 查询/权限/审计面（接入与持久化闸门后） | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-033` | 批量含成立、不成立、待判断、冲突和越权范围 | 查询/权限/审计面（接入与持久化闸门后） | UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md |
| `AT-SA-057` | 批量含确认、待判断和技术失败范围 | 查询/权限/审计面（接入与持久化闸门后） | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-058` | 检查本用例形成的对象 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md |
| `AT-SA-062` | 账户时区跨日 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md |
| `AT-SA-066` | 税务系统只返回未税金额 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md |
| `AT-SA-099` | 批量含通过、争议、拒绝和技术失败行 | 查询/权限/审计面（接入与持久化闸门后） | UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md |
| `AT-SA-116` | 供应商审核应付或客户赔付义务已形成，匹配的真实付款事实到达 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-117` | 代垫回收或供应商/保险追偿应收已形成但真实到账尚未发生，随后到账事实到达 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-119` | 同一外部事实重复通过文件和回调到达 | 事务发布/接入通道（ADR-0017 Bento/Outbox 闸门后） | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-120` | 批量含唯一、未分配、冲突和无权限范围 | 查询/权限/审计面（接入与持久化闸门后） | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-121` | 检查本用例形成的对象 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md |
| `AT-SA-134` | 追偿责任、对方认可、真实到账或运营核销后续变化 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-140` | 跨客户共享成本无授权范围 | 查询/权限/审计面（接入与持久化闸门后） | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-141` | `PAR-SET-06` 明确本期不适用 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-158` | 模拟赔付通过 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-160` | 检查本用例形成的对象 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md |
| `AT-SA-165` | 审核应付 100 后形成当前有效供应商费用贷项 20，尚无真实返款 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-SA-169` | 审核应付 100 已付款并核销后才形成当前有效贷项 20 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md |
| `AT-TF-026` | 跨客户批量查询或错误响应 | 查询/权限/审计面（接入与持久化闸门后） | UC-TF-002-PERFORM-OFFSITE-PICKUP.md |
| `AT-TF-038` | 跨客户共同班次查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md |
| `AT-TF-057` | 后续取得真实承运商收寄证据 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-063` | 跨客户共同运输查询 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md |
| `AT-TF-074` | 跨客户批量查询 POD | 查询/权限/审计面（接入与持久化闸门后） | UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md |
| `AT-TF-081` | 退运路径未纳入生产 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-TF-085` | 模拟退运通过 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md |
| `AT-VE-007` | 真实程序正常形成查验决定 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-019` | 案件需要关务补充查询或重新申报判断 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-024` | 客户影响明确且合同要求通知，但需要人工确认 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-025` | 内容包含受限监管原因、其他客户或未确认责任 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-045` | 包裹真实拆分 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-VE-002-BUILD-TRACKING-PROJECTION.md |
| `AT-VE-046` | 跨客户共同集运 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-VE-002-BUILD-TRACKING-PROJECTION.md |
| `AT-VE-067` | 查询只授权一个客户 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-089` | 一个共同案件只授权 C1 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-108` | C1/C2 同一共同案件 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-125` | 客户账户或申请人授权不匹配 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-148` | 模拟索赔/追偿通过 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-150` | C1 授权用户按本账户包裹查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-151` | C1 使用属于 C2 的有效外部标识查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-153` | 请求方只有外部运单号但没有客户账户授权 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-162` | 包裹发生真实拆分或合并 | 实例参数/证据层级（真实合同、税率、时区、账期、模拟隔离等待 PAR-* 提供） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-165` | 客户普通查询时不存在客户可见异常 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-166` | 合同要求异常主动送达，但客户已在门户查看 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-167` | 未认证收件人尝试公开查询 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-168` | 授权或视图结果保存失败 | 查询/权限/审计面（接入与持久化闸门后） | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |

## ④ 机制未落地（起步期上下文，等后续切片）

| AT | 场景 | UC |
|---|---|---|
| `AT-CC-001` | 已接受网络服务包裹具有完整出口监管辖区、方向、程序、法定义务和责任依据 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-002` | 同一包裹需要先后履行出口和进口监管程序 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-003` | 一次操作包含两个身份完整的出口包裹和一个进口程序尚无法确定的包裹 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-004` | 同一固定范围内关联多个同一客户包裹 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-006` | 多个包裹已同袋、同总单、同舱单或同班次 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-007` | 建案请求缺少监管辖区、方向、程序或法定义务范围之一 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-012` | 报关服务商存在，但申报人、法定义务人或账号使用授权无法解释 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-013` | 渠道服务方是底层监管渠道的代理商 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-014` | 固定监管范围完整，但路由尚未选定最终口岸或申报渠道 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-015` | 案件建立后参与方关系、资格或账号授权发生变化 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-016` | 读取商业资格或监管规则时依赖超时 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-017` | 案件最小结果提交成功，但结果事件首次投递失败 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-018` | 建案完成后检查下游对象 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-019` | 正式申报所需字段尚不完整，但案件四项固定身份、适用责任和初始角色均可确定 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-020` | 交接只有“已接受”状态字符串，没有可验证接受决定和接受基线 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-021` | 接受前可达性或初始路由只请求关务路径资格判断 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-022` | 建案后出现不同监管方向、程序或独立法定义务 | UC-CC-001-ESTABLISH-CUSTOMS-CASE.md |
| `AT-CC-023` | 已建立出口案件，单个包裹的单元规则和资料来源完整 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-024` | 同一案件中的多个同客户包裹在申报模式、责任和资料条件上兼容 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-026` | 跨客户候选中一个包裹的责任主体或资料条件不兼容 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-027` | 多个包裹同袋、同总单、同运输舱单或同班次 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-028` | 出口和进口案件同时请求准备 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-029` | 缺少判断合报资格所需的关键业务依据 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-030` | 客户原始值、节点实测和关务判断对同一资料存在差异 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-031` | 字段由客户值经过规则转换形成正式资料 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-032` | 输入完整、规则确定且风险允许的归类或监管条件判断 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-033` | 存在多个合理归类、受管制商品或高风险范围 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-034` | 正式资料缺少尚未确认的字段，但单元成员和其他来源已明确 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-035` | 当前口岸、申报渠道或凭证尚未确定，且当前准备规则不要求其作为字段前提 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-036` | 监管凭证存在但持有人、适用商品、期限或额度无法确认 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-037` | 同一资料请求和输入版本重复提交 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-038` | 同一请求携带不同案件、成员范围或资料来源 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-039` | 两个处理并发把同一包裹加入互斥单元 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-040` | 资料快照形成后，`UC-PS-002` 形成客户资料新版本，或实测、规则或角色资格发生变化 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-041` | 资料快照提交成功但事件首次投递失败 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-042` | 检查下游对象 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-044` | 候选范围为空，或所有成员均因不兼容而无法纳入单元 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-045` | 已有提交版本的单元收到资料修改请求 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-046` | 申报行由多个包裹聚合，或一个源字段拆成多个申报行 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-047` | 同一源值的业务发生时间早于接收时间，且迟到更正先后到达 | UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md |
| `AT-CC-048` | 当前组成版和资料版明确，必要资料、角色与账号、合报、凭证、适用口岸/渠道及限制门禁全部满足 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-049` | 正式资料缺少当前程序要求的必要字段，另一个缺失字段被当前规则明确标记为可选 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-050` | 必要字段存在未解决来源冲突、缺少字段溯源或人工复核尚未完成 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-051` | 同一参与方兼任多个关务角色，各角色资格和依据均分别有效 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-052` | 已有报关服务商，但申报人、责任主体或法定义务人无法明确 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-053` | 渠道账号持有人明确，但授权已过期、超出法人/程序/渠道范围或未授权当前使用 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-055` | 跨客户单元中一个成员的责任、资料或合报资格失效或无法确认 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-056` | 凭证身份、版本、签发方、持有人、辖区、商品、程序、线路、期限及当前可用额度均适用 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-058` | 当前程序要求的口岸、申报渠道、服务方和账号资格均明确且兼容 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-059` | 必要口岸或渠道未知、当前路由已改变口岸，或旧渠道资格不再适用 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-060` | 存在限制，但其对象、动作或时间不覆盖当前申报范围和拟提交动作 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-061` | 内部合规限制或外部监管限制仍有效并覆盖当前范围和拟提交动作 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-062` | 异常案件关闭、商业批准或其他非责任来源操作声称解除限制 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-063` | 规则法定适用时间早于消息接收时间，期间存在多个规则版本 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-065` | 已就绪后，资料、角色、合报、凭证、口岸、渠道或限制在提交前发生适用变化 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-066` | 原未就绪缺口被 `UC-PS-002` 新资料版本或有效解除补齐 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-067` | 首次申报、补充、更正或撤销重报分别请求就绪评估 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-068` | 相同请求身份和完全相同的输入、范围及版本再次到达 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-069` | 相同请求身份携带不同单元、资料、规则、拟提交动作或适用时间 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-070` | 两个处理并发评估相同范围和输入版本 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-071` | 评估期间任一权威输入版本发生变化 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-074` | 读取资格、规则、凭证或限制时超时，或计算、持久化失败 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-075` | 就绪判断已完整提交，但结果事件首次投递失败 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-077` | 检查已就绪判断产生的下游对象，或已有提交授权但当前判断未就绪 | UC-CC-003-ASSESS-DECLARATION-READINESS.md |
| `AT-CC-078` | 首发目标当前已就绪，有权业务角色逐次人工确认允许提交 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-079` | 请求方提交授权申请，但尚无实际决定方结论 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-081` | 决定方具有有效委派，但委派只覆盖当前法人、客户、程序、渠道和风险范围 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-083` | 权威治理明确长期目标不采用自动授权或要求人工决定 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-084` | 长期目标命中有效自动政策的全部条件并具有可验证授权依据 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-085` | 只有全局“自动申报”开关，没有适用政策版本和授权依据 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-086` | 自动政策明确不覆盖当前客户、程序、风险或动作 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-088` | 人工待决定超过预计处理时间或决定方暂未响应 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-089` | 授权目标明确绑定案件、单元、组成、资料、动作和指定渠道 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-090` | 授权后成员、资料、拟提交动作或指定渠道发生变化 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-091` | 授权形成后，申报就绪在提交前失效 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-092` | 授权自身明确有效截止时间到达 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-095` | 首次申报已授权，随后产生补充、更正或撤销重报动作 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-096` | 后续实际提交用例已返回某提交版本合法使用该授权的关系，随后请求另一个逻辑提交动作 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-097` | 相同请求身份和完全相同的目标、渠道及依据再次到达 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-098` | 相同请求身份携带不同资料、渠道、动作或请求方 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-099` | 两个有权角色并发对同一请求分别批准和拒绝 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-104` | 授权或拒绝决定已完整提交，但结果事件首次投递失败 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-105` | 授权请求进入时当前就绪不存在、已经失效，或与授权目标版本不一致 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-106` | 检查授权结果产生的下游对象 | UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md |
| `AT-CC-107` | 首发目标的当前就绪和人工提交授权完全一致，有权执行主体明确发起提交 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-108` | 一个工作界面连续完成授权确认和提交执行 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-109` | 执行请求到达时就绪不存在、已失效或与目标不一致 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-110` | 授权不存在、已撤销、已到期、目标不符或已被另一逻辑提交使用 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-111` | 就绪、授权和执行请求分别指向不同组成、资料、动作或渠道 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-113` | 实际渠道账号已撤销、到期、越出范围，或与就绪采用账号不相容 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-114` | 适用规则明确本次无需监管凭证，或凭证无需次数/额度占用 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-116` | 两个提交并发争用不足以同时覆盖两者的同一凭证额度 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-117` | 凭证要求存在，但占用时点、单位或释放规则尚无权威决定 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-118` | 检查一个已冻结提交版本 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-120` | 提交版本冻结后渠道模板或映射版本升级 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-121` | 提交版本合法形成后，资料、角色、规则、凭证或渠道资格发生变化 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-122` | 相同请求、目标、授权、账号和实际内容重复到达 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-123` | 相同请求身份携带不同资料、动作、渠道、账号、授权或内容 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-124` | 两个执行者并发使用同一具体授权提交同一目标 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-125` | 最终门禁、内容形成或本地持久化在完整业务边界前失败，且可证明未调用渠道 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-126` | 提交版本、授权使用和适用占用已形成，但确定外部发送尚未开始时进程中断 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-127` | 已登记发送意图首次投递失败或被重复投递 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-128` | 渠道调用已经开始，但尚未收到任何响应 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-130` | `UC-CC-006` 形成仍然有效且绑定 A1、原版本、渠道和范围的安全再次发送判断，并明确允许再次发送相同内容 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-133` | 渠道在发送调用中同步返回技术响应 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-136` | 跨客户申报单元中一个成员需要剔除后才能提交 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-137` | 首次申报后需要补充、更正或监管要求撤销重报 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-138` | 已发起或结果待确认后，有人要求释放凭证占用 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-139` | 检查本用例完成后产生的外部结果 | UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md |
| `AT-CC-140` | 申报渠道同步返回明确的报文技术接收成功 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-142` | 合格监管来源确认已经收到提交，但此前技术回执缺失 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-143` | 合格监管来源直接返回业务受理，但监管接收尚未到达 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-144` | 一个合格响应同时明确包含渠道技术结果和监管接收结果 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-145` | 报关服务方返回“已提交海关”，但来源合同未说明它代表渠道送达还是监管确认 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-146` | 监管机构只要求查验跨客户单元中的 P2 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-147` | 监管机构对 P1 放行、对 P2 未放行，并对 P1 附加明确条件 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-148` | P1 已取得监管放行，但仍存在覆盖 P1 出库动作的内部合规限制 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-149` | 同一监管响应包含税费核定和销毁决定 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-150` | 来源响应携带与原提交共同认可的唯一外部业务关联，渠道、账号和程序均一致 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-151` | 响应缺少稳定关联，或能够匹配两个候选提交 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-152` | 待关联响应后来取得唯一证据，或既有关联后来被合格证据证明错误，并由有权角色确认 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-155` | 相同来源消息身份和版本携带不同内容 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-156` | 来源明确发布对旧结果的更正或替代版本，且新旧结果范围存在交集和差集 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-157` | 业务发生更早的监管结果迟到，期间已有其他层结果 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-158` | 两个合格来源对同层同范围给出相反结果，且没有可用更正或优先规则 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-159` | 放行结果早于业务受理和监管接收到达，且放行来源及范围合格 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-161` | 代码映射发布新版本后重新解释一项历史未决响应 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-163` | 操作员上传截图或手工录入“海关已放行” | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-172` | 原始来源已保全，但关联或解释处理因技术故障中断 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-173` | 已接受权威结果和凭证规则共同证明监管凭证的明确额度已经使用成立 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-174` | 权威结果证明申报未使用凭证且规则明确允许释放 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-175` | 只有技术失败、待关联、解释未决、来源响应冲突、外部结果冲突或结果待确认 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-176` | 监管结果只确认部分申报范围使用凭证，或后续权威更正改变既有使用结论且已释放额度被其他提交占用 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-177` | 查验、扣留、补充资料或处置决定已经接受并发布下游 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-178` | 关务案件关闭后收到迟到更正、补税或稽核消息 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-179` | 检查本用例产生或修改的业务对象 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-180` | A1 被判断可安全再次发送后已经形成 A2，随后 A1 迟到取得监管接收或其他可继续有效的外部申报身份 | UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md |
| `AT-CC-181` | `UC-CC-006` 已接受并关联原提交的明确补充资料要求，范围和当前有效性完整 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-182` | `UC-PS-002` 新客户资料版本、节点事实、角色、渠道或账号变化经关务规则形成当前有效的后续申报要求 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-183` | 只有客户改值请求、操作员备注、截图或未接受外部消息 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-184` | 外部结果待关联、解释未决、存在未裁决冲突或已经被更正为不再有效 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-185` | 原处理要求被合格来源更正，范围由 P1+P2 缩小为 P2 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-186` | A1 已满足安全再次发送条件，提交版本、内容、动作和业务身份均不变 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-189` | 已提交资料收到直接覆盖修改请求 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-191` | 监管要求针对原外部申报身份执行撤销 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-192` | 撤销目标已经授权并发送，但只有技术成功 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-193` | 规则要求撤销原申报并重新申报 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-195` | 撤销与重报的顺序、生效结果或并行条件尚无权威规则 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-196` | 规则只允许撤销结果确认前准备替代资料，不允许发送重报 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-197` | 重报目标内容与原提交完全相同 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-198` | 原申报单元不能继续使用，但案件辖区、方向、程序和法定义务范围保持不变 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-199` | 重报需要改变监管辖区、方向、程序或形成独立法定义务 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-205` | 一项监管要求必须拆成一个撤销目标和两个替代范围，且规则明确允许 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-207` | A：多个已接受且各自当前有效的要求对同一原申报交集范围明确要求互斥动作；B：多个要求或范围是否可以合并、拆分尚无权威规则 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-209` | 相同请求身份携带不同触发版本、范围、动作意图或原提交 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-210` | 两个执行者并发处理同一触发和范围 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-211` | 决定提交前触发依据、规则、原结果或范围发生变化 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-214` | 已关闭案件收到后续要求，但尚无合法重开或后续案件决定 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-215` | A2 发起后 A1 迟到形成有效外部申报身份，当前规则要求受控消除重复申报 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-216` | 形成补充、更正、撤销或重报目标时检查监管凭证 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-217` | 补充、更正、撤销和重报目标进入后续流程 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-218` | 处理决定和目标已提交，但事件首次投递失败 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-219` | 触发要求在目标已提交后被更正或撤销 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-220` | 检查本用例直接产生或修改的业务对象 | UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md |
| `AT-CC-221` | `UC-CC-006` 已接受范围明确的监管查验决定 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-224` | 只有截图、客户要求、节点备注或未接受外部消息 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-225` | 决定待关联、解释未决、形成`来源响应冲突`或`外部结果冲突`，或者已被更正为不再有效 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-226` | 执行方、能力、位置或授权尚未确定 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-227` | 查验事项交给节点执行受控开封和重封 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-228` | 查验只覆盖合报范围中的 P2，P1 无查验决定 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-231` | 监管决定销毁 100 件，执行方只提供 80 件合格执行事实 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-232` | 执行事实对象、数量、条件或证据与处置决定不一致 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-234` | 监管退运决定需要新的路由和运输履约 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-235` | 执行方返回“已完成”但只提供批次级页面状态，缺少适用证据规则要求的范围依据 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-238` | 当前监管决定将未执行范围由 P1+P2 更正为 P2 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-239` | 当前监管决定取消或替代尚未执行的处置事项 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-240` | 监管期限届满，但没有明确届满后果规则 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-241` | 包裹已放行、移动或交付后收到新的监管执行要求 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-242` | 两个已分别接受且各自当前有效的决定不构成`外部结果冲突`，但在同一对象的交集范围和适用期间要求互斥动作，且没有权威更正、取消、替代、优先、范围裁决或合法执行顺序 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-243` | 跨客户合报中只有 C2/P2 需要处置执行 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-244` | 同一决定、范围和请求重复到达 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-245` | 同一执行事实通过回调、文件和人工录入重复到达 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-246` | 两个合格执行方分别对同一协作事项中不可并行的对象、动作和交集范围形成接受，且没有已确认的责任分配、合法顺序或共同执行规则 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-249` | 内部合规限制被解除，但外部扣留仍有效，或反之 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-251` | 检查本用例直接形成或修改的业务对象 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-255` | 节点只接受事项中的 P1、拒绝 P2 并要求补充 P3 的执行依据 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-256` | 执行上下文更正原接受结果或实际执行主体/资源安排 | UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md |
| `AT-CC-261` | 合格资金来源提供可关联税费的实际付款事实 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-262` | 付款金额与税费相同，但没有税费身份、申报范围或权威分配关系 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-263` | 当前税费要求 100，合格付款事实只覆盖 80 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-264` | 可关联付款事实为 120，当前税费要求为 100 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-265` | 付款币种与当前税费要求不一致，且没有适用换算或接受规则 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-268` | 只有付款申请、技术受理、银行待处理，或本次外部处理技术失败且没有权威付款结果 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-269` | 已参与覆盖的实际付款在 P1 范围后来被资金退回、付款撤销或更正为不再有效 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-270` | 监管税费更正后金额或范围增加 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-271` | 监管税费更正后金额或范围减少 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-272` | 关务案件关闭后收到仍属于原程序的付款或税费更正 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-273` | 客户或收件人直接承担并付款，运营企业无合同付款责任 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-274` | 外部资金事实显示运营企业付款，且合同明确客户承担 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-275` | 客户预收了预计税费，但尚无实际监管付款事实 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-276` | 税费付款核对已覆盖，但尚未收到监管放行 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-277` | 税费付款已覆盖，但内部合规限制或监管处置仍有效 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-278` | 付款已覆盖，但外部监管结果要求继续查验、扣留或补充资料 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-279` | 外部监管只放行申报范围中的 P1，P2 仍待税费或其他条件 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-281` | 两个合格来源对同一付款的金额、币种、状态或有效性冲突 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-282` | 付款事实已被外部系统确认，但本次关务核对技术处理失败 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-283` | `settlement-accounting` 回接客户代垫回收结果 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-284` | 客户代垫回收只成立部分金额、被拒绝或长期未收 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-285` | 客户代垫回收后来发生客户代垫回收调整、核销撤销或坏账处理 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-286` | 一笔实际付款覆盖两个税费版本或范围，来源提供明确分配 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-287` | 合报付款涉及两个客户，但只明确 C2/P2 的付款责任和分配 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-288` | 付款事实迟到，此前门禁判断为待满足 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-289` | 税费更正和付款事实并发到达 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-290` | 操作员手工点击“已缴税”但没有合格实际付款事实 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-291` | 节点或运输方试图凭付款页面、结算回收或异常关闭绕过当前门禁 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-292` | `UC-CC-006` 接受明确范围的外部监管放行或放行更正 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-293` | 检查本用例直接形成或修改的业务对象 | UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md |
| `AT-CC-296` | 进行中案件的当前申报、限制、处置、税费及其他义务可完整盘点 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-297` | 全部适用义务均有权威终结依据 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-299` | 存在仍有效且覆盖当前范围的内部合规限制 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-300` | 限制来源形成只覆盖部分范围的有效解除 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-301` | 查验执行方标记完成，但处置核对仍为部分覆盖或证据不足 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-302` | 税费已核定但付款义务或放行前置关系仍未决 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-303` | 监管已经放行，但仍有独立内部限制或后续资料义务 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-306` | 接收方拒绝、未响应、只接受部分范围或缺少续办引用 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-309` | 同一关闭请求身份携带不同义务范围或决定内容 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-310` | 关闭核对完成后、关闭决定提交前到达新的有效限制或监管事实 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-311` | 案件已关闭后检查原申报、限制、处置、税费、付款、结算和物流事实 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-312` | 用户仅凭监管放行、付款完成、运输完成、交付完成、SLA 到期或长期无消息请求关闭 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-313` | 关闭请求包含多个独立案件，其中部分满足、部分未决 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-314` | 已关闭案件收到合格来源的迟到更正事实 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-315` | 迟到事实的来源、范围或与原案关系尚不能确定 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-316` | 迟到事实仍属于原监管辖区、方向、程序和法定义务身份内的同一程序续办 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-317` | 同一程序迟到事实关联到原关闭决定和受影响依据项，相关原关闭责任来源及当前授权均允许 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-319` | 多个受影响关闭依据项的原责任来源要求冲突，或授权未完整覆盖范围 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-320` | 迟到事实改变监管辖区、进出口方向、监管程序或法定义务身份 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-321` | 迟到事实形成独立补税义务 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-322` | 迟到事实形成独立稽核、申诉或其他后续监管义务 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-323` | 建立后续案件时固定身份或必要责任角色证据不足 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-324` | 同一迟到事实重复投递或经不同传输方式再次到达 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-325` | 同一迟到事实被并发请求分别主张为重开和后续案件 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-326` | 原案已因事实 F1 重开，随后事实 F2 形成独立义务 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-327` | 重开后再次完成义务并申请关闭 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-328` | 受控重开后存在已完成节点作业、运输移动、交接、交付或付款 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-329` | 后续案件建立后需要新申报、处置或税费处理 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-330` | 迟到事实经权威判断只是已记录事实的合格重复且不产生新义务 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-331` | 关闭决定提交成功但发布失败，随后恢复 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-332` | 后续案件建立过程中技术失败 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-335` | 检查本用例直接形成或修改的业务对象 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-336` | 同一迟到来源事实包含仍属原程序的范围 R1 和形成独立义务的范围 R2 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-337` | 原案已经历关闭 C1、重开、再次关闭 C2，随后又收到同程序迟到事实 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-338` | 原案重开或后续案件建立后检查旧就绪、提交授权、凭证适用性、门禁及父子案件生命周期 | UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md |
| `AT-CC-340` | 请求缺少责任来源、依据、对象、范围或受限动作 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-341` | 必须参与判断的来源均有当前、无冲突且可追溯的无适用限制结论 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-342` | 必须参与判断的来源、规则、动作、范围或依据尚不确定 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-343` | 限制仅覆盖 P1/R1/A1，P2、R2 或 A2 不在适用规则明确范围内 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-349` | `UC-CC-006` 接收监管扣留或放行，但没有已登记规则要求形成内部限制 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-350` | 已登记规则允许将明确外部监管结果作为解除条件证据 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-351` | 商业批准、异常案件关闭、节点任务完成、运输交接、付款完成或管理员备注声称解除限制 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-354` | 解除请求未证明条件满足、范围不足、动作不明或来源授权失效 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-355` | 限制覆盖 R1 与 R2，来源只解除 R1 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-356` | 来源对该限制全部当前覆盖范围形成有效解除 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-357` | 来源只解除 A1，但同一对象 A2 仍被该限制覆盖 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-358` | 来源事实、规则或解除条件后来被更正、撤销、失效或迟到 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-359` | 预计复核时间、SLA、长期无新消息或人工勾选到达 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-360` | 内部限制解除后检查 `UC-CC-003` | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-361` | 内部限制解除后检查 `UC-CC-009` | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-362` | 内部限制解除后检查 `UC-CC-010` | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-363` | 有效限制覆盖当前申报范围和拟提交动作 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-364` | 有效限制覆盖当前拟执行动作和监管边界 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-365` | 有效限制仍在案件当前义务范围内 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-366` | 解除核对完成后、解除决定提交前出现新限制覆盖、来源更正、解除条件失效或授权变化 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-367` | 同一来源的限制形成、解除和更正并发到达 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-368` | 同一来源消息含 R1 与 R2，R1 可确定受限而 R2 依据待确认 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-371` | 相同解除请求身份携带不同限制、范围、动作、条件或证据 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-372` | 限制或解除的读取、判断、持久化或并发控制技术失败 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-373` | 限制或解除已提交但首次发布失败 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-375` | 检查本用例直接形成或修改的业务对象 | UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md |
| `AT-CC-376` | 首发出口程序收到责任承运商提供的外部监管舱单身份、版本、程序、方向和明确范围 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-377` | 首发进口程序收到同类合格来源 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-378` | 运输舱单和外部监管舱单使用相同外部文本标识 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-379` | 来源只提供一个外部编号，但程序、方向或范围不足 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-382` | 外部监管舱单列入 P1，但尚无实际装载或交接事实 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-383` | 承运商报告外部监管舱单已形成 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-384` | 承运商报告已向监管渠道提交 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-385` | 承运商同时传递来源可验证的监管接收结果 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-388` | 同一来源身份和版本携带不同内容 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-390` | V2 只更正 P2 范围 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-391` | 承运商明确撤销 V1 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-392` | V2 明确替代 V1，但 V1 的监管结果迟到 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-395` | 技术中介代表责任承运商传输来源 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-396` | 承运商关系在舱单形成后发生变化 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-397` | 来源先到，关务案件或运输舱单尚未可见 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-401` | 外部舱单范围与运输舱单范围不同 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-402` | 实际装载与两个舱单范围均不同 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-403` | 关务案件已关闭后收到外部舱单更正 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-405` | 操作员尝试编辑外部舱单内容或点击本地提交 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-406` | 为关联外部结果而尝试创建占位提交版本或提交尝试 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-407` | 引用和关系提交成功，但发布首次失败 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-408` | 来源已保全，但关联结果技术提交失败 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-CC-410` | 检查本用例直接形成或修改的对象 | UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md |
| `AT-VE-001` | `UC-CC-006` 提供已接受、范围明确的关务结果 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-002` | 只有原始监管消息、截图或操作员备注，尚未被关务接受 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-003` | 只有技术送达/监管接收且不存在适用异常条件 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-004` | 结果待确认但仍在已配置观察窗口内 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-005` | 结果待确认超过有效观察窗口 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-006` | 两项已接受关务结果冲突且关务尚不能裁决 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-008` | 扣留或限制只覆盖 P2 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-009` | `UC-CC-008` 形成执行部分覆盖、差异或证据不足 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-010` | `UC-CC-009` 形成付款关联冲突或门禁长期未决 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-011` | `UC-CC-011` 存在当前有效内部合规限制 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-012` | 信号高可信、高影响且命中已生效自动建案规则 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-013` | 信号资料不足、可能重复或影响不明 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-014` | 分诊明确无需持续处置 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-015` | 同一条件在连续影响期重复命中 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-016` | 信号条件恢复 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-017` | 同一监管或渠道故障影响多个客户 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-018` | 跨客户内部案件覆盖 C1/P1 与 C2/P2 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-020` | 关务只接受处置请求中的 P1，拒绝 P2 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-021` | 处置请求已接受但尚无实际业务结果 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-022` | 异常案件满足关闭条件，但关务限制、扣留或义务仍有效 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-023` | 内部案件已建立但当前披露规则不允许通知 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-026` | 明确批准范围允许自动发布 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-027` | 消息渠道接受通知 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-028` | 合同要求送达且消息能力返回已送达 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-029` | 合同要求客户确认但只有送达结果 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-030` | 投递失败后按规则重试或切换渠道 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-031` | 已发布内容因关务更正需要修改 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-032` | 案件关闭后同一因果链、责任范围和处置范围出现足以影响结论的新关务事实 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-033` | 新事实属于独立原因、责任或处置范围 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-034` | 相同关务事实版本、请求和业务内容重复到达，或同一请求身份携带不同客户、范围、事实版本或披露内容 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-035` | 关务案件关闭/重开与异常案件关闭/重开交错发生 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-036` | 分诊、建案、披露或发布在结果提交前技术失败 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-037` | 分诊发现两个开放异常案件实际属于同一因果链和共同处置范围，且确认其中一个为重复案件 | UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md |
| `AT-VE-041` | 外部状态码尚未被业务接受 | UC-VE-002-BUILD-TRACKING-PROJECTION.md |
| `AT-VE-043` | 两个有效事实无法裁决 | UC-VE-002-BUILD-TRACKING-PROJECTION.md |
| `AT-VE-047` | 外部段无内部节点明细 | UC-VE-002-BUILD-TRACKING-PROJECTION.md |
| `AT-VE-048` | 保存结果不确定 | UC-VE-002-BUILD-TRACKING-PROJECTION.md |
| `AT-VE-051` | 两个来源都提供 ETA | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |
| `AT-VE-052` | 内部 ETA 已形成，但质量明确未达适用客户门槛 | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |
| `AT-VE-055` | 外部段未承诺内部扫描 | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |
| `AT-VE-056` | 来源整体中断 | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |
| `AT-VE-058` | 缺口恢复后再次发生 | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |
| `AT-VE-059` | 保存结果不确定 | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |
| `AT-VE-060` | 接受事实形成计划偏离信号 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-061` | 缺口未命中异常规则 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-063` | 资料不足或可能重复 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-066` | 一个来源中断影响多个客户 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-068` | 同因果链已有案件 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-069` | 独立货损与运输中断并存 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-070` | 案件责任团队转交 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-071` | 结果保存失败 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-072` | 规则版本停用 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-073` | 关闭后出现同因果新事实 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-074` | 两案确认重复 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-075` | 无执行授权 | UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md |
| `AT-VE-076` | 新案件分派给责任团队 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-077` | 团队转派但接收方未接受 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-078` | 首次响应和下一行动完成 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-080` | 目标上下文拒绝请求 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-081` | 目标上下文部分接受 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-082` | 目标动作实际完成 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-083` | 案件需要等待伙伴 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-084` | 关闭条件满足 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-085` | 关闭后同因果新事实到达 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-086` | 新事实属于独立责任范围 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-087` | 两案确认是重复 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-088` | 响应目标调整或延期获批 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-090` | 请求或关闭提交技术失败 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-091` | 处置请求被接受但无执行结果 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-093` | 已接受请求超过期望完成时限 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-094` | 案件范围缩小且旧请求尚未接受 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-095` | 旧请求已部分执行后被替代 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-096` | 目标方表示动作已经开始、无法取消 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-097` | 案件归并但存在未完成请求 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-098` | 案件满足其他关闭条件但请求取消仍待目标确认 | UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md |
| `AT-VE-101` | 内容包含其他客户或未确认责任 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-102` | 批准范围允许自动发布 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-103` | 消息渠道接受 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-104` | 渠道返回送达 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-105` | 合同要求客户确认但只有送达 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-107` | 内容因新事实需要更正 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-109` | 客户确认或提出异议 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-110` | 通知提交结果保存失败 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-111` | 关务专项披露 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-112` | 自动发布规则停用 | UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md |
| `AT-VE-113` | 证据材料收到但尚未核实 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-115` | 索赔资料不足 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-116` | 客户在期限内提交材料但仍不完整 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-117` | 有权角色按允许规则批准补充延期 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-118` | 只有延期申请或内部无权批准 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-119` | 补充期限届满且规则明确逾期未补不予受理 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-120` | 补充期限届满但规则未规定后果或延期效力待确认 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-121` | 最终责任结论前收到授权客户逐项撤回 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-122` | 责任结论或赔付金额形成后客户要求撤回 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-123` | 客户撤回后重新提交同一范围 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-124` | 首次索赔超过合同期限 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-126` | 责任只覆盖部分对象 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-128` | 合同以结论送达起算复核期，但送达事实待确认 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-130` | 复核请求新增包裹、索赔类型或责任范围 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-132` | 通知内容和证据已准备但尚未对外提交 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-134` | 主张已提交且渠道接受，但协议要求实际送达 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-135` | 主张已送达，但条款要求对方确认 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-137` | 对方明确拒绝责任 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-138` | 合同允许且有权相对方确认延期 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-139` | 只有内部延期批准或已发送延期请求 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-140` | 期限届满且所需送达/确认未成立 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-141` | 对方在响应期限内没有回复 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-142` | 对方要求补充或表示审核中 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-143` | 对方仅接受部分责任范围 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-144` | `UC-SA-007` 已形成追偿金额 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-145` | 财务来源确认实际回款 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-146` | 客户赔付已形成但外部追偿仍在处理 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-147` | 同一证据被多个案件和追偿事项引用 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-149` | 索赔或追偿结果保存失败 | UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md |
| `AT-VE-152` | 同一外部标识存在多个有效候选 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-154` | 集团用户只获委派 C1、C3 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-155` | C1/C2 包裹处于同一集运单元、班次或共同案件 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-158` | 只有内部 ETA 或质量未达对客门槛 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-159` | `parcel-shipment` 已形成当前有效客户服务终局 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-160` | 只有有效交付扫描但终局尚未形成 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-163` | 一个维度存在无法裁决的事实冲突 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-164` | `UC-VE-006` 已批准当前客户可见异常 | UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md |
| `AT-VE-169` | 内部 ETA 已形成，且适用质量、合同和客户服务规则条件明确满足 | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |
| `AT-VE-170` | 内部 ETA 已形成，但适用合同、门槛或客户服务规则缺失或冲突 | UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md |

## 重盘变化摘要（0cddbba）

重盘方法与首盘逐字相同（全仓 `*_test.go` 扫 `AT-[A-Z]{2}-\d{3}` 引用、归属到所在测试函数，对 `docs/application` 的 AT 全集）；诚实约束照旧——机制在但无测试钉不计①，硬句测试不点 AT 号也不计①。

### 四桶数字：零变动

| 桶 | 首盘 `136f79b` | 重盘 `0cddbba` | 变化 |
|---|---|---|---|
| ① 已钉住 | 223 | 223 | ±0（集合逐条相等，无增无减无迁移） |
| ② 机制候选 | 241 | 241 | ±0 |
| ③ 实例/闸门 | 148 | 148 | ±0 |
| ④ 机制未落地 | 471 | 471 | ±0 |
| 合计 | 1083 | 1083 | ±0（`docs/application` 自首盘未改，AT 全集不变） |

四桶不动的原因不是没进展，而是引用口径：两盘之间落了 48 个测试文件、约 1.16 万行（十余个编排 handler 横跨 CC/VE/PP/PG/SA/TF/NO/PC/PS 适配器），其 Covers 注释全部引 CONTEXT/UC 硬句原文；仅 2 处新增 `AT-*` 点名（`AT-PS-047` 适配器面、`AT-TF-072` 编排面），且都点在已在册的①行上。机制与测试双双前进，但按本表口径①不动——④/② 的高估随之扩大，方向见方法节的重盘补充条。

### ①归属更新：16 行

集合不变，测试归属变了 16 行，已同步进①表：

- **补测增引 13 行**——`AT-NR-028`/`AT-NR-030`（+判断时间取时钟不取 asOf 测）、`AT-PC-006`（+锚点策略缺失未决测）、`AT-PC-022`（+同版本改绑引用冲突测）、`AT-PC-025`/`AT-PS-013`（+可达性判断被推翻停判测）、`AT-PS-020`/`AT-PS-031`（+修订规则未配置停等测）、`AT-PS-023`（+显式清除自成动作测）、`AT-PS-047`（+服务阶段规则适配器测）、`AT-PS-071`（+拒绝规则未配置停等测）、`AT-SA-167`（+冲正追加不删原始测）、`AT-TF-072`（+交付生效编排更正链测）。
- **函数改名/引用改动 3 行**——`AT-NR-025`（`TestServiceAreaEvaluationFoldsTheMatrixRows` → `TestServiceAreaEvaluationDrivesTheThreeValuedConclusion`）、`AT-PC-032`（→ `TestConflictOutranksAMissingBasis`）、`AT-TF-078`（→ `TestRegulatoryReturnIsMarkedByItsBasisKind`）。

### 点名票后第二次重盘（同日追记）

对 VE 五编排、NO 关务协作、SA 费用确认、CC 放行门禁的测试逐条对 UC 验收表补 AT 点名后重扫：

| 桶 | 重盘 `0cddbba` | 点名后 | 变化 |
|---|---|---|---|
| ① 已钉住 | 223 | 242 | +19（VE +16：038/039/040/042/044/062半边/064/065/079/092/099半边/100半边/106/156/157/161；NO +13−10=+3：008/009半边/012半边） |
| ② 机制候选 | 241 | 238 | −3（NO 三条迁①） |
| ③ 实例/闸门 | 148 | 148 | ±0 |
| ④ 机制未落地 | 471 | 455 | −16（VE 十六条迁①） |
| 合计 | 1083 | 1083 | ±0 |

SA 费用确认（3 测）与 CC 放行门禁（2 测）对表后**零点名**——所钉语义在结果契约与 CONTEXT 硬句上，验收表无对应行，按宁缺勿滥不硬点；两组继续按硬句口径计。

同日追记二：VE 批收尾票（ETA 与可见性缺口编排 `FormETAHandler`）随编排落地点名 `AT-VE-049`、`AT-VE-050`（七件缺一半边）、`AT-VE-053`、`AT-VE-054`、`AT-VE-057`——①242→247、④455→450，其余桶与合计 1083 不变。

同日追记三：VE 收官票（索赔与追偿编排 `HandleClaimHandler`）随编排落地点名 `AT-VE-114`（逐项独立半边）、`AT-VE-127`、`AT-VE-129`、`AT-VE-131`、`AT-VE-133`（分序半边）、`AT-VE-136`——①247→253、④450→444，合计 1083 不变；另 `AT-TF-080` 归属随 TF 侧新落的替代旅程编排测试增引（同步入①表）。

同日追记四（第二轮点名票）：CC 六编排点名 20 处（建案 008/009/010、舱单 380/381/387/389、后续动作 201/202/208、处置核对 233 半边/237/250、案件关闭 307/308、内部限制 345/346/347/348/370），TF 三处新入①（061/069 半边/073）加五处增引（016/019/020/024/062 编排面），NO 合箱三处（035/036/040）——①253→279（CC 0→20、NO 13→16、TF 22→25）、②238→232、③148→147、④444→425，合计 1083 守恒。PP/PG 无 AT 清单（PG 两编排的 GOV-03/04/06/07 机制半边已在测试注释点名，不入本表）；PC 声明与 SA 费用确认维持零点。另同步 `AT-TF-027`/`AT-TF-031` 归属漂移（机会准备编排测试增引）。

同日追记五（UC-TF-001 真空缺票）：监管处置承接编排 `AcceptRegulatoryDispositionHandler` 落地（新领域件 `regulatory_disposition.go` 三格决定+移动授权门、TF ports 尾部四件、编排+测试）并点名 `AT-TF-002`、`AT-TF-003`、`AT-TF-004`（授权缺口半边）、`AT-TF-006`（承接半边）、`AT-TF-010`（承接请求半边）——①279→284（TF 25→30）、②232→227，合计 1083 守恒；另同步 `AT-SA-002` 归属漂移（SA 批并行落测）。

### 两盘之间新落的硬句测试面（不入①，逐面列出供领票比对）

| 面 | 新测试文件 | 对应 UC 区域 |
|---|---|---|
| CC 编排×4 | submit_declaration / receive_external_result / verify_disposition / close_customs_case | UC-CC 申报提交、外部结果、处置核验、案件关闭 |
| CC/NO 领域 | duty_collaboration、customs_collaboration | 税费协作、节点关务协作 |
| VE 编排×5 | derive_projection / derive_customer_view / raise_signal / notify_customer / send_disposition_request | UC-VE 投影、客户视图、信号分诊、客户通知、处置请求 |
| PP | evaluate_pricing 编排、feature_category 领域 | UC-PP 计价评价 |
| PG | record_stage_review 编排 | 治理阶段评审 |
| SA | confirm_charge 编排 | UC-SA 费用确认 |
| TF | register_effective_delivery 编排、dispatch_task 领域 | UC-TF 交付生效、派送任务 |
| PS/PC | service_stage_rules 适配器、service_stage_content 领域 | 服务阶段规则声明（第十适配器） |
