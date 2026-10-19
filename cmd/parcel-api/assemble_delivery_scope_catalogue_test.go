package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// unwiredDeliveryScopeRecords 是送站预报与送站交接单查阅读口的占位：装配测试用它证四行都接到了读口、且未配置 Intake 拒在读口之前。
type unwiredDeliveryScopeRecords struct{}

func (unwiredDeliveryScopeRecords) ListDeliveryPreAdvices(context.Context, nodomain.TenantID, noports.DeliveryScopeCatalogueFilter, int) ([]nodomain.DeliveryPreAdviceState, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredDeliveryScopeRecords) FindDeliveryPreAdvice(context.Context, nodomain.TenantID, nodomain.DeliveryPreAdviceID) (nodomain.DeliveryPreAdviceState, bool, error) {
	return nodomain.DeliveryPreAdviceState{}, false, errOrchestrationNotWired
}

func (unwiredDeliveryScopeRecords) ListDeliveryHandoverNotes(context.Context, nodomain.TenantID, noports.DeliveryScopeCatalogueFilter, int) ([]noports.DeliveryHandoverNoteCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredDeliveryScopeRecords) FindDeliveryHandoverNoteDetail(context.Context, nodomain.TenantID, nodomain.DeliveryHandoverNoteID) (noports.DeliveryHandoverNoteDetail, bool, error) {
	return noports.DeliveryHandoverNoteDetail{}, false, errOrchestrationNotWired
}

// Covers: 票 ps-external-mark-relations/13 完成判据「读口用例：预报与交接单的列表与详情，跨租户与查无同答」（真库，经 main 同一个读口构造与
// 操作者查阅读 Intake）：持查阅读授予的合成操作者只看得到信封租户 SYN-TENANT-01 的预报与交接单；指名只在 SYN-TENANT-02 登记的那一份，与
// 指名一个谁都没登记的标识，答复逐字节相同（404 + 同一个码）；不带令牌答 401、不读册。测试输入是隔离合成，只记 `S`。
func TestDeliveryScopeReadsAnswerOnlyTheEnvelopeTenantAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := nopostgres.NewDeliveryScopeCatalogue(db)
	if err != nil {
		t.Fatalf("构造查阅读口：%v", err)
	}
	reader, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatalf("构造查阅读铸造器：%v", err)
	}
	readIntakes, err := buildOperatorRegistryIntakes(reader)
	if err != nil {
		t.Fatalf("构造查阅读 Intake：%v", err)
	}
	seedDeliveryScopeRegisters(t, db)

	router := httpapi.NewWithEndpoints(buildinfo.Info{}, []httpapi.BusinessEndpoint{
		{Pattern: "/node-operations-delivery-pre-advices", Handler: nodeopshttp.NewQueryDeliveryPreAdvicesEndpoint(readIntakes.nodeOperationsRead, catalogue)},
		{Pattern: "/node-operations-delivery-pre-advices/{preAdviceId}", Handler: nodeopshttp.NewQueryDeliveryPreAdviceEndpoint(readIntakes.nodeOperationsRead, catalogue)},
		{Pattern: "/node-operations-delivery-handover-notes", Handler: nodeopshttp.NewQueryDeliveryHandoverNotesEndpoint(readIntakes.nodeOperationsRead, catalogue)},
		{Pattern: "/node-operations-delivery-handover-notes/{noteId}", Handler: nodeopshttp.NewQueryDeliveryHandoverNoteEndpoint(readIntakes.nodeOperationsRead, catalogue)},
	})
	get := func(target, token string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	preAdvices := get("/node-operations-delivery-pre-advices?node=SYN-NODE-PSXM13", presentedOperatorToken)
	var preAdviceList struct {
		PreAdvices []struct {
			PreAdviceID string `json:"preAdviceId"`
			Withdrawn   bool   `json:"withdrawn"`
		} `json:"preAdvices"`
	}
	if preAdvices.Code != http.StatusOK || json.Unmarshal(preAdvices.Body.Bytes(), &preAdviceList) != nil ||
		len(preAdviceList.PreAdvices) != 1 || preAdviceList.PreAdvices[0].PreAdviceID != "SYN-PREADV-PSXM13-API-1" {
		t.Fatalf("预报列表 = %d %s, want 只有信封租户那一份", preAdvices.Code, preAdvices.Body.String())
	}
	notes := get("/node-operations-delivery-handover-notes", presentedOperatorToken)
	var noteList struct {
		HandoverNotes []struct {
			NoteID           string `json:"noteId"`
			ScopeEstablished bool   `json:"scopeEstablished"`
		} `json:"handoverNotes"`
	}
	if notes.Code != http.StatusOK || json.Unmarshal(notes.Body.Bytes(), &noteList) != nil ||
		len(noteList.HandoverNotes) != 1 || noteList.HandoverNotes[0].NoteID != "SYN-HN-PSXM13-API-1" || !noteList.HandoverNotes[0].ScopeEstablished {
		t.Fatalf("交接单列表 = %d %s, want 只有信封租户那一张、范围已成立", notes.Code, notes.Body.String())
	}
	if own := get("/node-operations-delivery-handover-notes/SYN-HN-PSXM13-API-1", presentedOperatorToken); own.Code != http.StatusOK {
		t.Fatalf("本租户的交接单详情 = %d %s", own.Code, own.Body.String())
	}
	if own := get("/node-operations-delivery-pre-advices/SYN-PREADV-PSXM13-API-1", presentedOperatorToken); own.Code != http.StatusOK {
		t.Fatalf("本租户的预报详情 = %d %s", own.Code, own.Body.String())
	}

	for _, pair := range [][2]string{
		{"/node-operations-delivery-pre-advices/SYN-PREADV-PSXM13-API-2", "/node-operations-delivery-pre-advices/SYN-PREADV-PSXM13-API-UNKNOWN"},
		{"/node-operations-delivery-handover-notes/SYN-HN-PSXM13-API-2", "/node-operations-delivery-handover-notes/SYN-HN-PSXM13-API-UNKNOWN"},
	} {
		elsewhere, unknown := get(pair[0], presentedOperatorToken), get(pair[1], presentedOperatorToken)
		if elsewhere.Code != http.StatusNotFound || elsewhere.Code != unknown.Code || elsewhere.Body.String() != unknown.Body.String() {
			t.Fatalf("跨租户 %d %s 与查无 %d %s 不同答", elsewhere.Code, elsewhere.Body.String(), unknown.Code, unknown.Body.String())
		}
		assertNoOutcome(t, elsewhere, pair[0])
	}
	if anonymous := get("/node-operations-delivery-pre-advices", ""); anonymous.Code != http.StatusUnauthorized || problemCode(t, anonymous) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("不带令牌 = %d %s", anonymous.Code, anonymous.Body.String())
	}
}

