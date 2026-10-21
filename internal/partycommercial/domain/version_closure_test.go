package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证按后继收尾旧版（ADR-0233，票 demo-chain-gaps/50）在领域一侧的判据：截止落一条区间更正，解析据它在两版之间
// 按时点唯一选择；同起点重发时旧版按决定退役，此后只选后继；重放不重登；收尾不成立的各格拒；自带区间的政策正文同样
// 被收尾收窄；供应商协议正文折进更正与退役后照新的口径答支不支持采购。

var (
	cutPredecessorStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cutSuccessorStart   = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	cutCorrectedAt      = time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	beforeCut           = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

// Covers: `AT-PC-054`「同一对象新版已生效、区间与旧版重叠，按后继截止把旧版截止在新版生效时点 → 原版本、原正文与
// 原区间保留，截止关系留在册上，修订标识推进；新版生效时点之前解析得旧版，之后得新版」。
func TestCutAtSuccessorStartSplitsTheOverlapByTime(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
	next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)

	if got := domain.ResolveCommercialBasis(registry, resolutionKeyAt(t, "scope-a", domain.CustomerContractObject, anchorAt), nil); got.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("两版重叠时 outcome = %q, want APPLICABILITY_CONFLICT（现状复现）", got.Outcome())
	}
	before := registry.ViewRevision(old.Tenant(), old.Scope())

	correction, replay := mustCut(t, registry, old, next, "SYN-CUT-1")
	if replay {
		t.Fatal("第一次截止被认成了重放")
	}
	if end, bounded := correction.CorrectedInterval().EndsAt(); !bounded || !end.Equal(cutSuccessorStart) {
		t.Fatalf("截止点 = %v bounded=%v, want 后继生效时点 %v", end, bounded, cutSuccessorStart)
	}
	if !correction.CorrectedInterval().StartsAt().Equal(cutPredecessorStart) {
		t.Fatalf("截止改了旧版起点：%v", correction.CorrectedInterval().StartsAt())
	}
	after, err := registry.RegisterValidityCorrection(correction)
	if err != nil {
		t.Fatalf("register correction: %v", err)
	}
	if after.String() == before.String() {
		t.Fatal("截止后修订标识没有推进，消费方测不出旧解析已失效")
	}

	stored, _ := registry.Lookup(old.Tenant(), old.Kind(), old.ObjectID(), old.Version())
	if stored.Effective() != old.Effective() || stored.ContentDigest() != old.ContentDigest() || stored.Status() != domain.CommercialVersionEffective {
		t.Fatal("截止改写了旧版的原区间、正文或生命周期位置")
	}

	for name, probe := range map[string]struct {
		at   time.Time
		want domain.CommercialVersionLabel
	}{
		"后继生效之后得新版":  {at: anchorAt, want: next.Version()},
		"后继生效那一刻得新版": {at: cutSuccessorStart, want: next.Version()},
		"后继生效之前得旧版":  {at: beforeCut, want: old.Version()},
	} {
		t.Run(name, func(t *testing.T) {
			got := domain.ResolveCommercialBasis(registry, resolutionKeyAt(t, "scope-a", domain.CustomerContractObject, probe.at), nil)
			if got.Outcome() != domain.UniquelyResolved {
				t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", got.Outcome())
			}
			adopted, _ := got.AdoptedVersion()
			if adopted.Version() != probe.want {
				t.Fatalf("adopted = %q, want %q", adopted.Version(), probe.want)
			}
		})
	}
}

// Covers: `AT-PC-056` 截止那一半「同一次截止重放 → 认出册上那一条，不追加，修订标识不变」。
//
// 第一次截止之后旧版的终点就是截止点；重放若先过区间检查，会被读成越界。换一份更正依据再截同一处则不是重放：
// 旧版已在那一刻截止，答越界。
func TestReplayingACutRecognisesTheRegisteredOne(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
	next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)
	first, _ := mustCut(t, registry, old, next, "SYN-CUT-1")
	revision, err := registry.RegisterValidityCorrection(first)
	if err != nil {
		t.Fatalf("register correction: %v", err)
	}

	again, replay := mustCut(t, registry, old, next, "SYN-CUT-1")
	if !replay {
		t.Fatal("同一次截止重放没有被认出")
	}
	if again.CorrectedInterval() != first.CorrectedInterval() || again.Reference() != first.Reference() {
		t.Fatal("重放交回的不是册上那一条")
	}
	if replayed, _ := registry.RegisterValidityCorrection(again); replayed.String() != revision.String() {
		t.Fatal("重放推动了修订标识")
	}

	_, _, err = registry.CutAtSuccessorStart(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), next.Version(),
		commercialValue(t, domain.NewValidityCorrectionReference, "SYN-CUT-OTHER"), cutCorrectedAt)
	if !errors.Is(err, domain.ErrSuccessorCutOutsideInterval) {
		t.Fatalf("换依据再截同一处 err = %v, want ErrSuccessorCutOutsideInterval", err)
	}
}

