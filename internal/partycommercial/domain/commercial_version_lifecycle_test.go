package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var (
	effectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	effectiveTo   = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	publishedOn   = time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
)

func publishedVersion(t *testing.T, objectID, version, digest string) domain.CommercialVersion {
	t.Helper()
	published, err := commercialDraft(t, domain.ServiceProductObject, objectID, version, digest).
		Publish(approval(t, "approval-"+objectID+"-"+version), domain.ApprovalRoleConfirmed, publishedOn)
	if err != nil {
		t.Fatalf("publish %s/%s: %v", objectID, version, err)
	}
	return published
}

func effectiveVersion(t *testing.T, objectID, version, digest string) domain.CommercialVersion {
	t.Helper()
	effective, err := publishedVersion(t, objectID, version, digest).TakeEffect(effectiveFrom)
	if err != nil {
		t.Fatalf("take effect %s/%s: %v", objectID, version, err)
	}
	return effective
}

// Covers: party-commercial CONTEXT 已发布 → 已生效 — 到达明确生效边界才生效；以及
// `AT-PC-004`「新版本按生效区间参与新选择」的边界半边。
// 发布不等于生效：一个停在自己边界之前的已发布版本，不得用于新的解析。
func TestPublishedVersionTakesEffectOnlyAtItsBoundary(t *testing.T) {
	published := publishedVersion(t, "product-1", "v1", "sha256:content-1")

	early := effectiveFrom.Add(-time.Second)
	if _, err := published.TakeEffect(early); !errors.Is(err, domain.ErrInvalidCommercialTransition) {
		t.Fatalf("error = %v, want ErrInvalidCommercialTransition before the boundary", err)
	}

	effective, err := published.TakeEffect(effectiveFrom)
	if err != nil {
		t.Fatalf("take effect at the boundary: %v", err)
	}
	if effective.Status() != domain.CommercialVersionEffective {
		t.Fatalf("status = %q, want EFFECTIVE", effective.Status())
	}
	if published.Status() != domain.CommercialVersionPublished {
		t.Fatal("taking effect mutated the published value")
	}

	if _, err := effective.TakeEffect(effectiveFrom); !errors.Is(err, domain.ErrInvalidCommercialTransition) {
		t.Fatalf("an effective version took effect twice: err = %v", err)
	}
	if _, err := commercialDraft(t, domain.ServiceProductObject, "product-2", "v1", "sha256:x").TakeEffect(effectiveFrom); !errors.Is(err, domain.ErrInvalidCommercialTransition) {
		t.Fatalf("a draft took effect without publication: err = %v", err)
	}
}

// Covers: party-commercial CONTEXT 已生效 → 已到期/已退役/已替代 — 三种收尾都停止
// 参与新解析，但都保留历史：内容、批准依据与发布时间不得被收尾动作抹掉。
func TestEndingAnEffectiveVersionPreservesItsHistory(t *testing.T) {
	successor := publishedVersion(t, "product-1", "v2", "sha256:content-2")

	endings := map[string]struct {
		apply func(domain.CommercialVersion) (domain.CommercialVersion, error)
		want  domain.CommercialVersionStatus
	}{
		"expired": {
			apply: func(version domain.CommercialVersion) (domain.CommercialVersion, error) {
				return version.Expire(effectiveTo)
			},
			want: domain.CommercialVersionExpired,
		},
		"retired": {
			apply: func(version domain.CommercialVersion) (domain.CommercialVersion, error) {
				return version.Retire(commercialValue(t, domain.NewRetirementReference, "retire-1"), effectiveFrom.AddDate(0, 3, 0))
			},
			want: domain.CommercialVersionRetired,
		},
		"superseded": {
			apply: func(version domain.CommercialVersion) (domain.CommercialVersion, error) {
				return version.SupersededBy(successor, effectiveFrom.AddDate(0, 4, 0))
			},
			want: domain.CommercialVersionSuperseded,
		},
	}

	for name, ending := range endings {
		t.Run(name, func(t *testing.T) {
			effective := effectiveVersion(t, "product-1", "v1", "sha256:content-1")

			ended, err := ending.apply(effective)
			if err != nil {
				t.Fatalf("end an effective version: %v", err)
			}
			if ended.Status() != ending.want {
				t.Fatalf("status = %q, want %q", ended.Status(), ending.want)
			}
			if effective.Status() != domain.CommercialVersionEffective {
				t.Fatal("ending mutated the effective value")
			}

			if ended.ContentDigest() != effective.ContentDigest() {
				t.Fatal("ending changed the fixed content")
			}
			basis, present := ended.ApprovalBasis()
			if !present || basis != mustBasis(t, effective) {
				t.Fatal("ending dropped the approval basis")
			}
			if _, present := ended.PublishedAt(); !present {
				t.Fatal("ending dropped the publication time")
			}
			if closedAt, present := ended.ClosedAt(); !present || closedAt.Before(effectiveFrom) {
				t.Fatalf("closed at = %v, present = %v", closedAt, present)
			}
		})
	}
}

