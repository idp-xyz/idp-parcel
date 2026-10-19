package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ErrDispositionCommandRejected 说明处置命令本身有缺陷（缺格、空白、带 NUL、结果与指定的包裹或连带实物对不上），重发同样的内容
// 不会变好。入口据此答坏报文（ADR-0210 决定六）。
var ErrDispositionCommandRejected = errors.New("node operations: identity conflict disposition command rejected")

// ErrDispositionSubjectNotFound 说明指名的作业实物与所处置的版本在本租户的收寄册上对不上：版本不在册、属于别的租户，或不是这件
// 实物的。三者同答，与越权探针同答（ADR-0210 决定六、ADR-0029）。
var ErrDispositionSubjectNotFound = errors.New("node operations: the named unit and disposed version are not in this tenant's reception register")

// DispositionOutcome 是一次身份冲突处置的应用结果（UC-NO-004 结果语义契约）。第一格追加了处置版；其余都没落版，是形成了的答案，
// 不是错误（ADR-0022）。
type DispositionOutcome uint8

const (
	DispositionOutcomeInvalid DispositionOutcome = iota
	// IdentityConflictDisposed：首次形成，主实物与连带实物各条判断链已追加处置版。
	IdentityConflictDisposed
	// DispositionExistingResult：所处置的版本已被一次处置接续，比对格逐格相同。
	DispositionExistingResult
	// DispositionAlreadyDisposed：所处置的版本已被一次处置接续，比对格不同。
	DispositionAlreadyDisposed
	// DispositionVersionSuperseded：所处置的版本已被不是处置的版本接续。
	DispositionVersionSuperseded
	// DispositionNoIdentityConflict：主实物各链当前有效版本合起来不判身份冲突。
	DispositionNoIdentityConflict
	// DispositionNotACandidate：指定的包裹不在处置候选里。
	DispositionNotACandidate
	// DispositionAssociatedElsewhere：指定的包裹此刻关联在别的实物上，连带实物漏列、多列、版本过时，或那件实物还牵连别的包裹。
	DispositionAssociatedElsewhere
)

func (outcome DispositionOutcome) String() string {
	switch outcome {
	case IdentityConflictDisposed:
		return "IDENTITY_CONFLICT_DISPOSED"
	case DispositionExistingResult:
		return "EXISTING_RESULT"
	case DispositionAlreadyDisposed:
		return "ALREADY_DISPOSED"
	case DispositionVersionSuperseded:
		return "VERSION_SUPERSEDED"
	case DispositionNoIdentityConflict:
		return "NO_IDENTITY_CONFLICT"
	case DispositionNotACandidate:
		return "NOT_A_CANDIDATE"
	case DispositionAssociatedElsewhere:
		return "ASSOCIATED_ELSEWHERE"
	default:
		return ""
	}
}

// DisposeIdentityConflictCommand 是一次身份冲突处置（ADR-0210 决定二、四、六）。租户与提交操作者只取认证结果；操作者写作
// `OPERATOR/<发行方>#<sub>`。所处置的版本是调用方看到的那一版，也是重放身份，不拿服务端此刻的当前版本顶替。
type DisposeIdentityConflictCommand struct {
	TenantID   domain.TenantID
	Operator   string
	Unit       domain.HandlingUnitID
	Disposed   domain.IntakeResultVersion
	Result     domain.IdentityDispositionResult
	Parcel     domain.ParcelAssociationReference
	CoDisposed []domain.CoDisposedUnit
	Basis      string
}

// AssociatedUnit 是「关联对象另有归属」回显的一件：此刻关联到指定包裹的别的作业实物，连同它各条链的当前有效版本。
type AssociatedUnit struct {
	Unit            domain.HandlingUnitID
	CurrentVersions []ports.ReceptionRecord
}