// Covers: `AT-PC-057` 截止那一半「收尾不成立（后继不晚于旧版起点、不早于旧版终点、后继比旧版先到头、后继不是同一
// 对象的另一版、任一版未生效、后继就是旧版）→ 一条更正都不构造」。
func TestACutThatDoesNotHoldIsRefused(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *domain.CommercialRegistry) (old domain.CommercialVersion, successor domain.CommercialVersionLabel)
		want    error
	}{
		"后继与旧版同一起点：走同起点退役，不是截止": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutPredecessorStart)
				return old, next.Version()
			},
			want: domain.ErrSuccessorCutOutsideInterval,
		},
		"后继早于旧版起点": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)
				next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				return old, next.Version()
			},
			want: domain.ErrSuccessorCutOutsideInterval,
		},
		"后继不早于旧版终点：两版本来不重叠": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionBetween(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart, cutSuccessorStart)
				next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)
				return old, next.Version()
			},
			want: domain.ErrSuccessorCutOutsideInterval,
		},
		"后继只在另一对象下": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				other := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-2", "v2", "scope-a", cutSuccessorStart)
				return old, other.Version()
			},
			want: domain.ErrClosureVersionNotRegistered,
		},
		"后继只在另一范围下": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				other := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-b", cutSuccessorStart)
				return old, other.Version()
			},
			want: domain.ErrClosureVersionNotRegistered,
		},
		"后继已计划生效、尚未取效": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				planned := plannedVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)
				return old, planned.Version()
			},
			want: domain.ErrClosureNotEffective,
		},
		"后继就是旧版": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				return old, old.Version()
			},
			want: domain.ErrClosureSameVersion,
		},
		"后继比旧版先到头：截掉的后一段没有人接": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				next := liveVersionBetween(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart, anchorAt)
				return old, next.Version()
			},
			want: domain.ErrClosureLeavesGap,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			registry := domain.NewCommercialRegistry()
			old, successor := tc.arrange(t, registry)
			_, _, err := registry.CutAtSuccessorStart(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), successor,
				commercialValue(t, domain.NewValidityCorrectionReference, "SYN-CUT-1"), cutCorrectedAt)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if _, held := registry.ValidityCorrectionOf(old.Tenant(), old.Kind(), old.ObjectID(), old.Version()); held {
				t.Fatal("被拒的截止在册上留下了更正")
			}
		})
	}

	t.Run("更正时刻缺席", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
		next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)
		_, _, err := registry.CutAtSuccessorStart(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), next.Version(),
			commercialValue(t, domain.NewValidityCorrectionReference, "SYN-CUT-1"), time.Time{})
		if !errors.Is(err, domain.ErrValidityCorrectionInvalid) {
			t.Fatalf("err = %v, want ErrValidityCorrectionInvalid——更正时刻不补成此刻", err)
		}
	})
}

