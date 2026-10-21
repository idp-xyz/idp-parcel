package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SubmissionOwnershipAnswers 实现 ports.SubmissionOwnershipAnswers：每一次判得的归属答案追加一行，决定整份进 jsonb。
//
// 读回逐字段过领域构造函数（NewProductionOwnershipDecision、AssessSafeHandoff、WithSafeHandoff）：库里一行坏数据在那几道门上
// 暴露，不会变成一份看起来合法的原归属结果交给客户。
type SubmissionOwnershipAnswers struct {
	db *bentopg.DB
}

func NewSubmissionOwnershipAnswers(db *bentopg.DB) (*SubmissionOwnershipAnswers, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &SubmissionOwnershipAnswers{db: db}, nil
}

var _ ports.SubmissionOwnershipAnswers = (*SubmissionOwnershipAnswers)(nil)

// RecordOwnershipAnswer 追加一次归属答案。走 RequireExecutor：它与来源保全一样要落在一个明确的事务里，事务边界归装配点。
func (repository *SubmissionOwnershipAnswers) RecordOwnershipAnswer(
	ctx context.Context,
	identity domain.SourceIdentity,
	answer ports.SubmissionOwnershipAnswer,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record ownership answer: %w", err)
	}
	// 放行与「有将要建的委托」两格错配是编排的错，在这里拒而不是让库约束答一个 23514：错配的答案落了库，重发就会把一份挡下
	// 的答案当成建单所据。
	if answer.Allowed() != (answer.ShipmentRequestID.String() != "") {
		return fmt.Errorf("record ownership answer: an allowed answer must name the request it builds, a blocked one none")
	}
	reasons := make([]int16, 0, len(answer.BlockReasons))
	for _, reason := range answer.BlockReasons {
		if reason.String() == "" {
			return fmt.Errorf("record ownership answer: gate block reason %d is outside the vocabulary", reason)
		}
		reasons = append(reasons, int16(reason))
	}
	raw, err := json.Marshal(ownershipDecisionDocumentOf(answer.Decision))
	if err != nil {
		return fmt.Errorf("record ownership answer: %w", err)
	}
	var requestID *string
	if answer.Allowed() {
		value := answer.ShipmentRequestID.String()
		requestID = &value
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO parcel_shipment.submission_ownership_answer
			(tenant_id, customer_account_id, source, source_request_key,
			 block_reasons, shipment_request_id, decision)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		reasons,
		requestID,
		raw,
	)
	if err != nil {
		return fmt.Errorf("record ownership answer: %w", err)
	}
	return nil
}

// FindLatestOwnershipAnswer 取这份来源身份最近记下的一次答案。否定结果不区分「没判过」与「属于另一个作用域」，与来源保全
// 仓储同一条纪律。
func (repository *SubmissionOwnershipAnswers) FindLatestOwnershipAnswer(
	ctx context.Context,
	identity domain.SourceIdentity,
) (ports.SubmissionOwnershipAnswer, bool, error) {
	return repository.findOne(ctx, "find latest ownership answer",
		`SELECT block_reasons, shipment_request_id, decision
		   FROM parcel_shipment.submission_ownership_answer
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND source = $3
		    AND source_request_key = $4
		  ORDER BY answer_id DESC
		  LIMIT 1`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	)
}

// FindOwnershipAnswerForRequest 取放行建这份委托的那一次答案。同一个号记过不止一次时取最早那一次：建单在记答案之后，最早放行
// 它的那一次一定先于建单落下。
func (repository *SubmissionOwnershipAnswers) FindOwnershipAnswerForRequest(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequestID,
) (ports.SubmissionOwnershipAnswer, bool, error) {
	return repository.findOne(ctx, "find ownership answer for request",
		`SELECT block_reasons, shipment_request_id, decision
		   FROM parcel_shipment.submission_ownership_answer
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND source = $3
		    AND source_request_key = $4
		    AND shipment_request_id = $5
		  ORDER BY answer_id
		  LIMIT 1`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		request.String(),
	)
}

