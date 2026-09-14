package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 「真实程序要不要求付款人」的规则册（0020 建表，票 sa-cc/12 裁决 1，形照 ADR-0137 决定三的规则型目录行）。
// 写口与读口都落在 DutyPaymentReconciliation 上：规则是核对那一步读的东西，与资金事实登记册、核对册同一族、
// 同一对适配器；它不挂门禁目录那一族——键是（租户、监管程序），不是（范围、动作、边界）。

var (
	_ ports.PayerRequirementRuleRegistry = (*DutyPaymentReconciliation)(nil)
	_ ports.PayerRequirementRuleView     = (*DutyPaymentReconciliation)(nil)
)

// RegisterPayerRequirement 登记某监管程序「要不要求付款人」。registered_at 取事务内库时钟：它是登记这一动作的
// 时间，不是规则的任何业务时间。写入代数与其余登记册同——不 UPSERT，同键由 DO NOTHING 折成`已登记`交回，
// 无 UPDATE 路径，改规则走复核另登。零值规则是调用方编程错误，写前拒——落库会撞 CHECK 而报成「依赖故障」。
func (store *DutyPaymentReconciliation) RegisterPayerRequirement(
	ctx context.Context,
	tenant domain.TenantID,
	procedure domain.CustomsProcedureReference,
	requirement domain.PayerRequirement,
) (ports.CaseConfigurationSaveOutcome, error) {
	word := requirement.String()
	if word == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register payer requirement: unknown requirement %d", requirement)
	}
	if strings.TrimSpace(procedure.String()) == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register payer requirement: the procedure is blank")
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register payer requirement: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.duty_payment_payer_rule
			(tenant_id, procedure_ref, payer_requirement, registered_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT DO NOTHING`,
		tenant.String(), procedure.String(), word,
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register payer requirement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// LoadPayerRequirement 按（租户、监管程序）取回规则。found=false 即这个程序还没登——核对编排答「规则未配置」；
// 库里的词形译不回封闭二值说明有人绕过写口改了它，作错误抛出。
func (store *DutyPaymentReconciliation) LoadPayerRequirement(
	ctx context.Context,
	tenant domain.TenantID,
	procedure domain.CustomsProcedureReference,
) (domain.PayerRequirement, bool, error) {
	if strings.TrimSpace(procedure.String()) == "" {
		return domain.PayerRequirementInvalid, false, fmt.Errorf("load payer requirement: the procedure is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PayerRequirementInvalid, false, fmt.Errorf("load payer requirement: %w", err)
	}

	var word string
	err = querier.QueryRow(ctx,
		`SELECT payer_requirement
		   FROM customs_compliance.duty_payment_payer_rule
		  WHERE tenant_id = $1 AND procedure_ref = $2`,
		tenant.String(), procedure.String(),
	).Scan(&word)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PayerRequirementInvalid, false, nil
	}
	if err != nil {
		return domain.PayerRequirementInvalid, false, fmt.Errorf("load payer requirement: %w", err)
	}

	requirement, err := closedWord(word, domain.PayerRequired, domain.PayerNotRequired)
	if err != nil {
		return domain.PayerRequirementInvalid, false, fmt.Errorf("rebuild payer requirement: %w", err)
	}
	return requirement, true, nil
}
