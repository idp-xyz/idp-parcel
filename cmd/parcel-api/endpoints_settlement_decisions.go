package main

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

type settlementDecisionsWiring struct {
	confirmer settlementhttp.ChargeConfirmer
	publisher settlementhttp.StatementPublisher
	receiver  settlementhttp.SupplierBillReceiver
	auditor   settlementhttp.SupplierBillAuditor
}

func init() {
	registerEndpoints(endpointFeature[settlementDecisionsWiring]{
		lines: map[string]endpointLine{
			"/settlement-charge-confirmations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionChargeConfirmation),
			},
			"/settlement-statement-publications": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionStatementPublication),
			},
			"/settlement-supplier-bill-receptions": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionSupplierBillReception),
			},
			"/settlement-supplier-bill-audits": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionSupplierBillAudit),
			},
		},
		build: func(sources endpointSources) (settlementDecisionsWiring, error) {
			orchestrations, err := buildSettlementOrchestrations(sources.db)
			return settlementDecisionsWiring{
				confirmer: orchestrations, publisher: orchestrations,
				receiver: supplierBillReceiver{orchestrations}, auditor: supplierBillAuditor{orchestrations},
			}, err
		},
		mount: func(channels endpointChannels, wiring settlementDecisionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 运营确认口与截单发布口（ADR-0339 决定一、二）：两种结算运营决定，进的是 ADR-0170 装在这里的确认与截单编排，先过结算
				// 时点那一道门。写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/settlement-charge-confirmations", Handler: settlementhttp.NewConfirmChargeEndpoint(channels.operatorDecisions.settlement, wiring.confirmer)},
				{Pattern: "/settlement-statement-publications", Handler: settlementhttp.NewPublishStatementEndpoint(channels.operatorDecisions.settlement, wiring.publisher, systemClock{}.Now)},
				// 供应商账单接收口与审核口（ADR-0340 决定一、二）：进的是同一只接收编排，审核是它的 Audit。
				{Pattern: "/settlement-supplier-bill-receptions", Handler: settlementhttp.NewReceiveSupplierBillEndpoint(channels.operatorDecisions.settlement, wiring.receiver)},
				{Pattern: "/settlement-supplier-bill-audits", Handler: settlementhttp.NewAuditSupplierBillEndpoint(channels.operatorDecisions.settlement, wiring.auditor)},
			}
		},
	})
}

// supplierBillReceiver 与 supplierBillAuditor 把端点要的方法名接到 settlementOrchestrations 上那两个包事务的方法。
type supplierBillReceiver struct{ orchestrations settlementOrchestrations }

func (receiver supplierBillReceiver) Handle(
	ctx context.Context, command saapplication.ReceiveSupplierBillCommand,
) (saapplication.ReceiveSupplierBillResult, error) {
	return receiver.orchestrations.ReceiveSupplierBill(ctx, command)
}

type supplierBillAuditor struct{ orchestrations settlementOrchestrations }

func (auditor supplierBillAuditor) Audit(
	ctx context.Context, command saapplication.AuditBillLineCommand,
) (saapplication.AuditResult, error) {
	return auditor.orchestrations.AuditSupplierBill(ctx, command)
}
