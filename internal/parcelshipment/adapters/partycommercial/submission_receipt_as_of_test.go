package partycommercial_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

func TestSubmissionReceiptAsOfUsesTheRecordedReceiveTime(t *testing.T) {
	identity := stageIdentity(t)
	request := submittedRequest(t, identity)
	source, err := adapter.NewSubmissionReceiptAsOf(&requestFinderDouble{request: request, found: true})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	at, formed, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, "SYN-ASOF-SUBMIT-TIME"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	want := request.CurrentSubmissionVersion().SourceSubmission().ReceivedAt()
	if !at.Equal(want) {
		t.Fatalf("at = %v, want %v", at, want)
	}
}

func TestSubmissionReceiptAsOfLeavesOtherSemanticsUnconfigured(t *testing.T) {
	identity := stageIdentity(t)
	request := submittedRequest(t, identity)
	source, err := adapter.NewSubmissionReceiptAsOf(&requestFinderDouble{request: request, found: true})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	at, formed, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, "SYN-ASOF-ACCEPT-TIME"))
	if err != nil || formed || !at.IsZero() {
		t.Fatalf("at=%v formed=%v err=%v, want unconfigured", at, formed, err)
	}
}

func TestSubmissionReceiptAsOfRefusesAMissingRequest(t *testing.T) {
	identity := stageIdentity(t)
	request := submittedRequest(t, identity)
	source, err := adapter.NewSubmissionReceiptAsOf(&requestFinderDouble{})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, _, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, "SYN-ASOF-SUBMIT-TIME")); err == nil {
		t.Fatal("找不到委托却形成了时点")
	}
}

func TestSubmissionReceiptAsOfRefusesNilRequests(t *testing.T) {
	if _, err := adapter.NewSubmissionReceiptAsOf(nil); err == nil {
		t.Fatal("nil 委托读口被收下了")
	}
}

func TestSubmissionReceiptAsOfSurfacesLookupErrors(t *testing.T) {
	identity := stageIdentity(t)
	request := submittedRequest(t, identity)
	source, err := adapter.NewSubmissionReceiptAsOf(&requestFinderDouble{err: errors.New("读库失败")})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, _, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, "SYN-ASOF-SUBMIT-TIME")); err == nil {
		t.Fatal("读口失败被当成未配置")
	}
}

func receiptQuery(
	t *testing.T,
	identity psdomain.SourceIdentity,
	request psdomain.ShipmentRequest,
	semantics string,
) psports.JudgmentAsOfQuery {
	t.Helper()
	declared, err := psdomain.NewDeclaredAsOf(
		psdomain.ReachabilityJudgmentKind,
		value(t, psdomain.NewAsOfSemanticsReference, semantics),
		value(t, psdomain.NewAsOfPolicyVersion, "SYN-ASOF-POLICY-01"),
	)
	if err != nil {
		t.Fatalf("声明时点：%v", err)
	}
	return psports.JudgmentAsOfQuery{
		Identity:          identity,
		ShipmentRequestID: request.ShipmentRequestID(),
		SubmissionVersion: request.CurrentSubmissionVersion().VersionID(),
		Declared:          declared,
	}
}
