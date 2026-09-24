// Package registrationjson 把 VE 各登记种类的登记快照 JSON 折成应用命令。
//
// 它从 parcel-ve-register 的 package main 下沉到上下文的适配器层，是因为登记快照的
// 形状不再只属受控 CLI：配置登记的在线登记口（ADR-0085）收的是同一份快照本体，与
// CLI 的 -input 同源。翻译只此一份——两口各写一份解析，同一个字段名会在两处各自演化，
// 而登记方看到的「形状」从此取决于他走哪个口。
//
// 它不是接入渠道 Intake，也顶替不了：Intake 还要认操作者、定租户，那两件归操作者渠道
// （ADR-0100，真 Intake 未就位）。本包只认字节到命令这一段，不读任何身份，因此拿它拼不出
// 一个采信自报租户的实现。
package registrationjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 翻译严格且零默认：未知字段拒收（打错字段名不得静默变成「没给」）、有构造门的标识在
// 这里就拒、其余内容原样递给用例门——缺版本号、缺发布批准责任、条目撞键、缺收讫
// 时刻那类判据在用例，这里绝不代填。
//
// 输入里没有任何通道技术身份字段：那是身份双轨的第①轨，由入口自取（见
// parcel-ve-register 的 currentChannelIdentity），不可由参数传入或覆盖；这里翻译的
// approvedBy 是第②轨——登记内容，册面语义是「登记者声明了谁批准」。

func decodeStrict(raw []byte, document any, what string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(document); err != nil {
		return fmt.Errorf("%s输入不是本入口的形状：%w", what, err)
	}
	return nil
}

// versionHeaderDocument 是三份区间型目录（映射/分诊/披露）共用的抬头字段。
// effectiveTo 缺席即未闭区间（当前版本）——用指针区分「没给」与零值，零时刻是一个
// 合法的绝对时刻，不拿它兼作「没有终点」。
type versionHeaderDocument struct {
	TenantID      string     `json:"tenantId"`
	Version       string     `json:"version"`
	ApprovedBy    string     `json:"approvedBy"`
	EffectiveFrom time.Time  `json:"effectiveFrom"`
	EffectiveTo   *time.Time `json:"effectiveTo,omitempty"`
}

func (document versionHeaderDocument) header() ports.CatalogVersionHeader {
	header := ports.CatalogVersionHeader{
		Version:       document.Version,
		ApprovedBy:    document.ApprovedBy,
		EffectiveFrom: document.EffectiveFrom,
	}
	if document.EffectiveTo != nil {
		header.EffectiveTo = *document.EffectiveTo
		header.HasEffectiveTo = true
	}
	return header
}

// sourceContextFromToken 把输入词译成封闭五值。词形取 domain.SourceContext.String()
// 的原词（与目录列取值同源），不自造第二套拼法。
func sourceContextFromToken(token string) (domain.SourceContext, error) {
	for _, candidate := range []domain.SourceContext{
		domain.SourceParcelShipment,
		domain.SourceNetworkRouting,
		domain.SourceNodeOperations,
		domain.SourceTransportFulfillment,
		domain.SourceCustomsCompliance,
	} {
		if candidate.String() == token {
			return candidate, nil
		}
	}
	return domain.SourceContextInvalid, fmt.Errorf("未知源上下文 %q", token)
}

// triageOutcomeFromToken 把输入词译成封闭四走向，词形取 domain.TriageOutcome.String()。
func triageOutcomeFromToken(token string) (domain.TriageOutcome, error) {
	for _, candidate := range []domain.TriageOutcome{
		domain.AttachToExistingCase,
		domain.AutoEstablishCase,
		domain.ManualReviewRequired,
		domain.NoCaseNeeded,
	} {
		if candidate.String() == token {
			return candidate, nil
		}
	}
	return domain.TriageOutcomeInvalid, fmt.Errorf("未知分诊走向 %q", token)
}

type milestoneEntryDocument struct {
	Source    string `json:"source"`
	FactKind  string `json:"factKind"`
	Milestone string `json:"milestone"`
}

type milestoneMappingDocument struct {
	versionHeaderDocument
	Entries []milestoneEntryDocument `json:"entries"`
}

