package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证票 88 的补形成与票 57 的更正两口合在一起之后的两处接缝（ADR-0231、ADR-0232）：自动形成不把运营决定失效过的
// 发生项长回来，除非这一段在失效之后才落下当前的采用；委托换版之后按后继形成的发生项照样挡住解除采用。段、参与、采用、换版、失效与解除
// 都走生产编排，派发照生产路由表一拍一拍跑。夹具全部合成，证据只记 `S`。

const correctionOperator = "https://dex.syn.example#SYN-OPERATOR-CONFIG-01"

// invalidateFormed 走真失效编排（同一笔落失效版本、交已形成信封），让 occurrence 的 version 那一版失效。
func invalidateFormed(t *testing.T, db *bentopg.DB, store *outbox.Store, occurrence, version string) {
	t.Helper()
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := tfpostgres.NewOutboxChargeOccurrenceHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	handler := tfapplication.NewInvalidateChargeOccurrenceHandler(tfapplication.InvalidateChargeOccurrenceDeps{
		Occurrences: occurrences, Scopes: occurrences, Handoff: handoff, Clock: systemClock{},
	})
	var result tfapplication.ChargeOccurrenceInvalidationResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = handler.Invalidate(txCtx, tfapplication.InvalidateChargeOccurrenceCommand{
			TenantID: mustTF(t, tfdomain.NewTenantID, formationTenant), Operator: correctionOperator,
			Occurrence: occurrence, Version: version, Basis: "SYN-DECISION-INVALIDATE-DCG57",
		})
		return err
	})
	if result.Outcome() != tfapplication.ChargeOccurrenceInvalidated {
		t.Fatalf("失效 %s %s = %s", occurrence, version, result.Outcome())
	}
}

// withdrawFormationAdoption 走真解除编排，交回答复；被挡的那几格不落任何东西，整笔照样提交。
func withdrawFormationAdoption(t *testing.T, db *bentopg.DB, generation int) tfapplication.AdoptionWithdrawalResult {
	t.Helper()
	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		t.Fatal(err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	handler := tfapplication.NewWithdrawSegmentCommissionAdoptionHandler(tfapplication.WithdrawSegmentCommissionAdoptionDeps{
		Adoptions: adoptions, Withdrawals: adoptions, CommissionLinks: adoptions,
		Occurrences: occurrences, OccurrenceScope: occurrences,
		Commissions: commissions, CommissionHold: commissions, StartCorrection: commissions,
		Clock: systemClock{},
	})
	var result tfapplication.AdoptionWithdrawalResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = handler.Withdraw(txCtx, tfapplication.WithdrawSegmentCommissionAdoptionCommand{
			TenantID: mustTF(t, tfdomain.NewTenantID, formationTenant), Operator: correctionOperator,
			Segment: formationSegment, Commission: formationCommission, Generation: generation,
		})
		return err
	})
	return result
}

// readoptTrunkCommission 走真采用编排，在解除之后接着 after 那一代再采用同一份委托（同笔交采用门铃）。
func readoptTrunkCommission(t *testing.T, db *bentopg.DB, store *outbox.Store, after int) {
	t.Helper()
	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	doorbell, err := tfpostgres.NewOutboxSegmentCommissionAdoptionHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	adopter := tfapplication.NewAdoptSegmentCommissionHandler(tfapplication.AdoptSegmentCommissionDeps{
		Adoptions: adoptions, Segments: segments, Commissions: commissions, StartAdvancement: commissions, CommissionHold: commissions,
		Doorbell: doorbell, Withdrawals: adoptions, Clock: systemClock{},
	})
	var adopted tfapplication.CommissionAdoptionResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		adopted, err = adopter.Adopt(txCtx, tfapplication.AdoptSegmentCommissionCommand{
			TenantID: mustTF(t, tfdomain.NewTenantID, formationTenant), Segment: formationSegment, Commission: formationCommission,
			Provider: "SYN-PARTY-CARRIER-TRUNK-01", AfterGeneration: after,
		})
		return err
	})
	if adopted.Outcome() != tfapplication.CommissionAdopted {
		t.Fatalf("再采用：%s", adopted.Outcome())
	}
}

type occurrenceVersionRow struct {
	version, revisionKind, agreement string
}

