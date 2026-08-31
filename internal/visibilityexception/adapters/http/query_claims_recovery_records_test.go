package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// claimsReaderDouble 按注入行作答，并记录收到的租户与页大小。
type claimsReaderDouble struct {
	tenant domain.TenantID
	limit  int

	notifications []ports.CustomerNotificationCatalogueRow
	items         []ports.ClaimItemCatalogueRow
	matters       []ports.RecoveryMatterCatalogueRow
	err           error
}

func (double *claimsReaderDouble) record(tenant domain.TenantID, limit int) error {
	double.tenant = tenant
	double.limit = limit
	return double.err
}

func (double *claimsReaderDouble) ListCustomerNotifications(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CustomerNotificationCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.notifications, nil
}

func (double *claimsReaderDouble) ListClaimItems(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ClaimItemCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.items, nil
}

func (double *claimsReaderDouble) ListRecoveryMatters(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.RecoveryMatterCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.matters, nil
}

// unreachableClaimsReader 是「被调即失败」的替身。
type unreachableClaimsReader struct{ t *testing.T }

func (reader unreachableClaimsReader) fail() {
	reader.t.Error("a request passed the unconfigured intake and reached the claims reader")
}

func (reader unreachableClaimsReader) ListCustomerNotifications(
	context.Context, domain.TenantID, int,
) ([]ports.CustomerNotificationCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableClaimsReader) ListClaimItems(
	context.Context, domain.TenantID, int,
) ([]ports.ClaimItemCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableClaimsReader) ListRecoveryMatters(
	context.Context, domain.TenantID, int,
) ([]ports.RecoveryMatterCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

var claimsRegistries = []string{"customer-notification", "claim-item", "recovery-matter"}

func crServe(
	t *testing.T,
	intake visibilityhttp.OperationsTrackingIntake,
	reader visibilityhttp.ClaimsRecoveryReviewReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryClaimsRecoveryRecordsEndpoint(intake, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

// Covers: registry 是传输形状——缺席或集外按坏请求拒；方法检查同级先行；未配置
// Intake 对三本册子同答 403，读口不被触到。
func TestClaimsRecoveryRecordsRefuseWrongMethodForeignRegistryAndUnconfigured(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}

	wrongMethod := crServe(t, intake, &claimsReaderDouble{},
		http.MethodPost, "/claims-recovery-records?registry=claim-item")
	if wrongMethod.Code != http.StatusMethodNotAllowed ||
		wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405/GET",
			wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	for name, target := range map[string]string{
		"registry 缺席": "/claims-recovery-records",
		"registry 集外": "/claims-recovery-records?registry=claim-material-receipt",
	} {
		response := crServe(t, intake, &claimsReaderDouble{}, http.MethodGet, target)
		if response.Code != http.StatusBadRequest ||
			problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：status = %d body = %s，want 400 MALFORMED_REQUEST",
				name, response.Code, response.Body.String())
		}
	}

	for _, registry := range claimsRegistries {
		response := crServe(t, visibilityhttp.UnconfiguredIntake{},
			unreachableClaimsReader{t: t},
			http.MethodGet, "/claims-recovery-records?registry="+registry)
		if response.Code != http.StatusForbidden ||
			problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("registry=%s：status = %d body = %s，want 403 ACCESS_CHANNEL_NOT_CONFIGURED",
				registry, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
}

// Covers: 通知册 200 + 行体逐字段透出——里程碑整列照登记转写为数组（不折成单值），
// 尚无进展的行交回空数组而不是缺字段；作用域与页大小来自 Intake 裁决。
func TestClaimsRecoveryRecordsListCustomerNotificationsVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	deadline := operationsBaseAt.Add(72 * time.Hour)
	reader := &claimsReaderDouble{
		notifications: []ports.CustomerNotificationCatalogueRow{
			{
				NotificationID: "notification-1",
				Customer:       "CUST-01",
				Episode:        "episode-1",
				DecidedAt:      operationsBaseAt,
				Policy:         "disclose/v1",
				Content:        "content/v3",
				Deadline:       deadline,
				Channel:        "EMAIL",
				Obligation:     "ACK_REQUIRED",
				Milestones: []ports.NotificationMilestoneNode{
					{Milestone: "GENERATED", RecordedAt: operationsBaseAt},
					{Milestone: "DELIVERED", RecordedAt: operationsBaseAt.Add(time.Hour)},
				},
			},
			{
				NotificationID: "notification-2",
				Customer:       "CUST-02",
				Episode:        "episode-2",
				DecidedAt:      operationsBaseAt,
				Policy:         "disclose/v1",
				Content:        "content/v1",
				Deadline:       deadline,
				Channel:        "WEBHOOK",
				Obligation:     "DELIVERY_REQUIRED",
			},
		},
	}

	response := crServe(t, intake, reader,
		http.MethodGet, "/claims-recovery-records?registry=customer-notification")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome       string `json:"outcome"`
		Notifications []struct {
			NotificationID string `json:"notificationId"`
			Customer       string `json:"customer"`
			Episode        string `json:"episode"`
			DecidedAt      string `json:"decidedAt"`
			Content        string `json:"content"`
			Deadline       string `json:"deadline"`
			Channel        string `json:"channel"`
			Obligation     string `json:"obligation"`
			Milestones     []struct {
				Milestone  string `json:"milestone"`
				RecordedAt string `json:"recordedAt"`
			} `json:"milestones"`
		} `json:"notifications"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "CUSTOMER_NOTIFICATIONS_LISTED" || len(body.Notifications) != 2 {
		t.Fatalf("outcome = %q notifications = %d", body.Outcome, len(body.Notifications))
	}
	progressed := body.Notifications[0]
	if progressed.NotificationID != "notification-1" || progressed.Customer != "CUST-01" ||
		progressed.DecidedAt != operationsBaseAt.Format(time.RFC3339Nano) ||
		progressed.Content != "content/v3" || progressed.Channel != "EMAIL" ||
		progressed.Obligation != "ACK_REQUIRED" ||
		len(progressed.Milestones) != 2 ||
		progressed.Milestones[1].Milestone != "DELIVERED" {
		t.Fatalf("通知行走样：%+v", progressed)
	}
	// 尚无任何送达进展：空数组是如实答案，不是缺字段——义务是否履行由序列回答。
	if body.Notifications[1].Milestones == nil || len(body.Notifications[1].Milestones) != 0 {
		t.Fatalf("无进展行的里程碑应为空数组：%+v", body.Notifications[1])
	}
	if reader.tenant.String() != "tenant-1" || reader.limit != 50 {
		t.Fatalf("读口收到 tenant=%q limit=%d，应来自 Intake 裁决（tenant-1/50）",
			reader.tenant, reader.limit)
	}
}

// Covers: 索赔项册 200 + 行体逐字段透出——满生命周期行（初筛/补充/结论/复核替代/
// 撤回）与裸受理行对照，缺席键即答案；金额键结构上不存在。
func TestClaimsRecoveryRecordsListClaimItemsVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	supplementDeadline := operationsBaseAt.Add(120 * time.Hour)
	concludedAt := operationsBaseAt.Add(24 * time.Hour)
	reviewBy := operationsBaseAt.Add(240 * time.Hour)
	withdrawnAt := operationsBaseAt.Add(300 * time.Hour)
	reader := &claimsReaderDouble{
		items: []ports.ClaimItemCatalogueRow{
			{
				Batch:              "batch-1",
				ItemID:             "item-1",
				Customer:           "CUST-01",
				Applicant:          "applicant-7",
				Contract:           "CONTRACT-01",
				Target:             "parcel-1",
				Kind:               "DAMAGE",
				SubmittedAt:        operationsBaseAt,
				Revision:           2,
				Screen:             "AWAITING_SUPPLEMENT",
				ScreenBasis:        "claims/v1",
				MissingMaterials:   "materials/photos",
				SupplementScope:    "scope/outer-box",
				SupplementNotice:   "notice/1",
				SupplementDeadline: &supplementDeadline,
				DeadlineVersions:   2,
				Conclusion:         "LIABILITY_CONFIRMED",
				ConcludedAt:        &concludedAt,
				ReviewBy:           &reviewBy,
				PriorConclusion:    "LIABILITY_DENIED",
				Withdrawn:          true,
				WithdrawnAt:        &withdrawnAt,
			},
			{
				Batch:       "batch-2",
				ItemID:      "item-2",
				Customer:    "CUST-02",
				Contract:    "CONTRACT-02",
				Target:      "parcel-2",
				Kind:        "LOSS",
				SubmittedAt: operationsBaseAt,
				Revision:    1,
			},
		},
	}

	response := crServe(t, intake, reader,
		http.MethodGet, "/claims-recovery-records?registry=claim-item")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Items   []struct {
			Batch              string `json:"batch"`
			ItemID             string `json:"itemId"`
			Customer           string `json:"customer"`
			Applicant          any    `json:"applicant"`
			Kind               string `json:"kind"`
			SubmittedAt        string `json:"submittedAt"`
			Revision           int64  `json:"revision"`
			Screen             any    `json:"screen"`
			MissingMaterials   any    `json:"missingMaterials"`
			SupplementDeadline any    `json:"supplementDeadline"`
			DeadlineVersions   int64  `json:"deadlineVersions"`
			Conclusion         any    `json:"conclusion"`
			ConcludedAt        any    `json:"concludedAt"`
			ReviewBy           any    `json:"reviewBy"`
			PriorConclusion    any    `json:"priorConclusion"`
			Withdrawn          bool   `json:"withdrawn"`
			WithdrawnAt        any    `json:"withdrawnAt"`
			Amount             any    `json:"amount"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "CLAIM_ITEMS_LISTED" || len(body.Items) != 2 {
		t.Fatalf("outcome = %q items = %d", body.Outcome, len(body.Items))
	}
	full := body.Items[0]
	if full.ItemID != "item-1" || full.Applicant != "applicant-7" ||
		full.Kind != "DAMAGE" || full.Revision != 2 ||
		full.SubmittedAt != operationsBaseAt.Format(time.RFC3339Nano) ||
		full.Screen != "AWAITING_SUPPLEMENT" || full.MissingMaterials != "materials/photos" ||
		full.SupplementDeadline != supplementDeadline.Format(time.RFC3339Nano) ||
		full.DeadlineVersions != 2 ||
		full.Conclusion != "LIABILITY_CONFIRMED" ||
		full.ConcludedAt != concludedAt.Format(time.RFC3339Nano) ||
		full.ReviewBy != reviewBy.Format(time.RFC3339Nano) ||
		full.PriorConclusion != "LIABILITY_DENIED" ||
		!full.Withdrawn || full.WithdrawnAt != withdrawnAt.Format(time.RFC3339Nano) {
		t.Fatalf("满生命周期行走样：%+v", full)
	}
	// 金额键结构上不存在：赔付金额由 settlement-accounting 形成，本册连键都不带。
	if full.Amount != nil {
		t.Fatalf("索赔项行不得带金额键：%+v", full)
	}
	bare := body.Items[1]
	// 裸受理行：三判一个都没到，申请人缺席即客户自己提交——全部缺席键即答案。
	if bare.ItemID != "item-2" || bare.Applicant != nil || bare.Screen != nil ||
		bare.MissingMaterials != nil || bare.SupplementDeadline != nil ||
		bare.DeadlineVersions != 0 || bare.Conclusion != nil || bare.ReviewBy != nil ||
		bare.PriorConclusion != nil || bare.Withdrawn || bare.WithdrawnAt != nil {
		t.Fatalf("裸受理行不得带后续判断键：%+v", bare)
	}
}

// Covers: 追偿事项册 200 + 行体逐字段透出——预先通知与正式主张各取最近节点、成对
// 缺席表示该种类尚无动作，两类不折并成「已追偿」。
func TestClaimsRecoveryRecordsListRecoveryMattersVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	deadline := operationsBaseAt.Add(30 * 24 * time.Hour)
	noticeAt := operationsBaseAt.Add(2 * time.Hour)
	assertionAt := operationsBaseAt.Add(48 * time.Hour)
	reader := &claimsReaderDouble{
		matters: []ports.RecoveryMatterCatalogueRow{
			{
				MatterID:     "matter-1",
				CaseID:       "case-1",
				Counterparty: "carrier-x",
				Scope:        "leg/CN-DE",
				Basis:        "contract/carriage-7",
				LegalEntity:  "entity-1",
				Evidence:     "evidence/9",
				Deadline:     deadline,
				OpenedAt:     operationsBaseAt,
				PreliminaryNotice: &ports.RecoveryActionCell{
					Milestone: "DELIVERED", Attempt: 2, OccurredAt: noticeAt,
				},
				FormalAssertion: &ports.RecoveryActionCell{
					Milestone: "SUBMITTED", Attempt: 1, OccurredAt: assertionAt,
				},
			},
			{
				MatterID:     "matter-2",
				CaseID:       "case-2",
				Counterparty: "carrier-y",
				Scope:        "leg/DE-FR",
				Basis:        "contract/carriage-8",
				LegalEntity:  "entity-1",
				Evidence:     "evidence/10",
				Deadline:     deadline,
				OpenedAt:     operationsBaseAt,
			},
		},
	}

	response := crServe(t, intake, reader,
		http.MethodGet, "/claims-recovery-records?registry=recovery-matter")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Matters []struct {
			MatterID          string `json:"matterId"`
			CaseID            string `json:"caseId"`
			Counterparty      string `json:"counterparty"`
			Scope             string `json:"scope"`
			Basis             string `json:"basis"`
			Deadline          string `json:"deadline"`
			OpenedAt          string `json:"openedAt"`
			PreliminaryNotice *struct {
				Milestone  string `json:"milestone"`
				Attempt    int64  `json:"attempt"`
				OccurredAt string `json:"occurredAt"`
			} `json:"preliminaryNotice"`
			FormalAssertion *struct {
				Milestone string `json:"milestone"`
				Attempt   int64  `json:"attempt"`
			} `json:"formalAssertion"`
		} `json:"matters"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "RECOVERY_MATTERS_LISTED" || len(body.Matters) != 2 {
		t.Fatalf("outcome = %q matters = %d", body.Outcome, len(body.Matters))
	}
	acted := body.Matters[0]
	if acted.MatterID != "matter-1" || acted.Counterparty != "carrier-x" ||
		acted.Basis != "contract/carriage-7" ||
		acted.Deadline != deadline.Format(time.RFC3339Nano) ||
		acted.OpenedAt != operationsBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("事项行走样：%+v", acted)
	}
	if acted.PreliminaryNotice == nil || acted.PreliminaryNotice.Milestone != "DELIVERED" ||
		acted.PreliminaryNotice.Attempt != 2 ||
		acted.PreliminaryNotice.OccurredAt != noticeAt.Format(time.RFC3339Nano) {
		t.Fatalf("预先通知节点走样：%+v", acted.PreliminaryNotice)
	}
	if acted.FormalAssertion == nil || acted.FormalAssertion.Milestone != "SUBMITTED" ||
		acted.FormalAssertion.Attempt != 1 {
		t.Fatalf("正式主张节点走样：%+v", acted.FormalAssertion)
	}
	idle := body.Matters[1]
	// 两类动作成对缺席：尚无动作是如实答案，不是「已拒绝」也不是空对象。
	if idle.MatterID != "matter-2" || idle.PreliminaryNotice != nil || idle.FormalAssertion != nil {
		t.Fatalf("无动作行不得带动作节点：%+v", idle)
	}
}

// Covers: 读不回是答案未形成（500 + NO_ANSWER_FORMED），不伪装成空册。
func TestClaimsRecoveryRecordsReaderFailureFormsNoAnswer(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	for _, registry := range claimsRegistries {
		response := crServe(t, intake,
			&claimsReaderDouble{err: context.DeadlineExceeded},
			http.MethodGet, "/claims-recovery-records?registry="+registry)
		if response.Code != http.StatusInternalServerError ||
			problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("registry=%s：status = %d body = %s，want 500 NO_ANSWER_FORMED",
				registry, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
}
