package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件对真实 PostgreSQL 证委托提交口的生产装配（票 demo-chain-gaps/23）：演示租户的两份供应商协议照种子里的发布批经
// PC 发布用例落库，提交口经操作者渠道的「运营决定」能力面（决定授予 + 准入对照 + 本产品区间）走到真编排。种子从仓库里的
// 发布批读，不在测试里抄一份——两份各改各的，测试就会对着一份种子里已经不存在的协议证明它可用。

const commissionSubmissionPath = "/transport-fulfillment-transport-commission-submissions"

type commissionPublishClock struct{ at time.Time }

func (clock commissionPublishClock) Now() time.Time { return clock.at }

// silentCommercialRegistrationHandoff 收下「参数已登记」意图不入队：本文件证的是委托口，那封信封由 PC 自己的用例证。
type silentCommercialRegistrationHandoff struct{}

func (silentCommercialRegistrationHandoff) HandOffOperatorRegistrationCompleted(context.Context, pcports.OperatorRegistrationCompletedIntent) error {
	return nil
}

// publishSeededSupplierAgreements 把演示发布批里的供应商协议项经 PC 发布用例落库，交回按对象号索引的种子项。解码与
// parcel-commercial CLI 同一形状、同样拒未知字段。
func publishSeededSupplierAgreements(t *testing.T, db *bentopg.DB) map[string]registrationjson.PublicationItemDocument {
	t.Helper()
	raw, err := os.ReadFile("../../scripts/demo-seeds/data/commercial/publish-batch.json")
	if err != nil {
		t.Fatalf("读演示发布批：%v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var batch registrationjson.PublicationBatchDocument
	if err := decoder.Decode(&batch); err != nil {
		t.Fatalf("解码演示发布批：%v", err)
	}
	registry, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	handler := pcapplication.NewPublishCommercialAuthorityHandler(registry,
		commissionPublishClock{at: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}, silentCommercialRegistrationHandoff{})
	agreements := map[string]registrationjson.PublicationItemDocument{}
	for _, item := range batch.Items {
		if item.Kind != "SUPPLIER_AGREEMENT" {
			continue
		}
		command, err := registrationjson.PublicationCommand(item)
		if err != nil {
			t.Fatalf("%s：发布翻译：%v", item.ObjectID, err)
		}
		var result pcapplication.PublishCommercialAuthorityResult
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = handler.Handle(txCtx, command)
			return handleErr
		}); err != nil {
			t.Fatalf("%s：发布：%v", item.ObjectID, err)
		}
		if result.Outcome() != pcapplication.CommercialVersionPublishedEffective {
			t.Fatalf("%s：发布结果 = %s", item.ObjectID, result.Outcome())
		}
		agreements[item.ObjectID] = item
	}
	return agreements
}

// commissionSubmissionEndpointForTest 装生产的委托提交编排与操作者渠道：准入走生产的准入范围桥，合成操作者只持有运输委托
// 提交这一种决定。与生产装配的差别只在核验方与操作者册是替身。
func commissionSubmissionEndpointForTest(t *testing.T, db *bentopg.DB) http.Handler {
	t.Helper()
	submitter, err := buildTransportCommissionSubmission(db)
	if err != nil {
		t.Fatalf("装配委托提交编排：%v", err)
	}
	kinds := []accessidentity.DecisionKind{accessidentity.DecisionTransportCommissionSubmission}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: kinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	return tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, submitter)
}

type commissionAnswer struct {
	Outcome           string `json:"outcome"`
	UndecidedReason   string `json:"undecidedReason"`
	HandoffReference  string `json:"handoffReference"`
	AgreementSupplier string `json:"agreementSupplier"`
	Commission        *struct {
		Agreement   string `json:"agreement"`
		LegalEntity string `json:"legalEntity"`
		Provider    string `json:"provider"`
	} `json:"commission"`
}

func submitCommissionOverTheWire(t *testing.T, endpoint http.Handler, body string) (int, commissionAnswer) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, commissionSubmissionPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var answer commissionAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return response.Code, answer
}

func commissionBody(commission, provider, agreement, version, submittedAt string) string {
	return `{"commission": "` + commission + `", "provider": "` + provider + `", "agreement": "` + agreement +
		`", "agreementVersion": "` + version + `", "conditions": "SYN-CONDITIONS-TRUNK-01", "role": "SYN-ROLE-CONTRACTED-CARRIER",` +
		` "responsibility": "SYN-RESPONSIBILITY-TRUNK-01", "members": ["SYN-PARCEL-DCG23-01"], "submittedAt": "` + submittedAt + `"}`
}

