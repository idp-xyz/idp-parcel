package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证参与方关系三种结束的领域门（票 ps-external-mark-relations/17）：到期要依据、要区间终点已到；替代不能由原持有方
// 或相对方接替；接替关系只换持有方、接住余下的区间。

func openEndedRelationship(t *testing.T) domain.PartyRelationship {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(relationshipFrom, time.Time{})
	if err != nil {
		t.Fatalf("open interval: %v", err)
	}
	candidate, err := domain.NewCandidateRelationship(domain.PartyRelationshipSpec{
		Holder:       commercialValue(t, domain.NewPartyID, "party-1"),
		Counterparty: commercialValue(t, domain.NewPartyID, "operator-1"),
		Role:         domain.CustomerRole,
		Scope:        commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		Basis:        commercialValue(t, domain.NewRelationshipBasisReference, "basis-1"),
		Effective:    interval,
	})
	if err != nil {
		t.Fatalf("open candidate: %v", err)
	}
	live, err := candidate.Approve(commercialValue(t, domain.NewApprovalReference, "approval-rel"), relationshipFrom)
	if err != nil {
		t.Fatalf("approve open relationship: %v", err)
	}
	return live
}

// Covers: 票 17 完成判据「领域……用例」的到期一格——到期带依据，依据随终止事实留下；区间没有终点、终点未到、缺依据、
// 不在`已生效`都拒，拒时原值不变。
func TestExpiryNeedsABasisAndAReachedEnd(t *testing.T) {
	basis := commercialValue(t, domain.NewRelationshipBasisReference, "expiry-1")
	live := effectiveRelationship(t, "party-1", "operator-1", domain.CustomerRole)

	expired, err := live.Expire(basis, relationshipTo.Add(time.Hour))
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if expired.Status() != domain.RelationshipExpired {
		t.Fatalf("status = %s, want EXPIRED", expired.Status())
	}
	if endBasis, has := expired.EndBasis(); !has || endBasis != basis {
		t.Fatalf("end basis = (%v, %v), want %v", endBasis, has, basis)
	}
	if endedAt, has := expired.EndedAt(); !has || !endedAt.Equal(relationshipTo.Add(time.Hour)) {
		t.Fatalf("ended at = (%v, %v)", endedAt, has)
	}

	refusals := map[string]func() (domain.PartyRelationship, error){
		"缺依据": func() (domain.PartyRelationship, error) {
			return live.Expire(domain.RelationshipBasisReference{}, relationshipTo)
		},
		"终点未到": func() (domain.PartyRelationship, error) { return live.Expire(basis, relationshipTo.Add(-time.Second)) },
		"零时刻":  func() (domain.PartyRelationship, error) { return live.Expire(basis, time.Time{}) },
		"开区间": func() (domain.PartyRelationship, error) {
			return openEndedRelationship(t).Expire(basis, relationshipTo)
		},
		"候选关系": func() (domain.PartyRelationship, error) {
			return candidateRelationship(t, "party-1", "operator-1", domain.CustomerRole).Expire(basis, relationshipTo)
		},
		"已到期再到期": func() (domain.PartyRelationship, error) {
			return expired.Expire(basis, relationshipTo.Add(2*time.Hour))
		},
	}
	for name, refuse := range refusals {
		if _, err := refuse(); !errors.Is(err, domain.ErrInvalidRelationshipTransition) {
			t.Fatalf("%s: err = %v, want ErrInvalidRelationshipTransition", name, err)
		}
	}
	if live.Status() != domain.RelationshipEffective {
		t.Fatal("refused expiries mutated the effective value")
	}
}

