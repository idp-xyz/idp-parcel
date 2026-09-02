package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证发生项登记：整条往返（本体 + 对象范围）、更正换版本且
// 原版本不删、撞键译`已登记`、作用域隔离、无事务拒，以及库内 CHECK 挡住领域造不出的行
// ——含**本册没有金额列**那一条的守卫。夹具全部为合成登记（S 级）。

var occurredAtFixture = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

// TestAChargeOccurrenceRoundTripsWithItsMembers 证本体与对象范围一并往返。
func TestAChargeOccurrenceRoundTripsWithItsMembers(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	record := occurrenceRecord(t, "OCC-0001", "v1", "parcel-1", "parcel-2")
	mustSaveOccurrence(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, occurrenceKey(t, "tenant-1", "OCC-0001", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回发生项：%v exists=%v", err, exists)
	}
	if found.Occurrence.Reason() != domain.FailedAttemptOccurrence ||
		found.Occurrence.Provider().String() != "partner-1" ||
		found.Occurrence.Agreement().String() != "agreement-1/v1" ||
		!found.Occurrence.OccurredAt().Equal(occurredAtFixture) {
		t.Fatalf("本体往返变形：%+v", found.Occurrence)
	}
	if quantity, unit := found.Occurrence.Quantity(); quantity != 1 || unit.String() != "attempt" {
		t.Fatalf("数量与单位没成对带回：%d %q", quantity, unit)
	}
	if members := found.Occurrence.Members(); len(members) != 2 ||
		members[0].String() != "parcel-1" || members[1].String() != "parcel-2" {
		t.Fatalf("对象范围没有原样带回：%v", members)
	}
	if _, _, _, revised := found.Occurrence.Revision(); revised {
		t.Fatal("首版读回长出了修订")
	}
}

// TestARevisionLandsAsANewVersionAndTheOriginalStays 证 CONTEXT「保留原发生项……不删除
// 原成本」：更正是新版本新行，原行仍取得回来。
func TestARevisionLandsAsANewVersionAndTheOriginalStays(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	first := occurrenceRecord(t, "OCC-0002", "v1", "parcel-1")
	mustSaveOccurrence(t, transactor, ctx, repository, first)

	revised, err := first.Occurrence.ReviseValidity(
		domain.OccurrenceSuperseded,
		occurrenceRef(t, domain.NewOccurrenceValidityVersion, "v2"),
		occurrenceRef(t, domain.NewOccurrenceBasisReference, "CORRECTION/src-1"),
		occurredAtFixture.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("形成修订：%v", err)
	}
	second := ports.ChargeOccurrenceRecord{
		Key:        occurrenceKey(t, "tenant-1", "OCC-0002", "v2"),
		Occurrence: revised,
		RecordedAt: occurredAtFixture,
	}
	mustSaveOccurrence(t, transactor, ctx, repository, second)

	original, exists, err := repository.FindByKey(ctx, occurrenceKey(t, "tenant-1", "OCC-0002", "v1"))
	if err != nil || !exists {
		t.Fatalf("原版本不见了：%v exists=%v", err, exists)
	}
	if _, _, _, wasRevised := original.Occurrence.Revision(); wasRevised {
		t.Fatal("原版本被改写成了修订版——更正应当只加新行")
	}

	current, exists, err := repository.FindByKey(ctx, occurrenceKey(t, "tenant-1", "OCC-0002", "v2"))
	if err != nil || !exists {
		t.Fatalf("修订版取不回：%v exists=%v", err, exists)
	}
	corrects, has := current.Occurrence.Corrects()
	if !has || corrects.String() != "v1" {
		t.Fatalf("修订版没有回指前身：has=%v", has)
	}
	kind, basis, _, revisedFlag := current.Occurrence.Revision()
	if !revisedFlag || kind != domain.OccurrenceSuperseded || basis.String() != "CORRECTION/src-1" {
		t.Fatalf("修订三件没带回：%v %q %q", revisedFlag, kind, basis)
	}
}

// TestSavingTheSameOccurrenceVersionTwiceIsAlreadyRegistered 证撞键是业务答案（ADR-0031）。
func TestSavingTheSameOccurrenceVersionTwiceIsAlreadyRegistered(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	record := occurrenceRecord(t, "OCC-0003", "v1", "parcel-1")
	mustSaveOccurrence(t, transactor, ctx, repository, record)

	var outcome ports.ChargeOccurrenceSaveOutcome
	mustWithinOccurrenceTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	if outcome != ports.ChargeOccurrenceAlreadyRegistered {
		t.Fatalf("重放 outcome = %d, want ChargeOccurrenceAlreadyRegistered", outcome)
	}
}

// TestChargeOccurrencesAreInvisibleAcrossTenants 证租户隔离。
func TestChargeOccurrencesAreInvisibleAcrossTenants(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	mustSaveOccurrence(t, transactor, ctx, repository, occurrenceRecord(t, "OCC-0004", "v1", "parcel-1"))

	if _, exists, err := repository.FindByKey(ctx, occurrenceKey(t, "tenant-other", "OCC-0004", "v1")); err != nil || exists {
		t.Fatalf("他租看见了这条发生项：exists=%v err=%v", exists, err)
	}
}

// TestChargeOccurrenceWritesRefuseToRunOutsideATransaction 证无环境事务即拒——本体与
// 成员必须同笔落。断言指名 ErrTransactionRequired 而不是 err != nil。
func TestChargeOccurrenceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newChargeOccurrences(t)

	_, err := repository.Save(t.Context(), occurrenceRecord(t, "OCC-0005", "v1", "parcel-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestOccurrenceCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门。
func TestOccurrenceCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newChargeOccurrences(t)
	ctx := t.Context()
	mustSaveOccurrence(t, transactor, ctx, repository, occurrenceRecord(t, "OCC-0006", "v1", "parcel-1"))

	base := `INSERT INTO transport_fulfillment.transport_charge_occurrence
	    (tenant_id, occurrence_ref, validity_version, journey_ref, legal_entity_ref,
	     provider_ref, agreement_ref, reason, fact_basis, scope_ref, quantity, unit_ref,
	     occurred_at, corrects_version, revision_kind, revision_basis, revised_at, recorded_at)
	 VALUES `
	for name, values := range map[string]string{
		"集外发生原因": `('tenant-1','BAD-1','v1','j','le','pr','ag','SCANNED','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now())`,
		"数量非正":   `('tenant-1','BAD-2','v1','j','le','pr','ag','BOOKING','fb','sc',0,'u',now(),NULL,NULL,NULL,NULL,now())`,
		"缺协议快照":  `('tenant-1','BAD-3','v1','j','le','pr','','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now())`,
		"半截的修订":  `('tenant-1','BAD-4','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v1',NULL,NULL,NULL,now())`,
		"集外修订走向": `('tenant-1','BAD-5','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v1','GAVE_UP','rb',now(),now())`,
		"自指的修订":  `('tenant-1','BAD-6','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v2','SUPERSEDED','rb',now(),now())`,
		"修订早于发生": `('tenant-1','BAD-7','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v1','SUPERSEDED','rb',now() - interval '1 day',now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatalf("%s 落进去了", name)
			}
		})
	}

	// 本册没有金额列，这一条守的是它不被人「顺手」加回来：列不存在，写它即报错。
	t.Run("没有金额列", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`UPDATE transport_fulfillment.transport_charge_occurrence
			    SET amount_minor = 1 WHERE tenant_id = 'tenant-1'`); err == nil {
			t.Fatal("发生项表上出现了金额列——金额归 settlement-accounting")
		}
	})
}

// —— 夹具 ——

func newChargeOccurrences(t *testing.T) (*adapter.ChargeOccurrences, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewChargeOccurrences(db)
	if err != nil {
		t.Fatalf("构造发生项登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinOccurrenceTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSaveOccurrence(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ChargeOccurrences,
	record ports.ChargeOccurrenceRecord,
) {
	t.Helper()
	var outcome ports.ChargeOccurrenceSaveOutcome
	mustWithinOccurrenceTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	if outcome != ports.ChargeOccurrenceSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
}

func occurrenceRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func occurrenceKey(t *testing.T, tenant, occurrence, validity string) ports.ChargeOccurrenceKey {
	t.Helper()
	return ports.ChargeOccurrenceKey{
		TenantID:   occurrenceRef(t, domain.NewTenantID, tenant),
		Occurrence: occurrenceRef(t, domain.NewChargeOccurrenceReference, occurrence),
		Validity:   occurrenceRef(t, domain.NewOccurrenceValidityVersion, validity),
	}
}

func occurrenceRecord(t *testing.T, occurrence, validity string, objects ...string) ports.ChargeOccurrenceRecord {
	t.Helper()
	members := make([]domain.CarriedObjectReference, 0, len(objects))
	for _, object := range objects {
		members = append(members, occurrenceRef(t, domain.NewCarriedObjectReference, object))
	}
	key := occurrenceKey(t, "tenant-1", occurrence, validity)
	built, err := domain.FormTransportChargeOccurrence(domain.TransportChargeOccurrenceSpec{
		TenantID:    key.TenantID,
		Occurrence:  key.Occurrence,
		Journey:     occurrenceRef(t, domain.NewJourneyReference, "journey-1"),
		LegalEntity: occurrenceRef(t, domain.NewProcurementLegalEntityReference, "legal-1"),
		Provider:    occurrenceRef(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:   occurrenceRef(t, domain.NewAgreementSnapshotReference, "agreement-1/v1"),
		Reason:      domain.FailedAttemptOccurrence,
		FactBasis:   occurrenceRef(t, domain.NewOccurrenceBasisReference, "ATTEMPT-RESULT/a-1/parcel-1"),
		Scope:       occurrenceRef(t, domain.NewOccurrenceScopeReference, "scope-1"),
		Members:     members,
		Quantity:    1,
		Unit:        occurrenceRef(t, domain.NewQuantityUnitReference, "attempt"),
		OccurredAt:  occurredAtFixture,
		Validity:    key.Validity,
	})
	if err != nil {
		t.Fatalf("构造发生项夹具 %s/%s：%v", occurrence, validity, err)
	}
	return ports.ChargeOccurrenceRecord{Key: key, Occurrence: built, RecordedAt: occurredAtFixture}
}
