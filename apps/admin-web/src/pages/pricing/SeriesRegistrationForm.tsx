import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input, Textarea } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { accessRefusalHeadline, isOperatorRefusal } from '../../auth/access-refusal';
import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import { OperatorRefusalNote } from '../../components/states';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import {
  listQuoteBasisCandidates,
  previewReferenceSeries,
  registerReferenceSeriesPayload,
  registrationOutcomeLabels,
  type QuoteBasisCandidate,
  type QuoteBasisCandidateListResponseBody,
  type SeriesPeriodChangeRecord,
  type SeriesPreviewResponseBody,
} from './api';
import {
  comparisonOutcomeLabels,
  directionLabels,
  evidenceGradeLabels,
  labelOf,
  periodChangeKindLabels,
  previewOutcomeLabels,
  problemNote,
  seriesKindLabels,
} from './presentation';
import {
  draftProblems,
  emptyPeriodDraft,
  payloadKey,
  payloadOf,
  type PeriodDraft,
  type SeriesDraft,
  type SeriesKindDraft,
} from './series-form';
import { formatInstant } from '../moment';

/**
 * 参考序列的逐字段登记表单，带提交前预览（票 pricing-reference-series-operations/08；
 * ADR-0101 决定一、四、八）。
 *
 * **为什么是逐字段表单加一个无持久化的预览，而不是价卡那套草稿册**：本册按决定八自裁，
 * 判据是决定一（登记频次 × 操作者角色 × 载荷结构）的直接读数——低频、操作者是运营配置员、载荷是几到几十期的一维期次表；
 * 它又不像复核那样两格即止（有期次表、证据等级、内容摘要、更正链），盲提 JSON 不行，要一个
 * 先过领域构造门再回摘要的步骤。登记之后的那一道治理是复核（独立动作、已有自己的口，须否换人
 * 由租户的参考复核职责规则说），登记前再立一道批准就是两道门，所以预览**不写库、不进版本清单**。
 *
 * **登记按钮只在「预览过的就是眼前这一份」时可用。** 预览之后再改一格，预览作废、按钮收回——
 * 判据是载荷的稳定键（`payloadKey`），不是摘要：摘要只有服务端算，前端拿不到也不该算。预览
 * 与登记送**同一份**载荷、走服务端同一段解码，预览页上的内容摘要与登记册记下的逐字节相等
 * （决定四）；不然就会长出「预览通过、登记却答内容冲突」这一格，而它与真实的内容冲突不可分辨。
 *
 * **表单上没有的三样，不是漏做**：登记责任方与租户由接入渠道的操作者信封给（ADR-0100），
 * 自报即失效；证据等级与内容摘要由服务端答，前端不裁不算（票 04 红线）；版本引用只带三元、
 * 指纹可选（ADR-0108）——自身引用与口径不带指纹，更正回指带目录透出的前版 `contentDigest`
 * 作指纹，服务端不铸任何令牌（见传输层 `reference_series_payload.go` 文件头）。
 *
 * **更正模式锁定序列标识与种类**：领域拒绝换序列身份的更正，对照也只在同种类间比得出来；
 * 其余各格可改，更正本来就是为了改。页面上没有「编辑」——更正是新版本，原版本一字不动。
 */
export interface SeriesRegistrationFormProps {
  /** 初始草稿：新登记给 `emptySeriesDraft()`，更正给 `correctionDraftOf(row)`。 */
  initialDraft: SeriesDraft;
  /** 登记册答 `RECORDED` 后回调（目录页借它刷新）。 */
  onRecorded?: () => void;
  /** 有则显示「收起」。行动作面板用；登记签不用。 */
  onClose?: () => void;
}

type PreviewState =
  | { kind: 'idle' }
  | { kind: 'previewing' }
  | { kind: 'previewed'; key: string; answer: ApiResult<SeriesPreviewResponseBody> };

