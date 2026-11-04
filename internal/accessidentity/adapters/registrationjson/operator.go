// Package registrationjson 是操作者册登记载荷的 JSON 形状与它到领域类型的翻译，受控命令行（parcel-access-register）与在线
// 写口（adapters/http）共用同一份：决定种类只属「运营决定」、作业范围只属「作业事实登记」、生效起点不代填这几条规则只写在
// 这里，两个口不会各养一套（ADR-0363 决定二「CLI 与端点消费同一个 OperatorRegistrar」的翻译半边）。租户不在单项里：命令行
// 从批文头取，在线口从信封取。
package registrationjson

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// OperatorItem 是登一个操作者主体的单项。
type OperatorItem struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	Basis   string `json:"basis"`
}

// WorkScope 是作业范围的形状：种类与该租户给它的引用。
type WorkScope struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

// GrantItem 是登一笔授予的单项。决定种类与作业范围各只属一格，别的格带了即拒（GrantFrom）。
type GrantItem struct {
	GrantID           string     `json:"grantId"`
	Issuer            string     `json:"issuer"`
	Subject           string     `json:"subject"`
	CapabilityFace    string     `json:"capabilityFace"`
	DecisionKind      string     `json:"decisionKind,omitempty"`
	WorkScope         *WorkScope `json:"workScope,omitempty"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
	Basis             string     `json:"basis"`
}

// RevocationItem 是撤一笔授予的单项。
type RevocationItem struct {
	GrantID   string    `json:"grantId"`
	RevokedAt time.Time `json:"revokedAt"`
	Basis     string    `json:"basis"`
}

// BindingFrom 译一个操作者主体的绑定：逐字段过 accessidentity 的构造门，一格不代填。
func BindingFrom(tenantID string, entry OperatorItem) (accessidentity.OperatorBinding, error) {
	subject, err := accessidentity.NewOperatorSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.OperatorBinding{}, err
	}
	return accessidentity.NewOperatorBinding(subject, tenantID, entry.Basis)
}

// GrantFrom 译一笔授予。
func GrantFrom(tenantID string, entry GrantItem) (accessidentity.OperatorGrant, error) {
	subject, err := accessidentity.NewOperatorSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.OperatorGrant{}, err
	}
	face, err := accessidentity.ParseCapabilityFace(entry.CapabilityFace)
	if err != nil {
		return accessidentity.OperatorGrant{}, fmt.Errorf("capabilityFace %q：%w", entry.CapabilityFace, err)
	}
	interval, err := IntervalFrom(entry.EffectiveStartsAt, entry.EffectiveEndsAt)
	if err != nil {
		return accessidentity.OperatorGrant{}, err
	}
	// 决定种类只属「运营决定」一格（ADR-0151 决定一），作业范围只属「作业事实登记」一格（ADR-0149 决定二）：该带的缺了
	// 要拒，别的格带了也要拒——带着种类或范围的登记册配置写授予，读批文的人会以为它只授了某一种决定、某一个节点。
	if face != accessidentity.CapabilityOperationDecision && entry.DecisionKind != "" {
		return accessidentity.OperatorGrant{}, fmt.Errorf("decisionKind 只属 OPERATION_DECISION，%s 不带决定种类", face)
	}
	if face != accessidentity.CapabilityWorkFactRegistration && entry.WorkScope != nil {
		return accessidentity.OperatorGrant{}, fmt.Errorf("workScope 只属 WORK_FACT_REGISTRATION，%s 不带作业范围", face)
	}
	switch face {
	case accessidentity.CapabilityOperationDecision:
		kind, err := accessidentity.ParseDecisionKind(entry.DecisionKind)
		if err != nil {
			return accessidentity.OperatorGrant{}, fmt.Errorf("decisionKind %q：%w", entry.DecisionKind, err)
		}
		return accessidentity.NewOperationDecisionGrant(tenantID, entry.GrantID, subject, kind, interval, entry.Basis)
	case accessidentity.CapabilityWorkFactRegistration:
		scope, err := WorkScopeFrom(entry.WorkScope)
		if err != nil {
			return accessidentity.OperatorGrant{}, err
		}
		return accessidentity.NewWorkFactRegistrationGrant(tenantID, entry.GrantID, subject, scope, interval, entry.Basis)
	}
	return accessidentity.NewOperatorGrant(tenantID, entry.GrantID, subject, face, interval, entry.Basis)
}

// IntervalFrom 译授予的生效区间，各本册的授予共用：起点不代填。
func IntervalFrom(startsAt time.Time, endsAt *time.Time) (accessidentity.EffectiveInterval, error) {
	if startsAt.IsZero() {
		return accessidentity.EffectiveInterval{}, fmt.Errorf("effectiveStartsAt 缺席：生效起点不代填")
	}
	end := time.Time{}
	if endsAt != nil {
		// 显式给零值与缺席在领域里同义（不设终点），批文里却是两种写法；拒掉前者，免得一个写坏的
		// 终点被静静读成无限期。
		if endsAt.IsZero() {
			return accessidentity.EffectiveInterval{}, fmt.Errorf("effectiveEndsAt 给了零值：不设终点请省略该字段")
		}
		end = *endsAt
	}
	return accessidentity.NewEffectiveInterval(startsAt, end)
}

// WorkScopeFrom 译一个作业范围：缺席即拒，范围不代填。
func WorkScopeFrom(document *WorkScope) (accessidentity.WorkScope, error) {
	if document == nil {
		return accessidentity.WorkScope{}, fmt.Errorf("workScope 缺席：作业范围不代填")
	}
	kind, err := accessidentity.ParseWorkScopeKind(document.Kind)
	if err != nil {
		return accessidentity.WorkScope{}, fmt.Errorf("workScope.kind %q：%w", document.Kind, err)
	}
	scope, err := accessidentity.NewWorkScope(kind, document.Reference)
	if err != nil {
		return accessidentity.WorkScope{}, fmt.Errorf("workScope.reference：%w", err)
	}
	return scope, nil
}

// RevocationFrom 译一笔撤销：撤销时刻不代填。
func RevocationFrom(tenantID string, entry RevocationItem) (accessidentity.GrantRevocation, error) {
	if entry.RevokedAt.IsZero() {
		return accessidentity.GrantRevocation{}, fmt.Errorf("revokedAt 缺席：撤销时刻不代填")
	}
	return accessidentity.NewGrantRevocation(tenantID, entry.GrantID, entry.RevokedAt, entry.Basis)
}
