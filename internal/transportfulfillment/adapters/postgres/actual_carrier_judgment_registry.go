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

// ActualCarrierJudgments 实现 ports.ActualCarrierJudgmentRegistry：头行一张表、版本一张表、依据一张表，
// **只插不改**（0013）。
//
// 三张表同笔落。半份判断（有头无首版、有版无依据而版本不是无合格证据）是领域产不出的东西——重建门直接拒
// ——所以写口要求环境事务，宁可拒绝写入也不留下一份读不回来的判断。
type ActualCarrierJudgments struct {
	db *bentopg.DB
}

func NewActualCarrierJudgments(db *bentopg.DB) (*ActualCarrierJudgments, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ActualCarrierJudgments{db: db}, nil
}

var _ ports.ActualCarrierJudgmentRegistry = (*ActualCarrierJudgments)(nil)

// FindByKey 按（租户+段）取回整份判断历史。否定结果只回 false。
//
// 读回过重建门：序号连续、判断值恰居其一、业务时间不早于段成立、依据自身立得住——坏行在这里暴露，
// 而不是流到判断里。版本与依据各一次查询、按序号归组，不逐版本往返。
func (repository *ActualCarrierJudgments) FindByKey(
	ctx context.Context,
	key ports.ActualCarrierJudgmentKey,
) (ports.ActualCarrierJudgmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
	}

	var establishedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT segment_established_at, recorded_at
		   FROM transport_fulfillment.actual_carrier_judgment
		  WHERE tenant_id = $1 AND segment_ref = $2`,
		key.TenantID.String(), key.Segment.String(),
	).Scan(&establishedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ActualCarrierJudgmentRecord{}, false, nil
	}
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
	}

	basesBySequence, err := repository.loadBases(ctx, querier, key)
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT sequence_no, subject_kind, subject_ref, pending_reason, business_time, formed_at
		   FROM transport_fulfillment.actual_carrier_judgment_version
		  WHERE tenant_id = $1 AND segment_ref = $2
		  ORDER BY sequence_no`,
		key.TenantID.String(), key.Segment.String(),
	)
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
	}
	defer rows.Close()

	spec := domain.RehydrateActualCarrierJudgmentSpec{
		TenantID:      key.TenantID,
		Segment:       key.Segment,
		EstablishedAt: establishedAt.UTC(),
	}
	for rows.Next() {
		var sequence int
		var subjectKind, subjectRef, pendingReason *string
		var businessTime, formedAt time.Time
		if err := rows.Scan(&sequence, &subjectKind, &subjectRef, &pendingReason, &businessTime, &formedAt); err != nil {
			return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
		}
		version, err := judgmentVersionSpecFrom(sequence, subjectKind, subjectRef, pendingReason, businessTime, formedAt, basesBySequence[sequence])
		if err != nil {
			return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
		}
		spec.Versions = append(spec.Versions, version)
	}
	if err := rows.Err(); err != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
	}

	judgment, err := domain.RehydrateActualCarrierJudgment(spec)
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, fmt.Errorf("find actual carrier judgment: %w", err)
	}
	return ports.ActualCarrierJudgmentRecord{Key: key, Judgment: judgment, RecordedAt: recordedAt.UTC()}, true, nil
}

