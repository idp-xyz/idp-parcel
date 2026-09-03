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

// 本文件对真实 PostgreSQL 16 证实际履约段登记：整图往返（段 + 逐对象参与关系）、成员差异
// 不被整段结果覆盖、无计划段的参与如实存 NULL、撞键译`已登记`、作用域隔离、无事务拒，
// 以及库内 CHECK 挡住领域造不出的行。夹具全部为合成登记（S 级）。

var segmentEnteredAtFixture = time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)

// TestASegmentRoundTripsWithEachMemberSeparate 证 CONTEXT「每个载运对象分别成立、结束和
// 更正，不能由整段结果覆盖成员差异」——一在场一已离场，往返之后仍是一在场一已离场。
func TestASegmentRoundTripsWithEachMemberSeparate(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	record := segmentRecord(t, "SEG-0001",
		activeMember(t, "parcel-1"),
		deliveredMember(t, "parcel-2"),
	)
	mustSaveSegment(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-0001"))
	if err != nil || !exists {
		t.Fatalf("取回段：%v exists=%v", err, exists)
	}
	if found.Segment.Closed() || found.Segment.ActiveParticipations() != 1 {
		t.Fatalf("段往返变形：closed=%v active=%d", found.Segment.Closed(), found.Segment.ActiveParticipations())
	}

	active, ok := found.Segment.ParticipationFor(segmentRef(t, domain.NewCarriedObjectReference, "parcel-1"))
	if !ok || !active.Active() || active.EntryKind() != domain.EnteredByOffsitePickup {
		t.Fatalf("parcel-1 应当仍在场且入场种类照实：%v", active.EntryKind())
	}
	if planned, has := active.PlannedSegment(); !has || planned.String() != "planned-parcel-1" {
		t.Fatal("计划段没有随行保全")
	}

	ended, ok := found.Segment.ParticipationFor(segmentRef(t, domain.NewCarriedObjectReference, "parcel-2"))
	if !ok || ended.Active() {
		t.Fatal("parcel-2 应当已离场")
	}
	kind, basis, at, done := ended.End()
	if !done || kind != domain.EndedByEffectiveDelivery ||
		basis.String() != "EFFECTIVE-DELIVERY/parcel-2" || !at.Equal(segmentEnteredAtFixture.Add(8*time.Hour)) {
		t.Fatalf("离场三件没有原样带回：kind=%q basis=%q at=%v", kind, basis, at)
	}
}

// TestAParticipationWithoutAPlannedSegmentStoresNull 证「计划段可缺席」在库面是 NULL 而
// 不是空串——两者在领域里是不同的答案：缺席说「这个对象当时没有计划段」，空串说不出任何话。
func TestAParticipationWithoutAPlannedSegmentStoresNull(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()

	member := activeMember(t, "parcel-1")
	member.Planned = domain.PlannedSegmentReference{}
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-0002", member))

	var planned *string
	if err := pool.QueryRow(ctx,
		`SELECT planned_ref FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-0002' AND object_ref = 'parcel-1'`,
	).Scan(&planned); err != nil {
		t.Fatalf("读 planned_ref：%v", err)
	}
	if planned != nil {
		t.Fatalf("缺席的计划段落成了 %q，而不是 NULL", *planned)
	}

	found, _, err := repository.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-0002"))
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	participation, _ := found.Segment.ParticipationFor(segmentRef(t, domain.NewCarriedObjectReference, "parcel-1"))
	if _, has := participation.PlannedSegment(); has {
		t.Fatal("读回时凭空长出了一个计划段")
	}
}

// TestAClosedSegmentRoundTripsItsClosure 证关闭两件成对往返。
func TestAClosedSegmentRoundTripsItsClosure(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	closedAt := segmentEnteredAtFixture.Add(10 * time.Hour)
	record := segmentRecord(t, "SEG-0003", deliveredMember(t, "parcel-1"))
	closed, err := record.Segment.CloseSegment(closedAt)
	if err != nil {
		t.Fatalf("关段：%v", err)
	}
	record.Segment = closed
	mustSaveSegment(t, transactor, ctx, repository, record)

	found, _, err := repository.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-0003"))
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	at, isClosed := found.Segment.ClosedAt()
	if !found.Segment.Closed() || !isClosed || !at.Equal(closedAt) {
		t.Fatalf("关闭没有成对往返：closed=%v at=%v", found.Segment.Closed(), at)
	}
}

// TestSavingTheSameSegmentTwiceIsAlreadyRegistered 证撞键是业务答案不是错误（ADR-0031）。
func TestSavingTheSameSegmentTwiceIsAlreadyRegistered(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	record := segmentRecord(t, "SEG-0004", activeMember(t, "parcel-1"))
	mustSaveSegment(t, transactor, ctx, repository, record)

	var outcome ports.SegmentSaveOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	if outcome != ports.SegmentAlreadyRegistered {
		t.Fatalf("重放 outcome = %d, want SegmentAlreadyRegistered", outcome)
	}
}

// TestSegmentsAreInvisibleAcrossTenants 证租户是身份的最高隔离边界。
func TestSegmentsAreInvisibleAcrossTenants(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-0005", activeMember(t, "parcel-1")))

	if _, exists, err := repository.FindByKey(ctx, segmentKeyFixture(t, "tenant-other", "SEG-0005")); err != nil || exists {
		t.Fatalf("他租看见了这个段：exists=%v err=%v", exists, err)
	}
}

// TestSegmentWritesRefuseToRunOutsideATransaction 证写口无环境事务即拒——两张表必须同笔落，
// 半个段（有段无成员）是领域读不回来的东西。
//
// 断言指名 ErrTransactionRequired 而不是 err != nil：后者会把「拒得对」与「因为别的原因也
// 失败了」混成一格，而这条用例要钉的恰恰是前者。
func TestSegmentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newSegmentRegistry(t)

	_, err := repository.Save(t.Context(), segmentRecord(t, "SEG-0006", activeMember(t, "parcel-1")))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门：绕过领域
// 直接 INSERT 也落不进领域造不出的行。
func TestCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()

	// 先落一个真段：参与关系那几条坏行要挂在一个存在的段上，否则先撞外键、验不到本意
	// 要验的那几道 CHECK。
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-0007", activeMember(t, "parcel-1")))

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.actual_fulfillment_segment
		     (tenant_id, segment_ref, closed, closed_at, recorded_at)
		 VALUES ('tenant-1', 'SEG-BAD-1', true, NULL, now())`); err == nil {
		t.Fatal("「关了但不知何时关的」段落进去了")
	}

	base := `INSERT INTO transport_fulfillment.fulfillment_participation
	             (tenant_id, segment_ref, object_ref, planned_ref, entry_kind, entry_basis,
	              entered_at, end_kind, end_basis, ended_at, recorded_at) VALUES `
	for name, values := range map[string]string{
		"集外入场种类": `('tenant-1','SEG-0007','bad-1',NULL,'SCANNED','b',now(),NULL,NULL,NULL,now())`,
		"半截的离场":  `('tenant-1','SEG-0007','bad-2',NULL,'OFFSITE_PICKUP','b',now(),'EFFECTIVE_DELIVERY',NULL,NULL,now())`,
		"集外离场种类": `('tenant-1','SEG-0007','bad-3',NULL,'OFFSITE_PICKUP','b',now(),'GAVE_UP','b',now(),now())`,
		"离场早于入场": `('tenant-1','SEG-0007','bad-4',NULL,'OFFSITE_PICKUP','b',now(),'EFFECTIVE_DELIVERY','b',now() - interval '1 day',now())`,
		"空串计划段":  `('tenant-1','SEG-0007','bad-5','','OFFSITE_PICKUP','b',now(),NULL,NULL,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatalf("%s 落进去了", name)
			}
		})
	}
}

