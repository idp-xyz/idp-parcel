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
	if !found.Occurrence.SameContentAs(record.Occurrence) {
		t.Fatal("读回的发生项与落册的那一份内容不同（发生条件的依据与原话也要原样带回）")
	}
	// 断言人不进内容比对，单独核它原样带回：审计要问的正是「这一项是谁判的」。
	wantBy, _ := record.Occurrence.Condition().AssertedBy()
	if by, asserted := found.Occurrence.Condition().AssertedBy(); !asserted || by != wantBy {
		t.Fatalf("断言人没有原样带回：%q %v, want %q", by, asserted, wantBy)
	}
	if found.Occurrence.Condition().RegisteredBy() != wantBy {
		t.Fatalf("断言项的登记者 = %q，应就是断言人", found.Occurrence.Condition().RegisteredBy())
	}
}

// Covers: AT-TF-168 的册面（迁移 0034）——按协议声明判的发生项往返：依据是 AGREEMENT_DECLARED、登记者原样带回、
// 没有断言人与原话；与落册的那一份内容相同。
func TestAnAgreementDeclaredOccurrenceRoundTripsWithItsRegistrant(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	record := occurrenceRecordJudged(t, "OCC-DECLARED-1", "scope-1", "SEGMENT", declaredOccurrenceCondition(t), "parcel-1")
	mustSaveOccurrence(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	condition := found.Occurrence.Condition()
	if condition.Basis() != domain.ConditionDeclaredByAgreement ||
		condition.RegisteredBy().String() != "https://dex.syn.example#SYN-OPERATOR-CONFIG-01" {
		t.Fatalf("依据/登记者 = %q/%q", condition.Basis(), condition.RegisteredBy())
	}
	if _, asserted := condition.AssertedBy(); asserted {
		t.Fatal("按协议判的项读出了断言人")
	}
	if !found.Occurrence.SameContentAs(record.Occurrence) {
		t.Fatal("读回的发生项与落册的那一份内容不同")
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
	// 交集一问带事务级锁：事务外的锁在语句结束即放，查完到 Save 之间别的登记照样插得进来。
	_, _, err = repository.FindCovering(t.Context(), ports.ChargeOccurrenceCoverageQuery{
		TenantID:  occurrenceRef(t, domain.NewTenantID, "tenant-1"),
		Reason:    domain.FailedAttemptOccurrence,
		Scope:     occurrenceRef(t, domain.NewOccurrenceScopeReference, "scope-1"),
		FactBasis: occurrenceRef(t, domain.NewOccurrenceBasisReference, "ATTEMPT-RESULT/a-1/parcel-1"),
		Members:   []domain.CarriedObjectReference{occurrenceRef(t, domain.NewCarriedObjectReference, "parcel-1")},
		Except:    occurrenceRef(t, domain.NewChargeOccurrenceReference, "OCC-0005"),
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务的交集一问应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestOccurrenceCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门。
func TestOccurrenceCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newChargeOccurrences(t)
	ctx := t.Context()
	mustSaveOccurrence(t, transactor, ctx, repository, occurrenceRecord(t, "OCC-0006", "v1", "parcel-1"))

	// 每条非法行只坏它要证的那一格，其余各列都合法——尤其发生条件三列与登记者带齐：缺了它们，每一行都会先撞
	// 非空，各条用例就全成了空过。
	base := `INSERT INTO transport_fulfillment.transport_charge_occurrence
	    (tenant_id, occurrence_ref, validity_version, journey_ref, legal_entity_ref,
	     provider_ref, agreement_ref, reason, fact_basis, scope_ref, quantity, unit_ref,
	     occurred_at, corrects_version, revision_kind, revision_basis, revised_at, recorded_at,
	     condition_basis, condition_asserted_by, condition_assertion, registered_by)
	 VALUES `
	const asserted = `,'REGISTRANT_ASSERTED','iss#op','a','iss#op')`
	const declared = `,'AGREEMENT_DECLARED',NULL,NULL,'iss#op')`
	for name, values := range map[string]string{
		"集外发生原因": `('tenant-1','BAD-1','v1','j','le','pr','ag','SCANNED','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now()` + asserted,
		"数量非正":   `('tenant-1','BAD-2','v1','j','le','pr','ag','BOOKING','fb','sc',0,'u',now(),NULL,NULL,NULL,NULL,now()` + asserted,
		"缺协议快照":  `('tenant-1','BAD-3','v1','j','le','pr','','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now()` + asserted,
		"半截的修订":  `('tenant-1','BAD-4','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v1',NULL,NULL,NULL,now()` + asserted,
		"集外修订走向": `('tenant-1','BAD-5','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v1','GAVE_UP','rb',now(),now()` + asserted,
		"自指的修订":  `('tenant-1','BAD-6','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v2','SUPERSEDED','rb',now(),now()` + asserted,
		"修订早于发生": `('tenant-1','BAD-7','v2','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),'v1','SUPERSEDED','rb',now() - interval '1 day',now()` + asserted,
		"集外条件依据": `('tenant-1','BAD-8','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'GUESSED','iss#op','a','iss#op')`,
		"断言缺原话":  `('tenant-1','BAD-9','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'REGISTRANT_ASSERTED','iss#op',' ','iss#op')`,
		"断言原话为空": `('tenant-1','BAD-10','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'REGISTRANT_ASSERTED','iss#op',NULL,'iss#op')`,
		"没有条件依据": `('tenant-1','BAD-11','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),NULL,'iss#op','a','iss#op')`,
		"断言缺断言人": `('tenant-1','BAD-12','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'REGISTRANT_ASSERTED',' ','a','iss#op')`,
		"断言人为空":  `('tenant-1','BAD-13','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'REGISTRANT_ASSERTED',NULL,'a','iss#op')`,
		// 迁移 0034：按协议判的一格不带断言人与原话，登记者必带，断言人就是登记者。
		"协议判带断言人":  `('tenant-1','BAD-14','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'AGREEMENT_DECLARED','iss#op',NULL,'iss#op')`,
		"协议判带原话":   `('tenant-1','BAD-15','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'AGREEMENT_DECLARED',NULL,'a','iss#op')`,
		"没有登记者":    `('tenant-1','BAD-16','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'AGREEMENT_DECLARED',NULL,NULL,NULL)`,
		"登记者为空":    `('tenant-1','BAD-17','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'AGREEMENT_DECLARED',NULL,NULL,' ')`,
		"断言人不是登记者": `('tenant-1','BAD-18','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now(),'REGISTRANT_ASSERTED','iss#op','a','iss#other')`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatalf("%s 落进去了", name)
			}
		})
	}

	// 对照：同一个 base 写一行全合法的能落进去，上面各条拒的确是它们各自那一格。
	t.Run("合法行落得进去", func(t *testing.T) {
		if _, err := pool.Exec(ctx, base+`('tenant-1','GOOD-1','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now()`+asserted); err != nil {
			t.Fatalf("合法断言行被拒：%v", err)
		}
		if _, err := pool.Exec(ctx, base+`('tenant-1','GOOD-2','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now()`+declared); err != nil {
			t.Fatalf("合法的按协议判行被拒：%v", err)
		}
	})

	// 本册没有金额列，这一条守的是它不被人「顺手」加回来：列不存在，写它即报错。
	t.Run("没有金额列", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`UPDATE transport_fulfillment.transport_charge_occurrence
			    SET amount_minor = 1 WHERE tenant_id = 'tenant-1'`); err == nil {
			t.Fatal("发生项表上出现了金额列——金额归 settlement-accounting")
		}
	})
}

