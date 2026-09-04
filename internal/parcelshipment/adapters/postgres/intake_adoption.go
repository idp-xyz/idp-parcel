package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// IntakeAdoptions 实现 ports.IntakeAdoptionStore。幂等三维加租户就是主键；采用行成一条链
// （迁移 0017，ADR-0117 决定二）：根是先合法形成的责任起点，同来源更正逐版回指前版，
// 只插不改。「同一包裹只有一个责任起点」（AT-PS-049）由部分唯一索引「每租户+包裹至多
// 一行 adopted 且无回指」承担，「同一版本至多被取代一次」由另一条部分唯一索引承担——
// 撞任一条都由 ON CONFLICT DO NOTHING 译`已有记录`（ADR-0031），重试方经
// FindResponsibilityStart 读到链尾后收敛。
type IntakeAdoptions struct {
	db *bentopg.DB
}

func NewIntakeAdoptions(db *bentopg.DB) (*IntakeAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &IntakeAdoptions{db: db}, nil
}

const findAdoptionSQL = `SELECT adoption.parcel_id, adoption.source_kind, adoption.source_version,
       adoption.customer_account_id, adoption.shipment_request_id, adoption.content_digest, adoption.adopted,
       adoption.source_object, adoption.source_place, adoption.source_control, adoption.occurred_at,
       adoption.baseline_version, adoption.commitment_version, adoption.expected_commitment,
       adoption.refusal_basis, adoption.adopted_at,
       adoption.supersedes_source_version, adoption.commitment_prior_version, adoption.commitment_adjustment_reason
  FROM parcel_shipment.intake_adoption AS adoption`

// FindByKey 按幂等键取回已提交采用结果。否定结果只回 false，不区分「不存在」与
// 「属于另一个租户」（AT-PS-052 的统一不可见）。
func (repository *IntakeAdoptions) FindByKey(
	ctx context.Context,
	key ports.IntakeAdoptionKey,
) (ports.IntakeAdoptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("find intake adoption: %w", err)
	}
	row := querier.QueryRow(ctx,
		findAdoptionSQL+`
		 WHERE adoption.tenant_id = $1
		   AND adoption.parcel_id = $2
		   AND adoption.source_kind = $3
		   AND adoption.source_version = $4`,
		key.TenantID.String(),
		key.Parcel.String(),
		key.Kind.String(),
		key.Version.String(),
	)
	return scanAdoption(key.TenantID, row)
}

// FindResponsibilityStart 按包裹找回责任起点当前所在的那一版：采用链的链尾——没有任何行
// 回指它的那一行 adopted。根唯一与链线性两条部分唯一索引让这条子查询恰答一行；没被更正
// 过时链尾就是根，与 0004 时期「那一行 adopted」的读法同义。「先到」由写入路径的撞索引
// 裁决过了，这里不排序。
func (repository *IntakeAdoptions) FindResponsibilityStart(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (ports.IntakeAdoptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("find responsibility start: %w", err)
	}
	row := querier.QueryRow(ctx,
		findAdoptionSQL+`
		 WHERE adoption.tenant_id = $1
		   AND adoption.parcel_id = $2
		   AND adoption.adopted
		   AND NOT EXISTS (
		       SELECT 1
		         FROM parcel_shipment.intake_adoption AS successor
		        WHERE successor.tenant_id = adoption.tenant_id
		          AND successor.parcel_id = adoption.parcel_id
		          AND successor.source_kind = adoption.source_kind
		          AND successor.adopted
		          AND successor.supersedes_source_version = adoption.source_version)`,
		tenant.String(),
		parcel.String(),
	)
	return scanAdoption(tenant, row)
}

