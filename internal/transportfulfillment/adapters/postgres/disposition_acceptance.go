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

// DispositionAcceptances 实现 ports.DispositionAcceptanceStore（写入代数同 ADR-0031）。
type DispositionAcceptances struct {
	db *bentopg.DB
}

func NewDispositionAcceptances(db *bentopg.DB) (*DispositionAcceptances, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DispositionAcceptances{db: db}, nil
}

// FindByKey 按（租户+协作事项）取回已保存的承接决定。否定结果只回 false。读回经
// FormRegulatoryTransportDisposition 重建。
func (repository *DispositionAcceptances) FindByKey(
	ctx context.Context,
	key ports.DispositionAcceptanceKey,
) (ports.DispositionAcceptanceRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DispositionAcceptanceRecord{}, false, fmt.Errorf("find disposition acceptance: %w", err)
	}

	var basis, kindName, digest string
	var declineBasis, authority *string
	var objectsJSON []byte
	var decidedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT basis, kind, accepted_objects, decline_basis, movement_authority,
		        decided_at, content_digest
		   FROM transport_fulfillment.disposition_acceptance
		  WHERE tenant_id = $1
		    AND item_ref = $2`,
		key.Tenant.String(),
		key.Item.String(),
	).Scan(&basis, &kindName, &objectsJSON, &declineBasis, &authority, &decidedAt, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DispositionAcceptanceRecord{}, false, nil
	}
	if err != nil {
		return ports.DispositionAcceptanceRecord{}, false, fmt.Errorf("find disposition acceptance: %w", err)
	}

	decision, err := rebuildDisposition(key, basis, kindName, objectsJSON, declineBasis, authority, decidedAt)
	if err != nil {
		return ports.DispositionAcceptanceRecord{}, false, fmt.Errorf("find disposition acceptance: %w", err)
	}
	return ports.DispositionAcceptanceRecord{
		Key:           key,
		ContentDigest: digest,
		Decision:      decision,
	}, true, nil
}

// Save 写下一次承接决定。同（租户+事项）已有记录时答`已有记录`，不覆盖先到者。
func (repository *DispositionAcceptances) Save(
	ctx context.Context,
	record ports.DispositionAcceptanceRecord,
) (ports.DispositionAcceptanceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DispositionAcceptanceSaveOutcomeInvalid, fmt.Errorf("save disposition acceptance: %w", err)
	}

	objectIDs := make([]string, 0, len(record.Decision.AcceptedObjects()))
	for _, object := range record.Decision.AcceptedObjects() {
		objectIDs = append(objectIDs, object.String())
	}
	objectsJSON, err := json.Marshal(objectIDs)
	if err != nil {
		return ports.DispositionAcceptanceSaveOutcomeInvalid, fmt.Errorf("save disposition acceptance: %w", err)
	}

	var decline *string
	if raw := record.Decision.DeclineBasis(); raw != "" {
		decline = &raw
	}
	var authority *string
	if value, present := record.Decision.MovementAuthority(); present {
		raw := value.String()
		authority = &raw
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.disposition_acceptance
			(tenant_id, item_ref, basis, kind, accepted_objects, decline_basis,
			 movement_authority, decided_at, content_digest)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		record.Key.Tenant.String(),
		record.Key.Item.String(),
		record.Decision.Basis().String(),
		record.Decision.Kind().String(),
		objectsJSON,
		decline,
		authority,
		record.Decision.DecidedAt().UTC(),
		record.ContentDigest,
	)
	if err != nil {
		return ports.DispositionAcceptanceSaveOutcomeInvalid, fmt.Errorf("save disposition acceptance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DispositionAcceptanceAlreadyRecorded, nil
	}
	return ports.DispositionAcceptanceSaved, nil
}

func rebuildDisposition(
	key ports.DispositionAcceptanceKey,
	basis, kindName string,
	objectsJSON []byte,
	declineBasis, authority *string,
	decidedAt time.Time,
) (domain.RegulatoryTransportDisposition, error) {
	basisRef, err := domain.NewDispositionBasisReference(basis)
	if err != nil {
		return domain.RegulatoryTransportDisposition{}, err
	}
	kind, err := dispositionKindFrom(kindName)
	if err != nil {
		return domain.RegulatoryTransportDisposition{}, err
	}

	var objectIDs []string
	if err := json.Unmarshal(objectsJSON, &objectIDs); err != nil {
		return domain.RegulatoryTransportDisposition{}, err
	}
	objects := make([]domain.CarriedObjectReference, 0, len(objectIDs))
	for _, raw := range objectIDs {
		object, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.RegulatoryTransportDisposition{}, err
		}
		objects = append(objects, object)
	}

	spec := domain.RegulatoryTransportDispositionSpec{
		Tenant:          key.Tenant,
		Item:            key.Item,
		Basis:           basisRef,
		Kind:            kind,
		AcceptedObjects: objects,
		DecidedAt:       decidedAt,
	}
	if declineBasis != nil {
		spec.DeclineBasis = *declineBasis
	}
	if authority != nil {
		movement, err := domain.NewMovementAuthorityReference(*authority)
		if err != nil {
			return domain.RegulatoryTransportDisposition{}, err
		}
		spec.MovementAuthority = movement
	}
	return domain.FormRegulatoryTransportDisposition(spec)
}

func dispositionKindFrom(raw string) (domain.DispositionAcceptanceKind, error) {
	switch raw {
	case domain.DispositionAccepted.String():
		return domain.DispositionAccepted, nil
	case domain.DispositionPartiallyAccepted.String():
		return domain.DispositionPartiallyAccepted, nil
	case domain.DispositionDeclined.String():
		return domain.DispositionDeclined, nil
	default:
		return 0, fmt.Errorf("unknown disposition acceptance kind %q", raw)
	}
}
