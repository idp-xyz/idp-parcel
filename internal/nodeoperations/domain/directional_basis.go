package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// 方向依据记录（NO CONTEXT「方向依据记录」；票 no-directional-basis/01「偏离标记半边：设计代决」）：移入事实落册时，
// 同一笔记下该作业实物当时的方向依据。它只是事实旁边的依据快照——缺依据不拒收、读不动不挡落册；偏离的判断与追责
// 归 network-routing 的路由偏离判断与 visibility-exception 的异常，本上下文只留事实与当时的依据。

// ErrInvalidDirectionalBasis 说明一份方向依据记录立不起来：一格的取值与原因、引用对不上，或身份不成立却问了另两格。
// 缺依据本身不是这个错误——那是一格的取值。
var ErrInvalidDirectionalBasis = errors.New("node operations: invalid directional basis record")

// DirectionalBasisAspect 是方向依据记录的三格。身份排在最前：另两格要拿身份成立后的正式包裹关联去问。
type DirectionalBasisAspect uint8

const (
	DirectionalBasisAspectInvalid DirectionalBasisAspect = iota
	IdentityBasisAspect
	ServiceResultBasisAspect
	RoutingBasisAspect
)

func (aspect DirectionalBasisAspect) String() string {
	switch aspect {
	case IdentityBasisAspect:
		return "IDENTITY"
	case ServiceResultBasisAspect:
		return "SERVICE_RESULT"
	case RoutingBasisAspect:
		return "ROUTING"
	default:
		return ""
	}
}

// DirectionalBasisAnswer 是一格的取值。`读不动`与`未配置`、`未判`分开：前者是来源此刻答不出，`未配置`是来源在生产
// 装配上显式未接，`未判`是前提不成立、无从问——三者的续办各不相同，压成一个「不知道」就分不出该等谁。
type DirectionalBasisAnswer uint8

const (
	DirectionalBasisAnswerInvalid DirectionalBasisAnswer = iota
	BasisEstablished
	BasisMissing
	BasisNotConfigured
	BasisUnreadable
	BasisNotJudged
)

func (answer DirectionalBasisAnswer) String() string {
	switch answer {
	case BasisEstablished:
		return "ESTABLISHED"
	case BasisMissing:
		return "MISSING"
	case BasisNotConfigured:
		return "NOT_CONFIGURED"
	case BasisUnreadable:
		return "UNREADABLE"
	case BasisNotJudged:
		return "NOT_JUDGED"
	default:
		return ""
	}
}

func (answer DirectionalBasisAnswer) valid() bool {
	return answer >= BasisEstablished && answer <= BasisNotJudged
}

// DirectionalBasisReason 是`缺少`的原因。每个原因只属一格，一格可并列多个。
type DirectionalBasisReason uint8

const (
	DirectionalBasisReasonInvalid DirectionalBasisReason = iota
	ReasonNoIntakeRecord
	ReasonPendingIdentification
	ReasonIdentityConflict
	ReasonCancelled
	ReasonFinal
)

func (reason DirectionalBasisReason) String() string {
	switch reason {
	case ReasonNoIntakeRecord:
		return "NO_INTAKE_RECORD"
	case ReasonPendingIdentification:
		return "PENDING_IDENTIFICATION"
	case ReasonIdentityConflict:
		return "IDENTITY_CONFLICT"
	case ReasonCancelled:
		return "CANCELLED"
	case ReasonFinal:
		return "FINAL"
	default:
		return ""
	}
}

func (reason DirectionalBasisReason) aspect() DirectionalBasisAspect {
	switch reason {
	case ReasonNoIntakeRecord, ReasonPendingIdentification, ReasonIdentityConflict:
		return IdentityBasisAspect
	case ReasonCancelled, ReasonFinal:
		return ServiceResultBasisAspect
	default:
		return DirectionalBasisAspectInvalid
	}
}