// —— 夹具 ——

func newSegmentRegistry(t *testing.T) (*adapter.FulfillmentSegments, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatalf("构造段登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinSegmentTransaction(
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

func mustSaveSegment(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.FulfillmentSegments,
	record ports.FulfillmentSegmentRecord,
) {
	t.Helper()
	var outcome ports.SegmentSaveOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	if outcome != ports.SegmentSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
}

func segmentRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func segmentKeyFixture(t *testing.T, tenant, segment string) ports.FulfillmentSegmentKey {
	t.Helper()
	return ports.FulfillmentSegmentKey{
		TenantID: segmentRef(t, domain.NewTenantID, tenant),
		Segment:  segmentRef(t, domain.NewFulfillmentSegmentReference, segment),
	}
}

func activeMember(t *testing.T, object string) domain.RehydrateParticipationSpec {
	t.Helper()
	return domain.RehydrateParticipationSpec{
		Object:     segmentRef(t, domain.NewCarriedObjectReference, object),
		Planned:    segmentRef(t, domain.NewPlannedSegmentReference, "planned-"+object),
		EntryKind:  domain.EnteredByOffsitePickup,
		EntryBasis: segmentRef(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/"+object),
		EnteredAt:  segmentEnteredAtFixture,
	}
}

func deliveredMember(t *testing.T, object string) domain.RehydrateParticipationSpec {
	t.Helper()
	member := activeMember(t, object)
	member.EndKind = domain.EndedByEffectiveDelivery
	member.EndBasis = segmentRef(t, domain.NewParticipationBasisReference, "EFFECTIVE-DELIVERY/"+object)
	member.EndedAt = segmentEnteredAtFixture.Add(8 * time.Hour)
	return member
}

func segmentRecord(
	t *testing.T,
	segment string,
	members ...domain.RehydrateParticipationSpec,
) ports.FulfillmentSegmentRecord {
	t.Helper()
	key := segmentKeyFixture(t, "tenant-1", segment)
	built, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       key.TenantID,
		Segment:        key.Segment,
		Participations: members,
	})
	if err != nil {
		t.Fatalf("构造段夹具 %s：%v", segment, err)
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: built, RecordedAt: segmentEnteredAtFixture}
}

// —— 三个窄写口（ADR-0097）——

// TestJoiningAnExistingSegmentIsInsertOnly 证第一个窄口：只插不改，撞键是业务答案。
func TestJoiningAnExistingSegmentIsInsertOnly(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	key := segmentKeyFixture(t, "tenant-1", "SEG-J001")
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-J001", activeMember(t, "parcel-1")))

	joined := rebuiltParticipation(t, "SEG-J001", activeMember(t, "parcel-2"))
	var first, second ports.SegmentJoinOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if first, err = repository.Join(txCtx, key, joined, segmentEnteredAtFixture); err != nil {
			return err
		}
		second, err = repository.Join(txCtx, key, joined, segmentEnteredAtFixture)
		return err
	})
	if first != ports.ObjectJoined {
		t.Fatalf("首次加入 outcome = %d", first)
	}
	if second != ports.ObjectAlreadyParticipating {
		t.Fatalf("重复加入 outcome = %d, want ObjectAlreadyParticipating", second)
	}

	found, _, err := repository.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	if found.Segment.ActiveParticipations() != 2 {
		t.Fatalf("在场参与 = %d, want 2", found.Segment.ActiveParticipations())
	}
}

