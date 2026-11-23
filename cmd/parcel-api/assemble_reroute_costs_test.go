package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证授权改路决定的候选成本取数侧在生产装配里取包裹事实的接线：
// 没登实测时退回客户声明并标明；登了实测后按实测出价。成本取数侧的装配失败不在构造处报错，
// 而是留到取数时如实报出，所以先认它不是装坏了的那一份。夹具为隔离合成，只记 S。

// Covers: rerouteCosts 的物理事实来源接线——委托已接受且带客户声明 2.5 KG，没登实测时授权改路决定经生产装配取包裹事实：
// 取数侧不是装配失败的那一份，成本依据标 DECLARATION_FALLBACK（退回声明并标明），而不是 DECLARATION（当成预路由）或 NONE（声明没读到）。
func TestWithoutARegisteredMeasurementTheRerouteDecisionFallsBackToTheDeclarationAndSaysSo(t *testing.T) {
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
	costs, err := source.LoadCandidateCosts(t.Context(), rerouteJudgmentKey(t, command), nrdomain.NewDecisionFactsPoint(), nrports.InitialRouteEvidence{}, nrports.RequestCarriedContent{})
	if err != nil {
		t.Fatalf("授权改路决定的候选成本：%v", err)
	}
	if costs.FactsBasis != nrdomain.ParcelFactsDeclarationFallback {
		t.Fatalf("成本依据 = %s, want %s", costs.FactsBasis, nrdomain.ParcelFactsDeclarationFallback)
	}
}

// Covers: 登了一条 7 KG 的实测之后，授权改路决定经生产装配按实测出价（一段 40 USD，4000 分）、出处 PHYSICAL。
// 同一张卡上声明 2.5 KG 落 10 USD；按声明出 1000 分即说明没读到这条实测。
func TestTheRerouteDecisionPricesTheCandidateOnTheRegisteredMeasurement(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	command := submissionCommand(t)
	parcel := command.DeclaredParcelIDs[0]
	command.DeclaredProfiles = []shipmentdomain.DeclaredParcelProfile{rerouteDeclaredProfile(t, parcel, "2.5", "KG")}
	acceptOnRealAssembly(t, db, command)
	registerRerouteSupplierLine(t, db, command.Identity.TenantID().String())
	registerRerouteSupplierCard(t, db)
	associateRerouteParcel(t, pool, command.Identity.TenantID().String(), parcel.String())
	recordRerouteMeasurement(t, db, command.Identity.TenantID().String(), "SYN-HU-RPF-01", "SYN-RPF-MEASURE-01", "7")

	source := rerouteCosts(db, systemClock{})
	if broken, isBroken := source.(brokenRerouteCostSource); isBroken {
		t.Fatalf("候选成本取数侧装配失败：%v", broken.err)
	}
	candidate, err := nrdomain.NewLineCandidateID("SYN-RPF-LINE", 1)
	if err != nil {
		t.Fatal(err)
	}
	evidence := nrports.InitialRouteEvidence{Paths: []nrports.CandidatePath{{
		Candidate: candidate,
		Legs:      make([]nrdomain.PlannedLeg, 1),
	}}}
	costs, err := source.LoadCandidateCosts(t.Context(), rerouteJudgmentKey(t, command), nrdomain.NewDecisionFactsPoint(), evidence, nrports.RequestCarriedContent{})
	if err != nil {
		t.Fatalf("授权改路决定的候选成本：%v", err)
	}
	if len(costs.Facts) != 1 || costs.Facts[0].Candidate().String() != "SYN-RPF-LINE@1" {
		t.Fatalf("成本事实 = %+v，want 这一条线路", costs.Facts)
	}
	if fact := costs.Facts[0]; fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 4000 {
		t.Fatalf("授权改路决定 = %s %d minor，want 按实测 7 KG 的 4000", fact.State(), fact.AmountMinor())
	}
	if costs.FactsBasis != nrdomain.ParcelFactsPhysical {
		t.Fatalf("成本依据 = %s, want %s", costs.FactsBasis, nrdomain.ParcelFactsPhysical)
	}
}

