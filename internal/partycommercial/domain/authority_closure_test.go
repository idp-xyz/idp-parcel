package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证授权裁定按所挂版本的收尾判（ADR-0261）：授予随授权规则版本、委派随客户合同版本的退役与截止离开新的裁定；
// 折完收尾之后一个请求命中多于一条授予、或命中委派方不同的多条委派，答`适用冲突`，不取首个。

var (
	closureRuleStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closureCut       = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	closureRuleEnd   = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	closureBefore    = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	closureAfter     = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	closureRetiredAt = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

func closureInterval(t *testing.T, startsAt, endsAt time.Time) domain.EffectiveInterval {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return interval
}

func liveVersion(t *testing.T, kind domain.CommercialObjectKind, objectID, label string) domain.CommercialVersion {
	t.Helper()
	version := registerable(t, kind, objectID, label, "sha256:"+objectID+"-"+label)
	live, err := version.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func rejectionGrantOn(t *testing.T, version domain.CommercialVersion, effective domain.EffectiveInterval) domain.AuthorityGrant {
	t.Helper()
	grant, err := domain.NewAuthorityGrant(
		version,
		domain.ActiveRejectionAction,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		effective,
	)
	if err != nil {
		t.Fatalf("new authority grant: %v", err)
	}
	return grant
}

func rejectionRequestAt(t *testing.T, at time.Time) domain.AuthorizationRequest {
	t.Helper()
	request, err := domain.NewAuthorizationRequest(
		domain.ActiveRejectionAction,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		commercialValue(t, domain.NewStructuredReason, "COMMERCIAL_RISK"),
		commercialValue(t, domain.NewEvidenceReference, "evidence-1"),
		at,
	)
	if err != nil {
		t.Fatalf("new authorization request: %v", err)
	}
	return request
}

func retirementOf(t *testing.T, version domain.CommercialVersion, successor string) domain.VersionRetirement {
	t.Helper()
	retirement, err := version.RetireInFavourOf(
		commercialValue(t, domain.NewCommercialVersionLabel, successor),
		commercialValue(t, domain.NewRetirementReference, "SYN-RETIRE-"+version.ObjectID().String()),
		closureRetiredAt,
	)
	if err != nil {
		t.Fatalf("retire in favour of %s: %v", successor, err)
	}
	return retirement
}

func cutOf(t *testing.T, version domain.CommercialVersion, endsAt time.Time) domain.ValidityCorrection {
	t.Helper()
	correction, err := version.CorrectEffectiveInterval(
		closureInterval(t, closureRuleStart, endsAt),
		commercialValue(t, domain.NewValidityCorrectionReference, "SYN-CUT-"+version.ObjectID().String()),
		closureRetiredAt,
	)
	if err != nil {
		t.Fatalf("correct effective interval: %v", err)
	}
	return correction
}

func assertApplicabilityConflict(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrAuthorityApplicabilityConflict) {
		t.Fatalf("error = %v, want ErrAuthorityApplicabilityConflict", err)
	}
	if errors.Is(err, domain.ErrNotAuthorized) || errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v：适用冲突与不允许、未配置互相 Is，恢复动作分不开（ADR-0029）", err)
	}
}

// Covers: `AT-PC-042` 领域那一半——同一刻两条授予都允许该请求，答适用冲突，不取首个：两条是同一授权规则对象换版
// 未收尾，还是两个对象各授一份，同一答法；交换装载次序答法不变（ADR-0261 决定三）。
func TestSeveralGrantsHittingOneRequestAreAnApplicabilityConflict(t *testing.T) {
	whole := closureInterval(t, closureRuleStart, closureRuleEnd)
	request := rejectionRequestAt(t, closureAfter)

	cases := map[string][]domain.AuthorityGrant{
		"two versions of one rule left unclosed": {
			rejectionGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v1"), whole),
			rejectionGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v2"), whole),
		},
		"two rule objects granting the same": {
			rejectionGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-a", "v1"), whole),
			rejectionGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-b", "v1"), whole),
		},
	}
	for name, grants := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.Authorize(grants, nil, request)
			assertApplicabilityConflict(t, err)
			_, err = domain.Authorize([]domain.AuthorityGrant{grants[1], grants[0]}, nil, request)
			assertApplicabilityConflict(t, err)
		})
	}
}