// DirectionalBasisCell 是记录的一格。原因只在`缺少`时在场，按词表序排列：同一份判断落成同一个样子，读回与比对
// 不受原因到来的先后摆布。
type DirectionalBasisCell struct {
	aspect  DirectionalBasisAspect
	answer  DirectionalBasisAnswer
	reasons []DirectionalBasisReason
}

func (cell DirectionalBasisCell) Aspect() DirectionalBasisAspect {
	return cell.aspect
}

func (cell DirectionalBasisCell) Answer() DirectionalBasisAnswer {
	return cell.answer
}

func (cell DirectionalBasisCell) Reasons() []DirectionalBasisReason {
	return slices.Clone(cell.reasons)
}

func (cell DirectionalBasisCell) has(reason DirectionalBasisReason) bool {
	return slices.Contains(cell.reasons, reason)
}

// directionalBasisCellOf 立一格：`缺少`至少带一个原因且个个属这一格、不重复；其余取值不带原因。
func directionalBasisCellOf(
	aspect DirectionalBasisAspect,
	answer DirectionalBasisAnswer,
	reasons []DirectionalBasisReason,
) (DirectionalBasisCell, error) {
	if !answer.valid() || (answer == BasisMissing) != (len(reasons) > 0) {
		return DirectionalBasisCell{}, ErrInvalidDirectionalBasis
	}
	if len(reasons) == 0 {
		return DirectionalBasisCell{aspect: aspect, answer: answer}, nil
	}
	ordered := slices.Clone(reasons)
	slices.Sort(ordered)
	if len(slices.Compact(slices.Clone(ordered))) != len(ordered) {
		return DirectionalBasisCell{}, ErrInvalidDirectionalBasis
	}
	for _, reason := range ordered {
		if reason.aspect() != aspect {
			return DirectionalBasisCell{}, ErrInvalidDirectionalBasis
		}
	}
	return DirectionalBasisCell{aspect: aspect, answer: answer, reasons: ordered}, nil
}

// IntakeIdentityObservation 是收寄册上带收寄的一行（收寄已形成与待识别两格）对一件作业实物身份说了什么。
// Association 缺席即这一行待识别；IdentityConflict 照收寄判断当时所记。Version 是这一版的判断版本——这两格的判断
// 版本就是收寄结果版本，不另签；Predecessor 是它接续的前版（ADR-0199 决定二），缺席即这一版不接续任何版本：首判，
// 或前版机制之前追加、不回填的旧行。
type IntakeIdentityObservation struct {
	SourceID         string
	Version          IntakeResultVersion
	Predecessor      IntakeResultVersion
	Association      ParcelAssociationReference
	IdentityConflict bool
	ReceivedAt       time.Time
}

// IdentityBasis 是身份那一格：`成立`、`缺少`或`读不动`。身份问的是本上下文自己的收寄册，没有「未配置」，也没有
// 「无从问」。成立时带所依的那一行收寄：正式包裹关联、来源标识与收寄版本。
type IdentityBasis struct {
	cell          DirectionalBasisCell
	association   ParcelAssociationReference
	intakeSource  string
	intakeVersion IntakeResultVersion
}

func (basis IdentityBasis) Cell() DirectionalBasisCell {
	return basis.cell
}

// Association 只在身份成立时在场。
func (basis IdentityBasis) Association() (ParcelAssociationReference, bool) {
	return basis.association, basis.cell.answer == BasisEstablished
}

func (basis IdentityBasis) IntakeSourceID() string {
	return basis.intakeSource
}

func (basis IdentityBasis) IntakeVersion() IntakeResultVersion {
	return basis.intakeVersion
}

