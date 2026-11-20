package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是运输准备两口的库面（迁移 0028，ADR-0201 决定六至八）：准备结果库，与按班次成批读池的那一问——装载分配锁池行，
// 准备评估不锁（票 demo-chain-gaps/127）。

var (
	_ ports.CapacityPoolsOnSchedules         = (*CapacityPools)(nil)
	_ ports.UnlockedCapacityPoolsOnSchedules = (*CapacityPools)(nil)
)

// ListPoolsOnSchedules 按班次交回登记册上的池，逐个锁住池行（装载分配那一口）。先取池身份再逐个按池身份读回：读回要过
// 重建门、核池身份由班次与维度派生，按班次读到的池与登记口、预占口读到的是同一个，没有第二条读法。按池身份的次序逐个
// 加锁，两笔事务锁同一组池时不会互等成环。
func (repository *CapacityPools) ListPoolsOnSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	schedules []domain.ScheduleReference,
) ([]ports.CapacityPoolRecord, error) {
	return repository.listPoolsOnSchedules(ctx, tenant, schedules, true)
}

// ListPoolsOnSchedulesUnlocked 同 ListPoolsOnSchedules，只是不锁池行（准备评估那一口，见 ports.UnlockedCapacityPoolsOnSchedules）。
//
// 一个池分几条语句读，读已提交下每条各取各的快照：两条之间恰有一笔释放提交时，释放明细与
// 预占上的已释放对不上，重建门拒读，准备答池册读不动、整封重投，下一次读到的是那一笔之后的池。不拼一条语句去换一个
// 一致的快照：那就成了第二条读法，而这一口要的正是与写编排读到同一个池。
func (repository *CapacityPools) ListPoolsOnSchedulesUnlocked(
	ctx context.Context,
	tenant domain.TenantID,
	schedules []domain.ScheduleReference,
) ([]ports.CapacityPoolRecord, error) {
	return repository.listPoolsOnSchedules(ctx, tenant, schedules, false)
}

