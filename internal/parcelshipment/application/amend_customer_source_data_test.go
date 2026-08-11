package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-002 `AT-PS-014`「接受后补充缺失的客户申报原始资料 → 形成新客户原始资料版本，
// 接受基线不变」与步骤 7-8「形成不可覆盖版本」「派生当前资料版本采用判断」。
func TestAnAuthorizedAmendmentFormsAVersionAndDerivesItsAdoption(t *testing.T) {
	fixture := newAmendmentFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentRecorded {
		t.Fatalf("outcome = %q, want RECORDED", result.Outcome())
	}
	version, present := result.Version()
	if !present {
		t.Fatal("an authorized amendment formed no customer source data version")
	}
	if version.Authority().String() != "PC-AMEND-GRANT-1" {
		t.Fatalf("authority = %q; the adopted authorization must be the one party-commercial issued", version.Authority())
	}
	// 请求方与实际决定方分立：UC-PS-002 要求「登录操作人不能替代实际决定方」。
	if version.Requester().String() != "CUSTOMER-CONTACT-1" || version.Decider().String() != "OPERATOR-1" {
		t.Fatalf("requester = %q decider = %q; the two must be recorded apart", version.Requester(), version.Decider())
	}
	adoption, derived := result.Adoption()
	if !derived || adoption.Outcome() != domain.SourceDataAdopted {
		t.Fatalf("adoption = %q derived = %v; a recorded version must leave a current adoption for downstream", adoption.Outcome(), derived)
	}
	if fixture.requests.saved == nil {
		t.Fatal("a version was formed but never saved")
	}
}

// Covers: UC-PS-002 步骤 9「将版本引用交给适用下游」与下游交接边界 — `UC-CC-002`、`UC-CC-003`、
// `UC-CC-007`、路由、节点与结算各自重新判断。
//
// 交出去的是引用而不是内容：跨上下文只传自己拥有的事实与引用，接收方形成自己的结果。一份版本
// 只发一份意图，下游按范围各自认领——逐下游各设一个端口会把「谁该重新判断」搬进本上下文，而那
// 是下游自己的事。
//
// 意图带上采用判断，是因为下游要消费的是「此刻该用哪一份」：范围上出现分叉时，它与刚形成的这
// 一份并不是同一个，只发版本号会让下游把一份没并的支线当成当前资料。
func TestARecordedAmendmentHandsTheVersionReferenceToDownstream(t *testing.T) {
	fixture := newAmendmentFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	version, formed := result.Version()
	if !formed {
		t.Fatal("the amendment formed no version")
	}

	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("handed off %d intents, want exactly one for the one version formed", len(fixture.downstream.intents))
	}
	intent := fixture.downstream.intents[0]
	if intent.Version != version.VersionID() {
		t.Fatalf("handed off %q, want the version just formed (%q)", intent.Version, version.VersionID())
	}
	if intent.Scope != consigneeDataScope(t) {
		t.Fatal("the intent named a scope other than the one amended; downstream cannot tell what to re-judge")
	}
	if intent.Adoption.Outcome() != domain.SourceDataAdopted {
		t.Fatalf("adoption = %q; downstream consumes the current adoption, not merely the version just formed", intent.Adoption.Outcome())
	}
}

