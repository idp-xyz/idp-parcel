import { useMemo } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../../components/primary-action';
import { TechnicalDetails } from '../../../components/states';
import { aggregationLabels, priceCardPurposeLabels, priceDirectionLabels } from '../../../vocabulary/price-card';
import type { ApiResult } from '../../catalogue-api';
import type { PriceCardDraftSubmissionBody, PriceCardImportReading, PriceCardWorkbook } from '../api';
import { workbookToDocument } from '../card-form/document';
import type { Issue } from '../card-form/validate';
import { problemWords, type LocatedProblem, type MappedProblems } from '../card-form/problems';
import { parsePriceCardContent, type PriceCardContentView } from '../price-card-content';
import { PriceCardViewSections } from '../PriceCardDetailPanel';
import { priceCardDraftStatusLabels } from '../price-card-import';
import { SectionTitle, hintClass } from './controls';
import { ChannelProblem } from './ChannelProblem';
import { stepTitles } from '../card-form/model';
import { stepOfField } from '../card-form/fields';
import { CurrencyLabel } from '../../refdata-labels';

/** 存草稿的结果，用运营听得懂的话。 */
const draftOutcomeText: Record<string, string> = {
  DRAFT_SUBMITTED: '已存为草稿。',
  DRAFT_REPLAYED: '这张卡已经存过一份一模一样的草稿，没有重复保存。',
  DRAFT_REVISED: '这一版还没批准，已用这一份替换了原来的草稿。',
  CONTENT_FIXED: '这一版已经批准，内容不能再改，这次没有保存。要调价请新建下一个版本。',
  NOT_ACCEPTED: '没有保存：这份内容连方案标识和版本都读不出来。',
};

export type PreviewState = {
  /** 预览时那一份内容的指纹；与眼前的内容不同说明预览之后又改过。 */
  signature: string;
  answer: ApiResult<PriceCardImportReading>;
  mapped: MappedProblems;
  workbook: PriceCardWorkbook;
};

function localView(workbook: PriceCardWorkbook): PriceCardContentView | null {
  try {
    return parsePriceCardContent(workbookToDocument(workbook));
  } catch {
    return null;
  }
}

/** 说不到具体格子的问题：中文留作话，夹带的英文原文折叠。 */
function UnlocatedLine({ message }: { message: string }) {
  const words = problemWords(message);
  return (
    <li className="rounded border border-idpxyz-border px-3 py-2 text-[12px] text-[color:var(--ds-status-danger-fg)]">
      {words.text}
      {words.original ? (
        <details className="ml-1 inline-block align-top text-idpxyz-breadcrumb">
          <summary className="cursor-pointer select-none">原话</summary>
          <span className="font-mono break-all">{words.original}</span>
        </details>
      ) : null}
    </li>
  );
}

function ProblemList({ located, onJump }: { located: readonly LocatedProblem[]; onJump: (fieldId: string) => void }) {
  return (
    <ul className="space-y-1.5">
      {located.map((entry, index) => (
        <li key={`${index}-${entry.field}-${entry.problem.code}`} className="flex flex-wrap items-baseline gap-x-2 rounded border border-idpxyz-border px-3 py-2 text-[12px]">
          <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-idpxyz-breadcrumb">{stepTitles[entry.step]}</span>
          <span className="font-medium text-idpxyz-textBright">{entry.where}</span>
          <span className="min-w-0 flex-1 text-[color:var(--ds-status-danger-fg)]">
            {entry.text}
            {entry.original ? (
              <details className="ml-1 inline-block align-top text-idpxyz-breadcrumb">
                <summary className="cursor-pointer select-none">原话</summary>
                <span className="font-mono break-all">{entry.original}</span>
              </details>
            ) : null}
          </span>
          <Button variant="outline" size="sm" onClick={() => onJump(entry.field)}>
            去修改
          </Button>
        </li>
      ))}
    </ul>
  );
}

