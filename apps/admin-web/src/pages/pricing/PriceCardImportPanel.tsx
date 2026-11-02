import { formatInstant } from '../moment';
import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { labelOf, problemNote } from './presentation';
import { priceCardPurposeLabels, priceDirectionLabels } from '../../vocabulary/price-card';
import { TechnicalDetails, httpRows } from '../../components/TechnicalDetails';
import { technicalRows } from '../settlement/ops-ui';
import {
  previewPriceCardImport,
  priceCardTemplateFileName,
  priceCardTemplateHref,
  submitPriceCardDraft,
  type PriceCardDraftSubmissionBody,
  type PriceCardImportContent,
  type PriceCardImportReading,
  type PriceCardProblem,
} from './api';
import {
  draftSubmittable,
  priceCardDraftOutcomeLabels,
  priceCardDraftStatusLabels,
  priceCardPreviewOutcomeLabels,
  sortPriceCardProblems,
} from './price-card-import';

type Phase = 'idle' | 'previewing' | 'saving';

/**
 * 导入价卡：下载模板、上传同一份字节做预览、再存为草稿。
 *
 * 端点今天答 403 时只呈现未配置。预览与草稿的样例不写在这里——没有真答复就不画表。
 */
export function PriceCardImportPanel() {
  const [file, setFile] = useState<File | null>(null);
  const [phase, setPhase] = useState<Phase>('idle');
  const [preview, setPreview] = useState<ApiResult<PriceCardImportReading> | null>(null);
  const [draft, setDraft] = useState<ApiResult<PriceCardDraftSubmissionBody> | null>(null);

  function choose(next: File | null) {
    setFile(next);
    setPreview(null);
    setDraft(null);
  }

  async function previewFile() {
    if (!file) return;
    setPhase('previewing');
    setDraft(null);
    const answer = await previewPriceCardImport(file, file.name);
    setPreview(answer);
    setPhase('idle');
  }

  async function saveDraft() {
    if (!file) return;
    setPhase('saving');
    const answer = await submitPriceCardDraft(file, file.name);
    setDraft(answer);
    setPhase('idle');
  }

  return (
    <div className="flex-1 overflow-auto p-4">
      <Card>
        <CardHeader>
          <CardTitle>导入价卡</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {/* 预览与存草稿打的是同一份字节（预览过的就是存下的那份）；录入人由接入渠道给，表单不收。 */}
          <p className="text-xs text-idpxyz-textMuted">
            适合已经在 Excel 里做好的价卡：下载模板，按模板填好后上传，先「检查并预览」，没有问题再「存为草稿」。
            存下的草稿到「草稿与审批」签里批准、发布。
          </p>
          <p className="text-xs">
            <a className="text-idpxyz-accent underline" href={priceCardTemplateHref} download={priceCardTemplateFileName}>
              下载模板（{priceCardTemplateFileName}）
            </a>
          </p>
          <label className="text-xs text-idpxyz-textMuted">
            选择填好的 Excel 文件
            <input
              className="mt-1 block text-xs text-idpxyz-text"
              type="file"
              accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              onChange={(event) => choose(event.target.files?.[0] ?? null)}
            />
          </label>
          <div className="flex items-center gap-3">
            <Button onClick={() => void previewFile()} disabled={!file || phase !== 'idle'}>
              {phase === 'previewing' ? '检查中…' : '检查并预览'}
            </Button>
            <Button
              variant="default"
              className={primaryActionClass}
              onClick={() => void saveDraft()}
              disabled={!file || phase !== 'idle' || !draftSubmittable(preview)}
            >
              {phase === 'saving' ? '保存中…' : '存为草稿'}
            </Button>
          </div>
          <p className="text-xs text-idpxyz-textMuted">
            先预览这份文件才能存为草稿；换了文件要重新预览。
          </p>
          {preview ? (
            <ReadingAnswer
              title="预览"
              answer={preview}
              labels={priceCardPreviewOutcomeLabels}
            />
          ) : null}
          {draft ? <DraftAnswer answer={draft} /> : null}
        </CardContent>
      </Card>
    </div>
  );
}

function ReadingAnswer({
  title,
  answer,
  labels,
}: {
  title: string;
  answer: ApiResult<PriceCardImportReading>;
  labels: Record<string, string>;
}) {
  if (answer.kind !== 'outcome') {
    return <ChannelNote title={title} answer={answer} />;
  }
  const reading = answer.body;
  return (
    <section className="flex flex-col gap-2 text-xs">
      <p>
        {title}：{labelOf(labels, reading.outcome)}
      </p>
      {reading.plan ? (
        <p>
          价卡 <span className="font-mono">{reading.plan.id}</span> · 版本 <span className="font-mono">{reading.plan.version}</span>
        </p>
      ) : null}
      {reading.content ? <ContentSummary content={reading.content} /> : null}
      <ProblemTable problems={sortPriceCardProblems(reading.problems ?? [])} />
      <TechnicalDetails
        rows={technicalRows([
          { label: '结果码', value: reading.outcome },
          { label: '模板版本', value: reading.templateVersion },
          { label: '文件名', value: reading.sourceFile?.name },
          { label: '文件指纹', value: reading.sourceFile?.sha256 },
          { label: '内容摘要', value: reading.content?.contentDigest },
          { label: '规范化版本', value: reading.content?.canonicalization },
          {
            label: '价表',
            value: reading.content ? `${reading.content.rateTable.id}@${reading.content.rateTable.version}` : undefined,
          },
        ])}
      />
    </section>
  );
}