// Covers: `AT-PC-054` 政策正文那一半「自带区间的政策正文（结算、信用、价格）重叠成`适用冲突`，按后继截止旧版 → 选候选
// 同样按截止点分开；后继正文不从截止点开始的截止拒；版本没有更正时只按正文自己的区间选」。
func TestACutNarrowsPolicyBodiesThatCarryTheirOwnInterval(t *testing.T) {
	t.Run("结算政策", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		old := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", cutPredecessorStart)
		next := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v2", "scope-a", cutSuccessorStart)
		registerSettlementBody(t, registry, old, old.Effective())
		registerSettlementBody(t, registry, next, next.Effective())
		assertCutSplitsPolicy(t, registry, old, next, domain.SettlementPolicyObject)
	})

	t.Run("信用政策", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		old := liveVersionFrom(t, registry, domain.CreditPolicyObject, "credit-1", "v1", "scope-a", cutPredecessorStart)
		next := liveVersionFrom(t, registry, domain.CreditPolicyObject, "credit-1", "v2", "scope-a", cutSuccessorStart)
		limit, err := domain.NewCreditAmountLimit(100000)
		if err != nil {
			t.Fatalf("credit limit: %v", err)
		}
		for _, version := range []domain.CommercialVersion{old, next} {
			policy, err := domain.NewCreditPolicy(version,
				commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
				commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
				commercialValue(t, domain.NewChargeTypeReference, "charge-freight"),
				limit, version.Effective())
			if err != nil {
				t.Fatalf("new credit policy: %v", err)
			}
			registry.RegisterCreditPolicy(policy)
		}
		assertCutSplitsPolicy(t, registry, old, next, domain.CreditPolicyObject)
	})

	t.Run("价格政策", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		old := liveVersionFrom(t, registry, domain.PriceRuleObject, "price-1", "v1", "scope-a", cutPredecessorStart)
		next := liveVersionFrom(t, registry, domain.PriceRuleObject, "price-1", "v2", "scope-a", cutSuccessorStart)
		for _, version := range []domain.CommercialVersion{old, next} {
			policy, err := domain.NewCommercialPricePolicy(version, domain.SellDirection,
				commercialValue(t, domain.NewPricingPlanReference, "plan-"+version.Version().String()),
				domain.SellDirection, domain.PlanBindingConversionNone,
				commercialValue(t, domain.NewCommercialScopeReference, "scope-a"), version.Effective())
			if err != nil {
				t.Fatalf("new price policy: %v", err)
			}
			registry.RegisterPricePolicy(policy)
		}
		assertCutSplitsPolicy(t, registry, old, next, domain.PriceRuleObject)
	})

	t.Run("后继正文起点早于壳：截在壳上仍会重叠，拒", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		old := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", cutPredecessorStart)
		next := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v2", "scope-a", cutSuccessorStart)
		registerSettlementBody(t, registry, old, old.Effective())
		registerSettlementBody(t, registry, next, mustInterval(t))
		_, _, err := registry.CutAtSuccessorStart(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), next.Version(),
			commercialValue(t, domain.NewValidityCorrectionReference, "SYN-CUT-POLICY"), cutCorrectedAt)
		if !errors.Is(err, domain.ErrSuccessorCutBodyStartsElsewhere) {
			t.Fatalf("err = %v, want ErrSuccessorCutBodyStartsElsewhere", err)
		}
	})

	t.Run("没有更正时正文区间照旧是唯一判据", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		version := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", cutSuccessorStart)
		registerSettlementBody(t, registry, version, mustInterval(t))
		got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, domain.SettlementPolicyObject, beforeCut), nil)
		if got.Outcome() != domain.UniquelyResolved {
			t.Fatalf("正文区间覆盖、壳区间未覆盖的锚点 outcome = %q, want UNIQUELY_RESOLVED——壳的原区间不收窄正文", got.Outcome())
		}
	})
}

// assertCutSplitsPolicy：截止之前同一锚点答`适用冲突`；截止之后按截止点两边各唯一。
func assertCutSplitsPolicy(t *testing.T, registry *domain.CommercialRegistry, old, next domain.CommercialVersion, kind domain.CommercialObjectKind) {
	t.Helper()
	if got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, kind, anchorAt), adoptable); got.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("截止前 outcome = %q, want APPLICABILITY_CONFLICT", got.Outcome())
	}
	correction, _ := mustCut(t, registry, old, next, "SYN-CUT-POLICY")
	if _, err := registry.RegisterValidityCorrection(correction); err != nil {
		t.Fatalf("register correction: %v", err)
	}
	for at, want := range map[time.Time]domain.CommercialVersionLabel{anchorAt: next.Version(), beforeCut: old.Version()} {
		got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, kind, at), adoptable)
		if got.Outcome() != domain.UniquelyResolved {
			t.Fatalf("at %v outcome = %q, want UNIQUELY_RESOLVED", at, got.Outcome())
		}
		adopted, _ := got.AdoptedVersion()
		if adopted.Version() != want {
			t.Fatalf("at %v adopted = %q, want %q", at, adopted.Version(), want)
		}
	}
}

