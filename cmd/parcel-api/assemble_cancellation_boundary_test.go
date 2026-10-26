package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// grantingCancellationAuthority 替的是唯一一条等租户登记的缝（PAR-COM-17 实例半边）：答允许并带依据，让
// 编排走到提交边界上那几道读。其余缝照生产原样（cancellationOrchestrationWith）。
type grantingCancellationAuthority struct{}

func (grantingCancellationAuthority) JudgeCancellationAuthority(
	context.Context, domain.SourceIdentity, domain.CancellationRequesterReference, domain.DeclaredParcelID,
) (psports.CancellationAuthorityJudgment, bool, error) {
	basis, err := domain.NewCheckReason("SYN-CANCEL-RULE/1")
	return psports.CancellationAuthorityJudgment{Granted: true, Basis: basis}, true, err
}

type fixedCancellationIntake struct {
	command shipmentapp.CancelParcelCommand
}

func (intake fixedCancellationIntake) IntakeCancellation(context.Context, *http.Request) (shipmentapp.CancelParcelCommand, error) {
	return intake.command, nil
}

func newBoundaryCancellation(t *testing.T) (*bentopg.DB, *pgxpool.Pool, shipmenthttp.CancellationHandler, domain.SourceIdentity) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cancellation, err := cancellationOrchestrationWith(db, grantingCancellationAuthority{})
	if err != nil {
		t.Fatalf("装配取消编排：%v", err)
	}
	identity := cancellationSeedIdentity(t, "SYN-SUBMIT-KEY-1")
	seedAcceptedShipmentRequest(t, db, identity)
	return db, pool, cancellation, identity
}

