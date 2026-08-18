package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// PreAcceptanceControlDeclarations 实现 ports.PreAcceptanceControlDeclarationView：
// 按已唯一选出的客户合同版本，取回它对「要不要接受前财务控制」的声明。
//
// 只读。声明正文属实例半边（`PAR-COM-15` 待提供），本适配器不提供写口，读不到时也不
// 代拟任何一格——缺声明既不是`要求控制`也不是`不适用`。控制方式与采用政策不在本口
// （ADR-0044），两层压成一层就会从结算方式倒推控制要不要。
type PreAcceptanceControlDeclarations struct {
	db *bentopg.DB
}

func NewPreAcceptanceControlDeclarations(db *bentopg.DB) (*PreAcceptanceControlDeclarations, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &PreAcceptanceControlDeclarations{db: db}, nil
}

var _ ports.PreAcceptanceControlDeclarationView = (*PreAcceptanceControlDeclarations)(nil)

// LoadPreAcceptanceControl 取回合同版本的「要不要」声明。
//
// found=false = **实例未配置**：CONTEXT 禁止用缺失结果代替「明确无控制」，所以查无行
// 时不拼一份`不适用`交出去。读取失败（error）是另一格，等依赖恢复——两格合成一格，
// 运维分不清该催登记还是该重试（ADR-0054 / ADR-0029）。
func (repository *PreAcceptanceControlDeclarations) LoadPreAcceptanceControl(
	ctx context.Context,
	tenant domain.TenantID,
	contract domain.CommercialVersion,
) (domain.PreAcceptanceControlDeclaration, bool, error) {
	none := domain.PreAcceptanceControlDeclaration{}
	if tenant.String() == "" ||
		contract.ObjectID().String() == "" || contract.Version().String() == "" {
		return none, false, fmt.Errorf("load pre-acceptance control: tenant and contract identity are required")
	}
	// 显式租户与合同对象必须是同一个身份：按租户查库、按合同重建，两处各写各的就会
	// 把 A 的行装进 B 的合同（ADR-0003/0040，租户是身份不是过滤器）。
	if tenant != contract.Tenant() {
		return none, false, fmt.Errorf("load pre-acceptance control: tenant does not own this contract")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance control: %w", err)
	}

	var requirementName string
	var basis *string
	err = querier.QueryRow(ctx,
		`SELECT requirement, not_applicable_basis
		   FROM party_commercial.pre_acceptance_control_declaration
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.CustomerContractObject),
		contract.ObjectID().String(),
		contract.Version().String(),
	).Scan(&requirementName, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance control: %w", err)
	}

	requirement, err := preAcceptanceControlRequirementFrom(requirementName)
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance control: %w", err)
	}
	notApplicable, err := notApplicableBasisFrom(requirement, basis)
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance control: %w", err)
	}

	// 重建走同一扇构造门：库里读出的取值同样得过「`不适用`必须带依据、`要求`不得带」。
	// 坏行在这里点名，不把它折成 found=false——那会把一份损坏的声明伪装成从未登记。
	declaration, err := domain.DeclarePreAcceptanceControl(contract, requirement, notApplicable)
	if err != nil {
		return none, false, fmt.Errorf("load pre-acceptance control: %w", err)
	}
	return declaration, true, nil
}

// preAcceptanceControlRequirementFrom 逐格翻译，default 报错不吸收：库内 CHECK 只放行
// 已声明两值，出现第三个取值说明两处封闭集已分叉，那要人来看。
func preAcceptanceControlRequirementFrom(raw string) (domain.PreAcceptanceControlRequirement, error) {
	switch raw {
	case domain.PreAcceptanceControlRequired.String():
		return domain.PreAcceptanceControlRequired, nil
	case domain.PreAcceptanceControlNotApplicable.String():
		return domain.PreAcceptanceControlNotApplicable, nil
	default:
		return domain.PreAcceptanceControlUndeclared, fmt.Errorf("unknown pre-acceptance control requirement %q", raw)
	}
}

func notApplicableBasisFrom(
	requirement domain.PreAcceptanceControlRequirement,
	raw *string,
) (domain.ControlNotApplicableBasis, error) {
	if requirement != domain.PreAcceptanceControlNotApplicable {
		if raw != nil {
			return domain.ControlNotApplicableBasis{}, fmt.Errorf(
				"required control carried a not-applicable basis")
		}
		return domain.ControlNotApplicableBasis{}, nil
	}
	if raw == nil {
		return domain.ControlNotApplicableBasis{}, fmt.Errorf(
			"not-applicable control is missing its commercial basis")
	}
	return domain.NewControlNotApplicableBasis(*raw)
}
