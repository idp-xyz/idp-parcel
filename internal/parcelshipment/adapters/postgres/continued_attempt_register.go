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

// ContinuedAttemptRegisters 实现 ports.ContinuedAttemptRegisterRepository：以（租户 + 包裹）为键的
// 全聚合快照持久化，纹样同 LabelTransactions。
//
// 快照文档的形状照 RehydrateContinuedAttemptRegisterSpec 设计，读回时逐字段过领域构造函数再进
// RehydrateContinuedAttemptRegister——库里一行坏数据在这两道门上暴露，不会变成一份看起来合法的
// 决定链（ADR-0028）。判断不落库：CONTEXT 说它「只由有效的关闭、重开决定及当前有效终局结果派生」，
// 读回之后由调用方拿当前终局现算。
//
// 本适配器今天没有生产写入方。写入方是形成关闭或重开决定的命令口，它要先过 party-commercial 的
// 授权校验，属另一张票——机制先立起来，那张票落地那天对着的不是一张裸表。
type ContinuedAttemptRegisters struct {
	db *bentopg.DB
}

func NewContinuedAttemptRegisters(db *bentopg.DB) (*ContinuedAttemptRegisters, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ContinuedAttemptRegisters{db: db}, nil
}

var _ ports.ContinuedAttemptRegisterRepository = (*ContinuedAttemptRegisters)(nil)

// FindByParcel 按（租户 + 包裹）取回登记册。否定结果只回 false，不区分「没开过册」与「属于另一个
// 租户」——区分它们等于泄露其他租户下是否存在该包裹。
func (repository *ContinuedAttemptRegisters) FindByParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.ContinuedAttemptRegister, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ContinuedAttemptRegister{}, false, fmt.Errorf("find continued attempt register: %w", err)
	}

	var revision int64
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT revision, snapshot
		   FROM parcel_shipment.continued_attempt_register
		  WHERE tenant_id = $1
		    AND parcel_id = $2`,
		tenant.String(),
		parcel.String(),
	).Scan(&revision, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ContinuedAttemptRegister{}, false, nil
	}
	if err != nil {
		return domain.ContinuedAttemptRegister{}, false, fmt.Errorf("find continued attempt register: %w", err)
	}

	register, err := rehydrateContinuedAttemptRegister(revision, tenant, parcel, raw)
	if err != nil {
		return domain.ContinuedAttemptRegister{}, false, fmt.Errorf("find continued attempt register: %w", err)
	}
	return register, true, nil
}

// Insert 开册只发生一次：revision 从 1 起写入，主键冲突译成`已存在`——那是业务答案（另一方先开了
// 这一册），不是技术故障（ADR-0031）。用 ON CONFLICT DO NOTHING 而不是捕 23505：撞键的 INSERT 会把
// 整个事务打进中止态，而拿到`已存在`的编排还要在同一事务里继续读原册。
func (repository *ContinuedAttemptRegisters) Insert(
	ctx context.Context,
	register domain.ContinuedAttemptRegister,
) (ports.ContinuedAttemptRegisterInsertOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ContinuedAttemptRegisterInsertOutcomeInvalid, fmt.Errorf("insert continued attempt register: %w", err)
	}

	raw, err := json.Marshal(continuedAttemptRegisterDocumentOf(register))
	if err != nil {
		return ports.ContinuedAttemptRegisterInsertOutcomeInvalid, fmt.Errorf("insert continued attempt register: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.continued_attempt_register
			(tenant_id, parcel_id, revision, snapshot)
		 VALUES ($1, $2, 1, $3)
		 ON CONFLICT DO NOTHING`,
		register.Tenant().String(),
		register.Parcel().String(),
		raw,
	)
	if err != nil {
		return ports.ContinuedAttemptRegisterInsertOutcomeInvalid, fmt.Errorf("insert continued attempt register: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ContinuedAttemptRegisterAlreadyExists, nil
	}
	return ports.ContinuedAttemptRegisterInserted, nil
}

