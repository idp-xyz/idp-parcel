package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 16 证初始路由未形成登记（ADR-0273 决定二，迁移 0025）：一行留下判断键、原因、交接关联与接受
// 时刻；同一判断键再记按最新一次改写；并列另记全部候选、并列的那几家与打平的金额；原因与在场件在库里封闭；无事务拒写，
// 回滚无痕。

var notFormedAcceptedAt = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)

type notFormedRow struct {
	correlation, reason             string
	candidates, tied, tiedCosts     []byte
	gaps                            []byte
	acceptedAt, settledAt, recorded time.Time
}

func notFormedRowOf(t *testing.T, pool *pgxpool.Pool, key domain.InitialRouteJudgmentKey) (notFormedRow, bool) {
	t.Helper()
	var row notFormedRow
	err := pool.QueryRow(t.Context(),
		`SELECT correlation_id, reason, candidates, tied, tied_costs, gaps, accepted_at, settled_at, recorded_at
		   FROM network_routing.initial_route_not_formed
		  WHERE tenant_id = $1 AND customer_account_id = $2 AND shipment_request_id = $3
		    AND acceptance_baseline = $4 AND declared_parcel_id = $5 AND service_purpose = $6`,
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(), key.DeclaredParcelID.String(), key.ServicePurpose.String(),
	).Scan(&row.correlation, &row.reason, &row.candidates, &row.tied, &row.tiedCosts, &row.gaps,
		&row.acceptedAt, &row.settledAt, &row.recorded)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFormedRow{}, false
	}
	if err != nil {
		t.Fatalf("读未形成行：%v", err)
	}
	return row, true
}

func notFormed(t *testing.T, key domain.InitialRouteJudgmentKey, correlation string, reason ports.InitialRouteNotFormedReason) ports.InitialRouteNotFormed {
	t.Helper()
	return ports.InitialRouteNotFormed{
		Key:         key,
		Correlation: scalar(t, domain.NewRequestCorrelationID, correlation),
		Reason:      reason,
		AcceptedAt:  notFormedAcceptedAt,
		SettledAt:   reassessedAtTS,
	}
}

func tiedNotFormed(t *testing.T, key domain.InitialRouteJudgmentKey) ports.InitialRouteNotFormed {
	t.Helper()
	record := notFormed(t, key, "acceptance/decision-tie", ports.NotFormedCandidatesTied)
	for _, id := range []string{"SYN-LINE-A@1", "SYN-LINE-B@1"} {
		candidate, err := domain.NewRouteCandidate(scalar(t, domain.NewCandidateID, id), domain.CandidateQualified, domain.CandidateReason{})
		if err != nil {
			t.Fatalf("候选：%v", err)
		}
		cost, err := domain.NewPricedCandidateCost(candidate.ID(), 4200, "CNY")
		if err != nil {
			t.Fatalf("候选成本：%v", err)
		}
		record.Candidates = append(record.Candidates, candidate)
		record.Tied = append(record.Tied, candidate.ID())
		record.TiedCosts = append(record.TiedCosts, cost)
	}
	return record
}

// Covers: `AT-NR-088`（登记那一半）——一行留下判断键、原因、交接关联与两个时刻，不带候选；同一判断键再记一次（另一格原因、
// 另一封交接）按最新一次改写，头一次落库的时刻不改；未形成不是判断结果，判断库里没有这件包裹。
func TestANotFormedParcelKeepsItsLatestReason(t *testing.T) {
	routes, _, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, notFormed(t, key, "acceptance/decision-1", ports.NotFormedEvidenceNotConfigured))
	})
	first, present := notFormedRowOf(t, pool, key)
	if !present {
		t.Fatal("未形成行没落库")
	}
	if first.reason != "ROUTE_EVIDENCE_NOT_CONFIGURED" || first.correlation != "acceptance/decision-1" ||
		!first.acceptedAt.Equal(notFormedAcceptedAt) || !first.settledAt.Equal(reassessedAtTS) ||
		first.candidates != nil || first.tied != nil || first.tiedCosts != nil || first.gaps != nil {
		t.Fatalf("未形成行 = %+v, want 证据未配置、交接关联与两个时刻原样、不带候选", first)
	}

	later := notFormed(t, key, "acceptance/decision-2", ports.NotFormedCandidateCostsPending)
	later.SettledAt = reassessedAtTS.Add(time.Hour)
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, later)
	})
	second, _ := notFormedRowOf(t, pool, key)
	if second.reason != "CANDIDATE_COSTS_PENDING" || second.correlation != "acceptance/decision-2" ||
		!second.settledAt.Equal(later.SettledAt) || !second.recorded.Equal(first.recorded) {
		t.Fatalf("改写后 = %+v, want 最新原因与关联、入账时刻随之、头一次落库时刻不变", second)
	}
	if _, found, err := routes.FindByKey(ctx, key); err != nil || found {
		t.Fatalf("未形成不是判断结果，判断库里却有：found=%v err=%v", found, err)
	}
}