func rerouteJudgmentKey(t *testing.T, command shipmentapp.SubmitShipmentRequestCommand) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	parcel := command.DeclaredParcelIDs[0]
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           mustNRValue(t, nrdomain.NewTenantID, command.Identity.TenantID().String()),
		CustomerAccountID:  mustNRValue(t, nrdomain.NewCustomerAccountID, command.Identity.CustomerAccountID().String()),
		ShipmentRequestID:  mustNRValue(t, nrdomain.NewShipmentRequestID, command.ShipmentRequestID.String()),
		AcceptanceBaseline: mustNRValue(t, nrdomain.NewAcceptanceBaselineReference, command.ShipmentRequestID.String()+"/v1"),
		DeclaredParcelID:   mustNRValue(t, nrdomain.NewDeclaredParcelID, parcel.String()),
		ServicePurpose:     mustNRValue(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

// registerRerouteSupplierLine 登一条一段的线路、供应商采购成本依据和段区 Z1。价卡不绑分区目录，出价用的就是这一段登的区；
// 段链不需要真有连接，取数只按线路版本与段号读依据和区。
func registerRerouteSupplierLine(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	id := mustNRValue(t, nrdomain.NewTenantID, tenant)
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if err := catalog.RegisterLineVersion(ctx, id, nrports.LineDefinitionVersion{
			Code: "SYN-RPF-LINE", Version: 1,
			Segments:         []string{"SYN-RPF-CONN"},
			BusinessTimezone: "Asia/Shanghai",
			ApplicableScope:  "SYN-RPF-SCOPE",
			EffectiveFrom:    time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		}); err != nil {
			return err
		}
		if err := catalog.RegisterLineCostBases(ctx, id, "SYN-RPF-LINE", 1, []nrports.LineSegmentCostBasis{{
			SegmentIndex: 0,
			Kind:         nrports.SupplierBuyPlanBasis,
			Reference:    "SYN-RPF-PLAN/v1",
		}}); err != nil {
			return err
		}
		return catalog.RegisterLegPricingZones(ctx, id, "SYN-RPF-LINE", 1, []nrports.LegPricingZone{{
			SegmentIndex: 0, Zone: "Z1", Basis: "SYN-RPF-ZONE",
		}})
	})
	if err != nil {
		t.Fatalf("登记线路：%v", err)
	}
}

func registerRerouteSupplierCard(t *testing.T, db *bentopg.DB) {
	t.Helper()
	cards, err := buildPriceCardRegistrationOrchestration(db, false)
	if err != nil {
		t.Fatal(err)
	}
	registration := rerouteSupplierCostCard(t)
	publishDirectionAuthorizationForCard(t, db, registration)
	outcome, err := cards.Handle(t.Context(), ppapplication.RegisterPriceCardCommand{Registration: registration})
	if err != nil {
		t.Fatalf("登记价卡：%v", err)
	}
	if outcome != ppapplication.PriceCardRecorded {
		t.Fatalf("价卡登记 = %s，want 已入册", outcome)
	}
}