// Covers: `AT-PS-086` 与 PS CONTEXT「面单渠道服务包裹只允许在实际承运商收寄前……取消」经生产装配对真库——
// 提交边界上读真的 TF 首次有效收寄登记册：包裹已有已形成的收寄，取消被拒并指名那一代收寄，拒绝记录落库、
// 重放答原结果；包裹已有当前有效终局，经 HTTP 答 FINAL_OUTCOME_STANDS 并带出那份终局，取消一行都不落；
// 两样都没有时照常取消。授权换成已登记的答复（唯一的实例半边），其余缝全是生产的。测试输入是隔离合成，只记 `S`。
func TestTheWiredCancellationStopsAtTheBoundaryFactsAgainstARealDatabase(t *testing.T) {
	t.Run("a formed carrier pickup refuses the cancellation", func(t *testing.T) {
		db, _, cancellation, identity := newBoundaryCancellation(t)
		seedFormedCarrierPickup(t, db, "SYN-PARCEL-1")

		command := cancelParcelCommand(t, identity)
		result, err := cancellation.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("取消：%v", err)
		}
		if result.Outcome() != shipmentapp.CancellationRefused ||
			result.Basis().String() != "CARRIER_FIRST_EFFECTIVE_PICKUP/SYN-CFEP-1@SYN-CFEV-1" {
			t.Fatalf("outcome = %v basis = %q, want CANCELLATION_REFUSED 指名那一代收寄", result.Outcome(), result.Basis())
		}
		replay, err := cancellation.Handle(t.Context(), command)
		if err != nil || replay.Outcome() != shipmentapp.CancellationExistingResult {
			t.Fatalf("重放 outcome = %v err = %v, want EXISTING_RESULT——拒绝已落库", replay.Outcome(), err)
		}
	})

	t.Run("a current final is returned over HTTP and nothing is recorded", func(t *testing.T) {
		db, pool, cancellation, identity := newBoundaryCancellation(t)
		seedCurrentFinal(t, db, "SYN-PARCEL-1")

		endpoint := shipmenthttp.NewCancelParcelEndpoint(
			fixedCancellationIntake{command: cancelParcelCommand(t, identity)}, cancellation)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/shipment-requests/parcel-cancellations", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		var body struct {
			Outcome       string `json:"outcome"`
			StandingFinal string `json:"standingFinal"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Outcome != "FINAL_OUTCOME_STANDS" || body.StandingFinal != "FINAL_OUTCOME/SYN-FINAL-1" {
			t.Fatalf("body = %+v, want FINAL_OUTCOME_STANDS 带出那份终局", body)
		}
		if n := countRows(t, pool, `SELECT count(*) FROM parcel_shipment.parcel_cancellation`); n != 0 {
			t.Fatalf("既有终局之上落了 %d 行取消", n)
		}
	})

	t.Run("without boundary facts the cancellation stands", func(t *testing.T) {
		_, _, cancellation, identity := newBoundaryCancellation(t)
		result, err := cancellation.Handle(t.Context(), cancelParcelCommand(t, identity))
		if err != nil {
			t.Fatalf("取消：%v", err)
		}
		if result.Outcome() != shipmentapp.ParcelCancelled {
			t.Fatalf("outcome = %v, want PARCEL_CANCELLED", result.Outcome())
		}
	})
}

// seedFormedCarrierPickup 经 TF 领域构造门与生产登记册落一版已形成的首次有效收寄（对象即包裹标识原串）。
func seedFormedCarrierPickup(t *testing.T, db *bentopg.DB, parcel string) {
	t.Helper()
	carrier, err := tfdomain.NewCarrierSubject(tfdomain.ExternalCarrierParty, "SYN-CARRIER-PARTY-1")
	if err != nil {
		t.Fatalf("承运主体：%v", err)
	}
	basis, err := tfdomain.NewCarrierPickupBasis(tfdomain.CarrierDirectPickupScan, "SYN-SCAN-1", "v1")
	if err != nil {
		t.Fatalf("依据：%v", err)
	}
	occurredAt := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	pickup, err := tfdomain.FormCarrierFirstEffectivePickup(tfdomain.CarrierFirstEffectivePickupSpec{
		TenantID:   mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Object:     mustValue(t, tfdomain.NewCarriedObjectReference, parcel),
		Fact:       mustValue(t, tfdomain.NewCarrierFirstEffectivePickupReference, "SYN-CFEP-1"),
		Version:    mustValue(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "SYN-CFEV-1"),
		Carrier:    carrier,
		OccurredAt: occurredAt,
		JudgedAt:   occurredAt.Add(time.Minute),
		Bases:      []tfdomain.CarrierPickupBasis{basis},
	})
	if err != nil {
		t.Fatalf("形成收寄：%v", err)
	}
	registry, err := tfpostgres.NewCarrierFirstEffectivePickups(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	var outcome tfports.CarrierPickupSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, saveErr := registry.Save(txCtx, tfports.CarrierFirstEffectivePickupRecord{
			Key:        tfports.CarrierFirstEffectivePickupKey{TenantID: pickup.TenantID(), Fact: pickup.Fact(), Version: pickup.Version()},
			Pickup:     pickup,
			RecordedAt: occurredAt.Add(2 * time.Minute),
		})
		outcome = saved
		return saveErr
	}); err != nil {
		t.Fatalf("落收寄：%v", err)
	}
	if outcome != tfports.CarrierPickupSaved {
		t.Fatalf("收寄 outcome = %v", outcome)
	}
}

// seedCurrentFinal 经 PS 领域构造门与生产终局库落一份当前有效终局。
func seedCurrentFinal(t *testing.T, db *bentopg.DB, parcel string) {
	t.Helper()
	parcelID := mustValue(t, domain.NewDeclaredParcelID, parcel)
	occurredAt := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	source, err := domain.NewResponsibilityOutcome(domain.ResponsibilityOutcomeSpec{
		Kind:       domain.LabelServiceOutcome,
		Parcel:     parcelID,
		Decision:   mustValue(t, domain.NewResponsibilityDecisionReference, "LABEL-SERVICE-FINAL/"+parcel),
		Execution:  mustValue(t, domain.NewExecutionEvidenceReference, "SYN-CFEP-1@SYN-CFEV-1"),
		Version:    mustValue(t, domain.NewResponsibilityOutcomeVersion, "SYN-CFEV-1"),
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("责任结果：%v", err)
	}
	final, err := domain.FormParcelFinalOutcome(domain.ParcelFinalOutcomeSpec{
		Version:     mustValue(t, domain.NewFinalOutcomeVersionID, "SYN-FINAL-1"),
		Parcel:      parcelID,
		Kind:        mustValue(t, domain.NewFinalKindReference, "SYN-LABEL-SERVICE-FINAL"),
		Source:      source,
		RuleVersion: mustValue(t, domain.NewFinalRuleVersionReference, "SYN-FINAL-RULE/1"),
	})
	if err != nil {
		t.Fatalf("终局：%v", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatalf("构造终局库：%v", err)
	}
	var outcome psports.FinalOutcomeSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, saveErr := finals.Save(txCtx, psports.FinalOutcomeRecord{
			Key: psports.FinalAdoptionKey{
				TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
				Parcel:   parcelID,
				Kind:     domain.LabelServiceOutcome,
				Version:  source.Version(),
			},
			ContentDigest: "SYN-FINAL-DIGEST-1",
			Finalized:     true,
			Final:         final,
			AdoptedAt:     occurredAt.Add(time.Minute),
		})
		outcome = saved
		return saveErr
	}); err != nil {
		t.Fatalf("落终局：%v", err)
	}
	if outcome != psports.FinalOutcomeSaved {
		t.Fatalf("终局 outcome = %v", outcome)
	}
}
