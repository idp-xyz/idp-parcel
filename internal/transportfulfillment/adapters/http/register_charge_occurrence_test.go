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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证运输收费发生项登记口的线格式与响应（票 demo-chain-gaps/27、29）：载荷逐格译成命令、租户与登记者只来自
// 认证结果、原因词集外是坏报文；断言期的断言、数量与单位三键退场，带来是坏报文；新落一项答 201 并逐字段回显（依据
// 「按协议声明判」与登记者都在），其余各格答 200，未配置与协议不计此原因带回作答的那一版协议；响应里没有任何金额
// 或币种键。夹具全是合成串，证据只记 S。

const decisionChargeOccurrenceBody = `{"reason":"ACTUAL_FULFILLMENT","segment":"SYN-SEG-SZX-SIN-01","objects":["SYN-PARCEL-01"],` +
	`"journey":"SYN-JRN-SYN-PARCEL-01","provider":"SYN-PARTY-CARRIER-TRUNK-01","agreement":"SYN-SUPPLIER-TRUNK-CN-SG-01",` +
	`"agreementVersion":"v2"}`

func TestChargeOccurrenceRegistrationTranslatesItsPayloadUnderTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	command, err := intake.IntakeChargeOccurrenceRegistration(context.Background(), bearerRequest("t", decisionChargeOccurrenceBody))
	if err != nil {
		t.Fatalf("合规载荷：%v", err)
	}
	if command.TenantID.String() != operatorTenant || command.Registrant != operatorSubject ||
		command.Reason != domain.ActualFulfillmentOccurrence ||
		command.Segment != "SYN-SEG-SZX-SIN-01" || len(command.Objects) != 1 || command.Objects[0] != "SYN-PARCEL-01" ||
		command.Journey != "SYN-JRN-SYN-PARCEL-01" || command.Provider != "SYN-PARTY-CARRIER-TRUNK-01" ||
		command.AgreementObject != "SYN-SUPPLIER-TRUNK-CN-SG-01" || command.AgreementVersion != "v2" ||
		command.SelfOperated {
		t.Fatalf("command = %+v，与载荷不符", command)
	}

	for name, body := range map[string]string{
		"集外原因词":   strings.Replace(decisionChargeOccurrenceBody, "ACTUAL_FULFILLMENT", "actual_fulfillment", 1),
		"自带业务时间":  `{"occurredAt":"2026-10-19T06:00:00Z",` + strings.TrimPrefix(decisionChargeOccurrenceBody, "{"),
		"自带发生项引用": `{"occurrence":"OCC-1",` + strings.TrimPrefix(decisionChargeOccurrenceBody, "{"),
		"自带登记者":   `{"registrant":"SYN-OPERATOR-99",` + strings.TrimPrefix(decisionChargeOccurrenceBody, "{"),
		// AT-TF-170：断言期的三键退场（ADR-0209 决定四），带来按未知键拒，不静默丢掉。
		"断言期的断言": `{"conditionAssertion":"干线协议段到段计一段",` + strings.TrimPrefix(decisionChargeOccurrenceBody, "{"),
		"断言期的数量": `{"quantity":1,` + strings.TrimPrefix(decisionChargeOccurrenceBody, "{"),
		"断言期的单位": `{"unit":"SEGMENT",` + strings.TrimPrefix(decisionChargeOccurrenceBody, "{"),
	} {
		if _, err := intake.IntakeChargeOccurrenceRegistration(context.Background(), bearerRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

type registrarDouble struct {
	result application.RegisterChargeOccurrenceResult
}

func (double registrarDouble) Register(context.Context, application.RegisterChargeOccurrenceCommand) (application.RegisterChargeOccurrenceResult, error) {
	return double.result, nil
}

// registeredResult 走真编排拿一份「已登记」的结果：结果类型的字段不导出，替身造不出带记录的那一格。
func registeredResult(t *testing.T) application.RegisterChargeOccurrenceResult {
	t.Helper()
	result := registeredResultWith(t, perPieceFulfillment(t))
	if result.Outcome() != application.ChargeOccurrenceRegistered {
		t.Fatalf("夹具登记：%q %q", result.Outcome(), result.Detail())
	}
	return result
}

// registeredResultWith 走真编排、按给定的协议条件登记一次干线实际履约。
func registeredResultWith(t *testing.T, conditions domain.AgreementChargeConditions) application.RegisterChargeOccurrenceResult {
	t.Helper()
	tenant := mustTF(t, domain.NewTenantID, operatorTenant)
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  mustTF(t, domain.NewFulfillmentSegmentReference, "SYN-SEG-SZX-SIN-01"),
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     mustTF(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01"),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: mustTF(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX"),
			EnteredAt:  time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC),
			EndKind:    domain.EndedByNextHandover,
			EndBasis:   mustTF(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SIN"),
			EndedAt:    time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := application.NewRegisterChargeOccurrenceHandler(application.RegisterChargeOccurrenceDeps{
		Segments:         segmentSource{record: ports.FulfillmentSegmentRecord{Segment: segment}},
		CarrierJudgments: noJudgment{},
		Adoptions:        noAdoption{},
		Agreements: agreementSource{check: ports.SupplierAgreementCheck{
			Standing:         ports.SupplierAgreementRegistered,
			Snapshot:         mustTF(t, domain.NewAgreementSnapshotReference, "SYN-SUPPLIER-TRUNK-CN-SG-01/v2"),
			LegalEntity:      mustTF(t, domain.NewProcurementLegalEntityReference, "SYN-LE-01"),
			Supplier:         mustTF(t, domain.NewServiceProviderReference, "SYN-PARTY-CARRIER-TRUNK-01"),
			ChargeConditions: conditions,
		}},
		Occurrences: &emptyOccurrences{},
		Handoff:     discardHandoff{},
		Clock:       occurrenceRegistrationClock{at: time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)},
	})
	intake := operatorIntake(t, authenticatorFake{})
	command, err := intake.IntakeChargeOccurrenceRegistration(context.Background(), bearerRequest("t", decisionChargeOccurrenceBody))
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("夹具登记：%v", err)
	}
	return result
}

