package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 「毛重差异」那一道的规则行（0039）挂在门禁目录既有登记册上：写口落在 GateConditionRegistrations，
// 读口落在 GateConditionView，与目录 / 认定 / 税费付款规则同一把三维键。

var (
	_ ports.WeightDiscrepancyGateRuleRegistry = (*GateConditionRegistrations)(nil)
	_ ports.WeightDiscrepancyGateRuleView     = (*GateConditionView)(nil)
	_ ports.CurrentDeclarationSubmissionView  = (*DeclarationSubmissions)(nil)
)

// RegisterWeightDiscrepancyGateRule 登记「毛重差异」那一道的规则。registered_at 取事务内的库时钟。
// 写入一律不 UPSERT；目录行不在场时外键拒。零值规则在进库前拒，免得撞 CHECK 被说成依赖故障。
func (registry *GateConditionRegistrations) RegisterWeightDiscrepancyGateRule(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
	rule domain.WeightDiscrepancyGateRule,
) (ports.CaseConfigurationSaveOutcome, error) {
	actionText := action.String()
	if actionText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register weight discrepancy gate rule: unknown guarded action %d", action)
	}
	var maxDelta, unit any
	if !rule.NotAPrecondition() {
		delta, weightUnit, ok := rule.MaxDelta()
		if !ok {
			return ports.CaseConfigurationSaveOutcomeInvalid,
				errors.New("register weight discrepancy gate rule: the rule is zero-valued")
		}
		maxDelta, unit = delta, weightUnit
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register weight discrepancy gate rule: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.gate_condition_weight_discrepancy_rule
			(tenant_id, scope_ref, action, boundary_ref, not_a_precondition, max_delta, weight_unit, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 ON CONFLICT DO NOTHING`,
		tenant.String(), scope.String(), actionText, boundary.String(),
		rule.NotAPrecondition(), maxDelta, unit,
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register weight discrepancy gate rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// LoadWeightDiscrepancyGateRule 按门禁三维键取回规则行。found=false 即这一道尚未登规则。
// 读回经领域构造重建；库里一行若立不起规则，说明有人绕过写口改了它。
func (view *GateConditionView) LoadWeightDiscrepancyGateRule(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
) (domain.WeightDiscrepancyGateRule, bool, error) {
	none := domain.WeightDiscrepancyGateRule{}
	actionText := action.String()
	if actionText == "" {
		return none, false, fmt.Errorf("load weight discrepancy gate rule: unknown guarded action %d", action)
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load weight discrepancy gate rule: %w", err)
	}
	var (
		notAPrecondition bool
		maxDelta, unit   *string
	)
	err = querier.QueryRow(ctx,
		`SELECT not_a_precondition, max_delta, weight_unit
		   FROM customs_compliance.gate_condition_weight_discrepancy_rule
		  WHERE tenant_id = $1 AND scope_ref = $2 AND action = $3 AND boundary_ref = $4`,
		tenant.String(), scope.String(), actionText, boundary.String(),
	).Scan(&notAPrecondition, &maxDelta, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load weight discrepancy gate rule: %w", err)
	}
	rule, err := rebuildWeightDiscrepancyGateRule(notAPrecondition, maxDelta, unit)
	if err != nil {
		return none, false, fmt.Errorf("rebuild weight discrepancy gate rule: %w", err)
	}
	return rule, true, nil
}

func rebuildWeightDiscrepancyGateRule(notAPrecondition bool, maxDelta, unit *string) (domain.WeightDiscrepancyGateRule, error) {
	if notAPrecondition {
		return domain.WeightDiscrepancyNotAPrecondition(), nil
	}
	if maxDelta == nil || unit == nil {
		return domain.WeightDiscrepancyGateRule{}, errors.New("the weight discrepancy rule columns are not paired")
	}
	return domain.AllowWeightDeltaUpTo(*maxDelta, *unit)
}

type weightDiscrepancyMemberDocument struct {
	Parcel            string `json:"parcel"`
	DeclaredWeight    string `json:"declaredWeight,omitempty"`
	DeclaredUnit      string `json:"declaredUnit,omitempty"`
	DeclaredSource    string `json:"declaredSource,omitempty"`
	MeasuredWeight    string `json:"measuredWeight"`
	MeasuredUnit      string `json:"measuredUnit"`
	MeasurementSource string `json:"measurementSource"`
	WithinTolerance   bool   `json:"withinTolerance"`
}

type weightDiscrepancyDocument struct {
	State             string                            `json:"state"`
	SubmissionVersion string                            `json:"submissionVersion"`
	MaxDelta          string                            `json:"maxDelta"`
	Unit              string                            `json:"unit"`
	Members           []weightDiscrepancyMemberDocument `json:"members"`
}

func weightDiscrepancyArg(gate domain.ReleaseGateVerification) (any, error) {
	reading, has := gate.WeightDiscrepancy()
	if !has {
		return nil, nil
	}
	document := weightDiscrepancyDocument{
		State:             reading.State.String(),
		SubmissionVersion: reading.SubmissionVersion.String(),
		MaxDelta:          reading.MaxDelta,
		Unit:              reading.Unit,
		Members:           make([]weightDiscrepancyMemberDocument, 0, len(reading.Members)),
	}
	for _, member := range reading.Members {
		document.Members = append(document.Members, weightDiscrepancyMemberDocument{
			Parcel:            member.Parcel.String(),
			DeclaredWeight:    member.DeclaredWeight,
			DeclaredUnit:      member.DeclaredUnit,
			DeclaredSource:    member.DeclaredSource,
			MeasuredWeight:    member.MeasuredWeight,
			MeasuredUnit:      member.MeasuredUnit,
			MeasurementSource: member.MeasurementSource,
			WithinTolerance:   member.WithinTolerance,
		})
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func attachWeightDiscrepancy(gate domain.ReleaseGateVerification, raw []byte) (domain.ReleaseGateVerification, error) {
	var document weightDiscrepancyDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	state, err := closedWord(document.State, domain.PreconditionMet, domain.PreconditionUnmet)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	version, err := domain.NewSubmissionVersionID(document.SubmissionVersion)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	members := make([]domain.WeightDiscrepancyMemberReading, 0, len(document.Members))
	for _, member := range document.Members {
		parcel, err := domain.NewDeclaredParcelReference(member.Parcel)
		if err != nil {
			return domain.ReleaseGateVerification{}, err
		}
		members = append(members, domain.WeightDiscrepancyMemberReading{
			Parcel:            parcel,
			DeclaredWeight:    member.DeclaredWeight,
			DeclaredUnit:      member.DeclaredUnit,
			DeclaredSource:    member.DeclaredSource,
			MeasuredWeight:    member.MeasuredWeight,
			MeasuredUnit:      member.MeasuredUnit,
			MeasurementSource: member.MeasurementSource,
			WithinTolerance:   member.WithinTolerance,
		})
	}
	return gate.WithWeightDiscrepancy(domain.WeightDiscrepancyGateReading{
		State:             state,
		SubmissionVersion: version,
		MaxDelta:          document.MaxDelta,
		Unit:              document.Unit,
		Members:           members,
	})
}