// Covers: AT-TF-125 与 ADR-0207 决定四的册面——同租户、同原因、同事实依据上另一项仍有效、成员有交集才算计过：交回
// 那一项的当前版（被替代的旧版不算，失效的那一项不算）；同一身份、无交集、别的依据、别的租户都答没有。无事务即拒
// 由 TestChargeOccurrenceWritesRefuseToRunOutsideATransaction 证。
func TestCoverageFindsAnotherLiveOccurrenceSharingAMemberOnTheSameFact(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	covering := occurrenceRecord(t, "OCC-COVER-1", "v1", "parcel-1", "parcel-2")
	mustSaveOccurrence(t, transactor, ctx, repository, covering)
	invalidated := occurrenceRecord(t, "OCC-COVER-3", "v1", "parcel-3")
	mustSaveOccurrence(t, transactor, ctx, repository, invalidated)
	revise := func(record ports.ChargeOccurrenceRecord, kind domain.OccurrenceRevisionKind) ports.ChargeOccurrenceRecord {
		t.Helper()
		revised, err := record.Occurrence.ReviseValidity(kind,
			occurrenceRef(t, domain.NewOccurrenceValidityVersion, "v2"),
			occurrenceRef(t, domain.NewOccurrenceBasisReference, "CORRECTION/cover-1"),
			occurredAtFixture.Add(time.Hour))
		if err != nil {
			t.Fatalf("形成修订：%v", err)
		}
		next := ports.ChargeOccurrenceRecord{
			Key:        ports.ChargeOccurrenceKey{TenantID: record.Key.TenantID, Occurrence: record.Key.Occurrence, Validity: revised.Validity()},
			Occurrence: revised,
			RecordedAt: occurredAtFixture.Add(time.Hour),
		}
		mustSaveOccurrence(t, transactor, ctx, repository, next)
		return next
	}
	revise(invalidated, domain.OccurrenceInvalidated)

	query := func(tenant, basis, except string, objects ...string) ports.ChargeOccurrenceCoverageQuery {
		members := make([]domain.CarriedObjectReference, 0, len(objects))
		for _, object := range objects {
			members = append(members, occurrenceRef(t, domain.NewCarriedObjectReference, object))
		}
		return ports.ChargeOccurrenceCoverageQuery{
			TenantID:  occurrenceRef(t, domain.NewTenantID, tenant),
			Reason:    domain.FailedAttemptOccurrence,
			Scope:     occurrenceRef(t, domain.NewOccurrenceScopeReference, "scope-1"),
			FactBasis: occurrenceRef(t, domain.NewOccurrenceBasisReference, basis),
			Members:   members,
			Except:    occurrenceRef(t, domain.NewChargeOccurrenceReference, except),
		}
	}
	find := func(q ports.ChargeOccurrenceCoverageQuery) (ports.ChargeOccurrenceRecord, bool) {
		t.Helper()
		var record ports.ChargeOccurrenceRecord
		var found bool
		mustWithinOccurrenceTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			record, found, err = repository.FindCovering(txCtx, q)
			return err
		})
		return record, found
	}
	const basis = "ATTEMPT-RESULT/a-1/parcel-1"

	if record, found := find(query("tenant-1", basis, "OCC-NEW", "parcel-2", "parcel-9")); !found || record.Key != covering.Key {
		t.Fatalf("有交集的另一项没找到：found=%v key=%+v", found, record.Key)
	}
	for name, q := range map[string]ports.ChargeOccurrenceCoverageQuery{
		"同一身份":   query("tenant-1", basis, "OCC-COVER-1", "parcel-1"),
		"无交集":    query("tenant-1", basis, "OCC-NEW", "parcel-9"),
		"别的事实依据": query("tenant-1", "ATTEMPT-RESULT/a-2/parcel-1", "OCC-NEW", "parcel-1"),
		"别的租户":   query("tenant-2", basis, "OCC-NEW", "parcel-1"),
		"失效的那一项": query("tenant-1", basis, "OCC-NEW", "parcel-3"),
	} {
		if _, found := find(q); found {
			t.Fatalf("%s：不该算计过", name)
		}
	}

	head := revise(covering, domain.OccurrenceSuperseded)
	if record, found := find(query("tenant-1", basis, "OCC-NEW", "parcel-1")); !found || record.Key != head.Key {
		t.Fatalf("被替代之后应交回当前版 v2：found=%v key=%+v", found, record.Key)
	}
}