// Covers: UC-PS-002「具体字段、字段组、阶段和允许动作由 `PAR-COM-13` 登记……未登记时只能形成
// 未决或业务拒绝，不能以系统便利推断允许」，以及结果语义`待补充/待复核`。
//
// 停在`待复核`而不是业务拒绝：未登记是「还没人说这处资料能不能改」，不是「客户违规」。判成
// 拒绝会让客户以为自己请求有错，而错的是我们还没登记规则，等矩阵登记后要回头翻案。
//
// 不形成版本：结果语义表里只有`已记录并采用`那一行写着「新版本已形成」，`待补充/待复核`那一行
// 要保留的是「缺失/冲突范围、**当前版本**、待补足原因和续办引用」——当前版本指既有的那一份，
// 不是新造一份。请求本身的留痕由步骤 1 的来源保全承担，与版本是两回事。
func TestAnUndeclaredFieldStageRuleStopsAtAwaitingReviewRatherThanAllowing(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.allowance = ports.SourceDataAmendmentNotDeclared

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentAwaitingReview {
		t.Fatalf("outcome = %q, want AWAITING_REVIEW; an unregistered matrix must never be read as permission", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("an undeclared rule formed a version anyway; downstream would then adopt data no rule cleared")
	}
	if fixture.requests.saved != nil {
		t.Fatal("an undeclared rule wrote a version onto the accepted request")
	}
	// 来源已保全：请求到达过这件事必须留下，否则事后连客户提没提过都无从追溯。
	if _, preserved := fixture.sources.stored(t, amendmentSourceIdentity(t)); !preserved {
		t.Fatal("an undeclared rule dropped the source fact too; the request still arrived")
	}
}

// Covers: UC-PS-002 步骤 6「不允许的阶段或字段形成业务拒绝」与`业务拒绝`结果语义。
//
// 与上一条的分界：已登记规则说「这处资料在这个阶段不能这样改」是确定答案，复核也翻不了案；
// 未登记等的是有人去登记矩阵。两者合成一格，客户就分不清该改请求还是该等我们登记规则。
func TestADisallowedFieldStageRuleFormsABusinessRejectionRatherThanAwaitingReview(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.allowance = ports.SourceDataAmendmentDisallowed

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentDisallowed {
		t.Fatalf("outcome = %q, want DISALLOWED; a registered rule forbidding the change is a business answer, not a pending review", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("a disallowed amendment formed a version anyway")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; a disallowed attempt consumed a scarce identity", fixture.identities.issued)
	}
	if fixture.requests.saved != nil {
		t.Fatal("a disallowed amendment wrote a version onto the accepted request")
	}
	if len(fixture.downstream.intents) != 0 {
		t.Fatalf("handed off %d intents; a round that formed no version has nothing for downstream to re-judge", len(fixture.downstream.intents))
	}
}

// Covers: UC-PS-002 `AT-PS-023`「请求新增或删除已接受委托成员 → 拒绝普通资料更正，转取消、
// 重组或关联新委托路径」与步骤 5「增删成员……拒绝并转关联路径」。
//
// 交回业务拒绝而不是上抛技术错误：拿一份「更正」往已接受委托里塞成员，是一个确定的业务答案，
// 客户据以改走关联新委托；写成技术错误，接入层只能回一个看不出下一步该做什么的失败，而客户
// 请求本身并没有任何格式问题。
func TestAnAmendmentNamingAParcelOutsideTheAcceptanceBaselineIsRejectedRatherThanErroring(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	command.Scope = outsideBaselineScope(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentDisallowed {
		t.Fatalf("outcome = %q, want DISALLOWED; reaching outside the baseline is a business answer, not a failure", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("a version reached a parcel the acceptance baseline never covered")
	}
	if fixture.requests.saved != nil {
		t.Fatal("a rejected amendment wrote onto the accepted request")
	}
}

// Covers: UC-PS-002 步骤 5（成员）排在步骤 6（字段与阶段规则）之前，以及 CONTEXT「接受基线
// 自己就是成员集合的权威」— 基线外成员的拒绝不需要任何已登记目录，因此它不问矩阵，矩阵读不
// 回也改不了这个答案。
//
// 守卫一旦压到形成版本之后，它就落在了矩阵查询的下游：一次矩阵抖动会把一个确定的业务拒绝
// 变成未决，客户被告知「等依赖恢复」，而真相是这个请求无论矩阵怎么登记都不成立。
//
// 同时钉住不签发版本标识。理由与撤回那一刀的短路一致：一份注定不形成版本的请求不该消耗一个
// 本上下文签发的稀缺身份。
func TestAnAmendmentOutsideTheBaselineIsRejectedWithoutConsultingTheRuleMatrix(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.err = errors.New("rule declaration unreachable")
	command := fixture.command(t)
	command.Scope = outsideBaselineScope(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentDisallowed {
		t.Fatalf(
			"outcome = %q, want DISALLOWED; the baseline answers on its own, so an unreadable matrix cannot turn this into a pending round",
			result.Outcome(),
		)
	}
	if fixture.rules.calls != 0 {
		t.Fatalf("consulted the rule matrix %d times; membership is settled by the baseline before any registered catalogue", fixture.rules.calls)
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; a rejected amendment consumed a scarce identity", fixture.identities.issued)
	}
}

// Covers: UC-PS-002 目标与边界「本用例从已接受委托或包裹的客户……提出请求开始」与 CONTEXT
// 「决定前的纠错形成新的提交版本并重新判断」。
//
// 尚未接受的委托根本没有接受基线，「越过基线」这个问题因此谈不上。答成 `AT-PS-023` 的业务
// 拒绝会打发客户去走关联新委托，而他实际要做的是等这份委托决定——两条路差得很远。
func TestAnAmendmentToARequestThatIsNotYetAcceptedIsNotReportedAsOutsideTheBaseline(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.requests.notYetAccepted = true
	command := fixture.command(t)
	command.Scope = outsideBaselineScope(t)

	_, err := fixture.handler.Handle(context.Background(), command)

	if !errors.Is(err, domain.ErrShipmentRequestNotAccepted) {
		t.Fatalf("error = %v, want ErrShipmentRequestNotAccepted; a request that fixed no baseline cannot have been reached past one", err)
	}
}

// Covers: UC-PS-002 步骤 4「授权不足形成业务拒绝」与`业务拒绝`结果语义 — 未获授权是确定的
// 业务答案，不是未决，续办也补不出授权来；它与「授权服务答不出」分属两回事。
func TestAnUnauthorizedAmendmentFormsNoVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.authorizer.granted = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentNotAuthorized {
		t.Fatalf("outcome = %q, want NOT_AUTHORIZED", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("an unauthorized request formed a version anyway")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; an unauthorized attempt consumed a scarce identity", fixture.identities.issued)
	}
	if fixture.requests.saved != nil {
		t.Fatal("an unauthorized attempt saved the request")
	}
}

// Covers: UC-PS-002「同一请求身份、相同规范化内容摘要、相同目标范围和基础版本的重试返回已有
// 结果；不能创建第二个资料版本」与 `AT-PS-016`。
//
// 重放不再走一遍授权与规则：授权在两次之间可能已经失效，同一份已经形成的版本会因此读出两种
// 回执。网络重试是常态，这条路一旦缺席，一次重发就在已接受委托上多挂一份版本——而下游按范围
// 取当前采用判断，多出来的那一份会直接改掉它消费的内容。
func TestARepeatedAmendmentReturnsTheOriginalVersionWithoutFormingASecond(t *testing.T) {
	fixture := newAmendmentFixture(t)
	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	original, formed := first.Version()
	if !formed {
		t.Fatal("the first amendment formed no version")
	}

	repeated, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle repeat: %v", err)
	}

	if repeated.Outcome() != application.AmendmentAlreadyHandled {
		t.Fatalf("outcome = %q, want ALREADY_HANDLED", repeated.Outcome())
	}
	version, present := repeated.Version()
	if !present || version.VersionID() != original.VersionID() {
		t.Fatalf(
			"version = %q present = %v; a retry must read back the version it already formed (%q)",
			version.VersionID(), present, original.VersionID(),
		)
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d version IDs; the retry formed a second version", fixture.identities.issued)
	}
	if fixture.authorizer.calls != 1 {
		t.Fatalf("authorized %d times; the retry re-ran authorization and may read a different answer", fixture.authorizer.calls)
	}
	if kept := fixture.requests.saved.CustomerSourceDataVersions(); len(kept) != 1 {
		t.Fatalf("versions on the request = %d, want the one original", len(kept))
	}
}

// Covers: UC-PS-002「同一请求身份携带不同内容、范围、基准或 `requestEffectiveAt` 形成请求冲突」
// 与 `AT-PS-017`「原请求和原版本不被覆盖」。
//
// 与重放分成两个结果，因为客户要做的事不同：重放是「这次请求已经办过了」，冲突是「同一个请求
// 身份底下压着两份不同内容，先纠正是哪一份」。按最后到达覆盖是用例明禁的。
func TestAnAmendmentSourceConflictNeitherOverwritesNorFormsASecondVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	original, formed := first.Version()
	if !formed {
		t.Fatal("the first amendment formed no version")
	}

	conflicting := fixture.command(t)
	conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "amend-digest-2")
	result, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("handle conflicting: %v", err)
	}

	if result.Outcome() != application.AmendmentSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("a conflicting request formed a version anyway")
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d version IDs; a conflicting request formed a second version", fixture.identities.issued)
	}
	// 原来源不被后到的内容覆盖：留痕一旦被改写，事后就分不出客户先说了什么、后说了什么。
	preserved, kept := fixture.sources.stored(t, amendmentSourceIdentity(t))
	if !kept || preserved.Digest().String() != "amend-digest-1" {
		t.Fatalf("preserved digest = %q kept = %v, want the original amend-digest-1", preserved.Digest(), kept)
	}
	versions := fixture.requests.saved.CustomerSourceDataVersions()
	if len(versions) != 1 || versions[0].VersionID() != original.VersionID() {
		t.Fatalf("versions on the request = %d; the conflicting request disturbed the original", len(versions))
	}
}

