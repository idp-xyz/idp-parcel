import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, ConfirmDialog } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { declarationLandingLabels, publicationOutcomeLabels } from './api';
import { publicationSummary, type PublicationFact } from './publication-summary';
import { labelOf, problemNote } from './presentation';
import { fieldProblemText } from './field-problem-text';
import { ProblemText } from './PublicationFormFields';
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
  /** 表单是从目录里的某一版预填起头的（票 party-commercial-uiux/14）：例「信用政策 SYN-CREDIT-01 v1」；给了就在顶上提示。 */
  prefilledFrom?: string;
  /** 预填特有的警示（读面还原不了、留空会悄悄丢掉的内容），随提示一起显示，见 NewVersionStart.caution。 */
  prefilledCaution?: string;
}

export interface PublicationFormContext {
  problems: Record<string, string[]>;
  locked: boolean;
}

// 这几口等的是 ADR-0100 那一族**操作者身份**的接入渠道（ADR-0126 Decision 五：Intake 到位那天把操作者信封译成
// 录入者 / 批准者），不是 `PAR-INT-01` 的客户生产委托渠道——两者是两族，参数登记册今天也没有操作者渠道那一行，
// 所以这里说「机制半边待接线」而不指某个登记册行。
const unconfiguredNote = '改内容或重试都不会改变结果，要先由管理员完成操作者接入配置。';