function DraftAnswer({ answer }: { answer: ApiResult<PriceCardDraftSubmissionBody> }) {
  if (answer.kind !== 'outcome') {
    return <ChannelNote title="存草稿" answer={answer} />;
  }
  const body = answer.body;
  return (
    <section className="flex flex-col gap-2 text-xs">
      <p>存草稿：{labelOf(priceCardDraftOutcomeLabels, body.outcome)}</p>
      {body.draft ? (
        <p>
          草稿现在是「{labelOf(priceCardDraftStatusLabels, body.draft.status)}」 ·{' '}
          <span className="font-mono">{body.draft.plan.id}</span> 版本 <span className="font-mono">{body.draft.plan.version}</span> ·
          录入于 {formatInstant(body.draft.submittedAt)}
        </p>
      ) : (
        <p className="text-idpxyz-textMuted">这次没有改动草稿；草稿现在的样子到「草稿与审批」签里看。</p>
      )}
      <TechnicalDetails
        rows={technicalRows([
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '结果码', value: body.outcome },
          { label: '草稿状态码', value: body.draft?.status },
          { label: '录入人', value: body.draft?.submitter },
        ])}
      />
      <ReadingAnswer title="这次检查" answer={{ kind: 'outcome', status: answer.status, body: body.reading }} labels={priceCardPreviewOutcomeLabels} />
    </section>
  );
}

function ContentSummary({ content }: { content: PriceCardImportContent }) {
  return (
    <dl className="grid grid-cols-[8rem_1fr] gap-x-3 gap-y-1 text-idpxyz-textMuted">
      <dt>方向</dt>
      <dd>
        {labelOf(priceDirectionLabels, content.direction)} · {labelOf(priceCardPurposeLabels, content.purpose)}
      </dd>
      <dt>适用范围</dt>
      <dd className="font-mono">{content.scope}</dd>
      <dt>生效自</dt>
      <dd className="font-mono">{content.startsAt.includes('T') ? formatInstant(content.startsAt) : content.startsAt}</dd>
      <dt>价表</dt>
      <dd>{content.rateTable.rows} 行</dd>
    </dl>
  );
}

/** 逐格问题表；草稿签展开`草稿`行时用同一张，同一格问题两签摆法一样。 */
export function ProblemTable({ problems }: { problems: PriceCardProblem[] }) {
  if (problems.length === 0) {
    return <p className="text-idpxyz-textMuted">没有发现问题。</p>;
  }
  return (
    <table className="w-full text-left text-xs">
      <thead className="text-idpxyz-textMuted">
        <tr>
          <th className="py-1 pr-3 font-normal">工作表</th>
          <th className="py-1 pr-3 font-normal">行</th>
          <th className="py-1 pr-3 font-normal">列</th>
          <th className="py-1 font-normal">问题</th>
        </tr>
      </thead>
      <tbody>
        {problems.map((problem, index) => (
          <tr key={`${problem.sheet}-${problem.row}-${problem.column}-${problem.code}-${index}`}>
            <td className="py-1 pr-3 font-mono">{problem.sheet}</td>
            <td className="py-1 pr-3 font-mono">{problem.row}</td>
            <td className="py-1 pr-3 font-mono">{problem.column}</td>
            {/* 问题码不上屏，悬停可见：对接模板规则时要它。 */}
            <td className="py-1" title={problem.code}>
              {problem.message}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ChannelNote({ title, answer }: { title: string; answer: ApiResult<unknown> }) {
  switch (answer.kind) {
    case 'outcome':
      return null;
    case 'unconfigured':
      return (
        // 渠道未配置是诚实答案（ADR-0055）：改文件或重试都不会变，页面也不拿演示数据顶替。
        <p className="text-xs text-idpxyz-textMuted">
          {title}：访问通道尚未配置（403），这次没有处理；改文件或重试都不会变，请管理员先配置访问通道。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what={title} />;
    case 'callerProblem':
      return (
        <div className="text-xs text-idpxyz-danger">
          <p>
            {title}没成功：{problemNote(answer.code)}
            {answer.detail ? ` 原因：${answer.detail}` : ''}
          </p>
          <TechnicalDetails rows={httpRows(answer.status, answer.code)} className="mt-1 text-idpxyz-text" />
        </div>
      );
    case 'noAnswer':
      return (
        <div className="text-xs text-idpxyz-danger">
          <p>
            {title}这次没处理完：{problemNote(answer.code)}
          </p>
          <TechnicalDetails rows={httpRows(answer.status, answer.code)} className="mt-1 text-idpxyz-text" />
        </div>
      );
    case 'transport':
      return (
        <p className="text-xs text-idpxyz-danger">
          {title}：连不上系统（{answer.message}）；检查网络后重试。
        </p>
      );
  }
}
