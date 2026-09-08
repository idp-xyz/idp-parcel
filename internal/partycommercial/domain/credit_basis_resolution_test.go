package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// creditSelector 与 registerCreditPolicyIn 的正文逐维对齐：键上的等级与费用类型要能命中夹具
// 登记的那份政策（ADR-0127）。法人取键上 LegalEntityCandidate（"legal-1"），时点取锚点。
func creditSelector(t *testing.T) domain.CreditSelector {
	t.Helper()
	return creditSelectorFor(t, "charge-freight")
}

func creditSelectorFor(t *testing.T, chargeType string) domain.CreditSelector {
	t.Helper()
	return domain.CreditSelector{
		Level:      commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		ChargeType: commercialValue(t, domain.NewChargeTypeReference, chargeType),
	}
}

func registerCreditPolicyIn(
	t *testing.T,
	registry *domain.CommercialRegistry,
	scope, objectID, chargeType string,
	limit domain.CreditLimit,
) domain.CreditPolicy {
	t.Helper()
	version := effectiveIn(t, registry, domain.CreditPolicyObject, objectID, "v1", "sha256:"+objectID, scope)
	policy, err := domain.NewCreditPolicy(
		version,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewChargeTypeReference, chargeType),
		limit,
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new credit policy: %v", err)
	}
	registry.RegisterCreditPolicy(policy)
	return policy
}

// Covers: ADR-0127 决定一与三——信用依据经 ResolveCreditPolicy 在同一范围的多份政策之间选，
// 结果带回出自哪一版政策与授权多少额度，不再只剩一份裸版本；不同费用类型各守各的，互不冲突。
func TestCreditBasisIsSelectedAmongPoliciesAndCarriesLimitAndVersion(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerCreditPolicyIn(t, registry, "scope-a", "credit-freight", "charge-freight", creditAmount(t, 500000))
	registerCreditPolicyIn(t, registry, "scope-a", "credit-surcharge", "charge-surcharge", creditAmount(t, 20000))

	expected := map[string]int64{"charge-freight": 500000, "charge-surcharge": 20000}
	identities := map[string]domain.ResolutionID{}
	for chargeType, limit := range expected {
		t.Run(chargeType, func(t *testing.T) {
			key := resolutionKey(t, "scope-a", domain.CreditPolicyObject)
			key.Credit = creditSelectorFor(t, chargeType)

			result := domain.ResolveCommercialBasis(registry, key, nil)
			if result.Outcome() != domain.UniquelyResolved {
				t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（不同费用类型不得互相冲突）", result.Outcome())
			}
			basis, ok := result.AdoptedCreditBasis()
			if !ok || !basis.Applicable() {
				t.Fatal("信用依据解出后额度与出处不可观察——只剩裸版本")
			}
			if minor, ok := basis.AuthorizedLimit().AmountMinor(); !ok || minor != limit {
				t.Fatalf("limit = (%d, %v), want %d", minor, ok, limit)
			}
			adopted, _ := result.AdoptedVersion()
			if basis.PolicyVersion().ObjectID() != adopted.ObjectID() {
				t.Fatalf("basis names %q, adopted version is %q——两者必须是同一版",
					basis.PolicyVersion().ObjectID(), adopted.ObjectID())
			}
			identities[chargeType] = result.ResolutionID()
		})
	}
	if identities["charge-freight"] == identities["charge-surcharge"] {
		t.Fatal("两个费用类型共用同一解析身份，一个类型的结果就能回答另一个类型")
	}
}

// Covers: ADR-0127 决定一——同一（法人、等级、费用类型、时点）被两份政策覆盖是`适用冲突`，
// 不取额度较大或较小的那条（ResolveCreditPolicy 既有纪律在解析入口照旧成立）。
func TestCreditBasisConflictsWhenOneRangeIsCoveredByTwoPolicies(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerCreditPolicyIn(t, registry, "scope-a", "credit-1", "charge-freight", creditAmount(t, 500000))
	registerCreditPolicyIn(t, registry, "scope-a", "credit-2", "charge-freight", creditAmount(t, 900000))

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CreditPolicyObject), nil)
	if result.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", result.Outcome())
	}
	if _, present := result.AdoptedCreditBasis(); present {
		t.Fatal("冲突仍任选了一份额度")
	}
	if result.CandidateCount() != 2 {
		t.Fatalf("candidate count = %d, want 2", result.CandidateCount())
	}
}