// DisposeIdentityConflictResult 是一次处置的结果。各格随结果在场（UC-NO-004 结果表的「必须保存」一列）：
//   - Disposition：首次形成时是这一次，已有结果与已处置时是在册的那一次。
//   - Versions：首次形成时是追加的各版（主实物在前、连带实物在后）；已有结果与已处置时是接续所处置版本的那一版。
//   - CurrentVersions：版本已换代与无可处置的冲突时，是主实物此刻各链的当前有效版本。
//   - Candidates：不在候选之内时，是主实物的处置候选。
//   - AssociatedUnits：关联对象另有归属时，是此刻关联到指定包裹的别的实物。
//   - IntakeHandedOff：首次形成且交了收寄发布意图时，是交了意图的那一版。
type DisposeIdentityConflictResult struct {
	outcome        DispositionOutcome
	disposition    domain.IdentityDisposition
	versions       []ports.ReceptionRecord
	current        []ports.ReceptionRecord
	candidates     []domain.ParcelAssociationReference
	associated     []AssociatedUnit
	handedOff      ports.ReceptionKey
	intakeHandedIn bool
}

func (result DisposeIdentityConflictResult) Outcome() DispositionOutcome { return result.outcome }

func (result DisposeIdentityConflictResult) Disposition() (domain.IdentityDisposition, bool) {
	return result.disposition, result.disposition.Recorded()
}

func (result DisposeIdentityConflictResult) Versions() []ports.ReceptionRecord {
	return slices.Clone(result.versions)
}

func (result DisposeIdentityConflictResult) CurrentVersions() []ports.ReceptionRecord {
	return slices.Clone(result.current)
}

func (result DisposeIdentityConflictResult) Candidates() []domain.ParcelAssociationReference {
	return slices.Clone(result.candidates)
}

func (result DisposeIdentityConflictResult) AssociatedUnits() []AssociatedUnit {
	return slices.Clone(result.associated)
}

func (result DisposeIdentityConflictResult) IntakeHandedOff() (ports.ReceptionKey, bool) {
	return result.handedOff, result.intakeHandedIn
}

// DisposeIdentityConflictDeps 是处置编排的全部依赖，缺任一个构造即拒。收寄库、签发、意图交付与服务结果读口与收寄编排是同一组口：
// 处置版落在同一本册的版本链上，指定一件的那几版照 ADR-0186 取服务结果。
type DisposeIdentityConflictDeps struct {
	ServiceResults ports.ParcelServiceResultView
	Receptions     ports.ReceptionStore
	Register       ports.IdentityDispositionRegister
	Versions       ports.IntakeIdentityFactory
	Downstream     ports.NodeIntakeHandoff
	Clock          ports.Clock
}

// DisposeIdentityConflictHandler 编排身份冲突处置（UC-NO-004；ADR-0210 决定二至四、六、七）。
//
// 调用方负责事务：包裹锁、追加的各版与收寄意图必须同生共死。
type DisposeIdentityConflictHandler struct {
	deps DisposeIdentityConflictDeps
}

func NewDisposeIdentityConflictHandler(deps DisposeIdentityConflictDeps) (*DisposeIdentityConflictHandler, error) {
	switch {
	case deps.ServiceResults == nil:
		return nil, errors.New("node operations: identity disposition needs a parcel service result view")
	case deps.Receptions == nil:
		return nil, errors.New("node operations: identity disposition needs a reception store")
	case deps.Register == nil:
		return nil, errors.New("node operations: identity disposition needs a disposition register")
	case deps.Versions == nil:
		return nil, errors.New("node operations: identity disposition needs an intake result version factory")
	case deps.Downstream == nil:
		return nil, errors.New("node operations: identity disposition needs a node intake handoff")
	case deps.Clock == nil:
		return nil, errors.New("node operations: identity disposition needs a clock")
	}
	return &DisposeIdentityConflictHandler{deps: deps}, nil
}

