package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// MasterDocuments 实现 ports.MasterDocumentRegistry：总单一个版本一行、关联逐条一行挂在版本上，只插不改
// （ADR-0113；迁移 0017）。
//
// 写口没有任何 UPDATE：撤销、替代、关联重述都是往主表插一版、往子表插这一版的关联集；「当前版」按回指派生。
// 读口全部经领域重建门（ADR-0028）——坏行要在这里响亮，不在列面。
type MasterDocuments struct {
	db *bentopg.DB
}

func NewMasterDocuments(db *bentopg.DB) (*MasterDocuments, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &MasterDocuments{db: db}, nil
}

var _ ports.MasterDocumentRegistry = (*MasterDocuments)(nil)

const masterDocumentColumns = `tenant_id, master_document_ref, version, issuer_ref, scope_ref,
		        commission_ref, booking_ref, standing, changed_at, supersedes_version,
		        replaced_by_document, recorded_at`

// FindByKey 按（租户+总单+版本）取回一个版本。否定结果只回 false。
func (repository *MasterDocuments) FindByKey(
	ctx context.Context,
	key ports.MasterDocumentKey,
) (ports.MasterDocumentRecord, bool, error) {
	records, err := repository.query(ctx, "find master document",
		`SELECT `+masterDocumentColumns+`
		   FROM transport_fulfillment.carrier_master_document
		  WHERE tenant_id = $1 AND master_document_ref = $2 AND version = $3`,
		key.TenantID.String(), key.Document.String(), key.Version.String())
	if err != nil || len(records) == 0 {
		return ports.MasterDocumentRecord{}, false, err
	}
	return records[0], true, nil
}

// FindCurrent 取回一份总单此刻未被任何版本回指的那一版。「当前」是派生问答不是可变标记：表上没有 current
// 列，改一版就得回写的东西这里一个都没有。链由 0017 的两道部分唯一索引守成线性，所以至多一行。
func (repository *MasterDocuments) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	document domain.MasterDocumentReference,
) (ports.MasterDocumentRecord, bool, error) {
	records, err := repository.query(ctx, "find current master document",
		`SELECT `+masterDocumentColumns+`
		   FROM transport_fulfillment.carrier_master_document AS current
		  WHERE tenant_id = $1 AND master_document_ref = $2
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.carrier_master_document AS successor
		         WHERE successor.tenant_id = current.tenant_id
		           AND successor.master_document_ref = current.master_document_ref
		           AND successor.supersedes_version = current.version)
		  ORDER BY recorded_at DESC
		  LIMIT 1`,
		tenant.String(), document.String())
	if err != nil || len(records) == 0 {
		return ports.MasterDocumentRecord{}, false, err
	}
	return records[0], true, nil
}

// ListVersions 按登记先后交回一份总单的全部版本；同一时刻落的再按版本号排，让顺序可复现。
func (repository *MasterDocuments) ListVersions(
	ctx context.Context,
	tenant domain.TenantID,
	document domain.MasterDocumentReference,
) ([]ports.MasterDocumentRecord, error) {
	return repository.query(ctx, "list master document versions",
		`SELECT `+masterDocumentColumns+`
		   FROM transport_fulfillment.carrier_master_document
		  WHERE tenant_id = $1 AND master_document_ref = $2
		  ORDER BY recorded_at, version`,
		tenant.String(), document.String())
}

// masterDocumentRow 是主表一行在装回领域之前的中间形——关联要另查子表再合到一起，重建门在两份都到手
// 之后才走。
type masterDocumentRow struct {
	spec       domain.RehydrateMasterDocumentSpec
	recordedAt time.Time
}

func (repository *MasterDocuments) query(
	ctx context.Context,
	verb string,
	sql string,
	args ...any,
) ([]ports.MasterDocumentRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	heads, err := scanMasterDocumentRows(rows)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}

	records := make([]ports.MasterDocumentRecord, 0, len(heads))
	for _, head := range heads {
		associations, err := repository.loadAssociations(ctx, querier, head.spec)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", verb, err)
		}
		head.spec.Associations = associations
		document, err := domain.RehydrateMasterDocument(head.spec)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", verb, err)
		}
		records = append(records, ports.MasterDocumentRecord{
			Key:        ports.MasterDocumentKey{TenantID: head.spec.TenantID, Document: head.spec.Document, Version: head.spec.Version},
			Document:   document,
			RecordedAt: head.recordedAt,
		})
	}
	return records, nil
}

