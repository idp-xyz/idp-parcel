package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// Covers: AT-SA-200（持久化半边）——操作证据随版本行落库并原样读回（settlement_accounting 0041）；不记提交方的一路落 NULL、读回为空；
// 同一版本再写一次不顶替先到者的操作证据；空白串由库上那道 CHECK 拒。
func TestAFundsFactVersionKeepsItsOperationEvidence(t *testing.T) {
	facts, _, _, transactor, pool := newFundsStores(t)
	ctx := t.Context()

	attributed := adoptedFactRecord(t, "tenant-a", "bank-fact-evidence-1", domain.FundsReceiptConfirmed, 8000)
	attributed.OperationEvidence = "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01"
	unattributed := adoptedFactRecord(t, "tenant-a", "bank-fact-evidence-2", domain.FundsReceiptConfirmed, 9000)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.FundsFactRecord{attributed, unattributed} {
			if outcome, err := facts.Save(txCtx, record); err != nil || outcome != ports.FundsFactSaved {
				t.Errorf("save %s：%d %v", record.Key.Fact, outcome, err)
			}
		}
		return nil
	})

	found, exists, err := facts.FindVersion(ctx, attributed.Key, attributed.Fact.Version())
	if err != nil || !exists || found.OperationEvidence != attributed.OperationEvidence {
		t.Fatalf("操作证据没有原样读回：%q %v %v", found.OperationEvidence, exists, err)
	}
	if plain, exists, err := facts.FindByKey(ctx, unattributed.Key); err != nil || !exists || plain.OperationEvidence != "" {
		t.Fatalf("不记提交方的一版读回了 %q（%v %v）", plain.OperationEvidence, exists, err)
	}
	var nulls int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM settlement_accounting.external_funds_fact_version
		 WHERE fact_id = 'bank-fact-evidence-2' AND operation_evidence IS NULL`).Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("不记提交方的一版没落成 NULL：%d %v", nulls, err)
	}

	again := attributed
	again.OperationEvidence = "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-02"
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if outcome, err := facts.Save(txCtx, again); err != nil || outcome != ports.FundsFactAlreadyAdopted {
			t.Errorf("同一版本再写：%d %v", outcome, err)
		}
		return nil
	})
	if kept, _, _ := facts.FindVersion(ctx, attributed.Key, attributed.Fact.Version()); kept.OperationEvidence != attributed.OperationEvidence {
		t.Fatalf("同一版本再写顶替了操作证据：%q", kept.OperationEvidence)
	}

	if _, err := pool.Exec(ctx, `UPDATE settlement_accounting.external_funds_fact_version
		 SET operation_evidence = '  ' WHERE fact_id = 'bank-fact-evidence-2'`); err == nil {
		t.Fatal("空白的操作证据溜进了版本行")
	}
}
