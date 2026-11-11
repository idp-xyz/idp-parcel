package partycommercial

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// 本文件把采用的接单规则包引用的提交条件读成依据快照上的条件集（`BD-PS-006`；ADR-0376 决定一；票 declared-goods-lines/09）。
// 规则包里 `CROSS_FIELD_CONDITION` 一类的规则引用，带 `REFCFG-1:` 前缀、指向小包托运提交条件目录的，打开那一版参考配置读出
// 条件码；不带前缀的不透明引用不判——没有执行器读得懂它，判成不满足等于替租户发明规则。条件的判法在领域
// （ShipmentRequest.RequiredDocumentChecks），这里只读引用、译成条件码。

const (
	submissionConditionsDirectory = "parcel-shipment/submission-conditions/"

	reasonRulePackageUnreadable       = "PC-RULE_PACKAGE_UNREADABLE"
	reasonSubmissionConditionsMissing = "PS-SUBMISSION_CONDITIONS_UNREADABLE"
)

// submissionConditionDocument 是提交条件参考配置的形：只读条件码，说明文字给人看、不进判断。
type submissionConditionDocument struct {
	Identifier string `json:"identifier"`
	Version    int    `json:"version"`
	Conditions []struct {
		Code string `json:"code"`
	} `json:"conditions"`
}

// SubmissionConditionsOf 把一版已发布的提交条件参考配置读成条件集，也是这一目录的构造门：条件码不在本上下文的封闭集合里、
// 原文的标识或版本与引用对不上、一条条件都没有，都答错误——这样的一版不能发布（ADR-0147 决定五），已发布的每一版都由
// 测试逐版过一遍。
func SubmissionConditionsOf(reference referenceconfig.Reference) (psdomain.SubmissionConditions, error) {
	if !strings.HasPrefix(reference.Identifier(), submissionConditionsDirectory) {
		return psdomain.SubmissionConditions{}, fmt.Errorf("%s is not a submission condition reference", reference)
	}
	raw, err := referenceconfig.Open(reference)
	if err != nil {
		return psdomain.SubmissionConditions{}, err
	}
	var document submissionConditionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return psdomain.SubmissionConditions{}, fmt.Errorf("%s: %w", reference, err)
	}
	if document.Identifier != reference.Identifier() || document.Version != reference.Version() {
		return psdomain.SubmissionConditions{}, fmt.Errorf("%s: document names %s@%d", reference, document.Identifier, document.Version)
	}
	if len(document.Conditions) == 0 {
		return psdomain.SubmissionConditions{}, fmt.Errorf("%s: no conditions", reference)
	}
	conditions := make([]psdomain.SubmissionCondition, 0, len(document.Conditions))
	for _, entry := range document.Conditions {
		condition, ok := psdomain.ParseSubmissionCondition(entry.Code)
		if !ok {
			return psdomain.SubmissionConditions{}, fmt.Errorf("%s: unknown condition %q", reference, entry.Code)
		}
		conditions = append(conditions, condition)
	}
	return psdomain.NewSubmissionConditions(conditions...)
}

// submissionConditionsFor 读采用的规则包里提交条件的引用并读成条件集。只在规则包声明了「必要资料」组适用时读：没声明就
// 不判，也就不必多一次依赖读取，既有不引用条件的规则包行为不变。
//
// 读不回规则包、或引用指的那一版打不开，交回`无法判定`的解析：依赖读不回，内部重试，不当成满足——当成满足会让一票缺
// 箱标号的货件因为一次读取失败被接受。
func (adapter *CommercialBasisAdapter) submissionConditionsFor(
	ctx context.Context,
	tenant pcdomain.TenantID,
	adopted pcdomain.AdoptedBasis,
	applicable psdomain.ApplicableCheckGroups,
) (psdomain.SubmissionConditions, string, error) {
	if !applicable.Includes(psdomain.RequiredDocumentCheck) {
		return psdomain.SubmissionConditions{}, "", nil
	}
	if adapter.rules == nil {
		return psdomain.SubmissionConditions{}, reasonRulePackageUnreadable, nil
	}
	pack, found, err := adapter.rules.LoadAcceptanceRulePackage(ctx, tenant, adopted.Version())
	if err != nil || !found {
		return psdomain.SubmissionConditions{}, reasonRulePackageUnreadable, nil
	}
	var conditions psdomain.SubmissionConditions
	for _, rule := range pack.RulesIn(pcdomain.CrossFieldConditionRules) {
		reference, claimed, err := referenceconfig.OpenCitation(rule.Reference().String())
		if !claimed {
			continue
		}
		if err != nil {
			return psdomain.SubmissionConditions{}, reasonSubmissionConditionsMissing, nil
		}
		if !strings.HasPrefix(reference.Identifier(), submissionConditionsDirectory) {
			// 引的是别的目录（不是提交条件）：不是本执行器的，照不透明引用办。
			continue
		}
		set, err := SubmissionConditionsOf(reference)
		if err != nil {
			return psdomain.SubmissionConditions{}, "", fmt.Errorf("%w: %v", ErrUntranslatableAnswer, err)
		}
		conditions = conditions.Union(set)
	}
	return conditions, "", nil
}
