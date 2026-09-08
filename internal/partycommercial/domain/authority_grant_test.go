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

	authorized, err := domain.Authorize(grants, nil, rejectionRequest(t, "level-commercial", "scope-a"))
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

	// 根本不存在任何授权；一段生效中的承运代理关系不得顶替它。这里等的是`未配置`而不是
	// `不允许`：一条规则都没登记时，本上下文还答不了「许不许」——而关系角色照样什么都不带来。
	if _, err := domain.Authorize(nil, nil, rejectionRequest(t, "level-commercial", "scope-a")); !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured", err)
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
		if _, err := domain.Authorize(grants, nil, rejectionRequest(t, "level-clerk", "scope-a")); !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v; a different authority level was accepted", err)
		}
	})

	t.Run("another scope with no rules of its own is unconfigured", func(t *testing.T) {
		// scope-a 的授权不为 scope-b 作答；而 scope-b 一条规则都没有，所以答案是`未配置`。
		if _, err := domain.Authorize(grants, nil, rejectionRequest(t, "level-commercial", "scope-b")); !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
			t.Fatalf("error = %v; a grant answered outside its scope", err)
		}
	})

	t.Run("another scope that has its own rules refuses rather than borrowing", func(t *testing.T) {
		// scope-b 自己有规则（只授人工复核），于是它已被配置：这次主动拒绝落`不允许`。
		// 这一格与上一格分开，才证明得了 scope-a 那条主动拒绝授权没有越界作答——只留上一格时，
		// 「scope-b 未配置」与「scope-a 的授权被借用了」在结果上分不开。
		crossScope := append([]domain.AuthorityGrant{}, grants...)
		crossScope = append(crossScope, authorityGrant(t, "auth-review-b", domain.ManualReviewAction, "level-commercial", "scope-b"))

		if _, err := domain.Authorize(crossScope, nil, rejectionRequest(t, "level-commercial", "scope-b")); !errors.Is(err, domain.ErrNotAuthorized) {
			t.Fatalf("error = %v; scope-a 的主动拒绝授权为 scope-b 作了答", err)
		}
	})

	t.Run("another action is not authorized", func(t *testing.T) {
		reviewGrants := []domain.AuthorityGrant{
			authorityGrant(t, "auth-review", domain.ManualReviewAction, "level-commercial", "scope-a"),
		}
		if _, err := domain.Authorize(reviewGrants, nil, rejectionRequest(t, "level-commercial", "scope-a")); !errors.Is(err, domain.ErrNotAuthorized) {
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
		// 过期与从未登记同落`未配置`：在请求那个时刻，这个范围没有一条管得着的规则，两者要做
		// 的事同为「让一条现行规则存在」。它绝不能是`不允许`——那会把一次续期疏忽说成业务拒绝。
		if _, err := domain.Authorize(grants, nil, late); !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
			t.Fatalf("error = %v; an expired grant still authorized", err)
		}
	})
}

// Covers: CONTEXT「授权只来自版本化的授权规则」与 ADR-0029「结果按消费方的恢复动作分格」。
//
// 「这个范围一条授权规则都没有」与「规则在，但不许你做这件事」此前共用 `ErrNotAuthorized`，
// 而两者的恢复动作相反：前者要租户先把 `PAR-COM-14` 的授权规则登记上，后者是权威已经答过的
// 业务拒绝，再登记也不会变。
//
// 这一条在首发尤其要紧：**没有租户就没有任何授权规则**，于是每一次授权请求都走未配置那一支，
// 却被报成业务拒绝——把一个尚未配置的产品说成「你无权这么做」。
func TestUnconfiguredAuthorityIsNotABusinessRefusal(t *testing.T) {
	configured := []domain.AuthorityGrant{
		authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a"),
	}

	// 规则在，只是请求的等级不在其内：权威已经就这个范围表过态，这是业务拒绝。
	_, refused := domain.Authorize(configured, nil, rejectionRequest(t, "level-clerk", "scope-a"))
	if !errors.Is(refused, domain.ErrNotAuthorized) {
		t.Fatalf("error = %v, want ErrNotAuthorized", refused)
	}

	// 这个范围一条规则都没有：还没配置，不是拒绝。
	_, unconfigured := domain.Authorize(nil, nil, rejectionRequest(t, "level-commercial", "scope-a"))
	if !errors.Is(unconfigured, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured", unconfigured)
	}

	// 两者必须分得开，否则调用方无从选恢复动作——那正是 ADR-0029 要防的错。
	if errors.Is(unconfigured, domain.ErrNotAuthorized) || errors.Is(refused, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatal("未配置与不允许仍然互相 Is，恢复动作因此分不开")
	}
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
