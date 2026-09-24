import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import {
  formEstimate,
  listPriceCards,
  type EstimateCandidateRecord,
  type EstimateEvaluationRecord,
  type EstimateResponseBody,
} from './api';
import { emptyEstimateDraft, estimateLocalProblems, estimatePayloadOf, type EstimateDraft } from './estimate-form';
import {
  directionLabels,
  estimateAnswerLabels,
  estimateMissingLabels,
  estimateOutcomeLabels,
  estimateReasonLabels,
  evaluationStatusLabels,
  evidenceLevelLabels,
  labelOf,
  problemNote,
  purposeLabels,
} from './presentation';

// 运营试算页（票 operator-workspace-gaps/06；UC-PP-001；ADR-0152）。判读在 estimate-form.ts 与传输层，这里只摆。
//
// 结果区**逐卡并列、不排序、不标首选**：多张适用卡各自的评价之间没有优劣，择优归网络与路由（CONTEXT-MAP）。按金额排一下看似
// 顺手，就是替路由作了它的决定。试算不形成承诺、不进结算、不入册——页面上没有「保存」或「生成报价」。

const info = moduleInfoById['pricing-estimate'];

const WEIGHT_UNITS = ['G', 'KG', 'OZ', 'LB'] as const;
const LENGTH_UNITS = ['CM', 'IN'] as const;

const selectClass =
  'rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[13px] text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

function Field({ label, hint, problem, children }: { label: string; hint?: string; problem?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1 text-[12px] text-idpxyz-textMuted">
      <span>{label}</span>
      {children}
      {problem ? <span className="text-idpxyz-danger">{problem}</span> : hint ? <span>{hint}</span> : null}
    </label>
  );
}

