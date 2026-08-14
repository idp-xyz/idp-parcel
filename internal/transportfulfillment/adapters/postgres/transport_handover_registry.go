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

// TransportHandovers 实现 ports.TransportHandoverRegistry：一行一个交接判断版本。
//
// 更正不换写原行，而是以新版本键落新行——版本在主键里，两代因此天然共存，
// corrects_version 回指前身。这正是「更正形成新版本使原结果失效或被替代，不删除原
// 交接」在库面的样子；用 DO UPDATE 换写会让原判断消失，审计再也答不出改判前是什么。
type TransportHandovers struct {
	db *bentopg.DB
}

func NewTransportHandovers(db *bentopg.DB) (*TransportHandovers, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &TransportHandovers{db: db}, nil
}

var _ ports.TransportHandoverRegistry = (*TransportHandovers)(nil)

// FindByKey 按（租户+对象+范围+版本）取回交接判断。否定结果只回 false。读回经
// RehydrateTransportHandover 复验逐格完备性与版本链，坏行在这里暴露。
func (repository *TransportHandovers) FindByKey(
	ctx context.Context,
	key ports.TransportHandoverKey,
) (ports.TransportHandoverRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.TransportHandoverRecord{}, false, fmt.Errorf("find transport handover: %w", err)
	}

	var releasedBy, receivedBy, verdictName, digest string
	var releasing, receiving, rule, basis, corrects *string
	var correctedAt *time.Time
	var judgedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT released_by, received_by, verdict,
		        releasing_evidence, receiving_evidence, rule_ref, basis_ref,
		        corrects_version, corrected_at, judged_at, content_digest, recorded_at
		   FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND scope_ref = $3
		    AND handover_version = $4`,
		key.TenantID.String(),
		key.Object.String(),
		key.Scope.String(),
		key.Version.String(),
	).Scan(&releasedBy, &receivedBy, &verdictName,
		&releasing, &receiving, &rule, &basis,
		&corrects, &correctedAt, &judgedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.TransportHandoverRecord{}, false, nil
	}
	if err != nil {
		return ports.TransportHandoverRecord{}, false, fmt.Errorf("find transport handover: %w", err)
	}

	handover, err := rebuildHandover(key, handoverRow{
		releasedBy:  releasedBy,
		receivedBy:  receivedBy,
		verdictName: verdictName,
		releasing:   releasing,
		receiving:   receiving,
		rule:        rule,
		basis:       basis,
		corrects:    corrects,
		correctedAt: correctedAt,
		judgedAt:    judgedAt,
	})
	if err != nil {
		return ports.TransportHandoverRecord{}, false, fmt.Errorf("find transport handover: %w", err)
	}
	return ports.TransportHandoverRecord{
		Key:           key,
		ContentDigest: digest,
		Handover:      handover,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 落一个交接判断版本。撞键答`已登记`（ADR-0031），编排据此读回赢家。
func (repository *TransportHandovers) Save(
	ctx context.Context,
	record ports.TransportHandoverRecord,
) (ports.HandoverSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.HandoverSaveOutcomeInvalid, fmt.Errorf("save transport handover: %w", err)
	}
	if err := assertHandoverKeyAgrees(record); err != nil {
		return ports.HandoverSaveOutcomeInvalid, fmt.Errorf("save transport handover: %w", err)
	}

	handover := record.Handover
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_handover
			(tenant_id, object_ref, scope_ref, handover_version,
			 released_by, received_by, verdict,
			 releasing_evidence, receiving_evidence, rule_ref, basis_ref,
			 corrects_version, corrected_at, judged_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Object.String(),
		record.Key.Scope.String(),
		record.Key.Version.String(),
		handover.ReleasedBy().String(),
		handover.ReceivedBy().String(),
		handover.Verdict().String(),
		optionalHandoverText(handover.ReleasingEvidence()),
		optionalHandoverText(handover.ReceivingEvidence()),
		optionalHandoverText(handover.Rule()),
		optionalHandoverText(handover.Basis()),
		optionalHandoverText(handover.Corrects()),
		optionalHandoverTime(handover.CorrectedAt()),
		handover.JudgedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.HandoverSaveOutcomeInvalid, fmt.Errorf("save transport handover: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.HandoverAlreadyRegistered, nil
	}
	return ports.HandoverSaved, nil
}

// handoverRow 收拢一行的可空列，只为让 FindByKey 的重建调用不长成十四个位置参数
// ——位置一多，两个相邻的 *string 调换了顺序编译器不会说话。
type handoverRow struct {
	releasedBy  string
	receivedBy  string
	verdictName string
	releasing   *string
	receiving   *string
	rule        *string
	basis       *string
	corrects    *string
	correctedAt *time.Time
	judgedAt    time.Time
}

func rebuildHandover(
	key ports.TransportHandoverKey,
	row handoverRow,
) (domain.TransportHandover, error) {
	verdict, err := handoverVerdictFrom(row.verdictName)
	if err != nil {
		return domain.TransportHandover{}, err
	}
	spec := domain.RehydrateTransportHandoverSpec{
		TenantID: key.TenantID,
		Object:   key.Object,
		Scope:    key.Scope,
		Version:  key.Version,
		Verdict:  verdict,
		JudgedAt: row.judgedAt,
	}
	if spec.ReleasedBy, err = domain.NewHandoverPartyReference(row.releasedBy); err != nil {
		return domain.TransportHandover{}, err
	}
	if spec.ReceivedBy, err = domain.NewHandoverPartyReference(row.receivedBy); err != nil {
		return domain.TransportHandover{}, err
	}
	if row.releasing != nil {
		if spec.ReleasingEvidence, err = domain.NewHandoverEvidenceReference(*row.releasing); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if row.receiving != nil {
		if spec.ReceivingEvidence, err = domain.NewHandoverEvidenceReference(*row.receiving); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if row.rule != nil {
		if spec.Rule, err = domain.NewHandoverRuleReference(*row.rule); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if row.basis != nil {
		if spec.Basis, err = domain.NewHandoverBasisReference(*row.basis); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if row.corrects != nil {
		if spec.Corrects, err = domain.NewHandoverResultVersion(*row.corrects); err != nil {
			return domain.TransportHandover{}, err
		}
	}
	if row.correctedAt != nil {
		spec.CorrectedAt = *row.correctedAt
	}
	return domain.RehydrateTransportHandover(spec)
}

func assertHandoverKeyAgrees(record ports.TransportHandoverRecord) error {
	handover := record.Handover
	if handover.TenantID() != record.Key.TenantID ||
		handover.Object() != record.Key.Object ||
		handover.Scope() != record.Key.Scope ||
		handover.Version() != record.Key.Version {
		return errors.New("record key disagrees with the handover's own identity")
	}
	return nil
}

// optionalHandoverText 把领域那套 (值, 在场) 折成可空列。缺席写 NULL 而不是空串：
// 库里的逐格完备性 CHECK 用 IS NULL 分形态，空串会绕过它。
func optionalHandoverText[T interface{ String() string }](value T, present bool) *string {
	if !present {
		return nil
	}
	raw := value.String()
	return &raw
}

func optionalHandoverTime(value time.Time, present bool) *time.Time {
	if !present {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func handoverVerdictFrom(raw string) (domain.HandoverVerdict, error) {
	switch raw {
	case domain.ObjectHandedOver.String():
		return domain.ObjectHandedOver, nil
	case domain.HandoverRefused.String():
		return domain.HandoverRefused, nil
	case domain.HandoverPendingConfirmation.String():
		return domain.HandoverPendingConfirmation, nil
	default:
		return 0, fmt.Errorf("unknown handover verdict %q", raw)
	}
}