// Covers: ADR-0022——新落一项答 201 并逐字段回显（业务时间与登记时刻分开、依据按协议声明判、登记者在），其余形成了的
// 答案一律 200；响应里没有金额、币种、价格或费用一类的键（发生项不是价格，CONTEXT）。
func TestChargeOccurrenceEndpointAnswers201OnlyForANewOccurrenceAndCarriesNoAmount(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	endpoint := tfhttp.NewRegisterChargeOccurrenceEndpoint(intake, registrarDouble{result: registeredResult(t)})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", decisionChargeOccurrenceBody))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201：%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	occurrence, _ := body["occurrence"].(map[string]any)
	if body["outcome"] != "REGISTERED" || occurrence["reason"] != "ACTUAL_FULFILLMENT" ||
		occurrence["agreement"] != "SYN-SUPPLIER-TRUNK-CN-SG-01/v2" || occurrence["legalEntity"] != "SYN-LE-01" ||
		occurrence["factBasis"] != "FULFILLMENT-SEGMENT/SYN-SEG-SZX-SIN-01" || occurrence["version"] != "V1" ||
		occurrence["occurredAt"] != "2026-10-19T06:00:00Z" || occurrence["recordedAt"] != "2026-10-19T08:00:00Z" ||
		occurrence["conditionBasis"] != "AGREEMENT_DECLARED" || occurrence["registeredBy"] != operatorSubject ||
		occurrence["quantity"] != float64(1) || occurrence["unit"] != "PIECE" {
		t.Fatalf("响应 = %s", recorder.Body.String())
	}
	if _, asserted := occurrence["conditionAssertion"]; asserted {
		t.Fatalf("按协议判的项回显了断言原话：%s", recorder.Body.String())
	}
	for key := range occurrence {
		for _, forbidden := range []string{"amount", "currency", "price", "cost", "fee"} {
			if strings.Contains(strings.ToLower(key), forbidden) {
				t.Fatalf("响应带了 %q——发生项不是金额", key)
			}
		}
	}

	notApplicable := tfhttp.NewRegisterChargeOccurrenceEndpoint(intake, registrarDouble{result: answeredWithoutRecord(t)})
	recorder = httptest.NewRecorder()
	notApplicable.ServeHTTP(recorder, bearerRequest("t", decisionChargeOccurrenceBody))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"outcome":"INPUT_NOT_ACCEPTED"`) ||
		!strings.Contains(recorder.Body.String(), `"detail":"REQUEST_MALFORMED"`) || strings.Contains(recorder.Body.String(), `"occurrence"`) {
		t.Fatalf("未受理 = %d %s", recorder.Code, recorder.Body.String())
	}
}

// Covers: AT-TF-170——未配置与协议不计此原因都是形成了的答案（200），带回作答的那一版协议，不带发生项。
func TestUnconfiguredAndUnchargedConditionsAnswer200WithTheAgreement(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	uncharged, err := domain.AgreementChargeConditions{}.DeclareAgreementChargeCondition(
		domain.ActualFulfillmentOccurrence, domain.NewUnchargedAgreementCondition())
	if err != nil {
		t.Fatal(err)
	}
	for want, conditions := range map[string]domain.AgreementChargeConditions{
		"NOT_CONFIGURED": {},
		"NOT_CHARGED":    uncharged,
	} {
		endpoint := tfhttp.NewRegisterChargeOccurrenceEndpoint(intake, registrarDouble{result: answeredByConditions(t, conditions)})
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, bearerRequest("t", decisionChargeOccurrenceBody))
		body := recorder.Body.String()
		if recorder.Code != http.StatusOK || !strings.Contains(body, `"outcome":"`+want+`"`) ||
			!strings.Contains(body, `"agreement":"SYN-SUPPLIER-TRUNK-CN-SG-01/v2"`) || strings.Contains(body, `"occurrence"`) {
			t.Fatalf("%s = %d %s", want, recorder.Code, body)
		}
	}
}

func perPieceFulfillment(t *testing.T) domain.AgreementChargeConditions {
	t.Helper()
	charged, err := domain.NewChargedAgreementCondition(domain.PerPieceQuantity)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := domain.AgreementChargeConditions{}.DeclareAgreementChargeCondition(domain.ActualFulfillmentOccurrence, charged)
	if err != nil {
		t.Fatal(err)
	}
	return conditions
}

// answeredByConditions 走真编排拿一份按协议条件作答、不带记录的结果。
func answeredByConditions(t *testing.T, conditions domain.AgreementChargeConditions) application.RegisterChargeOccurrenceResult {
	t.Helper()
	registered := registeredResultWith(t, conditions)
	if _, present := registered.Record(); present {
		t.Fatalf("夹具应不落册：%q", registered.Outcome())
	}
	return registered
}

// answeredWithoutRecord 走真编排拿一份不带记录的答案：订舱原因不指名订舱是坏请求，在读任何事实之前答未受理。
func answeredWithoutRecord(t *testing.T) application.RegisterChargeOccurrenceResult {
	t.Helper()
	handler := application.NewRegisterChargeOccurrenceHandler(application.RegisterChargeOccurrenceDeps{})
	result, err := handler.Register(context.Background(), application.RegisterChargeOccurrenceCommand{
		TenantID:   mustTF(t, domain.NewTenantID, operatorTenant),
		Registrant: operatorSubject,
		Reason:     domain.BookingOccurrence,
		Objects:    []string{"SYN-PARCEL-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustTF[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

type segmentSource struct {
	record ports.FulfillmentSegmentRecord
}

func (source segmentSource) FindByKey(context.Context, ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	return source.record, true, nil
}

type noAdoption struct{}

func (noAdoption) FindBySegment(context.Context, ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	return ports.SegmentCommissionAdoptionRecord{}, false, nil
}

type noJudgment struct{}

func (noJudgment) FindByKey(context.Context, ports.ActualCarrierJudgmentKey) (ports.ActualCarrierJudgmentRecord, bool, error) {
	return ports.ActualCarrierJudgmentRecord{}, false, nil
}

type agreementSource struct{ check ports.SupplierAgreementCheck }

func (source agreementSource) CheckSupplierAgreement(
	context.Context, domain.TenantID, domain.SupplierAgreementVersion, domain.ServiceProviderReference, time.Time,
) (ports.SupplierAgreementCheck, error) {
	return source.check, nil
}

type emptyOccurrences struct{}

func (*emptyOccurrences) FindByKey(context.Context, ports.ChargeOccurrenceKey) (ports.ChargeOccurrenceRecord, bool, error) {
	return ports.ChargeOccurrenceRecord{}, false, nil
}

func (*emptyOccurrences) Save(context.Context, ports.ChargeOccurrenceRecord) (ports.ChargeOccurrenceSaveOutcome, error) {
	return ports.ChargeOccurrenceSaved, nil
}

func (*emptyOccurrences) FindCovering(context.Context, ports.ChargeOccurrenceCoverageQuery) (ports.ChargeOccurrenceRecord, bool, error) {
	return ports.ChargeOccurrenceRecord{}, false, nil
}

type discardHandoff struct{}

func (discardHandoff) HandOffChargeOccurrence(context.Context, ports.ChargeOccurrenceIntent) error {
	return nil
}

type occurrenceRegistrationClock struct{ at time.Time }

func (clock occurrenceRegistrationClock) Now() time.Time { return clock.at }