// Dispose 走 UC-NO-004 步骤 2 至 8：认重放 → 取包裹锁 → 读主实物各链当前有效版本 → 核版本、核冲突 → 算处置候选、核指定的包裹 →
// 指定一件时按处置截断读此刻关联到它的别的实物、与连带实物对照 → 逐链追加处置版 → 需要时交一封收寄发布意图。
//
// 重放、已处置与各格业务拒绝都在签发版本之前认出，不耗版本号、不写册。没形成答案（读写失败、意图交不出去、撞上（租户，前版）唯一）
// 一律交回 error，调用方整笔回滚。
func (handler *DisposeIdentityConflictHandler) Dispose(
	ctx context.Context,
	command DisposeIdentityConflictCommand,
) (DisposeIdentityConflictResult, error) {
	if err := validateDispositionCommand(command); err != nil {
		return DisposeIdentityConflictResult{}, err
	}
	tenant := command.TenantID
	_, digest, err := domain.CanonicalizeIdentityDispositionComparison(command.Result, command.Parcel, coDisposedUnitsOf(command))
	if err != nil {
		return DisposeIdentityConflictResult{}, err
	}
	disposed, found, err := handler.deps.Register.FindByJudgmentVersion(ctx, tenant, command.Disposed)
	if err != nil {
		return DisposeIdentityConflictResult{}, fmt.Errorf("find the disposed judgment: %w", err)
	}
	if !found || disposed.Intake.ReceivedAt().IsZero() || disposed.Intake.Unit() != command.Unit {
		return DisposeIdentityConflictResult{}, ErrDispositionSubjectNotFound
	}
	if replayed, ok, err := handler.replay(ctx, tenant, command.Disposed, digest); err != nil || ok {
		return replayed, err
	}

	// 先实物后包裹（ADR-0210 决定四）：作业实物的锁未落地，只取包裹锁。要动的包裹先在锁外读出来，取锁之后再读一遍链头；锁外之后
	// 才落下的关联在下面补锁，漏锁的一格由（租户，前版）唯一兜底。
	held := map[domain.ParcelAssociationReference]bool{}
	if err := handler.holdParcels(ctx, command, held); err != nil {
		return DisposeIdentityConflictResult{}, err
	}
	rows, err := handler.deps.Register.FindUnitIntakes(ctx, tenant, command.Unit)
	if err != nil {
		return DisposeIdentityConflictResult{}, fmt.Errorf("read the disposed unit: %w", err)
	}
	heads, err := chainHeadsOf(rows)
	if err != nil {
		return DisposeIdentityConflictResult{}, err
	}
	if !slices.ContainsFunc(heads, func(head ports.ReceptionRecord) bool { return head.JudgmentVersion == command.Disposed }) {
		// 锁外认重放之后、取锁之前，另一次同版本的处置可能刚提交。
		if replayed, ok, err := handler.replay(ctx, tenant, command.Disposed, digest); err != nil || ok {
			return replayed, err
		}
		return DisposeIdentityConflictResult{outcome: DispositionVersionSuperseded, current: heads}, nil
	}
	if err := handler.holdParcels(ctx, command, held); err != nil {
		return DisposeIdentityConflictResult{}, err
	}
	conflicted, err := headsInConflict(heads)
	if err != nil {
		return DisposeIdentityConflictResult{}, err
	}
	if !conflicted {
		return DisposeIdentityConflictResult{outcome: DispositionNoIdentityConflict, current: heads}, nil
	}
	candidates := dispositionCandidatesOf(heads)
	designated := command.Result == domain.DispositionDesignatesParcel
	if designated && !slices.Contains(candidates, command.Parcel) {
		return DisposeIdentityConflictResult{outcome: DispositionNotACandidate, candidates: candidates}, nil
	}
	var coHeads []AssociatedUnit
	if designated {
		associated, matched, err := handler.associatedNow(ctx, command)
		if err != nil {
			return DisposeIdentityConflictResult{}, err
		}
		if !matched {
			return DisposeIdentityConflictResult{outcome: DispositionAssociatedElsewhere, associated: associated}, nil
		}
		coHeads = associated
	}
	return handler.append(ctx, command, digest, rows, heads, candidates, coHeads)
}