// Covers: UC-PS-002 步骤 4「依赖无法确定形成待复核」与业务未决结束「规则或授权无法确定」。
//
// 授权服务答不出与「这个人不能改这处资料」是两回事：后者续办也补不出授权来，前者重试就好。
// 判成未获授权会让客户以为自己越权，而真相是我们没问到；判成技术错误则连未决原因与续办引用
// 都给不出，而用例两样都要。
func TestAnUnavailableAmendmentAuthorizerIsUndecidedRatherThanUnauthorized(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.authorizer.err = errors.New("party-commercial unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED; an unreachable authorizer is not an answer about authority", result.Outcome())
	}
	if result.PendingReason() != application.SourceDataAmendmentAuthorityUnavailable {
		t.Fatalf("reason = %q, want SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("an undecided round left no continuation reference; the caller cannot resume what it cannot name")
	}
	if _, present := result.Version(); present {
		t.Fatal("an undecided round formed a version anyway")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; an undecided round consumed a scarce identity", fixture.identities.issued)
	}
}

// Covers: 同一条业务未决语义的规则一侧 — 「规则……无法确定」。
//
// 与未登记分开：未登记是矩阵答了「没有这条」，等的是有人去 `PAR-COM-13` 登记；读不回是矩阵没
// 答上话，等的是依赖恢复。合成一格，续办方就不知道该催人还是该重试。
func TestAnUnreadableSourceDataRuleIsUndecidedRatherThanAwaitingReview(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.err = errors.New("rule declaration unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED; an unreadable matrix is not the same as an unregistered one", result.Outcome())
	}
	if result.PendingReason() != application.SourceDataRuleUnavailable {
		t.Fatalf("reason = %q, want SOURCE_DATA_RULE_UNAVAILABLE", result.PendingReason())
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; an undecided round consumed a scarce identity", fixture.identities.issued)
	}
}

