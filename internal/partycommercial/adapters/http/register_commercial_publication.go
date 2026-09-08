package commercialhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 商业权威依据发布的在线登记口（ADR-0085 Decision 一；票 admin-write-faces/02 商业片），
// 与受控 CLI 的 publish 子命令同源，两口消费同一个 UC-PC-001 用例。
//
// 一个端点而不是按对象类别铺一排：服务产品、接单规则包、客户合同、供应商协议与各类
// 策略是同一个发布用例的输入，类别在版本规格里（`CommercialVersionSpec.Kind`），不是
// 另一种命令。这与网络七族、关务四类相反，那两处是七个/四个各自成形的命令类型；这里
// 拆成多个端点只会让同一个 `Handle` 挂在几处，而调用侧仍要把类别写进载荷。
//
// 因此路径也不取 `-registrations` 后缀：本上下文的动词是发布，答案代数说的也是发布
// （`已发布已生效`/`已计划生效`），叫成登记会让它与身份、映射那两族的登记答案混为一谈。
//
// 一次调用发布一个对象版本。批不是聚合（AT-PC-011）：受控 CLI 逐项各起事务、前项已落
// 库的不因后项失败被撤出，在线口把「一项」作为一次请求，同一条纪律在这里表现为一个
// 请求一笔事务。

// CommercialPublicationIntake 把一次已认证的接入请求翻译成发布命令。
//
// 它是接口而不是本包内的解析代码（ADR-0085 Decision 二，机制同 ADR-0055）：折叠、批准
// 角色核对与声明构造门在用例侧已实现，但「渠道原始载荷 → 发布批文项」的翻译与操作者
// 认证属渠道接入契约，`PAR-INT-01` 待提供。受控登记口的译装尤其不能照抄——那一份从
// 批文里读 tenantId，运维在库网内手跑时那是治理动作，搬到在线口上就是采信自报租户，
// 穿透 ADR-0003 的隔离边界。未决期间本包不带任何实现，包括「开发用」的采信头部版本。
type CommercialPublicationIntake interface {
	IntakeCommercialPublication(
		ctx context.Context,
		request *http.Request,
	) (application.PublishCommercialAuthorityCommand, error)
}

// CommercialAuthorityPublisher 是本端点转交的发布编排。事务边界在编排侧给出（版本与
// 声明同一事务落库，不留半份发布——UC-PC-001 步骤 6），适配器只转交与映射，不判断
// 任何业务结果。
type CommercialAuthorityPublisher interface {
	Handle(
		ctx context.Context,
		command application.PublishCommercialAuthorityCommand,
	) (application.PublishCommercialAuthorityResult, error)
}

// 编译期锁缝：本端点不新造发布语义，只消费 UC-PC-001 那一个用例，签名漂移在编译期
// 暴露。
var _ CommercialAuthorityPublisher = (*application.PublishCommercialAuthorityHandler)(nil)

// NewPublishCommercialAuthorityEndpoint 交回商业权威依据发布的 HTTP 入口（ADR-0085
// Decision 一：写面进端点表带未配置格）。
func NewPublishCommercialAuthorityEndpoint(
	intake CommercialPublicationIntake,
	publisher CommercialAuthorityPublisher,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeCommercialPublication,
		publisher.Handle,
		writePublicationAnswer,
	)
}

// publicationAnswer 是发布端点的封闭响应形状。它不复用身份族那份 `registrationAnswer`：
// 发布的答案多出声明落点这一栏，而 `pendingCause` 与那边的 `cause` 只是都叫原因，出现
// 的格不同——那边随`未受理`（输入被登记册的引用检查拒绝），这边随`发布未决`（角色未
// 确认或被引对象未发布，输入本身没问题）。共用一个字段名会让两种续办动作看起来是
// 一回事。
//
// `declarations` 在没带声明时缺席。它必须在场的理由是`内容冲突`那一格：声明与版本同笔
// 落库，某个通道撞上同键异内容时版本仍可能是`已发布已生效`——把声明落点省掉，一次
// 半数声明没进去的发布在调用侧看起来与全都落定的发布一模一样，而受控 CLI 恰恰按这一格
// 抬退出码要商业责任方去看（publish 的 attention 判定）。
//
// `cause` 与两个摘要串只随`未受理`在场（ADR-0126 Decision 二）：这一格与身份族的`未受理`同名同物
// ——输入自己不自洽，改载荷才会好——所以字段名照那边叫 `cause`，与 `pendingCause` 分开。
// `computedDigest` 是恢复动作本身：调用方抄它就对上了；正文折不成文档时没有算出的串，只有 `cause`。
type publicationAnswer struct {
	Outcome        string              `json:"outcome"`
	PendingCause   string              `json:"pendingCause,omitempty"`
	Cause          string              `json:"cause,omitempty"`
	DeclaredDigest string              `json:"declaredDigest,omitempty"`
	ComputedDigest string              `json:"computedDigest,omitempty"`
	Declarations   []declarationAnswer `json:"declarations,omitempty"`
}

