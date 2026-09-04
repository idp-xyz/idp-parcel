package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var evidenceSubmittedAt = time.Date(2026, 8, 12, 19, 0, 0, 0, time.UTC)

func submittedEvidence(t *testing.T) domain.EvidenceItem {
	t.Helper()
	item, err := domain.SubmitEvidence(
		mustValue(t, domain.NewEvidenceItemID, "evidence-1"),
		mustValue(t, domain.NewEvidenceProviderReference, "customer-1"),
		mustValue(t, domain.NewEvidenceContentDigest, "sha256/original-1"),
		evidenceSubmittedAt,
	)
	if err != nil {
		t.Fatalf("submit evidence: %v", err)
	}
	return item
}

// Covers: VE CONTEXT 硬句 171「客户或合作伙伴提交证据只表示材料已经收到，不证明其
// 陈述、事实或责任成立」——评价起点恒为已收到且不由调用方指定（提交即采信构造上
// 不可能）；采信/不采信是显式判断带依据；已评价不再评价（异议走调查不走改写）。
func TestSubmissionMeansReceivedNotCredited(t *testing.T) {
	item := submittedEvidence(t)
	if item.Appraisal() != domain.EvidenceReceived {
		t.Fatalf("appraisal = %q, want RECEIVED", item.Appraisal())
	}
	if _, has := item.AppraisalBasis(); has {
		t.Fatal("刚提交的证据凭空有了评价依据")
	}

	credited, err := item.Appraise(domain.EvidenceCredited, "matches carrier scan records", evidenceSubmittedAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("appraise: %v", err)
	}
	basis, has := credited.AppraisalBasis()
	if !has || basis == "" {
		t.Fatal("采信没带依据——与提交即采信分不开")
	}
	if item.Appraisal() != domain.EvidenceReceived {
		t.Fatal("原证据记录被改写了")
	}

	if _, err := credited.Appraise(domain.EvidenceDiscredited, "second thoughts", evidenceSubmittedAt.Add(48*time.Hour)); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Fatalf("err = %v; 已评价的证据又评了一次", err)
	}
	if _, err := item.Appraise(domain.EvidenceReceived, "no-op", evidenceSubmittedAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Fatalf("err = %v; 已收到不是可评出来的值", err)
	}
	if _, err := item.Appraise(domain.EvidenceCredited, "", evidenceSubmittedAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Fatalf("err = %v; 没有依据的采信被收下了", err)
	}
}

// Covers: VE CONTEXT 硬句 170「对外披露必须形成明确披露范围或脱敏版本，不能复制出
// 来源不明、内容不一致的附件」——披露版本锚定原件指纹（对得回原件）、披露范围必备、
// 脱敏指纹与原件相同即原件外流拒。
func TestDisclosureVersionsAnchorTheOriginal(t *testing.T) {
	item := submittedEvidence(t)

	disclosure, err := domain.PrepareDisclosure(
		item,
		mustValue(t, domain.NewEvidenceContentDigest, "sha256/redacted-1"),
		"customer-1 claim review",
		evidenceSubmittedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("prepare disclosure: %v", err)
	}
	if disclosure.Original().String() != "sha256/original-1" {
		t.Fatal("披露版本对不回原件")
	}
	if disclosure.Scope() == "" {
		t.Fatal("披露范围缺席")
	}

	if _, err := domain.PrepareDisclosure(
		item,
		mustValue(t, domain.NewEvidenceContentDigest, "sha256/original-1"),
		"customer-1 claim review",
		evidenceSubmittedAt.Add(time.Hour),
	); !errors.Is(err, domain.ErrInvalidDisclosureVersion) {
		t.Fatalf("err = %v; 脱敏指纹与原件相同——原件外流", err)
	}
	for _, scope := range []string{"", "   "} {
		if _, err := domain.PrepareDisclosure(
			item,
			mustValue(t, domain.NewEvidenceContentDigest, "sha256/redacted-1"),
			scope,
			evidenceSubmittedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidDisclosureVersion) {
			t.Fatalf("scope %q: err = %v; 说不出披露范围的版本被收下了（空白同缺席）", scope, err)
		}
	}
	if !disclosure.PreparedAt().Equal(evidenceSubmittedAt.Add(time.Hour)) {
		t.Fatal("准备完成时刻没固定")
	}
}

// Covers: 重建门只对持久化开放且不相信半截快照——评价三值封闭、经调查的评价必带依据而
// 已收到必不带，快照往返原样。
func TestEvidenceItemRehydrationRevalidatesTheAppraisalPair(t *testing.T) {
	item := submittedEvidence(t)
	credited, err := item.Appraise(domain.EvidenceCredited, "matches carrier scan records", evidenceSubmittedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("appraise: %v", err)
	}

	for name, snapshot := range map[string]domain.EvidenceItemSnapshot{
		"received": item.Snapshot(),
		"credited": credited.Snapshot(),
	} {
		rebuilt, err := domain.RehydrateEvidenceItem(snapshot)
		if err != nil {
			t.Fatalf("%s: rehydrate: %v", name, err)
		}
		if rebuilt.Snapshot() != snapshot {
			t.Fatalf("%s: 快照往返走样：%+v", name, rebuilt.Snapshot())
		}
	}

	orphanBasis := item.Snapshot()
	orphanBasis.AppraisalBasis = "no one appraised this"
	if _, err := domain.RehydrateEvidenceItem(orphanBasis); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Fatalf("err = %v; 已收到却带依据的快照被重建了", err)
	}
	bareCredit := credited.Snapshot()
	bareCredit.AppraisalBasis = ""
	if _, err := domain.RehydrateEvidenceItem(bareCredit); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Fatalf("err = %v; 没有依据的采信被重建了", err)
	}
	outOfSet := item.Snapshot()
	outOfSet.Appraisal = domain.EvidenceAppraisal(99)
	if _, err := domain.RehydrateEvidenceItem(outOfSet); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Fatalf("err = %v; 三值之外的评价被重建了", err)
	}
}
