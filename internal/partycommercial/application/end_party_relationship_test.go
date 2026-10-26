package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件证参与方关系的三个结束口（票 ps-external-mark-relations/17）的编排：走领域既有的结束方法落下一笔终止修订；
// 重放、冲突、未找到与修订错位照本能力面停用口的代数；结束时刻不得晚于登记时刻；已结束的关系不再收新修订；替代在同一笔里
// 登记接替关系，接替关系被拒时原关系一个字节不动。

var (
	endingRegisteredAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	endingAt           = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	boundedEndsAt      = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
)

type endingFixture struct {
	registry *fakePartyRegistry
	register *application.RegisterPartyIdentityHandler
	ending   *application.PartyRelationshipEndingHandler
	tenant   domain.TenantID
}

func newEndingFixture(t *testing.T) endingFixture {
	t.Helper()
	registry := newFakePartyRegistry()
	fixture := endingFixture{
		registry: registry,
		register: application.NewRegisterPartyIdentityHandler(registry, nil),
		ending:   application.NewPartyRelationshipEndingHandler(registry, fixedClock{at: endingRegisteredAt}),
		tenant:   identityValue(t, domain.NewTenantID, "tenant-1"),
	}
	for _, id := range []string{"party-1", "operator-1", "party-2"} {
		registerParty(t, fixture.register, "tenant-1", id, "合成参与方 "+id)
	}
	return fixture
}

// registerLiveRelationship 登一段已批准的客户关系；endsAt 为零即开区间。
func (fixture endingFixture) registerLiveRelationship(t *testing.T, id string, endsAt time.Time, approved bool) {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(relationStartsAt, endsAt)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	command := application.RegisterPartyRelationshipCommand{
		Tenant:   fixture.tenant,
		ID:       identityValue(t, domain.NewRelationshipID, id),
		Revision: 1,
		Spec: domain.PartyRelationshipSpec{
			Holder:       identityValue(t, domain.NewPartyID, "party-1"),
			Counterparty: identityValue(t, domain.NewPartyID, "operator-1"),
			Role:         domain.CustomerRole,
			Scope:        identityValue(t, domain.NewCommercialScopeReference, "scope-a"),
			Basis:        identityValue(t, domain.NewRelationshipBasisReference, "contract-"+id),
			Effective:    interval,
		},
	}
	if approved {
		command.Approval = &application.RelationshipApproval{
			Reference:  identityValue(t, domain.NewApprovalReference, "approval-"+id),
			ApprovedAt: relationStartsAt,
		}
	}
	result, err := fixture.register.RegisterRelationship(context.Background(), command)
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("register relationship %s = (%v, %v, %v)", id, result.Outcome(), result.Cause(), err)
	}
}

func (fixture endingFixture) latest(t *testing.T, id string) domain.PartyRelationshipRegistration {
	t.Helper()
	latest, found, err := fixture.registry.LoadLatestRelationship(context.Background(), fixture.tenant,
		identityValue(t, domain.NewRelationshipID, id))
	if err != nil || !found {
		t.Fatalf("load %s = (%v, %v)", id, found, err)
	}
	return latest
}

func (fixture endingFixture) revoke(t *testing.T, id string, revision int, basis string, at time.Time) application.RevokePartyRelationshipCommand {
	t.Helper()
	return application.RevokePartyRelationshipCommand{
		Tenant:   fixture.tenant,
		ID:       identityValue(t, domain.NewRelationshipID, id),
		Revision: revision,
		Basis:    identityValue(t, domain.NewRelationshipBasisReference, basis),
		At:       at,
	}
}

func wantOutcome(t *testing.T, label string, result application.PartyRegistryResult, err error, want application.PartyRegistryOutcome) {
	t.Helper()
	if err != nil || result.Outcome() != want {
		t.Fatalf("%s = (%v, cause %v, err %v), want %v", label, result.Outcome(), result.Cause(), err, want)
	}
}