// validateDispositionCommand 照 ADR-0210 决定六验命令的形状。入口已按线格式拦过一遍；这里再拦，是为了绕过入口的调用方（替身、将来
// 的别的入口）也落不下一份自相矛盾的处置。
func validateDispositionCommand(command DisposeIdentityConflictCommand) error {
	reject := func(reason string) error { return fmt.Errorf("%w: %s", ErrDispositionCommandRejected, reason) }
	blank := func(value string) bool { return strings.TrimSpace(value) == "" }
	switch {
	case blank(command.TenantID.String()) || blank(command.Operator):
		return reject("tenant and operator come from the envelope")
	case blank(command.Unit.String()) || blank(command.Disposed.String()):
		return reject("unit and disposed version are required")
	case blank(command.Basis) || strings.ContainsRune(command.Basis, 0):
		return reject("basis reference is blank or carries NUL")
	}
	switch command.Result {
	case domain.DispositionDesignatesParcel:
		if blank(command.Parcel.String()) {
			return reject("a designation names the designated parcel")
		}
		seen := map[domain.HandlingUnitID]bool{}
		for _, co := range command.CoDisposed {
			if blank(co.Unit.String()) || blank(co.Version.String()) || co.Unit == command.Unit || seen[co.Unit] {
				return reject("a co-disposed unit needs its version, is not the disposed unit and is listed once")
			}
			seen[co.Unit] = true
		}
	case domain.DispositionNoneOfCandidates:
		if !blank(command.Parcel.String()) || len(command.CoDisposed) > 0 {
			return reject("none-of-candidates names no parcel and no co-disposed unit")
		}
	default:
		return reject("result is neither a designation nor none-of-candidates")
	}
	return nil
}

func coDisposedUnitsOf(command DisposeIdentityConflictCommand) []domain.HandlingUnitID {
	units := make([]domain.HandlingUnitID, 0, len(command.CoDisposed))
	for _, co := range command.CoDisposed {
		units = append(units, co.Unit)
	}
	return units
}

// replay 按（租户，所处置的版本）认重放（ADR-0210 决定六）：所处置的版本已被一次处置接续，比对格相同答已有结果，不同答已处置，都
// 回显在册的那一次。经派生来源标识认出——接续所处置版本的那一版处置版，来源标识就是由它派生的。
func (handler *DisposeIdentityConflictHandler) replay(
	ctx context.Context,
	tenant domain.TenantID,
	disposed domain.IntakeResultVersion,
	digest string,
) (DisposeIdentityConflictResult, bool, error) {
	recorded, found, err := handler.deps.Receptions.FindByKey(ctx, ports.ReceptionKey{TenantID: tenant, SourceID: identityDispositionSourceID(disposed)})
	if err != nil {
		return DisposeIdentityConflictResult{}, false, fmt.Errorf("look up a recorded disposition: %w", err)
	}
	if !found {
		return DisposeIdentityConflictResult{}, false, nil
	}
	if !recorded.Disposition.Recorded() {
		return DisposeIdentityConflictResult{}, false, fmt.Errorf("node operations: derived disposition key %q holds a judgment that is not a disposition", recorded.Key.SourceID)
	}
	// 处置口生来就是 NOC-1，没有无版本那一版的摘要可比，第三个参数因此给空。
	outcome := DispositionOutcomeInvalid
	switch domain.CompareStoredDigest(recorded.ContentDigest, digest, "") {
	case domain.SamePayload:
		outcome = DispositionExistingResult
	case domain.DifferentPayload:
		outcome = DispositionAlreadyDisposed
	default:
		return DisposeIdentityConflictResult{}, false, errors.New("node operations: stored disposition digest is not comparable")
	}
	return DisposeIdentityConflictResult{outcome: outcome, disposition: recorded.Disposition, versions: []ports.ReceptionRecord{recorded}}, true, nil
}