// Save 在既有册上追加之后落库。预期版本由聚合自己携带（追加不动它），UPDATE 的 WHERE 带上它并加一：
// 零行命中即`版本冲突`——抢先那一方已经落库，本方要重读再重放。
//
// 快照整份覆写而不是逐条 INSERT 决定：决定链的顺序是「最近适用决定是哪一条」的全部依据（CONTEXT
// 「同一业务时点的冲突必须保存稳定、可审计的领域顺序」），整份快照让这个顺序与乐观版本同一次写下，
// 逐条写会把顺序交给另一列去维护。
func (repository *ContinuedAttemptRegisters) Save(
	ctx context.Context,
	register domain.ContinuedAttemptRegister,
) (ports.ContinuedAttemptRegisterSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ContinuedAttemptRegisterSaveOutcomeInvalid, fmt.Errorf("save continued attempt register: %w", err)
	}

	raw, err := json.Marshal(continuedAttemptRegisterDocumentOf(register))
	if err != nil {
		return ports.ContinuedAttemptRegisterSaveOutcomeInvalid, fmt.Errorf("save continued attempt register: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE parcel_shipment.continued_attempt_register
		    SET revision = $3 + 1,
		        snapshot = $4,
		        saved_at = now()
		  WHERE tenant_id = $1
		    AND parcel_id = $2
		    AND revision = $3`,
		register.Tenant().String(),
		register.Parcel().String(),
		register.Revision(),
		raw,
	)
	if err != nil {
		return ports.ContinuedAttemptRegisterSaveOutcomeInvalid, fmt.Errorf("save continued attempt register: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ContinuedAttemptRegisterRevisionConflict, nil
	}
	return ports.ContinuedAttemptRegisterSaved, nil
}

// rehydrateContinuedAttemptRegister 把一行读回聚合：先解文档、再逐字段过构造函数、最后进重建门。
// 读面与仓储共用它——判断只能由聚合的 Judge 现算，读面绕过重建门自己看快照就是把那条规则抄第二份。
func rehydrateContinuedAttemptRegister(
	revision int64,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
	raw []byte,
) (domain.ContinuedAttemptRegister, error) {
	var document continuedAttemptRegisterDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.ContinuedAttemptRegister{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	spec, err := document.rehydrationSpec(revision, tenant, parcel)
	if err != nil {
		return domain.ContinuedAttemptRegister{}, err
	}
	return domain.RehydrateContinuedAttemptRegister(spec)
}

// continuedAttemptRegisterDocument 是快照列里的文档形状——RehydrateContinuedAttemptRegisterSpec 的
// JSON 表达。租户、包裹与 revision 刻意不进文档：三者都是列（键定位、版本挡并发），文档里再存一份
// 就是第二个来源。
type continuedAttemptRegisterDocument struct {
	Decisions []continuedAttemptDecisionDoc `json:"decisions,omitempty"`
}

// continuedAttemptDecisionDoc 是一条决定的文档形状。关闭独有的两项（截断边界、关闭责任来源）与重开
// 独有的一项（关联此前关闭）都带 omitempty：哪一半在场由领域那道按 Kind 分派的校验裁决，文档只如实
// 带回。
type continuedAttemptDecisionDoc struct {
	ID                          string    `json:"id"`
	Kind                        uint8     `json:"kind"`
	Requester                   string    `json:"requester,omitempty"`
	Decider                     string    `json:"decider"`
	AuthorityRole               string    `json:"authorityRole"`
	AuthoritySnapshot           string    `json:"authoritySnapshot"`
	Reason                      string    `json:"reason"`
	EffectiveAt                 time.Time `json:"effectiveAt"`
	CutoffBoundary              string    `json:"cutoffBoundary,omitempty"`
	ClosureResponsibilitySource string    `json:"closureResponsibilitySource,omitempty"`
	RelatedPriorClosure         string    `json:"relatedPriorClosure,omitempty"`
}

// continuedAttemptRegisterDocumentOf 从聚合的公开访问器摊出文档。只读不判断：校验在读回那一侧的
// RehydrateContinuedAttemptRegister 上。
func continuedAttemptRegisterDocumentOf(register domain.ContinuedAttemptRegister) continuedAttemptRegisterDocument {
	var document continuedAttemptRegisterDocument
	for _, decision := range register.Decisions() {
		document.Decisions = append(document.Decisions, continuedAttemptDecisionDoc{
			ID:                          decision.ID().String(),
			Kind:                        uint8(decision.Kind()),
			Requester:                   decision.Requester().String(),
			Decider:                     decision.Decider().String(),
			AuthorityRole:               decision.AuthorityRole().String(),
			AuthoritySnapshot:           decision.AuthoritySnapshot().String(),
			Reason:                      decision.Reason().String(),
			EffectiveAt:                 decision.EffectiveAt().UTC(),
			CutoffBoundary:              decision.CutoffBoundary().String(),
			ClosureResponsibilitySource: decision.ClosureResponsibilitySource().String(),
			RelatedPriorClosure:         decision.RelatedPriorClosure().String(),
		})
	}
	return document
}

// rehydrationSpec 把文档逐字段过领域构造函数拼回重建规格。任何一个构造函数拒绝都说明这一行不是
// 本适配器写下的形状（或写它的版本有 bug），错误如实上抛。
func (document continuedAttemptRegisterDocument) rehydrationSpec(
	revision int64,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.RehydrateContinuedAttemptRegisterSpec, error) {
	spec := domain.RehydrateContinuedAttemptRegisterSpec{
		Revision: revision,
		Tenant:   tenant,
		Parcel:   parcel,
	}
	for _, raw := range document.Decisions {
		decision, err := raw.spec()
		if err != nil {
			return domain.RehydrateContinuedAttemptRegisterSpec{}, err
		}
		spec.Decisions = append(spec.Decisions, decision)
	}
	return spec, nil
}

func (document continuedAttemptDecisionDoc) spec() (domain.ContinuedAttemptDecisionSpec, error) {
	id, err := domain.NewContinuedAttemptDecisionID(document.ID)
	if err != nil {
		return domain.ContinuedAttemptDecisionSpec{}, err
	}
	decider, err := domain.NewDeciderReference(document.Decider)
	if err != nil {
		return domain.ContinuedAttemptDecisionSpec{}, err
	}
	authorityRole, err := domain.NewContinuedAttemptAuthorityRoleReference(document.AuthorityRole)
	if err != nil {
		return domain.ContinuedAttemptDecisionSpec{}, err
	}
	authoritySnapshot, err := domain.NewContinuedAttemptAuthoritySnapshot(document.AuthoritySnapshot)
	if err != nil {
		return domain.ContinuedAttemptDecisionSpec{}, err
	}
	reason, err := domain.NewContinuedAttemptReasonReference(document.Reason)
	if err != nil {
		return domain.ContinuedAttemptDecisionSpec{}, err
	}
	spec := domain.ContinuedAttemptDecisionSpec{
		ID:                id,
		Kind:              domain.ContinuedAttemptDecisionKind(document.Kind),
		Decider:           decider,
		AuthorityRole:     authorityRole,
		AuthoritySnapshot: authoritySnapshot,
		Reason:            reason,
		EffectiveAt:       document.EffectiveAt,
	}
	// 四个可缺项各自缺席即那一格（请求方「如有」；另三项由 Kind 决定哪一半在场）。在这里补空值判断
	// 之外的任何规则，都是把领域那道按 Kind 分派的校验抄成第二份。
	if document.Requester != "" {
		requester, err := domain.NewRequesterReference(document.Requester)
		if err != nil {
			return domain.ContinuedAttemptDecisionSpec{}, err
		}
		spec.Requester = requester
	}
	if document.CutoffBoundary != "" {
		boundary, err := domain.NewAuthoritativeCutoffBoundary(document.CutoffBoundary)
		if err != nil {
			return domain.ContinuedAttemptDecisionSpec{}, err
		}
		spec.CutoffBoundary = boundary
	}
	if document.ClosureResponsibilitySource != "" {
		source, err := domain.NewClosureResponsibilitySourceReference(document.ClosureResponsibilitySource)
		if err != nil {
			return domain.ContinuedAttemptDecisionSpec{}, err
		}
		spec.ClosureResponsibilitySource = source
	}
	if document.RelatedPriorClosure != "" {
		prior, err := domain.NewContinuedAttemptDecisionID(document.RelatedPriorClosure)
		if err != nil {
			return domain.ContinuedAttemptDecisionSpec{}, err
		}
		spec.RelatedPriorClosure = prior
	}
	return spec, nil
}