func MilestoneMappingFromJSON(raw []byte) (application.RegisterMilestoneMappingCommand, error) {
	none := application.RegisterMilestoneMappingCommand{}
	var document milestoneMappingDocument
	if err := decodeStrict(raw, &document, "里程碑映射"); err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	entries := make([]ports.MilestoneMappingEntry, 0, len(document.Entries))
	for _, entry := range document.Entries {
		source, err := sourceContextFromToken(entry.Source)
		if err != nil {
			return none, err
		}
		kind, err := domain.NewSourceFactKind(entry.FactKind)
		if err != nil {
			return none, err
		}
		milestone, err := domain.NewMilestoneReference(entry.Milestone)
		if err != nil {
			return none, err
		}
		entries = append(entries, ports.MilestoneMappingEntry{
			Source:    source,
			Kind:      kind,
			Milestone: milestone,
		})
	}
	return application.RegisterMilestoneMappingCommand{
		TenantID: tenant,
		Header:   document.header(),
		Entries:  entries,
	}, nil
}

// triageEntryDocument 的 team 只随 AUTO_ESTABLISH 在场（ports.TriageRuleEntry）；成对与否
// 的判据在用例（ENTRY_INCOMPLETE），这里只把在场的字面译成引用、缺席的留零值。
type triageEntryDocument struct {
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	Outcome    string `json:"outcome"`
	Team       string `json:"team,omitempty"`
}

type triageRulesDocument struct {
	versionHeaderDocument
	Entries []triageEntryDocument `json:"entries"`
}

func TriageRulesFromJSON(raw []byte) (application.RegisterTriageRulesCommand, error) {
	none := application.RegisterTriageRulesCommand{}
	var document triageRulesDocument
	if err := decodeStrict(raw, &document, "分诊规则"); err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	entries := make([]ports.TriageRuleEntry, 0, len(document.Entries))
	for _, entry := range document.Entries {
		kind, err := domain.NewExceptionSignalKindReference(entry.Kind)
		if err != nil {
			return none, err
		}
		confidence, err := domain.NewConfidenceReference(entry.Confidence)
		if err != nil {
			return none, err
		}
		outcome, err := triageOutcomeFromToken(entry.Outcome)
		if err != nil {
			return none, err
		}
		var team domain.ResponsibleTeamReference
		if strings.TrimSpace(entry.Team) != "" {
			if team, err = domain.NewResponsibleTeamReference(entry.Team); err != nil {
				return none, err
			}
		}
		entries = append(entries, ports.TriageRuleEntry{
			Kind:       kind,
			Confidence: confidence,
			Outcome:    outcome,
			Team:       team,
		})
	}
	return application.RegisterTriageRulesCommand{
		TenantID: tenant,
		Header:   document.header(),
		Entries:  entries,
	}, nil
}

// notificationPolicyDocument 没有版本抬头：这份目录以（租户+披露策略引用）为键，
// 版本化由引用值本身承担，换版即换引用（ports.NotificationPolicyRegistration）。
// deadlineAfter 收 Go 时长字面（如 "72h"）——合同写的是「披露后 N 小时内」这样的
// 相对量，绝对截止点在登记期根本不存在。
type notificationPolicyDocument struct {
	TenantID      string `json:"tenantId"`
	Policy        string `json:"policy"`
	Channel       string `json:"channel"`
	DeadlineAfter string `json:"deadlineAfter"`
	Obligation    string `json:"obligation"`
	ApprovedBy    string `json:"approvedBy"`
}

func NotificationPolicyFromJSON(raw []byte) (application.RegisterNotificationPolicyCommand, error) {
	none := application.RegisterNotificationPolicyCommand{}
	var document notificationPolicyDocument
	if err := decodeStrict(raw, &document, "通知策略"); err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	policy, err := domain.NewDisclosurePolicyReference(document.Policy)
	if err != nil {
		return none, err
	}
	channel, err := domain.NewNotificationChannelReference(document.Channel)
	if err != nil {
		return none, err
	}
	obligation, err := domain.NewDisclosurePolicyReference(document.Obligation)
	if err != nil {
		return none, err
	}
	deadline, err := time.ParseDuration(document.DeadlineAfter)
	if err != nil {
		return none, fmt.Errorf("时限 %q 不是时长字面（如 \"72h\"）：%w", document.DeadlineAfter, err)
	}
	// 时限必须为正的判据在用例（非正时限的通知一生成就已逾期），这里只管形状。
	return application.RegisterNotificationPolicyCommand{
		TenantID:      tenant,
		Policy:        policy,
		Channel:       channel,
		DeadlineAfter: deadline,
		Obligation:    obligation,
		ApprovedBy:    document.ApprovedBy,
	}, nil
}

type claimEligibilityDocument struct {
	TenantID     string   `json:"tenantId"`
	Version      string   `json:"version"`
	ApprovedBy   string   `json:"approvedBy"`
	Contract     string   `json:"contract"`
	CoveredKinds []string `json:"coveredKinds"`
}

