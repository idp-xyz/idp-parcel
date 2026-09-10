import type {
  AuthorizedDispositionChoice,
  AuthorizedDispositionOutcome,
  CancellationOutcome,
  SubmitOutcome,
  WithdrawalOutcome,
} from './api';

// outcome 与领域状态的中文标签。标签取 UC-PS-001 / UC-PS-005 / UC-PS-006 结果语义
// 的原词,不自造译法;note 讲操作员的下一步,措辞守住各结果的「禁止行为」——尤其是
// 准入暂停不是业务拒绝、未决不是拒绝、不可见结果不区分不存在与无权查看。

export interface OutcomeView {
  label: string;
  note: string;
  /** 标为 true 的结果代表本次动作成立,呈现时用强调色;其余是同等有效的业务答案,不是故障。 */
  affirmative?: boolean;
}

export const submitOutcomeViews: Record<SubmitOutcome, OutcomeView> = {
  SUBMITTED: {
    label: '已提交',
    affirmative: true,
    note: '委托已建立并记录为「已提交」。这不表示已接受:接受或拒绝由接受判断流程另行形成。',
  },
  EXISTING_RESULT: {
    label: '已有结果',
    note: '本次输入被识别为已有逻辑提交的重复或重试,返回原结果,不会建立第二份委托。',
  },
  INGRESS_CONFLICT: {
    label: '接入冲突',
    note: '相同请求身份携带了与原版本不一致的内容。原始提交不被覆盖,请经受控纠正入口处理。',
  },
  INPUT_NOT_ACCEPTED: {
    label: '输入未受理',
    note: '原始内容无法确定客户范围、委托边界或最小成员身份,尚不存在可接受或拒绝的委托。请修改后重新提交。',
  },
  OTHER_PRODUCTION_AUTHORITY: {
    label: '非本产品生产归属',
    note: '版本化试点规则已确定该范围由另一当前唯一生产权威承接,本产品不建立委托决定。',
  },
  OWNERSHIP_UNRESOLVED: {
    label: '生产归属未决',
    note: '当前唯一生产权威无法确定,或无法证明能够安全交接。原始提交已保全,可按续办引用安全续办;这不是拒绝。',
  },
  ADMISSION_PAUSED: {
    label: '准入暂停',
    note: '本产品当前暂停接纳新准入。这不是第四种权威身份,也不是客户业务拒绝,不改变既有权威与在途责任。',
  },
  PRIOR_REQUEST_NOT_FOUND: {
    label: '关联原委托不可见',
    note: '指名的原委托在当前客户范围内查不到。统一不可见结果,不区分「不存在」与「无权查看」。',
  },
  LINK_INELIGIBLE: {
    label: '原委托状态不承认该关联',
    note: '原委托已找到且属于当前客户,但其状态不支持这个关联方向。请改走资料修订,或等待其决定形成。',
  },
};

export const withdrawalOutcomeViews: Record<WithdrawalOutcome, OutcomeView> = {
  FORMED: {
    label: '撤回已成立',
    affirmative: true,
    note: '整份待决委托已撤回。原始提交、提交版本与判断历史全部保留,撤回不是运营企业拒绝。',
  },
  NOT_AUTHORIZED: {
    label: '未获授权',
    note: '请求方没有本次撤回的有效授权,委托当前状态不变。',
  },
  DECISION_ALREADY_FORMED: {
    label: '已有决定',
    note: '接受或拒绝决定已先行成立,决定前撤回不再可用。请按已形成的决定选择对应的后续入口。',
  },
  UNDECIDED: {
    label: '未决',
    note: '本次未能形成撤回结果,不会被写成拒绝。已保留续办引用,可安全续办。',
  },
  SOURCE_CONFLICT: {
    label: '来源冲突',
    note: '相同撤回请求身份携带了不一致的内容,原撤回请求不被覆盖。',
  },
};

