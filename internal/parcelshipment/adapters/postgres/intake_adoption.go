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

// IntakeAdoptions 实现 ports.IntakeAdoptionStore。幂等三维加租户就是主键；「同一包裹
// 只有一个责任起点」（AT-PS-049）由部分唯一索引（每租户+包裹至多一行 adopted）承担
// ——并发第二个采用撞索引，ON CONFLICT DO NOTHING 译`已有记录`（ADR-0031），重试
// 方经 FindResponsibilityStart 读到先到者后落不采用。
type IntakeAdoptions struct {
	db *bentopg.DB
}

func NewIntakeAdoptions(db *bentopg.DB) (*IntakeAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &IntakeAdoptions{db: db}, nil
}

const findAdoptionSQL = `SELECT parcel_id, source_kind, source_version,
       customer_account_id, shipment_request_id, content_digest, adopted,
       source_object, source_place, source_control, occurred_at,
       baseline_version, commitment_version, expected_commitment,
       refusal_basis, adopted_at
  FROM parcel_shipment.intake_adoption`

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
		 WHERE tenant_id = $1
		   AND parcel_id = $2
		   AND source_kind = $3
		   AND source_version = $4`,
		key.TenantID.String(),
		key.Parcel.String(),
		key.Kind.String(),
		key.Version.String(),
	)
	return scanAdoption(key.TenantID, row)
}

// FindResponsibilityStart 按包裹找回先合法形成的责任起点。部分唯一索引保证至多一行
// adopted，无需再排序挑先到者——「先到」由写入路径的撞索引裁决过了。
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
		 WHERE tenant_id = $1
		   AND parcel_id = $2
		   AND adopted`,
		tenant.String(),
		parcel.String(),
	)
	return scanAdoption(tenant, row)
}

// Save 写下一次采用结果。撞主键（重复到达）与撞责任起点索引（并发第二采用）都答
// `已有记录`——前者按原键读回原结果，后者读不回本键、由重试方经 FindResponsibilityStart
// 收敛到不采用。
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
			 refusal_basis, adopted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
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
	sourceObject       *string
	sourcePlace        *string
	sourceControl      *string
	occurredAt         *time.Time
	baseline           *string
	commitmentVersion  *string
	expectedCommitment *string
	refusalBasis       *string
}

// adoptionColumns 把记录折成行。采用面要求收寄与承诺同键同源、承诺为首版——本库
// 承载的是采用时刻的首版承诺，带前版的调整承诺属另一份记录，塞进来就是拼坏的聚合。
func adoptionColumns(record ports.IntakeAdoptionRecord) (adoptionRowColumns, error) {
	if !record.Adopted {
		if record.RefusalBasis.String() == "" {
			return adoptionRowColumns{}, errors.New("a refusal carries its basis")
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
	if _, adjusted := record.Commitment.PriorVersion(); adjusted {
		return adoptionRowColumns{}, errors.New("an adoption record carries the first commitment version only")
	}

	object := source.Object().String()
	place := source.Place().String()
	control := source.Control().String()
	occurredAt := source.OccurredAt()
	baseline := record.Intake.Baseline().String()
	commitmentVersion := record.Commitment.Version().String()
	expected := record.Commitment.Expected().String()
	return adoptionRowColumns{
		sourceObject:       &object,
		sourcePlace:        &place,
		sourceControl:      &control,
		occurredAt:         &occurredAt,
		baseline:           &baseline,
		commitmentVersion:  &commitmentVersion,
		expectedCommitment: &expected,
	}, nil
}

// scanAdoption 读一行并经领域构造门重建（NewIntakeSource / AdoptNetworkIntake /
// FormFormalCommitment）：来源五件完备与承诺引用在读回处复验。
func scanAdoption(
	tenant domain.TenantID,
	row pgx.Row,
) (ports.IntakeAdoptionRecord, bool, error) {
	var (
		parcel, kindRaw, version          string
		customerAccount, shipmentRequest  string
		digest                            string
		adopted                           bool
		object, place, control            *string
		occurredAt                        *time.Time
		baseline, commitVersion, expected *string
		refusalBasis                      *string
		adoptedAt                         time.Time
	)
	err := row.Scan(&parcel, &kindRaw, &version,
		&customerAccount, &shipmentRequest, &digest, &adopted,
		&object, &place, &control, &occurredAt,
		&baseline, &commitVersion, &expected,
		&refusalBasis, &adoptedAt)
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
	intake, commitment, err := rebuildAdoption(record.Key, adoptedColumns{
		object:     *object,
		place:      *place,
		control:    *control,
		occurredAt: *occurredAt,
		baseline:   *baseline,
		version:    *commitVersion,
		expected:   *expected,
	})
	if err != nil {
		return ports.IntakeAdoptionRecord{}, false, fmt.Errorf("read intake adoption: %w", err)
	}
	record.Intake = intake
	record.Commitment = commitment
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
}

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
	source, err := domain.NewIntakeSource(domain.IntakeSourceSpec{
		Kind:       key.Kind,
		Object:     object,
		Parcel:     key.Parcel,
		Place:      place,
		Control:    control,
		Version:    key.Version,
		OccurredAt: columns.occurredAt,
	})
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
	commitment, err := domain.FormFormalCommitment(version, intake, expected)
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
