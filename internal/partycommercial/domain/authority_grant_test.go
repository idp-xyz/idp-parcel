package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var authorityAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func authorityGrant(t *testing.T, objectID string, action domain.AuthorizedAction, level, scope string) domain.AuthorityGrant {
	t.Helper()
	version := registerable(t, domain.AuthorizationRuleObject, objectID, "v1", "sha256:"+objectID)
	live, err := version.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	grant, err := domain.NewAuthorityGrant(
		live,
		action,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, level),
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new authority grant: %v", err)
	}
	return grant
}

func rejectionRequest(t *testing.T, level, scope string) domain.AuthorizationRequest {
	t.Helper()
	request, err := domain.NewAuthorizationRequest(
		domain.ActiveRejectionAction,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, level),
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		commercialValue(t, domain.NewStructuredReason, "COMMERCIAL_RISK"),
		commercialValue(t, domain.NewEvidenceReference, "evidence-1"),
		authorityAt,
	)
	if err != nil {
		t.Fatalf("new authorization request: %v", err)
	}
	return request
}

// Covers: CONTEXT「主动拒绝授权必须按责任法人、角色、适用范围和有效期间版本化，并要求
// 结构化原因和证据」。
func TestActiveRejectionNeedsAMatchingVersionedGrant(t *testing.T) {
	grants := []domain.AuthorityGrant{
		authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"),
	}

	authorized, err := domain.Authorize(grants, rejectionRequest(t, "level-commercial", "scope-a"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if authorized.Action() != domain.ActiveRejectionAction {
		t.Fatalf("action = %q, want ACTIVE_REJECTION", authorized.Action())
	}
	if authorized.Reason().String() == "" || authorized.Evidence().String() == "" {
		t.Fatal("an authorization dropped its structured reason or evidence")
	}
	if authorized.GrantVersion().ObjectID().String() != "auth-reject" {
		t.Fatal("the authorization does not name the grant it rests on")
	}
}

// Covers: CONTEXT「渠道服务方或代理商也不能仅凭渠道角色取得资格」— 持有一个参与方角色
// 不授予任何授权，两者是不同的东西。
func TestHoldingAPartyRoleGrantsNoAuthority(t *testing.T) {
	agent := effectiveRelationship(t, "party-1", "operator-1", domain.CarrierAgentRole)
	if !agent.AppliesAt(authorityAt) {
		t.Fatal("fixture relationship is not effective")
	}

	// No grant exists at all; an effective carrier-agent relationship must not
	// substitute for one.
	if _, err := domain.Authorize(nil, rejectionRequest(t, "level-commercial", "scope-a")); !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("error = %v, want ErrNotAuthorized", err)
	}

	requestType := reflect.TypeOf(domain.AuthorizationRequest{})
	for index := 0; index < requestType.NumField(); index++ {
		name := strings.ToLower(requestType.Field(index).Name)
		if strings.Contains(name, "partyrole") || name == "role" {
			t.Fatalf("AuthorizationRequest carries %s, so a party role could confer authority",
				requestType.Field(index).Name)
		}
	}
}

// Covers: CONTEXT — 授权按责任法人、权限等级、适用范围和有效期间版本化，任一不符即不适用。
func TestEveryAuthorityDimensionDiscriminates(t *testing.T) {
	grants := []domain.AuthorityGrant{
		authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"),
	}

	t.Run("another authority level is not authorized", func(t *testing.T) {
		if _, err := domain.Authorize(grants, rejectionRequest(t, "level-clerk", "scope-a")); !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v; a different authority level was accepted", err)
		}
	})

	t.Run("another scope is not authorized", func(t *testing.T) {
		if _, err := domain.Authorize(grants, rejectionRequest(t, "level-commercial", "scope-b")); !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v; a grant answered outside its scope", err)
		}
	})

	t.Run("another action is not authorized", func(t *testing.T) {
		reviewGrants := []domain.AuthorityGrant{
			authorityGrant(t, "auth-review", domain.ManualReviewAction, "level-commercial", "scope-a"),
		}
		if _, err := domain.Authorize(reviewGrants, rejectionRequest(t, "level-commercial", "scope-a")); !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v; a manual-review grant authorized an active rejection", err)
		}
	})

	t.Run("outside the effective interval is not authorized", func(t *testing.T) {
		late, err := domain.NewAuthorizationRequest(
			domain.ActiveRejectionAction,
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
			commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
			commercialValue(t, domain.NewStructuredReason, "COMMERCIAL_RISK"),
			commercialValue(t, domain.NewEvidenceReference, "evidence-1"),
			time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if _, err := domain.Authorize(grants, late); !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v; an expired grant still authorized", err)
		}
	})
}