// rerouteSupplierCostCard 是一段 BUY·供应商成本卡：Z1 两档，[0,5) 10 USD、[5,30) 40 USD。声明 2.5 KG 与实测 7 KG 落不同档。
// 适用期从 2020 起不封口，判断时钟取此刻也落在期内。
func rerouteSupplierCostCard(t *testing.T) ppdomain.PriceCardRegistration {
	t.Helper()
	currency := mustValue(t, ppdomain.NewCurrency, "USD")
	low, err := ppdomain.NewMoneyFromString("10", currency)
	if err != nil {
		t.Fatal(err)
	}
	high, err := ppdomain.NewMoneyFromString("40", currency)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ppdomain.NewRateEntry(
		mustValue(t, ppdomain.NewRateEntryID, "SYN-RPF-ENTRY-LO"),
		"Z1", rerouteWeight(t, "0"), rerouteWeight(t, "5"), low)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ppdomain.NewRateEntry(
		mustValue(t, ppdomain.NewRateEntryID, "SYN-RPF-ENTRY-HI"),
		"Z1", rerouteWeight(t, "5"), rerouteWeight(t, "30"), high)
	if err != nil {
		t.Fatal(err)
	}
	period, err := ppdomain.NewUnboundedEffectivePeriod(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	table, err := ppdomain.NewRateTableVersion(
		rerouteReference(t, ppdomain.ArtifactRateTable, "SYN-RPF-TABLE", "v1"),
		ppdomain.RateTableFamilyWeightZone, currency, ppdomain.WeightUnitKilogram,
		period, []ppdomain.RateEntry{first, second})
	if err != nil {
		t.Fatal(err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, rerouteWeight(t, "0.5"))
	if err != nil {
		t.Fatal(err)
	}
	weightPolicy, err := ppdomain.NewPricingWeightPolicy(
		rerouteReference(t, ppdomain.ArtifactWeightPolicy, "SYN-RPF-WEIGHT", "v1"),
		ppdomain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(
		rerouteReference(t, ppdomain.ArtifactPricingPlan, "SYN-RPF-PLAN", "v1"),
		mustValue(t, ppdomain.NewPricingScopeID, "SYN-RPF-SCOPE"),
		ppdomain.PricingDirectionBuy,
		ppdomain.PricingPurposeSupplierCost,
		mustValue(t, ppdomain.NewChargeCode, "BASE_FREIGHT"),
		period, table, weightPolicy, nil, ppdomain.PricingPlanStructures{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := ppdomain.NewSourceFileIdentity(
		"SYN-RPF-CARD.xlsx",
		"9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := ppdomain.NewPriceCardRegistration(
		mustValue(t, ppdomain.NewTenantID, "SYN-TENANT-1"),
		plan, source,
		rerouteReference(t, ppdomain.ArtifactCommercialAuthorization, "SYN-RPF-GRANT", "v1"),
		"SYN-RPF-GOVERNANCE")
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func rerouteReference(t *testing.T, artifact ppdomain.ArtifactKind, id, version string) ppdomain.VersionReference {
	t.Helper()
	reference, err := ppdomain.NewVersionReferenceIdentity(artifact, id, version)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func rerouteWeight(t *testing.T, value string) ppdomain.Weight {
	t.Helper()
	weight, err := ppdomain.NewWeight(mustValue(t, ppdomain.ParseDecimal, value), ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	return weight
}

func associateRerouteParcel(t *testing.T, pool *pgxpool.Pool, tenant, parcel string) {
	t.Helper()
	intake := fmt.Sprintf(`{"unit":"SYN-HU-RPF-01","association":%q,"version":"v1"}`, parcel)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
			 service_markers, recorded_at, judgment_version, predecessor_version)
		 VALUES ($1, 'SYN-RPF-RECEPTION-01', 'digest', 'INTAKE_FORMED', $2::jsonb, '{}'::jsonb, '[]'::jsonb, false,
		         '[]'::jsonb, now(), 'v1', NULL)`,
		tenant, intake); err != nil {
		t.Fatalf("登记包裹关联：%v", err)
	}
}

func recordRerouteMeasurement(t *testing.T, db *bentopg.DB, tenant, unit, sourceID, kilograms string) {
	t.Helper()
	occurred := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		mustValue(t, nodomain.NewPerformingPartyReference, "SYN-SCALE-RPF"),
		mustValue(t, nodomain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
		occurred,
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	gross, err := nodomain.NewGrossWeight(kilograms, "KG")
	if err != nil {
		t.Fatalf("毛重：%v", err)
	}
	outer, err := nodomain.NewOuterDimensions("30", "20", "10", "CM")
	if err != nil {
		t.Fatalf("外廓：%v", err)
	}
	measurement, err := nodomain.NewActualMeasurement(nodomain.ActualMeasurementSpec{
		Source: source, ObjectKind: nodomain.HandlingUnitObject, ObjectID: unit,
		Node: mustValue(t, nodomain.NewNodeReference, "node-origin"), Location: mustValue(t, nodomain.NewWorkLocationReference, "bench-2"),
		Basis:  mustValue(t, nodomain.NewWorkBasisReference, "intake-1"),
		Weight: gross, Dimensions: outer,
	})
	if err != nil {
		t.Fatalf("测量：%v", err)
	}
	measurements, err := nopostgres.NewActualMeasurements(db)
	if err != nil {
		t.Fatal(err)
	}
	record := noports.ActualMeasurementRecord{
		Key:           noports.ActualMeasurementKey{TenantID: mustValue(t, nodomain.NewTenantID, tenant), SourceID: sourceID},
		Measurement:   measurement,
		ContentDigest: measurement.ContentDigest(),
		RecordedAt:    occurred.Add(time.Minute),
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, err := measurements.SaveMeasurement(ctx, record)
		if err != nil {
			return err
		}
		if outcome != noports.ActualMeasurementSaved {
			return fmt.Errorf("登记实测 %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("登记实测：%v", err)
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
