package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// OperationsRecordViews 实现关务运营查阅的四本记录读面（案件、提交版本、外部监管结果、动作准入判断册）。
// 它们是转述已存在事实的读面，不折叠结论：技术回执、监管接收、业务受理与放行分层保存，各层原样上列。
//
// 父子关系**一条语句取回**：ReadExecutor 不保证两条语句同一快照，分次取会拼出从未同时存在的父子状态
// （做法同 GateConditionCatalogue）。子集走相关子查询 json_agg 各聚各的，避免多族 LEFT JOIN 的笛卡尔积。
//
// 排序全部取稳定键，保证分页可重复。limit 非正是调用方编程错误，不静默答一页。
type OperationsRecordViews struct {
	db *bentopg.DB
}

func NewOperationsRecordViews(db *bentopg.DB) (*OperationsRecordViews, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &OperationsRecordViews{db: db}, nil
}

var (
	_ ports.CustomsCaseRecordRead        = (*OperationsRecordViews)(nil)
	_ ports.SubmissionRecordRead         = (*OperationsRecordViews)(nil)
	_ ports.ExternalResultViewRead       = (*OperationsRecordViews)(nil)
	_ ports.ActionAdmissionJudgementRead = (*OperationsRecordViews)(nil)
)

func (views *OperationsRecordViews) query(ctx context.Context, what string, limit int, sql string, args ...any) (rowsCloser, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list %s: limit must be positive, got %d", what, limit)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", what, err)
	}
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", what, err)
	}
	return rows, nil
}

type rowsCloser interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// —— 案件 ——

type caseUnitDocument struct {
	Unit      string    `json:"unit"`
	Procedure string    `json:"procedure"`
	Members   []string  `json:"members"`
	FormedAt  time.Time `json:"formedAt"`
	Replaces  *string   `json:"replaces"`
}

type caseClosureDocument struct {
	DecidedBy   string    `json:"decidedBy"`
	CutoffAt    time.Time `json:"cutoffAt"`
	VerifiedAt  time.Time `json:"verifiedAt"`
	ClosedAt    time.Time `json:"closedAt"`
	ItemCount   int       `json:"itemCount"`
	ReopenCount int       `json:"reopenCount"`
}