// Covers: UC-PS-002`技术未形成`「原始请求已保全，但版本提交或发布意图未完成」与「处理阶段、
// 失败位置和安全续办引用」— 签发不出版本标识时停在未决，来源已保全的事实不因此丢失。
func TestAnUnavailableVersionIdentityLeavesTheRequestUndecidedAndResumable(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.identities.err = errors.New("identity factory unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.SourceDataVersionIdentityUnavailable {
		t.Fatalf("reason = %q, want SOURCE_DATA_VERSION_IDENTITY_UNAVAILABLE", result.PendingReason())
	}
	if fixture.requests.saved != nil {
		t.Fatal("a round that formed no version saved the request anyway")
	}
	// 来源仍在：`技术未形成`的前提就是「原始请求已保全」，丢了它这一轮连续办都无从认领。
	if _, preserved := fixture.sources.stored(t, amendmentSourceIdentity(t)); !preserved {
		t.Fatal("an undecided round dropped the preserved source")
	}
}

// Covers: 同一条`技术未形成`的另一处失败位置 — 版本已形成但没落库。
//
// 不交回`已记录并采用`：用例明禁「不得返回已采用或业务拒绝」。版本没存住，下游按它办事就会
// 扑空，而本轮交回的续办引用正是让它重放同一次修订的凭据。
func TestAnUnsavedAmendmentIsUndecidedRatherThanRecorded(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.requests.saveErr = errors.New("storage unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.AmendedRequestNotSaved {
		t.Fatalf("reason = %q, want AMENDED_REQUEST_NOT_SAVED", result.PendingReason())
	}
	if _, present := result.Version(); present {
		t.Fatal("a version that never reached storage was reported as formed")
	}
	// 交接必须排在落库之后：先告诉下游再存，存失败时下游已经按一份并不存在的版本去重新判断了。
	if len(fixture.downstream.intents) != 0 {
		t.Fatalf("handed off %d intents for a version that never reached storage; downstream would fetch nothing", len(fixture.downstream.intents))
	}
}