// 光有已登记的信用政策**版本**没有正文 → 无适用依据：通用版本解析产不出额度，正是 ADR-0127 要修
// 的洞（镜像 ADR-0044「光有结算政策版本没有政策」）。另一范围的政策同样不作答：候选先按租户与范围收窄。
func TestABareCreditVersionOrAnotherScopesPolicyIsNoBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CreditPolicyObject, "credit-bare", "v1", "sha256:bare", "scope-a")
	registerCreditPolicyIn(t, registry, "scope-b", "credit-elsewhere", "charge-freight", creditAmount(t, 500000))

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CreditPolicyObject), nil)
	if result.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS——裸版本或他范围的政策被当成了可用信用依据", result.Outcome())
	}
	if _, present := result.AdoptedVersion(); present {
		t.Fatal("没有正文却采用了版本")
	}
	if _, present := result.AdoptedCreditBasis(); present {
		t.Fatal("无适用依据却交出了额度——零额度与无政策会读成同一个样子")
	}
}

// 镜像 SettlementSelector / PriceDirection 的键纪律（ADR-0127 决定二）：请求信用依据必须给出选择器，
// 其余请求必须缺席；部分给出同样不受理。
func TestCreditSelectorIsRequiredForCreditAndForbiddenOtherwise(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerCreditPolicyIn(t, registry, "scope-a", "credit-1", "charge-freight", creditAmount(t, 500000))
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	t.Run("credit without a selector is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.CreditPolicyObject)
		key.Credit = domain.CreditSelector{}
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("a partially given selector is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.CreditPolicyObject)
		key.Credit.ChargeType = domain.ChargeTypeReference{}
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("a contract request carrying a selector is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
		key.Credit = creditSelector(t)
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("a closure not asking for credit must not carry a selector", func(t *testing.T) {
		key := closureKey(t, "scope-a", domain.CustomerContractObject)
		key.Credit = creditSelector(t)
		if got := domain.ResolveCommercialClosure(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("a closure asking for credit without a selector is not accepted", func(t *testing.T) {
		key := closureKey(t, "scope-a", domain.CustomerContractObject, domain.CreditPolicyObject)
		key.Credit = domain.CreditSelector{}
		if got := domain.ResolveCommercialClosure(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})
}

// 闭包采用的信用依据同样携带额度与出处：SA 消费的是闭包，不是单依据结果（ADR-0127 决定三）。
// 信用依据没有结算政策那样的前提，与其余成员同段解出；非信用成员不得带上额度。
func TestClosureCarriesTheAdoptedCreditBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	bases := append(append([]domain.CommercialObjectKind(nil), closureBases...), domain.CreditPolicyObject)
	seedClosure(t, registry, "scope-a", bases...)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q, unresolved=%v）",
			closure.Outcome(), closure.Reason(), closure.UnresolvedBases())
	}
	basis, present := closure.AdoptedFor(domain.CreditPolicyObject)
	if !present {
		t.Fatal("闭包没有采用信用依据")
	}
	credit, ok := basis.CreditBasis()
	if !ok || !credit.Applicable() {
		t.Fatal("闭包采用了信用依据却丢了额度——出处与额度不可观察")
	}
	if minor, ok := credit.AuthorizedLimit().AmountMinor(); !ok || minor != 500000 {
		t.Fatalf("limit = (%d, %v), want 500000", minor, ok)
	}
	if credit.PolicyVersion().ObjectID() != basis.Version().ObjectID() {
		t.Fatal("额度出自的政策版本与闭包采用的版本不是同一版")
	}
	if contract, ok := closure.AdoptedFor(domain.CustomerContractObject); !ok {
		t.Fatal("closure lost the contract member")
	} else if _, leaked := contract.CreditBasis(); leaked {
		t.Fatal("非信用成员也带上了信用依据")
	}
	if settlement, ok := closure.AdoptedFor(domain.SettlementPolicyObject); !ok {
		t.Fatal("closure lost the settlement member")
	} else if _, leaked := settlement.CreditBasis(); leaked {
		t.Fatal("结算成员也带上了信用依据")
	}
}

// 同一范围两份政策覆盖同一格时闭包报`适用冲突`并点名信用依据；整份闭包一项都不采用（全有或全无）。
func TestAClosureReportsTheConflictingCreditBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	registerCreditPolicyIn(t, registry, "scope-a", "credit-1", "charge-freight", creditAmount(t, 500000))
	registerCreditPolicyIn(t, registry, "scope-a", "credit-2", "charge-freight", creditAmount(t, 900000))

	bases := append(append([]domain.CommercialObjectKind(nil), closureBases...), domain.CreditPolicyObject)
	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)
	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	conflicting := closure.ConflictingBases()
	if len(conflicting) != 1 || conflicting[0] != domain.CreditPolicyObject {
		t.Fatalf("conflicting = %v, want exactly the credit policy", conflicting)
	}
	if len(closure.Adopted()) != 0 {
		t.Fatal("冲突的闭包仍采用了成员")
	}
}

// 只改正文、不动版本时解析身份仍须变：信用政策参与 ViewRevision 派生（ADR-0127 决定三，镜像 ADR-0044）。
func TestRegisteringACreditPolicyAdvancesTheViewRevision(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	tenant := commercialValue(t, domain.NewTenantID, "tenant-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")

	version := effectiveIn(t, registry, domain.CreditPolicyObject, "credit-1", "v1", "sha256:c1", "scope-a")
	before := registry.ViewRevision(tenant, scope)

	policy, err := domain.NewCreditPolicy(
		version,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewChargeTypeReference, "charge-freight"),
		creditAmount(t, 500000),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new credit policy: %v", err)
	}
	registry.RegisterCreditPolicy(policy)

	if registry.ViewRevision(tenant, scope) == before {
		t.Fatal("登记信用政策没有推进视图修订——先前解析的失效检测看不见它")
	}
	other := commercialValue(t, domain.NewCommercialScopeReference, "scope-b")
	if registry.ViewRevision(tenant, other) != domain.NewCommercialRegistry().ViewRevision(tenant, other) {
		t.Fatal("他范围的视图修订被本范围的信用政策推动了")
	}
}

// Covers: ADR-0028 在信用依据上的那一格——重建门收下快照里整份留存的额度，不回册重读；额度必须挂在
// 信用政策那一格、且已声明，否则整份拒。
func TestRehydratedClosureCarriesOrRefusesTheCreditBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	bases := append(append([]domain.CommercialObjectKind(nil), closureBases...), domain.CreditPolicyObject)
	seedClosure(t, registry, "scope-a", bases...)
	original := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)
	if original.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", original.Outcome())
	}
	revision, _ := original.ViewRevision()

	specOf := func(mutate func(*domain.RehydrateAdoptedBasisSpec)) domain.RehydrateCommercialClosureSpec {
		adopted := make([]domain.RehydrateAdoptedBasisSpec, 0, len(original.Adopted()))
		for _, basis := range original.Adopted() {
			item := domain.RehydrateAdoptedBasisSpec{Kind: basis.Kind(), Version: basis.Version()}
			if policy, ok := basis.SettlementPolicy(); ok {
				item.SettlementPolicy, item.HasSettlementPolicy = policy, true
			}
			if credit, ok := basis.CreditBasis(); ok {
				item.CreditLimit, item.HasCreditBasis = credit.AuthorizedLimit(), true
			}
			if basis.Kind() == domain.CreditPolicyObject && mutate != nil {
				mutate(&item)
			}
			adopted = append(adopted, item)
		}
		return domain.RehydrateCommercialClosureSpec{
			Outcome:      domain.UniquelyResolved,
			ResolutionID: original.ResolutionID(),
			Key:          original.ResolutionKey(),
			Anchor:       original.Anchor(),
			ViewRevision: revision,
			Adopted:      adopted,
		}
	}

	t.Run("carries the limit back", func(t *testing.T) {
		rebuilt, err := domain.RehydrateCommercialClosure(specOf(nil))
		if err != nil {
			t.Fatalf("rehydrate: %v", err)
		}
		basis, ok := rebuilt.AdoptedFor(domain.CreditPolicyObject)
		if !ok {
			t.Fatal("重建后丢了信用成员")
		}
		credit, ok := basis.CreditBasis()
		if !ok || !credit.Applicable() {
			t.Fatal("重建后信用成员丢了额度")
		}
		if minor, ok := credit.AuthorizedLimit().AmountMinor(); !ok || minor != 500000 {
			t.Fatalf("limit = (%d, %v), want 500000", minor, ok)
		}
		if credit.PolicyVersion().ObjectID() != basis.Version().ObjectID() ||
			credit.PolicyVersion().Version() != basis.Version().Version() {
			t.Fatal("重建后的额度出处与采用版本不是同一版")
		}
	})

	t.Run("refuses an undeclared limit", func(t *testing.T) {
		_, err := domain.RehydrateCommercialClosure(specOf(func(item *domain.RehydrateAdoptedBasisSpec) {
			item.CreditLimit = domain.CreditLimit{}
		}))
		if !errors.Is(err, domain.ErrInvalidRehydratedResolution) {
			t.Fatalf("error = %v, want ErrInvalidRehydratedResolution——零值额度会读成授予零信用", err)
		}
	})

	t.Run("refuses a limit hung on another member", func(t *testing.T) {
		spec := specOf(nil)
		for index := range spec.Adopted {
			if spec.Adopted[index].Kind == domain.CustomerContractObject {
				spec.Adopted[index].CreditLimit, spec.Adopted[index].HasCreditBasis = creditAmount(t, 1), true
			}
		}
		if _, err := domain.RehydrateCommercialClosure(spec); !errors.Is(err, domain.ErrInvalidRehydratedResolution) {
			t.Fatalf("error = %v, want ErrInvalidRehydratedResolution", err)
		}
	})
}
