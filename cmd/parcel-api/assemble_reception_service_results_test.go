package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是票 operator-channel/25 的真库半边：生产装配给收寄编排的服务结果读口，经 NO 的防腐适配器读 PS 的取消决定册与
// 终局册，在收寄那一笔事务里读得出。夹具全为合成登记（S 级）。

var serviceResultsAt = time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)

// seedParcelShipmentServiceResults 在 PS 两册上各登一份：parcel 上一份已成立的取消决定与一份已定案的终局，走 PS 的生产写口。
func seedParcelShipmentServiceResults(t *testing.T, db *bentopg.DB, tenant, parcel string) {
	t.Helper()
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		t.Fatal(err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatal(err)
	}
	psTenant := mustValue(t, psdomain.NewTenantID, tenant)
	psParcel := mustValue(t, psdomain.NewDeclaredParcelID, parcel)
	cancellation, err := psdomain.DecideParcelCancellation(psdomain.ParcelCancellationSpec{
		ID:          mustValue(t, psdomain.NewParcelCancellationID, "SYN-CANCEL-OC25"),
		Parcel:      psParcel,
		Requester:   mustValue(t, psdomain.NewCancellationRequesterReference, "SYN-CUSTOMER-OC25"),
		Authority:   mustValue(t, psdomain.NewCancellationAuthorityReference, "SYN-CANCEL-RULE-OC25"),
		Reason:      mustValue(t, psdomain.NewCancellationReasonReference, "SYN-CHANGED-MIND"),
		RequestedAt: serviceResultsAt,
	}, psdomain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("取消决定：%v", err)
	}
	finalKey := psports.FinalAdoptionKey{
		TenantID: psTenant,
		Parcel:   psParcel,
		Kind:     psdomain.EffectiveDeliveryOutcome,
		Version:  mustValue(t, psdomain.NewResponsibilityOutcomeVersion, "SYN-DELIVERY-OC25/v1"),
	}
	source, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind:       finalKey.Kind,
		Parcel:     psParcel,
		Decision:   mustValue(t, psdomain.NewResponsibilityDecisionReference, "SYN-DELIVERY-JUDGMENT-OC25"),
		Execution:  mustValue(t, psdomain.NewExecutionEvidenceReference, "SYN-POD-OC25"),
		Version:    finalKey.Version,
		OccurredAt: serviceResultsAt,
	})
	if err != nil {
		t.Fatalf("责任结果：%v", err)
	}
	final, err := psdomain.FormParcelFinalOutcome(psdomain.ParcelFinalOutcomeSpec{
		Version:     mustValue(t, psdomain.NewFinalOutcomeVersionID, "SYN-FINAL-OC25/v1"),
		Parcel:      psParcel,
		Kind:        mustValue(t, psdomain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
		Source:      source,
		RuleVersion: mustValue(t, psdomain.NewFinalRuleVersionReference, "SYN-FINAL-RULES-OC25/v1"),
	})
	if err != nil {
		t.Fatalf("终局：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := cancellations.Save(txCtx, psports.CancellationRecord{
			Key: psports.CancellationRequestKey{
				TenantID:   psTenant,
				RequestKey: mustValue(t, psdomain.NewSourceRequestKey, "SYN-CANCEL-REQUEST-OC25"),
				Parcel:     psParcel,
			},
			ContentDigest: "sha256:syn-cancel-oc25",
			Kind:          psports.RecordParcelCancelled,
			Cancellation:  cancellation,
			DecidedAt:     serviceResultsAt,
		}); err != nil {
			return err
		}
		_, err := finals.Save(txCtx, psports.FinalOutcomeRecord{
			Key:           finalKey,
			ContentDigest: "sha256:syn-final-oc25",
			Finalized:     true,
			Final:         final,
			AdoptedAt:     serviceResultsAt,
		})
		return err
	}); err != nil {
		t.Fatalf("登 PS 服务结果：%v", err)
	}
}

// Covers: 票 operator-channel/25 完成判据第三条的真库面——生产装配的服务结果读口在一笔事务里读出 PS 的取消决定与已定案终局，
// 另一件什么都没有的包裹答空。
func TestTheWiredServiceResultViewReadsParcelShipmentCancellationAndFinal(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seedParcelShipmentServiceResults(t, db, "SYN-TENANT-01", "SYN-PARCEL-OC25")
	view, err := buildParcelServiceResultView(db)
	if err != nil {
		t.Fatalf("装配服务结果读口：%v", err)
	}
	read := func(parcel string) noports.ParcelServiceResults {
		t.Helper()
		var results noports.ParcelServiceResults
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var findErr error
			results, findErr = view.FindParcelServiceResults(txCtx,
				mustValue(t, nodomain.NewTenantID, "SYN-TENANT-01"), mustValue(t, nodomain.NewParcelAssociationReference, parcel))
			return findErr
		}); err != nil {
			t.Fatalf("读 %s：%v", parcel, err)
		}
		return results
	}
	if got := read("SYN-PARCEL-OC25"); got != (noports.ParcelServiceResults{
		Cancellation: "SYN-CANCEL-OC25", FinalKind: "NETWORK_SERVICE_DELIVERED", FinalVersion: "SYN-FINAL-OC25/v1",
	}) {
		t.Fatalf("登过的包裹读回 %+v", got)
	}
	if got := read("SYN-PARCEL-OC25-NONE"); got != (noports.ParcelServiceResults{}) {
		t.Fatalf("什么都没登的包裹读回 %+v，want 空", got)
	}
}