// JudgeIdentityBasis 把收寄册上这件实物的各行折成身份一格。今天正式包裹 → 作业实物的映射只有收寄判断里识别成功
// 建立的版本化关联，所以身份只看这本册；而且只看每条判断链的当前有效版本（ADR-0210 决定五，NO CONTEXT「判断链与
// 当前有效版本」）：被组里另一行指名为前版的行已被接续，不再参与。对全部行取或，冲突版一落这一格就再也解不开——
// 接在它后面的处置版推不翻一行已记的冲突。这两格的后继只会是这两格（冲突版、重核版、处置版都是），所以组里就算得出
// 链头；前版指向组外的行（例如续办接在一条不带收寄的待确认之后）照常是链头。链头再照原折法折：
//
//   - 一行都没有：缺少·无收寄记录。未取得控制的，与经交接进来、在本上下文没有收寄行的，都落在这里。
//   - 有一版链头记了身份冲突，或带关联的链头指向不同的正式包裹：缺少·身份冲突（CONTEXT：身份冲突「暂停方向性
//     作业」）。前版机制之前追加、没有前版的冲突版自成一条链，照旧判冲突，不回填。
//   - 没有一版链头带关联：缺少·待识别（CONTEXT「候选尚未确认时不得据此执行方向性作业」）。它可以与身份冲突并列：
//     待识别实物的多个候选正是冲突的来路；一码两物里先到那件的冲突版指名了原判断时，链头只剩冲突版，也落在这里。
//   - 其余为成立。所依收寄取带关联的链头里业务时间最晚的那一版，同刻按来源标识定序——同一件实物收过两次时，
//     后一次才是此刻的控制来由。被接续的版本不当所依收寄：它已不是此刻的判断。
//
// 组里一版被两行指名为前版、一行指名自己、几行互相接续成环，哪一版是链头都说不准，交回 ErrInvalidDirectionalBasis，
// 不替它挑一个。前两种库面拦着，读回见到即说明有绕过编排的写入；环库面拦不住，与收寄库的 CurrentVersion 走到环时
// 同一待遇：报错，不挑。
func JudgeIdentityBasis(observations []IntakeIdentityObservation) (IdentityBasis, error) {
	if len(observations) == 0 {
		return identityBasisOf(BasisMissing, []DirectionalBasisReason{ReasonNoIntakeRecord},
			ParcelAssociationReference{}, "", IntakeResultVersion{})
	}
	heads, err := judgmentChainHeads(observations)
	if err != nil {
		return IdentityBasis{}, err
	}
	parcels := map[ParcelAssociationReference]bool{}
	conflict := false
	var basedOn *IntakeIdentityObservation
	for _, head := range heads {
		conflict = conflict || head.IdentityConflict
		if !head.Association.valid() {
			continue
		}
		parcels[head.Association] = true
		if basedOn == nil || laterIntake(*head, *basedOn) {
			basedOn = head
		}
	}
	var reasons []DirectionalBasisReason
	if len(parcels) == 0 {
		reasons = append(reasons, ReasonPendingIdentification)
	}
	if conflict || len(parcels) > 1 {
		reasons = append(reasons, ReasonIdentityConflict)
	}
	if len(reasons) > 0 {
		return identityBasisOf(BasisMissing, reasons, ParcelAssociationReference{}, "", IntakeResultVersion{})
	}
	return identityBasisOf(BasisEstablished, nil, basedOn.Association, basedOn.SourceID, basedOn.Version)
}

// JudgmentChainHeads 交回组里各条判断链的当前有效版本，读法与验法同 JudgeIdentityBasis：身份冲突处置（ADR-0210 决定二、三）在这几
// 版上判冲突、算处置候选并逐链追加，与身份一格认的必须是同一组链头——两处各算一遍，处置解开的就可能不是身份一格判冲突的那几版。
func JudgmentChainHeads(observations []IntakeIdentityObservation) ([]IntakeIdentityObservation, error) {
	heads, err := judgmentChainHeads(observations)
	if err != nil {
		return nil, err
	}
	copied := make([]IntakeIdentityObservation, 0, len(heads))
	for _, head := range heads {
		copied = append(copied, *head)
	}
	return copied, nil
}

