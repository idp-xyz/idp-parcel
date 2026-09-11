import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Textarea } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../../pages/catalogue-api';

/**
 * 登记写面的表单区（ADR-0085，票 admin-write-faces/01 切片 01b 首落于计价页；票 02
 * 切片 02a 抽为与上下文无关的共享组件，供各登记册页的「登记」签复用）。
 *
 * **三态如实呈现**是本区存在的理由，不是顺带：
 *
 *   - 403 `ACCESS_CHANNEL_NOT_CONFIGURED` —— 接入渠道未配置。这是今天必然的答复：登记
 *     端点已在装配表里，但挂的是字面量 `UnconfiguredIntake{}`，写准入与其余命令面同等
 *     `PAR-INT-01` 证据（决定一）。它不是接线缺陷，改请求或重试都不会好。
 *   - 登记册治理答案 —— 各登记口自己的答案代数（已入册 / 已在册 / 内容冲突 / 受理门
 *     拒绝……），逐格中文由调用页经 `outcomeLabels` 给出。负向答案是**答案不是失败**：
 *     原行不被顶替，续办属登记方或治理裁决。折成一句「提交失败」会让操作者以为重试有用。
 *   - 未决 —— 依赖故障，登记与否未知，可重试。
 *
 * 表单收的是登记快照 JSON 本体，与受控登记 CLI 吃的同一份形状。**不逐字段建表单**：
 * 「渠道原始载荷 → 登记快照」的翻译按决定三属渠道接入契约、随 `PAR-INT-01` 提供，
 * 现在拆成字段就是替租户拟那份契约。
 *
 * 词表与错误码说明都由调用页传入而不在这里内置：答案代数归各登记口自己的应用结果枚举，
 * 错误码说明归各上下文自己的 presentation——共享的是呈现形状，不是任何一个上下文的词。
 */

/**
 * 登记端点的封闭响应形状：`outcome` 取应用结果枚举原名，与登记 CLI 同源。
 * `refusalReason` 只在受理门拒绝且该登记口的答案代数带指名理由时在场（network-routing
 * 目录登记是首例）；其余登记口缺席，缺席即该口的答案不分理由，不是漏字段。
 */
export interface RegistrationResponseBody {
  outcome: string;
  refusalReason?: string;
  /**
   * 受理门拒绝的**散文**原因（party-commercial 的身份族与产品渠道族是首例）。
   *
   * 它与 `refusalReason` 分开而不是共用一格：那一格是封闭枚举，逐格有中文词表，缺格
   * 时原名过线仍读得懂；这一格是用例随结果交回的一句话（「修订必须连续：册上最新为
   * 2，收到 5」），没有代数可查表。折进 `refusalReason` 会让散文冒充枚举，而调用侧
   * 一旦对着它分支，用例改一个字就拆掉了。
   *
   * 那为什么不丢掉：登记方拿一个没有指名的 `NOT_ACCEPTED` 什么也补不了——他分不出是
   * 修订跳号、引用悬空还是领域门拒。要可判别的理由代数，得在用例侧立封闭枚举（先例
   * 是网络目录登记的 `CatalogRefusalReason`），不是在传输层按字符串拼。
   */
  cause?: string;
  /**
   * 发布未决的原因（party-commercial 的发布口）。它只随`发布未决`在场，说的是「等批准
   * 角色确认」还是「等正文指名的对象发布」——两者的续办动作不同，靠格分辨不出来。
   *
   * 不与 `cause` 共用一个名：那一格随`未受理`（输入被登记册的引用检查拒绝），这一格的
   * 输入本身没问题。同名会让两种续办动作看起来是一回事。
   */
  pendingCause?: string;
  /**
   * 业务未决的**具名**原因（customs-compliance 的税费付款协作 / 核对两口是首例，票 sa-cc/07）。
   * 它只随 `UNDECIDED` 在场，说的是编排在等谁（「等核定税费或明确无需付款依据」）——那是一个
   * 形成了的业务答案，按 ADR-0022 走 200；依赖故障那种未决不带 outcome、走 5xx，到不了这一格。
   *
   * 不与 `pendingCause` 共用：那一格是散文、原样示出；这一格是封闭枚举原名，逐格有中文词表
   * （`undecidedReasonLabels`），缺格时原名过线。也不与 `refusalReason` 共用：受理门拒绝要改
   * 内容再来，业务未决要等前置到了重发同一份——续办动作相反，同名会让两种动作看起来是一回事。
   */
  undecidedReason?: string;
  /**
   * 随本次发布一并登记的各声明通道落点。
   *
   * **这一栏不能省。** 声明与版本同笔落库，某个通道撞上同键异内容时版本仍可能答
   * `PUBLISHED_EFFECTIVE`——不呈现它，一次半数声明没进去的发布在页面上与全都落定的
   * 发布长得一模一样，而受控 CLI 的 publish 恰恰按这一格抬退出码要商业责任方去看。
   * 两口对同一件事不同答法，本身就是个缺口。
   */
  declarations?: RegistrationDeclarationLanding[];
}