// UC-PS-006 接受后取消的结果词表。三种已提交走向(取消成立、待处置、拒绝)全是
// 同等有效的业务答案:待处置与拒绝不是故障,不得画成错误;未决不是拒绝。
export const cancellationOutcomeViews: Record<CancellationOutcome, OutcomeView> = {
  PARCEL_CANCELLED: {
    label: '包裹已取消',
    affirmative: true,
    note: '包裹在适用取消边界前形成取消终局。身份、接受基线、请求、授权与既有交易或作业历史全部保留;已有面单交易时,取消不能代替渠道作废或渠道退款结果。',
  },
  DISPOSITION_PENDING: {
    label: '待处置',
    note: '有效网络收寄已先行成立,明确不能回退取消。没有授权处置决定时保持待处置:处置决定由后续独立请求形成,越过的收寄版本已随记录保全。',
  },
  CANCELLATION_REFUSED: {
    label: '取消被拒绝',
    note: '当前有效规则明确不允许所请求的取消,规则依据随结果返回。这是业务答案,不是故障;包裹当前状态不变。',
  },
  CANCELLATION_UNDECIDED: {
    label: '未决',
    note: '当前事实、规则、授权或可执行性不足,本次未形成取消或处置决定,不会被写成拒绝。缺口与续办引用已保存,可安全续办。',
  },
  EXISTING_RESULT: {
    label: '已有结果',
    note: '本次请求被识别为同一请求身份的重复或重试,返回原逐包裹结果,不形成第二份决定。',
  },
  REQUEST_CONFLICT: {
    label: '请求冲突',
    note: '同一请求身份携带了与原取消请求不一致的内容,原请求不被覆盖。请核对后以新的请求身份重新提出。',
  },
  REQUEST_NOT_ACCEPTED: {
    label: '请求未受理',
    note: '必要内容缺失,或指名的委托与包裹在当前客户范围内查不到。统一不可见结果,不区分「不存在」与「无权查看」,也不提示差在哪一段指名。',
  },
};

export interface PendingReasonView {
  label: string;
  note: string;
  /**
   * 标为 true 的原因是实例参数未登记的「未配置态」:恢复动作是登记参数,重试、
   * 改请求或修依赖都不会改变答案。它不是错误,也不是业务否定。
   */
  unconfigured?: boolean;
}

// 取消未决原因(CancelUndecidedReason 的字符串)。词表区分两类恢复动作:
// AUTHORITY_UNCONFIGURED 是唯一的未配置态;其余 *_UNAVAILABLE 是依赖本次没答上,
// 可稍后按续办引用续办。
export const cancellationPendingReasonViews: Record<string, PendingReasonView> = {
  REQUEST_UNAVAILABLE: {
    label: '委托读取暂不可用',
    note: '本次没能读到目标委托,判断没有开始。可稍后按续办引用续办。',
  },
  AUTHORITY_UNAVAILABLE: {
    label: '授权判断暂不可用',
    note: '授权服务本次没有给出答案,判断停在授权步。可稍后按续办引用续办;这不是拒绝。',
  },
  AUTHORITY_UNCONFIGURED: {
    label: '授权规则未登记',
    unconfigured: true,
    note: '租户尚未登记取消授权规则(PAR-COM-17 实例半边),判断停在指名未决。系统不默认任何角色可取消或不可取消(UC-PS-006 明禁双向默认),恢复动作是租户登记授权规则——重试或改请求都不会改变答案。',
  },
  ADOPTION_STORE_UNAVAILABLE: {
    label: '收寄事实读取暂不可用',
    note: '本次没能读到当前有效收寄,取消边界无法核验。可稍后按续办引用续办。',
  },
  CANCELLATION_STORE_UNAVAILABLE: {
    label: '取消决定库暂不可用',
    note: '判断结果本次没能提交。可稍后按续办引用续办,重放不会形成第二份决定。',
  },
  CANCELLATION_IDENTITY_UNAVAILABLE: {
    label: '取消标识签发暂不可用',
    note: '取消决定标识本次没能签发,决定未形成。可稍后按续办引用续办。',
  },
};

// 授权处置的去向(domain.AuthorizedDispositionChoice 的字符串)。词取 ADR-0132 决定一原词:两个去向
// 都是「决定去向」,不是对受限控制的表决;这张表没有也不会有「放行」——硬句「人工处理不得绕过
// 硬规则或把缺少的权威结果改成通过」。
export const dispositionChoiceLabels: Record<AuthorizedDispositionChoice, string> = {
  REJECT: '拒绝',
  CUSTOMER_SUPPLEMENT: '交客户补充',
};

// 策略正文登记的失败处置(ControlFailureDisposition 的字符串)。词取 PC CONTEXT「失败处置只回答委托的
// 去向——按策略拒绝或进入授权处置」;它答的是委托去向,不拥有拒绝决定。
export const controlFailureDispositionLabels: Record<string, string> = {
  REJECT: '按策略拒绝',
  AUTHORIZED_DISPOSITION: '进入授权处置',
};

