package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 本文件把各种登记输入 JSON 折成领域规格。翻译严格且零默认：未知字段拒收（打错
// 字段名不得静默变成「没给」）、有构造门的标识在这里就拒、其余内容原样递给领域门
// ——九件缺一、恢复四件那类判据在域，这里绝不代填。
//
// 输入里没有任何通道技术身份字段：那是身份双轨的第①轨，由入口自取（见 main.go
// 的 currentChannelIdentity），不可由参数传入或覆盖；这里翻译的 executedBy /
// decidedBy 是第②轨——登记内容，册面语义是「登记者声明了谁」。

type authorityIntervalDocument struct {
	ObjectScope string     `json:"objectScope"`
	Capability  string     `json:"capability"`
	FactKind    string     `json:"factKind"`
	Authority   string     `json:"authority"`
	FromAt      time.Time  `json:"fromAt"`
	ToAt        *time.Time `json:"toAt,omitempty"`
}

func authorityIntervalFromJSON(raw []byte) (domain.AuthorityInterval, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document authorityIntervalDocument
	if err := decoder.Decode(&document); err != nil {
		return domain.AuthorityInterval{}, fmt.Errorf("权威区间输入不是本入口的形状：%w", err)
	}
	interval := domain.AuthorityInterval{
		ObjectScope: document.ObjectScope,
		Capability:  document.Capability,
		FactKind:    document.FactKind,
		Authority:   document.Authority,
		From:        document.FromAt,
	}
	if document.ToAt != nil {
		interval.To = *document.ToAt
	}
	return interval, nil
}

type suspensionDocument struct {
	SuspensionID  string    `json:"suspensionId"`
	TriggerSource string    `json:"triggerSource"`
	Basis         string    `json:"basis"`
	Evidence      string    `json:"evidence"`
	Scope         string    `json:"scope"`
	ExecutedBy    string    `json:"executedBy"`
	OccurredAt    time.Time `json:"occurredAt"`
	EffectiveAt   time.Time `json:"effectiveAt"`
	InTransitNote string    `json:"inTransitNote"`
}

func suspensionSpecFromJSON(raw []byte) (domain.SuspensionDecisionSpec, error) {
	none := domain.SuspensionDecisionSpec{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document suspensionDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("暂停输入不是本入口的形状：%w", err)
	}

	spec := domain.SuspensionDecisionSpec{
		TriggerSource: document.TriggerSource,
		Basis:         document.Basis,
		Evidence:      document.Evidence,
		ExecutedBy:    document.ExecutedBy,
		OccurredAt:    document.OccurredAt,
		EffectiveAt:   document.EffectiveAt,
		InTransitNote: document.InTransitNote,
	}
	var err error
	if spec.ID, err = domain.NewSuspensionID(document.SuspensionID); err != nil {
		return none, err
	}
	if spec.Scope, err = domain.NewScopeVersionReference(document.Scope); err != nil {
		return none, err
	}
	return spec, nil
}

type inventoryEntryDocument struct {
	ObjectIdentity   string    `json:"objectIdentity"`
	CurrentFacts     string    `json:"currentFacts"`
	CurrentAuthority string    `json:"currentAuthority"`
	ResponsibleParty string    `json:"responsibleParty"`
	NextAction       string    `json:"nextAction"`
	ReviewBy         time.Time `json:"reviewBy"`
}

type inventoryDocument struct {
	TakenAt time.Time                `json:"takenAt"`
	Entries []inventoryEntryDocument `json:"entries"`
}

type resumptionDocument struct {
	SuspensionID     string            `json:"suspensionId"`
	ReleaseEvidence  string            `json:"releaseEvidence"`
	ConsistencyCheck string            `json:"consistencyCheck"`
	Inventory        inventoryDocument `json:"inventory"`
	DecidedBy        string            `json:"decidedBy"`
	DecidedAt        time.Time         `json:"decidedAt"`
	EffectiveAt      time.Time         `json:"effectiveAt"`
}