// Covers: `AT-NR-089`（登记那一半）——并列一行另记全部候选、并列的那几家与它们打平的金额。
func TestATieRecordsItsCandidatesAndTheTiedAmount(t *testing.T) {
	routes, _, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, tiedNotFormed(t, key))
	})
	row, _ := notFormedRowOf(t, pool, key)
	if row.reason != "CANDIDATES_TIED" {
		t.Fatalf("原因 = %q, want CANDIDATES_TIED", row.reason)
	}
	var candidates []map[string]string
	var tied []string
	var costs []struct {
		Candidate   string `json:"candidate"`
		AmountMinor int64  `json:"amountMinor"`
		Currency    string `json:"currency"`
	}
	if err := errors.Join(
		json.Unmarshal(row.candidates, &candidates),
		json.Unmarshal(row.tied, &tied),
		json.Unmarshal(row.tiedCosts, &costs),
	); err != nil {
		t.Fatalf("解并列三列：%v", err)
	}
	if len(candidates) != 2 || candidates[0]["outcome"] != domain.CandidateQualified.String() ||
		len(tied) != 2 || tied[0] != "SYN-LINE-A@1" || tied[1] != "SYN-LINE-B@1" ||
		len(costs) != 2 || costs[0].AmountMinor != 4200 || costs[1].Currency != "CNY" {
		t.Fatalf("并列 = 候选 %v、并列 %v、金额 %+v, want 两家合格、两家并列、各 4200 CNY", candidates, tied, costs)
	}
}

// Covers: 原因与在场件在库里封闭（ports.InitialRouteNotFormedReason、迁移 0025）：绕过适配器直插的词汇表外原因、带了候选的
// 证据未配置被 CHECK 拒；适配器拒交零值原因与缺并列理由的并列。
func TestTheNotFormedReasonAndItsPartsAreClosed(t *testing.T) {
	routes, _, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	insert := `INSERT INTO network_routing.initial_route_not_formed
			(tenant_id, customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose,
			 correlation_id, reason, candidates, accepted_at, settled_at)
		 VALUES ('tenant-1', 'customer-a', 'request-1', 'baseline-v1', 'parcel-9', 'LAST_MILE_DELIVERY',
		         'acceptance/x', $1, $2::jsonb, now(), now())`
	if _, err := pool.Exec(ctx, insert, "SOMETHING_ELSE", nil); err == nil {
		t.Fatal("词汇表外的原因进了库")
	}
	if _, err := pool.Exec(ctx, insert, "ROUTE_EVIDENCE_NOT_CONFIGURED", `[]`); err == nil {
		t.Fatal("证据未配置带着候选进了库")
	}

	key := routeKey(t, "tenant-1", "parcel-1")
	invalid := notFormed(t, key, "acceptance/decision-3", ports.InitialRouteNotFormedReasonInvalid)
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, invalid)
	}); err == nil {
		t.Fatal("零值原因写进了未形成登记")
	}
	bare := tiedNotFormed(t, key)
	bare.TiedCosts = nil
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, bare)
	}); err == nil {
		t.Fatal("缺打平金额的并列写进了未形成登记")
	}
	if _, present := notFormedRowOf(t, pool, key); present {
		t.Fatal("被拒的写入留下了行")
	}
}

// Covers: 未形成原因与同一交接里判成的包裹、交接登记、inbox 入账同笔提交：无事务拒写，回滚无痕。
func TestANotFormedParcelIsWrittenOnlyInsideATransaction(t *testing.T) {
	routes, _, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	record := notFormed(t, key, "acceptance/decision-4", ports.NotFormedRankingFormNotConfigured)

	if err := routes.RecordNotFormed(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记未形成应返回 ErrTransactionRequired，实得：%v", err)
	}
	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := routes.RecordNotFormed(txCtx, record); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, present := notFormedRowOf(t, pool, key); present {
		t.Error("回滚后未形成行仍在")
	}
}