// 授权处置命令的结果词表(AuthorizedDispositionOutcome 的字符串)。分格照复核完成:`已记录`是本次动作
// 成立;`已有处置`/`任务已完结`/`版本已换代`/`未停在等待授权处置`/`并发冲突`都是同等有效的业务答案,
// 不画成错误;`未获授权`是确定的业务答案不是未决;`授权规则未登记`是唯一的未配置态。
export const authorizedDispositionOutcomeViews: Record<AuthorizedDispositionOutcome, OutcomeView> = {
  RECORDED: {
    label: '已记录',
    affirmative: true,
    note: '处置已记录并在同一命令事务里收口:「拒绝」形成授权角色拒绝决定,「交客户补充」把任务转入等待受控补充;两个去向都释放本版本已成立项的占用。处置从不形成接受。',
  },
  ALREADY_DISPOSED: {
    label: '已有处置',
    note: '本提交版本已由先到的处置记录定了去向(一版至多一次,不覆盖),交回的是先到那一份的去向。',
  },
  TASK_ALREADY_CLOSED: {
    label: '任务已完结',
    note: '接受判断任务已由既有决定收口或已随撤回停止,去向没有可选的对象;交回既有决定,处置不再可用。',
  },
  VERSION_SUPERSEDED: {
    label: '版本已换代',
    note: '客户已形成新的提交版本,这一份版本上的等待态随之失效。处置不会签到新版本头上;请按当前版本重读队列。',
  },
  NOT_WAITING_ON_DISPOSITION: {
    label: '未停在等待授权处置',
    note: '该委托当前停在别的等待态或根本没停,处置在此刻没有对象;本次不形成处置。',
  },
  REVISION_CONFLICT: {
    label: '并发冲突',
    note: '委托在处置期间被另一次写入推进,本次未落地。重读后再处置,重放不会形成第二份处置。',
  },
  NOT_AUTHORIZED: {
    label: '未获授权',
    note: '处置人没有本次处置的有效授权(处置权与复核权、拒绝权互不蕴含),委托当前状态不变。这是确定的业务答案,不是未决。',
  },
  AUTHORITY_RULES_NOT_CONFIGURED: {
    label: '授权规则未登记',
    note: '租户尚未登记授权处置的授权规则(PAR-COM-14 实例半边;首发期 party-commercial 的授权动作词汇里还没有这一格),什么也不落库。系统不默认任何角色可处置,恢复动作是租户登记授权规则——重试或改请求都不会改变答案。',
  },
};

/** 委托生命周期状态(domain.ShipmentRequestState 的字符串)。 */
export const requestStateLabels: Record<string, string> = {
  SUBMITTED: '已提交',
  ACCEPTED: '已接受',
  REJECTED: '已拒绝',
  WITHDRAWN: '已撤回',
};

/** 生产归属的权威身份(domain.ProductionAuthorityKind 的字符串)。 */
export const authorityLabels: Record<string, string> = {
  IDP_PARCEL: '本产品',
  OTHER: '其他权威',
  UNRESOLVED: '未决',
};

/** 准入控制。OPEN 只表示没有生效暂停,不表示允许建单。 */
export const admissionControlLabels: Record<string, string> = {
  OPEN: '开放',
  PAUSED: '暂停',
};

export const decisionKindLabels: Record<string, string> = {
  ACCEPTED: '已接受',
  REJECTED: '已拒绝',
};

/**
 * 接受判断任务状态(domain.AcceptanceTaskState 的字符串)。「已完成」只在接受或拒绝
 * 决定形成时到达;撤回成立到达的是「已停止」——判完了与没人再判了是两个事实,词表
 * 不合并(词取领域注释原词)。
 */
export const acceptanceTaskStateLabels: Record<string, string> = {
  RUNNING: '运行中',
  COMPLETE: '已完成',
  STOPPED: '已停止',
};

// 传输层错误码说明。它们不是业务原因目录:出现即表示应用层没答过,响应体也
// 刻意不带自由文本(防泄露),所以措辞只指下一步动作。
export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST:
    '请求构造不出命令,重发同样的内容不会改变结果。请检查填写内容后再提交。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
  UNNAMED_OUTCOME: '服务端交回了无法命名的结果,已按未形成答案处理,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

/** 中文标签后缀原词码,如「已提交(SUBMITTED)」,查记录与对协议都以原词为准。 */
export function withCode(label: string | undefined, code: string): string {
  return label ? `${label}(${code})` : code;
}
