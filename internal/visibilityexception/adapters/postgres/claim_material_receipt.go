package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ClaimMaterialReceipts 实现 ports.ClaimEvidenceView：读收讫减撤销的现存集
// （.scratch/ve-claims-read-seams/02）。租户随每次调用到达（端口签名如此）——本视图
// 给多租户编排（`cmd/parcel-api`）消费，没有一个装配期可钉的租户。
//
// 第二个返回值恒答 true：归集面自 0021 起存在，零行是「查过了，一件都没收到」的有效
// 事实——差集等于整份清单，索赔照常进限期补充。答 false 的「无从查起」那一格随
// `cmd/parcel-api` 的显式未配置桩一并退役；读不通仍作为错误返回，不折成任何一格。
//
// 同一材料收讫多次时去重交一份：端口只答「已收到什么」，收到几次不改差集。撤销按
// 收讫行逐次抵扣——一次收讫被撤销不影响同一材料的另一次收讫。
type ClaimMaterialReceipts struct {
	db *bentopg.DB
}

func NewClaimMaterialReceipts(db *bentopg.DB) (*ClaimMaterialReceipts, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ClaimMaterialReceipts{db: db}, nil
}

var _ ports.ClaimEvidenceView = (*ClaimMaterialReceipts)(nil)

// ReceivedMaterials 交出（租户+批次+项）名下现存的已收讫材料引用，按引用排序——同
// 一份归集每次读回同一串，差集入账才不会看着像变过。三件缺一按「依赖调不通」报错，
// 不答业务格：编排在取证据之前已由 loadClaim 校验过三件非空，走到这里还缺是接线
// 错误，答「零件」会替一个没被问对的问题立出「一件都没收到」的事实。
func (view *ClaimMaterialReceipts) ReceivedMaterials(
	ctx context.Context,
	tenant domain.TenantID,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
) ([]domain.MaterialRequirementReference, bool, error) {
	if tenant.String() == "" || batch.String() == "" || item.String() == "" {
		return nil, false, fmt.Errorf(
			"received materials: query carries blank identity (tenant=%q batch=%q item=%q)",
			tenant, batch, item)
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("received materials: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT DISTINCT receipt.material_ref
		   FROM visibility_exception.claim_material_receipt AS receipt
		  WHERE receipt.tenant_id = $1
		    AND receipt.claim_batch_ref = $2
		    AND receipt.claim_item_id = $3
		    AND NOT EXISTS (
		            SELECT 1
		              FROM visibility_exception.claim_material_receipt_revocation AS revocation
		             WHERE revocation.tenant_id = receipt.tenant_id
		               AND revocation.claim_batch_ref = receipt.claim_batch_ref
		               AND revocation.claim_item_id = receipt.claim_item_id
		               AND revocation.material_ref = receipt.material_ref
		               AND revocation.received_at = receipt.received_at
		        )
		  ORDER BY receipt.material_ref`,
		tenant.String(), batch.String(), item.String(),
	)
	if err != nil {
		return nil, false, fmt.Errorf("received materials: %w", err)
	}
	defer rows.Close()

	var received []domain.MaterialRequirementReference
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, false, fmt.Errorf("received materials: %w", err)
		}
		reference, err := domain.NewMaterialRequirementReference(value)
		if err != nil {
			return nil, false, fmt.Errorf("received materials: 列值 %q：%w", value, err)
		}
		received = append(received, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("received materials: %w", err)
	}
	return received, true, nil
}

// MaterialReceiptRegistrar 实现 ports.MaterialReceiptRegistry：材料归集面的写入方。
//
// 租户随每次调用到达，不在装配期固定——与 CatalogRegistrar 同款：登记是管理动作，
// 同一个进程入口可能替不同租户登记。两个方法都在调用方的事务内执行（RequireExecutor）：
// 登记与通道留痕必须同一提交（`cmd/parcel-ve-register` 的纪律）。
type MaterialReceiptRegistrar struct {
	db *bentopg.DB
}

func NewMaterialReceiptRegistrar(db *bentopg.DB) (*MaterialReceiptRegistrar, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &MaterialReceiptRegistrar{db: db}, nil
}

var _ ports.MaterialReceiptRegistry = (*MaterialReceiptRegistrar)(nil)

// RegisterReceipt 登记一笔收讫。同五件重登幂等（ON CONFLICT DO NOTHING）：行身份就
// 是事实本身，没有内容可被顶替——先登的经手声明留在行上，重放答「已在场」即指名本次
// 没有写入。
func (registrar *MaterialReceiptRegistrar) RegisterReceipt(
	ctx context.Context,
	tenant domain.TenantID,
	receipt ports.MaterialReceipt,
) (ports.MaterialReceiptWriteOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MaterialReceiptWriteOutcomeInvalid, fmt.Errorf("register material receipt: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.claim_material_receipt
			(tenant_id, claim_batch_ref, claim_item_id, material_ref, received_at, received_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), receipt.Batch.String(), receipt.Item.String(),
		receipt.Material.String(), receipt.ReceivedAt.UTC(), receipt.ReceivedBy,
	)
	if err != nil {
		return ports.MaterialReceiptWriteOutcomeInvalid, fmt.Errorf("register material receipt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.MaterialReceiptAlreadyRecorded, nil
	}
	return ports.MaterialReceiptRecorded, nil
}

// RevokeReceipt 给一笔收讫另立撤销行。
//
// 先核收讫行在场再插入：孤立撤销本可靠外键拦，但外键违规会把整笔事务打进中止态，
// 而撤销与留痕同事务（ADR-0031 不捕 23505 的同一条理由）——所以「无从撤销」要在
// 违规发生之前判出来、作为治理答案交回。先查后插的窗口无害：收讫行没有 DELETE
// 路径，EXISTS 成立后不会再消失；并发重复撤销由主键 ON CONFLICT 接住。
func (registrar *MaterialReceiptRegistrar) RevokeReceipt(
	ctx context.Context,
	tenant domain.TenantID,
	revocation ports.MaterialReceiptRevocation,
) (ports.MaterialReceiptWriteOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MaterialReceiptWriteOutcomeInvalid, fmt.Errorf("revoke material receipt: %w", err)
	}

	var exists bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
		            SELECT 1 FROM visibility_exception.claim_material_receipt
		             WHERE tenant_id = $1 AND claim_batch_ref = $2 AND claim_item_id = $3
		               AND material_ref = $4 AND received_at = $5
		        )`,
		tenant.String(), revocation.Batch.String(), revocation.Item.String(),
		revocation.Material.String(), revocation.ReceivedAt.UTC(),
	).Scan(&exists); err != nil {
		return ports.MaterialReceiptWriteOutcomeInvalid, fmt.Errorf("revoke material receipt: %w", err)
	}
	if !exists {
		return ports.MaterialReceiptUnknown, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.claim_material_receipt_revocation
			(tenant_id, claim_batch_ref, claim_item_id, material_ref, received_at,
			 revoked_by, revoked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), revocation.Batch.String(), revocation.Item.String(),
		revocation.Material.String(), revocation.ReceivedAt.UTC(),
		revocation.RevokedBy, revocation.RevokedAt.UTC(),
	)
	if err != nil {
		return ports.MaterialReceiptWriteOutcomeInvalid, fmt.Errorf("revoke material receipt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.MaterialReceiptRevocationAlreadyRecorded, nil
	}
	return ports.MaterialReceiptRevocationRecorded, nil
}
