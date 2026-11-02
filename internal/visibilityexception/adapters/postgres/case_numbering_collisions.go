package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// CaseNumberingCollisions 实现案件撞号册（ports.CaseNumberingCollisionRecorder 与 ports.CaseNumberingCollisionQueue），落 VE
// 迁移 0033 的 case_numbering_collision。写口一律要事务：记撞号与开立事务同一笔，重驱的推后与销账与那一次分诊同一笔。
type CaseNumberingCollisions struct {
	db *bentopg.DB
}

var (
	_ ports.CaseNumberingCollisionRecorder = (*CaseNumberingCollisions)(nil)
	_ ports.CaseNumberingCollisionQueue    = (*CaseNumberingCollisions)(nil)
)

func NewCaseNumberingCollisions(db *bentopg.DB) (*CaseNumberingCollisions, error) {
	if db == nil {
		return nil, errors.New("visibility exception postgres: db is nil")
	}
	return &CaseNumberingCollisions{db: db}, nil
}

// RecordCaseNumberingCollision 记下或覆盖一次撞号：命中各样换成最近这一次，次数加一，并让它此刻到期——下一拍重驱就再试一回，
// 撞着的那一刻派发日志里看得见。
func (book *CaseNumberingCollisions) RecordCaseNumberingCollision(ctx context.Context, collision ports.CaseNumberingCollision) error {
	executor, err := book.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record case numbering collision: %w", err)
	}
	hit := collision.Hit
	if collision.Tenant.String() == "" || !hit.Complete() || collision.Template == "" || collision.At.IsZero() {
		return errors.New("record case numbering collision: the collision is incomplete")
	}
	basis, err := signalFactBasisToJSON(hit.Basis)
	if err != nil {
		return fmt.Errorf("record case numbering collision: fact basis: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.case_numbering_collision
			(tenant_id, parcel_ref, kind_ref, causal_condition, rule_ref, confidence_ref, fact_basis, hit_at,
			 template, first_collided_at, last_collided_at, collisions, next_attempt_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, 1, $10)
		 ON CONFLICT (tenant_id, parcel_ref, kind_ref, causal_condition) DO UPDATE SET
			rule_ref = EXCLUDED.rule_ref,
			confidence_ref = EXCLUDED.confidence_ref,
			fact_basis = EXCLUDED.fact_basis,
			hit_at = EXCLUDED.hit_at,
			template = EXCLUDED.template,
			last_collided_at = GREATEST(case_numbering_collision.last_collided_at, EXCLUDED.last_collided_at),
			collisions = case_numbering_collision.collisions + 1,
			next_attempt_at = EXCLUDED.next_attempt_at`,
		collision.Tenant.String(), hit.Parcel.String(), hit.Kind.String(), hit.Cause.String(),
		hit.Rule.String(), hit.Confidence.String(), basis, hit.At.UTC(),
		collision.Template, collision.At.UTC(),
	); err != nil {
		return fmt.Errorf("record case numbering collision: %w", err)
	}
	return nil
}

// DueCaseNumberingCollisions 交回 at 时刻到期的撞号命中，最早到期的在前，最多 limit 件。跨租户取：重驱在派发进程里统一跑，
// 每一行带着自己的租户，交分诊时按它绑规则。读回经 SignalHit.Complete 把门，一行坏数据不会冒充一次命中。
func (book *CaseNumberingCollisions) DueCaseNumberingCollisions(ctx context.Context, at time.Time, limit int) ([]ports.CaseNumberingCollision, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("due case numbering collisions: limit must be positive, got %d", limit)
	}
	querier, err := book.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("due case numbering collisions: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT tenant_id, parcel_ref, kind_ref, causal_condition, rule_ref, confidence_ref, fact_basis, hit_at,
		        template, last_collided_at
		   FROM visibility_exception.case_numbering_collision
		  WHERE next_attempt_at <= $1
		  ORDER BY next_attempt_at, tenant_id, parcel_ref, kind_ref, causal_condition
		  LIMIT $2`,
		at.UTC(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("due case numbering collisions: %w", err)
	}
	defer rows.Close()
	var due []ports.CaseNumberingCollision
	for rows.Next() {
		var (
			tenant, parcel, kind, cause, rule, confidence, template string
			basisJSON                                               []byte
			hitAt, collidedAt                                       time.Time
		)
		if err := rows.Scan(&tenant, &parcel, &kind, &cause, &rule, &confidence, &basisJSON, &hitAt, &template, &collidedAt); err != nil {
			return nil, fmt.Errorf("due case numbering collisions: %w", err)
		}
		collision, err := rebuildCaseNumberingCollision(tenant, parcel, kind, cause, rule, confidence, basisJSON, hitAt, template, collidedAt)
		if err != nil {
			return nil, fmt.Errorf("due case numbering collisions: %s/%s: %w", tenant, parcel, err)
		}
		due = append(due, collision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("due case numbering collisions: %w", err)
	}
	return due, nil
}

func rebuildCaseNumberingCollision(
	tenant, parcel, kind, cause, rule, confidence string,
	basisJSON []byte,
	hitAt time.Time,
	template string,
	collidedAt time.Time,
) (ports.CaseNumberingCollision, error) {
	var (
		collision ports.CaseNumberingCollision
		err       error
	)
	if collision.Tenant, err = domain.NewTenantID(tenant); err != nil {
		return collision, err
	}
	if collision.Hit.Parcel, err = domain.NewTrackedParcelReference(parcel); err != nil {
		return collision, err
	}
	if collision.Hit.Kind, err = domain.NewExceptionSignalKindReference(kind); err != nil {
		return collision, err
	}
	if collision.Hit.Cause, err = domain.NewCausalConditionReference(cause); err != nil {
		return collision, err
	}
	if collision.Hit.Rule, err = domain.NewSignalRuleVersionReference(rule); err != nil {
		return collision, err
	}
	if collision.Hit.Confidence, err = domain.NewConfidenceReference(confidence); err != nil {
		return collision, err
	}
	if collision.Hit.Basis, err = signalFactBasisFromJSON(basisJSON); err != nil {
		return collision, err
	}
	collision.Hit.At = hitAt.UTC()
	if !collision.Hit.Complete() {
		return collision, errors.New("the recorded hit is incomplete")
	}
	collision.Template = template
	collision.At = collidedAt.UTC()
	return collision, nil
}

// PostponeCaseNumberingCollision 把一件撞号命中推后到 until。行不在了不算错：同一原因的发作期在这期间落了，那一行已随之销掉。
func (book *CaseNumberingCollisions) PostponeCaseNumberingCollision(ctx context.Context, key ports.CaseNumberingCollisionKey, until time.Time) error {
	executor, err := book.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("postpone case numbering collision: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE visibility_exception.case_numbering_collision SET next_attempt_at = $5
		  WHERE tenant_id = $1 AND parcel_ref = $2 AND kind_ref = $3 AND causal_condition = $4`,
		key.Tenant.String(), key.Parcel.String(), key.Kind.String(), key.Cause.String(), until.UTC(),
	); err != nil {
		return fmt.Errorf("postpone case numbering collision: %w", err)
	}
	return nil
}

// ResolveCaseNumberingCollision 销掉一件撞号命中；行不在了同样不算错。
func (book *CaseNumberingCollisions) ResolveCaseNumberingCollision(ctx context.Context, key ports.CaseNumberingCollisionKey) error {
	executor, err := book.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("resolve case numbering collision: %w", err)
	}
	return resolveCaseNumberingCollision(ctx, executor, key)
}

// resolveCaseNumberingCollision 是销账那一条语句，SaveRaised 落发作期时同一笔调它：这一原因有了发作期，撞号那一次就算分诊过了。
func resolveCaseNumberingCollision(ctx context.Context, executor bentopg.Executor, key ports.CaseNumberingCollisionKey) error {
	if _, err := executor.Exec(ctx,
		`DELETE FROM visibility_exception.case_numbering_collision
		  WHERE tenant_id = $1 AND parcel_ref = $2 AND kind_ref = $3 AND causal_condition = $4`,
		key.Tenant.String(), key.Parcel.String(), key.Kind.String(), key.Cause.String(),
	); err != nil {
		return fmt.Errorf("resolve case numbering collision: %w", err)
	}
	return nil
}