// Save 写下一次采用结果。撞主键（重复到达）、撞责任起点索引（并发第二个根）与撞链线性
// 索引（并发第二个更正）都答`已有记录`——前者按原键读回原结果，后两者读不回本键、由
// 重试方经 FindResponsibilityStart 读到链尾后收敛。
func (repository *IntakeAdoptions) Save(
	ctx context.Context,
	record ports.IntakeAdoptionRecord,
) (ports.IntakeAdoptionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.IntakeAdoptionSaveOutcomeInvalid, fmt.Errorf("save intake adoption: %w", err)
	}

	columns, err := adoptionColumns(record)
	if err != nil {
		return ports.IntakeAdoptionSaveOutcomeInvalid, fmt.Errorf("save intake adoption: %w", err)
	}

	key := record.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.intake_adoption
			(tenant_id, parcel_id, source_kind, source_version,
			 customer_account_id, shipment_request_id, content_digest, adopted,
			 source_object, source_place, source_control, occurred_at,
			 baseline_version, commitment_version, expected_commitment,
			 refusal_basis, adopted_at,
			 supersedes_source_version, commitment_prior_version, commitment_adjustment_reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		key.Parcel.String(),
		key.Kind.String(),
		key.Version.String(),
		record.CustomerAccountID.String(),
		record.ShipmentRequestID.String(),
		record.ContentDigest,
		record.Adopted,
		columns.sourceObject,
		columns.sourcePlace,
		columns.sourceControl,
		columns.occurredAt,
		columns.baseline,
		columns.commitmentVersion,
		columns.expectedCommitment,
		columns.refusalBasis,
		record.AdoptedAt.UTC(),
		columns.supersedesVersion,
		columns.commitmentPriorVersion,
		columns.commitmentAdjustmentReason,
	)
	if err != nil {
		return ports.IntakeAdoptionSaveOutcomeInvalid, fmt.Errorf("save intake adoption: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.IntakeAdoptionAlreadyRecorded, nil
	}
	return ports.IntakeAdoptionSaved, nil
}

type adoptionRowColumns struct {
	sourceObject               *string
	sourcePlace                *string
	sourceControl              *string
	occurredAt                 *time.Time
	baseline                   *string
	commitmentVersion          *string
	expectedCommitment         *string
	refusalBasis               *string
	supersedesVersion          *string
	commitmentPriorVersion     *string
	commitmentAdjustmentReason *string
}

// adoptionColumns 把记录折成行。采用面要求收寄与承诺同键同源；承诺的形状随链上位置走：
// 根采用承载首版承诺，更正形成的采用（带 SupersedesVersion）承载在前版上重述的承诺——
// 带前版与原因，且来源自报的被更正版本要与记录回指的版本一致。两面混搭就是拼坏的聚合。
func adoptionColumns(record ports.IntakeAdoptionRecord) (adoptionRowColumns, error) {
	if !record.Adopted {
		if record.RefusalBasis.String() == "" {
			return adoptionRowColumns{}, errors.New("a refusal carries its basis")
		}
		if _, chained := record.Supersedes(); chained {
			return adoptionRowColumns{}, errors.New("a refusal does not take a place on the adoption chain")
		}
		basis := record.RefusalBasis.String()
		return adoptionRowColumns{refusalBasis: &basis}, nil
	}

	source := record.Intake.Source()
	if source.Parcel() != record.Key.Parcel ||
		source.Kind() != record.Key.Kind ||
		source.Version() != record.Key.Version {
		return adoptionRowColumns{}, errors.New("record key disagrees with the intake's source")
	}
	if record.Commitment.Parcel() != record.Key.Parcel {
		return adoptionRowColumns{}, errors.New("record key disagrees with the commitment's parcel")
	}

	columns := adoptionRowColumns{}
	priorCommitment, restated := record.Commitment.PriorVersion()
	supersedes, chained := record.Supersedes()
	switch {
	case chained && restated:
		if corrects, declared := source.Corrects(); !declared || corrects != supersedes {
			return adoptionRowColumns{}, errors.New("the superseded version disagrees with what the source declares it corrects")
		}
		reason, reasoned := record.Commitment.AdjustmentReason()
		if !reasoned {
			return adoptionRowColumns{}, errors.New("a restated commitment carries its adjustment reason")
		}
		supersedesValue, priorValue, reasonValue := supersedes.String(), priorCommitment.String(), reason.String()
		columns.supersedesVersion = &supersedesValue
		columns.commitmentPriorVersion = &priorValue
		columns.commitmentAdjustmentReason = &reasonValue
	case chained:
		return adoptionRowColumns{}, errors.New("a superseding adoption carries a commitment restated on the prior version")
	case restated:
		return adoptionRowColumns{}, errors.New("a root adoption carries the first commitment version only")
	}

	object := source.Object().String()
	place := source.Place().String()
	control := source.Control().String()
	occurredAt := source.OccurredAt()
	baseline := record.Intake.Baseline().String()
	commitmentVersion := record.Commitment.Version().String()
	expected := record.Commitment.Expected().String()
	columns.sourceObject = &object
	columns.sourcePlace = &place
	columns.sourceControl = &control
	columns.occurredAt = &occurredAt
	columns.baseline = &baseline
	columns.commitmentVersion = &commitmentVersion
	columns.expectedCommitment = &expected
	return columns, nil
}

