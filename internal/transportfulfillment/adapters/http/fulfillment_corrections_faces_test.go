package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证更正两口的线格式（票 demo-chain-gaps/57，ADR-0232 决定一、四）：各按自己那一种运营决定认证，租户与操作者取认证结果，
// 载荷封闭；只有真落了更正才答 201。

// keyedOccurrences 是按键存取的发生项册替身：失效编排只沿链按键读、落新版本，不问覆盖。
type keyedOccurrences struct {
	records map[ports.ChargeOccurrenceKey]ports.ChargeOccurrenceRecord
}

func (double *keyedOccurrences) FindByKey(_ context.Context, key ports.ChargeOccurrenceKey) (ports.ChargeOccurrenceRecord, bool, error) {
	record, found := double.records[key]
	return record, found, nil
}

func (double *keyedOccurrences) Save(_ context.Context, record ports.ChargeOccurrenceRecord) (ports.ChargeOccurrenceSaveOutcome, error) {
	if _, exists := double.records[record.Key]; exists {
		return ports.ChargeOccurrenceAlreadyRegistered, nil
	}
	double.records[record.Key] = record
	return ports.ChargeOccurrenceSaved, nil
}

func (*keyedOccurrences) FindCovering(context.Context, ports.ChargeOccurrenceCoverageQuery) (ports.ChargeOccurrenceRecord, bool, error) {
	return ports.ChargeOccurrenceRecord{}, false, nil
}

type freeScopes struct{}

func (freeScopes) HoldOccurrenceScope(context.Context, domain.TenantID, domain.ChargeOccurrenceReason, domain.OccurrenceScopeReference) error {
	return nil
}

// occurrencesWithFirstVersion 交一本只有一项首版发生项的册，键是登记口线格式用例那一份。
func occurrencesWithFirstVersion(t *testing.T) (*keyedOccurrences, ports.ChargeOccurrenceKey) {
	t.Helper()
	registered := registeredResult(t)
	record, _ := registered.Record()
	return &keyedOccurrences{records: map[ports.ChargeOccurrenceKey]ports.ChargeOccurrenceRecord{record.Key: record}}, record.Key
}

// Covers: AT-TF-311 的线格式半边——失效口按「运输收费发生项失效」认证，租户与操作者取认证结果；新落失效版本答 201 并回显修订五件，
// 重放与其余各格答 200。载荷夹带租户或版本号以外的键一律 400。
func TestTheInvalidationFaceTranslatesAndAnswers201OnlyForANewInvalidation(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	intake := operatorIntake(t, authenticatorFake{asked: &asked})
	occurrences, key := occurrencesWithFirstVersion(t)
	invalidator := application.NewInvalidateChargeOccurrenceHandler(application.InvalidateChargeOccurrenceDeps{
		Occurrences: occurrences,
		Scopes:      freeScopes{},
		Handoff:     discardHandoff{},
		Clock:       occurrenceRegistrationClock{at: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)},
	})
	endpoint := tfhttp.NewInvalidateChargeOccurrenceEndpoint(intake, invalidator)
	body := `{"occurrence":"` + key.Occurrence.String() + `","version":"V1","basis":"SYN-EVIDENCE-WRONG-ADOPTION"}`

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", body))
	if recorder.Code != http.StatusCreated || len(asked) != 1 || asked[0] != tfhttp.OperatorDecisionChargeOccurrenceInvalidation {
		t.Fatalf("失效 = %d %s，问的决定 %v", recorder.Code, recorder.Body.String(), asked)
	}
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	occurrence, _ := answer["occurrence"].(map[string]any)
	if answer["outcome"] != "OCCURRENCE_INVALIDATED" || occurrence["version"] != "V2" || occurrence["corrects"] != "V1" ||
		occurrence["revisionKind"] != "INVALIDATED" || occurrence["revisionBasis"] != "SYN-EVIDENCE-WRONG-ADOPTION" ||
		occurrence["revisedAt"] != "2026-10-21T09:00:00Z" || occurrence["revisedBy"] != operatorSubject {
		t.Fatalf("失效回显 = %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", body))
	if recorder.Code != http.StatusOK || !jsonHas(t, recorder, "outcome", "EXISTING_INVALIDATION") {
		t.Fatalf("重放 = %d %s", recorder.Code, recorder.Body.String())
	}
	for name, malformed := range map[string]string{
		"夹带租户": `{"occurrence":"TCO-1","version":"V1","basis":"B","tenantId":"SYN-TENANT-02"}`,
		"夹带新版": `{"occurrence":"TCO-1","version":"V1","basis":"B","newVersion":"V2"}`,
	} {
		if _, err := intake.IntakeChargeOccurrenceInvalidation(context.Background(), bearerRequest("t", malformed)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

func jsonHas(t *testing.T, recorder *httptest.ResponseRecorder, key, want string) bool {
	t.Helper()
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	return answer[key] == want
}