func registerSettlementBody(t *testing.T, registry *domain.CommercialRegistry, version domain.CommercialVersion, effective domain.EffectiveInterval) {
	t.Helper()
	applies, err := domain.NewSettlementApplicability(
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCounterpartyReference, "customer-1"),
		fixtureContractLabel(t),
		commercialValue(t, domain.NewChargeScopeReference, "charge-express"),
		commercialValue(t, domain.NewCurrencyCode, "SYN"),
		effective,
	)
	if err != nil {
		t.Fatalf("new settlement applicability: %v", err)
	}
	policy, err := domain.NewSettlementPolicy(version, domain.PrepaidMethod, applies)
	if err != nil {
		t.Fatalf("new settlement policy: %v", err)
	}
	registry.RegisterSettlementPolicy(policy)
}

func adoptable(domain.PricingPlanReference) domain.PricingPlanStanding {
	return domain.PricingPlanAdoptable
}

func policyKeyAt(t *testing.T, kind domain.CommercialObjectKind, at time.Time) domain.ResolutionKey {
	t.Helper()
	key := resolutionKeyAt(t, "scope-a", kind, at)
	if kind == domain.PriceRuleObject {
		key.Purpose = domain.PricingPurpose
		key.PriceDirection = domain.SellDirection
	}
	return key
}

func mustCut(t *testing.T, registry *domain.CommercialRegistry, old, next domain.CommercialVersion, reference string) (domain.ValidityCorrection, bool) {
	t.Helper()
	correction, replay, err := registry.CutAtSuccessorStart(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), next.Version(),
		commercialValue(t, domain.NewValidityCorrectionReference, reference), cutCorrectedAt)
	if err != nil {
		t.Fatalf("cut at successor start: %v", err)
	}
	return correction, replay
}

func resolutionKeyAt(t *testing.T, scope string, basis domain.CommercialObjectKind, at time.Time) domain.ResolutionKey {
	t.Helper()
	key := resolutionKey(t, scope, basis)
	anchor, err := domain.NewSelectionAnchor(at, commercialValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	key.Anchor = anchor
	return key
}

func liveVersionFrom(t *testing.T, registry *domain.CommercialRegistry, kind domain.CommercialObjectKind, objectID, label, scope string, startsAt time.Time) domain.CommercialVersion {
	t.Helper()
	return liveVersionBetween(t, registry, kind, objectID, label, scope, startsAt, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
}

func liveVersionBetween(t *testing.T, registry *domain.CommercialRegistry, kind domain.CommercialObjectKind, objectID, label, scope string, startsAt, endsAt time.Time) domain.CommercialVersion {
	t.Helper()
	published := publishedCutVersion(t, kind, objectID, label, scope, startsAt, endsAt)
	live, err := published.TakeEffect(startsAt.Add(48 * time.Hour))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}

func plannedVersionFrom(t *testing.T, registry *domain.CommercialRegistry, kind domain.CommercialObjectKind, objectID, label, scope string, startsAt time.Time) domain.CommercialVersion {
	t.Helper()
	planned := publishedCutVersion(t, kind, objectID, label, scope, startsAt, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, err := registry.Register(planned); err != nil {
		t.Fatalf("register: %v", err)
	}
	return planned
}

func publishedCutVersion(t *testing.T, kind domain.CommercialObjectKind, objectID, label, scope string, startsAt, endsAt time.Time) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, kind, objectID, label, "sha256:"+objectID+"-"+label)
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, scope)
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	spec.Effective = interval
	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(approval(t, "approval-"+objectID+"-"+label), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return published
}

var retiredAtRegistration = time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)

