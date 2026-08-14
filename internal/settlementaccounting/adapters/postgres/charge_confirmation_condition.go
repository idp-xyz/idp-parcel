package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ConfirmationBasisAbsent 是确认条件已配置但要求的依据尚未到达时的缺口前缀。
//
// 缺口是续办入口而不是拒绝理由：条件配着、依据没到，等的是那份依据，不是等谁改配置。
// 前缀带上被要求的依据种类，续办方才知道该去取什么。
const ConfirmationBasisAbsent = "CONFIRMATION_BASIS_ABSENT/"

// RegisterOutcome 是本包各登记册的落点封闭代数（ADR-0031）：同键已有行是重放，绝不
// 覆盖。改配置是另一件事——谁有权改、原值留不留痕都还没定，而一个静默覆盖的登记面
// 会让那个问题永远不必回答。
//
// 确认条件、确认依据与分摊规则适用共用它：三者不是碰巧长得像，它们在持久化面就是
// 同一条代数（登记一次、重放同答、不覆盖）。随外部事实推进的余额与信用状况不在此列，
// 那两个允许同键往前推进，用的是 PositionRecordOutcome。
type RegisterOutcome uint8

const (
	RegisterOutcomeInvalid RegisterOutcome = iota
	Registered
	AlreadyRegistered
)

func (outcome RegisterOutcome) String() string {
	switch outcome {
	case Registered:
		return "REGISTERED"
	case AlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// ChargeConfirmationConditions 实现 ports.ConfirmationConditionView，并承载它读的
// 两张表的登记面。
//
// 核对是两张表的交集：目录说这类费用要什么依据，依据登记说这笔费用到了什么依据。
// 适配器自己不产生第三种成立路径——`已满足`只可能来自一条真实登记的依据，配置里
// 没有任何一个字段能直接写成`已满足`。这正是 UC-SA-002 禁止的`默认转正`要堵的口。
type ChargeConfirmationConditions struct {
	db *bentopg.DB
}

func NewChargeConfirmationConditions(db *bentopg.DB) (*ChargeConfirmationConditions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ChargeConfirmationConditions{db: db}, nil
}

var _ ports.ConfirmationConditionView = (*ChargeConfirmationConditions)(nil)

// LoadConfirmationCondition 核对该费用类型的确认条件。
//
// 三种答案分得开：目录无行是 found=false（实例半边未配置，编排停在未决）；配了而
// 要求的依据没到是`未满足`带缺口；依据到了才是`已满足`带依据。到达的若是另一种依据，
// 不顶替要求的那种——那样的`已满足`是拿一份不相干的证据放行。
func (repository *ChargeConfirmationConditions) LoadConfirmationCondition(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
	feeItem domain.FeeItemReference,
) (ports.ConfirmationCondition, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ConfirmationCondition{}, false, fmt.Errorf("load confirmation condition: %w", err)
	}

	var requiredKind string
	err = querier.QueryRow(ctx,
		`SELECT required_basis_kind
		   FROM settlement_accounting.charge_confirmation_condition
		  WHERE tenant_id = $1
		    AND fee_item = $2`,
		tenant.String(),
		feeItem.String(),
	).Scan(&requiredKind)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ConfirmationCondition{}, false, nil
	}
	if err != nil {
		return ports.ConfirmationCondition{}, false, fmt.Errorf("load confirmation condition: %w", err)
	}

	var basisRef string
	err = querier.QueryRow(ctx,
		`SELECT basis_ref
		   FROM settlement_accounting.charge_confirmation_basis
		  WHERE tenant_id = $1
		    AND charge_id = $2
		    AND fee_item = $3
		    AND basis_kind = $4`,
		tenant.String(),
		charge.String(),
		feeItem.String(),
		requiredKind,
	).Scan(&basisRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ConfirmationCondition{
			Met: false,
			Gap: ConfirmationBasisAbsent + requiredKind,
		}, true, nil
	}
	if err != nil {
		return ports.ConfirmationCondition{}, false, fmt.Errorf("load confirmation condition: %w", err)
	}

	basis, err := domain.NewConfirmationBasisReference(basisRef)
	if err != nil {
		return ports.ConfirmationCondition{}, false, fmt.Errorf("load confirmation condition: %w", err)
	}
	return ports.ConfirmationCondition{Met: true, Basis: basis}, true, nil
}

// RegisterCondition 登记「这类费用的确认条件要哪种依据」。内容属实例半边——没有租户
// 时这张表是空的，视图据以答`未配置`。
func (repository *ChargeConfirmationConditions) RegisterCondition(
	ctx context.Context,
	tenant domain.TenantID,
	feeItem domain.FeeItemReference,
	requiredBasisKind string,
	registeredAt time.Time,
) (RegisterOutcome, error) {
	if strings.TrimSpace(requiredBasisKind) == "" {
		return RegisterOutcomeInvalid, errors.New(
			"register confirmation condition: 确认条件必须点名要求的依据种类")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return RegisterOutcomeInvalid, fmt.Errorf("register confirmation condition: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.charge_confirmation_condition
			(tenant_id, fee_item, required_basis_kind, registered_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		feeItem.String(),
		requiredBasisKind,
		registeredAt.UTC(),
	)
	if err != nil {
		return RegisterOutcomeInvalid, fmt.Errorf("register confirmation condition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AlreadyRegistered, nil
	}
	return Registered, nil
}

// RecordBasis 登记一笔费用到达的确认依据。依据种类进键：同一笔费用可以到达多种依据，
// 核对只认目录点名的那一种。
func (repository *ChargeConfirmationConditions) RecordBasis(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
	feeItem domain.FeeItemReference,
	basisKind string,
	basis domain.ConfirmationBasisReference,
	recordedAt time.Time,
) (RegisterOutcome, error) {
	if strings.TrimSpace(basisKind) == "" {
		return RegisterOutcomeInvalid, errors.New(
			"record confirmation basis: 依据必须带种类，否则核对无从判断它满足的是哪条条件")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return RegisterOutcomeInvalid, fmt.Errorf("record confirmation basis: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.charge_confirmation_basis
			(tenant_id, charge_id, fee_item, basis_kind, basis_ref, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		charge.String(),
		feeItem.String(),
		basisKind,
		basis.String(),
		recordedAt.UTC(),
	)
	if err != nil {
		return RegisterOutcomeInvalid, fmt.Errorf("record confirmation basis: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AlreadyRegistered, nil
	}
	return Registered, nil
}
