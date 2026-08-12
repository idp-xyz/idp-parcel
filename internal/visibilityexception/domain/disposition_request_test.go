package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var requestSentAt = time.Date(2026, 8, 11, 11, 0, 0, 0, time.UTC)

func sentRequest(t *testing.T, id string, version int) *domain.DispositionRequest {
	t.Helper()
	request, err := domain.SendDispositionRequest(domain.DispositionRequestSpec{
		ID:               mustValue(t, domain.NewDispositionRequestID, id),
		Case:             mustValue(t, domain.NewCaseID, "case-1"),
		Target:           domain.SourceNetworkRouting,
		Action:           mustValue(t, domain.NewRequestedActionReference, "REROUTE"),
		Scope:            mustValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Reason:           "route deviation confirmed",
		Evidence:         mustValue(t, domain.NewRequestEvidenceReference, "evidence/route-deviation"),
		IntentVersion:    version,
		SentAt:           requestSentAt,
		AcceptanceWindow: requestSentAt.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("send disposition request %q: %v", id, err)
	}
	return request
}

// Covers: VE CONTEXT「处置请求必须明确目标对象、请求动作、原因、证据、请求方、接收
// 上下文和期望时限」与「请求发送、源上下文接受……分别记录；结果只表达请求判断，不
// 证明实际执行」——七件缺一立不起；判断四走向封闭且不判第二次；类型上没有执行结果
// 字段（推定完成无处落脚）。
func TestARequestDemandsItsShapeAndJudgesOnce(t *testing.T) {
	request := sentRequest(t, "request-1", 1)
	if _, judged := request.Judgment(); judged {
		t.Fatal("刚发送的请求凭空有了判断")
	}

	if err := request.RecordSourceJudgment(domain.RequestPartiallyAccepted, requestSentAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("record judgment: %v", err)
	}
	judgment, judged := request.Judgment()
	if !judged || judgment != domain.RequestPartiallyAccepted {
		t.Fatalf("judgment = %q judged = %v", judgment, judged)
	}
	if err := request.RecordSourceJudgment(domain.RequestAccepted, requestSentAt.Add(3*time.Hour)); !errors.Is(err, domain.ErrRequestAlreadyJudged) {
		t.Fatalf("err = %v; 判了两次", err)
	}

	missingEvidence := domain.DispositionRequestSpec{
		ID:            mustValue(t, domain.NewDispositionRequestID, "request-9"),
		Case:          mustValue(t, domain.NewCaseID, "case-1"),
		Target:        domain.SourceNetworkRouting,
		Action:        mustValue(t, domain.NewRequestedActionReference, "REROUTE"),
		Scope:         mustValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Reason:        "reason",
		IntentVersion: 1,
		SentAt:        requestSentAt,
	}
	if _, err := domain.SendDispositionRequest(missingEvidence); !errors.Is(err, domain.ErrInvalidDispositionRequest) {
		t.Fatalf("err = %v; 没有证据的请求被收下了", err)
	}
}

// Covers: VE CONTEXT「受理有效期届满后，尚未被目标上下文接受的范围不得再按旧请求
// 启动」——过期后接受与部分接受都拒（独立哨兵）；拒绝与要求补充仍可记录（它们不
// 启动任何东西）。
func TestAnExpiredWindowRefusesLateAcceptance(t *testing.T) {
	expired := sentRequest(t, "request-1", 1)
	lateAt := requestSentAt.Add(48 * time.Hour)

	if err := expired.RecordSourceJudgment(domain.RequestAccepted, lateAt); !errors.Is(err, domain.ErrRequestWindowExpired) {
		t.Fatalf("err = %v; 过期后按旧请求启动了", err)
	}
	if err := expired.RecordSourceJudgment(domain.RequestRefused, lateAt); err != nil {
		t.Fatalf("record refusal after expiry: %v", err)
	}
}

// Covers: VE CONTEXT「取消或替代只改变未来意图，不撤销已经发生的源业务事实。目标
// 上下文必须分别返回取消已接受、部分取消、已无法取消或拒绝取消」与「案件状态变化
// 不能静默使旧请求失效」——取消答复四值封闭且只对已判断的请求记录；替代必须换身份、
// 升意图版本，原请求与其判断原样保留。
func TestCancellationAndSupersessionOnlyChangeFutureIntent(t *testing.T) {
	request := sentRequest(t, "request-1", 1)

	if err := request.RecordCancellationAnswer(domain.NoLongerCancellable, requestSentAt.Add(time.Hour)); !errors.Is(err, domain.ErrRequestNotJudged) {
		t.Fatalf("err = %v; 没人接的请求谈不上取消", err)
	}

	if err := request.RecordSourceJudgment(domain.RequestAccepted, requestSentAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("record judgment: %v", err)
	}
	if err := request.RecordCancellationAnswer(domain.NoLongerCancellable, requestSentAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("record cancellation answer: %v", err)
	}
	answer, answered := request.CancellationOutcome()
	if !answered || answer != domain.NoLongerCancellable {
		t.Fatalf("answer = %q answered = %v；已无法取消是如实结果", answer, answered)
	}
	if err := request.RecordCancellationAnswer(domain.CancellationAcceptedByTarget, requestSentAt.Add(4*time.Hour)); !errors.Is(err, domain.ErrRequestAlreadyJudged) {
		t.Fatalf("err = %v; 取消答了两次", err)
	}

	successor := sentRequest(t, "request-2", 2)
	if err := request.SupersedeWith(successor); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	supersededBy, superseded := request.SupersededBy()
	if !superseded || supersededBy.String() != "request-2" {
		t.Fatalf("superseded by = %s present = %v", supersededBy, superseded)
	}
	if judgment, judged := request.Judgment(); !judged || judgment != domain.RequestAccepted {
		t.Fatal("替代抹掉了原判断——替代不是删除")
	}

	sameVersion := sentRequest(t, "request-3", 1)
	if err := sentRequest(t, "request-4", 1).SupersedeWith(sameVersion); !errors.Is(err, domain.ErrInvalidDispositionRequest) {
		t.Fatalf("err = %v; 不升版本的替代被收下了", err)
	}
}