type RegistrationState =
  | { kind: 'idle' }
  | { kind: 'submitting' }
  | { kind: 'answered'; answer: ApiResult<RegistrationResponseBody> };

const fieldLabel = 'block text-[12px] text-idpxyz-textMuted mb-1';
const selectClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[13px] ' +
  'text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

export function SeriesRegistrationForm({ initialDraft, onRecorded, onClose }: SeriesRegistrationFormProps) {
  const [draft, setDraft] = useState<SeriesDraft>(initialDraft);
  const [preview, setPreview] = useState<PreviewState>({ kind: 'idle' });
  const [registration, setRegistration] = useState<RegistrationState>({ kind: 'idle' });

  const correcting = draft.correction !== null;
  const problems = draftProblems(draft);
  const payload = payloadOf(draft);
  const currentKey = payloadKey(payload);
  // 只有键相同的预览才算「预览过眼前这一份」；键不同的留着不显示，免得旧摘要冒充新的。
  const currentPreview = preview.kind === 'previewed' && preview.key === currentKey ? preview.answer : null;
  const previewAccepted =
    currentPreview !== null && currentPreview.kind === 'outcome' && currentPreview.body.outcome === 'PREVIEWED';
  const recorded =
    registration.kind === 'answered' &&
    registration.answer.kind === 'outcome' &&
    registration.answer.body.outcome === 'RECORDED';

  const patch = (change: Partial<SeriesDraft>) => {
    setDraft((current) => ({ ...current, ...change }));
    // 改动后上一次登记答复也不再对应眼前这一份：留着会让人以为改过的这份已入册。
    setRegistration({ kind: 'idle' });
  };
  const patchPeriod = (index: number, change: Partial<PeriodDraft>) =>
    patch({ periods: draft.periods.map((period, at) => (at === index ? { ...period, ...change } : period)) });

  const runPreview = () => {
    setPreview({ kind: 'previewing' });
    const key = currentKey;
    void previewReferenceSeries(payload).then((answer) => setPreview({ kind: 'previewed', key, answer }));
  };
  const runRegister = () => {
    setRegistration({ kind: 'submitting' });
    void registerReferenceSeriesPayload(payload).then((answer) => {
      setRegistration({ kind: 'answered', answer });
      if (answer.kind === 'outcome' && answer.body.outcome === 'RECORDED') onRecorded?.();
    });
  };

  return (
    <Card className="m-4">
      <CardHeader>
        <CardTitle>
          {correcting ? (
            <>
              更正 <span className="font-mono text-[13px]">{draft.seriesId}</span>
              <span className="text-[13px] text-idpxyz-textMuted"> 版本 {draft.correction?.priorVersion}</span>
              <span className="text-[13px] text-idpxyz-textMuted">（另登一个新版本，原版本保持不变）</span>
            </>
          ) : (
            '登记汇率或费率'
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* 登记人由接入渠道给，表单不收也不送；凭证等级、内容摘要都由后端算，页面只呈现（ADR-0101 决定四）。 */}
        <p className="text-xs text-idpxyz-textMuted">
          填好后先点「预览」核对：系统会检查内容、列出与对照版本的逐期差异，这一步不会保存。预览通过后再点「登记」；
          预览之后改了任何一格，要重新预览。登记后要有人复核通过，这一版才参与计价。
        </p>

        <div className="grid grid-cols-2 gap-3">
          <label className="block">
            <span className={fieldLabel}>名称 *</span>
            <Input
              value={draft.seriesId}
              readOnly={correcting}
              className="font-mono text-[13px]"
              placeholder="如 FX-CNY-SGD，以后每期都登在这个名称下"
              onChange={(event) => patch({ seriesId: event.target.value })}
            />
          </label>
          <label className="block">
            <span className={fieldLabel}>{correcting ? '新版本号 *' : '版本号 *'}</span>
            <Input
              value={draft.seriesVersion}
              className="font-mono text-[13px]"
              placeholder={correcting ? `换一个，不能再用 ${draft.correction?.priorVersion ?? ''}` : '如 v1'}
              onChange={(event) => patch({ seriesVersion: event.target.value })}
            />
          </label>
          <div>
            <span className={fieldLabel}>种类 *</span>
            <KindPicker
              value={draft.kind}
              locked={correcting}
              onChange={(kind) =>
                patch({
                  kind,
                  quoteBasis: kind === 'EXCHANGE_RATE' ? draft.quoteBasis : null,
                  currency: kind === 'PUBLISHED_AMOUNT' ? draft.currency : '',
                })
              }
            />
          </div>
          <label className="block">
            <span className={fieldLabel}>数据来源 *</span>
            <Input
              value={draft.sourceIdentifier}
              className="font-mono text-[13px]"
              placeholder="数值是谁公布的，如 cfets-daily"
              onChange={(event) => patch({ sourceIdentifier: event.target.value })}
            />
          </label>
        </div>

        {draft.kind === 'EXCHANGE_RATE' ? (
          <QuoteBasisField
            value={draft.quoteBasis}
            onChange={(quoteBasis) => patch({ quoteBasis })}
          />
        ) : null}

        {draft.kind === 'PUBLISHED_AMOUNT' ? (
          <label className="block max-w-xs">
            <span className={fieldLabel}>金额币种 *</span>
            <Input
              value={draft.currency}
              className="font-mono text-[13px]"
              placeholder="三位大写代码，如 CNY；与用到它的价格表一致"
              onChange={(event) => patch({ currency: event.target.value.toUpperCase() })}
            />
          </label>
        ) : null}

        <PeriodsEditor
          periods={draft.periods}
          onChange={patchPeriod}
          onAdd={() => patch({ periods: [...draft.periods, emptyPeriodDraft()] })}
          onRemove={(index) => patch({ periods: draft.periods.filter((_, at) => at !== index) })}
        />

        {correcting && draft.correction ? (
          <div className="flex flex-col gap-2">
            <p className="text-xs text-idpxyz-textMuted">
              这一版登记后会标明是更正 <span className="font-mono">{draft.seriesId}</span> 版本{' '}
              <span className="font-mono">{draft.correction.priorVersion}</span>。请写清为什么更正，复核的同事会看。
            </p>
            <Textarea
              value={draft.correction.basis}
              rows={3}
              className="text-xs"
              placeholder="更正理由（必填）：例如「第 2 期数值录错，按来源公布值改正」"
              onChange={(event) =>
                patch({ correction: draft.correction ? { ...draft.correction, basis: event.target.value } : null })
              }
            />
          </div>
        ) : null}

        <label className="block">
          <span className={fieldLabel}>和哪一版对照（可不填）</span>
          <Input
            value={draft.compareWithVersion}
            className="font-mono text-[13px] max-w-[280px]"
            placeholder={correcting ? '不填就和被更正的那一版比' : '不填就不比对'}
            onChange={(event) => patch({ compareWithVersion: event.target.value })}
          />
        </label>

        {problems.length > 0 ? (
          <ul className="text-xs text-idpxyz-danger list-disc ml-4">
            {problems.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        ) : null}

        <div className="flex items-center gap-3 flex-wrap">
          <Button
            variant="outline"
            onClick={runPreview}
            disabled={problems.length > 0 || preview.kind === 'previewing' || recorded}
          >
            {preview.kind === 'previewing' ? '预览中…' : currentPreview ? '重新预览' : '预览'}
          </Button>
          <Button
            variant="default"
            className={primaryActionClass}
            onClick={runRegister}
            disabled={!previewAccepted || registration.kind === 'submitting' || recorded}
            title={previewAccepted ? undefined : '先预览通过，再登记'}
          >
            {registration.kind === 'submitting' ? '登记中…' : correcting ? '登记更正版本' : '登记'}
          </Button>
          {onClose ? (
            <Button variant="outline" onClick={onClose}>
              收起
            </Button>
          ) : null}
        </div>

        {preview.kind === 'previewed' && currentPreview === null ? (
          <p className="text-xs text-idpxyz-textMuted">内容改过了，请重新预览。</p>
        ) : null}
        {currentPreview ? <PreviewNote answer={currentPreview} /> : null}
        {registration.kind === 'answered' ? <RegistrationNote answer={registration.answer} /> : null}
      </CardContent>
    </Card>
  );
}

function KindPicker({
  value,
  locked,
  onChange,
}: {
  value: SeriesKindDraft;
  locked: boolean;
  onChange: (kind: SeriesKindDraft) => void;
}) {
  const kinds: Exclude<SeriesKindDraft, ''>[] = ['FUEL_RATE', 'EXCHANGE_RATE', 'PUBLISHED_AMOUNT'];
  return (
    <div className="flex items-center gap-2">
      {kinds.map((kind) => (
        <Button
          key={kind}
          variant={value === kind ? 'default' : 'outline'}
          disabled={locked && value !== kind}
          onClick={() => onChange(kind)}
        >
          {labelOf(seriesKindLabels, kind)}
        </Button>
      ))}
    </div>
  );
}

/**
 * 汇率口径：一版商业价格政策。选单只列**声明了 fx 口径**的版本——领域不接受未声明口径的裸汇率，
 * 把另一类列出来只是多一个必然被拒的选项。候选读口今天同样在未配置那堵墙前，读不到时退回两格
 * 手填，表单不因此变死。
 */
function QuoteBasisField({
  value,
  onChange,
}: {
  value: SeriesDraft['quoteBasis'];
  onChange: (basis: SeriesDraft['quoteBasis']) => void;
}) {
  const [candidates, setCandidates] = useState<ApiResult<QuoteBasisCandidateListResponseBody> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void listQuoteBasisCandidates().then((answer) => {
      if (!cancelled) setCandidates(answer);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const usable: QuoteBasisCandidate[] =
    candidates?.kind === 'outcome'
      ? candidates.body.policies.filter((policy) => policy.caliberDeclared && policy.caliber?.fx)
      : [];
  const selected = value ? `${value.policyId}@${value.policyVersion}` : '';

  if (candidates?.kind === 'outcome') {
    return (
      <label className="block">
        <span className={fieldLabel}>汇率口径（定了汇率口径的价格政策）*</span>
        <select
          className={selectClass}
          value={selected}
          onChange={(event) => {
            const [policyId, policyVersion] = event.target.value.split('@');
            onChange(policyId && policyVersion ? { policyId, policyVersion } : null);
          }}
        >
          <option value="">请选择</option>
          {usable.map((policy) => (
            <option key={`${policy.objectId}@${policy.version}`} value={`${policy.objectId}@${policy.version}`}>
              {policy.objectId} 版本 {policy.version} · {labelOf(directionLabels, policy.direction)}
            </option>
          ))}
        </select>
        {usable.length === 0 ? (
          <span className="block text-[11px] text-idpxyz-textMuted mt-1">
            还没有定了汇率口径的价格政策；先到「商业规则」页的价格政策里定好汇率口径，再来登记汇率。
          </span>
        ) : null}
      </label>
    );
  }

  return (
    <div>
      <span className={fieldLabel}>汇率口径（价格政策）*</span>
      <div className="grid grid-cols-2 gap-3">
        <Input
          value={value?.policyId ?? ''}
          className="font-mono text-[13px]"
          placeholder="价格政策编号"
          onChange={(event) => onChange({ policyId: event.target.value, policyVersion: value?.policyVersion ?? '' })}
        />
        <Input
          value={value?.policyVersion ?? ''}
          className="font-mono text-[13px]"
          placeholder="版本，如 v1"
          onChange={(event) => onChange({ policyId: value?.policyId ?? '', policyVersion: event.target.value })}
        />
      </div>
      <span className="block text-[11px] text-idpxyz-textMuted mt-1">
        {candidates === null
          ? '正在读取价格政策…'
          : candidates.kind === 'unconfigured'
            ? '价格政策列表没读到（访问通道尚未配置），请手填编号与版本；预览时会检查它有没有定汇率口径。'
            : isOperatorRefusal(candidates)
              ? `价格政策列表没读到（${accessRefusalHeadline(candidates.kind)}），请手填编号与版本；预览时会检查它有没有定汇率口径。`
              : '价格政策列表没读到，请手填编号与版本；预览时会检查它有没有定汇率口径。'}
      </span>
    </div>
  );
}

function PeriodsEditor({
  periods,
  onChange,
  onAdd,
  onRemove,
}: {
  periods: PeriodDraft[];
  onChange: (index: number, change: Partial<PeriodDraft>) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
}) {
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <span className={fieldLabel}>每期数值（起点 / 止点 / 数值 / 凭证编号）*</span>
        <Button variant="outline" onClick={onAdd}>
          加一期
        </Button>
      </div>
      <p className="text-[11px] text-idpxyz-textMuted mb-2">
        日期填 2026-11-01（按当天零点 UTC 算）或带时刻的 2026-11-01T00:00:00Z；只有最后一期的止点可以留空，表示长期。
        凭证编号填来源公告或文件的编号，可不填；每期都有凭证，这一版才算「有凭证可查」。期次有没有重叠，预览时会检查。
      </p>
      <div className="flex flex-col gap-2">
        {periods.map((period, index) => (
          <div key={index} className="grid grid-cols-[1fr_1fr_120px_1fr_auto] gap-2 items-center">
            <Input
              value={period.startsAt}
              className="font-mono text-[12px]"
              placeholder="起点"
              onChange={(event) => onChange(index, { startsAt: event.target.value })}
            />
            <Input
              value={period.endsAt}
              className="font-mono text-[12px]"
              placeholder="止点（最后一期可空）"
              onChange={(event) => onChange(index, { endsAt: event.target.value })}
            />
            <Input
              value={period.value}
              inputMode="decimal"
              className="font-mono text-[12px]"
              placeholder="数值"
              onChange={(event) => onChange(index, { value: event.target.value })}
            />
            <Input
              value={period.evidenceRef}
              className="font-mono text-[12px]"
              placeholder="凭证编号（可空）"
              onChange={(event) => onChange(index, { evidenceRef: event.target.value })}
            />
            <Button variant="outline" disabled={periods.length <= 1} onClick={() => onRemove(index)}>
              删除
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}

function PreviewNote({ answer }: { answer: ApiResult<SeriesPreviewResponseBody> }) {
  return (
    <AnswerNote
      answer={answer}
      what="预览"
      unconfiguredNote="这次没能预览，重试也不会变；请管理员先配置访问通道。"
    >
      {(body) => <PreviewBody body={body} />}
    </AnswerNote>
  );
}

function PreviewBody({ body }: { body: SeriesPreviewResponseBody }) {
  const outcome = body.outcome;
  return (
    <div className="text-xs text-idpxyz-textMuted flex flex-col gap-1">
      <p>
        {labelOf(previewOutcomeLabels, outcome)}
      </p>
      {outcome === 'PREVIEWED' ? (
        <>
          <p>
            凭证：<strong>{labelOf(evidenceGradeLabels, body.evidenceGrade ?? '')}</strong>
          </p>
          {/* 内容摘要整串显示不截断：它就是登记后列表技术详情里那一格，对账的人要拿它去对。 */}
          <TechnicalDetails
            rows={[
              { label: '内容摘要', value: body.contentDigest ?? '' },
              { label: '规范化版本', value: body.canonicalization ?? '' },
            ]}
          />
          {body.comparison ? <ComparisonBody comparison={body.comparison} /> : null}
        </>
      ) : null}
    </div>
  );
}

function ComparisonBody({ comparison }: { comparison: NonNullable<SeriesPreviewResponseBody['comparison']> }) {
  return (
    <div className="mt-1">
      <p>
        {labelOf(comparisonOutcomeLabels, comparison.outcome)}
        {comparison.baseVersion ? (
          <>
            {' '}（对照版本 <span className="font-mono">{comparison.baseVersion}</span>）
          </>
        ) : null}
      </p>
      {comparison.outcome === 'COMPARED' ? (
        comparison.changes.length === 0 ? (
          <p>每一期都和对照版本一样。</p>
        ) : (
          <table className="mt-1 w-full text-[11px]">
            <thead>
              <tr className="text-left text-idpxyz-textMuted">
                <th className="pr-2 font-normal">起点</th>
                <th className="pr-2 font-normal">变化</th>
                <th className="pr-2 font-normal">对照版本</th>
                <th className="pr-2 font-normal">这次登记</th>
              </tr>
            </thead>
            <tbody>
              {comparison.changes.map((change) => (
                <tr key={change.startsAt}>
                  <td className="pr-2 font-mono">{change.startsAt.includes('T') ? formatInstant(change.startsAt) : change.startsAt}</td>
                  <td className="pr-2">{changeLabel(change)}</td>
                  <td className="pr-2 font-mono">{periodText(change.base)}</td>
                  <td className="pr-2 font-mono">{periodText(change.proposed)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )
      ) : null}
    </div>
  );
}

// CHANGED 时把哪几样变了点出来：只说「有改动」，看的人还得自己逐格比。
function changeLabel(change: SeriesPeriodChangeRecord): string {
  const base = labelOf(periodChangeKindLabels, change.kind);
  if (change.kind !== 'CHANGED') return base;
  const parts = [
    change.valueChanged ? '数值' : null,
    change.endChanged ? '止点' : null,
    change.evidenceChanged ? '凭证' : null,
  ].filter((part): part is string => part !== null);
  return parts.length > 0 ? `${base}（${parts.join('、')}）` : base;
}

function periodText(period: SeriesPeriodChangeRecord['base']): string {
  if (!period) return '—';
  const end = period.endsAt ?? '长期';
  const evidence = period.evidenceRef ? ` · ${period.evidenceRef}` : '';
  return `${period.value} → ${end}${evidence}`;
}

function RegistrationNote({ answer }: { answer: ApiResult<RegistrationResponseBody> }) {
  return (
    <AnswerNote
      answer={answer}
      what="登记"
      unconfiguredNote="这次没有登记，重试也不会变；请管理员先配置访问通道。"
    >
      {(body) => (
        <p className="text-xs text-idpxyz-textMuted">{registrationOutcomeLabels[body.outcome] ?? body.outcome}</p>
      )}
    </AnswerNote>
  );
}

/**
 * 五格判别与本目录其余面板同款（ADR-0022：状态码只答「有没有形成答案」，业务判别在响应体）。
 * 未收录的 outcome 原样示出英文原名：服务端新增一格时宁可显示原名，也不把它归进既有中文说法。
 */
function AnswerNote<Body>({
  answer,
  what,
  unconfiguredNote,
  children,
}: {
  answer: ApiResult<Body>;
  what: string;
  unconfiguredNote: string;
  children: (body: Body) => ReactNode;
}) {
  switch (answer.kind) {
    case 'outcome':
      return <>{children(answer.body)}</>;
    case 'unconfigured':
      return (
        // 渠道未配置是诚实答案不是尚未实现（ADR-0055）：改请求或重试都不会变，续办在配置接入渠道。
        <p className="text-xs text-idpxyz-textMuted">访问通道尚未配置：{unconfiguredNote}</p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what={what} />;
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          没能{what}：{problemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          这次{what}没处理完：{problemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className="text-xs text-idpxyz-danger">连不上系统，没能{what}（{answer.message}）；检查网络后重试。</p>;
  }
}