export function StepPreview({
  localIssues,
  preview,
  fresh,
  workbook,
  phase,
  draft,
  draftSaved,
  onPreview,
  onSave,
  onJump,
  onOpenDrafts,
}: {
  localIssues: readonly Issue[];
  preview: PreviewState | null;
  /** 预览之后没有再改过内容。 */
  fresh: boolean;
  workbook: PriceCardWorkbook | null;
  phase: 'idle' | 'previewing' | 'saving';
  draft: ApiResult<PriceCardDraftSubmissionBody> | null;
  draftSaved: boolean;
  onPreview: () => void;
  onSave: () => void;
  onJump: (fieldId: string) => void;
  onOpenDrafts: () => void;
}) {
  const reading = preview?.answer.kind === 'outcome' ? preview.answer.body : null;
  const passed = reading?.outcome === 'VALIDATED';
  const view = useMemo(() => (passed && preview ? localView(preview.workbook) : null), [passed, preview]);
  const content = reading?.content;
  const savable = passed && fresh && !draftSaved;

  return (
    <div className="space-y-5">
      <p className="text-[12px] text-idpxyz-text">
        预览会把这张价卡完整检查一遍，但不会保存。通过之后再「存为草稿」，草稿要到「草稿」签批准、发布后才生效。
      </p>

      {localIssues.length > 0 ? (
        <section aria-label="还没填好的地方">
          <SectionTitle>还有 {localIssues.length} 处没填好</SectionTitle>
          <ul className="space-y-1.5">
            {localIssues.map((issue, index) => (
              <li key={`${index}-${issue.field}`} className="flex flex-wrap items-baseline gap-x-2 rounded border border-idpxyz-border px-3 py-2 text-[12px]">
                <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-idpxyz-breadcrumb">{stepTitles[stepOfField(issue.field)]}</span>
                <span className="font-medium text-idpxyz-textBright">{issue.where}</span>
                <span className="min-w-0 flex-1 text-[color:var(--ds-status-danger-fg)]">{issue.text}</span>
                <Button variant="outline" size="sm" onClick={() => onJump(issue.field)}>
                  去修改
                </Button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <div className="flex flex-wrap items-center gap-3">
        {/* 主动作随进度换位：没预览通过之前是「预览」，通过之后是「存为草稿」。 */}
        <Button
          variant={savable ? 'outline' : 'default'}
          className={savable ? undefined : primaryActionClass}
          onClick={onPreview}
          disabled={phase !== 'idle'}
        >
          {phase === 'previewing' ? '预览中…' : preview ? '重新预览' : '预览'}
        </Button>
        <Button variant={savable ? 'default' : 'outline'} className={savable ? primaryActionClass : undefined} onClick={onSave} disabled={phase !== 'idle' || !savable}>
          {phase === 'saving' ? '保存中…' : '存为草稿'}
        </Button>
        {preview && !fresh ? <span className="text-[12px] text-[color:var(--ds-status-warning-fg)]">预览之后又改过内容，请重新预览；存草稿存的是预览过的那一份。</span> : null}
      </div>

      {draft ? (
        <section aria-label="存草稿的结果">
          {draft.kind !== 'outcome' ? (
            <ChannelProblem answer={draft} what="存草稿" onRetry={onSave} />
          ) : (
            <div className="rounded border border-idpxyz-border p-3 text-[12px] text-idpxyz-text">
              <p className="font-medium text-idpxyz-textBright">{draftOutcomeText[draft.body.outcome] ?? '草稿已处理。'}</p>
              {draft.body.draft ? (
                <p className="mt-1">
                  草稿号 <span className="font-mono">{draft.body.draft.plan.id}@{draft.body.draft.plan.version}</span> · 状态：
                  {priceCardDraftStatusLabels[draft.body.draft.status] ?? draft.body.draft.status}
                </p>
              ) : null}
              {draft.body.draft ? (
                <Button variant="outline" size="sm" className="mt-2" onClick={onOpenDrafts}>
                  去草稿签批准
                </Button>
              ) : null}
            </div>
          )}
        </section>
      ) : null}

      {preview && preview.answer.kind !== 'outcome' ? <ChannelProblem answer={preview.answer} what="预览" onRetry={onPreview} /> : null}

      {reading && !passed ? (
        <section aria-label="预览发现的问题">
          <SectionTitle hint="问题已经在对应的格子里标红；也可以在这里点「去修改」直达。改完回到这一步重新预览。">
            预览发现 {preview?.mapped.located.length ?? 0} 处要改
            {preview && preview.mapped.unlocated.length > 0 ? `，另有 ${preview.mapped.unlocated.length} 处说不到具体格子` : ''}
          </SectionTitle>
          {preview && preview.mapped.located.length > 0 ? <ProblemList located={preview.mapped.located} onJump={onJump} /> : null}
          {preview && preview.mapped.unlocated.length > 0 ? (
            <ul className="mt-2 space-y-1.5" aria-label="说不到具体格子的问题">
              {preview.mapped.unlocated.map((problem, index) => (
                <UnlocatedLine key={`${index}-${problem.code}`} message={problem.message} />
              ))}
            </ul>
          ) : null}
          {reading.outcome === 'NOT_ACCEPTED' && (preview?.mapped.located.length ?? 0) + (preview?.mapped.unlocated.length ?? 0) === 0 ? (
            <p className="text-[12px] text-[color:var(--ds-status-danger-fg)]">这份内容没有被接受，也没有给出具体原因；请检查方案标识和版本是否填好。</p>
          ) : null}
        </section>
      ) : null}

      {passed && content ? (
        <section aria-label="预览通过">
          <p className="mb-3 rounded border border-idpxyz-border border-l-2 border-l-[color:var(--ds-status-success-fg)] px-3 py-2 text-[12px] text-idpxyz-text">
            预览通过：这张价卡没有发现问题。
          </p>
          <dl className="grid grid-cols-[7rem_1fr] gap-x-3 gap-y-1 text-[12px] text-idpxyz-text">
            <dt className="text-idpxyz-breadcrumb">方案</dt>
            <dd className="font-mono">
              {reading?.plan?.id}@{reading?.plan?.version}
            </dd>
            <dt className="text-idpxyz-breadcrumb">方向 / 用途</dt>
            <dd>
              {priceDirectionLabels[content.direction] ?? content.direction} · {priceCardPurposeLabels[content.purpose] ?? content.purpose} ·{' '}
              {aggregationLabels[content.aggregation] ?? content.aggregation}
            </dd>
            <dt className="text-idpxyz-breadcrumb">适用范围</dt>
            <dd className="font-mono">{content.scope}</dd>
            <dt className="text-idpxyz-breadcrumb">币种与单位</dt>
            <dd>
              <CurrencyLabel code={content.rateTable.currency} /> · {view?.header.weightUnit ?? content.rateTable.weightUnit}
            </dd>
            <dt className="text-idpxyz-breadcrumb">费用项</dt>
            <dd>
              基础运费 1 项，固定金额 {content.fixedCharges} 项，其他附加费 {content.surcharges} 项
            </dd>
          </dl>
          <div className="mt-4">
            <SectionTitle hint="保存后在价格表里看到的就是这个样子。">这张卡会怎么收</SectionTitle>
            {view ? <PriceCardViewSections view={view} /> : <p className={hintClass}>这份内容暂时没法在页面上念出来，但预览已经通过，可以存草稿。</p>}
          </div>
          <TechnicalDetails
            rows={[
              { label: '内容摘要', value: content.contentDigest },
              { label: '内容格式版本', value: content.canonicalization },
              ...(reading?.sourceFile ? [{ label: '来源标识', value: reading.sourceFile.name }] : []),
            ]}
          />
        </section>
      ) : null}

    </div>
  );
}
