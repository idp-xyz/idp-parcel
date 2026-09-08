import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { publicationOutcomeLabels } from './api';
import { labelOf, problemNote } from './presentation';
import {
  approveOutcomeLabels,
  approvePublicationDraft,
  draftStatusLabels,
  previewCommercialPublication,
  previewOutcomeLabels,
  publishOutcomeLabels,
  publishPublicationDraft,
  submitOutcomeLabels,
  submitPublicationDraft,
  type CommercialObjectKindName,
  type CommercialPublicationAnswerBody,
  type CommercialPublicationPayload,
  type PublicationDraftPublicationResponseBody,
  type PublicationDraftResponseBody,
  type PublicationPreviewResponseBody,
} from './publication-draft-api';
import {
  currentAnswer,
  draftReferenceOf,
  emptyFlow,
  flowActionLabels,
  flowStepLabels,
  flowSteps,
  isFormLocked,
  nextAction,
  payloadKey,
  problemsByField,
  reachedStep,
  unclaimedProblems,
  withApproval,
  withPreview,
  withPublication,
  withSubmission,
  type FlowAction,
  type FlowStep,
  type PublicationFlowState,
} from './publication-draft-flow';

/**
 * 商业发布五步组件「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」（票 admin-write-faces/16 落，子票
 * 09–17 各册表单共用；ADR-0126 Decision 三、四，ADR-0101 决定四、八）。
 *
 * **各册只带表单正文来，五步由这里走。** 册的表单以子节点渲染自己那几格，用 `assemblePayload` 把草稿组成
 * 「壳 + 正文」的载荷；本组件按载荷键判「预览过的是不是眼前这一份」、逐口发请求、按 outcome 原词
 * 停在哪一步（纯逻辑在 publication-draft-flow.ts）。预览与录入送**同一份**载荷，预览页上的摘要与载体
 * 上记下的逐字节相等（决定四）。
 *
 * **表单不算摘要、不收也不送批准人、不裁任何门**（伞票 07 硬句）：摘要与规范化版本由服务端答，这里只显；
 * 录入者与批准者由 Intake 从操作者信封取，载荷里没有身份格；预览答哪几格不对就显哪几格——逐格问题按
 * JSON 路径交给子节点挂到对应格旁，表单没认领的路径在下面单独列，不静默丢。**今天四口都挂
 * UnconfiguredIntake{}**，答 403 是诚实答案（ADR-0085 两阶段），组件如实显示，不假装可用。
 *
 * 载体在册（存为待批准）之后表单锁定；改内容走「重新开始」再录，登记册按`待批准期间修订`就地更新。
 */
export interface PublicationDraftFlowProps {
  /** 发布轴的对象类别（`CommercialObjectKind` 原词），也是壳上 `kind` 的取值；标题旁显示。 */
  kind: CommercialObjectKindName;
  title: string;
  /** 从册的表单草稿组出「壳 + 正文」的载荷；纯函数，每次渲染调一次。 */
  assemblePayload: () => CommercialPublicationPayload;
  /**
   * 本地组不出合法 JSON 类型的格（如整数格填了非整数），按 JSON 路径归组；非空即不放预览。**只限编不进
   * 类型的**，不是领域校验——空字段、恰一约束、区间先后一律送上去让服务端答。
   */
  localProblems?: Record<string, string[]>;
  /** 表单声明自己渲染了哪几条 JSON 路径；服务端点名的其余路径由本组件单独列出。 */
  fieldPaths: readonly string[];
  /** 表单正文，收到逐格问题（服务端 + 本地，按路径）与是否锁定。 */
  children: (form: PublicationFormContext) => ReactNode;
  /** 载体到达「发布」那一步（本次发布落定或已在册）时回调，页面借它刷同册读面。 */
  onPublished?: () => void;
}

export interface PublicationFormContext {
  problems: Record<string, string[]>;
  locked: boolean;
}

const unconfiguredNote =
  '四口已建立并装配（parcel-api 端点表），但操作者身份的接入渠道尚未登记，服务端按 ADR-0055 如实拒绝——' +
  '这是诚实答案不是尚未实现（ADR-0085 两阶段），改请求或重试都不会改变结果。登记接入渠道认证参数' +
  '（PAR-INT-01，实例半边）后由装配侧换上真 Intake 即放行；此前发布仍走受控发布 CLI。';

