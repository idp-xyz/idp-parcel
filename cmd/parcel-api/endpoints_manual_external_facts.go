package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type manualExternalFactsWiring struct {
	externalFundsFactAdoption           settlementhttp.ExternalFundsFactRegistrar
	externalFundsFactCorrectionAdoption settlementhttp.ExternalFundsFactCorrectionRegistrar
	regulatoryCredentialRegistration    customshttp.RegulatoryCredentialRegistrar
	externalResults                     customshttp.ResultHandler
	trackingEntries                     tfhttp.TrackingMaterialAdopter
	manualLodgement                     customshttp.ManualLodgementRecorder
	manifests                           customshttp.ManifestHandler
}

func init() {
	registerEndpoints(endpointFeature[manualExternalFactsWiring]{
		lines: map[string]endpointLine{
			"/settlement-external-funds-fact-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionExternalFundsFactAdoption),
			},
			"/settlement-external-funds-fact-correction-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionExternalFundsFactAdoption),
			},
			"/customs-regulatory-credential-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionRegulatoryCredentialRegistration),
			},
			"/customs-external-result-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomsExternalResultEntry),
			},
			"/customs-external-manifest-reference-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionExternalManifestEntry),
			},
			"/customs-external-manifest-reference-revision-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionExternalManifestEntry),
			},
			"/customs-declaration-manual-lodgements": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomsManualLodgement),
			},
			"/transport-fulfillment-external-tracking-manual-entries": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionExternalTrackingEntry),
			},
		},
		build: func(sources endpointSources) (manualExternalFactsWiring, error) {
			// 编排取集成客户端族那边的同一套构造（buildSettlementRegistrationOrchestration、buildCustomsRegistrationOrchestration）：两族只差
			// Intake，采用、更正与凭证登记的规则、幂等与答案代数各只有一份。
			settlement, err := buildSettlementRegistrationOrchestration(sources.db)
			if err != nil {
				return manualExternalFactsWiring{}, err
			}
			customs, err := buildCustomsRegistrationOrchestration(sources.db)
			if err != nil {
				return manualExternalFactsWiring{}, err
			}
			// 海关外部结果的手工录入进的是集成客户端口后面的同一只接收编排。
			results, err := buildExternalResultsOrchestration(sources.db)
			if err != nil {
				return manualExternalFactsWiring{}, err
			}
			tracking, err := buildManualTrackingAdopter(sources.db)
			if err != nil {
				return manualExternalFactsWiring{}, err
			}
			lodgement, err := buildManualLodgement(sources.db)
			if err != nil {
				return manualExternalFactsWiring{}, err
			}
			// 外部舱单引用的手工录入进的是集成客户端口后面的同一只编排。
			manifests, err := buildManifestReferencesOrchestration(sources.db)
			return manualExternalFactsWiring{
				manifests:                           manifests,
				manualLodgement:                     lodgement,
				trackingEntries:                     tracking,
				externalResults:                     results,
				externalFundsFactAdoption:           settlement.externalFundsFact,
				externalFundsFactCorrectionAdoption: settlement.externalFundsFactCorrection,
				regulatoryCredentialRegistration:    customs.regulatoryCredential,
			}, err
		},
		mount: func(channels endpointChannels, wiring manualExternalFactsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 外部资金事实与监管凭证的手工采用口（ADR-0236）：租户的财务、关务人员照银行回单或凭证原件在管理台录入，挂「运营
				// 决定」能力面，两类事实各按自己那一种认证，提交操作者记成操作证据。与集成客户端族的同名口并存而不替代：有接口的外部
				// 系统照旧推，没接口的由人照单录，路径带 manual 让两族在端点表上一眼分得开。写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/settlement-external-funds-fact-manual-registrations", Handler: settlementhttp.NewRegisterExternalFundsFactEndpoint(channels.operatorDecisions.settlement, wiring.externalFundsFactAdoption)},
				{Pattern: "/settlement-external-funds-fact-correction-manual-registrations", Handler: settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(channels.operatorDecisions.settlement, wiring.externalFundsFactCorrectionAdoption)},
				{Pattern: "/customs-regulatory-credential-manual-registrations", Handler: customshttp.NewRegisterRegulatoryCredentialEndpoint(channels.operatorDecisions.customs, wiring.regulatoryCredentialRegistration)},
				// 海关外部结果的手工录入口（dox/decisions.md「产品形态」）：没有对接海关或报关行系统的租户，由关务人员照回执录入。
				// 录入人与所凭原件记在结果上，与接口来的分得开。
				{Pattern: "/customs-external-result-manual-registrations", Handler: customshttp.NewReceiveExternalResultEndpoint(customshttp.ManualResultIntake(channels.operatorDecisions.customs, systemClock{}.Now), wiring.externalResults)},
				// 承运商轨迹的手工录入口（dox/decisions.md「产品形态」）：一次一条或一批，逐条认领、各自一笔事务。
				{Pattern: "/transport-fulfillment-external-tracking-manual-entries", Handler: tfhttp.NewManualTrackingEntriesEndpoint(channels.operatorDecisions.transport, wiring.trackingEntries, systemClock{}.Now)},
				// 申报的线下递交标记口（dox/decisions.md「产品形态」）：没有对接报关行或单一窗口时，线下递交之后回来记一笔。
				{Pattern: "/customs-declaration-manual-lodgements", Handler: customshttp.NewRecordManualLodgementEndpoint(channels.operatorDecisions.customs, wiring.manualLodgement)},
				// 外部监管舱单引用的手工录入两口（dox/decisions.md「产品形态」）：首登与版本推进，承运商由录入的人照材料指名。
				{Pattern: "/customs-external-manifest-reference-manual-registrations", Handler: customshttp.NewReceiveManifestReferenceEndpoint(customshttp.ManualManifestIntake(channels.operatorDecisions.customs, systemClock{}.Now), wiring.manifests)},
				{Pattern: "/customs-external-manifest-reference-revision-manual-registrations", Handler: customshttp.NewReviseManifestReferenceEndpoint(customshttp.ManualManifestIntake(channels.operatorDecisions.customs, systemClock{}.Now), wiring.manifests)},
			}
		},
	})
}