// scanAdoption 读一行并经领域构造门重建（NewIntakeSource / AdoptNetworkIntake /
// FormFormalCommitment）：来源五件完备与承诺引用在读回处复验。
func scanAdoption(
	tenant domain.TenantID,
	row pgx.Row,
) (ports.IntakeAdoptionRecord, bool, error) {
	var (
		parcel, kindRaw, version                  string
		customerAccount, shipmentRequest          string
		digest                                    string
		adopted                                   bool
		object, place, control                    *string
		occurredAt                                *time.Time
		baseline, commitVersion, expected         *string
		refusalBasis                              *string
		adoptedAt                                 time.Time
		supersedes, priorCommitment, adjustReason *string
	)
	err := row.Scan(&parcel, &kindRaw, &version,
		&customerAccount, &shipmentRequest, &digest, &adopted,
		&object, &place, &control, &occurredAt,
		&baseline, &commitVersion, &expected,
		&refusalBasis, &adoptedAt,
		&supersedes, &priorCommitment, &adjustReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.IntakeAdoptionRecord{}, false, nil
	}
	if err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}

	kind, err := intakeSourceKindFrom(kindRaw)
	if err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}
	record := ports.IntakeAdoptionRecord{
		ContentDigest: digest,
		Adopted:       adopted,
		AdoptedAt:     adoptedAt.UTC(),
	}
	record.Key.TenantID = tenant
	if record.Key.Parcel, err = domain.NewDeclaredParcelID(parcel); err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}
	record.Key.Kind = kind
	if record.Key.Version, err = domain.NewSourceResultVersion(version); err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}
	if record.CustomerAccountID, err = domain.NewCustomerAccountID(customerAccount); err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}
	if record.ShipmentRequestID, err = domain.NewShipmentRequestID(shipmentRequest); err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}

	if !adopted {
		if refusalBasis == nil {
			return ports.IntakeAdoptionRecord{}, false, errors.New(
				"read intake adoption: a refusal row arrived without its basis")
		}
		if record.RefusalBasis, err = domain.NewCheckReason(*refusalBasis); err != nil {
			return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
		}
		return record, true, nil
	}

	if object == nil || place == nil || control == nil || occurredAt == nil ||
		baseline == nil || commitVersion == nil || expected == nil {
		return ports.IntakeAdoptionRecord{}, false, errors.New(
			"read intake adoption: an adopted row arrived with missing intake columns")
	}
	// 三列同在同缺由库内 CHECK 守；这里只把「在」译成链上位置，缺一半仍如实报坏行。
	if (supersedes == nil) != (priorCommitment == nil) || (supersedes == nil) != (adjustReason == nil) {
		return ports.IntakeAdoptionRecord{}, false, errors.New(
			"read intake adoption: a row arrived with half of its supersession columns")
	}
	columns := adoptedColumns{
		object:     *object,
		place:      *place,
		control:    *control,
		occurredAt: *occurredAt,
		baseline:   *baseline,
		version:    *commitVersion,
		expected:   *expected,
	}
	if supersedes != nil {
		columns.supersedes = *supersedes
		columns.priorCommitment = *priorCommitment
		columns.adjustReason = *adjustReason
	}
	intake, commitment, err := rebuildAdoption(record.Key, columns)
	if err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}
	record.Intake = intake
	record.Commitment = commitment
	if supersedes != nil {
		if record.SupersedesVersion, err = domain.NewSourceResultVersion(*supersedes); err != nil {
			return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
		}
	}
	return record, true, nil
}