// holdParcels 取这次处置要动的各件包裹的锁：指定的那件，加主实物与所列连带实物在册上带过的每一件关联。锁就是收寄与重核按包裹读
// 别的实物关联时取的那一把（ports.ReceptionStore.FindOtherUnitIntakes），按关联排序取，与同一件包裹上的收寄、重核串行。多锁几件只是
// 多等一会儿；被这次处置截断的关联都在这几件里，按包裹读关联的两面因此读不到半截处置。已取过的不再取。
func (handler *DisposeIdentityConflictHandler) holdParcels(
	ctx context.Context,
	command DisposeIdentityConflictCommand,
	held map[domain.ParcelAssociationReference]bool,
) error {
	parcels := []domain.ParcelAssociationReference{}
	if command.Result == domain.DispositionDesignatesParcel {
		parcels = append(parcels, command.Parcel)
	}
	for _, unit := range append([]domain.HandlingUnitID{command.Unit}, coDisposedUnitsOf(command)...) {
		rows, err := handler.deps.Register.FindUnitIntakes(ctx, command.TenantID, unit)
		if err != nil {
			return fmt.Errorf("read the units to dispose: %w", err)
		}
		for _, row := range rows {
			if association, associated := row.Intake.Association(); associated {
				parcels = append(parcels, association)
			}
		}
	}
	slices.SortFunc(parcels, func(left, right domain.ParcelAssociationReference) int {
		return strings.Compare(left.String(), right.String())
	})
	for _, parcel := range slices.Compact(parcels) {
		if held[parcel] {
			continue
		}
		if _, err := handler.deps.Receptions.FindOtherUnitIntakes(ctx, command.TenantID, parcel, command.Unit); err != nil {
			return fmt.Errorf("hold the parcel %q: %w", parcel, err)
		}
		held[parcel] = true
	}
	return nil
}

// associatedNow 按处置截断读此刻关联到指定包裹的别的作业实物（ADR-0210 决定四、八），与连带实物逐件对照：所列的实物恰好是这几件，
// 各带的版本是它此刻某条链的当前有效版本，它的处置候选恰好只有指定的那件。不符时交回 false，回显这几件与它们的当前有效版本。
func (handler *DisposeIdentityConflictHandler) associatedNow(
	ctx context.Context,
	command DisposeIdentityConflictCommand,
) ([]AssociatedUnit, bool, error) {
	others, err := handler.deps.Receptions.FindOtherUnitIntakes(ctx, command.TenantID, command.Parcel, command.Unit)
	if err != nil {
		return nil, false, fmt.Errorf("find units associated with the designated parcel: %w", err)
	}
	var units []domain.HandlingUnitID
	for _, other := range others {
		if unit := other.Intake.Unit(); !slices.Contains(units, unit) {
			units = append(units, unit)
		}
	}
	slices.SortFunc(units, func(left, right domain.HandlingUnitID) int { return strings.Compare(left.String(), right.String()) })
	named := map[domain.HandlingUnitID]domain.IntakeResultVersion{}
	for _, co := range command.CoDisposed {
		named[co.Unit] = co.Version
	}
	matched := len(units) == len(named)
	associated := make([]AssociatedUnit, 0, len(units))
	for _, unit := range units {
		rows, err := handler.deps.Register.FindUnitIntakes(ctx, command.TenantID, unit)
		if err != nil {
			return nil, false, fmt.Errorf("read an associated unit: %w", err)
		}
		heads, err := chainHeadsOf(rows)
		if err != nil {
			return nil, false, err
		}
		associated = append(associated, AssociatedUnit{Unit: unit, CurrentVersions: heads})
		version, listed := named[unit]
		onlyDesignated := slices.Equal(dispositionCandidatesOf(heads), []domain.ParcelAssociationReference{command.Parcel})
		if !listed || !onlyDesignated ||
			!slices.ContainsFunc(heads, func(head ports.ReceptionRecord) bool { return head.JudgmentVersion == version }) {
			matched = false
		}
	}
	return associated, matched, nil
}

