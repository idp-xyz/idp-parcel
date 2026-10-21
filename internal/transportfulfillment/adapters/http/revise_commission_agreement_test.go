package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 本文件证委托协议换版那一口的线格式（ADR-0231 决定三）：按委托协议换版这一种认证，决定人取认证出的操作者、载荷带身份即拒；各格的状态码
// 与回显。真库上的成立、重放与补形成由 cmd/parcel-api 与 cmd/parcel-dispatch 的用例证。

type reviserSpy struct {
	commands []application.ReviseCommissionAgreementCommand
}

func (spy *reviserSpy) Revise(_ context.Context, command application.ReviseCommissionAgreementCommand) (application.CommissionAgreementRevisionResult, error) {
	spy.commands = append(spy.commands, command)
	return application.CommissionAgreementRevisionResult{}, nil
}

func postRevision(t *testing.T, endpoint http.Handler, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-transport-commission-agreement-revisions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("答复不是 JSON：%d %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Code, decoded
}

// Covers: AT-TF-297 的线格式那一半——换版口按委托协议换版这一种认证（持别的决定的人进不来）；载荷译成命令：租户与决定人取认证、起用时刻
// 按 RFC 3339 译；载荷带租户、起用时刻不是 RFC 3339 都是坏报文（400），不交编排；编排答不出名的格答 500。
func TestTheRevisionFaceAuthenticatesItsOwnDecisionAndDecodesItsPayload(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	refused := operatorIntake(t, authenticatorFake{err: tfhttp.ErrOperatorNotGranted, asked: &asked})
	if _, err := refused.IntakeCommissionAgreementRevision(context.Background(), bearerRequest("t", "{}")); !errors.Is(err, tfhttp.ErrOperatorNotGranted) ||
		len(asked) != 1 || asked[0] != tfhttp.OperatorDecisionTransportCommissionAgreementRevision {
		t.Fatalf("认证问的是 %v（err %v）, want 恰委托协议换版", asked, err)
	}

	spy := &reviserSpy{}
	endpoint := tfhttp.NewReviseCommissionAgreementEndpoint(operatorIntake(t, authenticatorFake{}), spy)
	status, _ := postRevision(t, endpoint, `{"commission":"SYN-COMMISSION-01","successorAgreementObject":"SYN-SUPPLIER-01",`+
		`"successorAgreementVersion":"v3","appliesFrom":"2026-10-19T00:00:00Z","basis":"SYN-DECISION-01"}`)
	if status != http.StatusInternalServerError || len(spy.commands) != 1 {
		t.Fatalf("编排答空格 = %d、交编排 %d 次", status, len(spy.commands))
	}
	command := spy.commands[0]
	if command.TenantID.String() != operatorTenant || command.DecidedBy != operatorSubject || command.Commission != "SYN-COMMISSION-01" ||
		command.SuccessorObject != "SYN-SUPPLIER-01" || command.SuccessorVersion != "v3" || command.Basis != "SYN-DECISION-01" ||
		!command.AppliesFrom.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("命令 = %+v", command)
	}
	for _, body := range []string{
		`{"tenantId":"SYN-TENANT-02","commission":"SYN-COMMISSION-01"}`,
		`{"commission":"SYN-COMMISSION-01","appliesFrom":"yesterday"}`,
	} {
		if status, _ := postRevision(t, endpoint, body); status != http.StatusBadRequest {
			t.Fatalf("%s 答 %d, want 400", body, status)
		}
	}
	if len(spy.commands) != 1 {
		t.Fatalf("坏报文交了编排：%d 次", len(spy.commands))
	}
}