func scanMasterDocumentRows(rows pgx.Rows) ([]masterDocumentRow, error) {
	defer rows.Close()
	var heads []masterDocumentRow
	for rows.Next() {
		head, err := scanMasterDocumentRow(rows)
		if err != nil {
			return nil, err
		}
		heads = append(heads, head)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return heads, nil
}

// scanMasterDocumentRow 把主表一行逐列走各自的构造门；关联与重建门在 query 里接上——不按列直接拼结构体
// （ADR-0028）。
func scanMasterDocumentRow(rows pgx.Rows) (masterDocumentRow, error) {
	var tenantID, documentRef, version, issuerRef, scopeRef, standing string
	var commissionRef, bookingRef, supersedes, replacedBy *string
	var changedAt *time.Time
	var recordedAt time.Time
	if err := rows.Scan(
		&tenantID, &documentRef, &version, &issuerRef, &scopeRef,
		&commissionRef, &bookingRef, &standing, &changedAt, &supersedes,
		&replacedBy, &recordedAt,
	); err != nil {
		return masterDocumentRow{}, err
	}

	var spec domain.RehydrateMasterDocumentSpec
	var err error
	if spec.TenantID, err = domain.NewTenantID(tenantID); err != nil {
		return masterDocumentRow{}, err
	}
	if spec.Document, err = domain.NewMasterDocumentReference(documentRef); err != nil {
		return masterDocumentRow{}, err
	}
	if spec.Version, err = domain.NewMasterDocumentVersion(version); err != nil {
		return masterDocumentRow{}, err
	}
	if spec.Issuer, err = domain.NewMasterDocumentIssuerReference(issuerRef); err != nil {
		return masterDocumentRow{}, err
	}
	if spec.Scope, err = domain.NewTransportScopeReference(scopeRef); err != nil {
		return masterDocumentRow{}, err
	}
	if commissionRef != nil {
		if spec.Commission, err = domain.NewTransportCommissionReference(*commissionRef); err != nil {
			return masterDocumentRow{}, err
		}
	}
	if bookingRef != nil {
		if spec.Booking, err = domain.NewBookingReference(*bookingRef); err != nil {
			return masterDocumentRow{}, err
		}
	}
	if spec.Standing, err = domain.ParseMasterDocumentStanding(standing); err != nil {
		return masterDocumentRow{}, err
	}
	if changedAt != nil {
		spec.ChangedAt = changedAt.UTC()
	}
	if supersedes != nil {
		if spec.Supersedes, err = domain.NewMasterDocumentVersion(*supersedes); err != nil {
			return masterDocumentRow{}, err
		}
	}
	if replacedBy != nil {
		if spec.ReplacedBy, err = domain.NewMasterDocumentReference(*replacedBy); err != nil {
			return masterDocumentRow{}, err
		}
	}
	return masterDocumentRow{spec: spec, recordedAt: recordedAt.UTC()}, nil
}

// loadAssociations 装回一版的关联集；顺序由领域重建门整理，这里只按主键取。
func (repository *MasterDocuments) loadAssociations(
	ctx context.Context,
	querier bentopg.Querier,
	spec domain.RehydrateMasterDocumentSpec,
) ([]domain.MasterDocumentAssociation, error) {
	rows, err := querier.Query(ctx,
		`SELECT associated_kind, associated_ref
		   FROM transport_fulfillment.carrier_master_document_association
		  WHERE tenant_id = $1 AND master_document_ref = $2 AND version = $3
		  ORDER BY associated_kind, associated_ref`,
		spec.TenantID.String(), spec.Document.String(), spec.Version.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var associations []domain.MasterDocumentAssociation
	for rows.Next() {
		var kindWord, reference string
		if err := rows.Scan(&kindWord, &reference); err != nil {
			return nil, err
		}
		kind, err := domain.ParseAssociatedObjectKind(kindWord)
		if err != nil {
			return nil, err
		}
		association, err := domain.NewMasterDocumentAssociation(kind, reference)
		if err != nil {
			return nil, err
		}
		associations = append(associations, association)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return associations, nil
}

// Save 登记一个版本：先插主表一行，再逐条插这一版的关联。撞键答`已登记`（ADR-0031）——撞的可能是主键
// （同一版本重放）也可能是 0017 的两道部分唯一索引（第二个首版、同一前版被回指两次），写口不分，调用方读回
// 比对；撞了就不插关联，子表上不会留下一组没有主行的关联（外键也不允许）。
func (repository *MasterDocuments) Save(
	ctx context.Context,
	record ports.MasterDocumentRecord,
) (ports.MasterDocumentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MasterDocumentSaveOutcomeInvalid, fmt.Errorf("save master document: %w", err)
	}
	if err := assertMasterDocumentKeyAgrees(record); err != nil {
		return ports.MasterDocumentSaveOutcomeInvalid, fmt.Errorf("save master document: %w", err)
	}

	document := record.Document
	var commission, booking, supersedes, replacedBy *string
	if reference, has := document.Commission(); has {
		text := reference.String()
		commission = &text
	}
	if reference, has := document.Booking(); has {
		text := reference.String()
		booking = &text
	}
	if prior, has := document.Supersedes(); has {
		text := prior.String()
		supersedes = &text
	}
	if replacement, has := document.ReplacedBy(); has {
		text := replacement.String()
		replacedBy = &text
	}
	changedAt, changed := document.ChangedAt()

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.carrier_master_document
		     (`+masterDocumentColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Document.String(),
		record.Key.Version.String(),
		document.Issuer().String(),
		document.Scope().String(),
		commission,
		booking,
		document.Standing().String(),
		nullableTime(changedAt, changed),
		supersedes,
		replacedBy,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.MasterDocumentSaveOutcomeInvalid, fmt.Errorf("save master document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.MasterDocumentAlreadyRegistered, nil
	}

	for _, association := range document.Associations() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.carrier_master_document_association
			     (tenant_id, master_document_ref, version, associated_kind, associated_ref)
			 VALUES ($1, $2, $3, $4, $5)`,
			record.Key.TenantID.String(),
			record.Key.Document.String(),
			record.Key.Version.String(),
			association.Kind().String(),
			association.Reference(),
		); err != nil {
			return ports.MasterDocumentSaveOutcomeInvalid, fmt.Errorf("save master document association: %w", err)
		}
	}
	return ports.MasterDocumentSaved, nil
}

// assertMasterDocumentKeyAgrees 挡住「键说的是一个版本、聚合说的是另一个」那种写入。
func assertMasterDocumentKeyAgrees(record ports.MasterDocumentRecord) error {
	if record.Key.TenantID != record.Document.TenantID() ||
		record.Key.Document != record.Document.Document() ||
		record.Key.Version != record.Document.Version() {
		return fmt.Errorf("master document key disagrees with the aggregate")
	}
	return nil
}
