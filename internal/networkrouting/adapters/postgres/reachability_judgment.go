// Package postgres 是 network-routing 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与 SQLSTATE 翻译都留在这里，不进领域对象。所有语句显式携带租户
// 条件：按 ADR-0003 运营集团租户是最高数据隔离边界，缺了它另一个租户的同名请求
// 关联就会被当成同一次判断。
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ReachabilityJudgments 实现 ports.ReachabilityJudgmentStore——写入代数（ADR-0031）
// 落到真库的第一例：`已有记录`由主键冲突（SQLSTATE 23505）翻译，不是错误。
type ReachabilityJudgments struct {
	db *bentopg.DB
}

func NewReachabilityJudgments(db *bentopg.DB) (*ReachabilityJudgments, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &ReachabilityJudgments{db: db}, nil
}

// candidateRow 与 gapRow 是 jsonb 列的行模型。它们只在本包存在：领域对象经构造函数
// 重建，行模型不外泄。
type candidateRow struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

type gapRow struct {
	Reference    string   `json:"reference"`
	Scope        string   `json:"scope"`
	Affected     []string `json:"affected,omitempty"`
	Reassessment string   `json:"reassessment"`
}

// FindByCorrelation 按（租户+请求关联）取回已提交判断。
//
// 走 ReadExecutor：事务内读得到本事务刚写的行，事务外用显式注入的连接池。否定结果
// 只回 false，不区分「不存在」与「属于另一个租户」——区分它们等于泄露另一个租户
// 是否发起过这次请求。读回的一切经领域构造函数重建，三值结论再经领域矩阵重算比对：
// 一次坏写入在这里暴露，而不是变成一个看起来合法的判断。
func (repository *ReachabilityJudgments) FindByCorrelation(
	ctx context.Context,
	tenant domain.TenantID,
	correlation domain.RequestCorrelationID,
) (ports.ReachabilityJudgmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReachabilityJudgmentRecord{}, false, fmt.Errorf("find reachability judgment: %w", err)
	}

	var (
		customerAccount, shipmentRequest, submissionVersion string
		declaredParcel, servicePurpose                      string
		asOfSemantic, asOfStrategy                          string
		asOfAt, judgedAt                                    time.Time
		conclusion, viewRevision                            string
		candidatesJSON, gapsJSON                            []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT customer_account_id, shipment_request_id, submission_version_id,
		        declared_parcel_id, service_purpose,
		        as_of_semantic, as_of_at, as_of_strategy_version,
		        conclusion, candidates, evidence_gaps, view_revision, judged_at
		   FROM network_routing.reachability_judgment
		  WHERE tenant_id = $1
		    AND correlation_id = $2`,
		tenant.String(),
		correlation.String(),
	).Scan(&customerAccount, &shipmentRequest, &submissionVersion,
		&declaredParcel, &servicePurpose,
		&asOfSemantic, &asOfAt, &asOfStrategy,
		&conclusion, &candidatesJSON, &gapsJSON, &viewRevision, &judgedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReachabilityJudgmentRecord{}, false, nil
	}
	if err != nil {
		return ports.ReachabilityJudgmentRecord{}, false, fmt.Errorf("find reachability judgment: %w", err)
	}

	key, err := rebuildKey(tenant, keyColumns{
		customerAccount:   customerAccount,
		shipmentRequest:   shipmentRequest,
		submissionVersion: submissionVersion,
		declaredParcel:    declaredParcel,
		servicePurpose:    servicePurpose,
		asOfSemantic:      asOfSemantic,
		asOfAt:            asOfAt,
		asOfStrategy:      asOfStrategy,
	})
	if err != nil {
		return ports.ReachabilityJudgmentRecord{}, false, fmt.Errorf("find reachability judgment: %w", err)
	}
	finding, err := rebuildFinding(conclusion, candidatesJSON, gapsJSON)
	if err != nil {
		return ports.ReachabilityJudgmentRecord{}, false, fmt.Errorf("find reachability judgment: %w", err)
	}
	revision, err := domain.NewNetworkViewRevision(viewRevision)
	if err != nil {
		return ports.ReachabilityJudgmentRecord{}, false, fmt.Errorf("find reachability judgment: %w", err)
	}

	return ports.ReachabilityJudgmentRecord{
		Key:          key,
		Finding:      finding,
		JudgedAt:     judgedAt.UTC(),
		ViewRevision: revision,
	}, true, nil
}

// Save 写下一次判断。同（租户+请求关联）已有记录时答`已有记录`——那是业务答案不是
// 错误（ADR-0031）：第二个写入方据此读回赢家，迟到结果不按到达顺序覆盖原判断
// （AT-NR-028）。
//
// 走 RequireExecutor：判断落库与将来同一步的 Outbox 发布必须同生共死，无事务时框架
// 直接拒绝，而不是悄悄改用连接池。
func (repository *ReachabilityJudgments) Save(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	record ports.ReachabilityJudgmentRecord,
) (ports.ReachabilityJudgmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReachabilityJudgmentSaveOutcomeInvalid, fmt.Errorf("save reachability judgment: %w", err)
	}

	candidatesJSON, gapsJSON, err := marshalFinding(record.Finding)
	if err != nil {
		return ports.ReachabilityJudgmentSaveOutcomeInvalid, fmt.Errorf("save reachability judgment: %w", err)
	}

	// `已有记录`用 ON CONFLICT DO NOTHING 而不是捕 23505 译码：撞键的 INSERT 会把
	// 整个事务打进中止态，同一事务里的后续读写全部失败——而`已有记录`是业务答案
	// （ADR-0031），编排拿到它还要在同一个事务里读回赢家作答。零行命中即已有记录。
	key := record.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO network_routing.reachability_judgment
			(tenant_id, correlation_id,
			 customer_account_id, shipment_request_id, submission_version_id,
			 declared_parcel_id, service_purpose,
			 as_of_semantic, as_of_at, as_of_strategy_version,
			 conclusion, candidates, evidence_gaps, view_revision, judged_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		correlation.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.SubmissionVersion.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
		key.AsOf.Semantic().String(),
		key.AsOf.At().UTC(),
		key.AsOf.StrategyVersion().String(),
		record.Finding.Value().String(),
		candidatesJSON,
		gapsJSON,
		record.ViewRevision.String(),
		record.JudgedAt.UTC(),
	)
	if err != nil {
		return ports.ReachabilityJudgmentSaveOutcomeInvalid, fmt.Errorf("save reachability judgment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ReachabilityJudgmentAlreadyRecorded, nil
	}
	return ports.ReachabilityJudgmentSaved, nil
}

type keyColumns struct {
	customerAccount   string
	shipmentRequest   string
	submissionVersion string
	declaredParcel    string
	servicePurpose    string
	asOfSemantic      string
	asOfAt            time.Time
	asOfStrategy      string
}

func rebuildKey(tenant domain.TenantID, columns keyColumns) (domain.ReachabilityJudgmentKey, error) {
	customer, err := domain.NewCustomerAccountID(columns.customerAccount)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	request, err := domain.NewShipmentRequestID(columns.shipmentRequest)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	submission, err := domain.NewSubmissionVersionID(columns.submissionVersion)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	parcel, err := domain.NewDeclaredParcelID(columns.declaredParcel)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	purpose, err := domain.NewServicePurpose(columns.servicePurpose)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	semantic, err := domain.NewAsOfSemantic(columns.asOfSemantic)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	strategy, err := domain.NewAsOfStrategyVersion(columns.asOfStrategy)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	asOf, err := domain.NewJudgmentAsOf(semantic, columns.asOfAt.UTC(), strategy)
	if err != nil {
		return domain.ReachabilityJudgmentKey{}, err
	}
	return domain.ReachabilityJudgmentKey{
		TenantID:          tenant,
		CustomerAccountID: customer,
		ShipmentRequestID: request,
		SubmissionVersion: submission,
		DeclaredParcelID:  parcel,
		ServicePurpose:    purpose,
		AsOf:              asOf,
	}, nil
}

func marshalFinding(finding domain.ReachabilityFinding) ([]byte, []byte, error) {
	candidates := finding.Candidates()
	candidateRows := make([]candidateRow, 0, len(candidates))
	for _, candidate := range candidates {
		candidateRows = append(candidateRows, candidateRow{
			ID:      candidate.ID().String(),
			Outcome: candidate.Outcome().String(),
			Reason:  candidate.Reason().String(),
		})
	}
	gaps := finding.EvidenceGaps()
	gapRows := make([]gapRow, 0, len(gaps))
	for _, gap := range gaps {
		affected := gap.AffectedCandidates()
		ids := make([]string, 0, len(affected))
		for _, id := range affected {
			ids = append(ids, id.String())
		}
		gapRows = append(gapRows, gapRow{
			Reference:    gap.Reference().String(),
			Scope:        gap.Scope().String(),
			Affected:     ids,
			Reassessment: gap.ReassessmentCondition().String(),
		})
	}

	candidatesJSON, err := json.Marshal(candidateRows)
	if err != nil {
		return nil, nil, err
	}
	gapsJSON, err := json.Marshal(gapRows)
	if err != nil {
		return nil, nil, err
	}
	return candidatesJSON, gapsJSON, nil
}

func rebuildFinding(conclusion string, candidatesJSON, gapsJSON []byte) (domain.ReachabilityFinding, error) {
	var candidateRows []candidateRow
	if err := json.Unmarshal(candidatesJSON, &candidateRows); err != nil {
		return domain.ReachabilityFinding{}, err
	}
	candidates := make([]domain.RouteCandidate, 0, len(candidateRows))
	for _, row := range candidateRows {
		id, err := domain.NewCandidateID(row.ID)
		if err != nil {
			return domain.ReachabilityFinding{}, err
		}
		outcome, err := candidateOutcomeFrom(row.Outcome)
		if err != nil {
			return domain.ReachabilityFinding{}, err
		}
		var reason domain.CandidateReason
		if row.Reason != "" {
			reason, err = domain.NewCandidateReason(row.Reason)
			if err != nil {
				return domain.ReachabilityFinding{}, err
			}
		}
		candidate, err := domain.NewRouteCandidate(id, outcome, reason)
		if err != nil {
			return domain.ReachabilityFinding{}, err
		}
		candidates = append(candidates, candidate)
	}

	var gapRows []gapRow
	if err := json.Unmarshal(gapsJSON, &gapRows); err != nil {
		return domain.ReachabilityFinding{}, err
	}
	gaps := make([]domain.EvidenceGap, 0, len(gapRows))
	for _, row := range gapRows {
		reference, err := domain.NewEvidenceGapReference(row.Reference)
		if err != nil {
			return domain.ReachabilityFinding{}, err
		}
		scope, err := gapScopeFrom(row.Scope)
		if err != nil {
			return domain.ReachabilityFinding{}, err
		}
		affected := make([]domain.CandidateID, 0, len(row.Affected))
		for _, raw := range row.Affected {
			id, err := domain.NewCandidateID(raw)
			if err != nil {
				return domain.ReachabilityFinding{}, err
			}
			affected = append(affected, id)
		}
		reassess, err := domain.NewReassessmentCondition(row.Reassessment)
		if err != nil {
			return domain.ReachabilityFinding{}, err
		}
		gap, err := domain.NewEvidenceGap(reference, scope, affected, reassess)
		if err != nil {
			return domain.ReachabilityFinding{}, err
		}
		gaps = append(gaps, gap)
	}

	finding, err := domain.ConcludeReachability(candidates, gaps)
	if err != nil {
		return domain.ReachabilityFinding{}, err
	}
	// 三值结论经领域矩阵重算后与库列比对：不一致说明这一行被写坏或被改写过，
	// 报错而不是把坏行交回去。
	if finding.Value().String() != conclusion {
		return domain.ReachabilityFinding{}, fmt.Errorf(
			"stored conclusion %q disagrees with the evidence matrix %q", conclusion, finding.Value())
	}
	return finding, nil
}

func candidateOutcomeFrom(raw string) (domain.CandidateOutcome, error) {
	switch raw {
	case domain.CandidateQualified.String():
		return domain.CandidateQualified, nil
	case domain.CandidateEliminated.String():
		return domain.CandidateEliminated, nil
	case domain.CandidateEvidenceUnknown.String():
		return domain.CandidateEvidenceUnknown, nil
	default:
		return 0, fmt.Errorf("unknown candidate outcome %q", raw)
	}
}

func gapScopeFrom(raw string) (domain.EvidenceGapScope, error) {
	switch raw {
	case domain.CandidateScopedGap.String():
		return domain.CandidateScopedGap, nil
	case domain.GlobalGap.String():
		return domain.GlobalGap, nil
	default:
		return 0, fmt.Errorf("unknown evidence gap scope %q", raw)
	}
}
