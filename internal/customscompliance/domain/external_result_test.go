package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var resultReceivedAt = time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)

func externalResult(t *testing.T, layer domain.ResultLayer, semantics string) domain.ExternalResult {
	t.Helper()
	result, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     "channel-response/771",
		Role:         mustValue(t, domain.NewSourceAuthorityRole, "CUSTOMS_AUTHORITY"),
		RawSemantics: semantics,
		Rule:         mustValue(t, domain.NewInterpretationRuleReference, "interpretation/v3"),
		Version:      mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Attempt:      1,
		Scope:        mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		OccurredAt:   resultReceivedAt.Add(-time.Hour),
		ReceivedAt:   resultReceivedAt,
	})
	if err != nil {
		t.Fatalf("interpret external result (%s): %v", layer, err)
	}
	return result
}

// Covers: CC CONTEXT 硬句 185/186「监管接收、业务受理、监管过程决定、监管核定税费、
// 放行结果和监管处置决定必须分层保存……不能使用一个『清关成功』状态覆盖各层事实」
// 「每项外部结果必须保存来源身份、权威角色、原始业务语义、业务时间、接收时间、解释
// 规则及其与提交版本、尝试和结果范围的关系」——六层封闭各自成立、八件缺一立不起、
// 类型上没有层间派生方法（前层成功造不出后层）。
func TestSixLayersStandAloneWithTheirEightAnchors(t *testing.T) {
	receipt := externalResult(t, domain.RegulatoryReceiptLayer, "MANIFEST_RECEIVED")
	if receipt.Layer() != domain.RegulatoryReceiptLayer || receipt.RawSemantics() != "MANIFEST_RECEIVED" {
		t.Fatalf("receipt = %#v", receipt)
	}

	missingRole := domain.ExternalResultSpec{
		Layer:        domain.ReleaseResultLayer,
		SourceID:     "channel-response/772",
		RawSemantics: "RELEASED",
		Rule:         mustValue(t, domain.NewInterpretationRuleReference, "interpretation/v3"),
		Version:      mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Attempt:      1,
		Scope:        mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		OccurredAt:   resultReceivedAt.Add(-time.Hour),
		ReceivedAt:   resultReceivedAt,
	}
	if _, err := domain.InterpretExternalResult(missingRole); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 没有权威角色的结果被收下了——中介响应只有语义明确代表监管机构才成监管事实", err)
	}

	unattributable := missingRole
	unattributable.Role = mustValue(t, domain.NewSourceAuthorityRole, "CUSTOMS_AUTHORITY")
	unattributable.Version = domain.SubmissionVersionID{}
	if _, err := domain.InterpretExternalResult(unattributable); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 关联不上原提交的结果不得猜测成层", err)
	}
}

// Covers: CC CONTEXT 硬句 187「与同层现有事实冲突时，不得据此猜测提交、补造缺失层次
// 或按最后到达直接改变当前判断」——同层同版本同范围异语义即冲突（独立哨兵，双方保留
// 不选边）；不同层与不同范围的事实各归各位不构成冲突。
func TestSameLayerConflictsAreDetectedNotOverwritten(t *testing.T) {
	released := externalResult(t, domain.ReleaseResultLayer, "FULL_RELEASE")
	held := externalResult(t, domain.ReleaseResultLayer, "HOLD")

	if err := domain.CheckLayerConsistency([]domain.ExternalResult{released}, held); !errors.Is(err, domain.ErrLayerConflict) {
		t.Fatalf("err = %v; 同层异语义没报冲突——按最后到达覆盖被明禁", err)
	}

	duty := externalResult(t, domain.AssessedDutyLayer, "DUTY_ASSESSED/120USD")
	if err := domain.CheckLayerConsistency([]domain.ExternalResult{released}, duty); err != nil {
		t.Fatalf("err = %v; 不同层的事实各归各位", err)
	}

	same := externalResult(t, domain.ReleaseResultLayer, "FULL_RELEASE")
	if err := domain.CheckLayerConsistency([]domain.ExternalResult{released}, same); err != nil {
		t.Fatalf("err = %v; 同语义重复到达不是冲突（那是重放，编排按已有结果作答）", err)
	}
}
