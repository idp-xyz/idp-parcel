package main

// 票 tf-face-algebra/11 的真渠道真库回归：监管凭证登记口按登记册存得下的值判。链路同票 operator-channel/52 那份回归
// （integrationClientChannelOnRealDB）：替身核验方 → 生产准入桥 → 生产 Intake → 生产编排 → 真库。登记册的时刻列只存到
// 微秒、额度列是 integer、文本列存不下 NUL——写得进的读回来要认得出是同一张，存不进的要答「改输入」，不许答 500。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
)

// registrationAnswer 交回状态码与答复里那一格：成功答 outcome，问题答 error.code。
func registrationAnswer(t *testing.T, endpoint http.Handler, body string) (int, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/customs-regulatory-credential-registrations", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.client.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var answer struct {
		Outcome string `json:"outcome"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode answer %s: %v", response.Body.Bytes(), err)
	}
	if answer.Outcome != "" {
		return response.Code, answer.Outcome
	}
	return response.Code, answer.Error.Code
}

type credentialFields struct {
	id, issuer, holder, validFrom, validTo, uses string
}

func (fields credentialFields) body() string {
	return `{"credentialId":"` + fields.id + `","issuerRef":"` + fields.issuer + `","holderRef":"` + fields.holder + `",` +
		`"procedureRef":"SYN-PROCEDURE/import-general","validFrom":"` + fields.validFrom + `","validTo":"` + fields.validTo + `",` +
		`"uses":` + fields.uses + `}`
}

func ordinaryCredential(id string) credentialFields {
	return credentialFields{id: id, issuer: "SYN-AUTHORITY/customs-sg", holder: "SYN-HOLDER/broker-tfa11",
		validFrom: "2026-01-01T00:00:00Z", validTo: "2027-01-01T00:00:00Z", uses: "3"}
}

func TestIntegrationClientCredentialRegistrationJudgesStorableValuesOnRealDB(t *testing.T) {
	intakes, db, count := integrationClientChannelOnRealDB(t, accessidentity.FactRegulatoryCredential)
	registration, err := buildCustomsRegistrationOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := customshttp.NewRegisterRegulatoryCredentialEndpoint(intakes.customs, registration.regulatoryCredential)
	rowsOf := func(id string) int {
		return count(`SELECT count(*) FROM customs_compliance.regulatory_credential WHERE credential_id = $1`, id)
	}

	t.Run("微秒以下有非零位的有效期原样重放", func(t *testing.T) {
		for validFrom, oneMicrosecondLater := range map[string]string{
			"2026-01-01T00:00:00.1234567Z":   "2026-01-01T00:00:00.1234577Z",
			"2026-01-01T00:00:00.123456789Z": "2026-01-01T00:00:00.123457789Z",
			"2026-01-01T00:00:00.000000001Z": "2026-01-01T00:00:00.000001001Z",
		} {
			fields := ordinaryCredential("SYN-CRED-TFA11-" + validFrom)
			fields.validFrom = validFrom
			if status, answer := registrationAnswer(t, endpoint, fields.body()); status != http.StatusCreated {
				t.Fatalf("%s 首登 = %d %s，want 201", validFrom, status, answer)
			}
			if status, answer := registrationAnswer(t, endpoint, fields.body()); status != http.StatusOK || answer != "EXISTING" {
				t.Errorf("%s 原样重放 = %d %s，want 200 EXISTING", validFrom, status, answer)
			}
			later := fields
			later.validFrom = oneMicrosecondLater
			if status, answer := registrationAnswer(t, endpoint, later.body()); status != http.StatusOK || answer != "CONTENT_CONFLICT" {
				t.Errorf("%s 起点晚一微秒 = %d %s，want 200 CONTENT_CONFLICT", validFrom, status, answer)
			}
			if rows := rowsOf(fields.id); rows != 1 {
				t.Errorf("%s 册上 %d 行，want 1", validFrom, rows)
			}
		}
	})

	notAccepted := map[string]credentialFields{}
	sameMicrosecond := ordinaryCredential("SYN-CRED-TFA11-SAME-MICRO")
	sameMicrosecond.validFrom, sameMicrosecond.validTo = "2026-01-01T00:00:00.0000001Z", "2026-01-01T00:00:00.0000009Z"
	notAccepted["有效期两端落在同一微秒"] = sameMicrosecond
	nearZero := ordinaryCredential("SYN-CRED-TFA11-NEAR-ZERO")
	nearZero.validFrom = "0001-01-01T00:00:00.0000001Z"
	notAccepted["起点在零时刻之后不足一微秒"] = nearZero
	beyondColumn := ordinaryCredential("SYN-CRED-TFA11-USES")
	beyondColumn.uses = "2147483648"
	notAccepted["额度超出额度列的上限"] = beyondColumn
	for name, fields := range notAccepted {
		t.Run(name, func(t *testing.T) {
			for attempt := 1; attempt <= 2; attempt++ {
				if status, answer := registrationAnswer(t, endpoint, fields.body()); status != http.StatusOK || answer != "NOT_ACCEPTED" {
					t.Errorf("第 %d 遍 = %d %s，want 200 NOT_ACCEPTED", attempt, status, answer)
				}
			}
			if rows := rowsOf(fields.id); rows != 0 {
				t.Errorf("册上 %d 行，want 零", rows)
			}
		})
	}

	// 含 NUL 的引用在译装时由必备值构造拒，与空白引用同一条路：答的是空白引用那一格，不是 500。
	blank := ordinaryCredential(" ")
	blankStatus, blankAnswer := registrationAnswer(t, endpoint, blank.body())
	if blankStatus == http.StatusInternalServerError {
		t.Fatalf("对照：空白凭证身份答 %d %s", blankStatus, blankAnswer)
	}
	withNUL := map[string]credentialFields{}
	nulID := ordinaryCredential(`SYN-CRED-TFA11-NUL\u0000`)
	withNUL["凭证身份"] = nulID
	nulIssuer := ordinaryCredential("SYN-CRED-TFA11-NUL-ISSUER")
	nulIssuer.issuer = `SYN-AUTHORITY\u0000/customs-sg`
	withNUL["签发机构"] = nulIssuer
	nulHolder := ordinaryCredential("SYN-CRED-TFA11-NUL-HOLDER")
	nulHolder.holder = `SYN-HOLDER\u0000/broker-tfa11`
	withNUL["持有人"] = nulHolder
	for name, fields := range withNUL {
		t.Run(name+"带 NUL", func(t *testing.T) {
			if status, answer := registrationAnswer(t, endpoint, fields.body()); status != blankStatus || answer != blankAnswer {
				t.Errorf("答 %d %s，want 与空白引用同一格 %d %s", status, answer, blankStatus, blankAnswer)
			}
		})
	}
	if rows := count(`SELECT count(*) FROM customs_compliance.regulatory_credential WHERE credential_id LIKE 'SYN-CRED-TFA11-NUL%'`); rows != 0 {
		t.Errorf("带 NUL 的登记册上 %d 行，want 零", rows)
	}
}