// Covers: `AT-PC-059` 领域那一半——同起点重发 v2、退役 v1 之后，按两版共同的坐标询问只命中 v2，指名 v2；已退役的 v1
// 不参与，也不使这次询问成为适用冲突（ADR-0261 决定一）。只剩已退役的那一版时，这个范围此刻没有现行规则。
func TestARetiredGrantVersionTakesNoPartInANewAdjudication(t *testing.T) {
	whole := closureInterval(t, closureRuleStart, closureRuleEnd)
	v1 := liveVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v1")
	retired, err := rejectionGrantOn(t, v1, whole).EndedBy(retirementOf(t, v1, "v2"))
	if err != nil {
		t.Fatalf("EndedBy: %v", err)
	}
	successor := rejectionGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v2"), whole)

	for _, at := range []time.Time{closureBefore, closureAfter} {
		authorized, err := domain.Authorize([]domain.AuthorityGrant{retired, successor}, nil, rejectionRequestAt(t, at))
		if err != nil {
			t.Fatalf("at %v authorize: %v", at, err)
		}
		if got := authorized.GrantVersion().Version().String(); got != "v2" {
			t.Fatalf("at %v 采用 %q, want v2——退役只管此后的新裁定，与业务时点无关", at, got)
		}
	}

	_, err = domain.Authorize([]domain.AuthorityGrant{retired}, nil, rejectionRequestAt(t, closureAfter))
	if !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured——已退役的授予不算现行规则", err)
	}
	if retired.InForceAt(closureAfter) {
		t.Fatal("已退役的授予报为现行")
	}
}

// Covers: `AT-PC-060` 领域那一半——旧版截止在后继的生效时点：截止点之前命中旧版，之后只命中后继；截止点之后只剩旧版
// 时这个范围没有现行规则（ADR-0261 决定一）。只截终点：更正的起点晚于授予的起点也不裁授予的前段。
func TestACutGrantVersionNarrowsAtTheCut(t *testing.T) {
	v1 := liveVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v1")
	old := rejectionGrantOn(t, v1, closureInterval(t, closureRuleStart, closureRuleEnd))
	correction, err := v1.CorrectEffectiveInterval(
		closureInterval(t, closureBefore.Add(-24*time.Hour), closureCut),
		commercialValue(t, domain.NewValidityCorrectionReference, "SYN-CUT-auth-reject"),
		closureRetiredAt,
	)
	if err != nil {
		t.Fatalf("correction: %v", err)
	}
	cut, err := old.NarrowedBy(correction)
	if err != nil {
		t.Fatalf("NarrowedBy: %v", err)
	}
	successor := rejectionGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v2"),
		closureInterval(t, closureCut, closureRuleEnd))

	for at, want := range map[time.Time]string{closureBefore: "v1", closureCut: "v2", closureAfter: "v2"} {
		authorized, err := domain.Authorize([]domain.AuthorityGrant{cut, successor}, nil, rejectionRequestAt(t, at))
		if err != nil {
			t.Fatalf("at %v authorize: %v", at, err)
		}
		if got := authorized.GrantVersion().Version().String(); got != want {
			t.Fatalf("at %v 采用 %q, want %q", at, got, want)
		}
	}

	early := closureRuleStart.Add(24 * time.Hour)
	if _, err := domain.Authorize([]domain.AuthorityGrant{cut}, nil, rejectionRequestAt(t, early)); err != nil {
		t.Fatalf("更正起点之前的前段：%v——截止只截终点", err)
	}
	if _, err := domain.Authorize([]domain.AuthorityGrant{cut}, nil, rejectionRequestAt(t, closureAfter)); !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured——截止之后的旧版授予不算现行规则", err)
	}
}