// occurrenceVersions 按版本号列出一项发生项的各版：版本、修订种类（首版为空）与协议快照。
func occurrenceVersions(t *testing.T, db *bentopg.DB, occurrence string) []occurrenceVersionRow {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT validity_version, COALESCE(revision_kind, ''), agreement_ref
		   FROM transport_fulfillment.transport_charge_occurrence
		  WHERE tenant_id = $1 AND occurrence_ref = $2
		  ORDER BY length(validity_version), validity_version`, formationTenant, occurrence)
	if err != nil {
		t.Fatalf("读发生项各版：%v", err)
	}
	defer rows.Close()
	var versions []occurrenceVersionRow
	for rows.Next() {
		var row occurrenceVersionRow
		if err := rows.Scan(&row.version, &row.revisionKind, &row.agreement); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return versions
}

// Covers: AT-TF-312 的自动形成那一格（S，ADR-0232 决定五，与 ADR-0231 决定五的换版补形成合在一起）——SIN 交接之后自动形成了那一项，
// 运营人员让它失效；委托随后换版到 v2，换版门铃一拍补形成：同一件事实以系统身份再交登记，答已存在、不长出替代版本，门铃 PUBLISHED。
// 接着解除采用（链头已失效，不再挡）、接在第 1 代之后再采用：这一段的当前采用晚于那一次失效，采用门铃的补形成照常从失效版本长出 V3，
// 按 v2 的快照——指错了委托的更正一路走完，不必人工重登。
func TestAnInvalidatedOccurrenceIsNotRegrownByCatchUpUntilTheSegmentIsAdoptedAgain(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG57-01"
	beat, db, store := wiredBeat(t)
	publishTrunkVersion(t, db, "v1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)
	publishTrunkVersion(t, db, "v2", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	adoptTrunkCommission(t, db)
	dispatchUntilPublished(t, beat, db, handOverAtSIN(t, register, db, parcel))
	formed := formationOccurrences(t, db)
	if len(formed) != 1 || formed[0].agreement != trunkAgreementObject+"/v1" {
		t.Fatalf("自动形成 = %+v", formed)
	}
	occurrence := formed[0].occurrence

	invalidateFormed(t, db, store, occurrence, "V1")
	reviseTrunkCommission(t, db, store, "v2", formationAt(0))
	dispatchBeats(t, beat, 3)
	for id, status := range outboxEventsOf(t, db, tfpostgres.CommissionAgreementRevisedEventType) {
		if status != "PUBLISHED" {
			t.Fatalf("换版门铃 %s = %s（%s）", id, status, recordedFailureCode(t, db, id))
		}
	}
	if versions := occurrenceVersions(t, db, occurrence); len(versions) != 2 || versions[1].revisionKind != tfdomain.OccurrenceInvalidated.String() {
		t.Fatalf("换版补形成之后各版 = %+v, want 只有 V1 与失效版本 V2", versions)
	}

	if withdrawn := withdrawFormationAdoption(t, db, 1); withdrawn.Outcome() != tfapplication.AdoptionWithdrawn {
		t.Fatalf("链头已失效之后解除 = %s", withdrawn.Outcome())
	}
	readoptTrunkCommission(t, db, store, 1)
	dispatchBeats(t, beat, 3)
	for id, status := range outboxEventsOf(t, db, tfpostgres.SegmentCommissionAdoptionRegisteredEventType) {
		if status != "PUBLISHED" {
			t.Fatalf("采用门铃 %s = %s（%s）", id, status, recordedFailureCode(t, db, id))
		}
	}
	versions := occurrenceVersions(t, db, occurrence)
	if len(versions) != 3 || versions[2].version != "V3" || versions[2].revisionKind != tfdomain.OccurrenceSuperseded.String() ||
		versions[2].agreement != trunkAgreementObject+"/v2" {
		t.Fatalf("再采用之后各版 = %+v, want 从失效版本长出 V3、按 v2", versions)
	}
}

// Covers: AT-TF-309 的换版那一格（S，ADR-0232 决定一、ADR-0231 决定四）——委托在 SIN 交接之前换版到 v2，自动形成按 v2 的快照形成那一项；
// 这一项仍有效时解除采用被挡，答 OCCURRENCES_FORMED、回显它，什么都不写。
func TestAnOccurrenceFormedUnderTheRevisedAgreementBlocksTheWithdrawal(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG57-02"
	beat, db, store := wiredBeat(t)
	publishTrunkVersion(t, db, "v1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)
	publishTrunkVersion(t, db, "v2", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	adoptTrunkCommission(t, db)
	reviseTrunkCommission(t, db, store, "v2", formationAt(0))
	dispatchUntilPublished(t, beat, db, handOverAtSIN(t, register, db, parcel))
	formed := formationOccurrences(t, db)
	if len(formed) != 1 || formed[0].agreement != trunkAgreementObject+"/v2" {
		t.Fatalf("换版之后自动形成 = %+v, want 按 v2", formed)
	}

	blocked := withdrawFormationAdoption(t, db, 1)
	if blocked.Outcome() != tfapplication.AdoptionWithdrawalOccurrencesFormed || len(blocked.FormedOccurrences()) != 1 ||
		blocked.FormedOccurrences()[0].Key.Occurrence.String() != formed[0].occurrence {
		t.Fatalf("按后继形成的那一项仍有效时解除 = %s %d 项", blocked.Outcome(), len(blocked.FormedOccurrences()))
	}
}