func ClaimEligibilityFromJSON(raw []byte) (application.RegisterClaimEligibilityCommand, error) {
	none := application.RegisterClaimEligibilityCommand{}
	var document claimEligibilityDocument
	if err := decodeStrict(raw, &document, "索赔资格声明"); err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	contract, err := domain.NewContractScopeReference(document.Contract)
	if err != nil {
		return none, err
	}
	kinds := make([]domain.ClaimKindReference, 0, len(document.CoveredKinds))
	for _, kind := range document.CoveredKinds {
		reference, err := domain.NewClaimKindReference(kind)
		if err != nil {
			return none, err
		}
		kinds = append(kinds, reference)
	}
	// 空覆盖集不在这里拦：用例已把它按「通向永久格」拒绝（ENTRIES_MISSING），这里
	// 再拦一道只会让同一判据长在两处。
	return application.RegisterClaimEligibilityCommand{
		TenantID: tenant,
		Header: ports.CatalogApprovalHeader{
			Version:    document.Version,
			ApprovedBy: document.ApprovedBy,
		},
		Contract:     contract,
		CoveredKinds: kinds,
	}, nil
}

// claimAuthorizationDocument 的名单收指针：空名单是「这个账户目前不授权任何人代提」
// 的显式声明（用例允许），缺字段是漏填——两者恢复动作不同，不许压成一格。
type claimAuthorizationDocument struct {
	TenantID   string    `json:"tenantId"`
	Version    string    `json:"version"`
	ApprovedBy string    `json:"approvedBy"`
	Customer   string    `json:"customer"`
	Applicants *[]string `json:"applicants"`
}

