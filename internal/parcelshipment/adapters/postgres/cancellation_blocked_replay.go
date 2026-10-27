package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CancellationBlockedReplays 实现 ports.CancellationBlockedReplays：被一份取消挡住的来源回放原料（ADR-0295 决定四）。写不单设入口——收寄采用的冲突行
// 与终局采用的不采用行各在自己落库的那一句旁边带上它（insertBlockedReplay），原料与被挡住的记录同笔成立；本类型只管读回。
type CancellationBlockedReplays struct {
	db *bentopg.DB
}

func NewCancellationBlockedReplays(db *bentopg.DB) (*CancellationBlockedReplays, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &CancellationBlockedReplays{db: db}, nil
}

var _ ports.CancellationBlockedReplays = (*CancellationBlockedReplays)(nil)

// 回放所属的链，库内 CHECK 钉住这两个词。
const (
	blockedReplayChainIntake = "NETWORK_INTAKE_ADOPTION"
	blockedReplayChainFinal  = "PARCEL_FINAL_ADOPTION"
)

// blockedReplayBody 是 replay 列的线格式：收寄来源规格与终局责任结果规格二居其一，字段全是字符串与时间，读回时逐项经领域构造器复验。
type blockedReplayBody struct {
	Intake *blockedIntakeBody `json:"intake,omitempty"`
	Final  *blockedFinalBody  `json:"final,omitempty"`
}

type blockedIntakeBody struct {
	Kind       string    `json:"kind"`
	Object     string    `json:"object"`
	Parcel     string    `json:"parcel"`
	Place      string    `json:"place"`
	Control    string    `json:"control"`
	Version    string    `json:"version"`
	OccurredAt time.Time `json:"occurredAt"`
	Corrects   string    `json:"corrects,omitempty"`
}

type blockedFinalBody struct {
	Kind       string    `json:"kind"`
	Parcel     string    `json:"parcel"`
	Decision   string    `json:"decision"`
	Execution  string    `json:"execution"`
	Version    string    `json:"version"`
	OccurredAt time.Time `json:"occurredAt"`
}

// insertBlockedReplay 记下一份回放原料；同一来源重投撞主键即已有，不翻倍。必须在调用方的事务里：原料与被挡住的记录同笔成立。
func insertBlockedReplay(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	replay ports.CancellationBlockedReplay,
	recordedAt time.Time,
) error {
	chain, parcel, sourceRef, body, err := encodeBlockedReplay(replay)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("record cancellation blocked replay: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.cancellation_blocked_replay
			(tenant_id, parcel_id, cancellation_id, chain, source_ref, replay, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), parcel, replay.Cancellation.String(), chain, sourceRef, string(encoded), recordedAt.UTC(),
	); err != nil {
		return fmt.Errorf("record cancellation blocked replay: %w", err)
	}
	return nil
}

func encodeBlockedReplay(replay ports.CancellationBlockedReplay) (chain, parcel, sourceRef string, body blockedReplayBody, err error) {
	if replay.Cancellation.String() == "" {
		return "", "", "", blockedReplayBody{}, errors.New("record cancellation blocked replay: the cancellation is required")
	}
	switch {
	case replay.Intake != nil && replay.Final == nil:
		spec := replay.Intake
		return blockedReplayChainIntake, spec.Parcel.String(), spec.Kind.String() + "/" + spec.Version.String(),
			blockedReplayBody{Intake: &blockedIntakeBody{
				Kind:       spec.Kind.String(),
				Object:     spec.Object.String(),
				Parcel:     spec.Parcel.String(),
				Place:      spec.Place.String(),
				Control:    spec.Control.String(),
				Version:    spec.Version.String(),
				OccurredAt: spec.OccurredAt.UTC(),
				Corrects:   spec.Corrects.String(),
			}}, nil
	case replay.Final != nil && replay.Intake == nil:
		spec := replay.Final
		return blockedReplayChainFinal, spec.Parcel.String(), spec.Kind.String() + "/" + spec.Version.String(),
			blockedReplayBody{Final: &blockedFinalBody{
				Kind:       spec.Kind.String(),
				Parcel:     spec.Parcel.String(),
				Decision:   spec.Decision.String(),
				Execution:  spec.Execution.String(),
				Version:    spec.Version.String(),
				OccurredAt: spec.OccurredAt.UTC(),
			}}, nil
	default:
		return "", "", "", blockedReplayBody{}, errors.New(
			"record cancellation blocked replay: exactly one of the intake source and the final outcome is required")
	}
}

