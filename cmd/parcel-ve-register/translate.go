package main

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

// 本文件把六种登记输入 JSON 折成应用命令。翻译严格且零默认：未知字段拒收（打错
// 字段名不得静默变成「没给」）、有构造门的标识在这里就拒、其余内容原样递给用例门
// ——缺版本号、缺发布批准责任、条目撞键那类判据在用例，这里绝不代填。
//
// 输入里没有任何通道技术身份字段：那是身份双轨的第①轨，由入口自取（见 main.go 的
// currentChannelIdentity），不可由参数传入或覆盖；这里翻译的 approvedBy 是第②轨
// ——登记内容，册面语义是「登记者声明了谁批准」。

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

func milestoneMappingFromJSON(raw []byte) (application.RegisterMilestoneMappingCommand, error) {
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

type triageEntryDocument struct {
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	Outcome    string `json:"outcome"`
}

type triageRulesDocument struct {
	versionHeaderDocument
	Entries []triageEntryDocument `json:"entries"`
}

func triageRulesFromJSON(raw []byte) (application.RegisterTriageRulesCommand, error) {
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
		entries = append(entries, ports.TriageRuleEntry{
			Kind:       kind,
			Confidence: confidence,
			Outcome:    outcome,
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

func notificationPolicyFromJSON(raw []byte) (application.RegisterNotificationPolicyCommand, error) {
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

func claimEligibilityFromJSON(raw []byte) (application.RegisterClaimEligibilityCommand, error) {
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

func claimAuthorizationFromJSON(raw []byte) (application.RegisterClaimAuthorizationCommand, error) {
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

func disclosurePolicyFromJSON(raw []byte) (application.RegisterDisclosurePolicyCommand, error) {
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