export function PublicationDraftFlow({
  kind,
  title,
  assemblePayload,
  localProblems = {},
  fieldPaths,
  children,
  onPublished,
}: PublicationDraftFlowProps) {
  const [flow, setFlow] = useState<PublicationFlowState>(emptyFlow());
  const [busy, setBusy] = useState<FlowAction | null>(null);

  const payload = assemblePayload();
  const key = payloadKey(payload);
  const step = reachedStep(flow, key);
  const action = nextAction(step);
  const locked = isFormLocked(step);

  const preview = currentAnswer(flow.preview, key);
  const submission = currentAnswer(flow.submission, key);
  const approval = currentAnswer(flow.approval, key);
  const publication = currentAnswer(flow.publication, key);

  const serverProblems =
    preview?.kind === 'outcome' && preview.body.outcome === 'NOT_ACCEPTED'
      ? problemsByField(preview.body.problems)
      : {};
  const problems = mergeProblems(serverProblems, localProblems);
  const unclaimed = unclaimedProblems(problems, fieldPaths);
  const hasLocalProblems = Object.values(localProblems).some((lines) => lines.length > 0);

  // 发布落定回调按键去重：同一份载荷到达发布那一步只报一次，重渲染不重复刷读面。
  const publishedKey = useRef<string | null>(null);
  useEffect(() => {
    if (step === 'publish' && publishedKey.current !== key) {
      publishedKey.current = key;
      onPublished?.();
    }
  }, [step, key, onPublished]);

  const run = (which: FlowAction) => {
    setBusy(which);
    const at = key;
    const settle = <Body,>(
      request: Promise<ApiResult<Body>>,
      apply: (flow: PublicationFlowState, key: string, answer: ApiResult<Body>) => PublicationFlowState,
    ) => {
      void request.then((answer) => {
        setFlow((current) => apply(current, at, answer));
        setBusy(null);
      });
    };
    switch (which) {
      case 'preview':
        return settle(previewCommercialPublication(payload), withPreview);
      case 'submit':
        return settle(submitPublicationDraft(payload), withSubmission);
      case 'approve':
        return settle(approvePublicationDraft(draftReferenceOf(payload)), withApproval);
      case 'publish':
        return settle(publishPublicationDraft(draftReferenceOf(payload)), withPublication);
    }
  };

  const started = flow.preview !== null;
  const actionDisabled = busy !== null || (action === 'preview' && hasLocalProblems);

  return (
    <Card className="m-4">
      <CardHeader>
        <CardTitle>
          {title}
          <span className="ml-2 font-mono text-[12px] text-idpxyz-textMuted">kind = {kind}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <p className="text-xs text-idpxyz-textMuted">
          五步：填表 → 预览摘要 → 存为待批准 → 批准 → 发布。预览与录入送同一份载荷，服务端过同一道构造门、
          在同一处算摘要，预览页上的规范化版本与内容摘要与载体上记下的逐字节相等。<strong>表单不算摘要、
          不收也不送批准人、不裁任何门</strong>：租户、录入者、批准者由接入渠道的操作者信封给；哪几格不对由
          预览答，答什么显什么。载体存为待批准之后表单锁定，改内容走「重新开始」再录。
        </p>

        <Stepper step={step} />

        {children({ problems, locked })}

        {unclaimed.length > 0 ? (
          <ul className="text-xs text-idpxyz-danger list-disc ml-4">
            {unclaimed.map((problem) => (
              <li key={`${problem.field}:${problem.problem}`}>
                <span className="font-mono">{problem.field}</span>：{problem.problem}
              </li>
            ))}
          </ul>
        ) : null}

        <div className="flex items-center gap-3 flex-wrap">
          {action !== null ? (
            <Button
              onClick={() => run(action)}
              disabled={actionDisabled}
              title={
                action === 'preview' && hasLocalProblems
                  ? '有格填的东西编不进载荷类型，先改那几格'
                  : undefined
              }
            >
              {busy === action ? `${flowActionLabels[action]}中…` : flowActionLabels[action]}
            </Button>
          ) : (
            <span className="text-xs text-idpxyz-textMuted">已发布。结果在对应册立刻可见。</span>
          )}
          {started ? (
            <Button variant="outline" disabled={busy !== null} onClick={() => setFlow(emptyFlow())}>
              重新开始
            </Button>
          ) : null}
        </div>

        {flow.preview !== null && preview === null && step === 'form' ? (
          <p className="text-xs text-idpxyz-textMuted">表单已改动，上一次预览及其后各步作废；请重新预览。</p>
        ) : null}
        {preview ? <PreviewNote answer={preview} /> : null}
        {submission ? <DraftNote answer={submission} what="录入" labels={submitOutcomeLabels} /> : null}
        {approval ? <DraftNote answer={approval} what="批准" labels={approveOutcomeLabels} /> : null}
        {publication ? <PublicationNote answer={publication} /> : null}
      </CardContent>
    </Card>
  );
}

function mergeProblems(
  server: Record<string, string[]>,
  local: Record<string, string[]>,
): Record<string, string[]> {
  const merged: Record<string, string[]> = {};
  for (const source of [server, local]) {
    for (const [field, lines] of Object.entries(source)) {
      if (lines.length === 0) continue;
      merged[field] = [...(merged[field] ?? []), ...lines];
    }
  }
  return merged;
}

function Stepper({ step }: { step: FlowStep }) {
  const reachedIndex = flowSteps.indexOf(step);
  return (
    <ol className="flex items-center gap-2 text-[12px]">
      {flowSteps.map((candidate, index) => {
        const reached = index <= reachedIndex;
        const current = index === reachedIndex;
        return (
          <li key={candidate} className="flex items-center gap-2">
            <span
              className={`px-2 py-0.5 rounded border ${
                current
                  ? 'border-idpxyz-accent text-idpxyz-accent'
                  : reached
                    ? 'border-idpxyz-border text-idpxyz-text'
                    : 'border-idpxyz-border text-idpxyz-textMuted'
              }`}
            >
              {index + 1}. {flowStepLabels[candidate]}
            </span>
            {index < flowSteps.length - 1 ? <span className="text-idpxyz-textMuted">→</span> : null}
          </li>
        );
      })}
    </ol>
  );
}