// loadBases 一次取回全部版本的依据，按序号归组。组内按来源事实引用排序只为读回稳定，不进任何判断。
func (repository *ActualCarrierJudgments) loadBases(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.ActualCarrierJudgmentKey,
) (map[int][]domain.CarrierEvidence, error) {
	rows, err := querier.Query(ctx,
		`SELECT sequence_no, evidence_ref, evidence_source, occurred_at, subject_kind, subject_ref, name_material
		   FROM transport_fulfillment.actual_carrier_judgment_basis
		  WHERE tenant_id = $1 AND segment_ref = $2
		  ORDER BY sequence_no, occurred_at, evidence_ref`,
		key.TenantID.String(), key.Segment.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bases := map[int][]domain.CarrierEvidence{}
	for rows.Next() {
		var sequence int
		var evidenceRef, evidenceSource string
		var occurredAt time.Time
		var subjectKind, subjectRef, material *string
		if err := rows.Scan(&sequence, &evidenceRef, &evidenceSource, &occurredAt, &subjectKind, &subjectRef, &material); err != nil {
			return nil, err
		}
		evidence, err := carrierEvidenceFrom(evidenceRef, evidenceSource, occurredAt, subjectKind, subjectRef, material)
		if err != nil {
			return nil, err
		}
		bases[sequence] = append(bases[sequence], evidence)
	}
	return bases, rows.Err()
}

// Open 首登一份判断：头行连同它此刻的全部版本（段成立时只有首版）同笔落。撞键只看头行——版本挂在它下面，
// 头行已在册就说明这一份已经有人开过；答`已开`（ADR-0031），编排据此读回赢家。
func (repository *ActualCarrierJudgments) Open(
	ctx context.Context,
	record ports.ActualCarrierJudgmentRecord,
) (ports.JudgmentOpenOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.JudgmentOpenOutcomeInvalid, fmt.Errorf("open actual carrier judgment: %w", err)
	}
	if err := assertJudgmentKeyAgrees(record); err != nil {
		return ports.JudgmentOpenOutcomeInvalid, fmt.Errorf("open actual carrier judgment: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.actual_carrier_judgment
		     (tenant_id, segment_ref, segment_established_at, recorded_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Segment.String(),
		record.Judgment.SegmentEstablishedAt().UTC(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.JudgmentOpenOutcomeInvalid, fmt.Errorf("open actual carrier judgment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.JudgmentAlreadyOpened, nil
	}
	for _, version := range record.Judgment.Versions() {
		inserted, err := insertJudgmentVersion(ctx, executor, record.Key, version, record.RecordedAt)
		if err != nil {
			return ports.JudgmentOpenOutcomeInvalid, fmt.Errorf("open actual carrier judgment: %w", err)
		}
		// 头行刚由本笔插入，版本表里不可能已有这一份的行；撞上就是记录自己重复了序号。
		if inserted == 0 {
			return ports.JudgmentOpenOutcomeInvalid, fmt.Errorf("open actual carrier judgment: version %d duplicated within the record", version.Sequence())
		}
	}
	return ports.JudgmentOpened, nil
}

// AppendVersion 只插指名的那一版连同它的依据。撞序号答`版本已在册`：两条编排各基于同一个当前版算出了
// 同一个下一序号，后写的读回再来——不是错误，也不是重放。
//
// 不校验序号是否恰为「当前最大加一」：那是领域在 appendVersion 里定的；本口若替它判，就成了第二个口径。
// 主键挡住的是「同一序号两份内容」，那正是追加式版本表唯一要在库面守住的东西。
func (repository *ActualCarrierJudgments) AppendVersion(
	ctx context.Context,
	key ports.ActualCarrierJudgmentKey,
	version domain.ActualCarrierJudgmentVersion,
	recordedAt time.Time,
) (ports.JudgmentVersionAppendOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.JudgmentVersionAppendOutcomeInvalid, fmt.Errorf("append actual carrier judgment version: %w", err)
	}
	inserted, err := insertJudgmentVersion(ctx, executor, key, version, recordedAt)
	if err != nil {
		return ports.JudgmentVersionAppendOutcomeInvalid, fmt.Errorf("append actual carrier judgment version: %w", err)
	}
	if inserted == 0 {
		return ports.JudgmentVersionAlreadyRecorded, nil
	}
	return ports.JudgmentVersionAppended, nil
}

// insertJudgmentVersion 落一版连同它的依据。版本行 ON CONFLICT DO NOTHING、依据行不容忍冲突：版本行撞键是
// 业务答案（并发追加），依据行在版本刚插入之后撞键只可能是记录自己重复了引用——那由重建门与 CHECK 各拦一道。
//
// Open 与 AppendVersion 共用同一段 INSERT 而不是各写一遍，理由同 insertParticipationRow：列一多，两份就会在
// 下一次加列时分叉，而分叉处正是「首登写得对、追加写漏一列」这种不可能靠测试穷尽的错。
func insertJudgmentVersion(
	ctx context.Context,
	executor bentopg.Executor,
	key ports.ActualCarrierJudgmentKey,
	version domain.ActualCarrierJudgmentVersion,
	recordedAt time.Time,
) (int64, error) {
	var subjectKind, subjectRef, pendingReason *string
	if subject, identified := version.Verdict().Identified(); identified {
		kind, reference := subject.Kind().String(), subject.Reference()
		subjectKind, subjectRef = &kind, &reference
	}
	if reason, pending := version.Verdict().Pending(); pending {
		name := reason.String()
		pendingReason = &name
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.actual_carrier_judgment_version
		     (tenant_id, segment_ref, sequence_no, subject_kind, subject_ref, pending_reason,
		      business_time, formed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		key.Segment.String(),
		version.Sequence(),
		subjectKind,
		subjectRef,
		pendingReason,
		version.BusinessTime().UTC(),
		version.FormedAt().UTC(),
		recordedAt.UTC(),
	)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, nil
	}

	for _, basis := range version.Bases() {
		var basisSubjectKind, basisSubjectRef, material *string
		if subject, registered := basis.Subject(); registered {
			kind, reference := subject.Kind().String(), subject.Reference()
			basisSubjectKind, basisSubjectRef = &kind, &reference
		} else {
			text := basis.Material()
			material = &text
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.actual_carrier_judgment_basis
			     (tenant_id, segment_ref, sequence_no, evidence_ref, evidence_source, occurred_at,
			      subject_kind, subject_ref, name_material)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			key.TenantID.String(),
			key.Segment.String(),
			version.Sequence(),
			basis.Reference().String(),
			basis.Source().String(),
			basis.OccurredAt().UTC(),
			basisSubjectKind,
			basisSubjectRef,
			material,
		); err != nil {
			return 0, err
		}
	}
	return tag.RowsAffected(), nil
}

// judgmentVersionSpecFrom 逐列走各自的构造门装回一版，不按列直接拼结构体——构造门是坏行的第一道拦截，
// 绕过它等于把库当成可信来源（ADR-0028）。
func judgmentVersionSpecFrom(
	sequence int,
	subjectKind, subjectRef, pendingReason *string,
	businessTime, formedAt time.Time,
	bases []domain.CarrierEvidence,
) (domain.RehydrateActualCarrierJudgmentVersionSpec, error) {
	spec := domain.RehydrateActualCarrierJudgmentVersionSpec{
		Sequence:     sequence,
		BusinessTime: businessTime.UTC(),
		FormedAt:     formedAt.UTC(),
		Bases:        bases,
	}
	if subjectKind != nil && subjectRef != nil {
		subject, err := carrierSubjectFrom(*subjectKind, *subjectRef)
		if err != nil {
			return domain.RehydrateActualCarrierJudgmentVersionSpec{}, err
		}
		spec.Subject = subject
	}
	if pendingReason != nil {
		reason, err := domain.ParsePendingCarrierReason(*pendingReason)
		if err != nil {
			return domain.RehydrateActualCarrierJudgmentVersionSpec{}, err
		}
		spec.Pending = reason
	}
	return spec, nil
}

func carrierEvidenceFrom(
	evidenceRef, evidenceSource string,
	occurredAt time.Time,
	subjectKind, subjectRef, material *string,
) (domain.CarrierEvidence, error) {
	source, err := domain.ParseCarrierEvidenceSource(evidenceSource)
	if err != nil {
		return domain.CarrierEvidence{}, err
	}
	reference, err := domain.NewCarrierEvidenceReference(evidenceRef)
	if err != nil {
		return domain.CarrierEvidence{}, err
	}
	spec := domain.CarrierEvidenceSpec{Source: source, Reference: reference, OccurredAt: occurredAt.UTC()}
	if subjectKind != nil && subjectRef != nil {
		if spec.Subject, err = carrierSubjectFrom(*subjectKind, *subjectRef); err != nil {
			return domain.CarrierEvidence{}, err
		}
	}
	if material != nil {
		spec.Material = *material
	}
	return domain.NewCarrierEvidence(spec)
}

func carrierSubjectFrom(kind, reference string) (domain.CarrierSubject, error) {
	parsed, err := domain.ParseCarrierSubjectKind(kind)
	if err != nil {
		return domain.CarrierSubject{}, err
	}
	return domain.NewCarrierSubject(parsed, reference)
}

// assertJudgmentKeyAgrees 挡住「键与本体说的不是同一个段」——那种记录一旦落库，按键取回的东西与它自称的
// 身份对不上，而两边都看不出错。
func assertJudgmentKeyAgrees(record ports.ActualCarrierJudgmentRecord) error {
	if record.Key.TenantID != record.Judgment.TenantID() || record.Key.Segment != record.Judgment.Segment() {
		return errors.New("record key disagrees with the judgment it carries")
	}
	if len(record.Judgment.Versions()) == 0 {
		return errors.New("a judgment without versions was never opened")
	}
	return nil
}
