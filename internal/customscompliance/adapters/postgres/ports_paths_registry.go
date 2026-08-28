package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// PortsPathsRegistrations 实现 ports.PortsPathsRegistry：口岸目录与申报路径目录两本
// 登记册的写口（0013 建表，票 admin-remainder-mechanism-batch/03）。
//
// 版本代数逐句照 InterpretationRuleRegistrations：一律不 UPSERT——撞键与撞重叠都由
// 无 conflict target 的 DO NOTHING 折成`已登记`交回，内容是否同一份由编排读回自己比；
// 唯一的 UPDATE 是换版给开放前版落终点，applies_from 与内容列没有任何改写路径，终点
// 只从 NULL 走到后继起点、只走一次。行锁串行化同支并发换版的机理与那边同一段推理，
// 不在此复述。
type PortsPathsRegistrations struct {
	db *bentopg.DB
}

func NewPortsPathsRegistrations(db *bentopg.DB) (*PortsPathsRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &PortsPathsRegistrations{db: db}, nil
}

var _ ports.PortsPathsRegistry = (*PortsPathsRegistrations)(nil)

// RegisterCandidatePort 登记一版口岸合规候选。起点在键上且无默认可言（timestamptz
// 装得下 0001 年，缺格会静默变成一个错的版本边界——判据同解释规则写口的零起点门）。
func (registry *PortsPathsRegistrations) RegisterCandidatePort(
	ctx context.Context,
	tenant domain.TenantID,
	port domain.CustomsPortReference,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	if appliesFrom.IsZero() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register candidate port: the applicable interval has no start")
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register candidate port: %w", err)
	}

	// 换版：给同支（租户，口岸）下起点更早的开放版落终点。先跑它才插得进后继——
	// 开放区间与任何更晚起点的登记在排他约束上相斥（0013 自注）。
	if _, err := executor.Exec(ctx,
		`UPDATE customs_compliance.candidate_port
		    SET applies_until = $3
		  WHERE tenant_id = $1 AND port_ref = $2
		    AND applies_until IS NULL AND applies_from < $3`,
		tenant.String(), port.String(), appliesFrom.UTC(),
	); err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register candidate port: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.candidate_port
			(tenant_id, port_ref, applies_from)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), port.String(), appliesFrom.UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register candidate port: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// RegisterDeclarationPath 登记一版申报路径。路径三维（口岸、方向、模式）由领域路由
// 值对象把门，这里只再拒零值路由——零值 route 的 String 各维皆空，落库会撞 CHECK，
// 而那属「依赖故障」的答案，它明明是调用方编程错误。
func (registry *PortsPathsRegistrations) RegisterDeclarationPath(
	ctx context.Context,
	tenant domain.TenantID,
	path domain.DeclarationPathReference,
	route domain.DeclarationPathRoute,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	directionText := route.Direction().String()
	if directionText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register declaration path: unknown manifest direction %d", route.Direction())
	}
	if appliesFrom.IsZero() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register declaration path: the applicable interval has no start")
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register declaration path: %w", err)
	}

	if _, err := executor.Exec(ctx,
		`UPDATE customs_compliance.declaration_path
		    SET applies_until = $3
		  WHERE tenant_id = $1 AND path_ref = $2
		    AND applies_until IS NULL AND applies_from < $3`,
		tenant.String(), path.String(), appliesFrom.UTC(),
	); err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register declaration path: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.declaration_path
			(tenant_id, path_ref, applies_from, port_ref, direction, declaration_mode)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), path.String(), appliesFrom.UTC(),
		route.Port().String(), directionText, route.Mode().String(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register declaration path: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}