// Covers: ADR-0261 决定一「只截终点」的另一面——开放结束的更正不收窄；收尾只认同一版，挂到别的版本上的退役或更正立不住。
func TestAClosureFoldsOnlyIntoTheVersionItNames(t *testing.T) {
	whole := closureInterval(t, closureRuleStart, closureRuleEnd)
	v1 := liveVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v1")
	other := liveVersion(t, domain.AuthorizationRuleObject, "auth-other", "v1")
	grant := rejectionGrantOn(t, v1, whole)

	if _, err := grant.EndedBy(retirementOf(t, other, "v2")); !errors.Is(err, domain.ErrInvalidAuthorityGrant) {
		t.Fatalf("EndedBy(另一对象的退役) error = %v, want ErrInvalidAuthorityGrant", err)
	}
	if _, err := grant.NarrowedBy(cutOf(t, other, closureCut)); !errors.Is(err, domain.ErrInvalidAuthorityGrant) {
		t.Fatalf("NarrowedBy(另一对象的更正) error = %v, want ErrInvalidAuthorityGrant", err)
	}

	open, err := v1.CorrectEffectiveInterval(
		closureInterval(t, closureRuleStart, time.Time{}),
		commercialValue(t, domain.NewValidityCorrectionReference, "SYN-OPEN"),
		closureRetiredAt,
	)
	if err != nil {
		t.Fatalf("open correction: %v", err)
	}
	unchanged, err := grant.NarrowedBy(open)
	if err != nil {
		t.Fatalf("NarrowedBy(open): %v", err)
	}
	if !unchanged.InForceAt(closureAfter) {
		t.Fatal("开放结束的更正收窄了授予")
	}

	contract := liveVersion(t, domain.CustomerContractObject, "contract-1", "v1")
	delegation := amendmentDelegation(t, contract, accountDelegator(t, "account-1"), "level-commercial", "scope-a", whole)
	if _, err := delegation.EndedBy(retirementOf(t, v1, "v2")); !errors.Is(err, domain.ErrInvalidContractDelegation) {
		t.Fatalf("委派 EndedBy(授权规则的退役) error = %v, want ErrInvalidContractDelegation", err)
	}
	if _, err := delegation.NarrowedBy(cutOf(t, v1, closureCut)); !errors.Is(err, domain.ErrInvalidContractDelegation) {
		t.Fatalf("委派 NarrowedBy(授权规则的更正) error = %v, want ErrInvalidContractDelegation", err)
	}
}

// Covers: ADR-0261 决定三的边界——适用冲突只在「采用哪一版」有分别时答。答案与选哪一条授予无关时照答确定的那一格：
// 客户账户请求只能由运营角色形成的关闭，哪一条授予都改变不了它是`不允许`；运营角色代录而委派缺席，同理。
func TestARefusalThatNoGrantChoiceChangesIsNotAConflict(t *testing.T) {
	whole := closureInterval(t, closureRuleStart, closureRuleEnd)
	closures := []domain.AuthorityGrant{
		closureGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-close-a", "v1"), domain.ControlledClosureAction, whole),
		closureGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-close-b", "v1"), domain.ControlledClosureAction, whole),
	}
	_, err := domain.Authorize(closures, nil,
		requestBy(t, customerRequester(t, "account-1"), domain.ControlledClosureAction, "level-commercial", "scope-a"))
	if !errors.Is(err, domain.ErrCustomerAccountCannotDecide) {
		t.Fatalf("error = %v, want ErrCustomerAccountCannotDecide", err)
	}

	amendments := []domain.AuthorityGrant{
		closureGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-amend-a", "v1"), domain.SourceDataAmendmentAction, whole),
		closureGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-amend-b", "v1"), domain.SourceDataAmendmentAction, whole),
	}
	_, err = domain.Authorize(amendments, nil,
		requestBy(t, operatorRequester(t, "operator-1", "account-1"), domain.SourceDataAmendmentAction, "level-commercial", "scope-a"))
	if !errors.Is(err, domain.ErrDelegationAbsent) {
		t.Fatalf("error = %v, want ErrDelegationAbsent", err)
	}

	_, err = domain.Authorize(amendments, nil,
		requestBy(t, customerRequester(t, "account-1"), domain.SourceDataAmendmentAction, "level-commercial", "scope-a"))
	assertApplicabilityConflict(t, err)
}

func closureGrantOn(t *testing.T, version domain.CommercialVersion, action domain.AuthorizedAction, effective domain.EffectiveInterval) domain.AuthorityGrant {
	t.Helper()
	grant, err := domain.NewAuthorityGrant(
		version,
		action,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		effective,
	)
	if err != nil {
		t.Fatalf("new authority grant: %v", err)
	}
	return grant
}

