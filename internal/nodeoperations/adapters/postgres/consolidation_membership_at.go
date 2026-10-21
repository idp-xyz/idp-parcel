package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ConsolidationMembershipsAt 实现 ports.ConsolidationMembershipAtView：按（租户，对象，时点）答这件对象是不是集运单元、那一刻直接
// 装着哪几件（ADR-0225）。只读，走 ReadExecutor。
//
// 成员取自集运作业的来源事实层（consolidation_fact，迁移 0003），不取单元行的 members 列、也不取容纳索引：那两样是此刻的派生状态，
// 答不出「那一刻」。每件成员取它在这一单元上业务发生时间不晚于时点的最后一条移入或移出，是移入就在；同一刻的两条按记录时刻、
// 再按来源标识定序——单元的状态推进按到达次序一步一步落，同刻先后只有记录时刻说得出，来源标识只为让同参必得同答。走
// consolidation_fact_by_unit（租户，单元，业务时间）。
//
// 时点之前已经落了关闭事实的实例那一刻不装任何东西：关闭只在成员全部移出或剩余成员已经处置转移之后才落（CONTEXT「集运单元实例」），
// 留在实例行上的处置成员已经不在袋里。
type ConsolidationMembershipsAt struct {
	db *bentopg.DB
}

func NewConsolidationMembershipsAt(db *bentopg.DB) (*ConsolidationMembershipsAt, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ConsolidationMembershipsAt{db: db}, nil
}

var _ ports.ConsolidationMembershipAtView = (*ConsolidationMembershipsAt)(nil)

var errBlankMembershipObject = errors.New("node operations postgres: the object asked about is blank")

func (view *ConsolidationMembershipsAt) ReadConsolidationMembershipAt(
	ctx context.Context,
	tenant domain.TenantID,
	object string,
	at time.Time,
) (ports.ConsolidationMembershipAt, error) {
	none := ports.ConsolidationMembershipAt{}
	if strings.TrimSpace(object) == "" {
		return none, fmt.Errorf("read consolidation membership: %w", errBlankMembershipObject)
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("read consolidation membership: %w", err)
	}

	var unit, closed bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (
		            SELECT 1 FROM node_operations.consolidation_unit WHERE tenant_id = $1 AND unit_id = $2),
		        EXISTS (
		            SELECT 1 FROM node_operations.consolidation_fact
		             WHERE tenant_id = $1 AND unit_id = $2 AND action = $3 AND occurred_at <= $4)`,
		tenant.String(), object, domain.CloseUnitAction.String(), at,
	).Scan(&unit, &closed); err != nil {
		return none, fmt.Errorf("read consolidation membership: %w", err)
	}
	if !unit {
		return none, nil
	}
	membership := ports.ConsolidationMembershipAt{Unit: true, Members: []ports.ConsolidationMemberAt{}}
	if closed {
		return membership, nil
	}

	rows, err := querier.Query(ctx,
		`SELECT latest.member_id,
		        EXISTS (
		            SELECT 1 FROM node_operations.consolidation_unit AS nested
		             WHERE nested.tenant_id = $1 AND nested.unit_id = latest.member_id)
		   FROM (
		         SELECT DISTINCT ON (fact.member_id) fact.member_id, fact.action
		           FROM node_operations.consolidation_fact AS fact
		          WHERE fact.tenant_id = $1
		            AND fact.unit_id = $2
		            AND fact.action IN ($3, $4)
		            AND fact.occurred_at <= $5
		          ORDER BY fact.member_id, fact.occurred_at DESC, fact.recorded_at DESC, fact.source_id DESC
		        ) AS latest
		  WHERE latest.action = $3
		  ORDER BY latest.member_id`,
		tenant.String(), object, domain.AddMemberAction.String(), domain.RemoveMemberAction.String(), at,
	)
	if err != nil {
		return none, fmt.Errorf("read consolidation membership: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var nested bool
		if err := rows.Scan(&raw, &nested); err != nil {
			return none, fmt.Errorf("read consolidation membership: %w", err)
		}
		member, err := domain.NewHandlingUnitID(raw)
		if err != nil {
			return none, fmt.Errorf("read consolidation membership: member: %w", err)
		}
		membership.Members = append(membership.Members, ports.ConsolidationMemberAt{Member: member, NestedUnit: nested})
	}
	if err := rows.Err(); err != nil {
		return none, fmt.Errorf("read consolidation membership: %w", err)
	}
	return membership, nil
}