// seedDeliveryScopeRegisters 在两个租户各落一份预报与一张交接单：SYN-TENANT-01 那张两个声称都已确认，SYN-TENANT-02 的标识只在那边登记。
func seedDeliveryScopeRegisters(t *testing.T, db *bentopg.DB) {
	t.Helper()
	preAdvices, err := nopostgres.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	preAdviceIn := func(tenant, id string) nodomain.DeliveryPreAdvice {
		preAdvice, err := nodomain.FormDeliveryPreAdvice(nodomain.DeliveryPreAdviceSpec{
			TenantID:          mustValue(t, nodomain.NewTenantID, tenant),
			ID:                mustValue(t, nodomain.NewDeliveryPreAdviceID, id),
			CustomerAccount:   mustValue(t, nodomain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
			Node:              mustValue(t, nodomain.NewNodeReference, "SYN-NODE-PSXM13"),
			ExpectedArrivalAt: at.Add(time.Hour),
			Numbers:           []nodomain.PreAdvisedParcelNumber{mustValue(t, nodomain.NewPreAdvisedParcelNumber, "SYN-NUMBER-PSXM13-API")},
			Source:            nodomain.CustomerRequestSource{Channel: "https://id.syn.example/dex#syn-client-psxm13", RequestKey: "SYN-REQUEST-" + id, ContentDigest: "sha256:" + id},
			FormedAt:          at,
		})
		if err != nil {
			t.Fatal(err)
		}
		return preAdvice
	}
	noteIn := func(tenant, id string, accounts ...string) nodomain.DeliveryHandoverNote {
		claimed := make([]nodomain.CustomerAccountReference, 0, len(accounts))
		for _, account := range accounts {
			claimed = append(claimed, mustValue(t, nodomain.NewCustomerAccountReference, account))
		}
		note, err := nodomain.OpenDeliveryHandoverNote(nodomain.DeliveryHandoverNoteSpec{
			TenantID:        mustValue(t, nodomain.NewTenantID, tenant),
			ID:              mustValue(t, nodomain.NewDeliveryHandoverNoteID, id),
			Node:            mustValue(t, nodomain.NewNodeReference, "SYN-NODE-PSXM13"),
			DeliveringParty: mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM13"),
			ClaimedAccounts: claimed,
			OpenedAt:        at,
		})
		if err != nil {
			t.Fatal(err)
		}
		return note
	}
	confirmed, err := nodomain.FormDeliveryHandoverBinding(nodomain.DeliveryHandoverBindingSpec{
		TenantID:              mustValue(t, nodomain.NewTenantID, "SYN-TENANT-01"),
		Note:                  mustValue(t, nodomain.NewDeliveryHandoverNoteID, "SYN-HN-PSXM13-API-1"),
		Account:               mustValue(t, nodomain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Conclusion:            nodomain.BindingConfirmed,
		Route:                 nodomain.ConcludedViaOperationDecision,
		Concluder:             "OPERATOR/https://id.syn.example/dex#syn-operator-psxm13",
		DeliveringParticipant: "SYN-PARTY-DELIVERER-01",
		Basis:                 "SYN-BASIS-PSXM13-API",
		ConcludedAt:           at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		for _, preAdvice := range []nodomain.DeliveryPreAdvice{
			preAdviceIn("SYN-TENANT-01", "SYN-PREADV-PSXM13-API-1"), preAdviceIn("SYN-TENANT-02", "SYN-PREADV-PSXM13-API-2"),
		} {
			if _, _, err := preAdvices.RecordDeliveryPreAdvice(ctx, preAdvice); err != nil {
				return err
			}
		}
		for _, note := range []nodomain.DeliveryHandoverNote{
			noteIn("SYN-TENANT-01", "SYN-HN-PSXM13-API-1", "SYN-ACCOUNT-01"), noteIn("SYN-TENANT-02", "SYN-HN-PSXM13-API-2", "SYN-ACCOUNT-02"),
		} {
			if _, _, err := notes.OpenDeliveryHandoverNote(ctx, note); err != nil {
				return err
			}
		}
		_, _, err := notes.RecordDeliveryHandoverBinding(ctx, confirmed)
		return err
	}); err != nil {
		t.Fatalf("落两本册：%v", err)
	}
}