/** 一个声明通道的落点。通道名与落点名都取应用枚举原名，不改名也不合并。 */
export interface RegistrationDeclarationLanding {
  channel: string;
  outcome: string;
}

export interface RegistrationPanelProps {
  moduleId: string;
  title: string;
  endpoint: string;
  /** 快照形状的一句话提示，取各自受控登记 CLI 的文档口径。 */
  snapshotHint: string;
  submit: (snapshot: unknown) => Promise<ApiResult<RegistrationResponseBody>>;
  /** 该登记口答案代数的逐格中文（outcome 原名 → 中文）；未收录的原样示出英文原名。 */
  outcomeLabels: Record<string, string>;
  /** 受理门拒绝理由的逐格中文；只有答案带 `refusalReason` 的登记口需要给。 */
  refusalReasonLabels?: Record<string, string>;
  /** 业务未决原因的逐格中文；只有答案带 `undecidedReason` 的登记口需要给。 */
  undecidedReasonLabels?: Record<string, string>;
  /**
   * 声明通道落点的逐格中文；只有答案带 `declarations` 的登记口需要给。
   *
   * 单立一张表而不复用 `outcomeLabels`：两套代数有重名格。`CONTENT_CONFLICT` 在版本那
   * 一栏说的是「同键异内容，改内容要发新版本号」，在声明这一栏说的是「同拥有版本携带
   * 不同正文」——续办动作不同。共用一张表，其中一种会顶着另一种的中文显示出来。
   */
  declarationLandingLabels?: Record<string, string>;
  /** problem+json 错误码的中文说明，取该上下文自己的 presentation，不在此处统一措辞。 */
  problemNote: (code: string) => string;
}

type PanelState =
  | { kind: 'idle' }
  | { kind: 'submitting' }
  | { kind: 'malformed'; message: string }
  | { kind: 'answered'; answer: ApiResult<RegistrationResponseBody> };

