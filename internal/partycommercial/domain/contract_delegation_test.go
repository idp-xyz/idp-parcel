package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func effectiveContract(t *testing.T, objectID string) domain.CommercialVersion {
	t.Helper()
	version := registerable(t, domain.CustomerContractObject, objectID, "v1", "sha256:"+objectID)
	live, err := version.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func customerRequester(t *testing.T, account string) domain.AuthorizationRequester {
	t.Helper()
	requester, err := domain.RequestedByCustomerAccount(commercialValue(t, domain.NewCustomerAccountID, account))
	if err != nil {
		t.Fatalf("customer account requester: %v", err)
	}
	return requester
}

func operatorRequester(t *testing.T, operator, onBehalfOf string) domain.AuthorizationRequester {
	t.Helper()
	requester, err := domain.RequestedByOperatorRole(
		commercialValue(t, domain.NewOperatorRoleReference, operator),
		commercialValue(t, domain.NewCustomerAccountID, onBehalfOf),
	)
	if err != nil {
		t.Fatalf("operator role requester: %v", err)
	}
	return requester
}

func requestBy(
	t *testing.T,
	requester domain.AuthorizationRequester,
	action domain.AuthorizedAction,
	level, scope string,
) domain.AuthorizationRequest {
	t.Helper()
	request, err := domain.NewAuthorizationRequestBy(
		requester,
		action,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, level),
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		commercialValue(t, domain.NewStructuredReason, "CUSTOMER_CORRECTION"),
		commercialValue(t, domain.NewEvidenceReference, "evidence-1"),
		authorityAt,
	)
	if err != nil {
		t.Fatalf("new authorization request: %v", err)
	}
	return request
}

func accountDelegator(t *testing.T, account string) domain.Delegator {
	t.Helper()
	delegator, err := domain.DelegatedByCustomerAccount(commercialValue(t, domain.NewCustomerAccountID, account))
	if err != nil {
		t.Fatalf("customer account delegator: %v", err)
	}
	return delegator
}

func legalEntityDelegator(t *testing.T, entity string) domain.Delegator {
	t.Helper()
	delegator, err := domain.DelegatedByLegalEntity(commercialValue(t, domain.NewLegalEntityReference, entity))
	if err != nil {
		t.Fatalf("legal entity delegator: %v", err)
	}
	return delegator
}

func amendmentDelegation(
	t *testing.T,
	contract domain.CommercialVersion,
	delegator domain.Delegator,
	level, scope string,
	effective domain.EffectiveInterval,
) domain.ContractDelegation {
	t.Helper()
	delegation, err := domain.NewContractDelegation(
		contract,
		delegator,
		domain.SourceDataAmendmentAction,
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		commercialValue(t, domain.NewAuthorityLevel, level),
		effective,
	)
	if err != nil {
		t.Fatalf("new contract delegation: %v", err)
	}
	return delegation
}

// Covers: CONTEXT「接受后客户原始资料的修订是与人工复核、主动拒绝并列的授权动作……三者互不蕴含、
// 不得互相顶替」（ADR-0116 Decision 一）。同范围已有别的动作的规则，说明范围已被表过态：答`不允许`，
// 不答`未配置`。
func TestSourceDataAmendmentIsNotImpliedByTheOtherTwoActions(t *testing.T) {
	customer := customerRequester(t, "account-1")

	t.Run("a rejection grant does not authorize an amendment", func(t *testing.T) {
		grants := []domain.AuthorityGrant{
			authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"),
		}
		_, err := domain.Authorize(grants, nil,
			requestBy(t, customer, domain.SourceDataAmendmentAction, "level-commercial", "scope-a"))
		if !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v, want ErrNotAuthorized", err)
		}
	})

	t.Run("an amendment grant does not authorize a rejection", func(t *testing.T) {
		grants := []domain.AuthorityGrant{
			authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a"),
		}
		_, err := domain.Authorize(grants, nil, rejectionRequest(t, "level-commercial", "scope-a"))
		if !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v, want ErrNotAuthorized", err)
		}
	})

	t.Run("an amendment grant authorizes an amendment", func(t *testing.T) {
		grants := []domain.AuthorityGrant{
			authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a"),
		}
		authorized, err := domain.Authorize(grants, nil,
			requestBy(t, customer, domain.SourceDataAmendmentAction, "level-commercial", "scope-a"))
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		if authorized.Action() != domain.SourceDataAmendmentAction || authorized.Action().String() != "SOURCE_DATA_AMENDMENT" {
			t.Fatalf("action = %q, want SOURCE_DATA_AMENDMENT", authorized.Action())
		}
		if authorized.GrantVersion().ObjectID().String() != "auth-amend" {
			t.Fatal("the authorization does not name the grant it rests on")
		}
	})
}