// Covers: 票 17 完成判据「领域、编排……各层用例」的编排一格（撤销）——撤销落下一笔终止修订、依据随它留下；同一笔重放答
// 重复，同修订异依据答冲突；一段关系只结束一次；已结束的关系不再收新修订（登记口也不收）；关系不在册答未找到；修订错位、
// 结束时刻晚于登记时刻、候选关系都答未受理，一个字节没写。
func TestRevokingARelationshipEndsItOnce(t *testing.T) {
	fixture := newEndingFixture(t)
	ctx := context.Background()
	fixture.registerLiveRelationship(t, "rel-1", time.Time{}, true)

	command := fixture.revoke(t, "rel-1", 2, "revoke-1", endingAt)
	result, err := fixture.ending.Revoke(ctx, command)
	wantOutcome(t, "revoke", result, err, application.PartyRelationshipEnded)
	if result.Outcome().String() != "ENDED" {
		t.Fatalf("outcome name = %q, want ENDED", result.Outcome())
	}
	latest := fixture.latest(t, "rel-1")
	relationship := latest.Relationship()
	if latest.Revision() != 2 || relationship.Status() != domain.RelationshipRevoked {
		t.Fatalf("latest = r%d %s, want r2 REVOKED", latest.Revision(), relationship.Status())
	}
	if basis, has := relationship.EndBasis(); !has || basis.String() != "revoke-1" {
		t.Fatalf("end basis = (%v, %v)", basis, has)
	}
	if relationship.AppliesAt(endingAt.Add(time.Hour)) {
		t.Fatal("撤销之后关系仍支持新的商业决定")
	}

	replay, err := fixture.ending.Revoke(ctx, command)
	wantOutcome(t, "replay", replay, err, application.PartyIdentityAlreadyRegistered)
	conflict, err := fixture.ending.Revoke(ctx, fixture.revoke(t, "rel-1", 2, "revoke-other", endingAt))
	wantOutcome(t, "same revision, other basis", conflict, err, application.PartyIdentityContentConflict)
	again, err := fixture.ending.Revoke(ctx, fixture.revoke(t, "rel-1", 3, "revoke-2", endingAt))
	wantOutcome(t, "second revocation", again, err, application.PartyIdentityNotAccepted)

	reopened, err := fixture.register.RegisterRelationship(ctx, application.RegisterPartyRelationshipCommand{
		Tenant:   fixture.tenant,
		ID:       identityValue(t, domain.NewRelationshipID, "rel-1"),
		Revision: 3,
		Spec: domain.PartyRelationshipSpec{
			Holder:       relationship.Holder(),
			Counterparty: relationship.Counterparty(),
			Role:         relationship.Role(),
			Scope:        relationship.Scope(),
			Basis:        relationship.Basis(),
			Effective:    relationship.Effective(),
		},
		Approval: &application.RelationshipApproval{
			Reference:  identityValue(t, domain.NewApprovalReference, "approval-reopen"),
			ApprovedAt: endingRegisteredAt,
		},
	})
	wantOutcome(t, "register a new revision on a revoked relationship", reopened, err, application.PartyIdentityNotAccepted)
	if !strings.Contains(reopened.Cause().Error(), "已结束") {
		t.Fatalf("reopen cause = %v", reopened.Cause())
	}
	if fixture.latest(t, "rel-1").Revision() != 2 {
		t.Fatal("被拒的结束或登记写进了册")
	}

	missing, err := fixture.ending.Revoke(ctx, fixture.revoke(t, "rel-missing", 2, "revoke-1", endingAt))
	wantOutcome(t, "missing relationship", missing, err, application.PartyIdentityNotFound)

	fixture.registerLiveRelationship(t, "rel-2", time.Time{}, true)
	fixture.registerLiveRelationship(t, "rel-candidate", time.Time{}, false)
	for name, probe := range map[string]application.RevokePartyRelationshipCommand{
		"修订错位":       fixture.revoke(t, "rel-2", 3, "revoke-1", endingAt),
		"结束时刻晚于登记时刻": fixture.revoke(t, "rel-2", 2, "revoke-1", endingRegisteredAt.Add(time.Second)),
		"候选关系":       fixture.revoke(t, "rel-candidate", 2, "revoke-1", endingAt),
	} {
		refused, err := fixture.ending.Revoke(ctx, probe)
		wantOutcome(t, name, refused, err, application.PartyIdentityNotAccepted)
		if refused.Cause() == nil {
			t.Fatalf("%s: 未受理没带原因", name)
		}
	}
	if fixture.latest(t, "rel-2").Revision() != 1 || fixture.latest(t, "rel-candidate").Revision() != 1 {
		t.Fatal("被拒的撤销写进了册")
	}
}