// judgmentChainHeads 交回组里没有后继的各行，并验链的形状。
func judgmentChainHeads(observations []IntakeIdentityObservation) ([]*IntakeIdentityObservation, error) {
	successorOf := map[IntakeResultVersion]int{}
	for index, observation := range observations {
		if strings.TrimSpace(observation.SourceID) == "" || !observation.Version.valid() {
			return nil, ErrInvalidDirectionalBasis
		}
		if !observation.Predecessor.valid() {
			continue
		}
		if _, taken := successorOf[observation.Predecessor]; taken || observation.Predecessor == observation.Version {
			return nil, ErrInvalidDirectionalBasis
		}
		successorOf[observation.Predecessor] = index
	}
	var heads []*IntakeIdentityObservation
	for index := range observations {
		// 每一行沿后继走，都得在组里走到头；走过的步数够绕全组一圈还没到头，就是在环里。
		version := observations[index].Version
		for steps := 0; ; steps++ {
			next, succeeded := successorOf[version]
			if !succeeded {
				break
			}
			if steps == len(observations) {
				return nil, ErrInvalidDirectionalBasis
			}
			version = observations[next].Version
		}
		if _, succeeded := successorOf[observations[index].Version]; !succeeded {
			heads = append(heads, &observations[index])
		}
	}
	return heads, nil
}

func laterIntake(candidate, current IntakeIdentityObservation) bool {
	if !candidate.ReceivedAt.Equal(current.ReceivedAt) {
		return candidate.ReceivedAt.After(current.ReceivedAt)
	}
	return candidate.SourceID < current.SourceID
}

// UnreadableIdentityBasis 是收寄册此刻读不动时的身份一格。
func UnreadableIdentityBasis() IdentityBasis {
	return IdentityBasis{cell: DirectionalBasisCell{aspect: IdentityBasisAspect, answer: BasisUnreadable}}
}

// identityBasisOf 验身份一格：成立时三样引用齐，其余取值一样都不带。
func identityBasisOf(
	answer DirectionalBasisAnswer,
	reasons []DirectionalBasisReason,
	association ParcelAssociationReference,
	intakeSource string,
	intakeVersion IntakeResultVersion,
) (IdentityBasis, error) {
	if answer != BasisEstablished && answer != BasisMissing && answer != BasisUnreadable {
		return IdentityBasis{}, ErrInvalidDirectionalBasis
	}
	cell, err := directionalBasisCellOf(IdentityBasisAspect, answer, reasons)
	if err != nil {
		return IdentityBasis{}, err
	}
	referenced := association.valid() && strings.TrimSpace(intakeSource) != "" && intakeVersion.valid()
	unreferenced := !association.valid() && intakeSource == "" && !intakeVersion.valid()
	if (answer == BasisEstablished && !referenced) || (answer != BasisEstablished && !unreferenced) {
		return IdentityBasis{}, ErrInvalidDirectionalBasis
	}
	return IdentityBasis{cell: cell, association: association, intakeSource: intakeSource, intakeVersion: intakeVersion}, nil
}

// ServiceResultBasis 是服务结果那一格：`成立`、`缺少`、`读不动`或`未判`。缺少时带结果引用——已取消带取消决定，
// 已终局带终局种类与版本，两者可并列。取消或终局成立与否是 parcel-shipment 的判断，本上下文只消费它的答复。
type ServiceResultBasis struct {
	cell         DirectionalBasisCell
	cancellation string
	finalKind    string
	finalVersion string
}

func (basis ServiceResultBasis) Cell() DirectionalBasisCell {
	return basis.cell
}

func (basis ServiceResultBasis) Cancellation() string {
	return basis.cancellation
}

func (basis ServiceResultBasis) Final() (kind, version string) {
	return basis.finalKind, basis.finalVersion
}

