package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 实现 ports.ShipmentDocumentNumberRecorder，并给取号编排的「是否已用」查询（ADR-0355 决定八）。
// 单据号在 shipment_request 行上占两列：document_number 与 document_number_absence（约束 shipment_request_document_number_or_absence
// 让两者互斥），两列皆空即上线前开立。

var _ ports.ShipmentDocumentNumberRecorder = (*ShipmentRequests)(nil)

// documentNumberingColumns 是两列在库里的样子。
type documentNumberingColumns struct {
	number  *string
	absence *string
}

// numbering 把两列译成领域那一格。两列皆空译成「上线前开立」而不是零值：零值是「还没开立」，读回一份那样的委托就能被补上号。
// 两列都有值、或原因不在封闭集合里，是一行坏数据，如实上抛（迁移上的 CHECK 本该拦住它）。
func (columns documentNumberingColumns) numbering() (domain.ShipmentDocumentNumbering, error) {
	switch {
	case columns.number != nil && columns.absence != nil:
		return domain.ShipmentDocumentNumbering{}, fmt.Errorf("单据号与无号原因同时在场")
	case columns.number != nil:
		number, err := domain.NewShipmentDocumentNumber(*columns.number)
		if err != nil {
			return domain.ShipmentDocumentNumbering{}, err
		}
		return domain.NumberedShipment(number)
	case columns.absence != nil:
		if *columns.absence != domain.DocumentNumberingTimeZoneNotConfigured.String() {
			return domain.ShipmentDocumentNumbering{}, fmt.Errorf("无号原因不是本上下文的取值：%q", *columns.absence)
		}
		return domain.UnnumberedShipment(domain.DocumentNumberingTimeZoneNotConfigured)
	default:
		return domain.UnnumberedShipment(domain.IssuedBeforeDocumentNumbering)
	}
}

// columnsOf 是写的那一半。上线前开立不写成值（约束 shipment_request_document_number_absence_closed 只收「单据编号时区未配置」），
// 所以记它是编程错误：开立路径只会交来有号或时区未配置。
func columnsOf(numbering domain.ShipmentDocumentNumbering) (documentNumberingColumns, error) {
	if number, numbered := numbering.Number(); numbered {
		value := number.String()
		return documentNumberingColumns{number: &value}, nil
	}
	if absence, absent := numbering.Absence(); absent && absence == domain.DocumentNumberingTimeZoneNotConfigured {
		value := absence.String()
		return documentNumberingColumns{absence: &value}, nil
	}
	return documentNumberingColumns{}, fmt.Errorf("document numbering %+v is not issued on submission", numbering)
}

// RecordShipmentDocumentNumbering 把开立结果写进刚建成的那一行，与建单同一笔事务（RequireExecutor：事务外即拒，不替调用方另开）。
// 只写两列都还空着的行，零行命中即失败：号开立后不变（ADR-0355 决定三），一行已有结果再写一次是编程错误，不是业务答案。
// 不推进 revision：这一笔与建单同生共死，提交之前没有人读得到这一行，委托版本仍是建单写下的那一个。
func (repository *ShipmentRequests) RecordShipmentDocumentNumbering(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) error {
	columns, err := columnsOf(request.DocumentNumbering())
	if err != nil {
		return fmt.Errorf("record shipment document number: %w", err)
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record shipment document number: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request
		    SET document_number = $6,
		        document_number_absence = $7
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND source = $3
		    AND source_request_key = $4
		    AND shipment_request_id = $5
		    AND document_number IS NULL
		    AND document_number_absence IS NULL`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		request.ShipmentRequestID().String(),
		columns.number,
		columns.absence,
	)
	if err != nil {
		return fmt.Errorf("record shipment document number: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("record shipment document number: %s 没有一行两列都空着的委托可写（命中 %d 行）",
			request.ShipmentRequestID(), tag.RowsAffected())
	}
	return nil
}

// ShipmentDocumentNumberUsed 答本租户的委托里是否已有这个单据号，给取号编排判撞（ADR-0355 决定八）。它在开立事务里查
// （RequireExecutor）：同一事务里先前跳过的号与这一笔的写入都看得见，事务外查到的「没用过」到提交时可能已不成立。
func (repository *ShipmentRequests) ShipmentDocumentNumberUsed(
	ctx context.Context,
	tenant domain.TenantID,
	number string,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("check shipment document number: %w", err)
	}
	var used bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		       FROM parcel_shipment.shipment_request
		      WHERE tenant_id = $1
		        AND document_number = $2)`,
		tenant.String(),
		number,
	).Scan(&used)
	if err != nil {
		return false, fmt.Errorf("check shipment document number: %w", err)
	}
	return used, nil
}