export function RegistrationPanel({
  moduleId,
  title,
  endpoint,
  snapshotHint,
  submit,
  outcomeLabels,
  refusalReasonLabels,
  undecidedReasonLabels,
  declarationLandingLabels,
  problemNote,
}: RegistrationPanelProps) {
  const info = moduleInfoById[moduleId];
  const [draft, setDraft] = useState('');
  const [state, setState] = useState<PanelState>({ kind: 'idle' });

  const send = () => {
    let snapshot: unknown;
    try {
      snapshot = JSON.parse(draft);
    } catch (error) {
      // 本地就能判的畸形不送上去：送一份解析不了的载荷，回来的 400 会与服务端的业务
      // 拒绝挤在同一格里，而两者的续办动作不同。
      setState({
        kind: 'malformed',
        message: error instanceof Error ? error.message : String(error),
      });
      return;
    }
    setState({ kind: 'submitting' });
    void submit(snapshot).then((answer) => setState({ kind: 'answered', answer }));
  };

  return (
    <div className="flex-1 overflow-auto p-4">
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-xs text-idpxyz-textMuted">
            {snapshotHint}
            <br />
            提交打到 <span className="font-mono">{endpoint}</span>；在线登记口与受控登记 CLI
            消费同一登记用例，答案代数一致。
          </p>
          <Textarea
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="粘贴登记快照 JSON"
            rows={12}
            className="font-mono text-xs"
          />
          <div className="flex items-center gap-3">
            <Button onClick={send} disabled={state.kind === 'submitting' || draft.trim() === ''}>
              {state.kind === 'submitting' ? '提交中…' : '提交登记'}
            </Button>
            <AnswerNote
              state={state}
              owner={info.owner}
              outcomeLabels={outcomeLabels}
              refusalReasonLabels={refusalReasonLabels}
              undecidedReasonLabels={undecidedReasonLabels}
              declarationLandingLabels={declarationLandingLabels}
              problemNote={problemNote}
            />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function AnswerNote({
  state,
  owner,
  outcomeLabels,
  refusalReasonLabels,
  undecidedReasonLabels,
  declarationLandingLabels,
  problemNote,
}: {
  state: PanelState;
  owner: string;
  outcomeLabels: Record<string, string>;
  refusalReasonLabels?: Record<string, string>;
  undecidedReasonLabels?: Record<string, string>;
  declarationLandingLabels?: Record<string, string>;
  problemNote: (code: string) => string;
}) {
  if (state.kind === 'idle' || state.kind === 'submitting') return null;
  if (state.kind === 'malformed') {
    return <p className="text-xs text-idpxyz-danger">不是合法 JSON，未提交：{state.message}</p>;
  }

  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const outcome = answer.body.outcome;
      const label = outcomeLabels[outcome] ?? outcome;
      // 未收录的 outcome 原样示出：服务端新增一格时，页面宁可显示英文原名，也不把它
      // 归进某个既有中文说法——那会让一种新答案冒充另一种。拒绝理由同一纪律。
      const reason = answer.body.refusalReason;
      // 业务未决的具名原因与拒绝理由同一纪律：查表译中文，未收录原名过线。
      const undecided = answer.body.undecidedReason;
      // 散文原因与发布未决原因原样示出，不查表也不截断：它们没有代数可查，而截断掉的
      // 往往正是「册上最新为几、收到几」那半句——登记方要改的就是那个数。
      const prose = answer.body.cause ?? answer.body.pendingCause;
      const landings = answer.body.declarations ?? [];
      return (
        <div className="text-xs text-idpxyz-textMuted">
          <p>
            登记册答复：<span className="font-mono">{outcome}</span> —— {label}
            {reason ? (
              <>
                ；拒绝理由：<span className="font-mono">{reason}</span> ——{' '}
                {refusalReasonLabels?.[reason] ?? reason}
              </>
            ) : null}
            {undecided ? (
              <>
                ；在等：<span className="font-mono">{undecided}</span> ——{' '}
                {undecidedReasonLabels?.[undecided] ?? undecided}
              </>
            ) : null}
          </p>
          {prose ? <p className="mt-1">原因：{prose}</p> : null}
          {landings.length > 0 ? (
            <div className="mt-1">
              {/* 声明落点与版本答案分行列出，不折进上面那句：某通道内容冲突时版本
                  仍可能是「已发布已生效」，两者挤在一行会让人只读到前半句。 */}
              <p>随本次发布登记的声明落点：</p>
              <ul className="mt-0.5 ml-4 list-disc">
                {landings.map((landing) => (
                  <li key={landing.channel}>
                    <span className="font-mono">{landing.channel}</span> ——{' '}
                    <span className="font-mono">{landing.outcome}</span>{' '}
                    {declarationLandingLabels?.[landing.outcome] ?? ''}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      );
    }
    case 'unconfigured':
      return (
        <p className="text-xs text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。登记端点已建立并装配，但
          {owner}的接入渠道认证方式尚未登记，服务端按 ADR-0055 如实拒绝——这是诚实答案，
          改请求或重试都不会改变结果；登记参数（PAR-INT-01，实例半边）到位后由装配侧换上
          真 Intake 即放行。此前登记仍走受控登记 CLI。
        </p>
      );
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          服务端未形成答案（HTTP {answer.status}）：{problemNote(answer.code)}；登记与否未知，
          可稍后重试。
        </p>
      );
    case 'transport':
      return (
        <p className="text-xs text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>
      );
  }
}