// Covers: ADR-0031 —— 输给并发写入的修订与「存不进去」分属两格。
//
// 这一支同样不得交接：`AT-PS-031` 要的是「版本只形成一次」，而下游按一份并没落库的版本去
// 重新判断会直接扑空。它与上一条用例共用这条断言，因为两者对下游的后果一模一样，分开的是
// 续办方接下来该做什么。
func TestAnAmendmentLostToAConcurrentWriterIsUndecidedNotRecorded(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.requests.saveConflict = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.StaleShipmentRequestRevision {
		t.Fatalf("reason = %q, want STALE_SHIPMENT_REQUEST_REVISION", result.PendingReason())
	}
	if _, present := result.Version(); present {
		t.Fatal("一份输给并发写入的版本被报成已形成")
	}
	if len(fixture.downstream.intents) != 0 {
		t.Fatalf("handed off %d intents for a version that never reached storage; downstream would fetch nothing", len(fixture.downstream.intents))
	}
}

// Covers: UC-PS-002 步骤 10「发布失败只重试同一发布意图，不重复形成资料版本」与`技术未形成`
// 「原始请求已保全，但版本或发布意图未完成提交」。
//
// 不交回`已记录并采用`：下游还没听说这份版本，而`已记录并采用`那一行的含义是「当前资料版本采用
// 判断明确可供下游引用」。版本本身已经形成并落库——`技术未形成`的前提正是它已经存住，续办要重
// 发的是同一份意图，不是再形成一份版本。
func TestAnUndeliveredHandoffLeavesTheRoundUndecidedWithoutReformingTheVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.downstream.err = errors.New("downstream unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED; a version no downstream has heard of is not yet available for them to reference", result.Outcome())
	}
	if result.PendingReason() != application.SourceDataVersionNotHandedOff {
		t.Fatalf("reason = %q, want SOURCE_DATA_VERSION_NOT_HANDED_OFF", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("an undelivered handoff left no continuation reference; the caller cannot resume what it cannot name")
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d version IDs, want the one version this round formed", fixture.identities.issued)
	}
	if kept := fixture.requests.saved.CustomerSourceDataVersions(); len(kept) != 1 {
		t.Fatalf("versions on the request = %d; the version must survive the failed handoff, or the retry has nothing to hand off", len(kept))
	}
}

// Covers: `AT-PS-031`「资料版本已形成但下游事件首次发布失败 → 版本只形成一次；仅重试同一发布
// 意图」。
//
// 重放必须把同一份意图再交一次。只答`已有结果`就收工，那份版本会永远停在「本上下文已形成、下游
// 从不知道」的状态——而这条路上没有别的东西会去补发：编排不记意图完没完成，那份状态要与版本同
// 一事务落库才算数，而事务与 outbox 仍阻断于 ADR-0017。
//
// 两次交出去的必须是同一个版本标识。不同就说明重放又形成了一份版本，那是第二份意图而不是同一
// 份的重试，下游会按两份互不相认的引用各判一次。
func TestARetryAfterAFailedHandoffResendsTheSameIntentWithoutFormingASecondVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.downstream.err = errors.New("downstream unreachable")
	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	if first.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want the first round to stop at UNDECIDED", first.Outcome())
	}

	fixture.downstream.err = nil
	retried, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle retry: %v", err)
	}

	if retried.Outcome() != application.AmendmentAlreadyHandled {
		t.Fatalf("outcome = %q, want ALREADY_HANDLED once the same intent gets through", retried.Outcome())
	}
	if len(fixture.downstream.intents) != 2 {
		t.Fatalf("handed off %d times; the retry never re-sent the intent, so downstream never hears of this version", len(fixture.downstream.intents))
	}
	if fixture.downstream.intents[0].Version != fixture.downstream.intents[1].Version {
		t.Fatalf(
			"retry carried %q but the first carried %q; that is a second intent, not a retry of the first",
			fixture.downstream.intents[1].Version, fixture.downstream.intents[0].Version,
		)
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d version IDs; the retry formed a second version", fixture.identities.issued)
	}
}

