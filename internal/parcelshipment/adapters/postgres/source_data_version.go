package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SourceDataVersions 实现 ports.SourceDataVersionRecords：客户原始资料版本的只追加
// 登记册。
//
// 版本不可覆盖，所以这里没有 UPDATE：Append 用 ON CONFLICT DO NOTHING 把重复追加译成
// `已记录`（撞键的 INSERT 不打中止事务，编排还要同事务继续），零行命中即重放。链信息
// （基准/前版/指纹）平铺成列，其余留痕清单进 jsonb，读回逐字段过领域构造函数——一行
// 坏数据在构造门上暴露，不会变成一份看起来合法的版本（ADR-0028 同款）。
type SourceDataVersions struct {
	db *bentopg.DB
}

func NewSourceDataVersions(db *bentopg.DB) (*SourceDataVersions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &SourceDataVersions{db: db}, nil
}

// FindVersion 按委托来源身份加版本标识取回一份版本。否定结果只回 false，不区分
// 「不存在」与「属于另一个租户或客户账户」。
func (repository *SourceDataVersions) FindVersion(
	ctx context.Context,
	identity domain.SourceIdentity,
	versionID domain.SourceDataVersionID,
) (domain.CustomerSourceDataVersion, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, false, fmt.Errorf("find source data version: %w", err)
	}

	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot
		   FROM parcel_shipment.customer_source_data_version
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND source = $3
		    AND source_request_key = $4
		    AND version_id = $5`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		versionID.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerSourceDataVersion{}, false, nil
	}
	if err != nil {
		return domain.CustomerSourceDataVersion{}, false, fmt.Errorf("find source data version: %w", err)
	}

	var document sourceDataVersionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.CustomerSourceDataVersion{}, false, fmt.Errorf("find source data version: 快照不是本适配器写下的形状：%w", err)
	}
	version, err := document.version()
	if err != nil {
		return domain.CustomerSourceDataVersion{}, false, fmt.Errorf("find source data version: %w", err)
	}
	return version, true, nil
}

// Append 追加一份不可覆盖的版本。同键重复追加交回`已记录`——版本只形成一次，先到者
// 的留痕清单原样保留。
func (repository *SourceDataVersions) Append(
	ctx context.Context,
	identity domain.SourceIdentity,
	version domain.CustomerSourceDataVersion,
) (ports.SourceDataVersionAppendOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SourceDataVersionAppendOutcomeInvalid, fmt.Errorf("append source data version: %w", err)
	}

	raw, err := json.Marshal(sourceDataVersionDocumentOf(version))
	if err != nil {
		return ports.SourceDataVersionAppendOutcomeInvalid, fmt.Errorf("append source data version: %w", err)
	}
	var priorVersion *string
	if prior, corrects := version.Basis().PriorVersion(); corrects {
		value := prior.String()
		priorVersion = &value
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.customer_source_data_version
			(tenant_id, customer_account_id, source, source_request_key, version_id,
			 shipment_request_id, on_baseline, prior_version_id, payload_digest,
			 snapshot, formed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		version.VersionID().String(),
		version.Scope().ShipmentRequestID().String(),
		version.Basis().OnAcceptanceBaseline(),
		priorVersion,
		version.Request().Digest().String(),
		raw,
		version.FormedAt().UTC(),
	)
	if err != nil {
		return ports.SourceDataVersionAppendOutcomeInvalid, fmt.Errorf("append source data version: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SourceDataVersionAlreadyRecorded, nil
	}
	return ports.SourceDataVersionAppended, nil
}

// sourceDataVersionDocument 是快照列里的文档形状——CustomerSourceDataVersionSpec 的
// JSON 表达。链列（on_baseline/prior_version_id/payload_digest）在文档里仍整份保留：
// 列是查询投影，文档是重建来源，重建只信文档一处。
type sourceDataVersionDocument struct {
	VersionID         string         `json:"versionId"`
	ShipmentRequestID string         `json:"shipmentRequestId"`
	ParcelID          string         `json:"parcelId,omitempty"`
	DataGroup         string         `json:"dataGroup"`
	OnBaseline        bool           `json:"onBaseline"`
	PriorVersionID    string         `json:"priorVersionId,omitempty"`
	Intent            uint8          `json:"intent"`
	Request           sourceDocument `json:"request"`
	Reason            string         `json:"reason"`
	Requester         string         `json:"requester"`
	Decider           string         `json:"decider"`
	Authority         string         `json:"authority"`
	EffectiveAt       *time.Time     `json:"effectiveAt,omitempty"`
	FormedAt          time.Time      `json:"formedAt"`
}

func sourceDataVersionDocumentOf(version domain.CustomerSourceDataVersion) sourceDataVersionDocument {
	request := version.Request()
	identity := request.Identity()
	document := sourceDataVersionDocument{
		VersionID:         version.VersionID().String(),
		ShipmentRequestID: version.Scope().ShipmentRequestID().String(),
		DataGroup:         version.Scope().DataGroup().String(),
		OnBaseline:        version.Basis().OnAcceptanceBaseline(),
		Intent:            uint8(version.Intent()),
		Request: sourceDocument{
			TenantID:          identity.TenantID().String(),
			CustomerAccountID: identity.CustomerAccountID().String(),
			Source:            identity.Source().String(),
			RequestKey:        identity.RequestKey().String(),
			PayloadDigest:     request.Digest().String(),
			OccurredAt:        request.OccurredAt().UTC(),
			ReceivedAt:        request.ReceivedAt().UTC(),
		},
		Reason:    version.Reason().String(),
		Requester: version.Requester().String(),
		Decider:   version.Decider().String(),
		Authority: version.Authority().String(),
		FormedAt:  version.FormedAt().UTC(),
	}
	if parcel, scoped := version.Scope().DeclaredParcelID(); scoped {
		document.ParcelID = parcel.String()
	}
	if prior, corrects := version.Basis().PriorVersion(); corrects {
		document.PriorVersionID = prior.String()
	}
	if version.HasEffectiveAt() {
		at := version.EffectiveAt().UTC()
		document.EffectiveAt = &at
	}
	return document
}

func (document sourceDataVersionDocument) version() (domain.CustomerSourceDataVersion, error) {
	versionID, err := domain.NewSourceDataVersionID(document.VersionID)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}
	requestID, err := domain.NewShipmentRequestID(document.ShipmentRequestID)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}
	dataGroup, err := domain.NewSourceDataGroupReference(document.DataGroup)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}
	var scope domain.SourceDataScope
	if document.ParcelID != "" {
		parcel, err := domain.NewDeclaredParcelID(document.ParcelID)
		if err != nil {
			return domain.CustomerSourceDataVersion{}, err
		}
		if scope, err = domain.NewParcelScopedSourceData(requestID, parcel, dataGroup); err != nil {
			return domain.CustomerSourceDataVersion{}, err
		}
	} else if scope, err = domain.NewShipmentScopedSourceData(requestID, dataGroup); err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}

	basis := domain.NewSupplementOnAcceptanceBaseline()
	if !document.OnBaseline {
		prior, err := domain.NewSourceDataVersionID(document.PriorVersionID)
		if err != nil {
			return domain.CustomerSourceDataVersion{}, err
		}
		if basis, err = domain.NewAmendmentOfVersion(prior); err != nil {
			return domain.CustomerSourceDataVersion{}, err
		}
	}

	request, err := document.Request.fingerprint()
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}
	reason, err := domain.NewAmendmentReasonReference(document.Reason)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}
	requester, err := domain.NewRequesterReference(document.Requester)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}
	decider, err := domain.NewDeciderReference(document.Decider)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}
	authority, err := domain.NewAmendmentAuthoritySnapshot(document.Authority)
	if err != nil {
		return domain.CustomerSourceDataVersion{}, err
	}

	spec := domain.CustomerSourceDataVersionSpec{
		VersionID: versionID,
		Scope:     scope,
		Basis:     basis,
		Intent:    domain.AmendmentIntent(document.Intent),
		Request:   request,
		Reason:    reason,
		Requester: requester,
		Decider:   decider,
		Authority: authority,
		FormedAt:  document.FormedAt,
	}
	if document.EffectiveAt != nil {
		spec.EffectiveAt = *document.EffectiveAt
	}
	return domain.FormCustomerSourceDataVersion(spec)
}
