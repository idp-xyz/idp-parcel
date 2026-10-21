package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证按后继截止（票 demo-chain-gaps/50）在领域一侧的判据：截出来的是一条区间更正，解析据它在两版之间按时点
// 唯一选择；重放不追加；截止不成立的各格拒；自带区间的政策正文同样被截止收窄。

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

// Covers: `AT-PC-055`「同一次截止重放 → 认出册上那一条，不追加，修订标识不变」。
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

// Covers: `AT-PC-056`「截止不成立（后继不晚于旧版起点、不早于旧版终点、后继不是同一对象的另一版、任一版未生效、
// 后继就是旧版、按版本点读的类别）→ 一条更正都不构造」。
func TestACutThatDoesNotHoldIsRefused(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *domain.CommercialRegistry) (old domain.CommercialVersion, successor domain.CommercialVersionLabel)
		want    error
	}{
		"后继与旧版同一起点：那是替代，不是截止": {
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
			want: domain.ErrSuccessorCutVersionNotRegistered,
		},
		"后继只在另一范围下": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				other := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-b", cutSuccessorStart)
				return old, other.Version()
			},
			want: domain.ErrSuccessorCutVersionNotRegistered,
		},
		"后继已计划生效、尚未取效": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				planned := plannedVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v2", "scope-a", cutSuccessorStart)
				return old, planned.Version()
			},
			want: domain.ErrSuccessorCutNotEffective,
		},
		"后继就是旧版": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.CustomerContractObject, "contract-1", "v1", "scope-a", cutPredecessorStart)
				return old, old.Version()
			},
			want: domain.ErrSuccessorCutSameVersion,
		},
		"供应商协议按版本点读": {
			arrange: func(t *testing.T, registry *domain.CommercialRegistry) (domain.CommercialVersion, domain.CommercialVersionLabel) {
				old := liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v1", "scope-a", cutPredecessorStart)
				next := liveVersionFrom(t, registry, domain.SupplierAgreementObject, "agreement-1", "v2", "scope-a", cutSuccessorStart)
				return old, next.Version()
			},
			want: domain.ErrSuccessorCutKindReadByVersion,
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

// Covers: `AT-PC-057`「自带区间的政策正文（结算、信用、价格）重叠成`适用冲突`，按后继截止旧版 → 选候选同样按截止点
// 分开；后继正文不从截止点开始的截止拒；版本没有更正时只按正文自己的区间选」。
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