// Covers: `AT-PC-055`「同一对象同起点重发、两版重叠成`适用冲突`，按决定引用退役旧版 → 旧版成`已退役`，原正文、原区间与
// 批准保留，退役关系与决定引用留在册上，修订标识推进；此后任何锚点都只选后继；自带区间的政策正文随壳落选」。
func TestSameStartRetirementLeavesOnlyTheSuccessor(t *testing.T) {
	t.Run("客户合同", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
		next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutPredecessorStart)
		if got := domain.ResolveCommercialBasis(registry, resolutionKeyAt(t, "scope-a", domain.CustomerContractObject, anchorAt), nil); got.Outcome() != domain.ApplicabilityConflict {
			t.Fatalf("同起点重发未收尾时 outcome = %q, want APPLICABILITY_CONFLICT", got.Outcome())
		}
		before := registry.ViewRevision(old.Tenant(), old.Scope())
		retirement := mustRetire(t, registry, old, next, "SYN-REISSUE-1")
		if retirement.Successor() != next.Version() || !retirement.RetiredAt().Equal(retiredAtRegistration) {
			t.Fatalf("退役 = %+v", retirement)
		}
		after, err := registry.RegisterRetirement(retirement)
		if err != nil {
			t.Fatalf("register retirement: %v", err)
		}
		if after.String() == before.String() {
			t.Fatal("退役后修订标识没有推进")
		}
		stored, _ := registry.Lookup(old.Tenant(), old.Kind(), old.ObjectID(), old.Version())
		reference, retired := stored.RetirementReference()
		if stored.Status() != domain.CommercialVersionRetired || !retired || reference != retirement.Reference() {
			t.Fatalf("旧版 status = %q, reference = %q", stored.Status(), reference)
		}
		if stored.Effective() != old.Effective() || stored.ContentDigest() != old.ContentDigest() {
			t.Fatal("退役改写了旧版的原区间或正文")
		}
		for _, at := range []time.Time{anchorAt, beforeCut} {
			got := domain.ResolveCommercialBasis(registry, resolutionKeyAt(t, "scope-a", domain.CustomerContractObject, at), nil)
			adopted, _ := got.AdoptedVersion()
			if got.Outcome() != domain.UniquelyResolved || adopted.Version() != next.Version() {
				t.Fatalf("at %v outcome = %q adopted = %q, want 唯一解析到后继", at, got.Outcome(), adopted.Version())
			}
		}
	})

	t.Run("结算政策", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		old := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", cutPredecessorStart)
		next := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v2", "scope-a", cutPredecessorStart)
		registerSettlementBody(t, registry, old, old.Effective())
		registerSettlementBody(t, registry, next, next.Effective())
		if got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, domain.SettlementPolicyObject, anchorAt), nil); got.Outcome() != domain.ApplicabilityConflict {
			t.Fatalf("退役前 outcome = %q, want APPLICABILITY_CONFLICT", got.Outcome())
		}
		if _, err := registry.RegisterRetirement(mustRetire(t, registry, old, next, "SYN-REISSUE-1")); err != nil {
			t.Fatalf("register retirement: %v", err)
		}
		got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, domain.SettlementPolicyObject, anchorAt), nil)
		adopted, _ := got.AdoptedVersion()
		if got.Outcome() != domain.UniquelyResolved || adopted.Version() != next.Version() {
			t.Fatalf("退役后 outcome = %q adopted = %q, want 唯一解析到后继", got.Outcome(), adopted.Version())
		}
	})
}

// Covers: `AT-PC-056` 退役那一半「同一个退役决定重放 → 交回册上那一次（原退役时刻），不重登、修订不变；换一份决定引用
// 或换一个后继再退 → 拒，不改写这一段史实」。
func TestReplayingARetirementRecognisesTheRegisteredOne(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
	next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutPredecessorStart)
	other := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v3", "scope-a", cutPredecessorStart)
	first := mustRetire(t, registry, old, next, "SYN-REISSUE-1")
	revision, err := registry.RegisterRetirement(first)
	if err != nil {
		t.Fatalf("register retirement: %v", err)
	}

	again, replay, err := registry.RetireAtSameStartSuccessor(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), next.Version(),
		commercialValue(t, domain.NewRetirementReference, "SYN-REISSUE-1"), retiredAtRegistration.Add(time.Hour))
	if err != nil || !replay || !again.RetiredAt().Equal(first.RetiredAt()) {
		t.Fatalf("重放 replay=%v err=%v at=%v, want 认出册上那一次与原时刻", replay, err, again.RetiredAt())
	}
	if replayed, _ := registry.RegisterRetirement(again); replayed.String() != revision.String() {
		t.Fatal("重放推动了修订标识")
	}
	for name, probe := range map[string]struct {
		successor domain.CommercialVersionLabel
		reference string
	}{
		"换一份决定引用": {successor: next.Version(), reference: "SYN-REISSUE-OTHER"},
		"换一个后继":   {successor: other.Version(), reference: "SYN-REISSUE-1"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := registry.RetireAtSameStartSuccessor(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), probe.successor,
				commercialValue(t, domain.NewRetirementReference, probe.reference), retiredAtRegistration)
			if !errors.Is(err, domain.ErrVersionRetiredOtherwise) {
				t.Fatalf("err = %v, want ErrVersionRetiredOtherwise", err)
			}
		})
	}
}

