package ports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 本文件声明「动作准入判断」（ADR-0206）与「节点关务区域」登记册的出入口。另立文件而不进 ports.go：准入判断
// 是一条新编排，它读的几口里有既有登记册（限制、门禁目录、付款核对）也有新册，放在一处读得出它到底问了谁。

// NodeCustomsZoneEntry 是节点关务区域登记册的一行：节点、区域与生效区间。AppliesUntil 零值即尚无终点（开放版），
// 与 CandidatePortEntry 同约定——终点不是登记输入，后继版本登记时落定。
type NodeCustomsZoneEntry struct {
	Node         domain.NetworkNodeReference
	Zone         domain.CustomsZoneReference
	AppliesFrom  time.Time
	AppliesUntil time.Time
}

// NodeCustomsZoneRegistry 是节点关务区域登记册的写口。区域取值属租户取值（PAR-CUS-08），放进库里的那条受控
// 路径属机制半边。版本代数照 PortsPathsRegistry：起点随登记给出并入键，终点在换版时落定，历史区间不接受
// 追改；撞键与撞重叠都折成`已登记`交回，内容是否同一份由编排读回自己比。
type NodeCustomsZoneRegistry interface {
	RegisterNodeCustomsZone(
		ctx context.Context,
		tenant domain.TenantID,
		node domain.NetworkNodeReference,
		zone domain.CustomsZoneReference,
		appliesFrom time.Time,
	) (CaseConfigurationSaveOutcome, error)
}

// NodeCustomsZoneView 按（租户，节点）在评估时点上解析区域版本（半开区间）。found=false 即该节点那一刻没有
// 在册版本——准入判断把它读作「未登记」，不拿开放版或当前时间兜底。
type NodeCustomsZoneView interface {
	LoadNodeCustomsZone(
		ctx context.Context,
		tenant domain.TenantID,
		node domain.NetworkNodeReference,
		evaluatedAt time.Time,
	) (NodeCustomsZoneEntry, bool, error)
}

// ObjectCustomsScopeView 按对象引用从本上下文自己的册子里解析关务范围（ADR-0206 决定四，先例
// ParcelDeclarationFactsView）：当前申报单元及其程序，以及在案却未成单元的程序。空册是「对象不在任何案件与
// 单元里」的如实答案，不是未决；依赖调不通作为错误返回。
type ObjectCustomsScopeView interface {
	LoadObjectCustomsScope(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.DeclaredParcelReference,
	) (domain.ObjectCustomsScope, error)
}

// RestrictionScopeView 是准入判断读限制的那一口：按范围盘出全部限制（含已解除的——那一条在动作时点是否有效，
// 由领域按时点判）。RestrictionStore 天然满足它；单立一口是让准入编排的替身不必背上限制的写口。
type RestrictionScopeView interface {
	ListByScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.DecisionScopeReference,
	) ([]domain.RegulatoryRestriction, error)
}

// ReleaseOutcomeView 按申报范围盘出已接收的放行事实，每份带其业务发生或适用时间。记了同层冲突的那份不交出：
// CC 对冲突双方都留存，当前判断不因后到的一方改变（ADR-0197 决定五）；部分与附条件放行照交——它们不等于放行，
// 怎么读归领域。
type ReleaseOutcomeView interface {
	ListReleaseOutcomes(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.DecisionScopeReference,
	) ([]domain.ReleaseReading, error)
}

type ActionAdmissionSaveOutcome uint8

const (
	ActionAdmissionSaveOutcomeInvalid ActionAdmissionSaveOutcome = iota
	ActionAdmissionSaved
	ActionAdmissionAlreadyRecorded
)

// ActionAdmissionJudgementStore 记下准入判断（写入代数同 ADR-0031，不 UPSERT）。判断标识由内容派生，同一请求在
// 同样的三源上重放命中同一行，答`已记录`；记录不入队——准入判断今天没有订阅者（ADR-0049：接得住才登记）。
type ActionAdmissionJudgementStore interface {
	SaveActionAdmissionJudgement(
		ctx context.Context,
		tenant domain.TenantID,
		judgementID string,
		judgement domain.ActionAdmissionJudgement,
	) (ActionAdmissionSaveOutcome, error)
	// FindActionAdmissionJudgedAt 交回已记录那一份的判断时刻：标识钉住了内容，册上那份与重算的这份只差这一格。
	FindActionAdmissionJudgedAt(
		ctx context.Context,
		tenant domain.TenantID,
		judgementID string,
	) (time.Time, bool, error)
}

// actionAdmissionJudgementPrefix 是准入判断标识的类别段。
const actionAdmissionJudgementPrefix = "AADJ"

// ActionAdmissionJudgementID 由判断内容派生标识：请求（租户、对象、意图、动作时点）、区域读数与逐动作的答复
// 及依据各成一行，判断时刻不进——同一请求在同样的三源上重放得同一标识，三源一变（限制解除、放行到达、区域
// 换版）答复或依据随之变、标识随之换，记成新的一份而不覆盖旧的。
func ActionAdmissionJudgementID(tenant domain.TenantID, judgement domain.ActionAdmissionJudgement) string {
	intent := judgement.Intent()
	onward, _ := intent.Onward()
	lines := []string{
		"tenant=" + tenant.String(),
		"object=" + judgement.Object().String(),
		"intent=" + intent.Kind().String() + "/" + intent.Node().String() + "/" + onward.String(),
		"at=" + judgement.At().UTC().Format(time.RFC3339Nano),
		"cross=" + judgement.CrossZone().String(),
	}
	for _, zone := range judgement.Zones() {
		value := "UNREGISTERED"
		if zone.Registered {
			value = zone.Zone.String()
		}
		lines = append(lines, "zone="+zone.Node.String()+"="+value)
	}
	for _, finding := range judgement.Findings() {
		lines = append(lines, "action="+finding.Action.String()+"="+finding.Answer.String())
		for _, basis := range finding.Bases {
			lines = append(lines, "basis="+finding.Action.String()+"|"+basis.Cause.String()+"|"+
				basis.Scope.String()+"|"+basis.Procedure.String()+"|"+strings.Join(basis.Sources, ","))
		}
	}
	lines = append(lines, "answer="+judgement.Answer().String())
	digest := sha256.Sum256([]byte(strings.Join(lines, "\x00")))
	return actionAdmissionJudgementPrefix + "-" + hex.EncodeToString(digest[:16])
}
