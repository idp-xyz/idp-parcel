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

// RouteReassessments 实现 ports.ReassessmentStore。主键=（租户+触发关联）：同一触发
// 和输入版本只处理一次，重复触发按关联读回原结果；`已有记录`由 ON CONFLICT DO
// NOTHING 加零行判定翻译（ADR-0031），撞键不把事务打进中止态。
//
// 四种领域走向的在场件矩阵在迁移里逐结论 CHECK；读回时改路三件经领域构造门重走
// （NewRerouteSuggestion / FormRerouteDecision）——一行「自动改路却没带新计划」在
// 读回处响亮暴露。
type RouteReassessments struct {
	db *bentopg.DB
}

func NewRouteReassessments(db *bentopg.DB) (*RouteReassessments, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &RouteReassessments{db: db}, nil
}

// suggestionRow 与 decisionRow 是改路两件 jsonb 列的行模型。判断键与新计划不进这两
// 份 json：键归平铺列、新计划归 new_plan 列独家拥有，决定只引用不复述。
type suggestionRow struct {
	Trigger     string         `json:"trigger"`
	Candidates  []candidateRow `json:"candidates"`
	Blockers    []string       `json:"blockers"`
	SuggestedAt time.Time      `json:"suggested_at"`
}

type decisionRow struct {
	Mode         string    `json:"mode"`
	DecidedBy    string    `json:"decided_by,omitempty"`
	Trigger      string    `json:"trigger"`
	OriginalPlan string    `json:"original_plan"`
	DecidedAt    time.Time `json:"decided_at"`
}