export function PublicationDraftFlow({
  kind,
  title,
  assemblePayload,
  localProblems = {},
  fieldPaths,
  children,
  onPublished,
  prefilledFrom,
  prefilledCaution,
}: PublicationDraftFlowProps) {
  const [flow, setFlow] = useState<PublicationFlowState>(emptyFlow());
  const [busy, setBusy] = useState<FlowAction | null>(null);
  const [confirmingPublish, setConfirmingPublish] = useState(false);

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
  // 预览通过之后，表单此刻这份载荷就是之后存、批、发的那一份（改一格即作废后面各步），版本信息从它取（票 party-commercial-uiux/11）。
  const previewed = preview?.kind === 'outcome' && preview.body.outcome === 'PREVIEWED';
  const summary = previewed && action !== null ? publicationSummary(payload, currentDisplayTimeZone()) : null;
  const actionDisabled = busy !== null || (action === 'preview' && hasLocalProblems);

  return (
    <Card className="m-4">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* 预览与录入送同一份载荷、服务端在同一处算内容摘要，所以批准与发布的必是预览过的那一份；表单不算摘要、
            不收批准人、不替服务端判格，录入者与批准者由操作者身份给出（ADR-0126）。这些是给开发者的，不上屏。 */}
        <p className="text-xs text-idpxyz-textMuted">
          {'发布分五步：填表 → 预览 → 存为待批准 → 批准 → 发布。预览会检查每一格并算出内容摘要；存为待批准之后表单锁定，要改内容请点「重新开始」。谁能批准由租户的审批职责规则决定。'}
          <strong>发布后这一版不能修改</strong>，要改须用新的版本号再发布。
        </p>

        {prefilledFrom ? (
          <div className="flex flex-col gap-1 rounded border border-idpxyz-border px-3 py-2 text-xs text-idpxyz-text">
            <p>
              {`已按「${prefilledFrom}」预填。版本号和生效区间要自己填；读面上看不到的格也是空的。发布前请逐格核对，改动的格才是这一版新的内容。`}
            </p>
            {prefilledCaution ? <p className="font-medium text-idpxyz-danger">{prefilledCaution}</p> : null}
          </div>
        ) : null}

        <Stepper step={step} />

        {children({ problems, locked })}

        {unclaimed.length > 0 ? (
          <ul className="text-xs text-idpxyz-danger list-disc ml-4">
            {unclaimed.map((problem) => (
              <li key={`${problem.field}:${problem.problem}`}>
                <span className="font-mono">{problem.field}</span>：<ProblemText problem={fieldProblemText(problem.problem)} />
              </li>
            ))}
          </ul>
        ) : null}

        {summary ? <PublicationSummaryCard facts={summary} /> : null}

        <div className="flex items-center gap-3 flex-wrap">
          {action !== null ? (
            <Button
              variant="default"
              className={primaryActionClass}
              // 发布不能撤回、改内容要换版本号：按下之前再拦一道，复述要发的是哪一版。
              onClick={() => (action === 'publish' ? setConfirmingPublish(true) : run(action))}
              disabled={actionDisabled}
              title={
                action === 'preview' && hasLocalProblems
                  ? '有几格的格式不对，先改正再预览'
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
        {submission ? <DraftNote answer={submission} what="存为待批准" labels={submitOutcomeLabels} /> : null}
        {approval ? <DraftNote answer={approval} what="批准" labels={approveOutcomeLabels} /> : null}
        {publication ? <PublicationNote answer={publication} /> : null}
      </CardContent>
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={confirmingPublish}
        tone="warning"
        title="发布这一版？"
        message="发布后这一版不能修改；要改内容，须用新的版本号再发布。"
        extraContent={summary ? <FactList facts={summary} /> : undefined}
        confirmLabel="发布"
        cancelLabel="先不发布"
        onConfirm={() => {
          setConfirmingPublish(false);
          run('publish');
        }}
        onCancel={() => setConfirmingPublish(false)}
      />
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
            <OutcomeText labels={previewOutcomeLabels} outcome={body.outcome} />
          </p>
          {body.outcome === 'PREVIEWED' ? <DigestLine canonicalization={body.canonicalization} digest={body.contentDigest} /> : null}
          {body.cause ? <p>原因：{body.cause}</p> : null}
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
            <OutcomeText labels={labels} outcome={body.outcome} />
          </p>
          {body.status ? (
            <p>
              发布稿状态：<span className="font-mono">{body.status}</span> —— {labelOf(draftStatusLabels, body.status)}
            </p>
          ) : null}
          {body.contentDigest ? <DigestLine canonicalization={body.canonicalization} digest={body.contentDigest} /> : null}
          {body.cause ? <p>原因：{body.cause}</p> : null}
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
        <OutcomeText labels={publicationOutcomeLabels} outcome={publication.outcome} />
      </p>
      {publication.pendingCause ? <p>未完成的原因：{publication.pendingCause}</p> : null}
      {publication.cause ? <p>原因：{publication.cause}</p> : null}
      {publication.declaredDigest || publication.computedDigest ? (
        <p className="font-mono break-all">
          提交的内容摘要 {publication.declaredDigest ?? '—'} · 服务端算出的内容摘要 {publication.computedDigest ?? '—'}
        </p>
      ) : null}
      {publication.declarations && publication.declarations.length > 0 ? (
        <p>
          随版本登记的正文：
          {publication.declarations.map((entry) => (
            <span key={entry.channel} className="font-mono ml-2">
              {entry.channel}：{labelOf(declarationLandingLabels, entry.outcome)}
            </span>
          ))}
        </p>
      ) : null}
    </div>
  );
}

/**
 * 答复一行：先中文结论，原码作次要信息跟在后面，给要对日志的人。未收录的原码照样显出原名——服务端新增一格时
 * 宁可显示原名，也不把它归进既有中文说法——那时不再重复显一遍。
 */
function OutcomeText({ labels, outcome }: { labels: Record<string, string>; outcome: string }) {
  const label = labelOf(labels, outcome);
  return (
    <>
      {label}
      {label !== outcome ? <span className="ml-1 font-mono text-idpxyz-breadcrumb">{outcome}</span> : null}
    </>
  );
}

// 摘要整串显示不截断：它就是载体与册上那一格，操作者要拿它去对。
function FactList({ facts }: { facts: readonly PublicationFact[] }) {
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-[13px]">
      {facts.map((fact) => (
        <div key={fact.label} className="contents">
          <dt className="text-idpxyz-textMuted">{fact.label}</dt>
          <dd className="font-mono break-all text-idpxyz-text">{fact.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** 预览通过后摆在按钮上方的「将要发布」卡：只复述版本信息，正文各格就在上面的表单里。 */
function PublicationSummaryCard({ facts }: { facts: readonly PublicationFact[] }) {
  return (
    <section className="rounded border border-idpxyz-border p-3 flex flex-col gap-2">
      <h3 className="text-[12px] font-medium text-idpxyz-text">将要发布</h3>
      <FactList facts={facts} />
    </section>
  );
}

function DigestLine({ canonicalization, digest }: { canonicalization?: string; digest?: string }) {
  return (
    <p className="font-mono break-all">
      内容摘要 {digest ?? '—'}（算法 {canonicalization ?? '—'}）
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
          {what}：暂时不能进行，接入渠道还没有配置。{unconfiguredNote}
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what={what} />;
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          {what}调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
          {answer.detail ? ` ${answer.detail}` : ''}
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
