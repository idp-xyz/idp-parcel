package application_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

type judgmentOnEstablishmentFixture struct {
	pickups   *pickupRegistryDouble
	segments  *segmentRegistryDouble
	judgments *judgmentRegistryDouble
	handler   *application.RegisterOffsitePickupHandler
}

func newJudgmentOnEstablishmentFixture(t *testing.T) *judgmentOnEstablishmentFixture {
	t.Helper()
	fixture := &judgmentOnEstablishmentFixture{
		pickups:   newPickupRegistry(),
		segments:  newSegmentRegistry(),
		judgments: newJudgmentRegistry(),
	}
	fixture.handler = application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    fixture.pickups,
		Segments:   fixture.segments,
		Judgments:  fixture.judgments,
		Versions:   &pickupRegVersionFactory{},
		Downstream: &pickupRegHandoffDouble{},
		Clock:      pickupRegClock{at: pickupRegisteredAt},
	})
	return fixture
}

func judgmentKeyFixture(t *testing.T, tenant, segment string) ports.ActualCarrierJudgmentKey {
	t.Helper()
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	segmentRef, err := domain.NewFulfillmentSegmentReference(segment)
	if err != nil {
		t.Fatalf("segment: %v", err)
	}
	return ports.ActualCarrierJudgmentKey{TenantID: tenantID, Segment: segmentRef}
}

// Covers: CONTEXT 生命周期「实际履约段成立 → 首个判断版本」与规则节「段没有『尚无判断』的状态」；票面
// 「段成立后铸第一版的挂点（挂在 enterFulfillmentSegment 落库之后）」。首版业务时间是段成立时刻——首个
// 对象的控制起点，不是登记时刻；形成时间是本上下文的时钟。
func TestEstablishingASegmentOpensTheCarrierJudgmentWithAPendingFirstVersion(t *testing.T) {
	fixture := newJudgmentOnEstablishmentFixture(t)
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.PickupRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("outcome = %q debt = %q，want PICKUP_REGISTERED 且无欠账", result.Outcome(), result.SegmentContinuationReference())
	}

	record, found, err := fixture.judgments.FindByKey(t.Context(), judgmentKeyFixture(t, "tenant-1", "segment-1"))
	if err != nil || !found {
		t.Fatalf("段成立了却没有判断：found=%v err=%v", found, err)
	}
	versions := record.Judgment.Versions()
	if len(versions) != 1 {
		t.Fatalf("首登应恰有一版，实得 %d", len(versions))
	}
	reason, pending := versions[0].Verdict().Pending()
	if !pending || reason != domain.NoQualifiedCarrierEvidence {
		t.Fatalf("首版应为待确认（无合格证据），实得 pending=%v reason=%s", pending, reason)
	}
	if !record.Judgment.SegmentEstablishedAt().Equal(pickupOccurredAt) || !versions[0].BusinessTime().Equal(pickupOccurredAt) {
		t.Fatalf("段成立时刻 / 首版业务时间应为首个对象的控制起点 %s，实得 %s / %s",
			pickupOccurredAt, record.Judgment.SegmentEstablishedAt(), versions[0].BusinessTime())
	}
	if !versions[0].FormedAt().Equal(pickupRegisteredAt) {
		t.Fatalf("首版形成时间应为本上下文时钟 %s，实得 %s", pickupRegisteredAt, versions[0].FormedAt())
	}
}

// 后续对象加入既有段不再开第二份判断：判断以段为单位，一段一份（ADR-0103 决定一）。
func TestJoiningAnExistingSegmentDoesNotOpenASecondJudgment(t *testing.T) {
	fixture := newJudgmentOnEstablishmentFixture(t)
	first := pickupRegistrationCommand(t)
	first.Segment = "segment-1"
	if _, err := fixture.handler.Register(t.Context(), first); err != nil {
		t.Fatalf("首个对象：%v", err)
	}
	second := pickupRegistrationCommand(t)
	second.Segment = "segment-1"
	second.Object = "parcel-2"
	second.Attempt = "attempt-2"
	if _, err := fixture.handler.Register(t.Context(), second); err != nil {
		t.Fatalf("第二个对象：%v", err)
	}
	if fixture.judgments.opens != 1 {
		t.Fatalf("判断按段一份：Open 应恰一次，实得 %d", fixture.judgments.opens)
	}
}

// 判断登记册可缺席，判据同 Segments 缺席那一条：派生一侧缺席不让来源保全停摆，也不留欠账。
func TestAnAbsentJudgmentRegistryLeavesTheSegmentHalfUntouched(t *testing.T) {
	fixture := newJudgmentOnEstablishmentFixture(t)
	fixture.handler = application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    fixture.pickups,
		Segments:   fixture.segments,
		Versions:   &pickupRegVersionFactory{},
		Downstream: &pickupRegHandoffDouble{},
		Clock:      pickupRegClock{at: pickupRegisteredAt},
	})
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.PickupRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("outcome = %q debt = %q", result.Outcome(), result.SegmentContinuationReference())
	}
	if fixture.segments.saves != 1 {
		t.Fatalf("段照常成立：saves = %d", fixture.segments.saves)
	}
}

// 判断登记册写不进是欠账：首版是段成立那一笔的一部分（CONTEXT「段成立即形成首个判断版本」），收寄与段
// 照常登记，续办引用说「回来补这一半」。
func TestAJudgmentRegistryFailureLeavesADebtOnTheSegmentHalf(t *testing.T) {
	fixture := newJudgmentOnEstablishmentFixture(t)
	fixture.judgments.openErr = errors.New("判断登记册不可用")
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.PickupRegistered {
		t.Fatalf("outcome = %q, want PICKUP_REGISTERED——派生一侧失败不回滚来源登记", result.Outcome())
	}
	if result.SegmentContinuationReference() == "" {
		t.Fatal("判断首版没落库却没留续办引用")
	}
	if _, found, _ := fixture.segments.FindByKey(t.Context(), segmentKeyOf(t, "tenant-1", "segment-1")); !found {
		t.Fatal("段那一半也被翻回去了")
	}
}

func segmentKeyOf(t *testing.T, tenant, segment string) ports.FulfillmentSegmentKey {
	t.Helper()
	key := judgmentKeyFixture(t, tenant, segment)
	return ports.FulfillmentSegmentKey{TenantID: key.TenantID, Segment: key.Segment}
}