// Covers: `AT-PC-057` 退役那一半「同起点退役不成立（后继晚于旧版起点、后继比旧版先到头、后继未生效、后继不是同一对象的
// 另一版、登记时刻早于旧版取效）→ 不构造退役，旧版仍在已生效」。
func TestASameStartRetirementThatDoesNotHoldIsRefused(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel)
		at      time.Time
		want    error
	}{
		"后继晚于旧版起点：走截止": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)
				return old, next.Version()
			},
			want: domain.ErrSameStartRetirementStartsDiffer,
		},
		"后继比旧版先到头": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				next := liveVersionBetween(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutPredecessorStart, anchorAt)
				return old, next.Version()
			},
			want: domain.ErrClosureLeavesGap,
		},
		"后继已计划生效、尚未取效": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				planned := plannedVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutPredecessorStart)
				return old, planned.Version()
			},
			want: domain.ErrClosureNotEffective,
		},
		"后继只在另一对象下": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				other := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-2", "v2", "scope-a", cutPredecessorStart)
				return old, other.Version()
			},
			want: domain.ErrClosureVersionNotRegistered,
		},
		"登记时刻早于旧版取效": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				next := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutPredecessorStart)
				return old, next.Version()
			},
			at:   cutPredecessorStart,
			want: domain.ErrInvalidCommercialTransition,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			registry := domain.NewCommercialRegistry()
			old, successor := tc.arrange(t, registry)
			at := tc.at
			if at.IsZero() {
				at = retiredAtRegistration
			}
			_, _, err := registry.RetireAtSameStartSuccessor(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), successor,
				commercialValue(t, domain.NewRetirementReference, "SYN-REISSUE-1"), at)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if stored, _ := registry.Lookup(old.Tenant(), old.Kind(), old.ObjectID(), old.Version()); stored.Status() != domain.CommercialVersionEffective {
				t.Fatalf("被拒的退役改了旧版的生命周期位置：%q", stored.Status())
			}
		})
	}
}

// Covers: `AT-PC-058` 领域那一半「供应商协议正文折进壳上的区间更正与同起点退役 → 截止点之前支持采购、之后不支持；退役
// 之后一律不支持，终止时刻读作退役时刻；别一版的更正或退役折不进来」。
func TestASupplierAgreementBodyFoldsTheVersionClosure(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	old := liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v1", "scope-a", cutPredecessorStart)
	next := liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v2", "scope-a", cutSuccessorStart)
	sameStart := liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v3", "scope-a", cutPredecessorStart)
	agreement := supplierAgreementOf(t, old)
	if !agreement.SupportsProcurementAt(anchorAt) {
		t.Fatal("未收尾的协议在区间内不支持采购")
	}

	correction, _ := mustCut(t, registry, old, next, "SYN-CUT-AGREEMENT")
	narrowed, err := agreement.NarrowedBy(correction)
	if err != nil {
		t.Fatalf("narrowed by: %v", err)
	}
	if narrowed.SupportsProcurementAt(anchorAt) || !narrowed.SupportsProcurementAt(beforeCut) {
		t.Fatal("截止后的协议没有按截止点分开支不支持采购")
	}
	if narrowed.Effective() != agreement.Effective() {
		t.Fatal("折进更正改写了正文自带的区间")
	}

	retirement := mustRetire(t, registry, old, sameStart, "SYN-REISSUE-AGREEMENT")
	ended, err := agreement.EndedBy(retirement)
	if err != nil {
		t.Fatalf("ended by: %v", err)
	}
	if ended.SupportsProcurementAt(beforeCut) || ended.SupportsProcurementAt(anchorAt) {
		t.Fatal("退役后的协议仍支持采购")
	}
	if terminatedAt, terminated := ended.TerminatedAt(); !terminated || !terminatedAt.Equal(retirement.RetiredAt()) {
		t.Fatalf("终止时刻 = %v %v, want 退役时刻", terminatedAt, terminated)
	}

	foreign, _ := mustCut(t, registry, next, liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v4", "scope-a", anchorAt), "SYN-CUT-OTHER")
	if _, err := agreement.NarrowedBy(foreign); !errors.Is(err, domain.ErrInvalidSupplierAgreement) {
		t.Fatalf("折进别一版的更正 err = %v, want ErrInvalidSupplierAgreement", err)
	}
}

func supplierAgreementOf(t *testing.T, version domain.CommercialVersion) domain.SupplierAgreement {
	t.Helper()
	agreement, err := domain.NewSupplierAgreement(version,
		commercialValue(t, domain.NewPartyID, "supplier-1"),
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		commercialValue(t, domain.NewPricingPlanReference, "plan-buy-1"),
		version.Effective())
	if err != nil {
		t.Fatalf("new supplier agreement: %v", err)
	}
	return agreement
}

func mustRetire(t *testing.T, registry *domain.CommercialRegistry, old, next domain.CommercialVersion, reference string) domain.VersionRetirement {
	t.Helper()
	retirement, replay, err := registry.RetireAtSameStartSuccessor(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), next.Version(),
		commercialValue(t, domain.NewRetirementReference, reference), retiredAtRegistration)
	if err != nil || replay {
		t.Fatalf("retire at same start successor: replay=%v err=%v", replay, err)
	}
	return retirement
}