// TestEndingAParticipationCannotOverwriteAnExistingEnd 证第二个窄口最要紧的那一条：
// WHERE ended_at IS NULL 让「改写一条已离场的参与」表达不出来——第二次离场既不报错也不
// 生效，原离场三件原样留着。
func TestEndingAParticipationCannotOverwriteAnExistingEnd(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	key := segmentKeyFixture(t, "tenant-1", "SEG-E001")
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-E001", activeMember(t, "parcel-1")))

	delivered := rebuiltParticipation(t, "SEG-E001", deliveredMember(t, "parcel-1"))
	var first ports.ParticipationEndOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		first, err = repository.EndParticipation(txCtx, key, delivered)
		return err
	})
	if first != ports.ParticipationEnded {
		t.Fatalf("首次离场 outcome = %d", first)
	}

	// 换一套完全不同的离场三件再试一次——若它生效，原离场就被改写了。
	other := activeMember(t, "parcel-1")
	other.EndKind = domain.EndedByControlTermination
	other.EndBasis = segmentRef(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/other")
	other.EndedAt = segmentEnteredAtFixture.Add(20 * time.Hour)
	var second ports.ParticipationEndOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		second, err = repository.EndParticipation(txCtx, key, rebuiltParticipation(t, "SEG-E001", other))
		return err
	})
	if second != ports.ParticipationAlreadyEnded {
		t.Fatalf("二次离场 outcome = %d, want ParticipationAlreadyEnded", second)
	}

	found, _, err := repository.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	participation, _ := found.Segment.ParticipationFor(segmentRef(t, domain.NewCarriedObjectReference, "parcel-1"))
	kind, basis, at, ended := participation.End()
	if !ended || kind != domain.EndedByEffectiveDelivery ||
		basis.String() != "EFFECTIVE-DELIVERY/parcel-1" || !at.Equal(segmentEnteredAtFixture.Add(8*time.Hour)) {
		t.Fatalf("原离场被改写了：%q %q %v", kind, basis, at)
	}
}