// Covers: 票 17 完成判据「领域、编排……各层用例」的编排一格（到期）——到期带依据落下终止修订、重放答重复；区间终点未到、
// 开区间、到期时刻晚于登记时刻都答未受理。
func TestExpiringARelationshipNeedsItsEndToHaveArrived(t *testing.T) {
	fixture := newEndingFixture(t)
	ctx := context.Background()
	fixture.registerLiveRelationship(t, "rel-bounded", boundedEndsAt, true)
	fixture.registerLiveRelationship(t, "rel-open", time.Time{}, true)
	fixture.registerLiveRelationship(t, "rel-future", endingRegisteredAt.Add(30*24*time.Hour), true)

	expire := func(id string, at time.Time) application.ExpirePartyRelationshipCommand {
		return application.ExpirePartyRelationshipCommand{
			Tenant:   fixture.tenant,
			ID:       identityValue(t, domain.NewRelationshipID, id),
			Revision: 2,
			Basis:    identityValue(t, domain.NewRelationshipBasisReference, "term-ended-"+id),
			At:       at,
		}
	}

	for name, probe := range map[string]application.ExpirePartyRelationshipCommand{
		"终点未到":       expire("rel-bounded", boundedEndsAt.Add(-time.Second)),
		"开区间":        expire("rel-open", endingAt),
		"到期时刻晚于登记时刻": expire("rel-future", endingRegisteredAt.Add(30*24*time.Hour)),
	} {
		refused, err := fixture.ending.Expire(ctx, probe)
		wantOutcome(t, name, refused, err, application.PartyIdentityNotAccepted)
	}

	command := expire("rel-bounded", boundedEndsAt)
	result, err := fixture.ending.Expire(ctx, command)
	wantOutcome(t, "expire", result, err, application.PartyRelationshipEnded)
	relationship := fixture.latest(t, "rel-bounded").Relationship()
	if relationship.Status() != domain.RelationshipExpired {
		t.Fatalf("status = %s, want EXPIRED", relationship.Status())
	}
	if basis, has := relationship.EndBasis(); !has || basis.String() != "term-ended-rel-bounded" {
		t.Fatalf("end basis = (%v, %v)", basis, has)
	}
	replay, err := fixture.ending.Expire(ctx, command)
	wantOutcome(t, "replay", replay, err, application.PartyIdentityAlreadyRegistered)
	revokeAfterExpiry, err := fixture.ending.Revoke(ctx, fixture.revoke(t, "rel-bounded", 2, "revoke-1", boundedEndsAt))
	wantOutcome(t, "revoke an expired relationship at its ending revision", revokeAfterExpiry, err, application.PartyIdentityContentConflict)
}

func (fixture endingFixture) supersede(t *testing.T, successor, successorRelationship string) application.SupersedePartyRelationshipCommand {
	t.Helper()
	return application.SupersedePartyRelationshipCommand{
		Tenant:                fixture.tenant,
		ID:                    identityValue(t, domain.NewRelationshipID, "rel-1"),
		Revision:              2,
		Successor:             identityValue(t, domain.NewPartyID, successor),
		At:                    endingAt,
		SuccessorRelationship: identityValue(t, domain.NewRelationshipID, successorRelationship),
		Basis:                 identityValue(t, domain.NewRelationshipBasisReference, "handover-1"),
		Approval: application.RelationshipApproval{
			Reference:  identityValue(t, domain.NewApprovalReference, "approval-handover-1"),
			ApprovedAt: endingAt,
		},
	}
}