// Covers: `AT-PC-057` 正文那一格（评审补）——同起点退役时后继正文起点晚于旧版正文起点：两版壳同起点，退役放行就会让旧版
// 正文起点到后继正文起点那一段从唯一可选变成无依据，答 ErrClosureLeavesGap，旧版照旧可选。
func TestASameStartRetirementRefusesASuccessorBodyStartingLater(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	old := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", cutPredecessorStart)
	next := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v2", "scope-a", cutPredecessorStart)
	registerSettlementBody(t, registry, old, old.Effective())
	registerSettlementBody(t, registry, next, intervalBetween(t, cutSuccessorStart, closureTestEnd))
	_, _, err := registry.RetireAtSameStartSuccessor(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), next.Version(),
		commercialValue(t, domain.NewRetirementReference, "SYN-REISSUE-1"), retiredAtRegistration)
	if !errors.Is(err, domain.ErrClosureLeavesGap) {
		t.Fatalf("err = %v, want ErrClosureLeavesGap", err)
	}
	if got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, domain.SettlementPolicyObject, beforeCut), nil); got.Outcome() != domain.UniquelyResolved {
		t.Fatalf("被拒之后 outcome = %q, want 旧版照旧唯一可选", got.Outcome())
	}
}

// Covers: `AT-PC-054` 正文那一格（评审补）——旧版正文起点早于壳起点（发布允许）时截止只截终点：截止点之后答新版，正文起点
// 到壳起点那一段照旧唯一选中旧版，不被更正的起点裁掉。
func TestACutLeavesTheFrontOfABodyStartingBeforeItsShell(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	shellStart := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	old := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", shellStart)
	next := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v2", "scope-a", cutSuccessorStart)
	registerSettlementBody(t, registry, old, intervalBetween(t, cutPredecessorStart, closureTestEnd))
	registerSettlementBody(t, registry, next, next.Effective())
	correction, _ := mustCut(t, registry, old, next, "SYN-CUT-FRONT")
	if _, err := registry.RegisterValidityCorrection(correction); err != nil {
		t.Fatal(err)
	}
	early := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	for at, want := range map[time.Time]domain.CommercialVersionLabel{early: old.Version(), beforeCut: old.Version(), anchorAt: next.Version()} {
		got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, domain.SettlementPolicyObject, at), nil)
		adopted, _ := got.AdoptedVersion()
		if got.Outcome() != domain.UniquelyResolved || adopted.Version() != want {
			t.Fatalf("at %v outcome = %q adopted = %q, want %q", at, got.Outcome(), adopted.Version(), want)
		}
	}
}

// Covers: 「接得住」按此刻还能选中的那一段比（评审补）——旧版先被截止过、正文开放结束，再同起点退役给一版区间正好是截止后
// 那一段的后继：放行，不因正文原区间更长而误拒。
func TestATakeOverIsJudgedOnWhatTheResolutionStillSelects(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	old := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", cutPredecessorStart)
	later := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v2", "scope-a", cutSuccessorStart)
	registerSettlementBody(t, registry, old, intervalBetween(t, cutPredecessorStart, time.Time{}))
	registerSettlementBody(t, registry, later, intervalBetween(t, cutSuccessorStart, time.Time{}))
	correction, _ := mustCut(t, registry, old, later, "SYN-CUT-1")
	if _, err := registry.RegisterValidityCorrection(correction); err != nil {
		t.Fatal(err)
	}
	reissue := liveVersionBetween(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1b", "scope-a", cutPredecessorStart, cutSuccessorStart)
	registerSettlementBody(t, registry, reissue, reissue.Effective())
	retirement := mustRetire(t, registry, old, reissue, "SYN-REISSUE-1")
	if _, err := registry.RegisterRetirement(retirement); err != nil {
		t.Fatal(err)
	}
	got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, domain.SettlementPolicyObject, beforeCut), nil)
	adopted, _ := got.AdoptedVersion()
	if got.Outcome() != domain.UniquelyResolved || adopted.Version() != reissue.Version() {
		t.Fatalf("outcome = %q adopted = %q, want 唯一解析到重发的 v1b", got.Outcome(), adopted.Version())
	}
}

