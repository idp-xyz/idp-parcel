package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

var _ ports.InitialRouteNotFormedRegister = (*InitialRoutes)(nil)

// tiedCostRow 是并列那几家候选成本事实的行模型。并列只在已计价候选之间成立，所以只记金额与币种。
type tiedCostRow struct {
	Candidate   string `json:"candidate"`
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
}

// RecordNotFormed 留下一件包裹初始路由未形成的原因与对象（ADR-0273 决定二）。同一判断键已有一行时按这一次改写：这一行答
// 这件包裹「最近一次停住的原因」，旧的那一次连同它的并列与缺口依据都不留。头一次落库的时刻不改。
//
// 走 RequireExecutor：这一行与同一交接里已判成的包裹、交接登记与 inbox 的入账同笔提交，脱开事务写会留下一条没人入账的
// 原因。在场件不合原因（并列缺候选、别的格带了候选）是写入方缺陷，响亮报错不落库；库上 CHECK 是第二道网。
func (repository *InitialRoutes) RecordNotFormed(ctx context.Context, record ports.InitialRouteNotFormed) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record initial route not formed: %w", err)
	}
	reason := record.Reason.String()
	if reason == "" {
		return fmt.Errorf("record initial route not formed: unknown reason %d", record.Reason)
	}
	if !record.Correlation.Valid() {
		return fmt.Errorf("record initial route not formed: correlation is missing")
	}
	if record.AcceptedAt.IsZero() || record.SettledAt.IsZero() {
		return fmt.Errorf("record initial route not formed: acceptance or settlement time is zero")
	}
	candidates, tied, tiedCosts, gaps, err := notFormedParts(record)
	if err != nil {
		return fmt.Errorf("record initial route not formed: %w", err)
	}
	key := record.Key
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.initial_route_not_formed
			(tenant_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 correlation_id, reason, candidates, tied, tied_costs, gaps, accepted_at, settled_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT ON CONSTRAINT initial_route_not_formed_pkey DO UPDATE SET
			correlation_id = EXCLUDED.correlation_id,
			reason         = EXCLUDED.reason,
			candidates     = EXCLUDED.candidates,
			tied           = EXCLUDED.tied,
			tied_costs     = EXCLUDED.tied_costs,
			gaps           = EXCLUDED.gaps,
			accepted_at    = EXCLUDED.accepted_at,
			settled_at     = EXCLUDED.settled_at`,
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
		record.Correlation.String(),
		reason,
		candidates,
		tied,
		tiedCosts,
		gaps,
		record.AcceptedAt.UTC(),
		record.SettledAt.UTC(),
	); err != nil {
		return fmt.Errorf("record initial route not formed: %w", err)
	}
	return nil
}

// notFormedParts 按原因折出在场件，与迁移 0025 的 initial_route_not_formed_parts_by_reason 逐格同形：并列带候选、并列的
// 那几家与它们的成本，候选证据不全带候选与缺口，其余各格一样都不带。缺席的件交 nil，落库为 NULL。
func notFormedParts(record ports.InitialRouteNotFormed) (candidates, tied, tiedCosts, gaps []byte, err error) {
	switch record.Reason {
	case ports.NotFormedCandidatesTied:
		if len(record.Candidates) == 0 || len(record.Tied) < 2 || len(record.TiedCosts) != len(record.Tied) {
			return nil, nil, nil, nil, fmt.Errorf("a tie carries all candidates, the tied ones and their costs")
		}
		if candidates, err = json.Marshal(rowsOfCandidates(record.Candidates)); err != nil {
			return nil, nil, nil, nil, err
		}
		ids := make([]string, 0, len(record.Tied))
		for _, id := range record.Tied {
			ids = append(ids, id.String())
		}
		if tied, err = json.Marshal(ids); err != nil {
			return nil, nil, nil, nil, err
		}
		rows := make([]tiedCostRow, 0, len(record.TiedCosts))
		for _, fact := range record.TiedCosts {
			if fact.State() != domain.CandidateCostPriced {
				return nil, nil, nil, nil, fmt.Errorf("tied candidate %q is not priced", fact.Candidate())
			}
			rows = append(rows, tiedCostRow{
				Candidate:   fact.Candidate().String(),
				AmountMinor: fact.AmountMinor(),
				Currency:    fact.Currency(),
			})
		}
		if tiedCosts, err = json.Marshal(rows); err != nil {
			return nil, nil, nil, nil, err
		}
		return candidates, tied, tiedCosts, nil, nil
	case ports.NotFormedCandidateEvidenceIncomplete:
		if len(record.Candidates) == 0 || len(record.Tied) != 0 || len(record.TiedCosts) != 0 {
			return nil, nil, nil, nil, fmt.Errorf("incomplete candidate evidence carries the candidates and nothing tied")
		}
		if candidates, err = json.Marshal(rowsOfCandidates(record.Candidates)); err != nil {
			return nil, nil, nil, nil, err
		}
		// 缺口可以为空：候选证据未知而取数侧没交缺口时如实记空数组，不编一条。
		if gaps, err = json.Marshal(rowsOfGaps(record.Gaps)); err != nil {
			return nil, nil, nil, nil, err
		}
		return candidates, nil, nil, gaps, nil
	default:
		if len(record.Candidates) != 0 || len(record.Tied) != 0 || len(record.TiedCosts) != 0 || len(record.Gaps) != 0 {
			return nil, nil, nil, nil, fmt.Errorf("reason %s carries no candidates, ties or gaps", record.Reason)
		}
		return nil, nil, nil, nil, nil
	}
}

// rowsOfGaps 与可达性判断的 gapRow 同一个行模型：两处缺口是同一个领域对象。
func rowsOfGaps(gaps []domain.EvidenceGap) []gapRow {
	rows := make([]gapRow, 0, len(gaps))
	for _, gap := range gaps {
		affected := gap.AffectedCandidates()
		ids := make([]string, 0, len(affected))
		for _, id := range affected {
			ids = append(ids, id.String())
		}
		rows = append(rows, gapRow{
			Reference:    gap.Reference().String(),
			Scope:        gap.Scope().String(),
			Affected:     ids,
			Reassessment: gap.ReassessmentCondition().String(),
		})
	}
	return rows
}
