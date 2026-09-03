package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ExternalTrackingFacts 实现 ports.ExternalTrackingFactRegistry 与
// ports.UnadoptedTrackingMaterialLedger：事实一个版本一行，留痕一条一行，两张表都只插不改。
//
// 一个类型担两个端口是因为两者是同一次收编的两种去向；装配处仍按端口各自注入。
type ExternalTrackingFacts struct {
	db *bentopg.DB
}

func NewExternalTrackingFacts(db *bentopg.DB) (*ExternalTrackingFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ExternalTrackingFacts{db: db}, nil
}

var (
	_ ports.ExternalTrackingFactRegistry    = (*ExternalTrackingFacts)(nil)
	_ ports.UnadoptedTrackingMaterialLedger = (*ExternalTrackingFacts)(nil)
)

const externalTrackingFactColumns = `tenant_id, fact_ref, version, source_ref, credential_ref, object_ref,
		        source_event, status_ref, occurred_at, received_at,
		        effective_basis, effective_at, effective_rule, effective_rule_version,
		        correction_of, supersedes_version, version_origin, recorded_at`

// FindByKey 按（租户+事实+版本）取回一个版本。否定结果只回 false。
func (repository *ExternalTrackingFacts) FindByKey(
	ctx context.Context,
	key ports.ExternalTrackingFactKey,
) (ports.ExternalTrackingFactRecord, bool, error) {
	return repository.findOne(ctx, "find external tracking fact",
		`SELECT `+externalTrackingFactColumns+`
		   FROM transport_fulfillment.external_carrier_tracking_fact
		  WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3`,
		key.TenantID.String(), key.Fact.String(), key.Version.String())
}

// FindBySourceEvent 按幂等锚（租户+源+源事件）取回该源事件到达时形成的那一版。只看素材版本：
// 判断版本原样携带同一个源事件，但它不是那次到达。
func (repository *ExternalTrackingFacts) FindBySourceEvent(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
	event domain.SourceEventReference,
) (ports.ExternalTrackingFactRecord, bool, error) {
	return repository.findOne(ctx, "find external tracking fact by source event",
		`SELECT `+externalTrackingFactColumns+`
		   FROM transport_fulfillment.external_carrier_tracking_fact
		  WHERE tenant_id = $1 AND source_ref = $2 AND source_event = $3
		    AND version_origin = $4`,
		tenant.String(), source.String(), event.String(), domain.VersionFromMaterial.String())
}

// FindCurrent 取回一条事实此刻未被任何版本回指的那一版。「当前」是派生问答不是可变标记：
// 表上没有 current 列，改一版就得回写的东西这里一个都没有。
func (repository *ExternalTrackingFacts) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalTrackingFactReference,
) (ports.ExternalTrackingFactRecord, bool, error) {
	return repository.findOne(ctx, "find current external tracking fact",
		`SELECT `+externalTrackingFactColumns+`
		   FROM transport_fulfillment.external_carrier_tracking_fact AS current
		  WHERE tenant_id = $1 AND fact_ref = $2
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.external_carrier_tracking_fact AS successor
		         WHERE successor.tenant_id = current.tenant_id
		           AND successor.fact_ref = current.fact_ref
		           AND successor.supersedes_version = current.version)
		  ORDER BY recorded_at DESC
		  LIMIT 1`,
		tenant.String(), fact.String())
}

func (repository *ExternalTrackingFacts) findOne(
	ctx context.Context,
	verb string,
	query string,
	args ...any,
) (ports.ExternalTrackingFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	var tenantID, factRef, version, sourceRef, credentialRef, objectRef, statusRef, basisText, originText string
	var sourceEvent, effectiveRule, effectiveRuleVersion, correctionOf, supersedes *string
	var occurredAt, receivedAt, recordedAt time.Time
	var effectiveAt *time.Time
	err = querier.QueryRow(ctx, query, args...).Scan(
		&tenantID, &factRef, &version, &sourceRef, &credentialRef, &objectRef,
		&sourceEvent, &statusRef, &occurredAt, &receivedAt,
		&basisText, &effectiveAt, &effectiveRule, &effectiveRuleVersion,
		&correctionOf, &supersedes, &originText, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ExternalTrackingFactRecord{}, false, nil
	}
	if err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}

	spec := domain.RehydrateExternalTrackingFactSpec{
		OccurredAt: occurredAt.UTC(),
		ReceivedAt: receivedAt.UTC(),
	}
	// 逐列走各自的构造门装回，不按列直接拼结构体。
	if spec.TenantID, err = domain.NewTenantID(tenantID); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if spec.Fact, err = domain.NewExternalTrackingFactReference(factRef); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if spec.Version, err = domain.NewExternalTrackingFactVersion(version); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if spec.Source, err = domain.NewTrackingSourceReference(sourceRef); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if spec.Credential, err = domain.NewExternalCarrierCredentialReference(credentialRef); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if spec.Object, err = domain.NewCarriedObjectReference(objectRef); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if spec.Status, err = domain.NewRawStatusReference(statusRef); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if sourceEvent != nil {
		if spec.SourceEvent, err = domain.NewSourceEventReference(*sourceEvent); err != nil {
			return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
		}
	}
	if correctionOf != nil {
		if spec.CorrectionOf, err = domain.NewSourceEventReference(*correctionOf); err != nil {
			return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
		}
	}
	if supersedes != nil {
		if spec.Supersedes, err = domain.NewExternalTrackingFactVersion(*supersedes); err != nil {
			return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
		}
	}
	if spec.EffectiveBasis, err = effectiveTimeBasisFrom(basisText); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if spec.Origin, err = versionOriginFrom(originText); err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	if effectiveAt != nil {
		spec.EffectiveAt = effectiveAt.UTC()
	}
	if effectiveRule != nil {
		spec.EffectiveRule = *effectiveRule
	}
	if effectiveRuleVersion != nil {
		spec.EffectiveRuleVersion = *effectiveRuleVersion
	}

	fact, err := domain.RehydrateExternalTrackingFact(spec)
	if err != nil {
		return ports.ExternalTrackingFactRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	return ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Fact:       fact,
		RecordedAt: recordedAt.UTC(),
	}, true, nil
}

