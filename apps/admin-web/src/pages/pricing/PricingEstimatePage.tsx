import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails, httpRows } from '../../components/TechnicalDetails';
import { lengthUnitLabels, weightUnitLabels } from '../../vocabulary/price-card';
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
import { ADDRESS_NATURE_CODES, emptyEstimateDraft, estimateLocalProblems, estimatePayloadOf, type EstimateDraft } from './estimate-form';
import {
  addressNatureLabels,
  chargeLineName,
  directionLabels,
  estimateAnswerLabels,
  estimateMissingLabels,
  estimateOutcomeLabels,
  estimateReasonLabels,
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
          客户问价、核对价卡时用：填一件假设的包裹，看适用的每张价卡各收多少钱。试算只是看看，不报价、不记账、不留记录；几张价卡并排列出，不分先后。
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>包裹信息</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <Field label="适用范围 *" hint="按范围、方向与时间找出当时有效的价卡">
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
            <Field label="价格方向 *" hint="售价是向客户收的，成本是供应商向我们收的">
              <select className={selectClass} value={draft.direction} onChange={(event) => patch({ direction: event.target.value })}>
                <option value="">请选择</option>
                {Object.entries(directionLabels).map(([code, word]) => (
                  <option key={code} value={code}>
                    {word}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="按哪个时间计价 *" hint={`按当地时间（${timeZone}）填`} problem={problems.basisAt}>
              <Input type="datetime-local" value={draft.basisAt} onChange={(event) => patch({ basisAt: event.target.value })} />
            </Field>

            <Field label="实重 *">
              <div className="flex gap-2">
                <Input value={draft.weightValue} placeholder="如 1.2" onChange={(event) => patch({ weightValue: event.target.value })} />
                <select className={selectClass} value={draft.weightUnit} onChange={(event) => patch({ weightUnit: event.target.value })}>
                  <option value="">单位</option>
                  {WEIGHT_UNITS.map((unit) => (
                    <option key={unit} value={unit}>
                      {labelOf(weightUnitLabels, unit)}
                    </option>
                  ))}
                </select>
              </div>
            </Field>
            <Field label="尺寸（可不填）" hint="长、宽、高与单位一起填" problem={problems.dimensions}>
              <div className="flex gap-2">
                <Input value={draft.length} placeholder="长" onChange={(event) => patch({ length: event.target.value })} />
                <Input value={draft.width} placeholder="宽" onChange={(event) => patch({ width: event.target.value })} />
                <Input value={draft.height} placeholder="高" onChange={(event) => patch({ height: event.target.value })} />
                <select className={selectClass} value={draft.lengthUnit} onChange={(event) => patch({ lengthUnit: event.target.value })}>
                  <option value="">单位</option>
                  {LENGTH_UNITS.map((unit) => (
                    <option key={unit} value={unit}>
                      {labelOf(lengthUnitLabels, unit)}
                    </option>
                  ))}
                </select>
              </div>
            </Field>
            <Field label="结算币种（可不填）" hint="价卡要换算币种而这里没填时，结果会标出缺这一项">
              <Input
                className="font-mono"
                value={draft.settlementCurrency}
                placeholder="如 CNY"
                onChange={(event) => patch({ settlementCurrency: event.target.value })}
              />
            </Field>

            <Field label="分区（可不填）" hint="价卡不按邮编查分区时用它">
              <Input className="font-mono" value={draft.zone} placeholder="如 Z1" onChange={(event) => patch({ zone: event.target.value })} />
            </Field>
            <Field label="收件地址性质（可不选）" hint="有的价卡对商业 / 住宅地址收费不同；不选时，这类价卡会标出缺这一项">
              <select className={selectClass} value={draft.addressNature} onChange={(event) => patch({ addressNature: event.target.value })}>
                <option value="">不选</option>
                {ADDRESS_NATURE_CODES.map((code) => (
                  <option key={code} value={code}>
                    {labelOf(addressNatureLabels, code)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="邮编（可不填）" hint="价卡按邮编查分区或偏远地区时用它" problem={problems.postalRoute}>
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
            <Button
              variant="default"
              className={primaryActionClass}
              onClick={submit}
              disabled={submitting || Object.keys(problems).length > 0}
            >
              {submitting ? '试算中…' : '试算'}
            </Button>
            <span className="text-[12px] text-idpxyz-textMuted">必填项空着也能试算：结果里会逐项说明缺什么。</span>
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
        // 渠道未配置是诚实答案（ADR-0055）：不是「没有价格」，改声明或重试都不会变。
        <p className="text-[12px] text-idpxyz-textMuted">
          访问通道尚未配置（403），这次没有试算；这不代表没有价格。改包裹信息或重试都不会变，请管理员先配置访问通道。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="试算" className="text-[12px] text-idpxyz-textMuted" />;
    case 'callerProblem':
      return (
        <div className="text-[12px] text-idpxyz-danger">
          <p>
            没能试算：{problemNote(answer.code)}
            {answer.detail ? ` ${answer.detail}` : ''}
          </p>
          <TechnicalDetails rows={httpRows(answer.status, answer.code)} className="mt-1 text-idpxyz-text" />
        </div>
      );
    case 'noAnswer':
      return (
        <div className="text-[12px] text-idpxyz-danger">
          <p>这次试算没算完：{problemNote(answer.code)}</p>
          <TechnicalDetails rows={httpRows(answer.status, answer.code)} className="mt-1 text-idpxyz-text" />
        </div>
      );
    case 'transport':
      return <p className="text-[12px] text-idpxyz-danger">连不上系统，没能试算（{answer.message}）；检查网络后重试。</p>;
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
            <li key={`${reference.id}@${reference.version}`}>
              同时有效的版本：<span className="font-mono">{reference.id}</span> 版本{' '}
              <span className="font-mono">{reference.version}</span>
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
          <span className="font-mono">{candidate.plan.id}</span>
          <span className="ml-1 text-[12px] font-normal text-idpxyz-textMuted">版本 {candidate.plan.version}</span>
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
      <p className="flex items-center gap-2">
        <StatusPill code={evaluation.status} scope="pricing-evaluation" />
        <span className="text-idpxyz-textMuted">
          {labelOf(directionLabels, evaluation.direction)} · {labelOf(purposeLabels, evaluation.purpose)}
        </span>
      </p>
      <p className="text-[15px] font-medium">
        {evaluation.total ? (
          <span className="font-mono">
            {evaluation.total.amount} {evaluation.total.currency}
          </span>
        ) : (
          <span className="text-[12px] font-normal text-idpxyz-textMuted">没有合计：这一份没算完，不显示金额，也不会用 0 代替</span>
        )}
      </p>
      {evaluation.chargeLines.length > 0 ? (
        <table className="w-full text-left">
          <thead className="text-idpxyz-textMuted">
            <tr>
              <th className="py-1 font-normal">费用</th>
              <th className="py-1 text-right font-normal">金额</th>
            </tr>
          </thead>
          <tbody>
            {evaluation.chargeLines.map((line, index) => (
              <tr key={`${line.code}-${index}`} className="border-t border-idpxyz-border align-top">
                {/* 费用代码不上屏，悬停可见；全部代码另在下方技术详情里。 */}
                <td className="py-1" title={line.code}>
                  {chargeLineName(line)}
                </td>
                <td className="py-1 text-right font-mono">
                  {line.amount.amount} {line.amount.currency}
                </td>
              </tr>
            ))}
          </tbody>
          {evaluation.total ? (
            <tfoot>
              <tr className="border-t border-idpxyz-border font-semibold">
                <td className="py-1">合计</td>
                <td className="py-1 text-right font-mono">
                  {evaluation.total.amount} {evaluation.total.currency}
                </td>
              </tr>
            </tfoot>
          ) : null}
        </table>
      ) : null}
      {evaluation.issues.length > 0 ? (
        <ul className="text-idpxyz-textMuted">
          {evaluation.issues.map((issue) => (
            <li key={issue.code} title={issue.code}>
              {issue.message}
            </li>
          ))}
        </ul>
      ) : null}
      {evaluation.explanation.length > 0 ? (
        <details>
          <summary className="cursor-pointer text-idpxyz-textMuted">怎么算的</summary>
          <ul className="mt-1 list-disc pl-4 text-idpxyz-textMuted">
            {evaluation.explanation.map((line, index) => (
              <li key={index}>{line}</li>
            ))}
          </ul>
        </details>
      ) : null}
      {/* 用到的各版本清单、计费编号、状态码、费用代码与证据层级只在对账与排障时用，收进技术详情。 */}
      <TechnicalDetails
        rows={[
          { label: '计费编号', value: evaluation.evaluationId },
          { label: '结果码', value: evaluation.status },
          { label: '证据层级', value: `${evaluation.evidence}（${labelOf(evidenceLevelLabels, evaluation.evidence)}）` },
          ...evaluation.chargeLines.map((line) => ({ label: chargeLineName(line), value: line.code })),
          ...evaluation.manifest.map((entry) => ({ label: entry.kind, value: `${entry.id}@${entry.version}` })),
        ]}
      />
    </div>
  );
}
