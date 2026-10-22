package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

type fixedWithdrawalIntake struct {
	command shipmentapp.WithdrawShipmentRequestCommand
}

func (intake fixedWithdrawalIntake) IntakeWithdrawal(context.Context, *http.Request) (shipmentapp.WithdrawShipmentRequestCommand, error) {
	return intake.command, nil
}

type fixedSupplementIntake struct {
	command shipmentapp.FormNewSubmissionVersionCommand
}

func (intake fixedSupplementIntake) IntakeSupplement(context.Context, *http.Request) (shipmentapp.FormNewSubmissionVersionCommand, error) {
	return intake.command, nil
}

// Covers: `AT-PS-075`、`AT-PS-090`、`AT-PS-106` 的传输层半边——ADR-0022「`!found` 一律映射为对应上下文的`统一不可见结果`，
// 用 `404`」在撤回、资料修订、受控补充与逐包裹取消各口的生产编排上：库里没有那份委托，真仓储查过，端点答 404 `SHIPMENT_REQUEST_NOT_VISIBLE`、
// 不带 outcome。不答 5xx `NO_ANSWER_FORMED`：离线客户端会把 5xx 留在队里无限重投；取消口也不并进字段缺的
// `REQUEST_NOT_ACCEPTED`（UC-PS-006 结果语义）。Intake 是固定替身：这四行今天挂未配置即拒，采信身份那半归首方 Intake
// 逐口换上时；测试输入是隔离合成，只记 `S`。
func TestCustomerCommandsNamingAnAbsentRequestAnswerNotFoundOnTheWiredOrchestrations(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	withdrawal, err := buildWithdrawalOrchestration(db)
	if err != nil {
		t.Fatalf("装配撤回编排：%v", err)
	}
	amendment, err := buildCustomerAmendmentOrchestration(db)
	if err != nil {
		t.Fatalf("装配资料修订编排：%v", err)
	}
	supplement, err := buildCustomerSupplementOrchestration(db)
	if err != nil {
		t.Fatalf("装配受控补充编排：%v", err)
	}
	cancellation, err := buildCancellationOrchestration(db)
	if err != nil {
		t.Fatalf("装配取消编排：%v", err)
	}

	endpoints := map[string]http.Handler{
		"撤回": shipmenthttp.NewWithdrawShipmentRequestEndpoint(
			fixedWithdrawalIntake{command: withdrawalCommand(t, withdrawalSeedIdentity(t))}, withdrawal),
		"资料修订": shipmenthttp.NewAmendCustomerSourceDataEndpoint(
			fixedAmendmentIntake{command: amendmentCommand(t, "SYN-KEY-1-AMENDMENT")}, amendment),
		"受控补充": shipmenthttp.NewFormNewSubmissionVersionEndpoint(
			fixedSupplementIntake{command: supplementCommand(t, mustValue(t, domain.NewSubmissionVersionID, "SYN-VERSION-ABSENT"),
				"SYN-KEY-1-SUPPLEMENT", "SYN-SUPPLEMENT-DIGEST-1")}, supplement),
		"逐包裹取消": shipmenthttp.NewCancelParcelEndpoint(
			fixedCancellationIntake{command: cancelParcelCommand(t, cancellationSeedIdentity(t, "SYN-SUBMIT-KEY-ABSENT"))}, cancellation),
	}
	for name, endpoint := range endpoints {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s; want 404——查无的委托是调用方的错", response.Code, response.Body)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
				t.Fatalf("decode %s: %v", response.Body, err)
			}
			if _, present := fields["outcome"]; present {
				t.Fatalf("404 带了 outcome：%s", response.Body)
			}
			if got := response.Header().Get("Content-Type"); got != httpapi.ProblemMediaType {
				t.Fatalf("Content-Type = %q, want %s", got, httpapi.ProblemMediaType)
			}
			if got := problemCode(t, response); got != "SHIPMENT_REQUEST_NOT_VISIBLE" {
				t.Fatalf("code = %q, want SHIPMENT_REQUEST_NOT_VISIBLE", got)
			}
		})
	}
}
