package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func rejectedPrior(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	rejected, err := submitted(t).RejectByAuthority(activeRejectionSpec(t))
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	return rejected
}

func acceptedPrior(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return accepted
}

func withdrawnPrior(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	withdrawn, err := submitted(t).WithdrawByCustomer(withdrawalSpec(t))
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	return withdrawn
}

// linkedSubmitSpec 造一份带关联出处、身份与首发委托不同的提交规格。
func linkedSubmitSpec(t *testing.T, link domain.PriorRequestLink) domain.SubmitShipmentRequestSpec {
	t.Helper()
	declared := []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-9")}
	candidate, err := domain.NewSubmissionCandidate(
		sourceFingerprint(t, "tenant-1", "customer-1", "source-a", "key-2", "digest-2"),
		mustValue(t, domain.NewSubmissionBatchID, "batch-2"),
		mustValue(t, domain.NewShipmentRequestID, "request-2"),
		declared,
	)
	if err != nil {
		t.Fatalf("new submission candidate: %v", err)
	}
	spec := submitSpec(t, "parcel-9")
	spec.Candidate = candidate
	spec.Link = link
	return spec
}

// Covers: `AT-PS-036`②③「已拒绝委托修正资料、已接受委托新增成员 → 关联新委托」与
// `AT-PS-076`「已撤回后客户要求恢复 → 原委托不恢复，建立关联新委托」——三个方向各锚
// 一个原委托终态，出处互证后落在新委托身上，原委托的状态与决定原样保留。
func TestALinkedRequestIsBornPointingAtItsTerminalPrior(t *testing.T) {
	cases := map[string]struct {
		prior domain.ShipmentRequest
		kind  domain.RequestLinkKind
	}{
		"rejected correction":    {prior: rejectedPrior(t), kind: domain.LinkRejectedCorrection},
		"accepted reshaping":     {prior: acceptedPrior(t), kind: domain.LinkAcceptedReshaping},
		"withdrawn resubmission": {prior: withdrawnPrior(t), kind: domain.LinkWithdrawnResubmission},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			priorState := testCase.prior.State()
			link, err := domain.EstablishPriorRequestLink(testCase.prior, testCase.kind)
			if err != nil {
				t.Fatalf("establish link: %v", err)
			}

			request, err := domain.SubmitShipmentRequest(linkedSubmitSpec(t, link))
			if err != nil {
				t.Fatalf("submit linked request: %v", err)
			}

			carried, present := request.PriorRequestLink()
			if !present {
				t.Fatal("关联新委托出生没带出处")
			}
			if carried.PriorRequestID() != testCase.prior.ShipmentRequestID() || carried.Kind() != testCase.kind {
				t.Fatalf("link = %s/%s; 出处必须指回原委托并声明方向", carried.PriorRequestID(), carried.Kind())
			}
			if testCase.prior.State() != priorState {
				t.Fatal("建立关联改动了原委托")
			}
		})
	}
}

// Covers: 方向与原委托终态互证——声称的终态与实际不符时关联建立不起来，且拒绝理由
// 与「请求本身不完整」分格（各自的续办动作不同）。待决委托没有任何方向可用：普通纠错
// 走同一委托的新提交版本，不走关联。
func TestALinkRefusesAPriorInTheWrongState(t *testing.T) {
	cases := map[string]struct {
		prior domain.ShipmentRequest
		kind  domain.RequestLinkKind
	}{
		"rejected correction against a pending prior":   {prior: submitted(t), kind: domain.LinkRejectedCorrection},
		"rejected correction against an accepted prior": {prior: acceptedPrior(t), kind: domain.LinkRejectedCorrection},
		"accepted reshaping against a withdrawn prior":  {prior: withdrawnPrior(t), kind: domain.LinkAcceptedReshaping},
		"withdrawn resubmission against a rejected one": {prior: rejectedPrior(t), kind: domain.LinkWithdrawnResubmission},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.EstablishPriorRequestLink(testCase.prior, testCase.kind)
			if !errors.Is(err, domain.ErrPriorStateIncompatibleWithLink) {
				t.Fatalf("err = %v, want ErrPriorStateIncompatibleWithLink", err)
			}
		})
	}

	if _, err := domain.EstablishPriorRequestLink(rejectedPrior(t), domain.RequestLinkKindInvalid); !errors.Is(err, domain.ErrInvalidRequestLink) {
		t.Fatalf("err = %v, want ErrInvalidRequestLink（方向缺格不是状态不符）", err)
	}
}

// Covers: 自指防线——出处指向委托自己会让关联链第一环就绕回；首次委托的出处报告缺席。
func TestALinkNeverPointsAtTheRequestItself(t *testing.T) {
	prior := rejectedPrior(t)
	link, err := domain.EstablishPriorRequestLink(prior, domain.LinkRejectedCorrection)
	if err != nil {
		t.Fatalf("establish link: %v", err)
	}

	// submitSpec 的委托 ID 与 rejectedPrior 同为 request-1：出处指向自己。
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Link = link
	if _, err := domain.SubmitShipmentRequest(spec); !errors.Is(err, domain.ErrInvalidRequestLink) {
		t.Fatalf("err = %v, want ErrInvalidRequestLink", err)
	}

	first, err := domain.SubmitShipmentRequest(submitSpec(t, "parcel-1"))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, present := first.PriorRequestLink(); present {
		t.Fatal("首次委托凭空长出了关联出处")
	}
}
