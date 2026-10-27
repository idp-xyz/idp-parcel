package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证票 demo-chain-gaps/111 的演示判据（ADR-0221）：照演示动线登各次交接，每次各起一个范围、版本都写
// `v1`，深圳口岸交进与交出同用范围 SZX（版本 v1、v2）。干线段在 SIN 交接之后实际履约发生项自动形成、BUY 请求发出，五封交接
// 信封都定稿 PUBLISHED。夹具全部合成，只记 `S`。

const (
	reuseSelfSegment     = "SYN-SEG-SHA-SZX-DCG111"
	reuseDeliverySegment = "SYN-SEG-SIN-LM-DCG111"
)

// registerReusedVersionHandover 用生产交接登记编排登一版`已交接`，范围与版本由调用方给；segment 非空时对象随之进那一段。
func registerReusedVersionHandover(
	t *testing.T,
	register *tfapplication.RegisterTransportHandoverHandler,
	db *bentopg.DB,
	parcel, scope, version, releasedBy, receivedBy string,
	at time.Time,
	segment string,
	action tfdomain.SegmentServiceAction,
) string {
	t.Helper()
	command := tfapplication.RegisterTransportHandoverCommand{
		TenantID:          mustTF(t, tfdomain.NewTenantID, formationTenant),
		Object:            parcel,
		Scope:             scope,
		ReleasedBy:        releasedBy,
		ReceivedBy:        receivedBy,
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: "SYN-EVIDENCE-RELEASE-" + scope,
		ReceivingEvidence: "SYN-EVIDENCE-RECEIVE-" + scope,
		Rule:              "SYN-HANDOVER-RULE/v1",
		Version:           version,
		JudgedAt:          at,
		Segment:           segment,
	}
	if segment != "" {
		command.SegmentServiceAction = action.String()
	}
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = register.Register(txCtx, command)
		return err
	})
	if result.Outcome() != tfapplication.HandoverRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("登记交接 %s/%s：outcome=%s debt=%q", scope, version, result.Outcome(), result.SegmentContinuationReference())
	}
	return handoverEventID(formationTenant, parcel, scope, version)
}

// Covers: AT-TF-267（S，票 demo-chain-gaps/111 演示判据的真库半边，ADR-0221）——同一包裹五次`已交接`：SHA/v1 立自营段、SZX/v1
// 交进深圳口岸结束自营段上的参与、SZX/v2 交给干线承运商立干线段（随后采用干线委托）、SIN/v1 交进新加坡枢纽结束干线段上的
// 参与、SINLM/v1 交给自营尾程立派送段。派发进程照生产路由表逐拍跑：五封交接信封都 PUBLISHED，自动形成那一路各记一行 inbox；
// 干线段恰形成一项实际履约发生项、恰发起一份 BUY 请求；段登记册上两段的终点依据各指回自己那一次交接。
func TestHandoversThatAllWriteV1FormTheTrunkOccurrenceAndFireBuy(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG111-01"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, formationTenant)
	registerBuyTriggers(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)

	events := []string{
		registerReusedVersionHandover(t, register, db, parcel, "SHA", "v1", "SYN-NODE-SHA-HUB", "SYN-PARTY-SELF-LINEHAUL",
			formationAt(10*time.Minute), reuseSelfSegment, tfdomain.SegmentServesLinehaul),
		registerReusedVersionHandover(t, register, db, parcel, "SZX", "v1", "SYN-PARTY-SELF-LINEHAUL", "SYN-NODE-SZX-GATE",
			formationAt(30*time.Minute), "", tfdomain.SegmentServiceActionUndeclared),
		registerReusedVersionHandover(t, register, db, parcel, "SZX", "v2", "SYN-NODE-SZX-GATE", "SYN-PARTY-CARRIER-TRUNK-01",
			formationAt(time.Hour), formationSegment, tfdomain.SegmentServesLinehaul),
	}
	adoptTrunkCommission(t, db)
	atSIN := registerReusedVersionHandover(t, register, db, parcel, "SIN", "v1", "SYN-PARTY-CARRIER-TRUNK-01", "SYN-NODE-SIN-HUB",
		formationAt(3*time.Hour), "", tfdomain.SegmentServiceActionUndeclared)
	events = append(events, atSIN,
		registerReusedVersionHandover(t, register, db, parcel, "SINLM", "v1", "SYN-NODE-SIN-HUB", "SYN-PARTY-SELF-LASTMILE",
			formationAt(3*time.Hour+30*time.Minute), reuseDeliverySegment, tfdomain.SegmentServesFinalDelivery))

	// 五封同一分区（租户/对象），一拍认领一个分区的队头；发生项信封在另一分区，与它们并行。
	for beats := 0; beats < 10 && !allPublished(t, db, events); beats++ {
		if _, err := beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
	}
	for _, eventID := range events {
		if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
			t.Fatalf("%s 没定稿：status = %s，失败码 = %q", eventID, status, recordedFailureCode(t, db, eventID))
		}
		if n := inboxRowsOf(t, db, actualOnHandoverConsumerName, eventID); n != 1 {
			t.Fatalf("%s 在自动形成那一路的 inbox 行 = %d, want 1", eventID, n)
		}
	}

	occurrences := formationOccurrences(t, db)
	if len(occurrences) != 1 || occurrences[0].journey != "JRN/"+parcel ||
		occurrences[0].registeredBy != tfapplication.ActualFulfillmentFormationRegistrant {
		t.Fatalf("发生项 = %+v, want 干线段恰一项、自动形成", occurrences)
	}
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("发生项信封那一拍：%v", err)
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	var requested string
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*), max(occurrence_id) FROM settlement_accounting.evaluation_request WHERE tenant_id = $1`,
		formationTenant).Scan(&requests, &requested); err != nil || requests != 1 || requested != occurrences[0].occurrence {
		t.Fatalf("BUY 请求 %d 份、指向 %q（%v）, want 恰一份、指向干线那一项 %s", requests, requested, err, occurrences[0].occurrence)
	}

	ends := map[string]string{}
	rows, err := querier.Query(t.Context(),
		`SELECT segment_ref, end_basis FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = $1 AND object_ref = $2 AND end_basis IS NOT NULL`, formationTenant, parcel)
	if err != nil {
		t.Fatalf("读参与终点：%v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var segment, basis string
		if err := rows.Scan(&segment, &basis); err != nil {
			t.Fatal(err)
		}
		ends[segment] = basis
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if ends[reuseSelfSegment] != "TRANSPORT-HANDOVER-KEY/20:"+parcel+"/3:SZX/2:v1" ||
		ends[formationSegment] != "TRANSPORT-HANDOVER-KEY/20:"+parcel+"/3:SIN/2:v1" || len(ends) != 2 {
		t.Fatalf("参与终点 = %v, want 自营段指回 SZX/v1、干线段指回 SIN/v1", ends)
	}
}

func allPublished(t *testing.T, db *bentopg.DB, events []string) bool {
	t.Helper()
	for _, eventID := range events {
		if outboxStatus(t, db, eventID) != "PUBLISHED" {
			return false
		}
	}
	return true
}
