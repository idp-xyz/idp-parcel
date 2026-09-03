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

// LoadAssignments 实现 ports.LoadAssignmentRegistry：版本一行，成员逐对象一行。
//
// **只插不改。** 变化与撤回在领域里各自形成新版本，库面跟上的方式就是再插一条版本行——本类型
// 上没有任何 UPDATE，所以「修改分配历史」在这里写不出来。
type LoadAssignments struct {
	db *bentopg.DB
}

func NewLoadAssignments(db *bentopg.DB) (*LoadAssignments, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &LoadAssignments{db: db}, nil
}

var _ ports.LoadAssignmentRegistry = (*LoadAssignments)(nil)

// FindByKey 按（租户+分配+版本）取回一个版本连同它的对象范围。否定结果只回 false。
func (repository *LoadAssignments) FindByKey(
	ctx context.Context,
	key ports.LoadAssignmentKey,
) (ports.LoadAssignmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
	}

	var scheduleRef string
	var assignedAt, recordedAt time.Time
	var correctsVersion *string
	var revisedAt, withdrawnAt *time.Time
	var withdrawn bool
	err = querier.QueryRow(ctx,
		`SELECT schedule_ref, assigned_at, corrects_version, revised_at,
		        withdrawn, withdrawn_at, recorded_at
		   FROM transport_fulfillment.load_assignment
		  WHERE tenant_id = $1 AND assignment_ref = $2 AND version = $3`,
		key.TenantID.String(), key.Assignment.String(), key.Version.String(),
	).Scan(&scheduleRef, &assignedAt, &correctsVersion, &revisedAt,
		&withdrawn, &withdrawnAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.LoadAssignmentRecord{}, false, nil
	}
	if err != nil {
		return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
	}

	// 成员按引用排序：顺序不进任何判断，但让读回稳定。
	rows, err := querier.Query(ctx,
		`SELECT object_ref
		   FROM transport_fulfillment.load_assignment_member
		  WHERE tenant_id = $1 AND assignment_ref = $2 AND version = $3
		  ORDER BY object_ref`,
		key.TenantID.String(), key.Assignment.String(), key.Version.String(),
	)
	if err != nil {
		return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
	}
	defer rows.Close()

	spec := domain.RehydrateLoadAssignmentSpec{
		TenantID:   key.TenantID,
		Assignment: key.Assignment,
		Version:    key.Version,
		AssignedAt: assignedAt.UTC(),
		Withdrawn:  withdrawn,
	}
	// 逐列走各自的构造门装回，不按列直接拼结构体——构造门是坏行的第一道拦截。
	if spec.Schedule, err = domain.NewScheduleReference(scheduleRef); err != nil {
		return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
	}
	if correctsVersion != nil {
		if spec.Corrects, err = domain.NewLoadAssignmentVersion(*correctsVersion); err != nil {
			return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
		}
	}
	if revisedAt != nil {
		spec.RevisedAt = revisedAt.UTC()
	}
	if withdrawnAt != nil {
		spec.WithdrawnAt = withdrawnAt.UTC()
	}

	for rows.Next() {
		var objectRef string
		if err := rows.Scan(&objectRef); err != nil {
			return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
		}
		member, err := domain.NewCarriedObjectReference(objectRef)
		if err != nil {
			return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
		}
		spec.Members = append(spec.Members, member)
	}
	if err := rows.Err(); err != nil {
		return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
	}

	assignment, err := domain.RehydrateLoadAssignment(spec)
	if err != nil {
		return ports.LoadAssignmentRecord{}, false, fmt.Errorf("find load assignment: %w", err)
	}
	return ports.LoadAssignmentRecord{Key: key, Assignment: assignment, RecordedAt: recordedAt.UTC()}, true, nil
}

// Save 登记一个版本连同它的对象范围。撞键答`已登记`（ADR-0031），编排据此读回原版本。
//
// 撞键只看版本那一行：成员挂在它下面，版本已在册就说明这一整版已经有人写过。
func (repository *LoadAssignments) Save(
	ctx context.Context,
	record ports.LoadAssignmentRecord,
) (ports.LoadAssignmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.LoadAssignmentSaveOutcomeInvalid, fmt.Errorf("save load assignment: %w", err)
	}
	if err := assertLoadAssignmentKeyAgrees(record); err != nil {
		return ports.LoadAssignmentSaveOutcomeInvalid, fmt.Errorf("save load assignment: %w", err)
	}

	var correctsText *string
	if corrects, has := record.Assignment.Corrects(); has {
		text := corrects.String()
		correctsText = &text
	}
	withdrawnAt, withdrawn := record.Assignment.Withdrawn()
	revisedAt, revised := record.Assignment.RevisedAt()
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.load_assignment
		     (tenant_id, assignment_ref, version, schedule_ref, assigned_at,
		      corrects_version, revised_at, withdrawn, withdrawn_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Assignment.String(),
		record.Key.Version.String(),
		record.Assignment.Schedule().String(),
		record.Assignment.AssignedAt().UTC(),
		correctsText,
		nullableTime(revisedAt, revised),
		withdrawn,
		nullableTime(withdrawnAt, withdrawn),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.LoadAssignmentSaveOutcomeInvalid, fmt.Errorf("save load assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.LoadAssignmentVersionAlreadyRegistered, nil
	}

	for _, member := range record.Assignment.Members() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.load_assignment_member
			     (tenant_id, assignment_ref, version, object_ref, recorded_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			record.Key.TenantID.String(),
			record.Key.Assignment.String(),
			record.Key.Version.String(),
			member.String(),
			record.RecordedAt.UTC(),
		); err != nil {
			return ports.LoadAssignmentSaveOutcomeInvalid, fmt.Errorf("save load assignment: %w", err)
		}
	}
	return ports.LoadAssignmentSaved, nil
}

// assertLoadAssignmentKeyAgrees 挡住「键说的是一个版本、聚合说的是另一个」那种写入。
func assertLoadAssignmentKeyAgrees(record ports.LoadAssignmentRecord) error {
	if record.Key.TenantID != record.Assignment.TenantID() ||
		record.Key.Assignment != record.Assignment.Assignment() ||
		record.Key.Version != record.Assignment.Version() {
		return fmt.Errorf("load assignment key disagrees with the aggregate")
	}
	return nil
}