// Covers: 票 17 完成判据「领域……用例」的替代一格——接替方不能是原持有方，也不能是相对方；已结束的关系不再替代。
func TestSupersessionNeedsADifferentHolder(t *testing.T) {
	live := effectiveRelationship(t, "party-1", "operator-1", domain.CustomerRole)
	for name, successor := range map[string]string{"原持有方": "party-1", "相对方": "operator-1"} {
		if _, err := live.SupersededBy(commercialValue(t, domain.NewPartyID, successor), withinRelation); !errors.Is(err, domain.ErrInvalidRelationshipTransition) {
			t.Fatalf("%s 接替：err = %v, want ErrInvalidRelationshipTransition", name, err)
		}
	}
	superseded, err := live.SupersededBy(commercialValue(t, domain.NewPartyID, "party-2"), withinRelation)
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if successor, has := superseded.Successor(); !has || successor.String() != "party-2" {
		t.Fatalf("successor = (%v, %v)", successor, has)
	}
	if _, err := superseded.SupersededBy(commercialValue(t, domain.NewPartyID, "party-3"), withinRelation); !errors.Is(err, domain.ErrInvalidRelationshipTransition) {
		t.Fatalf("supersede a superseded relationship: err = %v", err)
	}
	revoked, err := live.Revoke(commercialValue(t, domain.NewRelationshipBasisReference, "revoke-1"), withinRelation)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := revoked.Revoke(commercialValue(t, domain.NewRelationshipBasisReference, "revoke-2"), withinRelation); !errors.Is(err, domain.ErrInvalidRelationshipTransition) {
		t.Fatalf("revoke a revoked relationship: err = %v", err)
	}
}

// Covers: 票 17 裁定「替代：以接替的持有方结束本关系并起一段新关系」——接替关系只换持有方，相对方、角色、范围照旧，
// 接住替代时刻之后余下的区间；替代时刻早于本关系起点时从起点接；余下区间为空、接替方是原持有方或相对方、缺依据都拒。
// 已替代的关系仍推得出同一段接替关系（重放要用）。
func TestTheSuccessorTakesOverOnlyTheHolder(t *testing.T) {
	basis := commercialValue(t, domain.NewRelationshipBasisReference, "handover-1")
	successor := commercialValue(t, domain.NewPartyID, "party-2")
	live := effectiveRelationship(t, "party-1", "operator-1", domain.CustomerRole)

	spec, err := live.SuccessorSpec(successor, basis, withinRelation)
	if err != nil {
		t.Fatalf("successor spec: %v", err)
	}
	if spec.Holder != successor || spec.Counterparty != live.Counterparty() || spec.Role != live.Role() ||
		spec.Scope != live.Scope() || spec.Basis != basis {
		t.Fatalf("successor spec = %+v", spec)
	}
	if !spec.Effective.StartsAt().Equal(withinRelation) {
		t.Fatalf("successor starts at %v, want %v", spec.Effective.StartsAt(), withinRelation)
	}
	if endsAt, bounded := spec.Effective.EndsAt(); !bounded || !endsAt.Equal(relationshipTo) {
		t.Fatalf("successor ends at (%v, %v), want %v", endsAt, bounded, relationshipTo)
	}

	early, err := live.SuccessorSpec(successor, basis, relationshipFrom.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("successor spec before start: %v", err)
	}
	if !early.Effective.StartsAt().Equal(relationshipFrom) {
		t.Fatalf("早于起点的替代：接替关系起于 %v，want %v", early.Effective.StartsAt(), relationshipFrom)
	}

	open, err := openEndedRelationship(t).SuccessorSpec(successor, basis, withinRelation)
	if err != nil {
		t.Fatalf("open successor spec: %v", err)
	}
	if _, bounded := open.Effective.EndsAt(); bounded {
		t.Fatal("开区间关系的接替关系多出一个终点")
	}

	superseded, err := live.SupersededBy(successor, withinRelation)
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	replayed, err := superseded.SuccessorSpec(successor, basis, withinRelation)
	if err != nil || replayed != spec {
		t.Fatalf("已替代的关系推出的接替关系 = (%+v, %v), want %+v", replayed, err, spec)
	}

	for name, probe := range map[string]struct {
		successor domain.PartyID
		basis     domain.RelationshipBasisReference
		at        time.Time
	}{
		"余下区间为空": {successor, basis, relationshipTo},
		"原持有方":   {live.Holder(), basis, withinRelation},
		"相对方":    {live.Counterparty(), basis, withinRelation},
		"缺依据":    {successor, domain.RelationshipBasisReference{}, withinRelation},
		"缺接替方":   {domain.PartyID{}, basis, withinRelation},
		"零时刻":    {successor, basis, time.Time{}},
	} {
		if _, err := live.SuccessorSpec(probe.successor, probe.basis, probe.at); !errors.Is(err, domain.ErrInvalidRelationshipTransition) {
			t.Fatalf("%s: err = %v, want ErrInvalidRelationshipTransition", name, err)
		}
	}
}
