package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: `AT-TF-370` 库面半边（ADR-0303 的待遇，TF 0056）——承接决定与替代/退运旅程各在记录上多一列操作证据：给了就原样读回，缺席读回缺席
// （不落空串），库面不收空白的操作者。
func TestTheAcceptanceAndTheJourneyRecordTheirOperatorWhenOneIsGiven(t *testing.T) {
	ctx := t.Context()
	operator := deliveryValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-43")

	acceptances, transactor, pool := newDispositionAcceptances(t)
	withOperator := acceptedDisposition(t, "tenant-a", "item-with-operator")
	withOperator.DecidedBy = operator
	without := acceptedDisposition(t, "tenant-a", "item-without-operator")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		for _, saved := range []ports.DispositionAcceptanceRecord{withOperator, without} {
			if _, err := acceptances.Save(txCtx, saved); err != nil {
				return err
			}
		}
		return nil
	})
	for item, want := range map[string]string{"item-with-operator": operator.String(), "item-without-operator": ""} {
		key := withOperator.Key
		key.Item = deliveryValue(t, domain.NewCollaborationItemReference, item)
		found, exists, err := acceptances.FindByKey(ctx, key)
		if err != nil || !exists || found.DecidedBy.String() != want || found.DecidedBy.Present() != (want != "") {
			t.Fatalf("承接 %s 读回操作者 = %q（在场 %v，存在 %v，err %v），want %q", item, found.DecidedBy.String(), found.DecidedBy.Present(), exists, err, want)
		}
	}
	var nulls int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transport_fulfillment.disposition_acceptance WHERE decided_by IS NULL`).Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("缺席的操作者落成 NULL 的行数 = %d（%v），want 1", nulls, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE transport_fulfillment.disposition_acceptance SET decided_by = '  '`); err == nil {
		t.Fatal("库面收下了空白的承接决定人")
	}

	journeys, _, _, journeyTransactor, journeyPool := newJourneyScheduleCapacityStores(t)
	started := formedJourney(t, "tenant-a", domain.ReturnJourneyPurpose, domain.RegulatoryDispositionDecision, "journey-with-operator")
	started.StartedBy = operator
	mustWithinDeliveryTransaction(t, journeyTransactor, ctx, func(txCtx context.Context) error {
		_, err := journeys.Save(txCtx, started)
		return err
	})
	found, exists, err := journeys.FindByKey(ctx, started.Key)
	if err != nil || !exists || found.StartedBy.String() != operator.String() {
		t.Fatalf("旅程读回启动人 = %q（存在 %v，err %v），want %q", found.StartedBy.String(), exists, err, operator.String())
	}
	if _, err := journeyPool.Exec(ctx, `UPDATE transport_fulfillment.alternate_journey SET started_by = ''`); err == nil {
		t.Fatal("库面收下了空白的旅程启动人")
	}
}
