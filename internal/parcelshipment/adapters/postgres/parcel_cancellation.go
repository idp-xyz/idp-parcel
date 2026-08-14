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

// ParcelCancellations 实现 ports.ParcelCancellationStore 与 ports.ParcelCancellationView
// ——采用编排核对取消边界读的正是本库成立的取消决定，两口同一张表、同一行模型，分开
// 实现就是第二处定义。主键=（租户+请求键+包裹）：同一请求身份返回原逐包裹结果；
// `已有记录`由 ON CONFLICT DO NOTHING 加零行判定翻译（ADR-0031）。
type ParcelCancellations struct {
	db *bentopg.DB
}

func NewParcelCancellations(db *bentopg.DB) (*ParcelCancellations, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ParcelCancellations{db: db}, nil
}

// FindByKey 按请求键取回已提交取消判断。否定结果只回 false，不区分「不存在」与
// 「属于另一个租户」（AT-PS-090 的隔离面）。
func (repository *ParcelCancellations) FindByKey(
	ctx context.Context,
	key ports.CancellationRequestKey,
) (ports.CancellationRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CancellationRecord{}, false, fmt.Errorf("find cancellation: %w", err)
	}

	var (
		digest, kindRaw              string
		cancellationID, requester    *string
		authority, reason            *string
		requestedAt                  *time.Time
		crossedVersion, refusalBasis *string
		decidedAt                    time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT content_digest, kind,
		        cancellation_id, requester_ref, authority_ref, reason_ref,
		        requested_at, crossed_intake_version, refusal_basis, decided_at
		   FROM parcel_shipment.parcel_cancellation
		  WHERE tenant_id = $1
		    AND request_key = $2
		    AND parcel_id = $3`,
		key.TenantID.String(),
		key.RequestKey.String(),
		key.Parcel.String(),
	).Scan(&digest, &kindRaw,
		&cancellationID, &requester, &authority, &reason,
		&requestedAt, &crossedVersion, &refusalBasis, &decidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CancellationRecord{}, false, nil
	}
	if err != nil {
		return ports.CancellationRecord{}, false, fmt.Errorf("find cancellation: %w", err)
	}

	record := ports.CancellationRecord{
		Key:           key,
		ContentDigest: digest,
		DecidedAt:     decidedAt.UTC(),
	}
	record.Kind, err = cancellationKindFrom(kindRaw)
	if err != nil {
		return ports.CancellationRecord{}, false, fmt.Errorf("find cancellation: %w", err)
	}

	switch record.Kind {
	case ports.RecordParcelCancelled:
		if cancellationID == nil || requester == nil || authority == nil ||
			reason == nil || requestedAt == nil {
			return ports.CancellationRecord{}, false, errors.New(
				"find cancellation: a cancelled row arrived with missing decision columns")
		}
		record.Cancellation, err = rebuildCancellation(key.Parcel, cancelledColumns{
			id:          *cancellationID,
			requester:   *requester,
			authority:   *authority,
			reason:      *reason,
			requestedAt: *requestedAt,
		})
		if err != nil {
			return ports.CancellationRecord{}, false, fmt.Errorf("find cancellation: %w", err)
		}
	case ports.RecordDispositionPending:
		if crossedVersion == nil {
			return ports.CancellationRecord{}, false, errors.New(
				"find cancellation: a pending row arrived without the crossed intake version")
		}
		if record.IntakeVersion, err = domain.NewSourceResultVersion(*crossedVersion); err != nil {
			return ports.CancellationRecord{}, false, fmt.Errorf("find cancellation: %w", err)
		}
	case ports.RecordCancellationRefused:
		if refusalBasis == nil {
			return ports.CancellationRecord{}, false, errors.New(
				"find cancellation: a refused row arrived without its basis")
		}
		if record.RefusalBasis, err = domain.NewCheckReason(*refusalBasis); err != nil {
			return ports.CancellationRecord{}, false, fmt.Errorf("find cancellation: %w", err)
		}
	}
	return record, true, nil
}

// FindCancellation 按包裹读回已成立的取消决定（ParcelCancellationView）。同一包裹
// 多份成立取消时按业务时间取最早——「先合法形成者赢」用的比对时间正是它。
func (repository *ParcelCancellations) FindCancellation(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.ParcelCancellation, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ParcelCancellation{}, false, fmt.Errorf("find parcel cancellation: %w", err)
	}

	var (
		cancellationID, requester, authority, reason string
		requestedAt                                  time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT cancellation_id, requester_ref, authority_ref, reason_ref, requested_at
		   FROM parcel_shipment.parcel_cancellation
		  WHERE tenant_id = $1
		    AND parcel_id = $2
		    AND kind = 'PARCEL_CANCELLED'
		  ORDER BY requested_at
		  LIMIT 1`,
		tenant.String(),
		parcel.String(),
	).Scan(&cancellationID, &requester, &authority, &reason, &requestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ParcelCancellation{}, false, nil
	}
	if err != nil {
		return domain.ParcelCancellation{}, false, fmt.Errorf("find parcel cancellation: %w", err)
	}

	cancellation, err := rebuildCancellation(parcel, cancelledColumns{
		id:          cancellationID,
		requester:   requester,
		authority:   authority,
		reason:      reason,
		requestedAt: requestedAt,
	})
	if err != nil {
		return domain.ParcelCancellation{}, false, fmt.Errorf("find parcel cancellation: %w", err)
	}
	return cancellation, true, nil
}

