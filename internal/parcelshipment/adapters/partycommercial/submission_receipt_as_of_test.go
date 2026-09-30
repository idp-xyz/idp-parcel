package partycommercial_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	citation := submissionReceiptCitation(t)
	at, formed, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, citation))
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
	for _, semantics := range []string{"SYN-ASOF-SUBMIT-TIME", "SYN-ASOF-ACCEPT-TIME"} {
		at, formed, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, semantics))
		if err != nil || formed || !at.IsZero() {
			t.Fatalf("%s: at=%v formed=%v err=%v, want unconfigured", semantics, at, formed, err)
		}
	}
}

func TestSubmissionReceiptAsOfRefusesAMissingRequest(t *testing.T) {
	identity := stageIdentity(t)
	request := submittedRequest(t, identity)
	source, err := adapter.NewSubmissionReceiptAsOf(&requestFinderDouble{})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, _, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, submissionReceiptCitation(t))); err == nil {
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
	if _, _, err := source.FormAsOfValue(context.Background(), receiptQuery(t, identity, request, submissionReceiptCitation(t))); err == nil {
		t.Fatal("读口失败被当成未配置")
	}
}

func TestDemoSeedAdoptsTheSubmissionReceiptCitation(t *testing.T) {
	citation := submissionReceiptCitation(t)
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "scripts/demo-seeds/data/commercial/publish-batch.json"))
	if err != nil {
		t.Fatalf("读演示种子：%v", err)
	}
	text := string(raw)
	if strings.Contains(text, "SYN-ASOF-SUBMIT-TIME") {
		t.Fatal("演示种子仍写着租户自拟的提交接收语义")
	}
	if strings.Count(text, citation) != 2 {
		t.Fatalf("产品引用出现 %d 次，want 2", strings.Count(text, citation))
	}
}

func submissionReceiptCitation(t *testing.T) string {
	t.Helper()
	citation, err := adapter.SubmissionReceiptCitation()
	if err != nil {
		t.Fatalf("产品引用：%v", err)
	}
	return citation
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("工作目录：%v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到 go.mod")
		}
		dir = parent
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