// TestClosingASegmentTwiceIsAlreadyClosed 证第三个窄口：WHERE closed = false 让重复关段与
// 「把已关闭的段改回未关闭」都表达不出来。
func TestClosingASegmentTwiceIsAlreadyClosed(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	key := segmentKeyFixture(t, "tenant-1", "SEG-C001")
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-C001", deliveredMember(t, "parcel-1")))

	closedAt := segmentEnteredAtFixture.Add(12 * time.Hour)
	var first, second ports.SegmentCloseOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if first, err = repository.CloseSegment(txCtx, key, closedAt); err != nil {
			return err
		}
		second, err = repository.CloseSegment(txCtx, key, closedAt.Add(time.Hour))
		return err
	})
	if first != ports.SegmentClosed {
		t.Fatalf("首次关段 outcome = %d", first)
	}
	if second != ports.SegmentAlreadyClosed {
		t.Fatalf("二次关段 outcome = %d, want SegmentAlreadyClosed", second)
	}

	found, _, err := repository.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	at, isClosed := found.Segment.ClosedAt()
	if !isClosed || !at.Equal(closedAt) {
		t.Fatalf("关闭时刻被二次关段改写了：%v", at)
	}
}

// TestNarrowSegmentDoorsRefuseToRunOutsideATransaction 证三个窄口各自都要环境事务。
func TestNarrowSegmentDoorsRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newSegmentRegistry(t)
	ctx := t.Context()
	key := segmentKeyFixture(t, "tenant-1", "SEG-T001")
	member := rebuiltParticipation(t, "SEG-T001", deliveredMember(t, "parcel-1"))

	if _, err := repository.Join(ctx, key, member, segmentEnteredAtFixture); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("Join 无事务应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.EndParticipation(ctx, key, member); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("EndParticipation 无事务应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.CloseSegment(ctx, key, segmentEnteredAtFixture); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("CloseSegment 无事务应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// rebuiltParticipation 从重建规格取出单条参与关系——测试要传给窄口的是领域值，
// 而领域值只能由段交出来。
func rebuiltParticipation(
	t *testing.T,
	segment string,
	member domain.RehydrateParticipationSpec,
) domain.FulfillmentParticipation {
	t.Helper()
	built, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       segmentRef(t, domain.NewTenantID, "tenant-1"),
		Segment:        segmentRef(t, domain.NewFulfillmentSegmentReference, segment),
		Participations: []domain.RehydrateParticipationSpec{member},
	})
	if err != nil {
		t.Fatalf("构造参与关系：%v", err)
	}
	return built.Participations()[0]
}
