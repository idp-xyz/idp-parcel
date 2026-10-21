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

// SubmissionAnswers 实现 ports.SubmissionAnswers：每一次答出的答案追加一行，拒绝存一格，归属决定整份进 jsonb。
//
// 读回逐字段过领域构造函数（NewProductionOwnershipDecision、AssessSafeHandoff、WithSafeHandoff）：库里一行坏数据在那几道门上
// 暴露，不会变成一份看起来合法的原结果交给客户。
type SubmissionAnswers struct {
	db *bentopg.DB
}

func NewSubmissionAnswers(db *bentopg.DB) (*SubmissionAnswers, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &SubmissionAnswers{db: db}, nil
}

var _ ports.SubmissionAnswers = (*SubmissionAnswers)(nil)

// LockSubmission 在当前事务里取这份来源身份的续办锁，事务结束即放（pg_advisory_xact_lock）。它是 ports.SubmissionContinuationGuard
// 在本库的那一半，守卫的事务由装配点开。用咨询锁而不对 source_submission 那一行 FOR UPDATE：守卫事务要一直持锁到建单落下，而别的
// 重放在守卫外追加观察要对那一行取外键共享锁，FOR UPDATE 与它互斥，它们连这一次到达都记不下，只能干等。
func (repository *SubmissionAnswers) LockSubmission(ctx context.Context, identity domain.SourceIdentity) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock submission: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('parcel_shipment.submission_answer.source'),
		        hashtext($1 || chr(31) || $2 || chr(31) || $3 || chr(31) || $4))`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	); err != nil {
		return fmt.Errorf("lock submission: %w", err)
	}
	return nil
}

// RecordSubmissionAnswer 追加一次答案。走 RequireExecutor：它与来源保全一样要落在一个明确的事务里，事务边界归装配点。
func (repository *SubmissionAnswers) RecordSubmissionAnswer(
	ctx context.Context,
	identity domain.SourceIdentity,
	answer ports.SubmissionAnswer,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record submission answer: %w", err)
	}
	// 几格错配是编排的错，在这里拒而不是让库约束答一个 23514：错配的答案落了库，重发就会把一份没放行的答案当成建单所据，或把
	// 一格拒绝读成一次归属判断。
	if answer.Allowed() != (answer.ShipmentRequestID.String() != "") {
		return fmt.Errorf("record submission answer: an allowed answer must name the request it builds, any other none")
	}
	if answer.Refused() && (answer.Refusal.String() == "" || len(answer.BlockReasons) > 0) {
		return fmt.Errorf("record submission answer: a refusal %d carries no gate", answer.Refusal)
	}
	reasons := make([]int16, 0, len(answer.BlockReasons))
	for _, reason := range answer.BlockReasons {
		if reason.String() == "" {
			return fmt.Errorf("record submission answer: gate block reason %d is outside the vocabulary", reason)
		}
		reasons = append(reasons, int16(reason))
	}
	var refusal *int16
	var raw []byte
	if answer.Refused() {
		value := int16(answer.Refusal)
		refusal = &value
	} else {
		raw, err = json.Marshal(ownershipDecisionDocumentOf(answer.Decision))
		if err != nil {
			return fmt.Errorf("record submission answer: %w", err)
		}
	}
	var requestID *string
	if answer.Allowed() {
		value := answer.ShipmentRequestID.String()
		requestID = &value
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO parcel_shipment.submission_answer
			(tenant_id, customer_account_id, source, source_request_key,
			 refusal, block_reasons, shipment_request_id, decision)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		refusal,
		reasons,
		requestID,
		raw,
	)
	if err != nil {
		return fmt.Errorf("record submission answer: %w", err)
	}
	return nil
}

// FindLatestSubmissionAnswer 取这份来源身份最近记下的一次答案。否定结果不区分「没答过」与「属于另一个作用域」，与来源保全
// 仓储同一条纪律。
func (repository *SubmissionAnswers) FindLatestSubmissionAnswer(
	ctx context.Context,
	identity domain.SourceIdentity,
) (ports.SubmissionAnswer, bool, error) {
	return repository.findOne(ctx, "find latest submission answer",
		`SELECT refusal, block_reasons, shipment_request_id, decision
		   FROM parcel_shipment.submission_answer
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

// FindSubmissionAnswerForRequest 取放行建这份委托的那一次答案。同一个号记过不止一次时取最早那一次：建单在记答案之后，最早放行
// 它的那一次一定先于建单落下。
func (repository *SubmissionAnswers) FindSubmissionAnswerForRequest(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequestID,
) (ports.SubmissionAnswer, bool, error) {
	return repository.findOne(ctx, "find submission answer for request",
		`SELECT refusal, block_reasons, shipment_request_id, decision
		   FROM parcel_shipment.submission_answer
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

func (repository *SubmissionAnswers) findOne(
	ctx context.Context,
	operation string,
	query string,
	arguments ...any,
) (ports.SubmissionAnswer, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SubmissionAnswer{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var refusal *int16
	var reasons []int16
	var requestID *string
	var raw []byte
	err = querier.QueryRow(ctx, query, arguments...).Scan(&refusal, &reasons, &requestID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SubmissionAnswer{}, false, nil
	}
	if err != nil {
		return ports.SubmissionAnswer{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	answer, err := submissionAnswerFromRow(refusal, reasons, requestID, raw)
	if err != nil {
		return ports.SubmissionAnswer{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return answer, true, nil
}

func submissionAnswerFromRow(refusal *int16, reasons []int16, requestID *string, raw []byte) (ports.SubmissionAnswer, error) {
	var answer ports.SubmissionAnswer
	if refusal != nil {
		answer.Refusal = ports.SubmissionRefusal(*refusal)
		if *refusal < 0 || *refusal > 255 || answer.Refusal.String() == "" {
			return ports.SubmissionAnswer{}, fmt.Errorf("refusal %d is outside the vocabulary", *refusal)
		}
		return answer, nil
	}
	var document ownershipDecisionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return ports.SubmissionAnswer{}, fmt.Errorf("决定不是本适配器写下的形状：%w", err)
	}
	decision, err := document.decision()
	if err != nil {
		return ports.SubmissionAnswer{}, err
	}
	answer.Decision = decision
	for _, stored := range reasons {
		reason := domain.FutureSubmissionBlockReason(stored)
		if stored < 0 || stored > 255 || reason.String() == "" {
			return ports.SubmissionAnswer{}, fmt.Errorf("gate block reason %d is outside the vocabulary", stored)
		}
		answer.BlockReasons = append(answer.BlockReasons, reason)
	}
	if requestID != nil {
		answer.ShipmentRequestID, err = domain.NewShipmentRequestID(*requestID)
		if err != nil {
			return ports.SubmissionAnswer{}, err
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
