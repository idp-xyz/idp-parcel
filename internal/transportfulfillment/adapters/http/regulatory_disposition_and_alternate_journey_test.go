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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证监管运输处置承接与替代/退运旅程启动两口的传输面（票 docs-consistency/code/43，ADR-0337）：运营决定面各按自己那一种决定
// 认证、把提交操作者作为操作证据带进命令，租户只取认证结果；载荷封闭，决定取值与目的、来路不在封闭集合里是坏报文；只有真落了决定、
// 真启动了旅程才答 201，回执不回显操作者。夹具全是合成串，证据只记 S。

const regulatoryAcceptanceBody = `{"item":"SYN-ITEM-43","basis":"SYN-REG-BASIS-43","movementAction":"RETURN_TO_BONDED_WAREHOUSE",` +
	`"itemObjects":["SYN-UNIT-1","SYN-UNIT-2"],"origin":"SYN-A","destination":"SYN-B","decision":"PARTIALLY_ACCEPTED",` +
	`"acceptedObjects":["SYN-UNIT-1"],"declineBasis":"SYN-LIMIT","movementAuthority":"SYN-AUTH-43","decidedAt":"2026-10-27T16:00:00+08:00"}`

const alternateJourneyBody = `{"journey":"SYN-JOURNEY-NEW","purpose":"RETURN","original":"SYN-JOURNEY-OLD","basisKind":"REGULATORY_DISPOSITION",` +
	`"basis":"SYN-REG-BASIS-43","members":["SYN-UNIT-1"],"startedAt":"2026-10-27T17:00:00+08:00"}`

// Covers: `AT-TF-367`、`AT-TF-369`、`AT-TF-370`（传输面）——两口各按自己那一种决定认证，租户取认证结果、请求上自报的不采信，操作者随命令带进去；
// 事项抄录、决定与旅程各格原样译进命令。
func TestTheRegulatoryAcceptanceAndJourneyStartFacesTranslateUnderTheirOwnDecision(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	intake := operatorIntake(t, authenticatorFake{asked: &asked})
	selfReported := func(body string) *http.Request {
		request := bearerRequest("t", body)
		request.URL.RawQuery = "tenant=SYN-TENANT-FOREIGN"
		request.Header.Set("X-Reported-Tenant", "SYN-TENANT-FOREIGN")
		return request
	}

	acceptance, err := intake.IntakeRegulatoryDispositionAcceptance(context.Background(), selfReported(regulatoryAcceptanceBody))
	if err != nil {
		t.Fatalf("acceptance: %v", err)
	}
	if acceptance.TenantID.String() != operatorTenant || acceptance.DecidedBy != operatorSubject ||
		acceptance.Item.String() != "SYN-ITEM-43" || acceptance.Basis.String() != "SYN-REG-BASIS-43" ||
		acceptance.MovementAction != "RETURN_TO_BONDED_WAREHOUSE" || len(acceptance.ItemObjects) != 2 ||
		acceptance.Origin != "SYN-A" || acceptance.Destination != "SYN-B" || acceptance.Decision != domain.DispositionPartiallyAccepted ||
		len(acceptance.AcceptedObjects) != 1 || acceptance.DeclineBasis != "SYN-LIMIT" || acceptance.MovementAuthority != "SYN-AUTH-43" ||
		!acceptance.DecidedAt.Equal(time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("acceptance command = %+v", acceptance)
	}

	journey, err := intake.IntakeAlternateJourneyStart(context.Background(), selfReported(alternateJourneyBody))
	if err != nil {
		t.Fatalf("journey: %v", err)
	}
	if journey.TenantID.String() != operatorTenant || journey.StartedBy != operatorSubject || journey.Journey != "SYN-JOURNEY-NEW" ||
		journey.Purpose != domain.ReturnJourneyPurpose || journey.Original != "SYN-JOURNEY-OLD" ||
		journey.BasisKind != domain.RegulatoryDispositionDecision || journey.Basis != "SYN-REG-BASIS-43" ||
		len(journey.Members) != 1 || !journey.StartedAt.Equal(time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("journey command = %+v", journey)
	}
	if len(asked) != 2 || asked[0] != tfhttp.OperatorDecisionRegulatoryDispositionAcceptance || asked[1] != tfhttp.OperatorDecisionAlternateJourneyStart {
		t.Fatalf("问认证方的决定种类 = %v；两口各按自己那一种，能承接的不因此能开旅程", asked)
	}
}

