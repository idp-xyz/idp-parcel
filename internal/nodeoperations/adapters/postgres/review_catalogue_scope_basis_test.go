package postgres_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 票 ps-external-mark-relations/13「做什么」的「复核目录」的存储面（ADR-0212 决定五、七）——待识别册每行带这一版记下的交付范围核对态、
// 来源里的交付范围依据引用，与这件实物此刻最后归入的交接单：来源没带依据的读作没有；归入只看归入、不看收寄带来的成员，按业务时间取
// 最后一次；别的租户同名实物的归入不串进来。
func TestUnidentifiedItemsCarryTheScopeBasisAndTheLastAssignedNote(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := adapter.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	transactor := db.Transactor()
	ctx := t.Context()
	const tenant, other, node = "SYN-TENANT-PSXM13-RC", "SYN-TENANT-PSXM13-RC-OTHER", "SYN-NODE-PSXM13-RC"

	sourced := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM13-SOURCED", "SYN-INTAKEV-PSXM13-SOURCED", "SYN-UNIT-PSXM13-SOURCED", node)
	sourced.Source.ScopeBasis = scopeBasisRef(t, domain.BasisDeliveryHandoverNote, "SYN-HN-PSXM13-SOURCE")
	sourced.IdentityCheck = ports.IdentityChecked
	sourced.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeUnverified, domain.ScopeBindingsNotAllConfirmed, domain.ScopeInapplicabilityAbsent)

	assigned := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM13-ASSIGNED", "SYN-INTAKEV-PSXM13-ASSIGNED", "SYN-UNIT-PSXM13-ASSIGNED", node)
	assigned.IdentityCheck = ports.IdentityChecked
	assigned.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeUnverified, domain.ScopeNoBasis, domain.ScopeInapplicabilityAbsent)

	advised := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM13-ADVISED", "SYN-INTAKEV-PSXM13-ADVISED", "SYN-UNIT-PSXM13-ADVISED", node)
	advised.Source.ScopeBasis = scopeBasisRef(t, domain.BasisDeliveryPreAdvice, "SYN-PREADV-PSXM13-RC")
	advised.IdentityCheck = ports.IdentityChecked
	advised.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeUnconfigured, domain.ScopeUnverifiedReasonAbsent, domain.ScopeInapplicabilityAbsent)

	unchecked := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM13-UNCHECKED", "SYN-INTAKEV-PSXM13-UNCHECKED", "SYN-UNIT-PSXM13-UNCHECKED", node)
	unchecked.IdentityCheck = ports.IdentityUnchecked
	saveReceptions(t, transactor, ctx, receptions, sourced, assigned, advised, unchecked)

	at := func(minutes int) time.Time { return receivedAt.Add(time.Duration(minutes) * time.Minute) }
	if err := inTransaction(transactor, func(txCtx context.Context) error {
		for _, note := range []domain.DeliveryHandoverNote{
			handoverNoteAt(t, tenant, "SYN-HN-PSXM13-EARLIER", node, "SYN-ACCOUNT-PSXM13-1"),
			handoverNoteAt(t, tenant, "SYN-HN-PSXM13-LATER", node, "SYN-ACCOUNT-PSXM13-1"),
			handoverNoteAt(t, tenant, "SYN-HN-PSXM13-RECEIVED", node, "SYN-ACCOUNT-PSXM13-1"),
			handoverNoteAt(t, other, "SYN-HN-PSXM13-ELSEWHERE", node, "SYN-ACCOUNT-PSXM13-9"),
		} {
			if _, _, err := notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
				return err
			}
		}
		for _, member := range []domain.DeliveryHandoverMember{
			catalogueMember(t, tenant, "SYN-HN-PSXM13-LATER", "SYN-UNIT-PSXM13-ASSIGNED", domain.MembershipByAssignment, "SYN-ASSIGN-PSXM13-LATER", at(30)),
			catalogueMember(t, tenant, "SYN-HN-PSXM13-EARLIER", "SYN-UNIT-PSXM13-ASSIGNED", domain.MembershipByAssignment, "SYN-ASSIGN-PSXM13-EARLIER", at(20)),
			catalogueMember(t, tenant, "SYN-HN-PSXM13-RECEIVED", "SYN-UNIT-PSXM13-ASSIGNED", domain.MembershipByReception, "SYN-SCAN-PSXM13-RECEIVED", at(40)),
			catalogueMember(t, other, "SYN-HN-PSXM13-ELSEWHERE", "SYN-UNIT-PSXM13-SOURCED", domain.MembershipByAssignment, "SYN-ASSIGN-PSXM13-ELSEWHERE", at(50)),
		} {
			if _, _, err := notes.AppendDeliveryHandoverMember(txCtx, member); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("落交接单与成员：%v", err)
	}

	rows, err := catalogue.ListUnidentifiedItems(ctx, ref(t, domain.NewTenantID, tenant), 10)
	if err != nil {
		t.Fatalf("上列待识别册：%v", err)
	}
	type want struct {
		check    domain.DeliveryScopeCheck
		source   domain.DeliveryScopeBasisReference
		assigned string
	}
	wants := map[string]want{
		sourced.Key.SourceID:   {sourced.DeliveryScopeCheck, sourced.Source.ScopeBasis, ""},
		assigned.Key.SourceID:  {assigned.DeliveryScopeCheck, domain.DeliveryScopeBasisReference{}, "SYN-HN-PSXM13-LATER"},
		advised.Key.SourceID:   {advised.DeliveryScopeCheck, advised.Source.ScopeBasis, ""},
		unchecked.Key.SourceID: {domain.DeliveryScopeCheck{}, domain.DeliveryScopeBasisReference{}, ""},
	}
	if len(rows) != len(wants) {
		t.Fatalf("待识别册 %d 行, want %d", len(rows), len(wants))
	}
	for _, row := range rows {
		expected := wants[row.SourceID]
		if row.DeliveryScopeCheck != expected.check {
			t.Errorf("%s 核对态 = %+v, want %+v", row.SourceID, row.DeliveryScopeCheck, expected.check)
		}
		if row.DeliveryScopeBasis.Source != expected.source {
			t.Errorf("%s 来源依据 = %+v, want %+v", row.SourceID, row.DeliveryScopeBasis.Source, expected.source)
		}
		if got := row.DeliveryScopeBasis.LastAssignedNote.String(); got != expected.assigned {
			t.Errorf("%s 最后归入的交接单 = %q, want %q", row.SourceID, got, expected.assigned)
		}
	}
}

func scopeBasisRef(t *testing.T, kind domain.DeliveryScopeBasisKind, id string) domain.DeliveryScopeBasisReference {
	t.Helper()
	basis, err := domain.NewDeliveryScopeBasisReference(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	return basis
}