type adoptedColumns struct {
	object     string
	place      string
	control    string
	occurredAt time.Time
	baseline   string
	version    string
	expected   string
	// 三样只在更正形成的采用行上有值（同在同缺）：被取代的来源版本、前版承诺号、调整原因。
	supersedes      string
	priorCommitment string
	adjustReason    string
}

// rebuildAdoption 经领域构造门重建收寄与承诺。更正形成的行走两步：先以「被取代版本」为前版
// 承诺的收寄底重建一份前版承诺（它只为过 RestateOnCorrectedIntake 的门——同包裹、同基线、
// 同种类——收寄本体取本行的，前版真正的收寄本体在它自己那一行上），再在其上重述出本行的
// 承诺；于是读回的承诺与写入时一样带前版、原因与更正后的生效时间。
func rebuildAdoption(
	key ports.IntakeAdoptionKey,
	columns adoptedColumns,
) (domain.EffectiveNetworkIntake, domain.FormalCommitment, error) {
	object, err := domain.NewSourceObjectReference(columns.object)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	place, err := domain.NewIntakePlaceReference(columns.place)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	control, err := domain.NewIntakeControlReference(columns.control)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	spec := domain.IntakeSourceSpec{
		Kind:       key.Kind,
		Object:     object,
		Parcel:     key.Parcel,
		Place:      place,
		Control:    control,
		Version:    key.Version,
		OccurredAt: columns.occurredAt,
	}
	if columns.supersedes != "" {
		if spec.Corrects, err = domain.NewSourceResultVersion(columns.supersedes); err != nil {
			return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
		}
	}
	source, err := domain.NewIntakeSource(spec)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	baseline, err := domain.NewSubmissionVersionID(columns.baseline)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	intake, err := domain.AdoptNetworkIntake(source, baseline)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	version, err := domain.NewCommitmentVersionID(columns.version)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	expected, err := domain.NewExpectedCommitmentReference(columns.expected)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	if columns.supersedes == "" {
		commitment, err := domain.FormFormalCommitment(version, intake, expected)
		if err != nil {
			return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
		}
		return intake, commitment, nil
	}

	priorVersion, err := domain.NewCommitmentVersionID(columns.priorCommitment)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	reason, err := domain.NewCommitmentAdjustmentReason(columns.adjustReason)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	priorCommitment, err := domain.FormFormalCommitment(priorVersion, intake, expected)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	commitment, err := priorCommitment.RestateOnCorrectedIntake(version, intake, reason)
	if err != nil {
		return domain.EffectiveNetworkIntake{}, domain.FormalCommitment{}, err
	}
	return intake, commitment, nil
}

func intakeSourceKindFrom(raw string) (domain.IntakeSourceKind, error) {
	switch raw {
	case domain.NodeIntakeSource.String():
		return domain.NodeIntakeSource, nil
	case domain.OffsitePickupSource.String():
		return domain.OffsitePickupSource, nil
	default:
		return domain.IntakeSourceKindInvalid, fmt.Errorf("unknown intake source kind %q", raw)
	}
}
