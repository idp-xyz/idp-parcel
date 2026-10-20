package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

type veRegistrationWiring struct {
	milestoneMappingRegistration         visibilityhttp.MilestoneMappingRegistrar
	triageRulesRegistration              visibilityhttp.TriageRulesRegistrar
	notificationPolicyRegistration       visibilityhttp.NotificationPolicyRegistrar
	claimEligibilityRegistration         visibilityhttp.ClaimEligibilityRegistrar
	claimAuthorizationRegistration       visibilityhttp.ClaimAuthorizationRegistrar
	disclosurePolicyRegistration         visibilityhttp.DisclosurePolicyRegistrar
	exceptionDisclosureRulesRegistration visibilityhttp.ExceptionDisclosureRulesRegistrar
	conflictSignalRuleRegistration       visibilityhttp.ConflictSignalRuleRegistrar
}

func init() {
	registerEndpoints(endpointFeature[veRegistrationWiring]{
		lines: map[string]endpointLine{
			"/visibility-catalogue-milestone-mapping-registrations":         {method: http.MethodPost, face: registryWriteFace()},
			"/visibility-catalogue-triage-rule-registrations":               {method: http.MethodPost, face: registryWriteFace()},
			"/visibility-catalogue-notification-policy-registrations":       {method: http.MethodPost, face: registryWriteFace()},
			"/visibility-catalogue-claim-eligibility-registrations":         {method: http.MethodPost, face: registryWriteFace()},
			"/visibility-catalogue-claim-authorization-registrations":       {method: http.MethodPost, face: registryWriteFace()},
			"/visibility-catalogue-disclosure-policy-registrations":         {method: http.MethodPost, face: registryWriteFace()},
			"/visibility-catalogue-exception-disclosure-rule-registrations": {method: http.MethodPost, face: registryWriteFace()},
			"/visibility-catalogue-conflict-signal-rule-registrations":      {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (veRegistrationWiring, error) {
			veRegistration, err := buildVERegistrationOrchestration(sources.db)
			return veRegistrationWiring{
				milestoneMappingRegistration:         veRegistration.milestoneMapping,
				triageRulesRegistration:              veRegistration.triageRules,
				notificationPolicyRegistration:       veRegistration.notificationPolicy,
				claimEligibilityRegistration:         veRegistration.claimEligibility,
				claimAuthorizationRegistration:       veRegistration.claimAuthorization,
				disclosurePolicyRegistration:         veRegistration.disclosurePolicy,
				exceptionDisclosureRulesRegistration: veRegistration.exceptionDisclosureRules,
				conflictSignalRuleRegistration:       veRegistration.conflictSignalRule,
			}, err
		},
		mount: func(channels endpointChannels, wiring veRegistrationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// VE 六类配置登记写面（ADR-0085，票 admin-write-faces/02 切片 02d）：写准入不另
				// 立形，判据同网络目录登记写面。路径取「读口册名 + 该类种类词 + -registrations」，种类词与查阅
				// 的 `?kind=` 同字（换成小写连字）——同一本册在读口与写口不换词；分诊那一格 CLI
				// 叫 triage-rules 而读口 kind 叫 TRIAGE_RULE，路径随读口，单复数不在此处再分叉。
				{Pattern: "/visibility-catalogue-milestone-mapping-registrations", Handler: visibilityhttp.NewRegisterMilestoneMappingEndpoint(channels.operatorRegistries.visibility, wiring.milestoneMappingRegistration)},
				{Pattern: "/visibility-catalogue-triage-rule-registrations", Handler: visibilityhttp.NewRegisterTriageRulesEndpoint(channels.operatorRegistries.visibility, wiring.triageRulesRegistration)},
				{Pattern: "/visibility-catalogue-notification-policy-registrations", Handler: visibilityhttp.NewRegisterNotificationPolicyEndpoint(channels.operatorRegistries.visibility, wiring.notificationPolicyRegistration)},
				{Pattern: "/visibility-catalogue-claim-eligibility-registrations", Handler: visibilityhttp.NewRegisterClaimEligibilityEndpoint(channels.operatorRegistries.visibility, wiring.claimEligibilityRegistration)},
				{Pattern: "/visibility-catalogue-claim-authorization-registrations", Handler: visibilityhttp.NewRegisterClaimAuthorizationEndpoint(channels.operatorRegistries.visibility, wiring.claimAuthorizationRegistration)},
				{Pattern: "/visibility-catalogue-disclosure-policy-registrations", Handler: visibilityhttp.NewRegisterDisclosurePolicyEndpoint(channels.operatorRegistries.visibility, wiring.disclosurePolicyRegistration)},
				// 异常披露规则（0023）与冲突信号规则（0025）两册写面（票 ve-disclosure-policy-view/02
				// 步二，口径「同族一致」）：路径种类词随读口 kind 原词 EXCEPTION_DISCLOSURE_RULE /
				// CONFLICT_SIGNAL_RULE 变形，写准入同上，挂操作者渠道的登记册 Intake。
				{Pattern: "/visibility-catalogue-exception-disclosure-rule-registrations", Handler: visibilityhttp.NewRegisterExceptionDisclosureRulesEndpoint(channels.operatorRegistries.visibility, wiring.exceptionDisclosureRulesRegistration)},
				{Pattern: "/visibility-catalogue-conflict-signal-rule-registrations", Handler: visibilityhttp.NewRegisterConflictSignalRuleEndpoint(channels.operatorRegistries.visibility, wiring.conflictSignalRuleRegistration)},
			}
		},
	})
}
