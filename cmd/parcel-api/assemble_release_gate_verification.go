package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 本文件是放行门禁核对口的组合根（票 launch-wiring/03，UC-CC-009，code/34）。

// buildReleaseGateVerification 装配 `/customs-release-gate-verifications` 背后的真编排。缝全接真：门禁目录与「税费付款」规则行同一只读适配器、
// 付款核对当前版读口——与动作准入判断接的是同一批（读法只在 assessReleaseGate 一处定义，buildCustomsAdmissionOrchestration）——再加
// 门禁核对册、核对意图的 Outbox 交付与时钟。壳复用关务三口那一只：核对记录与它的发布意图同笔落地；未决（目录未登记、规则未配置、
// 折不出结论、读不动）整笔回滚、不落册。目录与规则的取值是租户取值，没登时本口如实答未决并指名未配置，不为此种任何行。
func buildReleaseGateVerification(db *bentopg.DB) (customsDecisionTransaction[customsapp.VerifyReleaseGateCommand, customsapp.VerifyReleaseGateResult], error) {
	none := customsDecisionTransaction[customsapp.VerifyReleaseGateCommand, customsapp.VerifyReleaseGateResult]{}
	gates, err := ccpostgres.NewGateConditionView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs gate condition view: %w", err)
	}
	dutyVerifications, err := ccpostgres.NewDutyPaymentReconciliation(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs duty payment reconciliation store: %w", err)
	}
	verifications, err := ccpostgres.NewGateVerifications(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs gate verifications: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := ccpostgres.NewOutboxGateVerificationHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: gate verification handoff: %w", err)
	}
	handler, err := customsapp.NewVerifyReleaseGateHandler(customsapp.VerifyReleaseGateDeps{
		Conditions: gates, DutyRules: gates, DutyVerifications: dutyVerifications,
		Store: verifications, Downstream: handoff, Clock: clock,
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: verify release gate: %w", err)
	}
	return customsDecisionTransaction[customsapp.VerifyReleaseGateCommand, customsapp.VerifyReleaseGateResult]{
		transactor: db.Transactor(), handle: handler.Handle,
		undecided: func(result customsapp.VerifyReleaseGateResult) bool {
			return result.Outcome() == customsapp.GateVerificationUndecided
		},
	}, nil
}