// Covers: 同一条业务未决语义在委托读取一侧 — 依赖答不出与「指名了一份不存在的委托」分开。
//
// 后者仍上抛（调用方对世界的判断就是错的），前者是依赖抖动，重试就好。两者都写成错误的话，
// 一次存储抖动会被记成调用方的编程错误。
func TestAnUnreadableShipmentRequestIsUndecidedRatherThanAnError(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.requests.err = errors.New("storage unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.ShipmentRequestUnavailable {
		t.Fatalf("reason = %q, want SHIPMENT_REQUEST_UNAVAILABLE", result.PendingReason())
	}
	if fixture.authorizer.calls != 0 {
		t.Fatalf("authorized %d times; a round that never read the request went on to ask about authority", fixture.authorizer.calls)
	}
}

// Covers: `judgment_continuation.go`「原因参与派生也意味着停在不同阶段的两次未决给出不同引用」。
//
// 两轮的范围完全一致，唯一的变量是原因；引用一旦相同就说明原因根本没进派生，那样所有按原因
// 区分的续办断言都是假的。这一条钉的是派生本身，不是某一个调用点。
func TestTwoDifferentAmendmentStoppingCausesNeverShareOneContinuation(t *testing.T) {
	unavailableAuthorizer := newAmendmentFixture(t)
	unavailableAuthorizer.authorizer.err = errors.New("party-commercial unreachable")
	authorityStop, err := unavailableAuthorizer.handler.Handle(context.Background(), unavailableAuthorizer.command(t))
	if err != nil {
		t.Fatalf("handle with unavailable authorizer: %v", err)
	}

	unreadableRules := newAmendmentFixture(t)
	unreadableRules.rules.err = errors.New("rule declaration unreachable")
	ruleStop, err := unreadableRules.handler.Handle(context.Background(), unreadableRules.command(t))
	if err != nil {
		t.Fatalf("handle with unreadable rules: %v", err)
	}

	if authorityStop.ContinuationReference() == ruleStop.ContinuationReference() {
		t.Fatalf(
			"both stopping causes derived %q; the reason does not participate in the derivation",
			authorityStop.ContinuationReference(),
		)
	}
}

type amendmentFixture struct {
	handler    *application.AmendCustomerSourceDataHandler
	sources    *sourceRepositoryDouble
	requests   *amendableRequestStore
	authorizer *amendmentAuthorizerDouble
	rules      *sourceDataRuleDouble
	identities *sourceDataIdentityFactory
	downstream *sourceDataHandoffDouble
	steps      []string
}