// Covers: `AT-PC-058` 正文起点那一格（评审补）——供应商协议正文起点早于壳起点时，折进截止也只截终点：正文起点到壳起点那一段
// 照旧支持采购。
func TestASupplierAgreementCutKeepsTheBodyFront(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	shellStart := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	old := liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v1", "scope-a", shellStart)
	next := liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v2", "scope-a", cutSuccessorStart)
	agreement, err := domain.NewSupplierAgreement(old,
		commercialValue(t, domain.NewPartyID, "supplier-1"),
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		commercialValue(t, domain.NewPricingPlanReference, "plan-buy-1"),
		intervalBetween(t, cutPredecessorStart, closureTestEnd))
	if err != nil {
		t.Fatal(err)
	}
	correction, _ := mustCut(t, registry, old, next, "SYN-CUT-AGREEMENT")
	narrowed, err := agreement.NarrowedBy(correction)
	if err != nil {
		t.Fatal(err)
	}
	early := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !narrowed.SupportsProcurementAt(early) || narrowed.SupportsProcurementAt(anchorAt) {
		t.Fatal("截止后的协议没有只截终点")
	}
}

var closureTestEnd = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

func intervalBetween(t *testing.T, startsAt, endsAt time.Time) domain.EffectiveInterval {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return interval
}

// Covers: 选用取登记顺序上的最后一条更正（ADR-0056），落在登记册读法上（bodyExcludedAt 与收尾门的 bodySelectionOf 经
// correctionEndOf、versionFold 取截止点）——同一份政策正文版本先截晚（截在 v3）、再截早（截在 v2）：两个截止点之间的
// 时刻旧版正文已落选，解析唯一答 v2；收尾门按较早那个截止点判接得住，正好接到它的重发可以同起点退役旧版。按前一条判，
// 那一刻旧版与 v2 并存成`适用冲突`，重发也会被误判为接不住。
func TestTheLatestOfTwoCutsGovernsThePolicyBodyInTheRegistry(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	lateCut := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	between := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	old := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1", "scope-a", cutPredecessorStart)
	early := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v2", "scope-a", cutSuccessorStart)
	late := liveVersionFrom(t, registry, domain.SettlementPolicyObject, "settlement-1", "v3", "scope-a", lateCut)
	for _, version := range []domain.CommercialVersion{old, early, late} {
		registerSettlementBody(t, registry, version, version.Effective())
	}
	for _, cut := range []struct {
		next      domain.CommercialVersion
		reference string
	}{{late, "SYN-CUT-LATE"}, {early, "SYN-CUT-EARLY"}} {
		correction, _ := mustCut(t, registry, old, cut.next, cut.reference)
		if _, err := registry.RegisterValidityCorrection(correction); err != nil {
			t.Fatalf("register correction %s: %v", cut.reference, err)
		}
	}

	got := domain.ResolveCommercialBasis(registry, policyKeyAt(t, domain.SettlementPolicyObject, between), nil)
	adopted, _ := got.AdoptedVersion()
	if got.Outcome() != domain.UniquelyResolved || adopted.Version() != early.Version() {
		t.Fatalf("两个截止点之间 outcome = %q adopted = %q, want 唯一解析到 v2——旧版正文按较早的截止点落选", got.Outcome(), adopted.Version())
	}

	reissue := liveVersionBetween(t, registry, domain.SettlementPolicyObject, "settlement-1", "v1b", "scope-a", cutPredecessorStart, cutSuccessorStart)
	registerSettlementBody(t, registry, reissue, reissue.Effective())
	if _, _, err := registry.RetireAtSameStartSuccessor(old.Tenant(), old.Scope(), old.Kind(), old.ObjectID(), old.Version(), reissue.Version(),
		commercialValue(t, domain.NewRetirementReference, "SYN-REISSUE-LATEST"), retiredAtRegistration); err != nil {
		t.Fatalf("同起点退役 err = %v——收尾门应按较早那个截止点判，重发接得住", err)
	}
}
