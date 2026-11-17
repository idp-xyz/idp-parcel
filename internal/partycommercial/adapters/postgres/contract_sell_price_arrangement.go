package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是客户合同版本的销售价格约定（0049；ADR-0380 决定一）在持久化面的两半：发布写口，与客户合同目录行上的
// 那一节。

// SaveContractSellPriceArrangement 登记一份客户合同版本的销售价格约定。判读纪律同本包其余声明写口：先读回既有
// 约定再决定写不写，既有在场即只比对、一行不改。
//
// 比对按位置进行，这与约定表、方式表按集合比对相反：来源的顺序是正文（回退只在声明的顺序里发生），同一组计价
// 范围换了先后就是另一份约定，答`内容冲突`而不是重放。
func (repository *CommercialPublications) SaveContractSellPriceArrangement(
	ctx context.Context,
	arrangement domain.ContractSellPriceArrangement,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract sell price arrangement: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(arrangement.Contract())

	sources, _ := arrangement.Sources()
	var incomingBasis *string
	if basis, notPriced := arrangement.NoSellPriceBasis(); notPriced {
		value := basis.String()
		incomingBasis = &value
	}

	var existingBasis *string
	present := true
	err = executor.QueryRow(ctx,
		`SELECT no_sell_price_basis
		   FROM party_commercial.customer_contract_sell_price_arrangement
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label).Scan(&existingBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		present, err = false, nil
	}
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract sell price arrangement: %w", err)
	}
	if present {
		existingSources, err := scanSellPriceSources(ctx, executor, tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract sell price arrangement: %w", err)
		}
		sameBasis := (existingBasis == nil && incomingBasis == nil) ||
			(existingBasis != nil && incomingBasis != nil && *existingBasis == *incomingBasis)
		if !sameBasis || len(existingSources) != len(sources) {
			return ports.DeclarationContentConflict, nil
		}
		for index, source := range sources {
			if existingSources[index] != source.String() {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.customer_contract_sell_price_arrangement
			(tenant_id, object_kind, object_id, version_label, no_sell_price_basis)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, kind, objectID, label, incomingBasis,
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract sell price arrangement: %w", err)
	}
	for index, source := range sources {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.customer_contract_sell_price_source
				(tenant_id, object_kind, object_id, version_label, ordinal, pricing_scope_ref)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant, kind, objectID, label, index+1, source.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save contract sell price arrangement: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// scanSellPriceSources 按声明的位置读回一份约定的来源。
func scanSellPriceSources(
	ctx context.Context,
	executor bentopg.Executor,
	tenant string, kind uint8, objectID, label string,
) ([]string, error) {
	rows, err := executor.Query(ctx,
		`SELECT pricing_scope_ref
		   FROM party_commercial.customer_contract_sell_price_source
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4
		  ORDER BY ordinal`,
		tenant, kind, objectID, label)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

// sellPriceArrangementColumns 是客户合同目录查询里销售价格约定的一节：父行的不计销售价依据与声明时刻，子表按声明
// 位置聚成的来源。两格都用相关子查询——父行按主键至多一行，子表若与约定行、方式行并列 LEFT JOIN 会互相做笛卡尔积
// （理由同 deliveryConditionColumns）。来源按 ordinal 排，不按范围名排：顺序是正文。调用方的 FROM 里版本表的别名
// 须是 version。
const sellPriceArrangementColumns = `
		        (SELECT json_build_object('noSellPriceBasis', arrangement.no_sell_price_basis,
		                                  'declaredAt',       arrangement.declared_at)
		           FROM party_commercial.customer_contract_sell_price_arrangement AS arrangement
		          WHERE arrangement.tenant_id     = version.tenant_id
		            AND arrangement.object_kind   = version.object_kind
		            AND arrangement.object_id     = version.object_id
		            AND arrangement.version_label = version.version_label) AS sell_price_arrangement,
		        (SELECT COALESCE(json_agg(source.pricing_scope_ref ORDER BY source.ordinal), '[]'::json)
		           FROM party_commercial.customer_contract_sell_price_source AS source
		          WHERE source.tenant_id     = version.tenant_id
		            AND source.object_kind   = version.object_kind
		            AND source.object_id     = version.object_id
		            AND source.version_label = version.version_label) AS sell_price_sources`

// sellPriceArrangementSelection 是 sellPriceArrangementColumns 在 listed 里导出的列名，次序同
// sellPriceArrangementScan.targets。
const sellPriceArrangementSelection = `sell_price_arrangement, sell_price_sources`

// sellPriceArrangementScan 是与 sellPriceArrangementColumns 同序的扫描目标。
type sellPriceArrangementScan struct {
	arrangementJSON []byte
	sourcesJSON     []byte
}

func (scan *sellPriceArrangementScan) targets() []any {
	return []any{&scan.arrangementJSON, &scan.sourcesJSON}
}

// row 把扫到的一节折成目录行上的可缺格：父行缺席答 nil（这一版合同没有销售价格约定），在场则照字面转写。
//
// 父子对不上要上抛而不是照样交回（判据同 controlBindingRowsFromJSON）：有依据又有来源、或两者皆无的约定，两个
// 构造门都造不出它，读回这样的行说明写口被绕开了——把它当成一份如实的约定，读的人会按一份不存在的声明去定价。
func (scan sellPriceArrangementScan) row() (*ports.SellPriceArrangementCatalogueRow, error) {
	if len(scan.arrangementJSON) == 0 {
		return nil, nil
	}
	var parent struct {
		NoSellPriceBasis *string   `json:"noSellPriceBasis"`
		DeclaredAt       time.Time `json:"declaredAt"`
	}
	if err := json.Unmarshal(scan.arrangementJSON, &parent); err != nil {
		return nil, fmt.Errorf("sell price arrangement is not this adapter's shape: %w", err)
	}
	row := &ports.SellPriceArrangementCatalogueRow{DeclaredAt: parent.DeclaredAt}
	if err := json.Unmarshal(scan.sourcesJSON, &row.Sources); err != nil {
		return nil, fmt.Errorf("sell price sources are not this adapter's shape: %w", err)
	}
	if row.Sources == nil {
		row.Sources = []string{}
	}
	if parent.NoSellPriceBasis != nil {
		row.NoSellPriceBasis = *parent.NoSellPriceBasis
	}
	if (parent.NoSellPriceBasis != nil) == (len(row.Sources) > 0) {
		return nil, fmt.Errorf(
			"sell price arrangement: 销售价格来源与不计销售价的依据恰有一个在场,读回的这份约定两者%s",
			bothOrNeither(parent.NoSellPriceBasis == nil))
	}
	return row, nil
}