// Covers: AT-TF-169 的册面（ADR-0209 决定四）——同一（租户、原因、范围）上，按范围计过的一项（单位在按范围计的那几个
// 词里）盖住范围上任何成员、任何事实依据；本次按范围计时范围上任何一项都算；别的范围、按件计且成员不交的不算。
func TestCoverageCountsAScopeChargedOccurrenceAcrossItsScope(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	perSegment := occurrenceRecordJudged(t, "OCC-SCOPE-1", "seg-1", "SEGMENT", declaredOccurrenceCondition(t), "parcel-1")
	mustSaveOccurrence(t, transactor, ctx, repository, perSegment)
	perPiece := occurrenceRecordJudged(t, "OCC-SCOPE-2", "seg-2", "PIECE", declaredOccurrenceCondition(t), "parcel-1")
	mustSaveOccurrence(t, transactor, ctx, repository, perPiece)

	query := func(scope, basis string, whole bool, objects ...string) ports.ChargeOccurrenceCoverageQuery {
		members := make([]domain.CarriedObjectReference, 0, len(objects))
		for _, object := range objects {
			members = append(members, occurrenceRef(t, domain.NewCarriedObjectReference, object))
		}
		return ports.ChargeOccurrenceCoverageQuery{
			TenantID:   occurrenceRef(t, domain.NewTenantID, "tenant-1"),
			Reason:     domain.FailedAttemptOccurrence,
			Scope:      occurrenceRef(t, domain.NewOccurrenceScopeReference, scope),
			FactBasis:  occurrenceRef(t, domain.NewOccurrenceBasisReference, basis),
			Members:    members,
			Except:     occurrenceRef(t, domain.NewChargeOccurrenceReference, "OCC-NEW"),
			WholeScope: whole,
			ScopeUnits: domain.ScopeCountingUnits(),
		}
	}
	find := func(q ports.ChargeOccurrenceCoverageQuery) (ports.ChargeOccurrenceRecord, bool) {
		t.Helper()
		var record ports.ChargeOccurrenceRecord
		var found bool
		mustWithinOccurrenceTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			record, found, err = repository.FindCovering(txCtx, q)
			return err
		})
		return record, found
	}

	if record, found := find(query("seg-1", "ATTEMPT-RESULT/a-9/parcel-9", false, "parcel-9")); !found || record.Key != perSegment.Key {
		t.Fatalf("按段计过的范围上另一成员、另一依据：found=%v key=%+v", found, record.Key)
	}
	if record, found := find(query("seg-2", "ATTEMPT-RESULT/a-9/parcel-9", true, "parcel-9")); !found || record.Key != perPiece.Key {
		t.Fatalf("本次按范围计、范围上已有按件计的一项：found=%v key=%+v", found, record.Key)
	}
	if _, found := find(query("seg-2", "ATTEMPT-RESULT/a-9/parcel-9", false, "parcel-9")); found {
		t.Fatal("按件计、成员不交、别的依据，不该算计过")
	}
	if _, found := find(query("seg-3", "ATTEMPT-RESULT/a-1/parcel-1", true, "parcel-1")); found {
		t.Fatal("别的范围不该算计过")
	}
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
		Condition:   assertedOccurrenceCondition(t, "agreement-1/v1 failed attempt clause met"),
	})
	if err != nil {
		t.Fatalf("构造发生项夹具 %s/%s：%v", occurrence, validity, err)
	}
	return ports.ChargeOccurrenceRecord{Key: key, Occurrence: built, RecordedAt: occurredAtFixture}
}