function PreviewNote({ answer }: { answer: ApiResult<PublicationPreviewResponseBody> }) {
  return (
    <FlowAnswerNote answer={answer} what="预览">
      {(body) => (
        <div className="text-xs text-idpxyz-textMuted flex flex-col gap-1">
          <p>
            预览答复：<span className="font-mono">{body.outcome}</span> —— {labelOf(previewOutcomeLabels, body.outcome)}
          </p>
          {body.outcome === 'PREVIEWED' ? <DigestLine canonicalization={body.canonicalization} digest={body.contentDigest} /> : null}
          {body.cause ? <p>成因：{body.cause}</p> : null}
        </div>
      )}
    </FlowAnswerNote>
  );
}

function DraftNote({
  answer,
  what,
  labels,
}: {
  answer: ApiResult<PublicationDraftResponseBody>;
  what: string;
  labels: Record<string, string>;
}) {
  return (
    <FlowAnswerNote answer={answer} what={what}>
      {(body) => (
        <div className="text-xs text-idpxyz-textMuted flex flex-col gap-1">
          <p>
            {what}答复：<span className="font-mono">{body.outcome}</span> —— {labelOf(labels, body.outcome)}
          </p>
          {body.status ? (
            <p>
              载体状态：<span className="font-mono">{body.status}</span> —— {labelOf(draftStatusLabels, body.status)}
            </p>
          ) : null}
          {body.contentDigest ? <DigestLine canonicalization={body.canonicalization} digest={body.contentDigest} /> : null}
          {body.cause ? <p>成因：{body.cause}</p> : null}
        </div>
      )}
    </FlowAnswerNote>
  );
}

function PublicationNote({ answer }: { answer: ApiResult<PublicationDraftPublicationResponseBody> }) {
  return (
    <FlowAnswerNote answer={answer} what="发布">
      {(body) => (
        <div className="text-xs text-idpxyz-textMuted flex flex-col gap-1">
          <p>
            发布答复：<span className="font-mono">{body.outcome}</span> —— {labelOf(publishOutcomeLabels, body.outcome)}
          </p>
          {body.publication ? <EmbeddedPublication publication={body.publication} /> : null}
        </div>
      )}
    </FlowAnswerNote>
  );
}

// 受控发布用例的整份答案嵌在发布答复里（同一个用例的答案两口不换形）；逐格中文取 api.ts 那张既有的表，
// 不另抄一份。对账门的两串只随受控批文那一半在场，主路径走不到，仍如实显示。
function EmbeddedPublication({ publication }: { publication: CommercialPublicationAnswerBody }) {
  return (
    <div className="ml-4 flex flex-col gap-1">
      <p>
        受控发布用例：<span className="font-mono">{publication.outcome}</span> ——{' '}
        {labelOf(publicationOutcomeLabels, publication.outcome)}
      </p>
      {publication.pendingCause ? <p>未决成因：{publication.pendingCause}</p> : null}
      {publication.cause ? <p>成因：{publication.cause}</p> : null}
      {publication.declaredDigest || publication.computedDigest ? (
        <p className="font-mono break-all">
          声明摘要 {publication.declaredDigest ?? '—'} · 算出摘要 {publication.computedDigest ?? '—'}
        </p>
      ) : null}
      {publication.declarations && publication.declarations.length > 0 ? (
        <p>
          声明落点：
          {publication.declarations.map((entry) => (
            <span key={entry.channel} className="font-mono ml-2">
              {entry.channel}={entry.outcome}
            </span>
          ))}
        </p>
      ) : null}
    </div>
  );
}

// 摘要整串显示不截断：它就是载体与册上那一格，操作者要拿它去对。
function DigestLine({ canonicalization, digest }: { canonicalization?: string; digest?: string }) {
  return (
    <p className="font-mono break-all">
      规范化 {canonicalization ?? '—'} · 内容摘要 {digest ?? '—'}
    </p>
  );
}

/**
 * 五格判别与本目录其余面板同款（ADR-0022：状态码只答「有没有形成答案」，业务判别在响应体）。未收录的
 * outcome 原样示出英文原名：服务端新增一格时宁可显示原名，也不把它归进既有中文说法。
 */
function FlowAnswerNote<Body>({
  answer,
  what,
  children,
}: {
  answer: ApiResult<Body>;
  what: string;
  children: (body: Body) => ReactNode;
}) {
  switch (answer.kind) {
    case 'outcome':
      return <>{children(answer.body)}</>;
    case 'unconfigured':
      return (
        <p className="text-xs text-idpxyz-textMuted">
          {what}：接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。{unconfiguredNote}
        </p>
      );
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          {what}调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          服务端未形成{what}答案（HTTP {answer.status}）：{problemNote(answer.code)}；可稍后重试。
        </p>
      );
    case 'transport':
      return <p className="text-xs text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>;
  }
}
