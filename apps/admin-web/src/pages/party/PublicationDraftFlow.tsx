import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, ConfirmDialog } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { publicationBodySummary } from './publication-body-summary';
import { publicationSummary, type PublicationFact } from './publication-summary';
import { DraftNote, FactList, PreviewNote, PublicationNote } from './PublicationAnswerNotes';
import { fieldProblemText } from './field-problem-text';
import { ProblemText } from './PublicationFormFields';
import { publicationGroup, useUnsavedGuard } from './unsaved-guard';
import {
  approveOutcomeLabels,
  approvePublicationDraft,
  previewCommercialPublication,
  publishPublicationDraft,
  submitOutcomeLabels,
  submitPublicationDraft,
  type CommercialObjectKindName,
  type CommercialPublicationPayload,
} from './publication-draft-api';
import {
  changedDecisionOf,
  currentAnswer,
  decisionPanelOf,
  decisionDigestOf,
  draftReferenceOf,
  emptyFlow,
  flowActionLabels,
  flowStepLabels,
  flowSteps,
  isFormLocked,
  publicationDraftUnsaved,
  nextAction,
  payloadKey,
  pendingPublicationsHash,
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
  /** 已有版本提示，原样经 `children` 的 form 上下文交给字段子组件；不给就是 null。 */
  versionNote?: ReactNode;
}

