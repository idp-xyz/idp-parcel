package customshttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

type capturingManifestHandler struct {
	received []application.ReceiveManifestCommand
	revised  []application.ReviseManifestCommand
}

func (fake *capturingManifestHandler) Receive(_ context.Context, command application.ReceiveManifestCommand) (application.ManifestResult, error) {
	fake.received = append(fake.received, command)
	return application.ManifestResult{}, nil
}

func (fake *capturingManifestHandler) Revise(_ context.Context, command application.ReviseManifestCommand) (application.ManifestResult, error) {
	fake.revised = append(fake.revised, command)
	return application.ManifestResult{}, nil
}

// Covers: dox/decisions.md「产品形态」——外部舱单引用的手工首登与版本推进两口按同一种决定认证；租户与录入人只取认证
// 结果，接受时刻由服务端给；承运商由录入的人指名；原件必填并同时充当来源事实引用；自报租户、录入人或 sourceFact，
// 没有承运商或原件都是坏报文，编排一次也不被调到。
func TestTheManualManifestFacesAuthenticateTheirKindAndTakeTheCarrierFromTheEntry(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	handler := &capturingManifestHandler{}
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	manual := customshttp.ManualManifestIntake(intake, func() time.Time { return now })
	receive := customshttp.NewReceiveManifestReferenceEndpoint(manual, handler)
	revise := customshttp.NewReviseManifestReferenceEndpoint(manual, handler)
	serve := func(endpoint http.Handler, body string) int {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, post("/probe", body))
		return response.Code
	}

	serve(receive, `{"manifestId":" SYN-MANIFEST-1 ","version":" v1 ","procedure":"SYN-PROC-EXPORT","direction":"EXPORT","scope":"SYN-MOVEMENT-1","carrier":" SYN-CARRIER-X ","original":" SYN-MAIL-1 "}`)
	serve(revise, `{"manifestId":"SYN-MANIFEST-1","version":"v2","scope":"SYN-MOVEMENT-2","carrier":"SYN-CARRIER-X","original":"SYN-MAIL-2"}`)
	if len(handler.received) != 1 || len(handler.revised) != 1 || len(asked) != 2 || asked[0] != "EXTERNAL_MANIFEST_ENTRY" || asked[1] != "EXTERNAL_MANIFEST_ENTRY" {
		t.Fatalf("received = %d, revised = %d, asked = %v", len(handler.received), len(handler.revised), asked)
	}
	first := handler.received[0]
	if first.TenantID.String() != "SYN-TENANT-01" || first.EnteredBy != "OPERATOR/https://issuer#alice" || first.Original != "SYN-MAIL-1" ||
		first.Spec.Carrier.String() != "SYN-CARRIER-X" || first.Spec.SourceFact != "SYN-MAIL-1" || !first.Spec.AcceptedAt.Equal(now) ||
		first.Spec.Manifest.String() != "SYN-MANIFEST-1" || first.Spec.Version.String() != "v1" {
		t.Fatalf("首登命令 = %+v", first)
	}
	second := handler.revised[0]
	if second.Carrier.String() != "SYN-CARRIER-X" || second.Version.String() != "v2" || second.EnteredBy != "OPERATOR/https://issuer#alice" ||
		second.Original != "SYN-MAIL-2" || !second.At.Equal(now) {
		t.Fatalf("推进命令 = %+v", second)
	}
	for name, body := range map[string]string{
		"没有原件":          `{"manifestId":"M","version":"v1","procedure":"P","direction":"EXPORT","scope":"S","carrier":"C"}`,
		"没有承运商":         `{"manifestId":"M","version":"v1","procedure":"P","direction":"EXPORT","scope":"S","original":"O"}`,
		"自报租户":          `{"tenantId":"x","manifestId":"M","version":"v1","procedure":"P","direction":"EXPORT","scope":"S","carrier":"C","original":"O"}`,
		"自报录入人":         `{"enteredBy":"x","manifestId":"M","version":"v1","procedure":"P","direction":"EXPORT","scope":"S","carrier":"C","original":"O"}`,
		"自报 sourceFact": `{"sourceFact":"x","manifestId":"M","version":"v1","procedure":"P","direction":"EXPORT","scope":"S","carrier":"C","original":"O"}`,
		"方向不在词表":        `{"manifestId":"M","version":"v1","procedure":"P","direction":"TRANSIT","scope":"S","carrier":"C","original":"O"}`,
	} {
		if status := serve(receive, body); status != http.StatusBadRequest {
			t.Fatalf("%s = %d，想要 400", name, status)
		}
	}
	if status := serve(revise, `{"manifestId":"M","version":"v2","scope":"S","carrier":"C"}`); status != http.StatusBadRequest {
		t.Fatalf("推进没有原件 = %d，想要 400", status)
	}
	if len(handler.received) != 1 || len(handler.revised) != 1 {
		t.Fatal("坏报文到了编排")
	}
}