// append 逐链追加处置版并交意图（ADR-0210 决定三、四、七）。
func (handler *DisposeIdentityConflictHandler) append(
	ctx context.Context,
	command DisposeIdentityConflictCommand,
	digest string,
	rows []ports.ReceptionRecord,
	heads []ports.ReceptionRecord,
	candidates []domain.ParcelAssociationReference,
	coHeads []AssociatedUnit,
) (DisposeIdentityConflictResult, error) {
	at := handler.deps.Clock.Now()
	spec := domain.IdentityDisposition{
		Disposed:   command.Disposed,
		Unit:       command.Unit,
		Result:     command.Result,
		Parcel:     command.Parcel,
		CoDisposed: command.CoDisposed,
		Basis:      command.Basis,
		Operator:   command.Operator,
		DisposedAt: at,
	}
	designated := command.Result == domain.DispositionDesignatesParcel
	if !designated {
		spec.Excluded = candidates
	}
	disposition, err := domain.FormIdentityDisposition(spec)
	if err != nil {
		return DisposeIdentityConflictResult{}, fmt.Errorf("%w: %w", ErrDispositionCommandRejected, err)
	}

	// 指定一件的那几版照 ADR-0186 向 PS 取服务结果：问一次，几条链同一个答复。读不动不挡处置——照常落版，核对态记读不动、标记留空。
	var markers []string
	check := ports.ServiceResultCheckAbsent
	if designated {
		check = ports.ServiceResultsUnreadable
		if results, err := handler.deps.ServiceResults.FindParcelServiceResults(ctx, command.TenantID, command.Parcel); err == nil {
			check, markers = ports.ServiceResultsChecked, results.Markers()
		}
	}

	ordered := slices.Clone(heads)
	slices.SortFunc(ordered, earliestReceptionFirst)
	var appended []ports.ReceptionRecord
	for _, head := range ordered {
		version, err := handler.dispositionVersion(ctx, head, disposition, digest, designated, markers, check, at)
		if err != nil {
			return DisposeIdentityConflictResult{}, err
		}
		appended = append(appended, version)
	}
	for _, co := range coHeads {
		coOrdered := slices.Clone(co.CurrentVersions)
		slices.SortFunc(coOrdered, earliestReceptionFirst)
		for _, head := range coOrdered {
			version, err := handler.dispositionVersion(ctx, head, disposition, digest, false, nil, ports.ServiceResultCheckAbsent, at)
			if err != nil {
				return DisposeIdentityConflictResult{}, err
			}
			appended = append(appended, version)
		}
	}
	for index, version := range appended {
		saved, err := handler.deps.Receptions.Save(ctx, version)
		if err != nil {
			return DisposeIdentityConflictResult{}, fmt.Errorf("record the disposition version %q: %w", version.Key.SourceID, err)
		}
		if saved == ports.ReceptionSaved {
			continue
		}
		// 第一版就撞上派生键：同一处置的另一次请求在锁外认重放之后先提交了（锁护不住没有关联可锁的那一格），读回它作答。其余情形
		// 是派生键被占，不拿占位的那一行顶替这次处置。
		if index == 0 && saved == ports.ReceptionAlreadyRecorded {
			if replayed, ok, err := handler.replay(ctx, command.TenantID, command.Disposed, digest); err != nil || ok {
				return replayed, err
			}
		}
		return DisposeIdentityConflictResult{}, fmt.Errorf("%w: disposition version %q", ErrUnexpectedReceptionSave, version.Key.SourceID)
	}

	result := DisposeIdentityConflictResult{outcome: IdentityConflictDisposed, disposition: disposition, versions: appended}
	// 主实物此前哪一版`节点收寄已形成`曾关联到指定的那件，就已交出过意图（原判断与重核版各交一封）；多条链只交一封，落在接收最早
	// 的那一版——责任起点是最早那次实际接收。确认都不是与连带实物不交。已交出的一律不撤回。
	if designated && !handedOffBefore(rows, command.Parcel) {
		if _, err := handOffIntake(ctx, handler.deps.Downstream, appended[0]); err != nil {
			return DisposeIdentityConflictResult{}, fmt.Errorf("hand off the disposed intake: %w", err)
		}
		result.handedOff, result.intakeHandedIn = appended[0].Key, true
	}
	return result, nil
}

