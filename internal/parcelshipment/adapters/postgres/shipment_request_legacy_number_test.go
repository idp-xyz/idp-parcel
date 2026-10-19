package postgres_test

import (
	"strings"
	"testing"
)

// Covers: 票 ps-external-mark-relations/06 评审阻断二——「存得下」那道门立起来之前，提交口只拒全空白，超过 512 字节的客户包裹
// 件号已经能进委托快照。这样的快照经仓储照样读得回，号码逐字带回；读回之后照常接受，号码随接受进册。库里那一份是直写的，
// 模拟门落地之前留下的行，不经今天的提交路。夹具全为合成（S 级）。
func TestASnapshotStoredBeforeTheNumberGateStillReadsBack(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	request := numberedRequest(t, "tenant-1", "customer-1", "req-key-legacy", "request-legacy",
		[]string{"parcel-1"}, map[string]string{"parcel-1": "SYN-LEGACY-PLACEHOLDER"})
	mustInsert(t, fixture.transactor, ctx, fixture.requests, request)

	legacy := strings.Repeat("SYN-LEGACY-0600-", 37) + "SYN-LEGA"
	if len(legacy) != 600 {
		t.Fatalf("夹具号码 %d 字节, want 600", len(legacy))
	}
	tag, err := fixture.pool.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request
		    SET snapshot = jsonb_set(snapshot, '{customerParcelNumbers,0,number}', to_jsonb($1::text))
		  WHERE tenant_id = 'tenant-1' AND snapshot -> 'customerParcelNumbers' -> 0 ->> 'number' = 'SYN-LEGACY-PLACEHOLDER'`,
		legacy)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("直写旧快照：rows = %d err = %v", tag.RowsAffected(), err)
	}

	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	loaded, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回带 600 字节客户包裹件号的委托：found = %v err = %v", found, err)
	}
	numbers := loaded.CustomerParcelNumbers()
	if len(numbers) != 1 || numbers[0].Number().String() != legacy || numbers[0].Parcel().String() != "parcel-1" {
		t.Fatalf("读回的配对 = %+v, want parcel-1 配那 600 字节、逐字", numbers)
	}

	mustSave(t, fixture.transactor, ctx, fixture.requests, identity, accept(t, loaded))
	if rows := fixture.relationRows(t); rows != 1 {
		t.Fatalf("接受之后关联册 %d 行, want 1——旧号随接受进册", rows)
	}
}