// Covers: `AT-TF-367`、`AT-TF-369`（传输面）——先认证、后解载荷；载荷封闭（夹带租户或操作者一律 400）；决定取值、目的、来路不在封闭集合里、
// 解不出的时刻是坏报文。
func TestTheRegulatoryAcceptanceAndJourneyStartFacesAuthenticateFirstAndDecodeClosed(t *testing.T) {
	denied := operatorIntake(t, authenticatorFake{err: tfhttp.ErrOperatorNotGranted})
	if _, err := denied.IntakeRegulatoryDispositionAcceptance(context.Background(), bearerRequest("t", `not json`)); !errors.Is(err, tfhttp.ErrOperatorNotGranted) {
		t.Fatalf("未授予的调用方看到了载荷校验的结果：err = %v", err)
	}
	if _, err := denied.IntakeAlternateJourneyStart(context.Background(), bearerRequest("", alternateJourneyBody)); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("没带令牌：err = %v", err)
	}

	intake := operatorIntake(t, authenticatorFake{})
	for name, body := range map[string]string{
		"承接夹带租户":   strings.Replace(regulatoryAcceptanceBody, `"item"`, `"tenantId":"SYN-TENANT-02","item"`, 1),
		"承接夹带操作者":  strings.Replace(regulatoryAcceptanceBody, `"item"`, `"decidedBy":"someone","item"`, 1),
		"决定取值不在集合": strings.Replace(regulatoryAcceptanceBody, `"PARTIALLY_ACCEPTED"`, `"PENDING"`, 1),
		"承接时刻解不出":  strings.Replace(regulatoryAcceptanceBody, `"2026-10-27T16:00:00+08:00"`, `"today"`, 1),
	} {
		if _, err := intake.IntakeRegulatoryDispositionAcceptance(context.Background(), bearerRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
	for name, body := range map[string]string{
		"旅程夹带操作者": strings.Replace(alternateJourneyBody, `"journey"`, `"startedBy":"someone","journey"`, 1),
		"目的不在集合":  strings.Replace(alternateJourneyBody, `"RETURN"`, `"CANCEL"`, 1),
		"来路不在集合":  strings.Replace(alternateJourneyBody, `"REGULATORY_DISPOSITION"`, `"WHATEVER"`, 1),
		"启动时刻解不出": strings.Replace(alternateJourneyBody, `"2026-10-27T17:00:00+08:00"`, `"soon"`, 1),
	} {
		if _, err := intake.IntakeAlternateJourneyStart(context.Background(), bearerRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

type acceptorFake struct {
	result application.AcceptDispositionResult
}

func (fake acceptorFake) Handle(context.Context, application.AcceptRegulatoryDispositionCommand) (application.AcceptDispositionResult, error) {
	return fake.result, nil
}

type starterFake struct {
	result application.StartAlternateJourneyResult
}

func (fake starterFake) Handle(context.Context, application.StartAlternateJourneyCommand) (application.StartAlternateJourneyResult, error) {
	return fake.result, nil
}

// Covers: `AT-TF-367`、`AT-TF-368`、`AT-TF-369`（传输面）——只有真落了决定、真启动了旅程用 201；其余格（这里取未受理）是形成了的答案，200 带 outcome；
// 回执不回显操作者。真正的各格由应用编排与生产装配真库用例钉，这里钉传输层的状态分法与方法门。
func TestTheRegulatoryAcceptanceAndJourneyStartEndpointsAnswerWithinTheStatusSplit(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	acceptEndpoint := tfhttp.NewAcceptRegulatoryDispositionEndpoint(intake, acceptorFake{})
	journeyEndpoint := tfhttp.NewStartAlternateJourneyEndpoint(intake, starterFake{})

	for name, call := range map[string]struct {
		endpoint http.Handler
		body     string
	}{
		"承接口": {acceptEndpoint, regulatoryAcceptanceBody},
		"旅程口": {journeyEndpoint, alternateJourneyBody},
	} {
		// 零值结果没有名字：传输层不替编排编一个答复。
		response := httptest.NewRecorder()
		call.endpoint.ServeHTTP(response, bearerRequest("t", call.body))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("%s：零值结果 = %d，want 500（无名 outcome）", name, response.Code)
		}
		get := httptest.NewRecorder()
		call.endpoint.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/x", nil))
		if get.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s：GET = %d，want 405", name, get.Code)
		}
	}

	var body map[string]any
	notAccepted := httptest.NewRecorder()
	// 经应用编排的真实 NOT_ACCEPTED 答复：空载荷落在受理门上。
	handler := application.NewAcceptRegulatoryDispositionHandler(application.AcceptRegulatoryDispositionDeps{})
	result, err := handler.Handle(context.Background(), application.AcceptRegulatoryDispositionCommand{})
	if err != nil {
		t.Fatal(err)
	}
	tfhttp.NewAcceptRegulatoryDispositionEndpoint(intake, acceptorFake{result: result}).ServeHTTP(notAccepted, bearerRequest("t", regulatoryAcceptanceBody))
	if notAccepted.Code != http.StatusOK {
		t.Fatalf("未受理 = %d，want 200", notAccepted.Code)
	}
	if err := json.Unmarshal(notAccepted.Body.Bytes(), &body); err != nil || body["outcome"] != "NOT_ACCEPTED" || strings.Contains(notAccepted.Body.String(), operatorSubject) {
		t.Fatalf("未受理回执 = %s（%v）", notAccepted.Body.String(), err)
	}
}