// JudgeServiceResultBasis 把 parcel-shipment 此刻的答复折成一格：无取消、无终局为成立；有取消决定记缺少·已取消，
// 当前有效终局记缺少·已终局。终局只答出种类或只答出版本的答复立不起来，交回错误。
func JudgeServiceResultBasis(cancellation, finalKind, finalVersion string) (ServiceResultBasis, error) {
	var reasons []DirectionalBasisReason
	if cancellation != "" {
		reasons = append(reasons, ReasonCancelled)
	}
	if finalKind != "" || finalVersion != "" {
		reasons = append(reasons, ReasonFinal)
	}
	answer := BasisEstablished
	if len(reasons) > 0 {
		answer = BasisMissing
	}
	return serviceResultBasisOf(answer, reasons, cancellation, finalKind, finalVersion)
}

// UnreadableServiceResultBasis 是 parcel-shipment 此刻读不动时的服务结果一格。
func UnreadableServiceResultBasis() ServiceResultBasis {
	return ServiceResultBasis{cell: DirectionalBasisCell{aspect: ServiceResultBasisAspect, answer: BasisUnreadable}}
}

// serviceResultBasisOf 验服务结果一格：引用与原因同在同缺。
func serviceResultBasisOf(
	answer DirectionalBasisAnswer,
	reasons []DirectionalBasisReason,
	cancellation, finalKind, finalVersion string,
) (ServiceResultBasis, error) {
	if answer == BasisNotConfigured {
		return ServiceResultBasis{}, ErrInvalidDirectionalBasis
	}
	cell, err := directionalBasisCellOf(ServiceResultBasisAspect, answer, reasons)
	if err != nil {
		return ServiceResultBasis{}, err
	}
	cancelled := strings.TrimSpace(cancellation) != ""
	final := strings.TrimSpace(finalKind) != "" && strings.TrimSpace(finalVersion) != ""
	if cell.has(ReasonCancelled) != cancelled || (!cancelled && cancellation != "") ||
		cell.has(ReasonFinal) != final || (!final && (finalKind != "" || finalVersion != "")) {
		return ServiceResultBasis{}, ErrInvalidDirectionalBasis
	}
	return ServiceResultBasis{cell: cell, cancellation: cancellation, finalKind: finalKind, finalVersion: finalVersion}, nil
}

// RoutingBasis 是路由依据那一格：当前有效路由指令与成员级集运兼容判断（UC-NO-003 启动条件），两样都归
// network-routing。今天只有`未配置`、`读不动`、`未判`三种取值：`成立`与`缺少`要等 NR 的路由指令与兼容判断定下
// 原因与引用（票 no-directional-basis/02）随它加，在那之前不在本上下文里自比下一节点。
type RoutingBasis struct {
	cell DirectionalBasisCell
}

func (basis RoutingBasis) Cell() DirectionalBasisCell {
	return basis.cell
}

// NotConfiguredRoutingBasis 是路由依据端口在生产装配上显式未配置时的答复。这一格不跳过：跳过就是替 NR 默认「兼容」。
func NotConfiguredRoutingBasis() RoutingBasis {
	return RoutingBasis{cell: DirectionalBasisCell{aspect: RoutingBasisAspect, answer: BasisNotConfigured}}
}

// UnreadableRoutingBasis 是路由依据此刻读不动时的一格。
func UnreadableRoutingBasis() RoutingBasis {
	return RoutingBasis{cell: DirectionalBasisCell{aspect: RoutingBasisAspect, answer: BasisUnreadable}}
}

func routingBasisOf(answer DirectionalBasisAnswer, reasons []DirectionalBasisReason) (RoutingBasis, error) {
	if answer != BasisNotConfigured && answer != BasisUnreadable && answer != BasisNotJudged {
		return RoutingBasis{}, ErrInvalidDirectionalBasis
	}
	cell, err := directionalBasisCellOf(RoutingBasisAspect, answer, reasons)
	if err != nil {
		return RoutingBasis{}, err
	}
	return RoutingBasis{cell: cell}, nil
}

