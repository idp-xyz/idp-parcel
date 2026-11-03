package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var _ ports.CustomerShipmentByDocumentNumberView = (*ShipmentRequestViews)(nil)

// FindCustomerShipmentByDocumentNumber 按（租户，客户账户，单据号）读一份委托：一条语句，账户与号同在 WHERE 上。
//
// 账户放在同一条语句里，「存在但不是你的」这一位就不出本上下文。时延没有拉平：迁移 0035 的（租户，单据号）唯一约束不含
// 账户列，号属别的账户时比查无此号多取一行、再按账户筛掉；ADR-0356 越权风险点 1 已认下这一格，不加常数时间。同一个约束
// 担保至多一行，所以用 QueryRow。号逐字比；两列皆空的旧行与记了无号原因的行 document_number 为空，等号永不命中。
//
// 与 ShipmentRequestViews 其余各口同一条纪律：直读状态列与快照文档，不经聚合重建。已接受时才解快照——接受之前的成员不是
// 正式包裹，读它也没有可答的。
func (views *ShipmentRequestViews) FindCustomerShipmentByDocumentNumber(
	ctx context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountID,
	number domain.ShipmentDocumentNumber,
) (ports.CustomerShipmentByDocumentNumberRecord, bool, error) {
	none := ports.CustomerShipmentByDocumentNumberRecord{}
	if tenant.String() == "" || customer.String() == "" || number.String() == "" {
		return none, false, fmt.Errorf("find customer shipment by document number: tenant, customer account and number are required")
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find customer shipment by document number: %w", err)
	}

	var state uint8
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT state, snapshot
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND document_number = $2
		    AND customer_account_id = $3`,
		tenant.String(),
		number.String(),
		customer.String(),
	).Scan(&state, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find customer shipment by document number: %w", err)
	}

	record := ports.CustomerShipmentByDocumentNumberRecord{State: domain.ShipmentRequestState(state)}
	if record.State.String() == "" {
		return none, false, fmt.Errorf("find customer shipment by document number: 状态列不是本上下文的取值：%d", state)
	}
	if record.State != domain.ShipmentRequestAccepted {
		return record, true, nil
	}
	var document requestDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return none, false, fmt.Errorf("find customer shipment by document number: 快照不是本适配器写下的形状：%w", err)
	}
	parcels, err := effectiveParcelsOf(document)
	if err != nil {
		return none, false, fmt.Errorf("find customer shipment by document number: %w", err)
	}
	record.EffectiveParcels = parcels
	return record, true, nil
}

// effectiveParcelsOf 从已接受委托的快照摊出当前有效包裹：基线成员、首个提交版本的成员与件号，交领域去立。首个提交版本是
// priorVersions 的头一份；没有被取代过的版本时就是当前版本。
func effectiveParcelsOf(document requestDocument) ([]domain.EffectiveParcel, error) {
	if document.Baseline == nil {
		return nil, fmt.Errorf("%w: an accepted snapshot has no acceptance baseline", domain.ErrInconsistentEffectiveParcels)
	}
	baseline, err := declaredParcelIDsOf(document.Baseline.DeclaredParcelIDs)
	if err != nil {
		return nil, err
	}
	first := document.CurrentVersion
	if len(document.PriorVersions) > 0 {
		first = document.PriorVersions[0]
	}
	firstMembers, err := declaredParcelIDsOf(first.DeclaredParcelIDs)
	if err != nil {
		return nil, err
	}
	numbers := make([]domain.CustomerParcelNumberDeclaration, 0, len(document.CustomerParcelNumbers))
	for _, raw := range document.CustomerParcelNumbers {
		declaration, err := raw.declaration()
		if err != nil {
			return nil, err
		}
		numbers = append(numbers, declaration)
	}
	return domain.CurrentEffectiveParcelsOfBaseline(firstMembers, baseline, numbers)
}

func declaredParcelIDsOf(raw []string) ([]domain.DeclaredParcelID, error) {
	parcels := make([]domain.DeclaredParcelID, 0, len(raw))
	for _, value := range raw {
		parcel, err := domain.NewDeclaredParcelID(value)
		if err != nil {
			return nil, err
		}
		parcels = append(parcels, parcel)
	}
	return parcels, nil
}
