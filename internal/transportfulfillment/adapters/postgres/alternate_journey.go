package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// AlternateJourneys 实现 ports.AlternateJourneyStore（写入代数同 ADR-0031）。
// 主键取（租户+原旅程+目的+处置依据）：同一处置决定不开两条替代旅程。
type AlternateJourneys struct {
	db *bentopg.DB
}

func NewAlternateJourneys(db *bentopg.DB) (*AlternateJourneys, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &AlternateJourneys{db: db}, nil
}

// FindByKey 按幂等键取回已启动的替代/退运旅程。否定结果只回 false。读回经公开
// 构造门复验：身份独立、成员非空不重、目的与依据种类封闭。
func (repository *AlternateJourneys) FindByKey(
	ctx context.Context,
	key ports.AlternateJourneyKey,
) (ports.AlternateJourneyRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AlternateJourneyRecord{}, false, fmt.Errorf("find alternate journey: %w", err)
	}

	var journeyID, basisKind, digest string
	var membersJSON []byte
	var startedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT journey_id, basis_kind, members, started_at, content_digest, recorded_at
		   FROM transport_fulfillment.alternate_journey
		  WHERE tenant_id = $1
		    AND original_journey = $2
		    AND purpose = $3
		    AND basis = $4`,
		key.TenantID.String(),
		key.Original.String(),
		key.Purpose.String(),
		key.Basis.String(),
	).Scan(&journeyID, &basisKind, &membersJSON, &startedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AlternateJourneyRecord{}, false, nil
	}
	if err != nil {
		return ports.AlternateJourneyRecord{}, false, fmt.Errorf("find alternate journey: %w", err)
	}

	record, err := alternateJourneyRecordFrom(key, journeyID, basisKind, membersJSON, startedAt, digest, recordedAt)
	if err != nil {
		return ports.AlternateJourneyRecord{}, false, fmt.Errorf("find alternate journey: %w", err)
	}
	return record, true, nil
}

// FindByMember 按成员读这个对象出现在其中的全部替代 / 退运旅程（实现 ports.AlternateJourneyMemberView，ADR-0218）。没有
// 就是空切片；哪一趟算数由领域判，这里不挑、不排序。读回同 FindByKey 一道经公开构造门复验。
func (repository *AlternateJourneys) FindByMember(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) ([]ports.AlternateJourneyRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find alternate journeys by member: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT original_journey, purpose, basis, journey_id, basis_kind, members, started_at, content_digest, recorded_at
		   FROM transport_fulfillment.alternate_journey
		  WHERE tenant_id = $1
		    AND members @> jsonb_build_array($2::text)`,
		tenant.String(),
		object.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find alternate journeys by member: %w", err)
	}
	defer rows.Close()

	var records []ports.AlternateJourneyRecord
	for rows.Next() {
		var original, purpose, basis, journeyID, basisKind, digest string
		var membersJSON []byte
		var startedAt, recordedAt time.Time
		if err := rows.Scan(&original, &purpose, &basis, &journeyID, &basisKind, &membersJSON, &startedAt, &digest, &recordedAt); err != nil {
			return nil, fmt.Errorf("find alternate journeys by member: %w", err)
		}
		key, err := alternateJourneyKeyFrom(tenant, original, purpose, basis)
		if err != nil {
			return nil, fmt.Errorf("find alternate journeys by member: %w", err)
		}
		record, err := alternateJourneyRecordFrom(key, journeyID, basisKind, membersJSON, startedAt, digest, recordedAt)
		if err != nil {
			return nil, fmt.Errorf("find alternate journeys by member: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find alternate journeys by member: %w", err)
	}
	return records, nil
}

var _ ports.AlternateJourneyMemberView = (*AlternateJourneys)(nil)