// ListCancellationBlockedReplays 按（租户，包裹，取消）读回被这份取消挡住的来源回放原料，最早记下的在前。租户条件进语句，否定结果只回空。
func (replays *CancellationBlockedReplays) ListCancellationBlockedReplays(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
	cancellation domain.ParcelCancellationID,
) ([]ports.CancellationBlockedReplay, error) {
	querier, err := replays.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cancellation blocked replays: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT chain, replay
		   FROM parcel_shipment.cancellation_blocked_replay
		  WHERE tenant_id = $1 AND parcel_id = $2 AND cancellation_id = $3
		  ORDER BY recorded_at, chain, source_ref`,
		tenant.String(), parcel.String(), cancellation.String())
	if err != nil {
		return nil, fmt.Errorf("list cancellation blocked replays: %w", err)
	}
	defer rows.Close()

	var listed []ports.CancellationBlockedReplay
	for rows.Next() {
		var chain, encoded string
		if err := rows.Scan(&chain, &encoded); err != nil {
			return nil, fmt.Errorf("list cancellation blocked replays: %w", err)
		}
		replay, err := decodeBlockedReplay(cancellation, chain, encoded)
		if err != nil {
			return nil, fmt.Errorf("list cancellation blocked replays: %w", err)
		}
		listed = append(listed, replay)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cancellation blocked replays: %w", err)
	}
	return listed, nil
}

func decodeBlockedReplay(
	cancellation domain.ParcelCancellationID,
	chain, encoded string,
) (ports.CancellationBlockedReplay, error) {
	var body blockedReplayBody
	if err := json.Unmarshal([]byte(encoded), &body); err != nil {
		return ports.CancellationBlockedReplay{}, fmt.Errorf("replay body: %w", err)
	}
	switch {
	case chain == blockedReplayChainIntake && body.Intake != nil && body.Final == nil:
		spec, err := intakeSpecOf(*body.Intake)
		if err != nil {
			return ports.CancellationBlockedReplay{}, err
		}
		return ports.CancellationBlockedReplay{Cancellation: cancellation, Intake: &spec}, nil
	case chain == blockedReplayChainFinal && body.Final != nil && body.Intake == nil:
		spec, err := finalSpecOf(*body.Final)
		if err != nil {
			return ports.CancellationBlockedReplay{}, err
		}
		return ports.CancellationBlockedReplay{Cancellation: cancellation, Final: &spec}, nil
	default:
		return ports.CancellationBlockedReplay{}, fmt.Errorf("replay body does not belong to chain %q", chain)
	}
}

func intakeSpecOf(body blockedIntakeBody) (domain.IntakeSourceSpec, error) {
	kind, err := intakeSourceKindFrom(body.Kind)
	if err != nil {
		return domain.IntakeSourceSpec{}, err
	}
	object, err := domain.NewSourceObjectReference(body.Object)
	if err != nil {
		return domain.IntakeSourceSpec{}, err
	}
	parcel, err := domain.NewDeclaredParcelID(body.Parcel)
	if err != nil {
		return domain.IntakeSourceSpec{}, err
	}
	place, err := domain.NewIntakePlaceReference(body.Place)
	if err != nil {
		return domain.IntakeSourceSpec{}, err
	}
	control, err := domain.NewIntakeControlReference(body.Control)
	if err != nil {
		return domain.IntakeSourceSpec{}, err
	}
	version, err := domain.NewSourceResultVersion(body.Version)
	if err != nil {
		return domain.IntakeSourceSpec{}, err
	}
	spec := domain.IntakeSourceSpec{
		Kind: kind, Object: object, Parcel: parcel, Place: place, Control: control, Version: version,
		OccurredAt: body.OccurredAt.UTC(),
	}
	if body.Corrects != "" {
		if spec.Corrects, err = domain.NewSourceResultVersion(body.Corrects); err != nil {
			return domain.IntakeSourceSpec{}, err
		}
	}
	return spec, nil
}

func finalSpecOf(body blockedFinalBody) (domain.ResponsibilityOutcomeSpec, error) {
	kind, err := responsibilityKindFrom(body.Kind)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	parcel, err := domain.NewDeclaredParcelID(body.Parcel)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	decision, err := domain.NewResponsibilityDecisionReference(body.Decision)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	execution, err := domain.NewExecutionEvidenceReference(body.Execution)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	version, err := domain.NewResponsibilityOutcomeVersion(body.Version)
	if err != nil {
		return domain.ResponsibilityOutcomeSpec{}, err
	}
	return domain.ResponsibilityOutcomeSpec{
		Kind: kind, Parcel: parcel, Decision: decision, Execution: execution, Version: version,
		OccurredAt: body.OccurredAt.UTC(),
	}, nil
}