func ClaimAuthorizationFromJSON(raw []byte) (application.RegisterClaimAuthorizationCommand, error) {
	none := application.RegisterClaimAuthorizationCommand{}
	var document claimAuthorizationDocument
	if err := decodeStrict(raw, &document, "申请人授权名单"); err != nil {
		return none, err
	}
	if document.Applicants == nil {
		return none, fmt.Errorf("申请人名单字段必须在场（不授权任何人写 []）——缺字段是漏填，不当显式空名单收")
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	customer, err := domain.NewCustomerAccountReference(document.Customer)
	if err != nil {
		return none, err
	}
	applicants := make([]domain.ApplicantReference, 0, len(*document.Applicants))
	for _, applicant := range *document.Applicants {
		reference, err := domain.NewApplicantReference(applicant)
		if err != nil {
			return none, err
		}
		applicants = append(applicants, reference)
	}
	return application.RegisterClaimAuthorizationCommand{
		TenantID: tenant,
		Header: ports.CatalogApprovalHeader{
			Version:    document.Version,
			ApprovedBy: document.ApprovedBy,
		},
		Customer:   customer,
		Applicants: applicants,
	}, nil
}

// dimensionDocument 是披露条目的一维：state 取封闭三态原词（SHOWN /
// PENDING_CONFIRMATION / NOT_DISCLOSED），content 只在 SHOWN 时在场。
type dimensionDocument struct {
	State   string `json:"state"`
	Content string `json:"content,omitempty"`
}

// dimensionFromDocument 经领域构造器成维：展示必带内容、待确认与不展示必不带——
// 带了就是矛盾输入，拒收而不是静默丢弃（丢弃等于替登记方改了声明）。
func dimensionFromDocument(document dimensionDocument, dimension string) (domain.ViewDimension, error) {
	none := domain.ViewDimension{}
	switch document.State {
	case domain.DimensionShown.String():
		content, err := domain.NewViewContentReference(document.Content)
		if err != nil {
			return none, fmt.Errorf("%s 维声明展示但缺内容来处：%w", dimension, err)
		}
		return domain.ShowDimension(content)
	case domain.DimensionPendingConfirmation.String():
		if strings.TrimSpace(document.Content) != "" {
			return none, fmt.Errorf("%s 维声明待确认却带了内容——矛盾输入不收", dimension)
		}
		return domain.PendDimension(), nil
	case domain.DimensionNotDisclosed.String():
		if strings.TrimSpace(document.Content) != "" {
			return none, fmt.Errorf("%s 维声明不展示却带了内容——矛盾输入不收", dimension)
		}
		return domain.WithholdDimension(), nil
	default:
		return none, fmt.Errorf("%s 维状态 %q 不在封闭三态内", dimension, document.State)
	}
}

type disclosureEntryDocument struct {
	Customer   string            `json:"customer"`
	Milestones dimensionDocument `json:"milestones"`
	ETA        dimensionDocument `json:"eta"`
	Final      dimensionDocument `json:"final"`
	Note       dimensionDocument `json:"note"`
}

type disclosurePolicyDocument struct {
	versionHeaderDocument
	Entries []disclosureEntryDocument `json:"entries"`
}

// materialReceiptDocument 是一笔收讫登记的输入：五件行身份加经手声明。receivedAt
// 取材料实际收讫的业务时刻（行身份的一件），不由本入口的时钟代填；receivedBy 是
// 身份双轨的第②轨，册面语义是「登记者声明了谁经手收讫」。
type materialReceiptDocument struct {
	TenantID   string    `json:"tenantId"`
	Batch      string    `json:"batch"`
	Item       string    `json:"item"`
	Material   string    `json:"material"`
	ReceivedAt time.Time `json:"receivedAt"`
	ReceivedBy string    `json:"receivedBy"`
}

// receiptIdentityFromDocument 折五件行身份里带构造门的四件。零值时刻放行——缺时刻
// 的判据在用例（TIME_MISSING），这里只管形状。
func receiptIdentityFromDocument(
	tenantValue, batchValue, itemValue, materialValue string,
) (domain.TenantID, domain.ClaimBatchReference, domain.ClaimItemID, domain.MaterialRequirementReference, error) {
	var (
		none         domain.TenantID
		noneBatch    domain.ClaimBatchReference
		noneItem     domain.ClaimItemID
		noneMaterial domain.MaterialRequirementReference
	)
	tenant, err := domain.NewTenantID(tenantValue)
	if err != nil {
		return none, noneBatch, noneItem, noneMaterial, err
	}
	batch, err := domain.NewClaimBatchReference(batchValue)
	if err != nil {
		return none, noneBatch, noneItem, noneMaterial, err
	}
	item, err := domain.NewClaimItemID(itemValue)
	if err != nil {
		return none, noneBatch, noneItem, noneMaterial, err
	}
	material, err := domain.NewMaterialRequirementReference(materialValue)
	if err != nil {
		return none, noneBatch, noneItem, noneMaterial, err
	}
	return tenant, batch, item, material, nil
}

func MaterialReceiptFromJSON(raw []byte) (application.RegisterMaterialReceiptCommand, error) {
	none := application.RegisterMaterialReceiptCommand{}
	var document materialReceiptDocument
	if err := decodeStrict(raw, &document, "材料收讫"); err != nil {
		return none, err
	}
	tenant, batch, item, material, err := receiptIdentityFromDocument(
		document.TenantID, document.Batch, document.Item, document.Material)
	if err != nil {
		return none, err
	}
	return application.RegisterMaterialReceiptCommand{
		TenantID:   tenant,
		Batch:      batch,
		Item:       item,
		Material:   material,
		ReceivedAt: document.ReceivedAt,
		ReceivedBy: document.ReceivedBy,
	}, nil
}

// materialReceiptRevocationDocument 以收讫行的五件全键指名撤销对象，加撤销声明两件。
type materialReceiptRevocationDocument struct {
	TenantID   string    `json:"tenantId"`
	Batch      string    `json:"batch"`
	Item       string    `json:"item"`
	Material   string    `json:"material"`
	ReceivedAt time.Time `json:"receivedAt"`
	RevokedBy  string    `json:"revokedBy"`
	RevokedAt  time.Time `json:"revokedAt"`
}

func MaterialReceiptRevocationFromJSON(raw []byte) (application.RevokeMaterialReceiptCommand, error) {
	none := application.RevokeMaterialReceiptCommand{}
	var document materialReceiptRevocationDocument
	if err := decodeStrict(raw, &document, "材料收讫撤销"); err != nil {
		return none, err
	}
	tenant, batch, item, material, err := receiptIdentityFromDocument(
		document.TenantID, document.Batch, document.Item, document.Material)
	if err != nil {
		return none, err
	}
	return application.RevokeMaterialReceiptCommand{
		TenantID:   tenant,
		Batch:      batch,
		Item:       item,
		Material:   material,
		ReceivedAt: document.ReceivedAt,
		RevokedBy:  document.RevokedBy,
		RevokedAt:  document.RevokedAt,
	}, nil
}

func DisclosurePolicyFromJSON(raw []byte) (application.RegisterDisclosurePolicyCommand, error) {
	none := application.RegisterDisclosurePolicyCommand{}
	var document disclosurePolicyDocument
	if err := decodeStrict(raw, &document, "披露策略"); err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	entries := make([]ports.DisclosurePolicyEntry, 0, len(document.Entries))
	for _, entry := range document.Entries {
		customer, err := domain.NewCustomerAccountReference(entry.Customer)
		if err != nil {
			return none, err
		}
		milestones, err := dimensionFromDocument(entry.Milestones, "milestones")
		if err != nil {
			return none, err
		}
		eta, err := dimensionFromDocument(entry.ETA, "eta")
		if err != nil {
			return none, err
		}
		final, err := dimensionFromDocument(entry.Final, "final")
		if err != nil {
			return none, err
		}
		note, err := dimensionFromDocument(entry.Note, "note")
		if err != nil {
			return none, err
		}
		entries = append(entries, ports.DisclosurePolicyEntry{
			Customer:   customer,
			Milestones: milestones,
			ETA:        eta,
			Final:      final,
			Note:       note,
		})
	}
	return application.RegisterDisclosurePolicyCommand{
		TenantID: tenant,
		Header:   document.header(),
		Entries:  entries,
	}, nil
}

// exceptionDisclosureRuleEntryDocument 是异常披露规则（0023）的一条条目。字段名照读面
// /visibility-catalogues?kind=EXCEPTION_DISCLOSURE_RULE 落下的原词（票
// ve-disclosure-policy-view/03），登记方对着读签写快照不必换词；分诊那册写口沿用更早的
// kind 一词，历史形状不在本票改。content 只在 disclosable 时在场——成对与否的判据在用例
// （ENTRY_INCOMPLETE），这里只把在场的字面译成引用、缺席的留零值，与分诊条目的 team 同一
// 条处理。
type exceptionDisclosureRuleEntryDocument struct {
	Customer    string `json:"customer"`
	SignalKind  string `json:"signalKind"`
	Confidence  string `json:"confidence"`
	Disclosable bool   `json:"disclosable"`
	AutoRelease bool   `json:"autoRelease"`
	Content     string `json:"content,omitempty"`
}

type exceptionDisclosureRulesDocument struct {
	versionHeaderDocument
	Entries []exceptionDisclosureRuleEntryDocument `json:"entries"`
}

func ExceptionDisclosureRulesFromJSON(raw []byte) (application.RegisterExceptionDisclosureRulesCommand, error) {
	none := application.RegisterExceptionDisclosureRulesCommand{}
	var document exceptionDisclosureRulesDocument
	if err := decodeStrict(raw, &document, "异常披露规则"); err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	entries := make([]ports.ExceptionDisclosureRuleEntry, 0, len(document.Entries))
	for _, entry := range document.Entries {
		customer, err := domain.NewCustomerAccountReference(entry.Customer)
		if err != nil {
			return none, err
		}
		kind, err := domain.NewExceptionSignalKindReference(entry.SignalKind)
		if err != nil {
			return none, err
		}
		confidence, err := domain.NewConfidenceReference(entry.Confidence)
		if err != nil {
			return none, err
		}
		var content domain.DisclosureContentReference
		if strings.TrimSpace(entry.Content) != "" {
			if content, err = domain.NewDisclosureContentReference(entry.Content); err != nil {
				return none, err
			}
		}
		entries = append(entries, ports.ExceptionDisclosureRuleEntry{
			Customer:    customer,
			Kind:        kind,
			Confidence:  confidence,
			Disclosable: entry.Disclosable,
			AutoRelease: entry.AutoRelease,
			Content:     content,
		})
	}
	return application.RegisterExceptionDisclosureRulesCommand{
		TenantID: tenant,
		Header:   document.header(),
		Entries:  entries,
	}, nil
}

// conflictSignalRuleDocument 没有版本抬头：这份目录一租户一条、换版是治理动作（0025），
// version 在这册里就是识别规则版本引用。字段名同样照读面原词。
type conflictSignalRuleDocument struct {
	TenantID   string `json:"tenantId"`
	SignalKind string `json:"signalKind"`
	Version    string `json:"version"`
	Confidence string `json:"confidence"`
	ApprovedBy string `json:"approvedBy"`
}

func ConflictSignalRuleFromJSON(raw []byte) (application.RegisterConflictSignalRuleCommand, error) {
	none := application.RegisterConflictSignalRuleCommand{}
	var document conflictSignalRuleDocument
	if err := decodeStrict(raw, &document, "冲突信号规则"); err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	kind, err := domain.NewExceptionSignalKindReference(document.SignalKind)
	if err != nil {
		return none, err
	}
	rule, err := domain.NewSignalRuleVersionReference(document.Version)
	if err != nil {
		return none, err
	}
	confidence, err := domain.NewConfidenceReference(document.Confidence)
	if err != nil {
		return none, err
	}
	// 批准责任缺件的判据在用例（APPROVAL_MISSING），这里只管形状。
	return application.RegisterConflictSignalRuleCommand{
		TenantID:   tenant,
		Kind:       kind,
		Rule:       rule,
		Confidence: confidence,
		ApprovedBy: document.ApprovedBy,
	}, nil
}