func (views *OperationsRecordViews) ListCustomsCases(
	ctx context.Context,
	tenant domain.TenantID,
	caseID string,
	limit int,
) ([]ports.CustomsCaseRecord, error) {
	rows, err := views.query(ctx, "customs cases", limit,
		`SELECT c.case_id, c.jurisdiction_ref, c.direction, c.procedure_ref, c.obligation_ref,
		        c.established_at, c.parcels, c.roles,
		        (SELECT COALESCE(json_agg(json_build_object(
		                    'unit', u.unit_id, 'procedure', u.procedure_ref, 'members', u.members,
		                    'formedAt', u.formed_at, 'replaces', u.replaces_unit_id)
		                    ORDER BY u.unit_id), '[]'::json)
		           FROM customs_compliance.declaration_unit AS u
		          WHERE u.tenant_id = c.tenant_id AND u.case_id = c.case_id),
		        (SELECT json_build_object(
		                    'decidedBy', cl.decided_by, 'cutoffAt', cl.cutoff_at,
		                    'verifiedAt', cl.verified_at, 'closedAt', cl.closed_at,
		                    'itemCount', jsonb_array_length(cl.items),
		                    'reopenCount', jsonb_array_length(cl.reopenings))
		           FROM customs_compliance.case_closure AS cl
		          WHERE cl.tenant_id = c.tenant_id AND cl.case_ref = c.case_id)
		   FROM customs_compliance.customs_case AS c
		  WHERE c.tenant_id = $1 AND ($2 = '' OR c.case_id = $2)
		  ORDER BY c.case_id
		  LIMIT $3`,
		tenant.String(), caseID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ports.CustomsCaseRecord, 0, limit)
	for rows.Next() {
		var (
			record                                 ports.CustomsCaseRecord
			parcelsRaw, rolesRaw, unitsRaw, closed []byte
		)
		if err := rows.Scan(&record.Case, &record.Jurisdiction, &record.Direction, &record.Procedure,
			&record.Obligation, &record.EstablishedAt, &parcelsRaw, &rolesRaw, &unitsRaw, &closed); err != nil {
			return nil, fmt.Errorf("list customs cases: %w", err)
		}
		record.EstablishedAt = record.EstablishedAt.UTC()
		var parcels []caseParcelRow
		if err := json.Unmarshal(parcelsRaw, &parcels); err != nil {
			return nil, fmt.Errorf("list customs cases: parcels: %w", err)
		}
		for _, parcel := range parcels {
			record.Parcels = append(record.Parcels, ports.CaseParcelRecord{
				Parcel: parcel.Parcel, Customer: parcel.Customer, SourceRef: parcel.SourceRef})
		}
		var roles []caseRoleRow
		if err := json.Unmarshal(rolesRaw, &roles); err != nil {
			return nil, fmt.Errorf("list customs cases: roles: %w", err)
		}
		for _, role := range roles {
			record.Roles = append(record.Roles, ports.CaseRoleRecord{
				Role: role.Role, Party: role.Party, Authority: role.Authority})
		}
		var units []caseUnitDocument
		if err := json.Unmarshal(unitsRaw, &units); err != nil {
			return nil, fmt.Errorf("list customs cases: units: %w", err)
		}
		for _, unit := range units {
			entry := ports.DeclarationUnitRecord{
				Unit: unit.Unit, Procedure: unit.Procedure, Members: unit.Members, FormedAt: unit.FormedAt.UTC()}
			if unit.Replaces != nil {
				entry.ReplacesUnit = *unit.Replaces
			}
			record.Units = append(record.Units, entry)
		}
		if len(closed) > 0 && string(closed) != "null" {
			var closure caseClosureDocument
			if err := json.Unmarshal(closed, &closure); err != nil {
				return nil, fmt.Errorf("list customs cases: closure: %w", err)
			}
			record.Closure = &ports.CaseClosureRecord{
				DecidedBy: closure.DecidedBy, CutoffAt: closure.CutoffAt.UTC(), VerifiedAt: closure.VerifiedAt.UTC(),
				ClosedAt: closure.ClosedAt.UTC(), ItemCount: closure.ItemCount, ReopenCount: closure.ReopenCount,
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customs cases: %w", err)
	}
	return records, nil
}

// —— 提交版本 ——

type attemptDocument struct {
	Sequence   int        `json:"sequence"`
	Target     string     `json:"target"`
	Result     string     `json:"result"`
	SafeResend *string    `json:"safeResend"`
	SentAt     *time.Time `json:"sentAt"`
	SendFact   *struct {
		SentAt         time.Time `json:"sentAt"`
		Outcome        string    `json:"outcome"`
		ChannelMessage *string   `json:"channelMessage"`
		ResponseRef    *string   `json:"responseRef"`
	} `json:"sendFact"`
	Receipts []struct {
		Source     string    `json:"source"`
		Outcome    string    `json:"outcome"`
		OccurredAt time.Time `json:"occurredAt"`
		ReceivedAt time.Time `json:"receivedAt"`
	} `json:"receipts"`
}

func orEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (views *OperationsRecordViews) ListSubmissionVersions(
	ctx context.Context,
	tenant domain.TenantID,
	version string,
	limit int,
) ([]ports.SubmissionVersionRecord, error) {
	rows, err := views.query(ctx, "submission versions", limit,
		`SELECT s.version_id, s.unit_id, s.procedure_ref, s.content_digest, s.members, s.dossier_ref, s.roles_ref,
		        s.readiness_basis, s.authority_ref, s.fixed_at, s.is_current, s.corrected_from,
		        (SELECT COALESCE(json_agg(json_build_object(
		                    'sequence', a.sequence, 'target', a.target, 'result', a.result,
		                    'safeResend', a.safe_resend_ref, 'sentAt', a.sent_at,
		                    'sendFact', (SELECT json_build_object(
		                                        'sentAt', f.sent_at, 'outcome', f.outcome,
		                                        'channelMessage', f.channel_message, 'responseRef', f.response_ref)
		                                   FROM customs_compliance.submission_send_fact AS f
		                                  WHERE f.tenant_id = a.tenant_id AND f.version_id = a.version_id
		                                    AND f.sequence = a.sequence),
		                    'receipts', (SELECT COALESCE(json_agg(json_build_object(
		                                        'source', r.source_id, 'outcome', r.outcome,
		                                        'occurredAt', r.occurred_at, 'receivedAt', r.received_at)
		                                        ORDER BY r.received_at, r.source_id), '[]'::json)
		                                   FROM customs_compliance.technical_receipt AS r
		                                  WHERE r.tenant_id = a.tenant_id AND r.version_id = a.version_id
		                                    AND r.sequence = a.sequence))
		                    ORDER BY a.sequence), '[]'::json)
		           FROM customs_compliance.submission_attempt AS a
		          WHERE a.tenant_id = s.tenant_id AND a.version_id = s.version_id)
		   FROM customs_compliance.declaration_submission AS s
		  WHERE s.tenant_id = $1 AND ($2 = '' OR s.version_id = $2)
		  ORDER BY s.unit_id, s.procedure_ref, s.fixed_at, s.version_id
		  LIMIT $3`,
		tenant.String(), version, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ports.SubmissionVersionRecord, 0, limit)
	for rows.Next() {
		var (
			record        ports.SubmissionVersionRecord
			membersRaw    []byte
			attemptsRaw   []byte
			correctedFrom *string
		)
		if err := rows.Scan(&record.Version, &record.Unit, &record.Procedure, &record.ContentDigest, &membersRaw,
			&record.Dossier, &record.Roles, &record.ReadinessBasis, &record.Authority, &record.FixedAt,
			&record.IsCurrent, &correctedFrom, &attemptsRaw); err != nil {
			return nil, fmt.Errorf("list submission versions: %w", err)
		}
		record.FixedAt = record.FixedAt.UTC()
		record.CorrectedFrom = orEmpty(correctedFrom)
		if err := json.Unmarshal(membersRaw, &record.Members); err != nil {
			return nil, fmt.Errorf("list submission versions: members: %w", err)
		}
		var attempts []attemptDocument
		if err := json.Unmarshal(attemptsRaw, &attempts); err != nil {
			return nil, fmt.Errorf("list submission versions: attempts: %w", err)
		}
		for _, attempt := range attempts {
			entry := ports.SubmissionAttemptRecord{
				Sequence: attempt.Sequence, Target: attempt.Target, Result: attempt.Result,
				SafeResend: orEmpty(attempt.SafeResend),
			}
			if attempt.SentAt != nil {
				entry.SentAt = attempt.SentAt.UTC()
			}
			if attempt.SendFact != nil {
				entry.SendFact = &ports.SendFactRecord{
					SentAt: attempt.SendFact.SentAt.UTC(), Outcome: attempt.SendFact.Outcome,
					ChannelMessage: orEmpty(attempt.SendFact.ChannelMessage), ResponseRef: orEmpty(attempt.SendFact.ResponseRef),
				}
			}
			for _, receipt := range attempt.Receipts {
				entry.Receipts = append(entry.Receipts, ports.TechnicalReceiptRecord{
					Source: receipt.Source, Outcome: receipt.Outcome,
					OccurredAt: receipt.OccurredAt.UTC(), ReceivedAt: receipt.ReceivedAt.UTC(),
				})
			}
			record.Attempts = append(record.Attempts, entry)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list submission versions: %w", err)
	}
	return records, nil
}

// —— 外部监管结果与放行 ——

func (views *OperationsRecordViews) ListExternalResults(
	ctx context.Context,
	tenant domain.TenantID,
	version string,
	limit int,
) ([]ports.ExternalResultViewRecord, error) {
	rows, err := views.query(ctx, "external results", limit,
		`SELECT e.source_id, COALESCE(e.result ->> 'layer', ''), COALESCE(e.result ->> 'role', ''),
		        COALESCE(NULLIF(e.raw_semantics, ''), e.result ->> 'rawSemantics', ''),
		        COALESCE(e.result ->> 'rule', ''), COALESCE(e.submission_version, ''), e.claimed_version,
		        COALESCE(e.result ->> 'scope', ''), e.unattributable, e.layer_conflict,
		        COALESCE(e.corrects_source_id, ''), e.correction_in_force,
		        (e.result ->> 'occurredAt')::timestamptz, (e.result ->> 'receivedAt')::timestamptz, e.recorded_at,
		        r.kind, r.authority_ref, r.scope_ref, r.condition
		   FROM customs_compliance.external_result AS e
		   LEFT JOIN customs_compliance.release_outcome AS r
		     ON r.tenant_id = e.tenant_id AND r.source_id = e.source_id
		  WHERE e.tenant_id = $1 AND ($2 = '' OR e.submission_version = $2)
		  ORDER BY e.recorded_at, e.source_id
		  LIMIT $3`,
		tenant.String(), version, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ports.ExternalResultViewRecord, 0, limit)
	for rows.Next() {
		var (
			record                                      ports.ExternalResultViewRecord
			occurredAt, receivedAt                      *time.Time
			releaseKind, releaseAuthority, releaseScope *string
			releaseCondition                            *string
		)
		if err := rows.Scan(&record.Source, &record.Layer, &record.Role, &record.RawSemantics, &record.Rule,
			&record.SubmissionVersion, &record.ClaimedVersion, &record.Scope, &record.Unattributable,
			&record.LayerConflict, &record.CorrectsSource, &record.CorrectionInForce,
			&occurredAt, &receivedAt, &record.RecordedAt,
			&releaseKind, &releaseAuthority, &releaseScope, &releaseCondition); err != nil {
			return nil, fmt.Errorf("list external results: %w", err)
		}
		record.RecordedAt = record.RecordedAt.UTC()
		if occurredAt != nil {
			record.OccurredAt = occurredAt.UTC()
		}
		if receivedAt != nil {
			record.ReceivedAt = receivedAt.UTC()
		}
		if releaseKind != nil {
			record.Release = &ports.ExternalResultReleaseRecord{
				Kind: *releaseKind, Authority: orEmpty(releaseAuthority),
				Scope: orEmpty(releaseScope), Condition: orEmpty(releaseCondition),
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list external results: %w", err)
	}
	return records, nil
}

// —— 动作准入判断册 ——

func (views *OperationsRecordViews) ListActionAdmissionJudgements(
	ctx context.Context,
	tenant domain.TenantID,
	object string,
	limit int,
) ([]ports.ActionAdmissionJudgementRecord, error) {
	rows, err := views.query(ctx, "action admission judgements", limit,
		`SELECT judgement_id, object_ref, intent_kind, node_ref, onward_node_ref, action_at,
		        cross_zone, answer, zones, findings, judged_at
		   FROM customs_compliance.action_admission_judgement
		  WHERE tenant_id = $1 AND ($2 = '' OR object_ref = $2)
		  ORDER BY judged_at DESC, judgement_id
		  LIMIT $3`,
		tenant.String(), object, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ports.ActionAdmissionJudgementRecord, 0, limit)
	for rows.Next() {
		var (
			record                ports.ActionAdmissionJudgementRecord
			onward                *string
			zonesRaw, findingsRaw []byte
		)
		if err := rows.Scan(&record.Judgement, &record.Object, &record.Intent, &record.Node, &onward,
			&record.ActionAt, &record.CrossZone, &record.Answer, &zonesRaw, &findingsRaw, &record.JudgedAt); err != nil {
			return nil, fmt.Errorf("list action admission judgements: %w", err)
		}
		record.OnwardNode = orEmpty(onward)
		record.ActionAt = record.ActionAt.UTC()
		record.JudgedAt = record.JudgedAt.UTC()
		var zones []admissionZoneRow
		if err := json.Unmarshal(zonesRaw, &zones); err != nil {
			return nil, fmt.Errorf("list action admission judgements: zones: %w", err)
		}
		for _, zone := range zones {
			record.Zones = append(record.Zones, ports.AdmissionZoneRecord{
				Node: zone.Node, Zone: zone.Zone, Registered: zone.Registered})
		}
		var findings []admissionFindingRow
		if err := json.Unmarshal(findingsRaw, &findings); err != nil {
			return nil, fmt.Errorf("list action admission judgements: findings: %w", err)
		}
		for _, finding := range findings {
			entry := ports.AdmissionFindingRecord{Action: finding.Action, Answer: finding.Answer}
			for _, basis := range finding.Bases {
				entry.Bases = append(entry.Bases, ports.AdmissionBasisRecord{
					Cause: basis.Cause, Scope: basis.Scope, Procedure: basis.Procedure, Sources: basis.Sources})
			}
			record.Findings = append(record.Findings, entry)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list action admission judgements: %w", err)
	}
	return records, nil
}
