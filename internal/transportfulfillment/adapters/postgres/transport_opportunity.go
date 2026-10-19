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

// 本文件是运输准备两口的库面（迁移 0028，ADR-0201 决定六至八）：准备结果库，与准备评估按班次成批读池的那一问。

var _ ports.CapacityPoolsOnSchedules = (*CapacityPools)(nil)

// ListPoolsOnSchedules 按班次交回登记册上的池。先取池身份再逐个经 FindByKey 读回：读回要过重建门、核池身份由班次与维度
// 派生，准备评估判容量用的池与登记口、预占口读到的是同一个，没有第二条读法。
func (repository *CapacityPools) ListPoolsOnSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	schedules []domain.ScheduleReference,
) ([]ports.CapacityPoolRecord, error) {
	if len(schedules) == 0 {
		return nil, nil
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list capacity pools on schedules: %w", err)
	}
	identities := make([]string, 0, len(schedules))
	for _, schedule := range schedules {
		identities = append(identities, schedule.String())
	}
	rows, err := querier.Query(ctx,
		`SELECT pool_id
		   FROM transport_fulfillment.capacity_pool
		  WHERE tenant_id = $1
		    AND schedule_id = ANY($2)
		  ORDER BY pool_id`,
		tenant.String(),
		identities,
	)
	if err != nil {
		return nil, fmt.Errorf("list capacity pools on schedules: %w", err)
	}
	var pools []string
	for rows.Next() {
		var pool string
		if err := rows.Scan(&pool); err != nil {
			rows.Close()
			return nil, fmt.Errorf("list capacity pools on schedules: %w", err)
		}
		pools = append(pools, pool)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list capacity pools on schedules: %w", err)
	}

	records := make([]ports.CapacityPoolRecord, 0, len(pools))
	for _, pool := range pools {
		reference, err := domain.NewCapacityPoolReference(pool)
		if err != nil {
			return nil, fmt.Errorf("list capacity pools on schedules: %w", err)
		}
		record, found, err := repository.FindByKey(ctx, ports.CapacityPoolKey{TenantID: tenant, Pool: reference})
		if err != nil {
			return nil, fmt.Errorf("list capacity pools on schedules: %w", err)
		}
		if !found {
			return nil, fmt.Errorf("list capacity pools on schedules: pool %q vanished between two reads", pool)
		}
		records = append(records, record)
	}
	return records, nil
}

// TransportPreparations 实现 ports.TransportPreparationStore。只插不改：同一身份已有一行时答已落过，由编排读回分重放与冲突。
type TransportPreparations struct {
	db *bentopg.DB
}

var _ ports.TransportPreparationStore = (*TransportPreparations)(nil)

func NewTransportPreparations(db *bentopg.DB) (*TransportPreparations, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &TransportPreparations{db: db}, nil
}

// storedDemandQuantity 与 storedCandidate、storedPoolAssessment 是两列 jsonb 快照的行形。键名照查阅册的 JSON 口径写，
// 读库的人与读接口的人看到同一组词。
type storedDemandQuantity struct {
	Dimension string `json:"dimension"`
	Amount    string `json:"amount"`
	Unit      string `json:"unit"`
}

type storedPoolAssessment struct {
	Pool      string `json:"pool"`
	Dimension string `json:"dimension"`
	Unit      string `json:"unit"`
	Available int64  `json:"available"`
	Requested string `json:"requested,omitempty"`
	Verdict   string `json:"verdict"`
}

type storedCandidate struct {
	Schedule  string                 `json:"schedule"`
	Version   int                    `json:"version"`
	DepartsAt time.Time              `json:"departsAt"`
	ArrivesAt time.Time              `json:"arrivesAt"`
	Verdict   string                 `json:"verdict"`
	Pools     []storedPoolAssessment `json:"pools"`
}