// declarationAnswer 是一个声明通道的落点。通道名与落点名都取应用枚举原名，不改名也
// 不合并——判据同 `outcome`。
type declarationAnswer struct {
	Channel string `json:"channel"`
	Outcome string `json:"outcome"`
}

// writePublicationAnswer 把发布用例的答案逐名转写。
//
// `已发布已生效`与`已计划生效`取 201，其余取 200：前两格是本次落库（后者是到界前的
// 如实入册，不得用于生产解析，那一格由消费侧结构性保证，传输层不替它把关）；重放与
// 内容冲突是登记册的治理答案，走 200 的判据同身份族。
//
// `发布未决`也走 200，而关务片把它那个 `UNDECIDED` 折成 5xx「没形成答案」——两者同名
// 不同物。关务那一格是用例把依赖故障折成的值，说的是「登记与否未知」，续办是重跑同一
// 份；这里的未决是 AT-PC-005/010 指名的业务答案：批准角色未确认或被引对象未发布，一个
// 字节没写，续办是去确认角色或先发布被引对象，重跑同一份不会变。折成 5xx 会让调用侧
// 把一件等人办的事留队重发。
//
// 输入立不起来（草稿构造门拒）不在这几格里：发布用例把它折成 error 而不是一格答案，
// 于是在线口按 ADR-0022 答「没形成答案」。这一格与它的续办动作对不上（重试不会好），
// 但传输层分辨不出——它只看见一个 error。要分开得在用例侧给发布加`未受理`一格，那是
// 用例侧改动，不在本片范围内（已记在票 admin-write-faces/02 的商业片 Comment）。
func writePublicationAnswer(
	response http.ResponseWriter,
	result application.PublishCommercialAuthorityResult,
) {
	answer, err := publicationAnswerOf(result)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	status := http.StatusOK
	switch result.Outcome() {
	case application.CommercialVersionPublishedEffective,
		application.CommercialVersionPlannedEffective:
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}

// errUnnamedPublicationOutcome 是 publicationAnswerOf 对没有名字的结果或落点的答复：编程错误，不是业务答案。
var errUnnamedPublicationOutcome = errors.New("party commercial http: publication outcome or declaration landing has no name")

// publicationAnswerOf 把发布用例的答案逐名转写成响应体。单立出来是因为载体发布口（ADR-0126 Decision 三）要把
// 同一个用例的答案嵌进自己的响应——同一个用例的答案在两口不换形，转写只能有一处。
func publicationAnswerOf(result application.PublishCommercialAuthorityResult) (publicationAnswer, error) {
	name := result.Outcome().String()
	if name == "" {
		return publicationAnswer{}, errUnnamedPublicationOutcome
	}

	answer := publicationAnswer{Outcome: name}
	if cause := result.PendingCause(); cause != nil {
		answer.PendingCause = cause.Error()
	}
	if cause := result.RefusalCause(); cause != nil {
		// `未受理`走 200 的判据同重放与冲突：答案形成了（ADR-0022），内容是「这一份输入对不上自己」，
		// 续办是改载荷——4xx 会让调用侧把它与「请求畸形」混在一起，而后者连解码都没过。
		answer.Cause = cause.Error()
		declared, computed, reconciled := result.DigestReconciliation()
		answer.DeclaredDigest = declared
		if reconciled {
			answer.ComputedDigest = computed
		}
	}
	for _, report := range result.Declarations() {
		channel := report.Channel.String()
		landing := report.Outcome.String()
		if channel == "" || landing == "" {
			return publicationAnswer{}, errUnnamedPublicationOutcome
		}
		answer.Declarations = append(answer.Declarations, declarationAnswer{
			Channel: channel,
			Outcome: landing,
		})
	}
	return answer, nil
}