// occurrenceRecordJudged 同 occurrenceRecord，范围、单位与发生条件由调用方给。
func occurrenceRecordJudged(
	t *testing.T,
	occurrence, scope, unit string,
	condition domain.OccurrenceCondition,
	objects ...string,
) ports.ChargeOccurrenceRecord {
	t.Helper()
	members := make([]domain.CarriedObjectReference, 0, len(objects))
	for _, object := range objects {
		members = append(members, occurrenceRef(t, domain.NewCarriedObjectReference, object))
	}
	key := occurrenceKey(t, "tenant-1", occurrence, "V1")
	built, err := domain.FormTransportChargeOccurrence(domain.TransportChargeOccurrenceSpec{
		TenantID:    key.TenantID,
		Occurrence:  key.Occurrence,
		Journey:     occurrenceRef(t, domain.NewJourneyReference, "journey-1"),
		LegalEntity: occurrenceRef(t, domain.NewProcurementLegalEntityReference, "legal-1"),
		Provider:    occurrenceRef(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:   occurrenceRef(t, domain.NewAgreementSnapshotReference, "agreement-1/v2"),
		Reason:      domain.FailedAttemptOccurrence,
		FactBasis:   occurrenceRef(t, domain.NewOccurrenceBasisReference, "ATTEMPT-RESULT/"+scope+"/"+objects[0]),
		Scope:       occurrenceRef(t, domain.NewOccurrenceScopeReference, scope),
		Members:     members,
		Quantity:    1,
		Unit:        occurrenceRef(t, domain.NewQuantityUnitReference, unit),
		OccurredAt:  occurredAtFixture,
		Validity:    key.Validity,
		Condition:   condition,
	})
	if err != nil {
		t.Fatalf("构造发生项夹具 %s：%v", occurrence, err)
	}
	return ports.ChargeOccurrenceRecord{Key: key, Occurrence: built, RecordedAt: occurredAtFixture}
}

// declaredOccurrenceCondition 是夹具里一份「按协议声明判」的依据，登记者写成操作者主体的「发行方#sub」。
func declaredOccurrenceCondition(t *testing.T) domain.OccurrenceCondition {
	t.Helper()
	condition, err := domain.NewAgreementDeclaredOccurrenceCondition(
		occurrenceRef(t, domain.NewRegistrantReference, "https://dex.syn.example#SYN-OPERATOR-CONFIG-01"))
	if err != nil {
		t.Fatalf("按协议判夹具：%v", err)
	}
	return condition
}

// assertedOccurrenceCondition 是夹具里一份登记者断言：断言人写成操作者主体的「发行方#sub」。
func assertedOccurrenceCondition(t *testing.T, assertion string) domain.OccurrenceCondition {
	t.Helper()
	condition, err := domain.NewAssertedOccurrenceCondition(
		occurrenceRef(t, domain.NewRegistrantReference, "https://dex.syn.example#SYN-OPERATOR-CONFIG-01"), assertion)
	if err != nil {
		t.Fatalf("断言夹具：%v", err)
	}
	return condition
}