// dispositionVersion 拼接续 head 的那一版处置版：收寄事实与控制照前版，业务时间是原接收时刻，不另起控制起点；来源文档取这条链
// 起点那一版的（ADR-0210 决定三）——冲突版带的是引起它的那次送站的来源，号码与作业实物都可能是另一件的。
func (handler *DisposeIdentityConflictHandler) dispositionVersion(
	ctx context.Context,
	head ports.ReceptionRecord,
	disposition domain.IdentityDisposition,
	digest string,
	associate bool,
	markers []string,
	check ports.ServiceResultCheck,
	at time.Time,
) (ports.ReceptionRecord, error) {
	origin, err := handler.chainOrigin(ctx, head)
	if err != nil {
		return ports.ReceptionRecord{}, err
	}
	version, err := handler.deps.Versions.NextIntakeResultVersion(ctx)
	if err != nil {
		return ports.ReceptionRecord{}, fmt.Errorf("next judgment version for the disposition: %w", err)
	}
	spec := domain.NodeIntakeSpec{
		TenantID:    head.Key.TenantID,
		Unit:        head.Intake.Unit(),
		Node:        head.Intake.Node(),
		DeliveredBy: head.Intake.DeliveredBy(),
		Evidence:    head.Intake.Evidence(),
		Version:     version,
		ReceivedAt:  head.Intake.ReceivedAt(),
	}
	record := ports.ReceptionRecord{
		Key:             ports.ReceptionKey{TenantID: head.Key.TenantID, SourceID: identityDispositionSourceID(head.JudgmentVersion)},
		ContentDigest:   digest,
		Kind:            ports.RecordPendingIdentification,
		JudgmentVersion: version,
		Predecessor:     head.JudgmentVersion,
		Source:          origin.Source,
		Control:         head.Control,
		RecordedAt:      at,
		Disposition:     disposition,
	}
	if associate {
		spec.Association = disposition.Parcel
		record.Kind = ports.RecordIntakeFormed
		record.ServiceMarkers, record.ServiceResultCheck = markers, check
	}
	if record.Intake, err = domain.FormNodeIntake(spec); err != nil {
		return ports.ReceptionRecord{}, fmt.Errorf("form the disposition version: %w", err)
	}
	return record, nil
}

// chainOrigin 沿前版往回走到这条链的起点那一版。步数照收寄库沿后继走的封顶，绕过编排写出的环在这里报错，不替它挑一个起点。
func (handler *DisposeIdentityConflictHandler) chainOrigin(ctx context.Context, head ports.ReceptionRecord) (ports.ReceptionRecord, error) {
	current := head
	for range maxDispositionChainWalk {
		if strings.TrimSpace(current.Predecessor.String()) == "" {
			return current, nil
		}
		previous, found, err := handler.deps.Register.FindByJudgmentVersion(ctx, head.Key.TenantID, current.Predecessor)
		if err != nil {
			return ports.ReceptionRecord{}, fmt.Errorf("walk back to the chain origin: %w", err)
		}
		if !found {
			return ports.ReceptionRecord{}, fmt.Errorf("node operations: predecessor %q of %q is not in the register", current.Predecessor, current.Key.SourceID)
		}
		current = previous
	}
	return ports.ReceptionRecord{}, fmt.Errorf("node operations: chain of %q does not start within %d versions", head.Key.SourceID, maxDispositionChainWalk)
}

// maxDispositionChainWalk 给往回走的步数封顶；一条链上的版本是重核、冲突与处置逐次追加的，远到不了这个数。
const maxDispositionChainWalk = 1024

