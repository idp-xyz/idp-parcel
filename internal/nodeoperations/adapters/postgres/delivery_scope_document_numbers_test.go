package postgres_test

import (
	"context"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证送站预报册与送站交接单册上的单据号（迁移 0026；ADR-0355 决定三、七、八；票 document-numbering/08）：号随单落、读回原样；
// 没有单据号的按原因读回；单据号这一格没定的新单据册拒收；（租户，单据号）唯一兜住绕过取号编排的写入；判撞的那一问按租户分。
// 夹具全为合成（S 级）。

func numbered(t *testing.T, value string) domain.DocumentNumbering {
	t.Helper()
	return domain.NumberedDocument(must(t, domain.NewDocumentNumber, value))
}

// Covers: AT-NO-135、AT-NO-136 的落库半边与 AT-NO-137 的读口半边——单据号随预报、交接单同一行落下并原样读回（按标识读、锁住重放
// 身份读、列表行）；单据编号时区未配置记那一个原因，迁移之前的旧行（号码与原因都空）读作上线前开立；单据号没定的新单据不落；同租户同号第二
// 份不落；判撞与按单据号检索都只看本租户。
func TestDeliveryScopeRegistersKeepTheDocumentNumber(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	preAdvices, err := adapter.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := adapter.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	transactor := db.Transactor()
	ctx := context.Background()
	tenant := must(t, domain.NewTenantID, registerTenant)

	first := registerPreAdvice(t, "SYN-PREADV-DN-01", "SYN-ACCOUNT-01", "SYN-REQ-DN-01", "sha256:dn01", "SYN-CPN-A1").
		WithDocumentNumbering(numbered(t, "PA-20261102-00001"))
	withheld := registerPreAdvice(t, "SYN-PREADV-DN-02", "SYN-ACCOUNT-01", "SYN-REQ-DN-02", "sha256:dn02", "SYN-CPN-A2")
	if err := inTransaction(transactor, func(txCtx context.Context) error {
		if _, _, err := preAdvices.RecordDeliveryPreAdvice(txCtx, first); err != nil {
			return err
		}
		_, _, err := preAdvices.RecordDeliveryPreAdvice(txCtx, withheld)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.delivery_pre_advice
			(tenant_id, pre_advice_id, customer_account_id, node_ref, expected_arrival_at, parcel_numbers,
			 source_channel, source_request_key, content_digest, formed_at)
		 VALUES ($1, 'SYN-PREADV-DN-OLD', 'SYN-ACCOUNT-01', 'SYN-NODE-SHA-HUB', now(), ARRAY['SYN-CPN-OLD'], 'SYN-CH', 'SYN-REQ-DN-OLD', 'sha256:old', now())`,
		registerTenant,
	); err != nil {
		t.Fatalf("植入迁移之前的旧行：%v", err)
	}

	state, found, err := preAdvices.FindDeliveryPreAdvice(ctx, tenant, first.ID())
	if number, numberedToo := state.PreAdvice.DocumentNumbering().Number(); err != nil || !found || !numberedToo || number.String() != "PA-20261102-00001" {
		t.Fatalf("按标识读回单据号 = %q %v %v %v", number, numberedToo, found, err)
	}
	var locked domain.DeliveryPreAdvice
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		locked, found, err = preAdvices.LockDeliveryPreAdviceSource(txCtx, tenant, first.Source())
		return err
	}); err != nil || !found {
		t.Fatalf("锁住来源读回：%v %v", found, err)
	}
	if number, _ := locked.DocumentNumbering().Number(); number.String() != "PA-20261102-00001" {
		t.Fatalf("锁住来源读回的单据号 = %q", number)
	}
	for id, want := range map[string]domain.DocumentNumberAbsence{
		"SYN-PREADV-DN-02":  domain.NumberingTimeZoneNotConfigured,
		"SYN-PREADV-DN-OLD": domain.IssuedBeforeNumbering,
	} {
		state, _, err := preAdvices.FindDeliveryPreAdvice(ctx, tenant, must(t, domain.NewDeliveryPreAdviceID, id))
		if absence, absent := state.PreAdvice.DocumentNumbering().Absence(); err != nil || !absent || absence != want {
			t.Fatalf("%s 的无号原因 = %v %v %v，want %v", id, absence, absent, err, want)
		}
	}

	for name, preAdvice := range map[string]domain.DeliveryPreAdvice{
		"单据号没定": registerPreAdvice(t, "SYN-PREADV-DN-03", "SYN-ACCOUNT-01", "SYN-REQ-DN-03", "sha256:dn03", "SYN-CPN-A3"),
		"同租户同号": registerPreAdvice(t, "SYN-PREADV-DN-04", "SYN-ACCOUNT-01", "SYN-REQ-DN-04", "sha256:dn04", "SYN-CPN-A4").
			WithDocumentNumbering(numbered(t, "PA-20261102-00001")),
	} {
		if name == "单据号没定" {
			preAdvice = preAdvice.WithDocumentNumbering(domain.DocumentNumbering{})
		}
		if err := inTransaction(transactor, func(txCtx context.Context) error {
			_, _, err := preAdvices.RecordDeliveryPreAdvice(txCtx, preAdvice)
			return err
		}); err == nil {
			t.Fatalf("%s 也落了", name)
		}
	}

	var used, usedElsewhere, otherNumber bool
	otherTenant := must(t, domain.NewTenantID, "SYN-TENANT-OTHER")
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		if used, err = preAdvices.DeliveryPreAdviceDocumentNumberUsed(txCtx, tenant, "PA-20261102-00001"); err != nil {
			return err
		}
		if usedElsewhere, err = preAdvices.DeliveryPreAdviceDocumentNumberUsed(txCtx, otherTenant, "PA-20261102-00001"); err != nil {
			return err
		}
		otherNumber, err = preAdvices.DeliveryPreAdviceDocumentNumberUsed(txCtx, tenant, "PA-20261102-00002")
		return err
	}); err != nil || !used || usedElsewhere || otherNumber {
		t.Fatalf("判撞 = 本租户 %v、别的租户 %v、别的号 %v，%v", used, usedElsewhere, otherNumber, err)
	}

	note := openNote(t, "SYN-HANDOVER-DN-01", "SYN-ACCOUNT-01").WithDocumentNumbering(numbered(t, "HO-20261102-00001"))
	if err := inTransaction(transactor, func(txCtx context.Context) error {
		_, _, err := notes.OpenDeliveryHandoverNote(txCtx, note)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	noteState, found, err := notes.FindDeliveryHandoverNote(ctx, tenant, note.ID())
	if number, _ := noteState.Note.DocumentNumbering().Number(); err != nil || !found || number.String() != "HO-20261102-00001" {
		t.Fatalf("交接单读回单据号 = %q %v %v", number, found, err)
	}
	var lockedNote domain.DeliveryHandoverNote
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		lockedNote, found, err = notes.LockDeliveryHandoverNoteOpening(txCtx, tenant, note.ID())
		if err != nil {
			return err
		}
		used, err = notes.DeliveryHandoverNoteDocumentNumberUsed(txCtx, tenant, "HO-20261102-00001")
		return err
	}); err != nil || !found || !used {
		t.Fatalf("锁住开立读回：%v，判撞 %v，%v", found, used, err)
	}
	if number, _ := lockedNote.DocumentNumbering().Number(); number.String() != "HO-20261102-00001" {
		t.Fatalf("锁住开立读回的单据号 = %q", number)
	}
	clash := openNote(t, "SYN-HANDOVER-DN-02", "SYN-ACCOUNT-01").WithDocumentNumbering(numbered(t, "HO-20261102-00001"))
	if err := inTransaction(transactor, func(txCtx context.Context) error {
		_, _, err := notes.OpenDeliveryHandoverNote(txCtx, clash)
		return err
	}); err == nil {
		t.Fatal("同租户同号的交接单也落了")
	}
	catalogue, err := adapter.NewDeliveryScopeCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	byNumber, err := catalogue.ListDeliveryPreAdvices(ctx, tenant, ports.DeliveryScopeCatalogueFilter{DocumentNumber: must(t, domain.NewDocumentNumber, "PA-20261102-00001")}, 10)
	if err != nil || len(byNumber) != 1 || byNumber[0].PreAdvice.ID() != first.ID() {
		t.Fatalf("按单据号列预报 = %d 份, %v", len(byNumber), err)
	}
	if none, err := catalogue.ListDeliveryPreAdvices(ctx, otherTenant, ports.DeliveryScopeCatalogueFilter{DocumentNumber: must(t, domain.NewDocumentNumber, "PA-20261102-00001")}, 10); err != nil || len(none) != 0 {
		t.Fatalf("别的租户按同一单据号列出 %d 份, %v", len(none), err)
	}
	noteRows, err := catalogue.ListDeliveryHandoverNotes(ctx, tenant, ports.DeliveryScopeCatalogueFilter{DocumentNumber: must(t, domain.NewDocumentNumber, "HO-20261102-00001")}, 10)
	if err != nil || len(noteRows) != 1 || noteRows[0].State.Note.ID() != note.ID() {
		t.Fatalf("按单据号列交接单 = %d 张, %v", len(noteRows), err)
	}
	if number, _ := noteRows[0].State.Note.DocumentNumbering().Number(); number.String() != "HO-20261102-00001" {
		t.Fatalf("交接单列表行的单据号 = %q", number)
	}

	undecided := openNote(t, "SYN-HANDOVER-DN-03", "SYN-ACCOUNT-01").WithDocumentNumbering(domain.DocumentNumbering{})
	if err := inTransaction(transactor, func(txCtx context.Context) error {
		_, _, err := notes.OpenDeliveryHandoverNote(txCtx, undecided)
		return err
	}); err == nil {
		t.Fatal("单据号没定的交接单也落了")
	}
}
