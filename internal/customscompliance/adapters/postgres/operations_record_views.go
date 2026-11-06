package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// OperationsRecordViews 实现关务运营查阅的记录读面：案件、提交版本、外部监管结果、动作准入判断册，以及从判断册里
// 挑出的卡关对象。它们是转述已存在事实的读面，不折叠结论：技术回执、监管接收、业务受理与放行分层保存，各层原样上列。
//
// 父子关系**一条语句取回**：ReadExecutor 不保证两条语句同一快照，分次取会拼出从未同时存在的父子状态
// （做法同 GateConditionCatalogue）。子集走相关子查询 json_agg 各聚各的，避免多族 LEFT JOIN 的笛卡尔积。
//
// 各册按 ports 里的声明（CustomsCasePages 等）取页：总数与本页同一组条件，游标位置取自 ORDER BY 用的那几列的扫描值。
// limit 非正是调用方编程错误，不静默答一页。
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
	_ ports.UnadmittedObjectRead         = (*OperationsRecordViews)(nil)
)

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

// customsCasePages 把 ports.CustomsCasePages 落到列上。q 在案件标识、单据号、监管范围与义务、关联包裹的包裹号上做字面
// 包含；包裹号只取 parcels 数组里 parcel 那一格，键名、客户与来源引用不进检索。
var customsCasePages = ports.CustomsCasePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"case": "c.case_id", "establishedAt": "c.established_at"},
	Identity: []string{"c.case_id"},
	Filters:  map[string]string{"case": "c.case_id", "direction": "c.direction"},
	Keyword: []string{
		"c.case_id", "c.document_number", "c.jurisdiction_ref", "c.procedure_ref", "c.obligation_ref",
		"jsonb_path_query_array(c.parcels, '$[*].parcel')::text",
	},
})