export interface PublicationFormContext {
  problems: Record<string, string[]>;
  locked: boolean;
  /** 「这个对象已有哪些版本」的提示（票 party-commercial-uiux/15），由表单自己组、经这里交给它的字段子组件摆在版本信息一节里。 */
  versionNote: ReactNode;
}

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
  versionNote = null,
}: PublicationDraftFlowProps) {
  const [flow, setFlow] = useState<PublicationFlowState>(emptyFlow());
  const [busy, setBusy] = useState<FlowAction | null>(null);
  const [confirmingPublish, setConfirmingPublish] = useState(false);

  const payload = assemblePayload();
  const key = payloadKey(payload);
  const step = reachedStep(flow, key);
  const action = nextAction(step);
  const locked = isFormLocked(step);
  // 表单起头那一份（空表单或预填）的键：人动过它、又还没发布，离开前要问（票 party-commercial-uiux/17）。
  const initialKey = useRef(key);
  useUnsavedGuard(publicationGroup(kind), publicationDraftUnsaved(initialKey.current, key, step));

  const preview = currentAnswer(flow.preview, key);
  const submission = currentAnswer(flow.submission, key);
  const approval = currentAnswer(flow.approval, key);
  const publication = currentAnswer(flow.publication, key);
  // 批准与发布绑定存为待批准时服务端答的摘要；拿不到时两个按钮都收回，不拿别的摘要顶。
  const digest = decisionDigestOf(flow, key);

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
      // 批准与发布要带存下那份载体的摘要；没有就不发（按钮此时本来就是收回的）。
      case 'approve':
        if (digest === null) return setBusy(null);
        return settle(approvePublicationDraft(draftReferenceOf(payload, digest)), withApproval);
      case 'publish':
        if (digest === null) return setBusy(null);
        return settle(publishPublicationDraft(draftReferenceOf(payload, digest)), withPublication);
    }
  };

  const started = flow.preview !== null;
  // 预览通过之后，表单此刻这份载荷就是之后存、批、发的那一份（改一格即作废后面各步），版本信息从它取（票 party-commercial-uiux/11）。
  const previewed = preview?.kind === 'outcome' && preview.body.outcome === 'PREVIEWED';
  const summary = previewed && action !== null ? publicationSummary(payload, currentDisplayTimeZone()) : null;
  const bodySummary = previewed && action !== null ? publicationBodySummary(payload, currentDisplayTimeZone()) : [];
  // 录入区只管预览与存为待批准；批准与发布各有自己的按钮区（票 party-commercial-uiux/19）。
  const recordAction = action === 'preview' || action === 'submit' ? action : null;
  const actionDisabled = busy !== null || (recordAction === 'preview' && hasLocalProblems);
  // 批准或发布答`已变更`：旧摘要再送也是同一个答案，按钮收回，下面另摆续办。
  const changed = changedDecisionOf(flow, key);
  const decision = decisionPanelOf(step, changed);

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

        {children({ problems, locked, versionNote })}

        {unclaimed.length > 0 ? (
          <ul className="text-xs text-idpxyz-danger list-disc ml-4">
            {unclaimed.map((problem) => (
              <li key={`${problem.field}:${problem.problem}`}>
                <span className="font-mono">{problem.field}</span>：<ProblemText problem={fieldProblemText(problem.problem)} />
              </li>
            ))}
          </ul>
        ) : null}

        {summary ? <PublicationSummaryCard facts={summary} bodyFacts={bodySummary} /> : null}

        <div className="flex items-center gap-3 flex-wrap">
          {recordAction !== null ? (
            <Button
              variant="default"
              className={primaryActionClass}
              onClick={() => run(recordAction)}
              disabled={actionDisabled}
              title={
                recordAction === 'preview' && hasLocalProblems
                  ? '有几格的格式不对，先改正再预览'
                  : undefined
              }
            >
              {busy === recordAction ? `${flowActionLabels[recordAction]}中…` : flowActionLabels[recordAction]}
            </Button>
          ) : null}
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
        {decision.visible ? (
          <section className="rounded border border-idpxyz-border p-3 flex flex-col gap-2">
            <h3 className="text-[12px] font-medium text-idpxyz-text">批准与发布</h3>
            <p className="text-xs text-idpxyz-textMuted">
              {decision.status}
              {decision.approve === 'ready'
                ? '批准由租户的审批职责规则决定谁能做，批准之后才能发布；发布后这一版不能修改。'
                : ''}
            </p>
            <div className="flex items-center gap-3 flex-wrap">
              <Button
                variant={decision.approve === 'ready' ? 'default' : 'outline'}
                className={decision.approve === 'ready' ? primaryActionClass : undefined}
                disabled={decision.approve !== 'ready' || busy !== null || digest === null}
                onClick={() => run('approve')}
              >
                {busy === 'approve' ? '批准中…' : decision.approve === 'done' ? '已批准' : '批准'}
              </Button>
              <Button
                variant={decision.publish === 'ready' ? 'default' : 'outline'}
                className={decision.publish === 'ready' ? primaryActionClass : undefined}
                disabled={decision.publish !== 'ready' || busy !== null || digest === null}
                title={decision.publish === 'waiting' ? '先完成批准' : undefined}
                // 发布不能撤回、改内容要换版本号：按下之前再拦一道，复述要发的是哪一版。
                onClick={() => setConfirmingPublish(true)}
              >
                {busy === 'publish' ? '发布中…' : decision.publish === 'done' ? '已发布' : '发布'}
              </Button>
            </div>
            {changed !== null ? (
              <>
                <p className="text-xs text-idpxyz-textMuted">
                  {'要采用别人改后的内容，去「待批准发布」页读回现在的内容后在那里批；要以表单上这份为准，重新存为待批准——' +
                    '册上那份会改回眼前这份（待批准期间修订），别人的改动随之覆盖，然后再批。'}
                </p>
                <div className="flex items-center gap-3 flex-wrap">
                  <Button
                    variant="outline"
                    disabled={busy !== null}
                    onClick={() => {
                      window.location.hash = pendingPublicationsHash(payload.objectId);
                    }}
                  >
                    去「待批准发布」页核对
                  </Button>
                  <Button variant="outline" disabled={busy !== null} onClick={() => run('submit')}>
                    {busy === 'submit' ? '存为待批准中…' : '用表单上这份重新存为待批准'}
                  </Button>
                </div>
              </>
            ) : null}
          </section>
        ) : null}
        {approval ? <DraftNote answer={approval} what="批准" labels={approveOutcomeLabels} /> : null}
        {publication ? <PublicationNote answer={publication} /> : null}
      </CardContent>
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={confirmingPublish}
        tone="warning"
        title="发布这一版？"
        message="发布后这一版不能修改；要改内容，须用新的版本号再发布。"
        extraContent={
          summary ? (
            <div className="flex max-h-[40vh] flex-col gap-3 overflow-auto">
              <FactList facts={summary} />
              {bodySummary.length > 0 ? <FactList facts={bodySummary} /> : null}
            </div>
          ) : undefined
        }
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

/** 预览通过后摆在按钮上方的「将要发布」卡：版本信息，加这一册正文的要点（只是复述，格子仍以上面的表单为准）。 */
function PublicationSummaryCard({
  facts,
  bodyFacts,
}: {
  facts: readonly PublicationFact[];
  bodyFacts: readonly PublicationFact[];
}) {
  return (
    <section className="rounded border border-idpxyz-border p-3 flex flex-col gap-2">
      <h3 className="text-[12px] font-medium text-idpxyz-text">将要发布</h3>
      <FactList facts={facts} />
      {bodyFacts.length > 0 ? (
        <>
          <h4 className="mt-1 text-[12px] font-medium text-idpxyz-text">正文要点</h4>
          <FactList facts={bodyFacts} />
        </>
      ) : null}
    </section>
  );
}