export function PricingEstimatePage() {
  const [draft, setDraft] = useState<EstimateDraft>(emptyEstimateDraft);
  const [answer, setAnswer] = useState<ApiResult<EstimateResponseBody> | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [scopes, setScopes] = useState<string[]>([]);
  const timeZone = currentDisplayTimeZone();
  const problems = estimateLocalProblems(draft, timeZone);
  const patch = (next: Partial<EstimateDraft>) => {
    setDraft((current) => ({ ...current, ...next }));
    // 改了任何一格，上一次的结果就不再是眼前这份声明的答案。
    setAnswer(null);
  };

  // 主要范围的候选只作输入提示（datalist），不预选、不当默认；价卡目录读不到时照样能手填。
  useEffect(() => {
    let cancelled = false;
    void listPriceCards().then((result) => {
      if (cancelled || result.kind !== 'outcome') return;
      setScopes([...new Set(result.body.cards.map((card) => card.scope))].sort());
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const submit = () => {
    setSubmitting(true);
    void formEstimate(estimatePayloadOf(draft, timeZone)).then((result) => {
      setAnswer(result);
      setSubmitting(false);
    });
  };

  return (
    <div className="flex-1 overflow-auto bg-idpxyz-editor p-4">
      <div className="mb-3">
        <h2 className="text-[15px] font-medium text-idpxyz-text">{info.title}</h2>
        <p className="mt-0.5 text-[12px] text-idpxyz-textMuted">
          按一件假设包裹，看此刻在用的每一张适用价卡各怎么计价。试算不形成承诺、不进结算、不入册；多张卡并列，不排序、不标首选。
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>试算声明</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <Field label="主要范围 *" hint="价卡按范围、方向与时点取适用版本">
              <Input
                list="pricing-estimate-scopes"
                className="font-mono"
                value={draft.scope}
                placeholder="如 SYN-SCOPE-01"
                onChange={(event) => patch({ scope: event.target.value })}
              />
              <datalist id="pricing-estimate-scopes">
                {scopes.map((scope) => (
                  <option key={scope} value={scope} />
                ))}
              </datalist>
            </Field>
            <Field label="价格方向 *" hint="计算目的由领域按方向成对取得">
              <select className={selectClass} value={draft.direction} onChange={(event) => patch({ direction: event.target.value })}>
                <option value="">选择方向</option>
                {Object.entries(directionLabels).map(([code, word]) => (
                  <option key={code} value={code}>
                    {word}（{code}）
                  </option>
                ))}
              </select>
            </Field>
            <Field label="计价基准时点 *" hint={`本地墙钟（${timeZone}）`} problem={problems.basisAt}>
              <Input type="datetime-local" value={draft.basisAt} onChange={(event) => patch({ basisAt: event.target.value })} />
            </Field>

            <Field label="实重 *">
              <div className="flex gap-2">
                <Input value={draft.weightValue} placeholder="如 1.2" onChange={(event) => patch({ weightValue: event.target.value })} />
                <select className={selectClass} value={draft.weightUnit} onChange={(event) => patch({ weightUnit: event.target.value })}>
                  <option value="">单位</option>
                  {WEIGHT_UNITS.map((unit) => (
                    <option key={unit} value={unit}>
                      {unit}
                    </option>
                  ))}
                </select>
              </div>
            </Field>
            <Field label="尺寸（可缺）" hint="长、宽、高与单位一起填" problem={problems.dimensions}>
              <div className="flex gap-2">
                <Input value={draft.length} placeholder="长" onChange={(event) => patch({ length: event.target.value })} />
                <Input value={draft.width} placeholder="宽" onChange={(event) => patch({ width: event.target.value })} />
                <Input value={draft.height} placeholder="高" onChange={(event) => patch({ height: event.target.value })} />
                <select className={selectClass} value={draft.lengthUnit} onChange={(event) => patch({ lengthUnit: event.target.value })}>
                  <option value="">单位</option>
                  {LENGTH_UNITS.map((unit) => (
                    <option key={unit} value={unit}>
                      {unit}
                    </option>
                  ))}
                </select>
              </div>
            </Field>
            <Field label="结算币种（可缺）" hint="卡需要而没给，评价如实落待判断">
              <Input
                className="font-mono"
                value={draft.settlementCurrency}
                placeholder="如 CNY"
                onChange={(event) => patch({ settlementCurrency: event.target.value })}
              />
            </Field>

            <Field label="分区（可缺）" hint="没绑分区目录的卡用它">
              <Input className="font-mono" value={draft.zone} placeholder="如 Z1" onChange={(event) => patch({ zone: event.target.value })} />
            </Field>
            <Field label="邮编路线（可缺）" hint="绑了分区或偏远档位目录的卡用它查目录" problem={problems.postalRoute}>
              <div className="flex gap-2">
                <Input className="font-mono" value={draft.origin} placeholder="始发邮编" onChange={(event) => patch({ origin: event.target.value })} />
                <Input
                  className="font-mono"
                  value={draft.destination}
                  placeholder="目的邮编"
                  onChange={(event) => patch({ destination: event.target.value })}
                />
              </div>
            </Field>
          </div>
          <div className="mt-4 flex items-center gap-3">
            <Button onClick={submit} disabled={submitting || Object.keys(problems).length > 0}>
              {submitting ? '试算中…' : '试算'}
            </Button>
            <span className="text-[12px] text-idpxyz-textMuted">必需格空着也能送：服务端逐格点名缺什么，页面不另拦。</span>
          </div>
        </CardContent>
      </Card>

      {answer ? (
        <section className="mt-4">
          <EstimateAnswer answer={answer} />
        </section>
      ) : null}
    </div>
  );
}

function EstimateAnswer({ answer }: { answer: ApiResult<EstimateResponseBody> }) {
  switch (answer.kind) {
    case 'outcome':
      return <EstimateBody body={answer.body} />;
    case 'unconfigured':
      return (
        <p className="text-[12px] text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）：试算端点（POST /pricing-estimates）已建立并装配，操作者身份的接入渠道尚未接上，
          服务端如实拒绝。这不是「没有价格」，改声明或重试都不会改变结果；操作者渠道接上后由装配侧换真 Intake 即放行。
        </p>
      );
    case 'callerProblem':
      return (
        <p className="text-[12px] text-idpxyz-danger">
          声明不成形（HTTP {answer.status}）：{problemNote(answer.code)}
          {answer.detail ? ` ${answer.detail}` : ''}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-[12px] text-idpxyz-danger">
          服务端未形成试算答案（HTTP {answer.status}）：{problemNote(answer.code)}；可稍后重试。
        </p>
      );
    case 'transport':
      return <p className="text-[12px] text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>;
  }
}

function EstimateBody({ body }: { body: EstimateResponseBody }) {
  return (
    <div className="flex flex-col gap-3">
      <p className="text-[13px] text-idpxyz-text">
        {labelOf(estimateOutcomeLabels, body.outcome)}
        {body.reason ? <span className="ml-2 text-idpxyz-textMuted">（{labelOf(estimateReasonLabels, body.reason)}）</span> : null}
      </p>
      {body.conflict.length > 0 ? (
        <ul className="text-[12px] text-idpxyz-textMuted">
          {body.conflict.map((reference) => (
            <li key={`${reference.id}@${reference.version}`} className="font-mono">
              候选 {reference.id}@{reference.version}
            </li>
          ))}
        </ul>
      ) : null}
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {body.candidates.map((candidate) => (
          <CandidateCard key={`${candidate.plan.id}@${candidate.plan.version}`} candidate={candidate} />
        ))}
      </div>
    </div>
  );
}

function CandidateCard({ candidate }: { candidate: EstimateCandidateRecord }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <span className="font-mono">
            {candidate.plan.id}@{candidate.plan.version}
          </span>
          <span className="ml-2 text-[12px] font-normal text-idpxyz-textMuted">{labelOf(estimateAnswerLabels, candidate.answer)}</span>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {candidate.evaluation ? (
          <EvaluationDetail evaluation={candidate.evaluation} />
        ) : (
          <ul className="text-[12px] text-idpxyz-textMuted">
            {(candidate.missing ?? []).map((code) => (
              <li key={code}>缺 {labelOf(estimateMissingLabels, code)}</li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function EvaluationDetail({ evaluation }: { evaluation: EstimateEvaluationRecord }) {
  return (
    <div className="flex flex-col gap-2 text-[12px] text-idpxyz-text">
      <p>
        评价状态：{labelOf(evaluationStatusLabels, evaluation.status)}
        <span className="ml-2 text-idpxyz-textMuted">
          {labelOf(directionLabels, evaluation.direction)} · {labelOf(purposeLabels, evaluation.purpose)} · 证据层级{' '}
          {evaluation.evidence}（{labelOf(evidenceLevelLabels, evaluation.evidence)}）
        </span>
      </p>
      <p className="text-[15px] font-medium">
        {evaluation.total ? (
          <span className="font-mono">
            {evaluation.total.amount} {evaluation.total.currency}
          </span>
        ) : (
          <span className="text-[12px] font-normal text-idpxyz-textMuted">没有合计（评价未完成；不以零金额顶替）</span>
        )}
      </p>
      {evaluation.chargeLines.length > 0 ? (
        <table className="w-full text-left">
          <thead className="text-idpxyz-textMuted">
            <tr>
              <th className="py-1 font-normal">费用代码</th>
              <th className="py-1 font-normal">说明</th>
              <th className="py-1 text-right font-normal">金额</th>
            </tr>
          </thead>
          <tbody>
            {evaluation.chargeLines.map((line, index) => (
              <tr key={`${line.code}-${index}`} className="border-t border-idpxyz-border">
                <td className="py-1 font-mono">{line.code}</td>
                <td className="py-1">{line.description}</td>
                <td className="py-1 text-right font-mono">
                  {line.amount.amount} {line.amount.currency}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      {evaluation.issues.length > 0 ? (
        <ul className="text-idpxyz-textMuted">
          {evaluation.issues.map((issue) => (
            <li key={issue.code}>
              <span className="font-mono">{issue.code}</span>：{issue.message}
            </li>
          ))}
        </ul>
      ) : null}
      <details>
        <summary className="cursor-pointer text-idpxyz-textMuted">解释与版本清单</summary>
        <ul className="mt-1 list-disc pl-4 text-idpxyz-textMuted">
          {evaluation.explanation.map((line, index) => (
            <li key={index}>{line}</li>
          ))}
        </ul>
        <ul className="mt-1 font-mono text-idpxyz-textMuted">
          {evaluation.manifest.map((entry) => (
            <li key={`${entry.kind}:${entry.id}@${entry.version}`}>
              {entry.kind} · {entry.id}@{entry.version}
            </li>
          ))}
        </ul>
        <p className="mt-1 font-mono text-idpxyz-textMuted">评价标识 {evaluation.evaluationId}</p>
      </details>
    </div>
  );
}