// DirectionalBasisRecord 是方向依据记录。零值是「没有记录」：这一格出现之前落的移入事实读回来就是它，读作
// `未记录`，绝不当作依据齐全。
type DirectionalBasisRecord struct {
	identity      IdentityBasis
	serviceResult ServiceResultBasis
	routing       RoutingBasis
	judgedAt      time.Time
}

// RecordDirectionalBasis 合身份已成立的三格成一份记录。judgedAt 是判断时刻：记录说的是那一刻所知，不是装袋那一刻
// ——导入的历史装袋可能早于 parcel-shipment 的取消，先后归 PS 按业务时间裁，本上下文不代判。
func RecordDirectionalBasis(
	identity IdentityBasis,
	serviceResult ServiceResultBasis,
	routing RoutingBasis,
	judgedAt time.Time,
) (DirectionalBasisRecord, error) {
	return directionalBasisRecordOf(identity, serviceResult, routing, judgedAt)
}

// RecordDirectionalBasisWithoutIdentity 在身份不成立（缺少或读不动）时成记录：另两格要拿正式包裹关联去问，此时
// 无从问，一律记`未判`。这一条由领域定，调用方不必也不能自己填。
func RecordDirectionalBasisWithoutIdentity(identity IdentityBasis, judgedAt time.Time) (DirectionalBasisRecord, error) {
	return directionalBasisRecordOf(identity,
		ServiceResultBasis{cell: DirectionalBasisCell{aspect: ServiceResultBasisAspect, answer: BasisNotJudged}},
		RoutingBasis{cell: DirectionalBasisCell{aspect: RoutingBasisAspect, answer: BasisNotJudged}},
		judgedAt)
}

// directionalBasisRecordOf 验三格之间那一条：身份成立时另两格都问过，不成立时都没问。零值格不算问过——它没经过
// 任何一扇构造门。
func directionalBasisRecordOf(
	identity IdentityBasis,
	serviceResult ServiceResultBasis,
	routing RoutingBasis,
	judgedAt time.Time,
) (DirectionalBasisRecord, error) {
	if judgedAt.IsZero() ||
		identity.cell.aspect != IdentityBasisAspect || !identity.cell.answer.valid() ||
		serviceResult.cell.aspect != ServiceResultBasisAspect || !serviceResult.cell.answer.valid() ||
		routing.cell.aspect != RoutingBasisAspect || !routing.cell.answer.valid() {
		return DirectionalBasisRecord{}, ErrInvalidDirectionalBasis
	}
	established := identity.cell.answer == BasisEstablished
	asked := serviceResult.cell.answer != BasisNotJudged && routing.cell.answer != BasisNotJudged
	unasked := serviceResult.cell.answer == BasisNotJudged && routing.cell.answer == BasisNotJudged
	if (established && !asked) || (!established && !unasked) {
		return DirectionalBasisRecord{}, ErrInvalidDirectionalBasis
	}
	return DirectionalBasisRecord{
		identity:      identity,
		serviceResult: serviceResult,
		routing:       routing,
		judgedAt:      judgedAt.UTC(),
	}, nil
}

// Recorded 为假即没有记录（零值）。
func (record DirectionalBasisRecord) Recorded() bool {
	return !record.judgedAt.IsZero()
}

func (record DirectionalBasisRecord) Identity() IdentityBasis {
	return record.identity
}

func (record DirectionalBasisRecord) ServiceResult() ServiceResultBasis {
	return record.serviceResult
}

func (record DirectionalBasisRecord) Routing() RoutingBasis {
	return record.routing
}

// Cells 按身份、服务结果、路由依据的次序交回三格；没有记录时交回空。
func (record DirectionalBasisRecord) Cells() []DirectionalBasisCell {
	if !record.Recorded() {
		return nil
	}
	return []DirectionalBasisCell{record.identity.cell, record.serviceResult.cell, record.routing.cell}
}

func (record DirectionalBasisRecord) JudgedAt() time.Time {
	return record.judgedAt
}