func resumptionSpecFromJSON(raw []byte) (domain.ResumptionDecisionSpec, error) {
	none := domain.ResumptionDecisionSpec{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document resumptionDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("恢复输入不是本入口的形状：%w", err)
	}

	entries := make([]domain.InventoryEntry, 0, len(document.Inventory.Entries))
	for _, entry := range document.Inventory.Entries {
		entries = append(entries, domain.InventoryEntry{
			ObjectIdentity:   entry.ObjectIdentity,
			CurrentFacts:     entry.CurrentFacts,
			CurrentAuthority: entry.CurrentAuthority,
			ResponsibleParty: entry.ResponsibleParty,
			NextAction:       entry.NextAction,
			ReviewBy:         entry.ReviewBy,
		})
	}
	// 盘点门在翻译处就过：条目六件齐全、身份不重、盘点时刻在场——恢复四件里的
	// 这一件不给任何「只填标识」的简化路径。
	inventory, err := domain.TakeInventory(entries, document.Inventory.TakenAt)
	if err != nil {
		return none, err
	}

	spec := domain.ResumptionDecisionSpec{
		ReleaseEvidence:  document.ReleaseEvidence,
		ConsistencyCheck: document.ConsistencyCheck,
		Inventory:        inventory,
		DecidedBy:        document.DecidedBy,
		DecidedAt:        document.DecidedAt,
		EffectiveAt:      document.EffectiveAt,
	}
	if spec.Suspension, err = domain.NewSuspensionID(document.SuspensionID); err != nil {
		return none, err
	}
	return spec, nil
}

type candidateSetDocument struct {
	SetID      string    `json:"setId"`
	Scope      string    `json:"scope"`
	Parameters string    `json:"parameters"`
	Rules      string    `json:"rules"`
	FormedAt   time.Time `json:"formedAt"`
}

type deviationDocument struct {
	Scope         string    `json:"scope"`
	Control       string    `json:"control"`
	Owner         string    `json:"owner"`
	CloseBy       time.Time `json:"closeBy"`
	ResidualRisk  string    `json:"residualRisk"`
	AcceptanceRef string    `json:"acceptanceRef"`
}

type stageReviewDecisionDocument struct {
	Stage        string              `json:"stage"`
	Objective    string              `json:"objective"`
	EvidencePack string              `json:"evidencePack"`
	Verdict      string              `json:"verdict"`
	Disposition  string              `json:"disposition,omitempty"`
	Deviations   []deviationDocument `json:"deviations,omitempty"`
	DecidedBy    string              `json:"decidedBy"`
	DecidedAt    time.Time           `json:"decidedAt"`
	EffectiveAt  time.Time           `json:"effectiveAt"`
}

type coverageDocument struct {
	Predecessor string `json:"predecessor"`
	Kind        string `json:"kind"`
}

type stageReviewDocument struct {
	CandidateSet    candidateSetDocument        `json:"candidateSet"`
	Review          stageReviewDecisionDocument `json:"review"`
	GrantedInterval *authorityIntervalDocument  `json:"grantedInterval,omitempty"`
	Coverage        []coverageDocument          `json:"coverage,omitempty"`
}

// stageReviewInput 是一份阶段评审输入的两半：本次评审固定的候选版本组，与评审命令本身。
// 评审的范围版本与候选组引用都取自这一份候选组——评审评的就是这组，输入里不另给第二份，
// 免得两处写得对不上。
type stageReviewInput struct {
	candidateSet domain.CandidateVersionSet
	command      application.RecordStageReviewCommand
}

