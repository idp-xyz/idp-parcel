package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 参与方资料（ADR-0369）的传输层用例：登记口把各项译进命令，形状不对即 MALFORMED_REQUEST；读口交全部修订与此刻有效那一笔。

const partyProfilePath = "/commercial-business-party-profile-registrations"

func TestOperatorRegistryIntakeTranslatesBusinessPartyProfile(t *testing.T) {
	intake := operatorIntake(t)
	command, err := intake.IntakeBusinessPartyProfileRegistration(context.Background(), identityRequest(partyProfilePath, `{"profiles":[{
		"partyId":"SYN-PARTY-02","revision":1,"basis":"SYN-PROFILE-02","effectiveFrom":"2026-09-16T00:00:00Z",
		"names":[{"language":"en","name":"SYN Party Two Ltd."}],
		"correspondenceAddress":{"country":"CN","lines":["上海市 SYN 路 2 号"]},
		"invoiceTitle":"合成参与方二号","invoiceTitleLanguage":"zh-Hans",
		"contacts":[{"name":"林悦","email":"finance@example.invalid","purposes":["FINANCE"],"primaryFor":["FINANCE"]}],
		"bankAccounts":[{"holderName":"合成参与方二号","bankName":"SYN 银行","accountNumber":"6222000000000002","currency":"CNY"}]
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	parts := command.Parts
	if command.Party.String() != "SYN-PARTY-02" || len(parts.Names) != 1 || parts.CorrespondenceAddress == nil || parts.RegisteredAddress != nil ||
		parts.Invoicing == nil || len(parts.Contacts) != 1 || len(parts.BankAccounts) != 1 {
		t.Fatalf("command = %+v", command)
	}
	for name, fields := range map[string]string{
		"带了租户":      `"tenantId":"x",`,
		"名称语言不成立":   `"names":[{"language":"english!","name":"x"}],`,
		"没主抬头带语言版本": `"invoiceTitles":[{"language":"en","title":"x"}],`,
		"币种不在目录":    `"bankAccounts":[{"holderName":"a","bankName":"b","accountNumber":"1","currency":"XXX"}],`,
		"未知键":       `"representative":{"kind":"LEGAL_REPRESENTATIVE","name":"x"},`,
	} {
		t.Run(name, func(t *testing.T) {
			body := `{"profiles":[{` + fields + `"partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]}`
			if name == "带了租户" {
				body = `{"tenantId":"x","profiles":[{"partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]}`
			}
			_, err := intake.IntakeBusinessPartyProfileRegistration(context.Background(), identityRequest(partyProfilePath, body))
			if err == nil {
				t.Fatal("变形载荷被收下了")
			}
			if name != "带了租户" && !errors.Is(err, commercialhttp.ErrMalformedRequest) {
				t.Fatalf("error = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

type partyProfileHistoryDouble struct {
	rows []ports.RecordedBusinessPartyProfileRevision
}

func (double partyProfileHistoryDouble) ListBusinessPartyProfileRevisions(
	context.Context, domain.TenantID, domain.PartyID,
) ([]ports.RecordedBusinessPartyProfileRevision, error) {
	return double.rows, nil
}

func partyProfileAt(t *testing.T, revision int, effectiveFrom time.Time, parts domain.BusinessPartyProfileParts) ports.RecordedBusinessPartyProfileRevision {
	t.Helper()
	content, err := domain.NewBusinessPartyProfileContent(parts)
	if err != nil {
		t.Fatal(err)
	}
	built, err := domain.NewBusinessPartyProfileRevision(catValue(t, domain.NewTenantID, "tenant-1"), catValue(t, domain.NewPartyID, "party-id"),
		revision, catValue(t, domain.NewBusinessPartyProfileBasisReference, "basis"), effectiveFrom, content)
	if err != nil {
		t.Fatal(err)
	}
	return ports.RecordedBusinessPartyProfileRevision{Revision: built, RecordedAt: effectiveFrom}
}

// Covers: 读口交全部修订，此刻有效的是生效时点不晚于此刻里修订号最大的那一笔；没有一笔生效时那一格缺席。
func TestBusinessPartyProfileRevisionsEndpointAnswersTheEffectiveRevision(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	names := domain.BusinessPartyProfileParts{Names: []domain.LocalizedPartyName{{Language: "en", Name: "SYN"}}}
	serve := func(rows []ports.RecordedBusinessPartyProfileRevision) map[string]json.RawMessage {
		recorder := httptest.NewRecorder()
		commercialhttp.NewQueryBusinessPartyProfileRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, partyProfileHistoryDouble{rows: rows}, func() time.Time { return now }).
			ServeHTTP(recorder, func() *http.Request {
				request := httptest.NewRequest(http.MethodGet, "/commercial-business-parties/party-id/profile-revisions", nil)
				request.SetPathValue("partyId", "party-id")
				return request
			}())
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	body := serve([]ports.RecordedBusinessPartyProfileRevision{
		partyProfileAt(t, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), names),
		partyProfileAt(t, 2, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), domain.BusinessPartyProfileParts{}),
		partyProfileAt(t, 3, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), names),
	})
	if string(body["effectiveRevision"]) != "2" {
		t.Fatalf("effectiveRevision = %s", body["effectiveRevision"])
	}
	var revisions []map[string]json.RawMessage
	if err := json.Unmarshal(body["revisions"], &revisions); err != nil || len(revisions) != 3 {
		t.Fatalf("revisions = %s", body["revisions"])
	}
	if _, has := revisions[1]["names"]; has {
		t.Fatal("空正文那一笔不该带 names")
	}
	if string(revisions[1]["contacts"]) != "[]" || string(revisions[1]["invoicingRegistered"]) != "false" {
		t.Fatalf("集合格总是数组、开票资料用显式布尔：%s %s", revisions[1]["contacts"], revisions[1]["invoicingRegistered"])
	}
	future := serve([]ports.RecordedBusinessPartyProfileRevision{partyProfileAt(t, 1, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), names)})
	if _, has := future["effectiveRevision"]; has {
		t.Fatal("没有一笔生效时 effectiveRevision 应缺席")
	}
}