func newAmendmentFixture(t *testing.T) *amendmentFixture {
	t.Helper()
	value := &amendmentFixture{
		requests:   &amendableRequestStore{t: t},
		authorizer: &amendmentAuthorizerDouble{t: t, granted: true},
		rules:      &sourceDataRuleDouble{allowance: ports.SourceDataAmendmentAllowed},
		identities: &sourceDataIdentityFactory{t: t},
		downstream: &sourceDataHandoffDouble{},
	}
	value.sources = &sourceRepositoryDouble{
		records: map[domain.SourceIdentity]domain.SourceSubmissionFingerprint{},
		record:  func(step string) { value.steps = append(value.steps, step) },
	}
	value.authorizer.record = func(step string) { value.steps = append(value.steps, step) }
	value.handler = application.NewAmendCustomerSourceDataHandler(application.AmendCustomerSourceDataDeps{
		Sources:    value.sources,
		Requests:   value.requests,
		Authorizer: value.authorizer,
		Rules:      value.rules,
		Identities: value.identities,
		Downstream: value.downstream,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return value
}

// amendmentSourceIdentity 是修订请求自己的来源身份，与产生委托的那一次提交分开：同一个客户
// 就同一份委托先提交后修订，是两次来源请求，各自有各自的请求标识与内容摘要。
func amendmentSourceIdentity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	return sourceIdentity(t, "tenant-1", "customer-1", "source-a", "amend-key-1")
}

// consigneeDataScope 是一处委托级资料范围。委托级而非逐包裹：寄收件一类资料本就作用于整份
// 委托，逐包裹填会把一处更正复制成成员份数。
func consigneeDataScope(t *testing.T) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewSourceDataGroupReference, "CONSIGNEE_ADDRESS"),
	)
	if err != nil {
		t.Fatalf("new shipment scoped source data: %v", err)
	}
	return scope
}

// outsideBaselineScope 指名一个接受基线之外的成员。acceptedRequest 把基线固定为 parcel-1 与
// parcel-2，所以 parcel-9 就是一次「拿更正往已接受委托里塞成员」——`AT-PS-023` 说的正是它。
func outsideBaselineScope(t *testing.T) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewParcelScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-9"),
		mustValue(t, domain.NewSourceDataGroupReference, "GOODS_DESCRIPTION"),
	)
	if err != nil {
		t.Fatalf("new parcel scoped source data: %v", err)
	}
	return scope
}

func (value *amendmentFixture) command(t *testing.T) application.AmendCustomerSourceDataCommand {
	t.Helper()
	return application.AmendCustomerSourceDataCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		AmendmentIdentity: amendmentSourceIdentity(t),
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "amend-digest-1"),
		OccurredAt:        handlerClockAt,
		ReceivedAt:        handlerClockAt,
		Scope:             consigneeDataScope(t),
		Basis:             domain.NewSupplementOnAcceptanceBaseline(),
		Reason:            mustValue(t, domain.NewAmendmentReasonReference, "CONSIGNEE_ADDRESS_CORRECTION"),
		Requester:         mustValue(t, domain.NewRequesterReference, "CUSTOMER-CONTACT-1"),
	}
}

// amendableRequestStore 交回一份真正经领域接受过的委托。造一个「看起来已接受」的假状态挡不住
// AmendCustomerSourceData 的状态闸门，也就证明不了编排走对了路。
//
// 保存过就交回保存的那一份，而不是每次都重新造：重放与冲突都要跨两次调用才谈得上，每次交回
// 一份干净委托等于让第二次调用看不见第一次的版本，「不能创建第二个资料版本」也就无从断言。
type amendableRequestStore struct {
	t *testing.T
	// notYetAccepted 交回一份停在`已提交`的委托，用来分开「没有基线」与「越过基线」。
	notYetAccepted bool
	err            error
	saveErr        error
	// saveConflict 用布尔而不是直接收枚举：那个枚举的零值是`未设`，收它会让每一处没显式
	// 设过的构造都变成一次端口坏了。
	saveConflict bool
	saved        *domain.ShipmentRequest
}

func (store *amendableRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
	if store.err != nil {
		return domain.ShipmentRequest{}, false, store.err
	}
	if store.notYetAccepted {
		return submittedRequest(store.t), true, nil
	}
	if store.saved != nil {
		return *store.saved, true, nil
	}
	return acceptedRequest(store.t), true, nil
}

func (store *amendableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) error {
	return nil
}

func (store *amendableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.saveErr != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, store.saveErr
	}
	if store.saveConflict {
		// 抢先那一方已经落库，本方这一份不留：留住它会让下一次 FindBySourceIdentity 读到
		// 一份其实没落库的聚合，而那正是这条分支要防的事。
		return ports.ShipmentRequestRevisionConflict, nil
	}
	store.saved = &request
	return ports.ShipmentRequestSaved, nil
}