// Covers: `AT-TF-105`、`AT-TF-106`、`AT-TF-107` 保存半边（真库面），票 demo-chain-gaps/23 完成判据（S）——
//   - 演示：干线委托采用种子里的干线协议、服务方填干线承运方，答 201 COMMISSION_SUBMITTED，快照是 PC 拼出的
//     「对象/版本」、采购责任法人是种子里协议正文的法人；同一份原样重交答 200 EXISTING_COMMISSION（首笔事务真的提交了）。
//   - 委托与 Outbox 同一笔：委托行与「委托已提交」信封同一个事务号（xmin 相同），信封载荷只带引用。
//   - 演示：服务方填末端承运商答 200 PROVIDER_MISMATCH，带协议约定的供应商；没发布的版本答 AGREEMENT_NOT_REGISTERED；
//     生效前的提交时刻答 AGREEMENT_NOT_EFFECTIVE。这几格都零写入。
func TestTheWiredTransportCommissionSubmissionChecksTheSeededAgreements(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk, lastMile := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"], seeded["SYN-SUPPLIER-LASTMILE-SG-01"]
	if trunk.Declarations == nil || trunk.Declarations.SupplierAgreementBody == nil ||
		lastMile.Declarations == nil || lastMile.Declarations.SupplierAgreementBody == nil {
		t.Fatal("演示发布批里缺干线或末端供应商协议的正文")
	}
	trunkBody := trunk.Declarations.SupplierAgreementBody
	endpoint := commissionSubmissionEndpointForTest(t, db)

	submitted := commissionBody("SYN-COMMISSION-TRUNK-01", trunkBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")
	status, answer := submitCommissionOverTheWire(t, endpoint, submitted)
	if status != http.StatusCreated || answer.Outcome != "COMMISSION_SUBMITTED" || answer.Commission == nil {
		t.Fatalf("干线委托 = %d %+v", status, answer)
	}
	if answer.Commission.Agreement != trunk.ObjectID+"/"+trunk.Version || answer.Commission.LegalEntity != trunkBody.LegalEntity {
		t.Fatalf("快照与法人 = %q %q，want 种子里的 %s/%s 与 %s", answer.Commission.Agreement, answer.Commission.LegalEntity,
			trunk.ObjectID, trunk.Version, trunkBody.LegalEntity)
	}
	if answer.HandoffReference != "" {
		t.Fatalf("意图应已与委托同笔入队，却留了续办引用 %q", answer.HandoffReference)
	}
	if status, replay := submitCommissionOverTheWire(t, endpoint, submitted); status != http.StatusOK || replay.Outcome != "EXISTING_COMMISSION" {
		t.Fatalf("原样重交 = %d %+v——首笔事务没有提交", status, replay)
	}

	var commissionTx, envelopeTx uint32
	var legalEntity, eventType, payload string
	if err := pool.QueryRow(t.Context(),
		`SELECT xmin::text::bigint, procurement_legal_entity_ref FROM transport_fulfillment.transport_commission
		  WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = 'SYN-COMMISSION-TRUNK-01'`,
	).Scan(&commissionTx, &legalEntity); err != nil {
		t.Fatalf("读委托行：%v", err)
	}
	eventID := outboxintent.FingerprintEventID("transport-commission", "SYN-TENANT-01", "SYN-COMMISSION-TRUNK-01")
	if err := pool.QueryRow(t.Context(),
		`SELECT xmin::text::bigint, event_type, payload::text FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, string(eventID),
	).Scan(&envelopeTx, &eventType, &payload); err != nil {
		t.Fatalf("读委托信封：%v", err)
	}
	if commissionTx != envelopeTx {
		t.Fatalf("委托行与信封不在同一笔：xmin %d / %d", commissionTx, envelopeTx)
	}
	if legalEntity != trunkBody.LegalEntity || eventType != "transport-fulfillment.transport-commission.submitted" ||
		!strings.Contains(payload, `"commission": "SYN-COMMISSION-TRUNK-01"`) {
		t.Fatalf("库里：法人 %q，信封 %s %s", legalEntity, eventType, payload)
	}

	refusals := map[string]struct {
		body     string
		outcome  string
		supplier string
	}{
		"末端承运商填进干线协议": {
			commissionBody("SYN-COMMISSION-TRUNK-02", lastMile.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
			"PROVIDER_MISMATCH", trunkBody.Supplier,
		},
		"没发布的版本": {
			commissionBody("SYN-COMMISSION-TRUNK-03", trunkBody.Supplier, trunk.ObjectID, "v9", "2026-10-19T08:00:00Z"),
			"AGREEMENT_NOT_REGISTERED", "",
		},
		"生效前提交": {
			commissionBody("SYN-COMMISSION-TRUNK-04", trunkBody.Supplier, trunk.ObjectID, trunk.Version, "2025-12-31T23:00:00Z"),
			"AGREEMENT_NOT_EFFECTIVE", "",
		},
	}
	for name, refusal := range refusals {
		status, answer := submitCommissionOverTheWire(t, endpoint, refusal.body)
		if status != http.StatusOK || answer.Outcome != refusal.outcome || answer.AgreementSupplier != refusal.supplier || answer.Commission != nil {
			t.Fatalf("%s = %d %+v, want 200 %s", name, status, answer, refusal.outcome)
		}
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.transport_commission`).Scan(&rows); err != nil {
		t.Fatalf("数委托行：%v", err)
	}
	if rows != 1 {
		t.Fatalf("委托表 %d 行，want 1——核不过协议的提交落了行", rows)
	}
}