// Covers: `AT-PC-061` 领域那一半——客户合同版本被同起点退役或截止之后，挂在已退役那一版、或截止点之后的旧版上的委派不再
// 解出实际决定方；后继合同版本声明了同一委派即由它解出，没有即`不允许`（委派缺席）（ADR-0261 决定二）。
func TestADelegationFollowsTheClosureOfItsContractVersion(t *testing.T) {
	whole := closureInterval(t, closureRuleStart, closureRuleEnd)
	grants := []domain.AuthorityGrant{
		closureGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-amend", "v1"), domain.SourceDataAmendmentAction, whole),
	}
	amendAt := func(at time.Time) domain.AuthorizationRequest {
		request, err := domain.NewAuthorizationRequestBy(
			operatorRequester(t, "operator-1", "account-1"),
			domain.SourceDataAmendmentAction,
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
			commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
			commercialValue(t, domain.NewStructuredReason, "CUSTOMER_CORRECTION"),
			commercialValue(t, domain.NewEvidenceReference, "evidence-1"),
			at,
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		return request
	}
	v1 := liveVersion(t, domain.CustomerContractObject, "contract-1", "v1")
	v2 := liveVersion(t, domain.CustomerContractObject, "contract-1", "v2")
	old := amendmentDelegation(t, v1, accountDelegator(t, "account-1"), "level-commercial", "scope-a", whole)

	t.Run("retired", func(t *testing.T) {
		retired, err := old.EndedBy(retirementOf(t, v1, "v2"))
		if err != nil {
			t.Fatalf("EndedBy: %v", err)
		}
		if _, err := domain.Authorize(grants, []domain.ContractDelegation{retired}, amendAt(closureAfter)); !errors.Is(err, domain.ErrDelegationAbsent) {
			t.Fatalf("error = %v, want ErrDelegationAbsent——挂在已退役合同版本上的委派不再解出决定方", err)
		}
		successor := amendmentDelegation(t, v2, legalEntityDelegator(t, "legal-1"), "level-commercial", "scope-a", whole)
		authorized, err := domain.Authorize(grants, []domain.ContractDelegation{retired, successor}, amendAt(closureAfter))
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		if decider, _ := authorized.Decider(); decider.Kind() != domain.LegalEntityDecider || decider.Reference() != "legal-1" {
			t.Fatalf("decider = %s %q, want 后继版本声明的 LEGAL_ENTITY legal-1", decider.Kind(), decider.Reference())
		}
	})

	t.Run("cut", func(t *testing.T) {
		cut, err := old.NarrowedBy(cutOf(t, v1, closureCut))
		if err != nil {
			t.Fatalf("NarrowedBy: %v", err)
		}
		if _, err := domain.Authorize(grants, []domain.ContractDelegation{cut}, amendAt(closureBefore)); err != nil {
			t.Fatalf("截止点之前：%v", err)
		}
		if _, err := domain.Authorize(grants, []domain.ContractDelegation{cut}, amendAt(closureAfter)); !errors.Is(err, domain.ErrDelegationAbsent) {
			t.Fatalf("error = %v, want ErrDelegationAbsent——截止点之后的旧版委派不再解出决定方", err)
		}
	})
}

// Covers: `AT-PC-042` 委派那一半（ADR-0261 决定三）——多条委派命中而委派方不同答适用冲突；委派方相同（同一委派方在换版未收尾的两版合同
// 上各声明一次）即同一个实际决定方，不算冲突。
func TestDelegationsNamingDifferentDelegatorsAreAnApplicabilityConflict(t *testing.T) {
	whole := closureInterval(t, closureRuleStart, closureRuleEnd)
	grants := []domain.AuthorityGrant{
		closureGrantOn(t, liveVersion(t, domain.AuthorizationRuleObject, "auth-amend", "v1"), domain.SourceDataAmendmentAction, whole),
	}
	request := requestBy(t, operatorRequester(t, "operator-1", "account-1"),
		domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
	v1 := liveVersion(t, domain.CustomerContractObject, "contract-1", "v1")
	v2 := liveVersion(t, domain.CustomerContractObject, "contract-1", "v2")

	same := []domain.ContractDelegation{
		amendmentDelegation(t, v1, accountDelegator(t, "account-1"), "level-commercial", "scope-a", whole),
		amendmentDelegation(t, v2, accountDelegator(t, "account-1"), "level-commercial", "scope-a", whole),
	}
	authorized, err := domain.Authorize(grants, same, request)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if decider, _ := authorized.Decider(); decider.Kind() != domain.CustomerAccountDecider || decider.Reference() != "account-1" {
		t.Fatalf("decider = %s %q, want CUSTOMER_ACCOUNT account-1", decider.Kind(), decider.Reference())
	}

	different := []domain.ContractDelegation{
		amendmentDelegation(t, v1, accountDelegator(t, "account-1"), "level-commercial", "scope-a", whole),
		amendmentDelegation(t, v2, legalEntityDelegator(t, "legal-1"), "level-commercial", "scope-a", whole),
	}
	_, err = domain.Authorize(grants, different, request)
	assertApplicabilityConflict(t, err)
}