// FindByCorrelation 按（租户+触发关联）取回已提交复核结果。否定结果只回 false，
// 不区分「不存在」与「属于另一个租户」。
func (repository *RouteReassessments) FindByCorrelation(
	ctx context.Context,
	tenant domain.TenantID,
	correlation domain.RequestCorrelationID,
) (ports.ReassessmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
	}

	var (
		customerAccount, shipmentRequest              string
		acceptanceBaseline, declaredParcel, purpose   string
		conclusion                                    string
		reviewedPlan, lapseBasis                      *string
		candidateState, rerouteState                  *string
		blockersJSON, planJSON, suggJSON, decisionRaw []byte
		reassessedAt                                  time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT customer_account_id, shipment_request_id,
		        acceptance_baseline, declared_parcel_id, service_purpose,
		        conclusion, reviewed_plan, lapse_basis,
		        candidate_state, reroute_state, reroute_blockers,
		        new_plan, suggestion, decision, reassessed_at
		   FROM network_routing.route_reassessment
		  WHERE tenant_id = $1
		    AND correlation_id = $2`,
		tenant.String(),
		correlation.String(),
	).Scan(&customerAccount, &shipmentRequest,
		&acceptanceBaseline, &declaredParcel, &purpose,
		&conclusion, &reviewedPlan, &lapseBasis,
		&candidateState, &rerouteState, &blockersJSON,
		&planJSON, &suggJSON, &decisionRaw, &reassessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReassessmentRecord{}, false, nil
	}
	if err != nil {
		return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
	}

	key, err := rebuildRouteKey(tenant, routeKeyColumns{
		customerAccount:    customerAccount,
		shipmentRequest:    shipmentRequest,
		acceptanceBaseline: acceptanceBaseline,
		declaredParcel:     declaredParcel,
		servicePurpose:     purpose,
	})
	if err != nil {
		return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
	}

	record := ports.ReassessmentRecord{
		Correlation:  correlation,
		Key:          key,
		ReassessedAt: reassessedAt.UTC(),
	}
	record.Conclusion, err = reassessmentConclusionFrom(conclusion)
	if err != nil {
		return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
	}
	if reviewedPlan != nil {
		record.ReviewedPlan, err = domain.NewRoutePlanVersionID(*reviewedPlan)
		if err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
	}
	if lapseBasis != nil {
		record.LapseBasis, err = domain.NewApplicabilityBasisReference(*lapseBasis)
		if err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
	}
	if candidateState != nil {
		record.CandidateState, err = candidateStateFrom(*candidateState)
		if err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
	}
	if rerouteState != nil {
		record.RerouteState, err = rerouteAuthorityFrom(*rerouteState)
		if err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
	}
	if blockersJSON != nil {
		if err := json.Unmarshal(blockersJSON, &record.RerouteBlockers); err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
	}
	if planJSON != nil {
		record.NewPlan, err = rebuildPlan(key, planJSON)
		if err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
		record.HasNewPlan = true
	}
	if suggJSON != nil {
		record.Suggestion, err = rebuildSuggestion(key, suggJSON)
		if err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
		record.HasSuggestion = true
	}
	if decisionRaw != nil {
		if !record.HasNewPlan {
			return ports.ReassessmentRecord{}, false, fmt.Errorf(
				"find reassessment: a stored decision arrived without its new plan")
		}
		record.Decision, err = rebuildDecision(record.NewPlan, decisionRaw)
		if err != nil {
			return ports.ReassessmentRecord{}, false, fmt.Errorf("find reassessment: %w", err)
		}
		record.HasDecision = true
	}
	return record, true, nil
}

// Save 写下一次复核结果。同（租户+触发关联）已有记录时答`已有记录`——业务答案不是
// 错误（ADR-0031），编排据此读回赢家、不重复决定。
func (repository *RouteReassessments) Save(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	record ports.ReassessmentRecord,
) (ports.ReassessmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReassessmentSaveOutcomeInvalid, fmt.Errorf("save reassessment: %w", err)
	}

	columns, err := reassessmentColumns(record)
	if err != nil {
		return ports.ReassessmentSaveOutcomeInvalid, fmt.Errorf("save reassessment: %w", err)
	}

	key := record.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id,
			 customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis,
			 candidate_state, reroute_state, reroute_blockers,
			 new_plan, suggestion, decision, reassessed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		correlation.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
		columns.conclusion,
		columns.reviewedPlan,
		columns.lapseBasis,
		columns.candidateState,
		columns.rerouteState,
		columns.blockersJSON,
		columns.planJSON,
		columns.suggestionJSON,
		columns.decisionJSON,
		record.ReassessedAt.UTC(),
	)
	if err != nil {
		return ports.ReassessmentSaveOutcomeInvalid, fmt.Errorf("save reassessment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ReassessmentAlreadyRecorded, nil
	}
	return ports.ReassessmentSaved, nil
}

type reassessmentRowColumns struct {
	conclusion     string
	reviewedPlan   *string
	lapseBasis     *string
	candidateState *string
	rerouteState   *string
	blockersJSON   []byte
	planJSON       []byte
	suggestionJSON []byte
	decisionJSON   []byte
}

func reassessmentColumns(record ports.ReassessmentRecord) (reassessmentRowColumns, error) {
	columns := reassessmentRowColumns{conclusion: record.Conclusion.String()}
	if columns.conclusion == "" {
		return reassessmentRowColumns{}, fmt.Errorf(
			"unknown reassessment conclusion %d", record.Conclusion)
	}
	columns.reviewedPlan = optionalColumn(record.ReviewedPlan.String())
	columns.lapseBasis = optionalColumn(record.LapseBasis.String())
	columns.candidateState = optionalColumn(record.CandidateState.String())
	columns.rerouteState = optionalColumn(record.RerouteState.String())

	var err error
	if len(record.RerouteBlockers) > 0 {
		if columns.blockersJSON, err = json.Marshal(record.RerouteBlockers); err != nil {
			return reassessmentRowColumns{}, err
		}
	}
	if record.HasNewPlan {
		if record.NewPlan.Key() != record.Key {
			return reassessmentRowColumns{}, errors.New(
				"record key disagrees with the new plan's judgment key")
		}
		if columns.planJSON, err = json.Marshal(rowOfPlan(record.NewPlan)); err != nil {
			return reassessmentRowColumns{}, err
		}
	}
	if record.HasSuggestion {
		if columns.suggestionJSON, err = json.Marshal(suggestionRow{
			Trigger:     record.Suggestion.Trigger().String(),
			Candidates:  rowsOfCandidates(record.Suggestion.Candidates()),
			Blockers:    record.Suggestion.Blockers(),
			SuggestedAt: record.Suggestion.SuggestedAt(),
		}); err != nil {
			return reassessmentRowColumns{}, err
		}
	}
	if record.HasDecision {
		row := decisionRow{
			Mode:         record.Decision.Mode().String(),
			Trigger:      record.Decision.Trigger().String(),
			OriginalPlan: record.Decision.OriginalPlan().String(),
			DecidedAt:    record.Decision.DecidedAt(),
		}
		if decidedBy, present := record.Decision.DecidedBy(); present {
			row.DecidedBy = decidedBy.String()
		}
		if columns.decisionJSON, err = json.Marshal(row); err != nil {
			return reassessmentRowColumns{}, err
		}
	}
	return columns, nil
}

// optionalColumn 把「零值即缺席」的领域标量折成可空列。领域侧不带 valid 出口，
// 空串即无值——与可达性适配器读 reason 的判定同一约定。
func optionalColumn(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

type routeKeyColumns struct {
	customerAccount    string
	shipmentRequest    string
	acceptanceBaseline string
	declaredParcel     string
	servicePurpose     string
}

func rebuildRouteKey(
	tenant domain.TenantID,
	columns routeKeyColumns,
) (domain.InitialRouteJudgmentKey, error) {
	customer, err := domain.NewCustomerAccountID(columns.customerAccount)
	if err != nil {
		return domain.InitialRouteJudgmentKey{}, err
	}
	request, err := domain.NewShipmentRequestID(columns.shipmentRequest)
	if err != nil {
		return domain.InitialRouteJudgmentKey{}, err
	}
	baseline, err := domain.NewAcceptanceBaselineReference(columns.acceptanceBaseline)
	if err != nil {
		return domain.InitialRouteJudgmentKey{}, err
	}
	parcel, err := domain.NewDeclaredParcelID(columns.declaredParcel)
	if err != nil {
		return domain.InitialRouteJudgmentKey{}, err
	}
	purpose, err := domain.NewServicePurpose(columns.servicePurpose)
	if err != nil {
		return domain.InitialRouteJudgmentKey{}, err
	}
	return domain.InitialRouteJudgmentKey{
		TenantID:           tenant,
		CustomerAccountID:  customer,
		ShipmentRequestID:  request,
		AcceptanceBaseline: baseline,
		DeclaredParcelID:   parcel,
		ServicePurpose:     purpose,
	}, nil
}

func rebuildSuggestion(
	key domain.InitialRouteJudgmentKey,
	data []byte,
) (domain.RerouteSuggestion, error) {
	var row suggestionRow
	if err := json.Unmarshal(data, &row); err != nil {
		return domain.RerouteSuggestion{}, err
	}
	trigger, err := domain.NewRerouteTriggerReference(row.Trigger)
	if err != nil {
		return domain.RerouteSuggestion{}, err
	}
	candidates, err := candidatesOfRows(row.Candidates)
	if err != nil {
		return domain.RerouteSuggestion{}, err
	}
	return domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
		Key:         key,
		Trigger:     trigger,
		Candidates:  candidates,
		Blockers:    row.Blockers,
		SuggestedAt: row.SuggestedAt,
	})
}

// rebuildDecision 经 FormRerouteDecision 重走决定门。Authority 按决定方式回推：自动
// 决定只在`已允许`下立得成，人工决定在`仅建议`下立——两者都是构造门可接受的最弱
// 前提，行里不另存一份判定（reroute_state 列归记录，不归决定）。
func rebuildDecision(
	newPlan domain.InitialRoutePlan,
	data []byte,
) (domain.RerouteDecision, error) {
	var row decisionRow
	if err := json.Unmarshal(data, &row); err != nil {
		return domain.RerouteDecision{}, err
	}
	mode, err := rerouteModeFrom(row.Mode)
	if err != nil {
		return domain.RerouteDecision{}, err
	}
	authority := domain.SuggestionOnly
	if mode == domain.AutomaticReroute {
		authority = domain.AutomaticRerouteAllowed
	}
	trigger, err := domain.NewRerouteTriggerReference(row.Trigger)
	if err != nil {
		return domain.RerouteDecision{}, err
	}
	original, err := domain.NewRoutePlanVersionID(row.OriginalPlan)
	if err != nil {
		return domain.RerouteDecision{}, err
	}
	spec := domain.RerouteDecisionSpec{
		Authority:    authority,
		Mode:         mode,
		Trigger:      trigger,
		OriginalPlan: original,
		NewPlan:      newPlan,
		DecidedAt:    row.DecidedAt,
	}
	if row.DecidedBy != "" {
		if spec.DecidedBy, err = domain.NewAuthorizedRoleReference(row.DecidedBy); err != nil {
			return domain.RerouteDecision{}, err
		}
	}
	return domain.FormRerouteDecision(spec)
}

func reassessmentConclusionFrom(raw string) (ports.ReassessmentConclusionKind, error) {
	switch raw {
	case ports.ReassessmentStillApplicable.String():
		return ports.ReassessmentStillApplicable, nil
	case ports.ReassessmentPlanLapsed.String():
		return ports.ReassessmentPlanLapsed, nil
	case ports.ReassessmentFirstPlanFormed.String():
		return ports.ReassessmentFirstPlanFormed, nil
	case ports.ReassessmentRerouted.String():
		return ports.ReassessmentRerouted, nil
	default:
		return ports.ReassessmentConclusionKindInvalid, fmt.Errorf(
			"unknown reassessment conclusion %q", raw)
	}
}

func candidateStateFrom(raw string) (ports.CandidateReviewState, error) {
	switch raw {
	case ports.CandidatesAvailable.String():
		return ports.CandidatesAvailable, nil
	case ports.NoQualifiedCandidates.String():
		return ports.NoQualifiedCandidates, nil
	case ports.CandidateReviewUndecided.String():
		return ports.CandidateReviewUndecided, nil
	default:
		return ports.CandidateReviewStateInvalid, fmt.Errorf(
			"unknown candidate review state %q", raw)
	}
}

func rerouteAuthorityFrom(raw string) (domain.RerouteAuthority, error) {
	switch raw {
	case domain.AutomaticRerouteAllowed.String():
		return domain.AutomaticRerouteAllowed, nil
	case domain.SuggestionOnly.String():
		return domain.SuggestionOnly, nil
	case domain.RerouteBarred.String():
		return domain.RerouteBarred, nil
	default:
		return domain.RerouteAuthorityInvalid, fmt.Errorf("unknown reroute authority %q", raw)
	}
}

func rerouteModeFrom(raw string) (domain.RerouteDecisionMode, error) {
	switch raw {
	case domain.AutomaticReroute.String():
		return domain.AutomaticReroute, nil
	case domain.AuthorizedRoleReroute.String():
		return domain.AuthorizedRoleReroute, nil
	default:
		return domain.RerouteDecisionModeInvalid, fmt.Errorf("unknown reroute mode %q", raw)
	}
}
