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

// 本文件让 ShipmentRequests 同时实现 ports.SubmissionVersionDeclarationView（ADR-0179 决定四）。读的是同一张表的同一份快照：
// 画像随 versionDocument.Profiles、寄收两段随 `elements` 子段落库，读回过重建门，答案由聚合自己译出——不加列不加迁移。

var _ ports.SubmissionVersionDeclarationView = (*ShipmentRequests)(nil)

// LoadSubmissionVersionDeclaration 走（租户 + 委托标识）唯一键取单行。状态只认`已提交`与`已接受`：重建门只开到这两态，`已拒绝`
// 与`已撤回`的委托也不再有接受前控制要估，两态与零行同答 false。
func (repository *ShipmentRequests) LoadSubmissionVersionDeclaration(
	ctx context.Context,
	tenant domain.TenantID,
	shipmentRequestID domain.ShipmentRequestID,
	submissionVersion domain.SubmissionVersionID,
) (domain.SubmissionVersionDeclaration, bool, error) {
	none := domain.SubmissionVersionDeclaration{}
	if tenant.String() == "" || shipmentRequestID.String() == "" || submissionVersion.String() == "" {
		return none, false, fmt.Errorf("load submission version declaration: tenant, shipment request and submission version are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load submission version declaration: %w", err)
	}

	var revision int64
	var state uint8
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT revision, state, snapshot
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND shipment_request_id = $2
		    AND state IN ($3, $4)`,
		tenant.String(),
		shipmentRequestID.String(),
		uint8(domain.ShipmentRequestSubmitted),
		uint8(domain.ShipmentRequestAccepted),
	).Scan(&revision, &state, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load submission version declaration: %w", err)
	}

	var document requestDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return none, false, fmt.Errorf("load submission version declaration: 快照不是本适配器写下的形状：%w", err)
	}
	spec, err := document.rehydrationSpec(revision, domain.ShipmentRequestState(state))
	if err != nil {
		return none, false, fmt.Errorf("load submission version declaration: %w", err)
	}
	request, err := domain.RehydrateShipmentRequest(spec)
	if err != nil {
		return none, false, fmt.Errorf("load submission version declaration: %w", err)
	}
	declaration, found := request.SubmissionVersionDeclarationFor(submissionVersion)
	return declaration, found, nil
}