// acceptedRequest 把 submittedRequest 经领域推到`已接受`。资料修订只对`已接受`开放，所以
// 这一层是被测行为的前提而不是它的一部分。
func acceptedRequest(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	applicable, err := domain.NewApplicableCheckGroups(domain.NetworkReachabilityCheck)
	if err != nil {
		t.Fatalf("new applicable check groups: %v", err)
	}
	snapshot, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID: mustValue(t, domain.NewCommercialResolutionID, "RES-1"),
		RulePackage:  mustValue(t, domain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision: mustValue(t, domain.NewCommercialViewRevision, "VIEW-1"),
		Applicable:   applicable,
		ManualReview: domain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("new commercial basis snapshot: %v", err)
	}

	checks := make([]domain.AcceptanceCheck, 0, 2)
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		check, err := domain.NewAcceptanceCheck(
			domain.NetworkReachabilityCheck,
			mustValue(t, domain.NewDeclaredParcelID, parcel),
			domain.CheckPassed,
			domain.CheckReason{},
		)
		if err != nil {
			t.Fatalf("new acceptance check: %v", err)
		}
		checks = append(checks, check)
	}

	accepted, err := submittedRequest(t).Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Checks:     checks,
		Basis:      snapshot,
		DecidedAt:  handlerClockAt,
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if accepted.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", accepted.State())
	}
	return accepted
}

type amendmentAuthorizerDouble struct {
	t       *testing.T
	granted bool
	err     error
	calls   int
	record  func(string)
}

func (double *amendmentAuthorizerDouble) AuthorizeSourceDataAmendment(
	_ context.Context,
	_ ports.SourceDataAmendmentAuthorizationQuery,
) (ports.SourceDataAmendmentAuthorization, error) {
	double.t.Helper()
	double.calls++
	if double.record != nil {
		double.record("authorize-amendment")
	}
	if double.err != nil {
		return ports.SourceDataAmendmentAuthorization{}, double.err
	}
	if !double.granted {
		return ports.SourceDataAmendmentAuthorization{}, nil
	}
	return ports.SourceDataAmendmentAuthorization{
		Authority: mustValue(double.t, domain.NewAmendmentAuthoritySnapshot, "PC-AMEND-GRANT-1"),
		Decider:   mustValue(double.t, domain.NewDeciderReference, "OPERATOR-1"),
	}, nil
}

type sourceDataRuleDouble struct {
	allowance ports.SourceDataAmendmentAllowance
	err       error
	calls     int
}

func (double *sourceDataRuleDouble) DeclareSourceDataAmendment(
	_ context.Context,
	_ ports.SourceDataAmendmentQuery,
) (ports.SourceDataAmendmentAllowance, error) {
	double.calls++
	if double.err != nil {
		return ports.SourceDataAmendmentNotDeclared, double.err
	}
	return double.allowance, nil
}

type sourceDataIdentityFactory struct {
	t      *testing.T
	err    error
	issued int
}

func (factory *sourceDataIdentityFactory) NextSourceDataVersionID(
	_ context.Context,
) (domain.SourceDataVersionID, error) {
	factory.t.Helper()
	if factory.err != nil {
		return domain.SourceDataVersionID{}, factory.err
	}
	factory.issued++
	return mustValue(factory.t, domain.NewSourceDataVersionID, "data-version-1"), nil
}

// sourceDataHandoffDouble 记下每一份交出去的意图。按份数而不是按布尔断言：`AT-PS-031` 要的是
// 「版本只形成一次、仅重试同一发布意图」，而两份意图与一份重发的意图，只有计数分得开。
type sourceDataHandoffDouble struct {
	err     error
	intents []ports.SourceDataVersionHandoffIntent
}

func (double *sourceDataHandoffDouble) HandOffSourceDataVersion(
	_ context.Context,
	intent ports.SourceDataVersionHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
}

var (
	_ ports.SourceDataVersionHandoff      = (*sourceDataHandoffDouble)(nil)
	_ ports.ShipmentRequestRepository     = (*amendableRequestStore)(nil)
	_ ports.SourceDataAmendmentAuthorizer = (*amendmentAuthorizerDouble)(nil)
	_ ports.SourceDataRuleDeclaration     = (*sourceDataRuleDouble)(nil)
	_ ports.SourceDataVersionIdentity     = (*sourceDataIdentityFactory)(nil)
)