// Save 落一份准备结果。同一身份已有一行时答已落过，不覆盖先到者。
func (repository *TransportPreparations) Save(
	ctx context.Context,
	record ports.TransportPreparationRecord,
) (ports.TransportPreparationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.TransportPreparationSaveOutcomeInvalid, fmt.Errorf("save transport preparation: %w", err)
	}
	preparation := record.Preparation
	demand, err := json.Marshal(demandRows(preparation.Demand()))
	if err != nil {
		return ports.TransportPreparationSaveOutcomeInvalid, fmt.Errorf("save transport preparation: %w", err)
	}
	candidates, err := json.Marshal(candidateRows(preparation.Candidates()))
	if err != nil {
		return ports.TransportPreparationSaveOutcomeInvalid, fmt.Errorf("save transport preparation: %w", err)
	}
	var anchor *string
	if value, present := preparation.Demand().Anchor(); present {
		anchor = &value
	}
	var selected *string
	if schedule, present := preparation.Selected(); present {
		value := schedule.String()
		selected = &value
	}
	leg := preparation.Leg()
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_preparation
			(tenant_id, planned_leg_ref, object_ref, rule_version,
			 connection_code, from_node, to_node, window_start, window_end,
			 demand_source, demand_anchor, demand, outcome, selected_schedule, candidates,
			 input_digest, judged_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Leg.String(),
		record.Key.Object.String(),
		record.Key.Rule.String(),
		leg.Direction().Connection().String(),
		leg.Direction().From().String(),
		leg.Direction().To().String(),
		leg.WindowStart().UTC(),
		leg.WindowEnd().UTC(),
		preparation.Demand().Source().String(),
		anchor,
		demand,
		preparation.Outcome().String(),
		selected,
		candidates,
		record.InputDigest,
		preparation.JudgedAt().UTC(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.TransportPreparationSaveOutcomeInvalid, fmt.Errorf("save transport preparation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.TransportPreparationAlreadyRecorded, nil
	}
	return ports.TransportPreparationSaved, nil
}

// FindByKey 按四维身份取回一份结果，经重建门读回：结论与候选依据推不到一处就答读不懂（包住
// ports.ErrTransportPreparationRecordUnreadable），不以坏行作答，也不冒充读不动。
func (repository *TransportPreparations) FindByKey(
	ctx context.Context,
	key ports.TransportPreparationKey,
) (ports.TransportPreparationRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.TransportPreparationRecord{}, false, fmt.Errorf("find transport preparation: %w", err)
	}
	var (
		connection, from, to, source, outcome, digest string
		anchor, selected                              *string
		windowStart, windowEnd, judgedAt, recordedAt  time.Time
		demand, candidates                            []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT connection_code, from_node, to_node, window_start, window_end,
		        demand_source, demand_anchor, demand, outcome, selected_schedule, candidates,
		        input_digest, judged_at, recorded_at
		   FROM transport_fulfillment.transport_preparation
		  WHERE tenant_id = $1
		    AND planned_leg_ref = $2
		    AND object_ref = $3
		    AND rule_version = $4`,
		key.TenantID.String(),
		key.Leg.String(),
		key.Object.String(),
		key.Rule.String(),
	).Scan(&connection, &from, &to, &windowStart, &windowEnd,
		&source, &anchor, &demand, &outcome, &selected, &candidates,
		&digest, &judgedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.TransportPreparationRecord{}, false, nil
	}
	if err != nil {
		return ports.TransportPreparationRecord{}, false, fmt.Errorf("find transport preparation: %w", err)
	}

	preparation, err := rehydratePreparation(key, preparationRow{
		connection: connection, from: from, to: to, windowStart: windowStart, windowEnd: windowEnd,
		source: source, anchor: anchor, demand: demand, outcome: outcome, selected: selected,
		candidates: candidates, judgedAt: judgedAt,
	})
	if err != nil {
		return ports.TransportPreparationRecord{}, false, fmt.Errorf("find transport preparation: %w: %w",
			ports.ErrTransportPreparationRecordUnreadable, err)
	}
	return ports.TransportPreparationRecord{
		Key:         key,
		InputDigest: digest,
		Preparation: preparation,
		RecordedAt:  recordedAt.UTC(),
	}, true, nil
}

type preparationRow struct {
	connection, from, to, source, outcome string
	anchor, selected                      *string
	windowStart, windowEnd, judgedAt      time.Time
	demand, candidates                    []byte
}

func rehydratePreparation(key ports.TransportPreparationKey, row preparationRow) (domain.TransportPreparation, error) {
	direction, err := domain.NewScheduleDirection(row.connection, row.from, row.to)
	if err != nil {
		return domain.TransportPreparation{}, err
	}
	leg, err := domain.NewPreparationLeg(key.Leg, direction, row.windowStart, row.windowEnd)
	if err != nil {
		return domain.TransportPreparation{}, err
	}
	source, err := domain.ParseDemandSource(row.source)
	if err != nil {
		return domain.TransportPreparation{}, err
	}
	var storedDemand []storedDemandQuantity
	if err := json.Unmarshal(row.demand, &storedDemand); err != nil {
		return domain.TransportPreparation{}, err
	}
	quantities := make([]domain.DemandQuantity, 0, len(storedDemand))
	for _, stored := range storedDemand {
		dimension, err := domain.ParseCapacityDimension(stored.Dimension)
		if err != nil {
			return domain.TransportPreparation{}, err
		}
		unit, err := domain.NewQuantityUnitReference(stored.Unit)
		if err != nil {
			return domain.TransportPreparation{}, err
		}
		quantities = append(quantities, domain.DemandQuantity{Dimension: dimension, Amount: stored.Amount, Unit: unit})
	}
	anchor := ""
	if row.anchor != nil {
		anchor = *row.anchor
	}
	demand, err := domain.RehydrateDemandSnapshot(source, anchor, quantities)
	if err != nil {
		return domain.TransportPreparation{}, err
	}
	outcome, err := domain.ParsePreparationOutcome(row.outcome)
	if err != nil {
		return domain.TransportPreparation{}, err
	}
	var selected domain.ScheduleReference
	if row.selected != nil {
		if selected, err = domain.NewScheduleReference(*row.selected); err != nil {
			return domain.TransportPreparation{}, err
		}
	}
	var storedCandidates []storedCandidate
	if err := json.Unmarshal(row.candidates, &storedCandidates); err != nil {
		return domain.TransportPreparation{}, err
	}
	candidates := make([]domain.PreparationCandidate, 0, len(storedCandidates))
	for _, stored := range storedCandidates {
		candidate, err := stored.candidate()
		if err != nil {
			return domain.TransportPreparation{}, err
		}
		candidates = append(candidates, candidate)
	}
	return domain.RehydrateTransportPreparation(domain.RehydrateTransportPreparationSpec{
		TenantID:   key.TenantID,
		Leg:        leg,
		Object:     key.Object,
		Rule:       key.Rule,
		Demand:     demand,
		Outcome:    outcome,
		Candidates: candidates,
		Selected:   selected,
		JudgedAt:   row.judgedAt,
	})
}

func (stored storedCandidate) candidate() (domain.PreparationCandidate, error) {
	schedule, err := domain.NewScheduleReference(stored.Schedule)
	if err != nil {
		return domain.PreparationCandidate{}, err
	}
	verdict, err := domain.ParseCandidateVerdict(stored.Verdict)
	if err != nil {
		return domain.PreparationCandidate{}, err
	}
	candidate := domain.PreparationCandidate{
		Schedule:  schedule,
		Version:   domain.ScheduleVersion(stored.Version),
		DepartsAt: stored.DepartsAt.UTC(),
		ArrivesAt: stored.ArrivesAt.UTC(),
		Verdict:   verdict,
	}
	for _, pool := range stored.Pools {
		reference, err := domain.NewCapacityPoolReference(pool.Pool)
		if err != nil {
			return domain.PreparationCandidate{}, err
		}
		dimension, err := domain.ParseCapacityDimension(pool.Dimension)
		if err != nil {
			return domain.PreparationCandidate{}, err
		}
		unit, err := domain.NewQuantityUnitReference(pool.Unit)
		if err != nil {
			return domain.PreparationCandidate{}, err
		}
		poolVerdict, err := domain.ParsePoolVerdict(pool.Verdict)
		if err != nil {
			return domain.PreparationCandidate{}, err
		}
		candidate.Pools = append(candidate.Pools, domain.PoolAssessment{
			Pool:      reference,
			Dimension: dimension,
			Unit:      unit,
			Available: pool.Available,
			Requested: pool.Requested,
			Verdict:   poolVerdict,
		})
	}
	return candidate, nil
}

func demandRows(demand domain.DemandSnapshot) []storedDemandQuantity {
	rows := make([]storedDemandQuantity, 0, len(demand.Quantities()))
	for _, quantity := range demand.Quantities() {
		rows = append(rows, storedDemandQuantity{
			Dimension: quantity.Dimension.String(),
			Amount:    quantity.Amount,
			Unit:      quantity.Unit.String(),
		})
	}
	return rows
}

func candidateRows(candidates []domain.PreparationCandidate) []storedCandidate {
	rows := make([]storedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		row := storedCandidate{
			Schedule:  candidate.Schedule.String(),
			Version:   int(candidate.Version),
			DepartsAt: candidate.DepartsAt.UTC(),
			ArrivesAt: candidate.ArrivesAt.UTC(),
			Verdict:   candidate.Verdict.String(),
			Pools:     make([]storedPoolAssessment, 0, len(candidate.Pools)),
		}
		for _, pool := range candidate.Pools {
			row.Pools = append(row.Pools, storedPoolAssessment{
				Pool:      pool.Pool.String(),
				Dimension: pool.Dimension.String(),
				Unit:      pool.Unit.String(),
				Available: pool.Available,
				Requested: pool.Requested,
				Verdict:   pool.Verdict.String(),
			})
		}
		rows = append(rows, row)
	}
	return rows
}