// Covers: party-commercial CONTEXT 商业版本…与前后版本的关系 — 替代必须指名后继，
// 且后继必须是同一对象的另一版本。
func TestSupersessionNamesASuccessorOfTheSameObject(t *testing.T) {
	effective := effectiveVersion(t, "product-1", "v1", "sha256:content-1")
	supersededAt := effectiveFrom.AddDate(0, 4, 0)

	t.Run("records the successor", func(t *testing.T) {
		successor := publishedVersion(t, "product-1", "v2", "sha256:content-2")
		ended, err := effective.SupersededBy(successor, supersededAt)
		if err != nil {
			t.Fatalf("supersede: %v", err)
		}
		named, present := ended.Successor()
		if !present || named != successor.Version() {
			t.Fatalf("successor = %q present = %v, want v2", named, present)
		}
	})

	t.Run("refuses a successor from another object", func(t *testing.T) {
		other := publishedVersion(t, "product-9", "v1", "sha256:other")
		if _, err := effective.SupersededBy(other, supersededAt); !errors.Is(err, domain.ErrInvalidCommercialTransition) {
			t.Fatalf("error = %v, want a refusal to supersede across objects", err)
		}
	})

	t.Run("refuses a successor of another kind", func(t *testing.T) {
		contract, err := commercialDraft(t, domain.CustomerContractObject, "product-1", "v2", "sha256:contract").
			Publish(approval(t, "approval-contract"), domain.ApprovalRoleConfirmed, publishedOn)
		if err != nil {
			t.Fatalf("publish contract: %v", err)
		}
		if _, err := effective.SupersededBy(contract, supersededAt); !errors.Is(err, domain.ErrInvalidCommercialTransition) {
			t.Fatalf("error = %v, want a refusal to supersede across kinds", err)
		}
	})

	t.Run("refuses itself as successor", func(t *testing.T) {
		if _, err := effective.SupersededBy(effective, supersededAt); !errors.Is(err, domain.ErrInvalidCommercialTransition) {
			t.Fatalf("error = %v, want a refusal to supersede itself", err)
		}
	})
}

// Covers: party-commercial CONTEXT 有效区间 — 开放结束的版本没有到期边界可用，
// 只能被退役或替代；用到期收尾会静默造出一个没有依据的边界。
func TestOpenEndedVersionCannotExpire(t *testing.T) {
	interval, err := domain.NewEffectiveInterval(effectiveFrom, time.Time{})
	if err != nil {
		t.Fatalf("new open interval: %v", err)
	}
	spec := commercialSpec(t, domain.SettlementPolicyObject, "policy-1", "v1", "sha256:policy-1")
	spec.Effective = interval

	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(approval(t, "approval-policy"), domain.ApprovalRoleConfirmed, publishedOn)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	effective, err := published.TakeEffect(effectiveFrom)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}

	if _, err := effective.Expire(effectiveTo); !errors.Is(err, domain.ErrInvalidCommercialTransition) {
		t.Fatalf("error = %v, want a refusal to expire an open-ended version", err)
	}
	if _, err := effective.Retire(commercialValue(t, domain.NewRetirementReference, "retire-policy"), effectiveTo); err != nil {
		t.Fatalf("an open-ended version could not be retired: %v", err)
	}
}

// Covers: party-commercial CONTEXT 已到期/已退役/已替代 停止用于新的解析和判断。
func TestEndedVersionNoLongerAppliesToNewResolution(t *testing.T) {
	effective := effectiveVersion(t, "product-1", "v1", "sha256:content-1")
	within := effectiveFrom.AddDate(0, 2, 0)

	if !effective.AppliesAt(within) {
		t.Fatal("an effective version inside its interval did not apply")
	}
	if effective.AppliesAt(effectiveFrom.Add(-time.Second)) {
		t.Fatal("an effective version applied before its interval")
	}

	retired, err := effective.Retire(commercialValue(t, domain.NewRetirementReference, "retire-2"), within)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if retired.AppliesAt(within) {
		t.Fatal("a retired version still applies to new resolution")
	}
}

func mustBasis(t *testing.T, version domain.CommercialVersion) domain.ApprovalBasis {
	t.Helper()
	basis, present := version.ApprovalBasis()
	if !present {
		t.Fatal("version carries no approval basis")
	}
	return basis
}