func (views *OperationsRecordViews) ListCustomsCases(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomsCaseRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.CustomsCaseRecord], error) {
		return cataloguepage.Result[ports.CustomsCaseRecord]{}, fmt.Errorf("list customs cases: %w", err)
	}
	const source = "customs_compliance.customs_case AS c"
	page, err := openPage(ctx, views.db, customsCasePages, query, limit, source, "c.tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`c.case_id, c.document_number, c.document_number_absence,
		        c.jurisdiction_ref, c.direction, c.procedure_ref, c.obligation_ref,
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
		          WHERE cl.tenant_id = c.tenant_id AND cl.case_ref = c.case_id)`,
		source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	records := make([]ports.CustomsCaseRecord, 0, limit+1)
	for rows.Next() {
		var (
			record                                 ports.CustomsCaseRecord
			number, absence                        *string
			parcelsRaw, rolesRaw, unitsRaw, closed []byte
		)
		if err := rows.Scan(&record.Case, &number, &absence, &record.Jurisdiction, &record.Direction, &record.Procedure,
			&record.Obligation, &record.EstablishedAt, &parcelsRaw, &rolesRaw, &unitsRaw, &closed); err != nil {
			return fail(err)
		}
		record.DocumentNumber, record.DocumentNumberAbsence = caseNumberRecord(number, absence)
		record.EstablishedAt = record.EstablishedAt.UTC()
		var parcels []caseParcelRow
		if err := json.Unmarshal(parcelsRaw, &parcels); err != nil {
			return fail(fmt.Errorf("parcels: %w", err))
		}
		for _, parcel := range parcels {
			record.Parcels = append(record.Parcels, ports.CaseParcelRecord{
				Parcel: parcel.Parcel, Customer: parcel.Customer, SourceRef: parcel.SourceRef})
		}
		var roles []caseRoleRow
		if err := json.Unmarshal(rolesRaw, &roles); err != nil {
			return fail(fmt.Errorf("roles: %w", err))
		}
		for _, role := range roles {
			record.Roles = append(record.Roles, ports.CaseRoleRecord{
				Role: role.Role, Party: role.Party, Authority: role.Authority})
		}
		var units []caseUnitDocument
		if err := json.Unmarshal(unitsRaw, &units); err != nil {
			return fail(fmt.Errorf("units: %w", err))
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
				return fail(fmt.Errorf("closure: %w", err))
			}
			record.Closure = &ports.CaseClosureRecord{
				DecidedBy: closure.DecidedBy, CutoffAt: closure.CutoffAt.UTC(), VerifiedAt: closure.VerifiedAt.UTC(),
				ClosedAt: closure.ClosedAt.UTC(), ItemCount: closure.ItemCount, ReopenCount: closure.ReopenCount,
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, records, limit, page.total, customsCasePosition)
	if err != nil {
		return fail(err)
	}
	return result, nil
}

// customsCasePosition 取的是扫描出来的 case_id 与 established_at 两列。
func customsCasePosition(record ports.CustomsCaseRecord, sortField string) cataloguepage.Position {
	value := record.Case
	if sortField == "establishedAt" {
		value = cataloguepage.FormatInstant(record.EstablishedAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{record.Case}}
}

// caseNumberRecord 转述单据号两列：两列皆空是迁移之前建立的旧行，转述成「上线前开立」——查阅面不重建领域对象，
// 这一格的读法与案件库的 rebuildCustomsCase 是同一句话。
func caseNumberRecord(number, absence *string) (string, string) {
	switch {
	case number != nil:
		return *number, ""
	case absence != nil:
		return "", *absence
	default:
		return "", domain.IssuedBeforeNumbering.String()
	}
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

// submissionVersionPages 把 ports.SubmissionVersionPages 落到列上。isCurrent 落在布尔列的文本形上（'true' / 'false'，与
// 词表同词）；q 在版本号、单元、程序与成员包裹（数组的文本形，包裹号照原样出现在里面）上做字面包含。
var submissionVersionPages = ports.SubmissionVersionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"unit": "s.unit_id", "fixedAt": "s.fixed_at"},
	Identity: []string{"s.unit_id", "s.procedure_ref", "s.fixed_at", "s.version_id"},
	Filters:  map[string]string{"version": "s.version_id", "isCurrent": "s.is_current::text"},
	Keyword:  []string{"s.version_id", "s.unit_id", "s.procedure_ref", "s.members::text"},
})

func (views *OperationsRecordViews) ListSubmissionVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.SubmissionVersionRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.SubmissionVersionRecord], error) {
		return cataloguepage.Result[ports.SubmissionVersionRecord]{}, fmt.Errorf("list submission versions: %w", err)
	}
	const source = "customs_compliance.declaration_submission AS s"
	page, err := openPage(ctx, views.db, submissionVersionPages, query, limit, source, "s.tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`s.version_id, s.unit_id, s.procedure_ref, s.content_digest, s.members, s.dossier_ref, s.roles_ref,
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
		          WHERE a.tenant_id = s.tenant_id AND a.version_id = s.version_id)`,
		source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	records := make([]ports.SubmissionVersionRecord, 0, limit+1)
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
			return fail(err)
		}
		record.FixedAt = record.FixedAt.UTC()
		record.CorrectedFrom = orEmpty(correctedFrom)
		if err := json.Unmarshal(membersRaw, &record.Members); err != nil {
			return fail(fmt.Errorf("members: %w", err))
		}
		var attempts []attemptDocument
		if err := json.Unmarshal(attemptsRaw, &attempts); err != nil {
			return fail(fmt.Errorf("attempts: %w", err))
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
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, records, limit, page.total, submissionVersionPosition)
	if err != nil {
		return fail(err)
	}
	return result, nil
}

// submissionVersionPosition 取的是扫描出来的单元、程序、固定时刻与版本号四列。
func submissionVersionPosition(record ports.SubmissionVersionRecord, sortField string) cataloguepage.Position {
	fixedAt := cataloguepage.FormatInstant(record.FixedAt)
	value := record.Unit
	if sortField == "fixedAt" {
		value = fixedAt
	}
	return cataloguepage.Position{
		Value:    value,
		Identity: []string{record.Unit, record.Procedure, fixedAt, record.Version},
	}
}

// —— 外部监管结果与放行 ——

// externalResultPages 把 ports.ExternalResultPages 落到列上。layer 落在结果文档里的层（归属不上的行没有层，NULL 不等于任何
// 一词）；q 在来源标识、归属的提交版本与来源自报的版本上做字面包含。
var externalResultPages = ports.ExternalResultPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "e.recorded_at"},
	Identity: []string{"e.source_id"},
	Filters:  map[string]string{"layer": "(e.result ->> 'layer')"},
	Keyword:  []string{"e.source_id", "e.submission_version", "e.claimed_version"},
})

// ListExternalResults 的 version 是选择器的取值（ExternalResultPages）：空即不收窄，非空只答归属于该提交版本的。
func (views *OperationsRecordViews) ListExternalResults(
	ctx context.Context,
	tenant domain.TenantID,
	version string,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ExternalResultViewRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.ExternalResultViewRecord], error) {
		return cataloguepage.Result[ports.ExternalResultViewRecord]{}, fmt.Errorf("list external results: %w", err)
	}
	page, err := openPage(ctx, views.db, externalResultPages, query, limit,
		"customs_compliance.external_result AS e",
		"e.tenant_id = $1 AND ($2 = '' OR e.submission_version = $2)", tenant.String(), version)
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`e.source_id, COALESCE(e.result ->> 'layer', ''), COALESCE(e.result ->> 'role', ''),
		        COALESCE(NULLIF(e.raw_semantics, ''), e.result ->> 'rawSemantics', ''),
		        COALESCE(e.result ->> 'rule', ''), COALESCE(e.submission_version, ''), e.claimed_version,
		        COALESCE(e.result ->> 'scope', ''), e.unattributable, e.layer_conflict,
		        COALESCE(e.corrects_source_id, ''), e.correction_in_force,
		        (e.result ->> 'occurredAt')::timestamptz, (e.result ->> 'receivedAt')::timestamptz, e.recorded_at,
		        r.kind, r.authority_ref, r.scope_ref, r.condition`,
		`customs_compliance.external_result AS e
		   LEFT JOIN customs_compliance.release_outcome AS r
		     ON r.tenant_id = e.tenant_id AND r.source_id = e.source_id`)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	records := make([]ports.ExternalResultViewRecord, 0, limit+1)
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
			return fail(err)
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
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, records, limit, page.total, externalResultPosition)
	if err != nil {
		return fail(err)
	}
	return result, nil
}

// externalResultPosition 取的是扫描出来的 recorded_at 与 source_id 两列；本册只有记下时刻一个可排维。
func externalResultPosition(record ports.ExternalResultViewRecord, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(record.RecordedAt),
		Identity: []string{record.Source},
	}
}

// —— 动作准入判断册 ——

// admissionJudgementPages 把 ports.AdmissionJudgementPages 落到列上。q 在判断标识、对象与两个节点上做字面包含。
var admissionJudgementPages = ports.AdmissionJudgementPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"judgedAt": "judged_at"},
	Identity: []string{"judgement_id"},
	Filters:  map[string]string{"object": "object_ref", "answer": "answer"},
	Keyword:  []string{"judgement_id", "object_ref", "node_ref", "onward_node_ref"},
})

func (views *OperationsRecordViews) ListActionAdmissionJudgements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ActionAdmissionJudgementRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.ActionAdmissionJudgementRecord], error) {
		return cataloguepage.Result[ports.ActionAdmissionJudgementRecord]{},
			fmt.Errorf("list action admission judgements: %w", err)
	}
	const source = "customs_compliance.action_admission_judgement"
	page, err := openPage(ctx, views.db, admissionJudgementPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`judgement_id, object_ref, intent_kind, node_ref, onward_node_ref, action_at,
		        cross_zone, answer, zones, findings, judged_at`,
		source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	records := make([]ports.ActionAdmissionJudgementRecord, 0, limit+1)
	for rows.Next() {
		var (
			record                ports.ActionAdmissionJudgementRecord
			onward                *string
			zonesRaw, findingsRaw []byte
		)
		if err := rows.Scan(&record.Judgement, &record.Object, &record.Intent, &record.Node, &onward,
			&record.ActionAt, &record.CrossZone, &record.Answer, &zonesRaw, &findingsRaw, &record.JudgedAt); err != nil {
			return fail(err)
		}
		if err := completeAdmissionJudgement(&record, onward, zonesRaw, findingsRaw); err != nil {
			return fail(err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, records, limit, page.total, admissionJudgementPosition)
	if err != nil {
		return fail(err)
	}
	return result, nil
}

// admissionJudgementPosition 取的是扫描出来的 judged_at 与 judgement_id 两列；本册只有判断时刻一个可排维。
func admissionJudgementPosition(record ports.ActionAdmissionJudgementRecord, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(record.JudgedAt),
		Identity: []string{record.Judgement},
	}
}

// completeAdmissionJudgement 把一行判断的可空列与两份 jsonb 补进记录；判断册与卡关对象两口共用，两边的同一份判断
// 读出来必须一样。
func completeAdmissionJudgement(
	record *ports.ActionAdmissionJudgementRecord,
	onward *string,
	zonesRaw, findingsRaw []byte,
) error {
	record.OnwardNode = orEmpty(onward)
	record.ActionAt = record.ActionAt.UTC()
	record.JudgedAt = record.JudgedAt.UTC()
	var zones []admissionZoneRow
	if err := json.Unmarshal(zonesRaw, &zones); err != nil {
		return fmt.Errorf("zones: %w", err)
	}
	for _, zone := range zones {
		record.Zones = append(record.Zones, ports.AdmissionZoneRecord{
			Node: zone.Node, Zone: zone.Zone, Registered: zone.Registered})
	}
	var findings []admissionFindingRow
	if err := json.Unmarshal(findingsRaw, &findings); err != nil {
		return fmt.Errorf("findings: %w", err)
	}
	for _, finding := range findings {
		entry := ports.AdmissionFindingRecord{Action: finding.Action, Answer: finding.Answer}
		for _, basis := range finding.Bases {
			entry.Bases = append(entry.Bases, ports.AdmissionBasisRecord{
				Cause: basis.Cause, Scope: basis.Scope, Procedure: basis.Procedure, Sources: basis.Sources})
		}
		record.Findings = append(record.Findings, entry)
	}
	return nil
}

// —— 卡关对象 ——

// unadmittedAnswers 是不让动作过门的那几格的库上词形：从领域给的全集里按 domain.AdmissionAnswer.Admits 挑，不在 SQL 里另抄一份，
// 也不依赖常量的次序。
var unadmittedAnswers = func() []string {
	var words []string
	for _, answer := range domain.AdmissionAnswers() {
		if !answer.Admits() {
			words = append(words, answer.String())
		}
	}
	return words
}()

// answerSeverity 是封闭五格的库上词形，按 domain.AdmissionAnswers 的次序（压制次序，轻在前、阻断在末）：同一刻几份都不过门时，
// 交回位次最大的那一份。
var answerSeverity = func() []string {
	var words []string
	for _, answer := range domain.AdmissionAnswers() {
		words = append(words, answer.String())
	}
	return words
}()

// unadmittedObjectPages 把 ports.UnadmittedObjectPages 落到 stuck 导出的列上。q 在对象、判断标识与两个节点上做字面包含。
var unadmittedObjectPages = ports.UnadmittedObjectPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"actionAt": "action_at"},
	Identity: []string{"object_ref"},
	Filters:  map[string]string{"answer": "answer"},
	Keyword:  []string{"object_ref", "judgement_id", "node_ref", "onward_node_ref"},
})

// unadmittedStuck 是卡关对象的前半句，导出 stuck：每个对象一行。$1 租户、$2 不过门的词、$3 压制次序，筛选维与 q 的参数接在后面。
const unadmittedStuck = `WITH latest_at AS (
		     SELECT object_ref, max(action_at) AS action_at
		       FROM customs_compliance.action_admission_judgement
		      WHERE tenant_id = $1
		      GROUP BY object_ref
		 ),
		 current AS (
		     SELECT DISTINCT ON (j.object_ref, j.intent_kind, j.node_ref, j.onward_node_ref)
		            j.judgement_id, j.object_ref, j.intent_kind, j.node_ref, j.onward_node_ref, j.action_at,
		            j.cross_zone, j.answer, j.zones, j.findings, j.judged_at
		       FROM customs_compliance.action_admission_judgement AS j
		       JOIN latest_at AS l ON l.object_ref = j.object_ref AND l.action_at = j.action_at
		      WHERE j.tenant_id = $1
		      ORDER BY j.object_ref, j.intent_kind, j.node_ref, j.onward_node_ref, j.judged_at DESC, j.judgement_id DESC
		 ),
		 stuck AS (
		     SELECT DISTINCT ON (object_ref)
		            judgement_id, object_ref, intent_kind, node_ref, onward_node_ref, action_at,
		            cross_zone, answer, zones, findings, judged_at
		       FROM current
		      WHERE answer = ANY($2)
		      ORDER BY object_ref, array_position($3::text[], answer) DESC, judged_at DESC, judgement_id DESC
		 )
		 `

// ListUnadmittedObjects 每个对象看它动作时点最晚的那一刻。那一刻上每一次请求（意图、节点、去向，与动作时点一起就是
// ports.ActionAdmissionJudgementID 派生标识时用的请求那几格）只取判断时刻最晚的一份——同一请求三源变了重判另记一行（ADR-0206 决定
// 六），出发前问询之后同参出发也是同一请求（ADR-0219），旧的那份已被新的取代；再看这些当前答复里有没有不过门的，有就算卡关，只计一次。
// 同一刻不同请求各算各的：交出被挡、出发放行，那一刻就有一件事没做成，不让另一请求的放行把它盖掉。
// 「最近」按动作时点而不按判断时刻：执行方晚报的一次早先动作不该盖过它之后那次动作的答复。
// 每个对象交回不过门的当前答复里最重的一份（压制次序，见 answerSeverity），同样重时交判断时刻最晚的。
//
// 两个件数——本次条件下的件数与整个队列的积压——作取页那一句的两列标量子查询，与这一页同一条语句求：分两条语句求，两次读之间
// 进来的判断会让件数与这一页对不上。这一页一行都没有时没有行可带件数，才另问一句只数件数的，那时也没有行可与件数对不上。
func (views *OperationsRecordViews) ListUnadmittedObjects(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.UnadmittedObjects, error) {
	fail := func(err error) (ports.UnadmittedObjects, error) {
		return ports.UnadmittedObjects{}, fmt.Errorf("list unadmitted objects: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := unadmittedObjectPages.Plan(query, "", []any{tenant.String(), unadmittedAnswers, answerSeverity}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	counts := "(" + plan.CountSQL("stuck AS counted") + "), (SELECT count(*) FROM stuck)"
	rows, err := querier.Query(ctx, unadmittedStuck+plan.PageSQL(
		`judgement_id, object_ref, intent_kind, node_ref, onward_node_ref, action_at,
		        cross_zone, answer, zones, findings, judged_at, `+counts,
		"stuck"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var total, backlog int64
	records := make([]ports.ActionAdmissionJudgementRecord, 0, limit+1)
	for rows.Next() {
		var (
			record                ports.ActionAdmissionJudgementRecord
			onward                *string
			zonesRaw, findingsRaw []byte
		)
		if err := rows.Scan(&record.Judgement, &record.Object, &record.Intent, &record.Node, &onward,
			&record.ActionAt, &record.CrossZone, &record.Answer, &zonesRaw, &findingsRaw, &record.JudgedAt,
			&total, &backlog); err != nil {
			return fail(err)
		}
		if err := completeAdmissionJudgement(&record, onward, zonesRaw, findingsRaw); err != nil {
			return fail(err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	rows.Close()
	if len(records) == 0 {
		if err := querier.QueryRow(ctx, unadmittedStuck+"SELECT "+counts, plan.Args...).Scan(&total, &backlog); err != nil {
			return fail(err)
		}
	}
	result, err := cataloguepage.Finish(query, records, limit, total, unadmittedObjectPosition)
	if err != nil {
		return fail(err)
	}
	return ports.UnadmittedObjects{Result: result, Count: int(backlog)}, nil
}

// unadmittedObjectPosition 取的是 stuck 导出的 action_at 与 object_ref 两列；本册只有动作时点一个可排维。
func unadmittedObjectPosition(record ports.ActionAdmissionJudgementRecord, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(record.ActionAt),
		Identity: []string{record.Object},
	}
}
