import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { labelOf, problemNote, directionLabels, purposeLabels } from './presentation';
import {
  previewPriceCardImport,
  priceCardDraftPath,
  priceCardPreviewPath,
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

const info = moduleInfoById['price-card-catalog'];

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
          <p className="text-xs text-idpxyz-textMuted">
            下载空白模板，填完后上传。先打{' '}
            <span className="font-mono">{priceCardPreviewPath}</span> 看校验结果与摘要，再把
            <span className="font-mono">同一份字节</span> 打到{' '}
            <span className="font-mono">{priceCardDraftPath}</span> 存为草稿。两口都不收租户或录入者，
            身份在操作者信封里。
          </p>
          <p className="text-xs">
            <a className="text-idpxyz-accent underline" href={priceCardTemplateHref} download={priceCardTemplateFileName}>
              下载模板（{priceCardTemplateFileName}）
            </a>
          </p>
          <label className="text-xs text-idpxyz-textMuted">
            价卡工作簿
            <input
              className="mt-1 block text-xs text-idpxyz-text"
              type="file"
              accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              onChange={(event) => choose(event.target.files?.[0] ?? null)}
            />
          </label>
          <div className="flex items-center gap-3">
            <Button onClick={() => void previewFile()} disabled={!file || phase !== 'idle'}>
              {phase === 'previewing' ? '校验中…' : '校验并预览'}
            </Button>
            <Button onClick={() => void saveDraft()} disabled={!file || phase !== 'idle' || !draftSubmittable(preview)}>
              {phase === 'saving' ? '保存中…' : '存为草稿'}
            </Button>
          </div>
          <p className="text-xs text-idpxyz-textMuted">
            存草稿要先看到这一份文件的预览。换文件之后预览作废，避免把上一份的摘要存成这一份。
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
        {title}：<span className="font-mono">{reading.outcome}</span> —— {labelOf(labels, reading.outcome)}
        {reading.templateVersion ? (
          <>
            {' '}
            · 模板 <span className="font-mono">{reading.templateVersion}</span>
          </>
        ) : null}
      </p>
      {reading.sourceFile ? (
        <p className="font-mono text-idpxyz-textMuted">
          {reading.sourceFile.name} · {reading.sourceFile.sha256}
        </p>
      ) : null}
      {reading.plan ? (
        <p>
          方案 <span className="font-mono">{reading.plan.id}@{reading.plan.version}</span>
        </p>
      ) : null}
      {reading.content ? <ContentSummary content={reading.content} /> : null}
      <ProblemTable problems={sortPriceCardProblems(reading.problems ?? [])} />
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
      <p>
        存草稿（HTTP {answer.status}）：<span className="font-mono">{body.outcome}</span> ——{' '}
        {labelOf(priceCardDraftOutcomeLabels, body.outcome)}
      </p>
      {body.draft ? (
        <p>
          册上这一行：<span className="font-mono">{body.draft.status}</span> ——{' '}
          {labelOf(priceCardDraftStatusLabels, body.draft.status)} ·{' '}
          <span className="font-mono">
            {body.draft.plan.id}@{body.draft.plan.version}
          </span>{' '}
          · 录入者 <span className="font-mono">{body.draft.submitter}</span> · {body.draft.submittedAt}
        </p>
      ) : (
        <p className="text-idpxyz-textMuted">这次没有写下册上的行。册上此刻是什么由草稿查阅口答。</p>
      )}
      <ReadingAnswer title="这次读法" answer={{ kind: 'outcome', status: answer.status, body: body.reading }} labels={priceCardPreviewOutcomeLabels} />
    </section>
  );
}

function ContentSummary({ content }: { content: PriceCardImportContent }) {
  return (
    <dl className="grid grid-cols-[8rem_1fr] gap-x-3 gap-y-1 text-idpxyz-textMuted">
      <dt>方向 / 目的</dt>
      <dd>
        {labelOf(directionLabels, content.direction)} · {labelOf(purposeLabels, content.purpose)}
      </dd>
      <dt>适用范围</dt>
      <dd className="font-mono">{content.scope}</dd>
      <dt>适用期起</dt>
      <dd className="font-mono">{content.startsAt}</dd>
      <dt>内容摘要</dt>
      <dd className="font-mono break-all">{content.contentDigest}</dd>
      <dt>规范化版本</dt>
      <dd className="font-mono">{content.canonicalization}</dd>
      <dt>价表</dt>
      <dd className="font-mono">
        {content.rateTable.id}@{content.rateTable.version} · {content.rateTable.rows} 行
      </dd>
    </dl>
  );
}

function ProblemTable({ problems }: { problems: PriceCardProblem[] }) {
  if (problems.length === 0) {
    return <p className="text-idpxyz-textMuted">没有逐格问题。</p>;
  }
  return (
    <table className="w-full text-left text-xs">
      <thead className="text-idpxyz-textMuted">
        <tr>
          <th className="py-1 pr-3 font-normal">表</th>
          <th className="py-1 pr-3 font-normal">行</th>
          <th className="py-1 pr-3 font-normal">列</th>
          <th className="py-1 pr-3 font-normal">码</th>
          <th className="py-1 font-normal">说明</th>
        </tr>
      </thead>
      <tbody>
        {problems.map((problem, index) => (
          <tr key={`${problem.sheet}-${problem.row}-${problem.column}-${problem.code}-${index}`}>
            <td className="py-1 pr-3 font-mono">{problem.sheet}</td>
            <td className="py-1 pr-3 font-mono">{problem.row}</td>
            <td className="py-1 pr-3 font-mono">{problem.column}</td>
            <td className="py-1 pr-3 font-mono">{problem.code}</td>
            <td className="py-1">{problem.message}</td>
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
        <p className="text-xs text-idpxyz-textMuted">
          {title}：接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。{info.owner}
          的操作者渠道发行方尚未登记，服务端按 ADR-0055 如实拒绝。这是诚实答案，改文件或重试不会改变结果。
          发行方参数到位后由装配侧换上真核验即放行。此前不要拿演示数据填这张表。
        </p>
      );
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          {title}：调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
          {answer.detail ? ` 原因：${answer.detail}` : ''}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          {title}：服务端未形成答案（HTTP {answer.status}）：{problemNote(answer.code)}；可稍后重试。
        </p>
      );
    case 'transport':
      return (
        <p className="text-xs text-idpxyz-danger">
          {title}：请求未到达 parcel-api：{answer.message}
        </p>
      );
  }
}