// Save 写下一次取消判断。同（租户+请求键+包裹）已有记录时答`已有记录`——业务答案
// 不是错误（ADR-0031），编排据此读回赢家、按 content_digest 分重放与冲突。
func (repository *ParcelCancellations) Save(
	ctx context.Context,
	record ports.CancellationRecord,
) (ports.CancellationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CancellationSaveOutcomeInvalid, fmt.Errorf("save cancellation: %w", err)
	}

	columns, err := cancellationColumns(record)
	if err != nil {
		return ports.CancellationSaveOutcomeInvalid, fmt.Errorf("save cancellation: %w", err)
	}

	key := record.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.parcel_cancellation
			(tenant_id, request_key, parcel_id, content_digest, kind,
			 cancellation_id, requester_ref, authority_ref, reason_ref,
			 requested_at, crossed_intake_version, refusal_basis, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		key.RequestKey.String(),
		key.Parcel.String(),
		record.ContentDigest,
		record.Kind.String(),
		columns.cancellationID,
		columns.requester,
		columns.authority,
		columns.reason,
		columns.requestedAt,
		columns.crossedVersion,
		columns.refusalBasis,
		record.DecidedAt.UTC(),
	)
	if err != nil {
		return ports.CancellationSaveOutcomeInvalid, fmt.Errorf("save cancellation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CancellationAlreadyRecorded, nil
	}
	return ports.CancellationSaved, nil
}

type cancellationRowColumns struct {
	cancellationID *string
	requester      *string
	authority      *string
	reason         *string
	requestedAt    *time.Time
	crossedVersion *string
	refusalBasis   *string
}

// cancellationColumns 把记录折成行：三走向各自的在场件在这里核一遍，缺件与混面
// 响亮报错不落库（数据库 CHECK 是第二道）。
func cancellationColumns(record ports.CancellationRecord) (cancellationRowColumns, error) {
	switch record.Kind {
	case ports.RecordParcelCancelled:
		cancellation := record.Cancellation
		if cancellation.Parcel() != record.Key.Parcel {
			return cancellationRowColumns{}, errors.New("record key disagrees with the cancellation's parcel")
		}
		id := cancellation.ID().String()
		requester := cancellation.Requester().String()
		authority := cancellation.Authority().String()
		reason := cancellation.Reason().String()
		requestedAt := cancellation.RequestedAt()
		return cancellationRowColumns{
			cancellationID: &id,
			requester:      &requester,
			authority:      &authority,
			reason:         &reason,
			requestedAt:    &requestedAt,
		}, nil
	case ports.RecordDispositionPending:
		if record.IntakeVersion.String() == "" {
			return cancellationRowColumns{}, errors.New("a pending record carries the crossed intake version")
		}
		crossed := record.IntakeVersion.String()
		return cancellationRowColumns{crossedVersion: &crossed}, nil
	case ports.RecordCancellationRefused:
		if record.RefusalBasis.String() == "" {
			return cancellationRowColumns{}, errors.New("a refusal carries its basis")
		}
		basis := record.RefusalBasis.String()
		return cancellationRowColumns{refusalBasis: &basis}, nil
	default:
		return cancellationRowColumns{}, fmt.Errorf("unknown cancellation record kind %d", record.Kind)
	}
}

type cancelledColumns struct {
	id          string
	requester   string
	authority   string
	reason      string
	requestedAt time.Time
}

// rebuildCancellation 经 DecideParcelCancellation 重走决定门（收寄事实以缺席传入：
// 边界核验属决定时刻，行里存的是已越过边界核验的决定本体）。
func rebuildCancellation(
	parcel domain.DeclaredParcelID,
	columns cancelledColumns,
) (domain.ParcelCancellation, error) {
	id, err := domain.NewParcelCancellationID(columns.id)
	if err != nil {
		return domain.ParcelCancellation{}, err
	}
	requester, err := domain.NewCancellationRequesterReference(columns.requester)
	if err != nil {
		return domain.ParcelCancellation{}, err
	}
	authority, err := domain.NewCancellationAuthorityReference(columns.authority)
	if err != nil {
		return domain.ParcelCancellation{}, err
	}
	reason, err := domain.NewCancellationReasonReference(columns.reason)
	if err != nil {
		return domain.ParcelCancellation{}, err
	}
	return domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          id,
		Parcel:      parcel,
		Requester:   requester,
		Authority:   authority,
		Reason:      reason,
		RequestedAt: columns.requestedAt,
	}, domain.CurrentIntakeFact{})
}

func cancellationKindFrom(raw string) (ports.CancellationRecordKind, error) {
	switch raw {
	case ports.RecordParcelCancelled.String():
		return ports.RecordParcelCancelled, nil
	case ports.RecordDispositionPending.String():
		return ports.RecordDispositionPending, nil
	case ports.RecordCancellationRefused.String():
		return ports.RecordCancellationRefused, nil
	default:
		return ports.CancellationRecordKindInvalid, fmt.Errorf("unknown cancellation kind %q", raw)
	}
}
