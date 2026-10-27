package main

import (
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证授权改路决定的候选成本取数侧在生产装配里取包裹事实的接线（ADR-0294）：包裹事实按判断点选来源，
// 物理事实来源装「一项都没取得」（node-operations 还没有实际测量登记册），所以取判断此刻物理事实的授权改路决定退回客户声明
// 出价并如实标明。成本取数侧的装配失败不在构造处报错，而是留到取数时如实报出，所以先认它不是装坏了的那一份。

// Covers: rerouteCosts 的物理事实来源接线——委托已接受且带客户声明 2.5 KG，授权改路决定的判断点经生产装配取包裹事实：取数侧
// 不是装配失败的那一份，成本依据标 DECLARATION_FALLBACK（退回声明并标明），而不是 DECLARATION（当成预路由）或 NONE（声明没读到）。
// 物理事实来源若接成空，装配当场失败，取数时整次报错。
func TestTheRerouteDecisionCostSourceFallsBackToTheDeclarationBecauseProductionHasNoPhysicalFactSource(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	command := submissionCommand(t)
	parcel := command.DeclaredParcelIDs[0]
	command.DeclaredProfiles = []shipmentdomain.DeclaredParcelProfile{rerouteDeclaredProfile(t, parcel, "2.5", "KG")}
	acceptOnRealAssembly(t, db, command)

	source := rerouteCosts(db, systemClock{})
	if broken, isBroken := source.(brokenRerouteCostSource); isBroken {
		t.Fatalf("候选成本取数侧装配失败：%v", broken.err)
	}
	key := nrdomain.InitialRouteJudgmentKey{
		TenantID:           mustNRValue(t, nrdomain.NewTenantID, command.Identity.TenantID().String()),
		CustomerAccountID:  mustNRValue(t, nrdomain.NewCustomerAccountID, command.Identity.CustomerAccountID().String()),
		ShipmentRequestID:  mustNRValue(t, nrdomain.NewShipmentRequestID, command.ShipmentRequestID.String()),
		AcceptanceBaseline: mustNRValue(t, nrdomain.NewAcceptanceBaselineReference, command.ShipmentRequestID.String()+"/v1"),
		DeclaredParcelID:   mustNRValue(t, nrdomain.NewDeclaredParcelID, parcel.String()),
		ServicePurpose:     mustNRValue(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}

	costs, err := source.LoadCandidateCosts(t.Context(), key, nrdomain.NewDecisionFactsPoint(), nrports.InitialRouteEvidence{}, nrports.RequestCarriedContent{})
	if err != nil {
		t.Fatalf("授权改路决定的候选成本：%v", err)
	}
	if costs.FactsBasis != nrdomain.ParcelFactsDeclarationFallback {
		t.Fatalf("成本依据 = %s, want %s", costs.FactsBasis, nrdomain.ParcelFactsDeclarationFallback)
	}
}

// rerouteDeclaredProfile 造一件声明包裹的客户申报画像：只报重量，值与单位原样。
func rerouteDeclaredProfile(t *testing.T, parcel shipmentdomain.DeclaredParcelID, weight, unit string) shipmentdomain.DeclaredParcelProfile {
	t.Helper()
	declaredWeight, err := shipmentdomain.NewDeclaredWeight(
		mustValue(t, shipmentdomain.NewMeasurementValue, weight),
		mustValue(t, shipmentdomain.NewMeasurementUnitReference, unit))
	if err != nil {
		t.Fatalf("申报重量：%v", err)
	}
	measurement, err := shipmentdomain.NewDeclaredMeasurement(declaredWeight, shipmentdomain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	profile, err := shipmentdomain.NewDeclaredParcelProfile(parcel, measurement)
	if err != nil {
		t.Fatalf("申报画像：%v", err)
	}
	return profile
}