// Save 登记一个版本。撞键答`已登记`（ADR-0031）：撞的可能是主键，也可能是（源，源事件）那个
// 幂等锚——两者对编排都是「另一方先落了」，读回赢家即可。
func (repository *ExternalTrackingFacts) Save(
	ctx context.Context,
	record ports.ExternalTrackingFactRecord,
) (ports.ExternalTrackingFactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ExternalTrackingFactSaveOutcomeInvalid, fmt.Errorf("save external tracking fact: %w", err)
	}
	if err := assertExternalTrackingKeyAgrees(record); err != nil {
		return ports.ExternalTrackingFactSaveOutcomeInvalid, fmt.Errorf("save external tracking fact: %w", err)
	}

	fact := record.Fact
	var sourceEvent, correctionOf, supersedes, ruleText, ruleVersionText *string
	if event, given := fact.SourceEvent(); given {
		text := event.String()
		sourceEvent = &text
	}
	if declared, has := fact.CorrectionOf(); has {
		text := declared.String()
		correctionOf = &text
	}
	if prior, has := fact.Supersedes(); has {
		text := prior.String()
		supersedes = &text
	}
	if rule, byRule := fact.Effective().Rule(); byRule {
		ruleName, ruleVersion := rule.Rule(), rule.Version()
		ruleText, ruleVersionText = &ruleName, &ruleVersion
	}
	effectiveAt, judged := fact.EffectiveAt()

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.external_carrier_tracking_fact
		     (`+externalTrackingFactColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Fact.String(),
		record.Key.Version.String(),
		fact.Source().String(),
		fact.Credential().String(),
		fact.Object().String(),
		sourceEvent,
		fact.Status().String(),
		fact.OccurredAt().UTC(),
		fact.ReceivedAt().UTC(),
		fact.Effective().Basis().String(),
		nullableTime(effectiveAt, judged),
		ruleText,
		ruleVersionText,
		correctionOf,
		supersedes,
		fact.Origin().String(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ExternalTrackingFactSaveOutcomeInvalid, fmt.Errorf("save external tracking fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ExternalTrackingFactAlreadyRegistered, nil
	}
	return ports.ExternalTrackingFactSaved, nil
}

// RecordUnadopted 追加一条留痕。没有幂等键是有意的：源未给事件标识的素材不判重，按内容判重
// 等于替源发明一个身份；同一素材真被拉到两次就如实留两条。
func (repository *ExternalTrackingFacts) RecordUnadopted(
	ctx context.Context,
	entry ports.UnadoptedTrackingMaterial,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record unadopted tracking material: %w", err)
	}
	if entry.Reason.String() == "" {
		return fmt.Errorf("record unadopted tracking material: reason is required")
	}
	var sourceEvent *string
	if entry.SourceEvent != "" {
		text := entry.SourceEvent
		sourceEvent = &text
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.unadopted_tracking_material
		     (tenant_id, source_ref, source_event, credential_ref, status_ref,
		      received_at, payload_digest, reason, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entry.TenantID.String(),
		string(entry.Source),
		sourceEvent,
		entry.Credential,
		entry.Status,
		entry.ReceivedAt.UTC(),
		entry.PayloadDigest,
		entry.Reason.String(),
		entry.RecordedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("record unadopted tracking material: %w", err)
	}
	return nil
}

// assertExternalTrackingKeyAgrees 挡住「键说的是一个版本、聚合说的是另一个」那种写入。
func assertExternalTrackingKeyAgrees(record ports.ExternalTrackingFactRecord) error {
	if record.Key.TenantID != record.Fact.TenantID() ||
		record.Key.Fact != record.Fact.Fact() ||
		record.Key.Version != record.Fact.Version() {
		return fmt.Errorf("external tracking fact key disagrees with the aggregate")
	}
	return nil
}

func versionOriginFrom(raw string) (domain.VersionOrigin, error) {
	switch raw {
	case domain.VersionFromMaterial.String():
		return domain.VersionFromMaterial, nil
	case domain.VersionFromJudgment.String():
		return domain.VersionFromJudgment, nil
	default:
		return domain.VersionOriginInvalid, fmt.Errorf("unknown version origin %q", raw)
	}
}

func effectiveTimeBasisFrom(raw string) (domain.EffectiveTimeBasis, error) {
	switch raw {
	case domain.EffectiveTimePending.String():
		return domain.EffectiveTimePending, nil
	case domain.EffectiveTimeJudgedExplicitly.String():
		return domain.EffectiveTimeJudgedExplicitly, nil
	case domain.EffectiveTimeJudgedByRule.String():
		return domain.EffectiveTimeJudgedByRule, nil
	default:
		return domain.EffectiveTimeBasisInvalid, fmt.Errorf("unknown effective time basis %q", raw)
	}
}
