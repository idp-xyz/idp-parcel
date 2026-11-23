package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestJudgeCitedDirectionAuthorizationSplitsTheThreeRefusals(t *testing.T) {
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	open := citedInterval(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	sell := []domain.PriceDirection{domain.SellDirection}

	cases := []struct {
		name       string
		found      bool
		version    domain.CommercialVersion
		selection  domain.EffectiveInterval
		directions []domain.PriceDirection
		want       domain.DirectionAuthorizationStanding
		direction  domain.PriceDirection
	}{
		{name: "不在册", found: false, want: domain.DirectionAuthorizationNotPublished},
		{
			name:  "已发布且时刻在区间内、方向唯一",
			found: true, version: citedVersion(t, domain.CommercialVersionPublished, open, time.Time{}, time.Time{}),
			selection: open, directions: sell,
			want: domain.DirectionAuthorizationAuthorizes, direction: domain.SellDirection,
		},
		{
			name:  "已生效、两版政策方向相同",
			found: true, version: citedVersion(t, domain.CommercialVersionEffective, open, open.StartsAt(), time.Time{}),
			selection: open, directions: []domain.PriceDirection{domain.SellDirection, domain.SellDirection},
			want: domain.DirectionAuthorizationAuthorizes, direction: domain.SellDirection,
		},
		{
			name:  "时刻还没到有效区间",
			found: true, version: citedVersion(t, domain.CommercialVersionPublished, open, time.Time{}, time.Time{}),
			selection:  citedInterval(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Time{}),
			directions: sell, want: domain.DirectionAuthorizationNotInForce,
		},
		{
			name:  "时刻已过有效区间",
			found: true, version: citedVersion(t, domain.CommercialVersionEffective, open, open.StartsAt(), time.Time{}),
			selection:  citedInterval(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)),
			directions: sell, want: domain.DirectionAuthorizationNotInForce,
		},
		{
			name:  "已到期",
			found: true, version: citedClosed(t, domain.CommercialVersionExpired, open),
			selection: open, directions: sell, want: domain.DirectionAuthorizationNotInForce,
		},
		{
			name:  "已退役",
			found: true, version: citedClosed(t, domain.CommercialVersionRetired, open),
			selection: open, directions: sell, want: domain.DirectionAuthorizationNotInForce,
		},
		{
			name:  "已替代",
			found: true, version: citedClosed(t, domain.CommercialVersionSuperseded, open),
			selection: open, directions: sell, want: domain.DirectionAuthorizationNotInForce,
		},
		{
			name:  "在效力内但没指名价格政策",
			found: true, version: citedVersion(t, domain.CommercialVersionEffective, open, open.StartsAt(), time.Time{}),
			selection: open, want: domain.DirectionAuthorizationDirectionUnreadable,
		},
		{
			name:  "指名的政策方向不一致",
			found: true, version: citedVersion(t, domain.CommercialVersionEffective, open, open.StartsAt(), time.Time{}),
			selection: open, directions: []domain.PriceDirection{domain.SellDirection, domain.BuyDirection},
			want: domain.DirectionAuthorizationDirectionUnreadable,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fact, err := domain.JudgeCitedDirectionAuthorization(test.found, test.version, test.selection, at, test.directions)
			if err != nil {
				t.Fatalf("判定：%v", err)
			}
			if fact.Standing() != test.want {
				t.Fatalf("standing = %s，want %s", fact.Standing(), test.want)
			}
			if fact.Standing() == domain.DirectionAuthorizationAuthorizes && fact.Direction() != test.direction {
				t.Fatalf("direction = %s，want %s", fact.Direction(), test.direction)
			}
		})
	}
	if _, err := domain.JudgeCitedDirectionAuthorization(false, domain.CommercialVersion{}, domain.EffectiveInterval{}, time.Time{}, nil); !errors.Is(err, domain.ErrInvalidCommercialVersion) {
		t.Fatalf("时刻为零：%v", err)
	}
}

func TestDirectionAuthorizationStandingNamesEveryValue(t *testing.T) {
	names := map[domain.DirectionAuthorizationStanding]string{
		domain.DirectionAuthorizationNotPublished:        "NOT_PUBLISHED",
		domain.DirectionAuthorizationNotInForce:          "NOT_IN_FORCE",
		domain.DirectionAuthorizationDirectionUnreadable: "DIRECTION_UNREADABLE",
		domain.DirectionAuthorizationAuthorizes:          "AUTHORIZES",
	}
	for standing, want := range names {
		if standing.String() != want {
			t.Fatalf("%s 的名字 = %q", want, standing.String())
		}
	}
	if domain.DirectionAuthorizationStandingInvalid.String() != "" {
		t.Fatal("零值不该有名字")
	}
}

func citedInterval(t *testing.T, startsAt, endsAt time.Time) domain.EffectiveInterval {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	return interval
}

func citedVersion(t *testing.T, status domain.CommercialVersionStatus, interval domain.EffectiveInterval, effectiveAt, closedAt time.Time) domain.CommercialVersion {
	t.Helper()
	spec := domain.RehydrateCommercialVersionSpec{
		TenantID:      commercialValue(t, domain.NewTenantID, "tenant-1"),
		Kind:          domain.AuthorizationRuleObject,
		ObjectID:      commercialValue(t, domain.NewCommercialObjectID, "auth-1"),
		Version:       commercialValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:         commercialValue(t, domain.NewCommercialScopeReference, "scope-1"),
		ContentDigest: commercialValue(t, domain.NewCommercialContentDigest, "digest-auth-1"),
		Effective:     interval,
		Status:        status,
		Approval:      approval(t, "approval-auth-1"),
		PublishedAt:   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		EffectiveAt:   effectiveAt,
		ClosedAt:      closedAt,
		References: map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.PriceRuleObject: commercialValue(t, domain.NewCommercialObjectID, "price-1"),
		},
	}
	if status == domain.CommercialVersionRetired {
		spec.RetirementRef = commercialValue(t, domain.NewRetirementReference, "retire-auth-1")
	}
	if status == domain.CommercialVersionSuperseded {
		spec.Successor = commercialValue(t, domain.NewCommercialVersionLabel, "v2")
	}
	version, err := domain.RehydrateCommercialVersion(spec)
	if err != nil {
		t.Fatalf("重建 %s：%v", status, err)
	}
	return version
}

func citedClosed(t *testing.T, status domain.CommercialVersionStatus, interval domain.EffectiveInterval) domain.CommercialVersion {
	t.Helper()
	return citedVersion(t, status, interval, interval.StartsAt(), interval.StartsAt().Add(24*time.Hour))
}
