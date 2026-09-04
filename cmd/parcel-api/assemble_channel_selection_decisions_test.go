package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	shipmentports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var channelSelectionDecidedAt = time.Date(2026, 9, 4, 19, 0, 0, 0, time.UTC)

// Covers: `/channel-selection-decisions` 的读口是登记册本尊（票 label-channel/23）——择优编排那一半写进去的
// 决定，查阅端点这一半在真实 PostgreSQL 上读得出来：并列冲突列表只列 TIED、按标识取回逐候选四格与出局因由、
// 别的标识答 404 单一 code。同一只读口挂未配置 Intake 时照旧 403（ADR-0055）：读口接真不改变准入。测试输入
// 是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredChannelSelectionDecisionReadAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reader, err := buildChannelSelectionDecisionRead(db)
	if err != nil {
		t.Fatalf("装配读口：%v", err)
	}
	registry, err := pspostgres.NewChannelSelectionDecisions(db)
	if err != nil {
		t.Fatalf("构造决定登记册：%v", err)
	}

	tenant := "SYN-TENANT-LC23"
	subject := channelSelectionSubject(t, "SYN-SCOPE-LC23", "SYN-MAPPING-LC23")
	tied := channelSelectionDecision(t, "SYN-CSDN-LC23-TIED", tenant, subject, channelSelectionDecidedAt,
		pricedChannelCandidate(t, "SYN-CAND-A", "10.00", "SYN-EVAL-A"),
		pricedChannelCandidate(t, "SYN-CAND-B", "10.00", "SYN-EVAL-B"),
		unpriceableChannelCandidate(t, "SYN-CAND-C", shipmentdomain.ChannelCostPendingEvidence),
	)
	resolved := channelSelectionDecision(t, "SYN-CSDN-LC23-RESOLVED", tenant, subject, channelSelectionDecidedAt.Add(time.Hour),
		pricedChannelCandidate(t, "SYN-CAND-A", "9.00", "SYN-EVAL-A2"),
		pricedChannelCandidate(t, "SYN-CAND-B", "10.00", "SYN-EVAL-B2"),
	)
	// 闭包只做 IO 并回 error，断言留在闭包外（判据同其余 assemble_*_test）。
	for _, decision := range []shipmentdomain.ChannelSelectionDecision{tied, resolved} {
		var appended shipmentports.ChannelSelectionDecisionAppendOutcome
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var appendErr error
			appended, appendErr = registry.Append(txCtx, decision)
			return appendErr
		}); err != nil {
			t.Fatalf("落决定 %s：%v", decision.ID(), err)
		}
		if appended != shipmentports.ChannelSelectionDecisionAppended {
			t.Fatalf("夹具落库 outcome = %d", appended)
		}
	}

	admitted, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		isolatedReadScopeReference, tenant, []string{isolatedReadCustomerAccount}, isolatedReadLimit)
	if err != nil {
		t.Fatalf("构造隔离读 Intake：%v", err)
	}
	endpoint := shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(admitted, reader)

	listed := serveChannelSelectionDecisions(t, endpoint, "/channel-selection-decisions?view=tied")
	if listed.Code != http.StatusOK {
		t.Fatalf("列并列冲突 status = %d, body = %s", listed.Code, listed.Body)
	}
	var list struct {
		Outcome   string `json:"outcome"`
		Decisions []struct {
			DecisionID string `json:"decisionId"`
			Conclusion string `json:"conclusion"`
			Candidates []struct {
				Candidate string `json:"candidate"`
				Outcome   string `json:"outcome"`
				Exclusion string `json:"exclusion"`
			} `json:"candidates"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode %s: %v", listed.Body, err)
	}
	if list.Outcome != "TIED_CHANNEL_SELECTION_DECISIONS_LISTED" || len(list.Decisions) != 1 ||
		list.Decisions[0].DecisionID != "SYN-CSDN-LC23-TIED" || list.Decisions[0].Conclusion != "TIED" {
		t.Fatalf("并列冲突列表应只有那一条 TIED：%s", listed.Body)
	}
	if candidates := list.Decisions[0].Candidates; len(candidates) != 3 ||
		candidates[0].Outcome != "TIED" || candidates[1].Outcome != "TIED" ||
		candidates[2].Outcome != "EXCLUDED" || candidates[2].Exclusion != "PENDING_EVIDENCE" {
		t.Fatalf("逐候选四格与因由没从库里带回：%s", listed.Body)
	}

	single := serveChannelSelectionDecisions(t, endpoint, "/channel-selection-decisions?decisionId=SYN-CSDN-LC23-RESOLVED")
	if single.Code != http.StatusOK {
		t.Fatalf("按标识取 status = %d, body = %s", single.Code, single.Body)
	}
	var detail struct {
		Outcome  string `json:"outcome"`
		Decision struct {
			Conclusion        string `json:"conclusion"`
			SelectedCandidate string `json:"selectedCandidate"`
		} `json:"decision"`
	}
	if err := json.Unmarshal(single.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode %s: %v", single.Body, err)
	}
	if detail.Outcome != "CHANNEL_SELECTION_DECISION" || detail.Decision.Conclusion != "SELECTED" || detail.Decision.SelectedCandidate != "SYN-CAND-A" {
		t.Fatalf("选出唯一者的记录按标识取回不对：%s", single.Body)
	}

	missing := serveChannelSelectionDecisions(t, endpoint, "/channel-selection-decisions?decisionId=SYN-CSDN-NEVER")
	if missing.Code != http.StatusNotFound || problemCode(t, missing) != "CHANNEL_SELECTION_DECISION_NOT_VISIBLE" {
		t.Fatalf("不存在的标识应答 404 单一 code：%d %s", missing.Code, missing.Body)
	}

	unconfigured := serveChannelSelectionDecisions(t,
		shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(shipmenthttp.UnconfiguredIntake{}, reader),
		"/channel-selection-decisions?view=tied")
	if unconfigured.Code != http.StatusForbidden || problemCode(t, unconfigured) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("读口接真不改变准入：未配置 Intake 应仍答 403，实得 %d %s", unconfigured.Code, unconfigured.Body)
	}
}

func serveChannelSelectionDecisions(t *testing.T, endpoint http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func channelSelectionSubject(t *testing.T, scope, mapping string) shipmentdomain.ChannelSelectionSubject {
	t.Helper()
	subject, err := shipmentdomain.NewChannelSelectionSubject(
		mustValue(t, shipmentdomain.NewCommercialScopeReference, scope),
		mustValue(t, shipmentdomain.NewProductChannelMappingReference, mapping),
	)
	if err != nil {
		t.Fatalf("造对象引用：%v", err)
	}
	return subject
}

func pricedChannelCandidate(t *testing.T, id, amount, evaluation string) shipmentdomain.ChannelCandidateCost {
	t.Helper()
	cost, err := shipmentdomain.PricedChannelCandidate(
		mustValue(t, shipmentdomain.NewChannelCandidateID, id),
		mustValue(t, shipmentdomain.NewChannelCostAmount, amount),
		mustValue(t, shipmentdomain.NewChannelCostCurrency, "SYN"),
	)
	if err != nil {
		t.Fatalf("造已定价候选 %s：%v", id, err)
	}
	cost, err = cost.WithEvaluation(mustValue(t, shipmentdomain.NewChannelCostEvaluationReference, evaluation))
	if err != nil {
		t.Fatalf("带评价引用：%v", err)
	}
	return cost
}

func unpriceableChannelCandidate(t *testing.T, id string, grade shipmentdomain.ChannelCostUnavailability) shipmentdomain.ChannelCandidateCost {
	t.Helper()
	cost, err := shipmentdomain.UnpriceableChannelCandidate(mustValue(t, shipmentdomain.NewChannelCandidateID, id), grade)
	if err != nil {
		t.Fatalf("造不可计价候选 %s：%v", id, err)
	}
	return cost
}

func channelSelectionDecision(
	t *testing.T,
	id, tenant string,
	subject shipmentdomain.ChannelSelectionSubject,
	decidedAt time.Time,
	costs ...shipmentdomain.ChannelCandidateCost,
) shipmentdomain.ChannelSelectionDecision {
	t.Helper()
	decision, err := shipmentdomain.FormChannelSelectionDecision(shipmentdomain.ChannelSelectionDecisionSpec{
		ID:            mustValue(t, shipmentdomain.NewChannelSelectionDecisionID, id),
		Tenant:        mustValue(t, shipmentdomain.NewTenantID, tenant),
		Subject:       subject,
		AssembledAsOf: decidedAt.Add(-time.Minute),
		DecidedAt:     decidedAt,
		Costs:         costs,
	})
	if err != nil {
		t.Fatalf("形成决定 %s：%v", id, err)
	}
	return decision
}