func (repository *SubmissionOwnershipAnswers) findOne(
	ctx context.Context,
	operation string,
	query string,
	arguments ...any,
) (ports.SubmissionOwnershipAnswer, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SubmissionOwnershipAnswer{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var reasons []int16
	var requestID *string
	var raw []byte
	err = querier.QueryRow(ctx, query, arguments...).Scan(&reasons, &requestID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SubmissionOwnershipAnswer{}, false, nil
	}
	if err != nil {
		return ports.SubmissionOwnershipAnswer{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	answer, err := ownershipAnswerFromRow(reasons, requestID, raw)
	if err != nil {
		return ports.SubmissionOwnershipAnswer{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return answer, true, nil
}

func ownershipAnswerFromRow(reasons []int16, requestID *string, raw []byte) (ports.SubmissionOwnershipAnswer, error) {
	var document ownershipDecisionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return ports.SubmissionOwnershipAnswer{}, fmt.Errorf("决定不是本适配器写下的形状：%w", err)
	}
	decision, err := document.decision()
	if err != nil {
		return ports.SubmissionOwnershipAnswer{}, err
	}
	answer := ports.SubmissionOwnershipAnswer{Decision: decision}
	for _, stored := range reasons {
		reason := domain.FutureSubmissionBlockReason(stored)
		if stored < 0 || stored > 255 || reason.String() == "" {
			return ports.SubmissionOwnershipAnswer{}, fmt.Errorf("gate block reason %d is outside the vocabulary", stored)
		}
		answer.BlockReasons = append(answer.BlockReasons, reason)
	}
	if requestID != nil {
		answer.ShipmentRequestID, err = domain.NewShipmentRequestID(*requestID)
		if err != nil {
			return ports.SubmissionOwnershipAnswer{}, err
		}
	}
	return answer, nil
}

// ownershipDecisionDocument 是归属决定的存储形状，字段照 ProductionOwnershipDecisionSpec 与 SafeHandoffAssessmentSpec 摊开；
// 可选引用缺席即空串，枚举存数值（与委托快照同一种做法）。交接评估的范围不另存：它必须与决定同一范围（WithSafeHandoff 守的）。
type ownershipDecisionDocument struct {
	DecisionID            string               `json:"decisionId"`
	ScopeReference        string               `json:"scopeReference"`
	ScopeDigest           string               `json:"scopeDigest"`
	Authority             uint8                `json:"authority"`
	OtherAuthority        string               `json:"otherAuthority,omitempty"`
	HandoffReference      string               `json:"handoffReference,omitempty"`
	UnresolvedReason      uint8                `json:"unresolvedReason,omitempty"`
	ContinuationReference string               `json:"continuationReference,omitempty"`
	AdmissionControl      uint8                `json:"admissionControl"`
	SuspensionReference   string               `json:"suspensionReference,omitempty"`
	RuleVersion           string               `json:"ruleVersion"`
	AsOf                  time.Time            `json:"asOf"`
	ValidFrom             time.Time            `json:"validFrom"`
	ValidUntil            time.Time            `json:"validUntil"`
	Revision              string               `json:"revision"`
	DecisionAt            time.Time            `json:"decisionAt"`
	SafeHandoff           *safeHandoffDocument `json:"safeHandoff,omitempty"`
}

type safeHandoffDocument struct {
	AttemptID             string    `json:"attemptId"`
	TargetAuthority       string    `json:"targetAuthority"`
	Observation           uint8     `json:"observation"`
	ConfirmedScopeDigest  string    `json:"confirmedScopeDigest,omitempty"`
	ConfirmationReference string    `json:"confirmationReference,omitempty"`
	QueryReference        string    `json:"queryReference,omitempty"`
	ContinuationReference string    `json:"continuationReference,omitempty"`
	EffectiveAt           time.Time `json:"effectiveAt,omitzero"`
	AssessedAt            time.Time `json:"assessedAt"`
}

func ownershipDecisionDocumentOf(decision domain.ProductionOwnershipDecision) ownershipDecisionDocument {
	document := ownershipDecisionDocument{
		DecisionID:       decision.DecisionID().String(),
		ScopeReference:   decision.Scope().Reference().String(),
		ScopeDigest:      decision.Scope().Digest().String(),
		Authority:        uint8(decision.Authority()),
		AdmissionControl: uint8(decision.AdmissionControl()),
		RuleVersion:      decision.RuleVersion().String(),
		AsOf:             decision.AsOf().UTC(),
		ValidFrom:        decision.Validity().ValidFrom().UTC(),
		ValidUntil:       decision.Validity().ValidUntil().UTC(),
		Revision:         decision.Revision().String(),
		DecisionAt:       decision.DecisionAt().UTC(),
	}
	if other, present := decision.OtherAuthorityReference(); present {
		document.OtherAuthority = other.String()
	}
	if handoff, present := decision.HandoffReference(); present {
		document.HandoffReference = handoff.String()
	}
	if reason, continuation, present := decision.UnresolvedDetails(); present {
		document.UnresolvedReason = uint8(reason)
		document.ContinuationReference = continuation.String()
	}
	if suspension, present := decision.SuspensionReference(); present {
		document.SuspensionReference = suspension.String()
	}
	if assessment, recorded := decision.SafeHandoff(); recorded {
		spec := assessment.FormationSpec()
		handoff := &safeHandoffDocument{
			AttemptID:             spec.AttemptID.String(),
			TargetAuthority:       spec.TargetAuthority.String(),
			Observation:           uint8(spec.Observation),
			ConfirmedScopeDigest:  spec.ConfirmedScopeDigest.String(),
			ConfirmationReference: spec.ConfirmationRef.String(),
			QueryReference:        spec.QueryRef.String(),
			ContinuationReference: spec.ContinuationRef.String(),
			AssessedAt:            spec.AssessedAt.UTC(),
		}
		if !spec.EffectiveAt.IsZero() {
			handoff.EffectiveAt = spec.EffectiveAt.UTC()
		}
		document.SafeHandoff = handoff
	}
	return document
}

func (document ownershipDecisionDocument) decision() (domain.ProductionOwnershipDecision, error) {
	scopeReference, err := domain.NewAdmissionScopeReference(document.ScopeReference)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	scopeDigest, err := domain.NewAdmissionScopeDigest(document.ScopeDigest)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	scope, err := domain.NewAdmissionScope(scopeReference, scopeDigest)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	validity, err := domain.NewOwnershipValidityInterval(document.ValidFrom, document.ValidUntil)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	spec := domain.ProductionOwnershipDecisionSpec{
		Scope:            scope,
		Authority:        domain.ProductionAuthorityKind(document.Authority),
		UnresolvedReason: domain.OwnershipUnresolvedReason(document.UnresolvedReason),
		AdmissionControl: domain.AdmissionControl(document.AdmissionControl),
		AsOf:             document.AsOf,
		Validity:         validity,
		DecisionAt:       document.DecisionAt,
	}
	if spec.DecisionID, err = domain.NewProductionOwnershipDecisionID(document.DecisionID); err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	if spec.RuleVersion, err = domain.NewProductionOwnershipRuleVersion(document.RuleVersion); err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	if spec.Revision, err = domain.NewProductionOwnershipRevision(document.Revision); err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	if document.OtherAuthority != "" {
		if spec.OtherAuthorityRef, err = domain.NewProductionAuthorityReference(document.OtherAuthority); err != nil {
			return domain.ProductionOwnershipDecision{}, err
		}
	}
	if document.HandoffReference != "" {
		if spec.HandoffRef, err = domain.NewHandoffConfirmationReference(document.HandoffReference); err != nil {
			return domain.ProductionOwnershipDecision{}, err
		}
	}
	if document.ContinuationReference != "" {
		if spec.ContinuationRef, err = domain.NewOwnershipContinuationReference(document.ContinuationReference); err != nil {
			return domain.ProductionOwnershipDecision{}, err
		}
	}
	if document.SuspensionReference != "" {
		if spec.SuspensionRef, err = domain.NewOwnershipSuspensionReference(document.SuspensionReference); err != nil {
			return domain.ProductionOwnershipDecision{}, err
		}
	}
	decision, err := domain.NewProductionOwnershipDecision(spec)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	if document.SafeHandoff == nil {
		return decision, nil
	}
	assessment, err := document.SafeHandoff.assessment(scope)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	return decision.WithSafeHandoff(assessment)
}

func (document safeHandoffDocument) assessment(scope domain.AdmissionScope) (domain.SafeHandoffAssessment, error) {
	spec := domain.SafeHandoffAssessmentSpec{
		Scope:       scope,
		Observation: domain.HandoffObservation(document.Observation),
		EffectiveAt: document.EffectiveAt,
		AssessedAt:  document.AssessedAt,
	}
	var err error
	if spec.AttemptID, err = domain.NewHandoffAttemptID(document.AttemptID); err != nil {
		return domain.SafeHandoffAssessment{}, err
	}
	if spec.TargetAuthority, err = domain.NewProductionAuthorityReference(document.TargetAuthority); err != nil {
		return domain.SafeHandoffAssessment{}, err
	}
	if document.ConfirmedScopeDigest != "" {
		if spec.ConfirmedScopeDigest, err = domain.NewAdmissionScopeDigest(document.ConfirmedScopeDigest); err != nil {
			return domain.SafeHandoffAssessment{}, err
		}
	}
	if document.ConfirmationReference != "" {
		if spec.ConfirmationRef, err = domain.NewHandoffConfirmationReference(document.ConfirmationReference); err != nil {
			return domain.SafeHandoffAssessment{}, err
		}
	}
	if document.QueryReference != "" {
		if spec.QueryRef, err = domain.NewHandoffQueryReference(document.QueryReference); err != nil {
			return domain.SafeHandoffAssessment{}, err
		}
	}
	if document.ContinuationReference != "" {
		if spec.ContinuationRef, err = domain.NewOwnershipContinuationReference(document.ContinuationReference); err != nil {
			return domain.SafeHandoffAssessment{}, err
		}
	}
	return domain.AssessSafeHandoff(spec)
}