// identityDispositionSourceID 派生处置版的来源标识：由前版的判断版本派生，同一版再处置撞键即重放（ADR-0210 决定三）。派生自判断
// 版本而不是来源标识，理由同 identityRecheckSourceID：长度有界。
func identityDispositionSourceID(predecessor domain.IntakeResultVersion) string {
	return "IDENTITY-DISPOSITION/" + predecessor.String()
}

// chainHeadsOf 交回这几行里各条判断链的当前有效版本，链头由领域认（domain.JudgmentChainHeads），与方向依据的身份一格同一个读法。
func chainHeadsOf(rows []ports.ReceptionRecord) ([]ports.ReceptionRecord, error) {
	observed, err := domain.JudgmentChainHeads(observationsOf(rows))
	if err != nil {
		return nil, fmt.Errorf("judgment chain heads: %w", err)
	}
	bySource := make(map[string]ports.ReceptionRecord, len(rows))
	for _, row := range rows {
		bySource[row.Key.SourceID] = row
	}
	heads := make([]ports.ReceptionRecord, 0, len(observed))
	for _, head := range observed {
		heads = append(heads, bySource[head.SourceID])
	}
	return heads, nil
}

// headsInConflict 用方向依据的身份一格判这几版链头合起来是否身份冲突（ADR-0210 决定二「读法见决定五」），不另写一套。
func headsInConflict(heads []ports.ReceptionRecord) (bool, error) {
	basis, err := domain.JudgeIdentityBasis(observationsOf(heads))
	if err != nil {
		return false, fmt.Errorf("judge the identity of the disposed unit: %w", err)
	}
	return slices.Contains(basis.Cell().Reasons(), domain.ReasonIdentityConflict), nil
}

func observationsOf(rows []ports.ReceptionRecord) []domain.IntakeIdentityObservation {
	observations := make([]domain.IntakeIdentityObservation, 0, len(rows))
	for _, row := range rows {
		observation := domain.IntakeIdentityObservation{
			SourceID:         row.Key.SourceID,
			Version:          row.Intake.Version(),
			Predecessor:      row.Predecessor,
			IdentityConflict: row.IdentityConflict,
			ReceivedAt:       row.Intake.ReceivedAt(),
		}
		if association, associated := row.Intake.Association(); associated {
			observation.Association = association
		}
		observations = append(observations, observation)
	}
	return observations
}

// dispositionCandidatesOf 是处置候选（ADR-0210 决定二）：各链当前有效版本上记的候选与关联合起来的那一组包裹，按标识定序。它们都出自
// PS 身份核对的答复，节点不往里添。
func dispositionCandidatesOf(heads []ports.ReceptionRecord) []domain.ParcelAssociationReference {
	var parcels []domain.ParcelAssociationReference
	for _, head := range heads {
		parcels = append(parcels, head.Candidates...)
		if association, associated := head.Intake.Association(); associated {
			parcels = append(parcels, association)
		}
	}
	slices.SortFunc(parcels, func(left, right domain.ParcelAssociationReference) int {
		return strings.Compare(left.String(), right.String())
	})
	return slices.Compact(parcels)
}

// handedOffBefore 说主实物此前是否已就 parcel 交出过收寄结果：哪一版`节点收寄已形成`曾关联到它（ADR-0210 决定七）。
func handedOffBefore(rows []ports.ReceptionRecord, parcel domain.ParcelAssociationReference) bool {
	return slices.ContainsFunc(rows, func(row ports.ReceptionRecord) bool {
		association, associated := row.Intake.Association()
		return row.Kind == ports.RecordIntakeFormed && associated && association == parcel
	})
}

// earliestReceptionFirst 按接收时刻排，同刻按来源标识定序：多条链只交一封意图时，落在接收最早的那一版。
func earliestReceptionFirst(left, right ports.ReceptionRecord) int {
	if compared := left.Intake.ReceivedAt().Compare(right.Intake.ReceivedAt()); compared != 0 {
		return compared
	}
	return strings.Compare(left.Key.SourceID, right.Key.SourceID)
}