// Covers: 票 17 裁定「替代：以接替的持有方结束本关系并起一段新关系」的编排一格——原关系落`已替代`、记下接替方，同一笔里
// 登记接替关系（持有方换成接替方，相对方、角色、范围照旧，自替代时刻起）；同一笔重放答重复，换了接替关系标识或接替方的
// 重放答冲突且不多立一段关系；接替方不在册、接替关系与原关系同标识、接替关系标识上已有别的内容，都不动原关系。
func TestSupersedingARelationshipRegistersItsSuccessorInTheSameStroke(t *testing.T) {
	fixture := newEndingFixture(t)
	ctx := context.Background()
	fixture.registerLiveRelationship(t, "rel-1", time.Time{}, true)
	fixture.registerLiveRelationship(t, "rel-taken", time.Time{}, true)

	for name, probe := range map[string]application.SupersedePartyRelationshipCommand{
		"接替方不在册":      fixture.supersede(t, "party-unregistered", "rel-1-successor"),
		"接替关系与原关系同标识": fixture.supersede(t, "party-2", "rel-1"),
		"接替方就是原持有方":   fixture.supersede(t, "party-1", "rel-1-successor"),
	} {
		refused, err := fixture.ending.Supersede(ctx, probe)
		wantOutcome(t, name, refused, err, application.PartyIdentityNotAccepted)
	}
	taken, err := fixture.ending.Supersede(ctx, fixture.supersede(t, "party-2", "rel-taken"))
	wantOutcome(t, "successor id carries other content", taken, err, application.PartyIdentityContentConflict)
	if fixture.latest(t, "rel-1").Revision() != 1 || fixture.latest(t, "rel-taken").Revision() != 1 {
		t.Fatal("被拒的替代动了原关系或占用的那段关系")
	}
	if _, found, _ := fixture.registry.LoadLatestRelationship(ctx, fixture.tenant,
		identityValue(t, domain.NewRelationshipID, "rel-1-successor")); found {
		t.Fatal("被拒的替代立起了接替关系")
	}

	command := fixture.supersede(t, "party-2", "rel-1-successor")
	result, err := fixture.ending.Supersede(ctx, command)
	wantOutcome(t, "supersede", result, err, application.PartyRelationshipEnded)

	old := fixture.latest(t, "rel-1")
	if old.Revision() != 2 || old.Relationship().Status() != domain.RelationshipSuperseded {
		t.Fatalf("old = r%d %s, want r2 SUPERSEDED", old.Revision(), old.Relationship().Status())
	}
	if successor, has := old.Relationship().Successor(); !has || successor.String() != "party-2" {
		t.Fatalf("old successor = (%v, %v)", successor, has)
	}
	successor := fixture.latest(t, "rel-1-successor")
	taker := successor.Relationship()
	if successor.Revision() != 1 || taker.Status() != domain.RelationshipEffective ||
		taker.Holder().String() != "party-2" || taker.Counterparty() != old.Relationship().Counterparty() ||
		taker.Role() != old.Relationship().Role() || taker.Scope() != old.Relationship().Scope() ||
		taker.Basis().String() != "handover-1" || !taker.Effective().StartsAt().Equal(endingAt) {
		t.Fatalf("successor relationship = r%d %+v", successor.Revision(), taker)
	}
	if !taker.AppliesAt(endingAt.Add(time.Hour)) || old.Relationship().AppliesAt(endingAt.Add(time.Hour)) {
		t.Fatal("替代之后应由接替关系支持新决定、原关系不再支持")
	}

	replay, err := fixture.ending.Supersede(ctx, command)
	wantOutcome(t, "replay", replay, err, application.PartyIdentityAlreadyRegistered)
	otherSuccessorID, err := fixture.ending.Supersede(ctx, fixture.supersede(t, "party-2", "rel-1-successor-bis"))
	wantOutcome(t, "replay with another successor relationship id", otherSuccessorID, err, application.PartyIdentityContentConflict)
	if _, found, _ := fixture.registry.LoadLatestRelationship(ctx, fixture.tenant,
		identityValue(t, domain.NewRelationshipID, "rel-1-successor-bis")); found {
		t.Fatal("换了接替关系标识的重放多立了一段接替关系")
	}
	otherSuccessor, err := fixture.ending.Supersede(ctx, fixture.supersede(t, "operator-2", "rel-1-successor"))
	wantOutcome(t, "replay with another successor party", otherSuccessor, err, application.PartyIdentityContentConflict)
}

// conflictingSaves 是登记册替身的一层包装：给定的那段关系在给定修订上一律答内容冲突，模拟两笔替代并发、对方先占住了
// 原关系的终止修订落点。
type conflictingSaves struct {
	*fakePartyRegistry
	relationship string
	revision     int
}

func (registry conflictingSaves) SaveRelationship(
	ctx context.Context, registration domain.PartyRelationshipRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	if registration.ID().String() == registry.relationship && registration.Revision() == registry.revision {
		return ports.PartyRegistryContentConflict, nil
	}
	return registry.fakePartyRegistry.SaveRelationship(ctx, registration)
}

// Covers: 票 17 替代口「不留半份」——接替关系已写进事务、原关系的终止修订落点却被并发占住时，用例交回 error 让调用方整笔
// 回滚，而不是答一个业务结果让事务带着半份提交。
func TestASupersessionThatCannotEndTheOldRelationshipFailsTheWholeStroke(t *testing.T) {
	fixture := newEndingFixture(t)
	fixture.registerLiveRelationship(t, "rel-1", time.Time{}, true)
	racing := application.NewPartyRelationshipEndingHandler(
		conflictingSaves{fakePartyRegistry: fixture.registry, relationship: "rel-1", revision: 2},
		fixedClock{at: endingRegisteredAt},
	)
	result, err := racing.Supersede(context.Background(), fixture.supersede(t, "party-2", "rel-1-successor"))
	if err == nil {
		t.Fatalf("supersede = %v, want an error that rolls the stroke back", result.Outcome())
	}
}