// alternateJourneyKeyFrom 把库面的键列译回幂等键；目的不在封闭二值里是库面违约。
func alternateJourneyKeyFrom(tenant domain.TenantID, original, purpose, basis string) (ports.AlternateJourneyKey, error) {
	originalJourney, err := domain.NewJourneyReference(original)
	if err != nil {
		return ports.AlternateJourneyKey{}, err
	}
	dispositionBasis, err := domain.NewDispositionBasisReference(basis)
	if err != nil {
		return ports.AlternateJourneyKey{}, err
	}
	var journeyPurpose domain.JourneyPurpose
	switch purpose {
	case domain.AlternateJourneyPurpose.String():
		journeyPurpose = domain.AlternateJourneyPurpose
	case domain.ReturnJourneyPurpose.String():
		journeyPurpose = domain.ReturnJourneyPurpose
	default:
		return ports.AlternateJourneyKey{}, fmt.Errorf("unknown journey purpose %q", purpose)
	}
	return ports.AlternateJourneyKey{TenantID: tenant, Original: originalJourney, Purpose: journeyPurpose, Basis: dispositionBasis}, nil
}

// alternateJourneyRecordFrom 把一行的内容列连同它的键经公开构造门重建成记录：身份独立、成员非空不重、目的与依据种类封闭。
func alternateJourneyRecordFrom(
	key ports.AlternateJourneyKey,
	journeyID, basisKind string,
	membersJSON []byte,
	startedAt time.Time,
	digest string,
	recordedAt time.Time,
) (ports.AlternateJourneyRecord, error) {
	kind, err := dispositionBasisKindFrom(basisKind)
	if err != nil {
		return ports.AlternateJourneyRecord{}, err
	}
	journeyRef, err := domain.NewJourneyReference(journeyID)
	if err != nil {
		return ports.AlternateJourneyRecord{}, err
	}
	var memberIDs []string
	if err := json.Unmarshal(membersJSON, &memberIDs); err != nil {
		return ports.AlternateJourneyRecord{}, err
	}
	members := make([]domain.CarriedObjectReference, 0, len(memberIDs))
	for _, raw := range memberIDs {
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return ports.AlternateJourneyRecord{}, err
		}
		members = append(members, member)
	}
	journey, err := domain.FormAlternateJourney(domain.AlternateJourneySpec{
		TenantID:        key.TenantID,
		Journey:         journeyRef,
		Purpose:         key.Purpose,
		OriginalJourney: key.Original,
		BasisKind:       kind,
		Basis:           key.Basis,
		Members:         members,
		StartedAt:       startedAt,
	})
	if err != nil {
		return ports.AlternateJourneyRecord{}, err
	}
	return ports.AlternateJourneyRecord{
		Key:           key,
		ContentDigest: digest,
		Journey:       journey,
		RecordedAt:    recordedAt.UTC(),
	}, nil
}

// Save 写下一次旅程启动。同键已有记录时答`已启动`，不覆盖先到者。
func (repository *AlternateJourneys) Save(
	ctx context.Context,
	record ports.AlternateJourneyRecord,
) (ports.AlternateJourneySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AlternateJourneySaveOutcomeInvalid, fmt.Errorf("save alternate journey: %w", err)
	}

	memberIDs := make([]string, 0, len(record.Journey.Members()))
	for _, member := range record.Journey.Members() {
		memberIDs = append(memberIDs, member.String())
	}
	membersJSON, err := json.Marshal(memberIDs)
	if err != nil {
		return ports.AlternateJourneySaveOutcomeInvalid, fmt.Errorf("save alternate journey: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.alternate_journey
			(tenant_id, original_journey, purpose, basis, journey_id, basis_kind,
			 members, started_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Original.String(),
		record.Key.Purpose.String(),
		record.Key.Basis.String(),
		record.Journey.Journey().String(),
		record.Journey.BasisKind().String(),
		membersJSON,
		record.Journey.StartedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AlternateJourneySaveOutcomeInvalid, fmt.Errorf("save alternate journey: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AlternateJourneyAlreadyStarted, nil
	}
	return ports.AlternateJourneySaved, nil
}

func dispositionBasisKindFrom(raw string) (domain.DispositionBasisKind, error) {
	switch raw {
	case domain.ServiceDispositionDecision.String():
		return domain.ServiceDispositionDecision, nil
	case domain.RegulatoryDispositionDecision.String():
		return domain.RegulatoryDispositionDecision, nil
	default:
		return 0, fmt.Errorf("unknown disposition basis kind %q", raw)
	}
}