func (repository *CapacityPools) listPoolsOnSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	schedules []domain.ScheduleReference,
	lock bool,
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
		record, found, err := repository.findPool(ctx, ports.CapacityPoolKey{TenantID: tenant, Pool: reference}, lock)
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
	source, measurement, err := preparationIdentity(record.Key)
	if err != nil {
		return ports.TransportPreparationSaveOutcomeInvalid, fmt.Errorf("save transport preparation: %w", err)
	}
	if preparation.Demand().Source() != record.Key.Source {
		return ports.TransportPreparationSaveOutcomeInvalid, fmt.Errorf(
			"save transport preparation: demand source %s disagrees with the key", preparation.Demand().Source())
	}
	if record.Key.Source == domain.DemandFromActualMeasurement {
		anchored, present := preparation.Demand().Anchor()
		if !present || anchored != measurement {
			return ports.TransportPreparationSaveOutcomeInvalid, fmt.Errorf(
				"save transport preparation: measured demand anchor disagrees with the measurement reference")
		}
	}
	leg := preparation.Leg()
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_preparation
			(tenant_id, planned_leg_ref, object_ref, rule_version,
			 connection_code, from_node, to_node, window_start, window_end,
			 demand_source, demand_anchor, measurement_ref, demand, outcome, selected_schedule, candidates,
			 input_digest, judged_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
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
		source,
		anchor,
		measurement,
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

// FindByKey 按身份取回一份结果，经重建门读回：结论与候选依据推不到一处就答读不懂（包住
// ports.ErrTransportPreparationRecordUnreadable），不以坏行作答，也不冒充读不动。来源不在封闭词里时直接拒，
// 不把零值来源当成申报去读。
func (repository *TransportPreparations) FindByKey(
	ctx context.Context,
	key ports.TransportPreparationKey,
) (ports.TransportPreparationRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.TransportPreparationRecord{}, false, fmt.Errorf("find transport preparation: %w", err)
	}
	sourceName, measurement, err := preparationIdentity(key)
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
		    AND rule_version = $4
		    AND demand_source = $5
		    AND measurement_ref = $6`,
		key.TenantID.String(),
		key.Leg.String(),
		key.Object.String(),
		key.Rule.String(),
		sourceName,
		measurement,
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

// HoldPreparationObject 把（租户、载运对象）收成一把事务级咨询锁。第一键是锁空间名的散列，第二键是两维以 chr(31) 拼接后的
// 散列，与本仓其余事务级锁同一个双 int 形式。哈希撞上只让两组不相干的对象多等一会儿。锁随事务释放；不在事务里调用由
// RequireExecutor 拒，否则这一把锁根本拿不住到提交。
func (repository *TransportPreparations) HoldPreparationObject(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("hold transport preparation object: %w", err)
	}
	if strings.TrimSpace(tenant.String()) == "" || strings.TrimSpace(object.String()) == "" {
		return fmt.Errorf("hold transport preparation object: tenant or object is blank")
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('transport_fulfillment.transport_preparation.object'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), object.String(),
	); err != nil {
		return fmt.Errorf("取准备对象锁：%w", err)
	}
	return nil
}

// ListDeclaredPreparationLegs 只列申报来源的段。重建不过门的行包住读不懂：坏行重读不会变好，编排不拿它当读不动去重投。
func (repository *TransportPreparations) ListDeclaredPreparationLegs(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	rule domain.PreparationRuleVersion,
) ([]domain.PreparationLeg, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list declared preparation legs: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT planned_leg_ref, connection_code, from_node, to_node, window_start, window_end
		   FROM transport_fulfillment.transport_preparation
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND rule_version = $3
		    AND demand_source = $4
		  ORDER BY planned_leg_ref`,
		tenant.String(),
		object.String(),
		rule.String(),
		domain.DemandFromDeclaration.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list declared preparation legs: %w", err)
	}
	defer rows.Close()
	var legs []domain.PreparationLeg
	for rows.Next() {
		var reference, connection, from, to string
		var windowStart, windowEnd time.Time
		if err := rows.Scan(&reference, &connection, &from, &to, &windowStart, &windowEnd); err != nil {
			return nil, fmt.Errorf("list declared preparation legs: %w", err)
		}
		leg, err := declaredPreparationLeg(reference, connection, from, to, windowStart, windowEnd)
		if err != nil {
			return nil, fmt.Errorf("list declared preparation legs: %w: %w",
				ports.ErrTransportPreparationRecordUnreadable, err)
		}
		legs = append(legs, leg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list declared preparation legs: %w", err)
	}
	return legs, nil
}

func declaredPreparationLeg(
	reference, connection, from, to string,
	windowStart, windowEnd time.Time,
) (domain.PreparationLeg, error) {
	legRef, err := domain.NewPlannedSegmentReference(reference)
	if err != nil {
		return domain.PreparationLeg{}, err
	}
	direction, err := domain.NewScheduleDirection(connection, from, to)
	if err != nil {
		return domain.PreparationLeg{}, err
	}
	return domain.NewPreparationLeg(legRef, direction, windowStart, windowEnd)
}

// preparationIdentity 把键上的来源与实测引用收成库里那两列。零值来源的 String 是空串，ParseDemandSource 认不回，
// 于是不会被当成申报去读写。申报必须不带引用；实测引用去空白后仍须与原串相同，空白串过不了库上的 btrim 检查。
func preparationIdentity(key ports.TransportPreparationKey) (source, measurement string, err error) {
	parsed, err := domain.ParseDemandSource(key.Source.String())
	if err != nil || parsed != key.Source {
		return "", "", fmt.Errorf("demand source is not admitted")
	}
	switch key.Source {
	case domain.DemandFromDeclaration:
		if key.MeasurementRef != "" {
			return "", "", fmt.Errorf("declaration preparation carries a measurement reference")
		}
	case domain.DemandFromActualMeasurement:
		if strings.TrimSpace(key.MeasurementRef) == "" || strings.TrimSpace(key.MeasurementRef) != key.MeasurementRef {
			return "", "", fmt.Errorf("measured preparation needs the measurement reference")
		}
	}
	return key.Source.String(), key.MeasurementRef, nil
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