// Covers: UC-PS-001 步骤 8「人工不得覆盖硬规则」的前置 — 主动拒绝必须带结构化原因与
// 证据，缺任一项都构造不出请求。
func TestActiveRejectionRequiresStructuredReasonAndEvidence(t *testing.T) {
	incomplete := map[string]struct {
		reason   domain.StructuredReason
		evidence domain.EvidenceReference
	}{
		"no reason":   {domain.StructuredReason{}, commercialValue(t, domain.NewEvidenceReference, "evidence-1")},
		"no evidence": {commercialValue(t, domain.NewStructuredReason, "COMMERCIAL_RISK"), domain.EvidenceReference{}},
	}
	for name, missing := range incomplete {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewAuthorizationRequest(
				domain.ActiveRejectionAction,
				commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
				commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
				commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				missing.reason,
				missing.evidence,
				authorityAt,
			); !errors.Is(err, domain.ErrInvalidAuthorizationRequest) {
				t.Fatalf("error = %v, want ErrInvalidAuthorizationRequest", err)
			}
		})
	}
}

// Covers: CONTEXT「人工复核不是默认步骤」— 未被规则显式要求时就是不需要，缺省不得读成
// 需要复核。
func TestManualReviewIsNotRequiredUnlessDeclared(t *testing.T) {
	silent := domain.ManualReviewRequirementFor(nil, commercialValue(t, domain.NewCommercialScopeReference, "scope-a"), authorityAt)
	if silent {
		t.Fatal("an undeclared manual review defaulted to required")
	}

	declared := []domain.AuthorityGrant{
		authorityGrant(t, "auth-review", domain.ManualReviewAction, "level-commercial", "scope-a"),
	}
	if !domain.ManualReviewRequirementFor(declared, commercialValue(t, domain.NewCommercialScopeReference, "scope-a"), authorityAt) {
		t.Fatal("an explicitly declared manual review was not reported as required")
	}
	if domain.ManualReviewRequirementFor(declared, commercialValue(t, domain.NewCommercialScopeReference, "scope-b"), authorityAt) {
		t.Fatal("a manual review requirement leaked into another scope")
	}
}

// Covers: CONTEXT 商业版本共同不变量 — 授权挂在一个当前可用的授权规则版本上。
func TestAuthorityGrantNeedsAUsableAuthorizationRuleVersion(t *testing.T) {
	t.Run("refuses a draft", func(t *testing.T) {
		draft := commercialDraft(t, domain.AuthorizationRuleObject, "auth-x", "v1", "sha256:x")
		if _, err := domain.NewAuthorityGrant(
			draft,
			domain.ActiveRejectionAction,
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
			commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
			mustInterval(t),
		); !errors.Is(err, domain.ErrInvalidAuthorityGrant) {
			t.Fatalf("error = %v, want ErrInvalidAuthorityGrant", err)
		}
	})

	t.Run("refuses another object kind", func(t *testing.T) {
		contract := effectiveVersion(t, "contract-8", "v1", "sha256:c8")
		if _, err := domain.NewAuthorityGrant(
			contract,
			domain.ActiveRejectionAction,
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
			commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
			mustInterval(t),
		); !errors.Is(err, domain.ErrInvalidAuthorityGrant) {
			t.Fatalf("error = %v, want ErrInvalidAuthorityGrant", err)
		}
	})
}