// Covers: ADR-0116 Decision 三「请求方是客户账户自己 → 决定方 = 请求方」。
func TestACustomerAccountRequestingAmendmentIsItsOwnDecider(t *testing.T) {
	grants := []domain.AuthorityGrant{
		authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a"),
	}
	authorized, err := domain.Authorize(grants, nil,
		requestBy(t, customerRequester(t, "account-1"), domain.SourceDataAmendmentAction, "level-commercial", "scope-a"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	decider, named := authorized.Decider()
	if !named {
		t.Fatal("a customer's own request came back without an actual decider")
	}
	if decider.Kind() != domain.CustomerAccountDecider || decider.Reference() != "account-1" {
		t.Fatalf("decider = %s %q, want CUSTOMER_ACCOUNT account-1", decider.Kind(), decider.Reference())
	}
}

// Covers: CONTEXT「运营角色代客户请求时，实际决定方由该合同版本的合同委派解出，没有委派即无权，
// 登录操作人不能替代实际决定方」；ADR-0116 Decision 三「grant 在场而委派缺席 → ErrNotAuthorized，
// 不是未配置」。
func TestAnOperatorRoleNeedsAContractDelegationToAmendSourceData(t *testing.T) {
	grants := []domain.AuthorityGrant{
		authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a"),
	}
	request := requestBy(t, operatorRequester(t, "operator-1", "account-1"),
		domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
	contract := effectiveContract(t, "contract-1")

	t.Run("without a delegation the request is refused, not unconfigured", func(t *testing.T) {
		_, err := domain.Authorize(grants, nil, request)
		if !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v, want ErrNotAuthorized", err)
		}
		if !errors.Is(err, domain.ErrDelegationAbsent) {
			t.Fatalf("error = %v, want the ErrDelegationAbsent facet so the caller can name what is missing", err)
		}
		if errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
			t.Fatal("a missing delegation was reported as an unconfigured authority rule")
		}
	})

	t.Run("a delegation by the customer account names the account as decider", func(t *testing.T) {
		delegations := []domain.ContractDelegation{
			amendmentDelegation(t, contract, accountDelegator(t, "account-1"), "level-commercial", "scope-a", mustInterval(t)),
		}
		authorized, err := domain.Authorize(grants, delegations, request)
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		decider, named := authorized.Decider()
		if !named || decider.Kind() != domain.CustomerAccountDecider || decider.Reference() != "account-1" {
			t.Fatalf("decider = %s %q (named=%v), want CUSTOMER_ACCOUNT account-1", decider.Kind(), decider.Reference(), named)
		}
	})

	t.Run("a delegation by the legal entity names the legal entity as decider", func(t *testing.T) {
		delegations := []domain.ContractDelegation{
			amendmentDelegation(t, contract, legalEntityDelegator(t, "legal-1"), "level-commercial", "scope-a", mustInterval(t)),
		}
		authorized, err := domain.Authorize(grants, delegations, request)
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		decider, named := authorized.Decider()
		if !named || decider.Kind() != domain.LegalEntityDecider || decider.Reference() != "legal-1" {
			t.Fatalf("decider = %s %q (named=%v), want LEGAL_ENTITY legal-1", decider.Kind(), decider.Reference(), named)
		}
	})
}

// Covers: ADR-0116 Decision 二 —— 委派按（委派方 × 动作 × 范围 × 等级 × 有效区间）成立，任一不符即
// 不解出实际决定方。
func TestEveryDelegationDimensionDiscriminates(t *testing.T) {
	grants := []domain.AuthorityGrant{
		authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a"),
	}
	request := requestBy(t, operatorRequester(t, "operator-1", "account-1"),
		domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
	contract := effectiveContract(t, "contract-1")
	expired, err := domain.NewEffectiveInterval(
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("expired interval: %v", err)
	}

	mismatched := map[string]domain.ContractDelegation{
		"another customer account delegated": amendmentDelegation(t, contract, accountDelegator(t, "account-2"), "level-commercial", "scope-a", mustInterval(t)),
		"another legal entity delegated":     amendmentDelegation(t, contract, legalEntityDelegator(t, "legal-2"), "level-commercial", "scope-a", mustInterval(t)),
		"another scope":                      amendmentDelegation(t, contract, accountDelegator(t, "account-1"), "level-commercial", "scope-b", mustInterval(t)),
		"another authority level":            amendmentDelegation(t, contract, accountDelegator(t, "account-1"), "level-clerk", "scope-a", mustInterval(t)),
		"outside the effective interval":     amendmentDelegation(t, contract, accountDelegator(t, "account-1"), "level-commercial", "scope-a", expired),
	}
	for name, delegation := range mismatched {
		t.Run(name, func(t *testing.T) {
			_, err := domain.Authorize(grants, []domain.ContractDelegation{delegation}, request)
			if !errors.Is(err, domain.ErrDelegationAbsent) {
				t.Fatalf("error = %v; a delegation answered outside its own dimensions", err)
			}
		})
	}
}

// Covers: ADR-0116 Decision 三「grant 缺席照旧 ErrAuthorityRulesNotConfigured」—— 委派回答「谁替谁」，
// 顶替不了「许不许」。
func TestADelegationWithoutAGrantIsStillUnconfigured(t *testing.T) {
	delegations := []domain.ContractDelegation{
		amendmentDelegation(t, effectiveContract(t, "contract-1"), accountDelegator(t, "account-1"), "level-commercial", "scope-a", mustInterval(t)),
	}
	_, err := domain.Authorize(nil, delegations,
		requestBy(t, operatorRequester(t, "operator-1", "account-1"), domain.SourceDataAmendmentAction, "level-commercial", "scope-a"))
	if !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured", err)
	}
}

// Covers: ADR-0116 Decision 三「既有两格动作的裁定也带 Decider：无委派参与时 = 请求方」——
// 主动拒绝是运营角色凭授权规则自己作的决定，委派不参与。
func TestAnOperatorRoleIsTheDeciderOfItsOwnRejection(t *testing.T) {
	grants := []domain.AuthorityGrant{
		authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"),
	}
	authorized, err := domain.Authorize(grants, nil,
		requestBy(t, operatorRequester(t, "operator-1", "account-1"), domain.ActiveRejectionAction, "level-commercial", "scope-a"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	decider, named := authorized.Decider()
	if !named || decider.Kind() != domain.OperatorRoleDecider || decider.Reference() != "operator-1" {
		t.Fatalf("decider = %s %q (named=%v), want OPERATOR_ROLE operator-1", decider.Kind(), decider.Reference(), named)
	}
}

// Covers: 票 pc-gaps/08 签名纪律（三步法）—— 不带请求方的旧构造器原样保留，它裁出的授权没有决定方，
// 且只对既有两格动作成立：资料修订没有请求方就解不出实际决定方，构造时就拒。
func TestALegacyRequestCarriesNoDeciderAndCannotAskForAmendment(t *testing.T) {
	grants := []domain.AuthorityGrant{
		authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"),
	}
	authorized, err := domain.Authorize(grants, nil, rejectionRequest(t, "level-commercial", "scope-a"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, named := authorized.Decider(); named {
		t.Fatal("a request that named no requester came back with an actual decider")
	}

	_, err = domain.NewAuthorizationRequest(
		domain.SourceDataAmendmentAction,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		commercialValue(t, domain.NewStructuredReason, "CUSTOMER_CORRECTION"),
		commercialValue(t, domain.NewEvidenceReference, "evidence-1"),
		authorityAt,
	)
	if !errors.Is(err, domain.ErrAuthorizationRequesterRequired) {
		t.Fatalf("error = %v, want ErrAuthorizationRequesterRequired", err)
	}
}

// Covers: ADR-0116 Decision 二 —— 委派挂在已生效的客户合同版本下，首发只开「资料修订」一格。
func TestAContractDelegationNeedsAnEffectiveCustomerContractAndADelegableAction(t *testing.T) {
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	level := commercialValue(t, domain.NewAuthorityLevel, "level-commercial")

	t.Run("refuses a draft contract", func(t *testing.T) {
		draft := commercialDraft(t, domain.CustomerContractObject, "contract-x", "v1", "sha256:x")
		if _, err := domain.NewContractDelegation(draft, accountDelegator(t, "account-1"),
			domain.SourceDataAmendmentAction, scope, level, mustInterval(t)); !errors.Is(err, domain.ErrInvalidContractDelegation) {
			t.Fatalf("error = %v, want ErrInvalidContractDelegation", err)
		}
	})

	t.Run("refuses another object kind", func(t *testing.T) {
		rulePackage := registerable(t, domain.AcceptanceRulePackageObject, "pack-1", "v1", "sha256:pack")
		live, err := rulePackage.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("take effect: %v", err)
		}
		if _, err := domain.NewContractDelegation(live, accountDelegator(t, "account-1"),
			domain.SourceDataAmendmentAction, scope, level, mustInterval(t)); !errors.Is(err, domain.ErrInvalidContractDelegation) {
			t.Fatalf("error = %v, want ErrInvalidContractDelegation", err)
		}
	})

	t.Run("refuses an action the customer does not own", func(t *testing.T) {
		if _, err := domain.NewContractDelegation(effectiveContract(t, "contract-1"), accountDelegator(t, "account-1"),
			domain.ActiveRejectionAction, scope, level, mustInterval(t)); !errors.Is(err, domain.ErrInvalidContractDelegation) {
			t.Fatalf("error = %v, want ErrInvalidContractDelegation", err)
		}
	})

	t.Run("refuses a zero delegator", func(t *testing.T) {
		if _, err := domain.NewContractDelegation(effectiveContract(t, "contract-1"), domain.Delegator{},
			domain.SourceDataAmendmentAction, scope, level, mustInterval(t)); !errors.Is(err, domain.ErrInvalidContractDelegation) {
			t.Fatalf("error = %v, want ErrInvalidContractDelegation", err)
		}
	})
}

// Covers: ADR-0116 Decision 二「同一合同版本内（动作 × 范围 × 等级）唯一」；至少一条——一条都没有的
// 委派声明什么都没说，不登记。
func TestAContractVersionDeclaresEachDelegationKeyOnce(t *testing.T) {
	contract := effectiveContract(t, "contract-1")

	t.Run("the same key twice is a duplicate even with different delegators", func(t *testing.T) {
		_, err := domain.NewContractDelegationContent(contract, []domain.ContractDelegationDeclaration{
			{Delegator: accountDelegator(t, "account-1"), Action: domain.SourceDataAmendmentAction,
				Scope: commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				Level: commercialValue(t, domain.NewAuthorityLevel, "level-commercial"), Effective: mustInterval(t)},
			{Delegator: legalEntityDelegator(t, "legal-1"), Action: domain.SourceDataAmendmentAction,
				Scope: commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				Level: commercialValue(t, domain.NewAuthorityLevel, "level-commercial"), Effective: mustInterval(t)},
		})
		if !errors.Is(err, domain.ErrDuplicateContractDelegation) {
			t.Fatalf("error = %v, want ErrDuplicateContractDelegation", err)
		}
	})

	t.Run("no delegation at all is not a declaration", func(t *testing.T) {
		if _, err := domain.NewContractDelegationContent(contract, nil); !errors.Is(err, domain.ErrInvalidContractDelegation) {
			t.Fatalf("error = %v, want ErrInvalidContractDelegation", err)
		}
	})

	t.Run("different levels are different delegations, returned in stable order", func(t *testing.T) {
		content, err := domain.NewContractDelegationContent(contract, []domain.ContractDelegationDeclaration{
			{Delegator: accountDelegator(t, "account-1"), Action: domain.SourceDataAmendmentAction,
				Scope: commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				Level: commercialValue(t, domain.NewAuthorityLevel, "level-commercial"), Effective: mustInterval(t)},
			{Delegator: accountDelegator(t, "account-1"), Action: domain.SourceDataAmendmentAction,
				Scope: commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				Level: commercialValue(t, domain.NewAuthorityLevel, "level-clerk"), Effective: mustInterval(t)},
		})
		if err != nil {
			t.Fatalf("new contract delegation content: %v", err)
		}
		delegations := content.Delegations()
		if len(delegations) != 2 {
			t.Fatalf("len = %d, want 2", len(delegations))
		}
		if delegations[0].Level().String() != "level-clerk" || delegations[1].Level().String() != "level-commercial" {
			t.Fatalf("order = %q, %q; want stable order by scope then level", delegations[0].Level(), delegations[1].Level())
		}
		if !content.Contract().SameVersionAs(contract) || !delegations[0].Contract().SameVersionAs(contract) {
			t.Fatal("the content or its delegations lost the owning contract version")
		}
	})
}