// RehydrateDirectionalBasisCellSpec 是一格在库里的样子：取值与原因都是词表里的词（各枚举的 String()）。
type RehydrateDirectionalBasisCellSpec struct {
	Answer  string
	Reasons []string
}

// RehydrateDirectionalBasisSpec 是一份方向依据记录在库里的样子。引用只随对应的取值或原因在场。
type RehydrateDirectionalBasisSpec struct {
	Identity       RehydrateDirectionalBasisCellSpec
	Association    string
	IntakeSourceID string
	IntakeVersion  string
	ServiceResult  RehydrateDirectionalBasisCellSpec
	Cancellation   string
	FinalKind      string
	FinalVersion   string
	Routing        RehydrateDirectionalBasisCellSpec
	JudgedAt       time.Time
}

// RehydrateDirectionalBasis 从行数据重建记录，过的是与构造同一套规则：词在词表里、原因属其格、引用与原因同在同缺、
// 身份不成立时另两格未判。坏行在这里响亮，不变成一份看起来合法的依据。
func RehydrateDirectionalBasis(spec RehydrateDirectionalBasisSpec) (DirectionalBasisRecord, error) {
	identityAnswer, identityReasons, err := cellWordsOf(spec.Identity)
	if err != nil {
		return DirectionalBasisRecord{}, err
	}
	var association ParcelAssociationReference
	if spec.Association != "" {
		if association, err = NewParcelAssociationReference(spec.Association); err != nil {
			return DirectionalBasisRecord{}, ErrInvalidDirectionalBasis
		}
	}
	var version IntakeResultVersion
	if spec.IntakeVersion != "" {
		if version, err = NewIntakeResultVersion(spec.IntakeVersion); err != nil {
			return DirectionalBasisRecord{}, ErrInvalidDirectionalBasis
		}
	}
	identity, err := identityBasisOf(identityAnswer, identityReasons, association, spec.IntakeSourceID, version)
	if err != nil {
		return DirectionalBasisRecord{}, err
	}

	serviceAnswer, serviceReasons, err := cellWordsOf(spec.ServiceResult)
	if err != nil {
		return DirectionalBasisRecord{}, err
	}
	serviceResult, err := serviceResultBasisOf(serviceAnswer, serviceReasons, spec.Cancellation, spec.FinalKind, spec.FinalVersion)
	if err != nil {
		return DirectionalBasisRecord{}, err
	}

	routingAnswer, routingReasons, err := cellWordsOf(spec.Routing)
	if err != nil {
		return DirectionalBasisRecord{}, err
	}
	routing, err := routingBasisOf(routingAnswer, routingReasons)
	if err != nil {
		return DirectionalBasisRecord{}, err
	}
	return directionalBasisRecordOf(identity, serviceResult, routing, spec.JudgedAt)
}

// cellWordsOf 把一格的词译回枚举。词表以各枚举的 String() 为唯一来源，不在这里另抄一份。
func cellWordsOf(spec RehydrateDirectionalBasisCellSpec) (DirectionalBasisAnswer, []DirectionalBasisReason, error) {
	answer := DirectionalBasisAnswerInvalid
	for candidate := BasisEstablished; candidate <= BasisNotJudged; candidate++ {
		if candidate.String() == spec.Answer {
			answer = candidate
		}
	}
	if answer == DirectionalBasisAnswerInvalid {
		return 0, nil, ErrInvalidDirectionalBasis
	}
	reasons := make([]DirectionalBasisReason, 0, len(spec.Reasons))
	for _, word := range spec.Reasons {
		reason := DirectionalBasisReasonInvalid
		for candidate := ReasonNoIntakeRecord; candidate <= ReasonFinal; candidate++ {
			if candidate.String() == word {
				reason = candidate
			}
		}
		if reason == DirectionalBasisReasonInvalid {
			return 0, nil, ErrInvalidDirectionalBasis
		}
		reasons = append(reasons, reason)
	}
	return answer, reasons, nil
}
