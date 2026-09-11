package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var recoveryOpenedAt = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func openMatter(t *testing.T) domain.RecoveryMatter {
	t.Helper()
	matter, err := domain.OpenRecoveryMatter(domain.RecoveryMatterSpec{
		ID:           mustValue(t, domain.NewRecoveryMatterID, "recovery-1"),
		Case:         mustValue(t, domain.NewCaseID, "case-1"),
		Counterparty: mustValue(t, domain.NewCounterpartyReference, "carrier-1"),
		Basis:        mustValue(t, domain.NewLiabilityBasisReference, "carriage-agreement/v3"),
		LegalEntity:  mustValue(t, domain.NewLegalEntityReference, "entity-cn-1"),
		Scope:        mustValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Evidence:     mustValue(t, domain.NewRequestEvidenceReference, "evidence/damage-survey"),
		Deadline:     recoveryOpenedAt.Add(30 * 24 * time.Hour),
		OpenedAt:     recoveryOpenedAt,
	})
	if err != nil {
		t.Fatalf("open recovery matter: %v", err)
	}
	return matter
}

// Covers: VE CONTEXT「相应通知或主张条件成立时即可独立发起」「无需等待客户提出索赔」
// 「按责任相对方和责任依据分别固定」——七件缺一
// 立不起；类型上没有客户索赔前置字段（独立发起是结构性的）。
func TestARecoveryMatterOpensIndependentlyWithItsFullShape(t *testing.T) {
	matter := openMatter(t)
	if matter.Counterparty().String() != "carrier-1" || matter.Basis().String() != "carriage-agreement/v3" {
		t.Fatalf("matter = %#v", matter)
	}

	missingBasis := domain.RecoveryMatterSpec{
		ID:           mustValue(t, domain.NewRecoveryMatterID, "recovery-2"),
		Case:         mustValue(t, domain.NewCaseID, "case-1"),
		Counterparty: mustValue(t, domain.NewCounterpartyReference, "carrier-1"),
		LegalEntity:  mustValue(t, domain.NewLegalEntityReference, "entity-cn-1"),
		Scope:        mustValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Evidence:     mustValue(t, domain.NewRequestEvidenceReference, "evidence/damage-survey"),
		Deadline:     recoveryOpenedAt.Add(30 * 24 * time.Hour),
		OpenedAt:     recoveryOpenedAt,
	}
	if _, err := domain.OpenRecoveryMatter(missingBasis); !errors.Is(err, domain.ErrInvalidRecovery) {
		t.Fatalf("err = %v; 没有责任依据的追偿被收下了", err)
	}

	inverted := domain.RecoveryMatterSpec{
		ID:           mustValue(t, domain.NewRecoveryMatterID, "recovery-3"),
		Case:         mustValue(t, domain.NewCaseID, "case-1"),
		Counterparty: mustValue(t, domain.NewCounterpartyReference, "carrier-1"),
		Basis:        mustValue(t, domain.NewLiabilityBasisReference, "carriage-agreement/v3"),
		LegalEntity:  mustValue(t, domain.NewLegalEntityReference, "entity-cn-1"),
		Scope:        mustValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Evidence:     mustValue(t, domain.NewRequestEvidenceReference, "evidence/damage-survey"),
		Deadline:     recoveryOpenedAt.Add(-time.Hour),
		OpenedAt:     recoveryOpenedAt,
	}
	if _, err := domain.OpenRecoveryMatter(inverted); !errors.Is(err, domain.ErrInvalidRecovery) {
		t.Fatalf("err = %v; 期限早于建立的追偿被收下了", err)
	}
}

// Covers: VE CONTEXT「不能合并为一个模糊的“已追偿”」与「必须分别记录准备完成、对外提交」
// ——动作种类封闭二值、过程节点封闭七值、义务判据引用必备
// （准备完成/渠道接受/内部审批不能默认满足对外义务——满足哪个节点由协议说了算）。
func TestActionsKeepNoticeAndAssertionApart(t *testing.T) {
	matter := openMatter(t)

	notice, err := domain.RecordRecoveryAction(
		matter.ID(),
		domain.PreliminaryNotice,
		"notice-content/v1",
		domain.ChannelAccepted,
		matter.Basis(),
		recoveryOpenedAt.Add(2*time.Hour),
		1,
	)
	if err != nil {
		t.Fatalf("record notice: %v", err)
	}
	if notice.Kind() != domain.PreliminaryNotice || notice.Milestone() != domain.ChannelAccepted {
		t.Fatalf("notice = %#v", notice)
	}

	if _, err := domain.RecordRecoveryAction(
		matter.ID(),
		domain.RecoveryActionKindInvalid,
		"content/v1",
		domain.ActionSubmitted,
		matter.Basis(),
		recoveryOpenedAt.Add(2*time.Hour),
		1,
	); !errors.Is(err, domain.ErrInvalidRecoveryAction) {
		t.Fatalf("err = %v; 模糊的已追偿被收下了——种类必须二居其一", err)
	}
	if _, err := domain.RecordRecoveryAction(
		matter.ID(),
		domain.FormalAssertion,
		"assertion-content/v1",
		domain.ActionDelivered,
		domain.LiabilityBasisReference{},
		recoveryOpenedAt.Add(3*time.Hour),
		1,
	); !errors.Is(err, domain.ErrInvalidRecoveryAction) {
		t.Fatalf("err = %v; 没有义务判据的动作被收下了", err)
	}
}

// Covers: VE CONTEXT「提交或送达失败是外部动作结果」「所有尝试和内容版本保留」
// ——失败是七值节点之二（类型
// 上没有对方拒绝字段），重试是 attempt 递增的新记录，前次记录原样存在。
func TestFailuresAreOutcomesNotRefusalsAndRetriesAreNewRecords(t *testing.T) {
	matter := openMatter(t)

	failed, err := domain.RecordRecoveryAction(
		matter.ID(),
		domain.FormalAssertion,
		"assertion-content/v1",
		domain.SubmissionFailed,
		matter.Basis(),
		recoveryOpenedAt.Add(4*time.Hour),
		1,
	)
	if err != nil {
		t.Fatalf("record failed action: %v", err)
	}

	retried, err := domain.RecordRecoveryAction(
		matter.ID(),
		domain.FormalAssertion,
		"assertion-content/v2",
		domain.ActionDelivered,
		matter.Basis(),
		recoveryOpenedAt.Add(6*time.Hour),
		2,
	)
	if err != nil {
		t.Fatalf("record retried action: %v", err)
	}
	if retried.Attempt() != 2 || retried.ContentRef() != "assertion-content/v2" {
		t.Fatalf("retried = %#v; 重试是新记录带新内容版本", retried)
	}
	if failed.Milestone() != domain.SubmissionFailed || failed.Attempt() != 1 {
		t.Fatal("前次失败记录被改写了")
	}

	if _, err := domain.RecordRecoveryAction(
		matter.ID(),
		domain.FormalAssertion,
		"assertion-content/v3",
		domain.ActionDelivered,
		matter.Basis(),
		recoveryOpenedAt.Add(7*time.Hour),
		0,
	); !errors.Is(err, domain.ErrInvalidRecoveryAction) {
		t.Fatalf("err = %v; 零次尝试没有意义", err)
	}
}