func stageReviewFromJSON(raw []byte) (stageReviewInput, error) {
	none := stageReviewInput{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document stageReviewDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("阶段评审输入不是本入口的形状：%w", err)
	}

	setID, err := domain.NewCandidateVersionSetID(document.CandidateSet.SetID)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewScopeVersionReference(document.CandidateSet.Scope)
	if err != nil {
		return none, err
	}
	parameters, err := domain.NewParameterSnapshotReference(document.CandidateSet.Parameters)
	if err != nil {
		return none, err
	}
	rules, err := domain.NewRuleVersionsReference(document.CandidateSet.Rules)
	if err != nil {
		return none, err
	}
	set, err := domain.FixCandidateVersionSet(setID, scope, parameters, rules, document.CandidateSet.FormedAt)
	if err != nil {
		return none, err
	}

	stage, known := executionStageNamed(document.Review.Stage)
	if !known {
		return none, fmt.Errorf("未知执行阶段 %q", document.Review.Stage)
	}
	verdict, known := reviewVerdictNamed(document.Review.Verdict)
	if !known {
		return none, fmt.Errorf("未知评审结论 %q", document.Review.Verdict)
	}
	// 处理方式缺席是 Go 的正当形状；写了就必须是封闭四值之一——Go 带处理方式、No-Go 缺处理
	// 方式都由领域门拒，这里不替它补。
	disposition := domain.NoGoDispositionInvalid
	if document.Review.Disposition != "" {
		if disposition, known = noGoDispositionNamed(document.Review.Disposition); !known {
			return none, fmt.Errorf("未知 No-Go 处理方式 %q", document.Review.Disposition)
		}
	}
	deviations := make([]domain.AcceptedDeviation, 0, len(document.Review.Deviations))
	for _, deviation := range document.Review.Deviations {
		deviations = append(deviations, domain.AcceptedDeviation{
			Scope:         deviation.Scope,
			Control:       deviation.Control,
			Owner:         deviation.Owner,
			CloseBy:       deviation.CloseBy,
			ResidualRisk:  deviation.ResidualRisk,
			AcceptanceRef: deviation.AcceptanceRef,
		})
	}

	command := application.RecordStageReviewCommand{
		Review: domain.StageReviewDecisionSpec{
			Stage:        stage,
			Objective:    document.Review.Objective,
			Scope:        set.Scope(),
			Candidates:   set.ID(),
			EvidencePack: document.Review.EvidencePack,
			Verdict:      verdict,
			Disposition:  disposition,
			Deviations:   deviations,
			DecidedBy:    document.Review.DecidedBy,
			DecidedAt:    document.Review.DecidedAt,
			EffectiveAt:  document.Review.EffectiveAt,
		},
	}
	if document.GrantedInterval != nil {
		granted := domain.AuthorityInterval{
			ObjectScope: document.GrantedInterval.ObjectScope,
			Capability:  document.GrantedInterval.Capability,
			FactKind:    document.GrantedInterval.FactKind,
			Authority:   document.GrantedInterval.Authority,
			From:        document.GrantedInterval.FromAt,
		}
		if document.GrantedInterval.ToAt != nil {
			granted.To = *document.GrantedInterval.ToAt
		}
		command.GrantedInterval = &granted
	}
	for _, declaration := range document.Coverage {
		predecessor, err := domain.NewScopeVersionReference(declaration.Predecessor)
		if err != nil {
			return none, err
		}
		kind, known := scopeRelationKindNamed(declaration.Kind)
		if !known {
			return none, fmt.Errorf("未知覆盖关系种类 %q", declaration.Kind)
		}
		command.Coverage = append(command.Coverage, domain.ScopeCoverageDeclaration{
			Predecessor: predecessor,
			Kind:        kind,
		})
	}
	return stageReviewInput{candidateSet: set, command: command}, nil
}

// 下面四个是领域封闭集的名称镜像：认 String() 的原词，集合外与空串一律不认，不猜近似。

func executionStageNamed(name string) (domain.ExecutionStage, bool) {
	for _, stage := range []domain.ExecutionStage{
		domain.NotYetInExecution, domain.HistoricalReplay, domain.ShadowRun, domain.LimitedProduction,
	} {
		if stage.String() == name {
			return stage, true
		}
	}
	return domain.ExecutionStageInvalid, false
}

func reviewVerdictNamed(name string) (domain.ReviewVerdict, bool) {
	for _, verdict := range []domain.ReviewVerdict{domain.StageGo, domain.StageNoGo} {
		if verdict.String() == name {
			return verdict, true
		}
	}
	return domain.ReviewVerdictInvalid, false
}

func noGoDispositionNamed(name string) (domain.NoGoDisposition, bool) {
	for _, disposition := range []domain.NoGoDisposition{
		domain.KeepCurrentScope, domain.SuspendNewAdmission, domain.FixAndReassess, domain.ObjectLevelTakeover,
	} {
		if disposition.String() == name {
			return disposition, true
		}
	}
	return domain.NoGoDispositionInvalid, false
}

func scopeRelationKindNamed(name string) (domain.ScopeVersionRelationKind, bool) {
	for _, kind := range []domain.ScopeVersionRelationKind{domain.ScopeInheritsSuspensions, domain.ScopeUnrelated} {
		if kind.String() == name {
			return kind, true
		}
	}
	return domain.ScopeVersionRelationKindInvalid, false
}
