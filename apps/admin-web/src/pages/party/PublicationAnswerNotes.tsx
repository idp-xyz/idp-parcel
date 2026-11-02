import type { ReactNode } from 'react';
import { OperatorRefusalNote, TechnicalDetails, httpRows } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { declarationLandingLabels, publicationOutcomeLabels } from './api';
import { labelOf, problemNote } from './presentation';
import {
  draftStatusLabels,
  previewOutcomeLabels,
  publishOutcomeLabels,
  type CommercialPublicationAnswerBody,
  type PublicationDraftPublicationResponseBody,
  type PublicationDraftResponseBody,
  type PublicationPreviewResponseBody,
} from './publication-draft-api';
import type { PublicationFact } from './publication-summary';

// 发布路径四口答复的呈现件：录入页的五步流程与「待批准发布」的详情抽屉共用同一份，两处各写一份会在某一格中文改动时分叉。

// 这几口等的是 ADR-0100 那一族**操作者身份**的接入渠道（ADR-0126 Decision 五：Intake 到位那天把操作者信封译成
// 录入者 / 批准者），不是 `PAR-INT-01` 的客户生产委托渠道——两者是两族，参数登记册今天也没有操作者渠道那一行，
// 所以这里说「机制半边待接线」而不指某个登记册行。
const unconfiguredNote = '改内容或重试都不会改变结果，要先由管理员完成操作者接入配置。';

export function PreviewNote({ answer }: { answer: ApiResult<PublicationPreviewResponseBody> }) {
  return (
    <FlowAnswerNote answer={answer} what="预览">
      {(body) => (
        <div className="text-xs text-idpxyz-textMuted flex flex-col gap-1">
          <p>
            <OutcomeText labels={previewOutcomeLabels} outcome={body.outcome} />
          </p>
          {body.cause ? <p>原因：{body.cause}</p> : null}
          <TechnicalDetails
            rows={[
              { label: '结果码', value: body.outcome },
              ...(body.outcome === 'PREVIEWED' ? digestRows(body.canonicalization, body.contentDigest) : []),
            ]}
          />
        </div>
      )}
    </FlowAnswerNote>
  );
}

export function DraftNote({
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
          {body.status ? <p>现在的状态：{labelOf(draftStatusLabels, body.status)}</p> : null}
          {body.cause ? <p>原因：{body.cause}</p> : null}
          <TechnicalDetails
            rows={[
              { label: '结果码', value: body.outcome },
              { label: '状态码', value: body.status ?? '' },
              ...(body.contentDigest ? digestRows(body.canonicalization, body.contentDigest) : []),
            ]}
          />
        </div>
      )}
    </FlowAnswerNote>
  );
}

export function PublicationNote({ answer }: { answer: ApiResult<PublicationDraftPublicationResponseBody> }) {
  return (
    <FlowAnswerNote answer={answer} what="发布">
      {(body) => (
        <div className="text-xs text-idpxyz-textMuted flex flex-col gap-1">
          <p>
            <OutcomeText labels={publishOutcomeLabels} outcome={body.outcome} />
          </p>
          {body.publication ? <EmbeddedPublication publication={body.publication} /> : null}
          {body.status ? <p>现在的状态：{labelOf(draftStatusLabels, body.status)}</p> : null}
          {body.cause ? <p>原因：{body.cause}</p> : null}
          <TechnicalDetails
            rows={[
              { label: '结果码', value: body.outcome },
              { label: '状态码', value: body.status ?? '' },
            ]}
          />
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
      <TechnicalDetails
        rows={[
          { label: '发布结果码', value: publication.outcome },
          { label: '提交的摘要', value: publication.declaredDigest ?? '' },
          { label: '核算的摘要', value: publication.computedDigest ?? '' },
        ]}
      />
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
 * 答复一行只说中文结论；原码进各答复底部的「技术详情」，给要对日志的人。未收录的原码照样显出原名——服务端新增一格时
 * 宁可显示原名，也不把它归进既有中文说法。
 */
function OutcomeText({ labels, outcome }: { labels: Record<string, string>; outcome: string }) {
  return <>{labelOf(labels, outcome)}</>;
}

// 摘要整串显示不截断：它就是载体与册上那一格，操作者要拿它去对。
export function FactList({ facts }: { facts: readonly PublicationFact[] }) {
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

function digestRows(canonicalization?: string, digest?: string): { label: string; value: string }[] {
  return [
    { label: '内容摘要', value: digest ?? '' },
    { label: '摘要算法', value: canonicalization ?? '' },
  ];
}

/**
 * 五格判别与本目录其余面板同款（ADR-0022：状态码只答「有没有形成答案」，业务判别在响应体）。未收录的
 * outcome 原样示出英文原名：服务端新增一格时宁可显示原名，也不把它归进既有中文说法。
 */
export function FlowAnswerNote<Body>({
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
        <div className="text-xs text-idpxyz-danger">
          <p>
            {what}没有被接受：{problemNote(answer.code)}
            {answer.detail ? ` ${answer.detail}` : ''}
          </p>
          <TechnicalDetails rows={httpRows(answer.status, answer.code)} />
        </div>
      );
    case 'noAnswer':
      return (
        <div className="text-xs text-idpxyz-danger">
          <p>系统这次没有处理完{what}：{problemNote(answer.code)}；请稍后重试。</p>
          <TechnicalDetails rows={httpRows(answer.status, answer.code)} />
        </div>
      );
    case 'transport':
      return (
        <div className="text-xs text-idpxyz-danger">
          <p>连不上服务，{what}没有完成；请确认网络与服务在运行后重试。</p>
          <TechnicalDetails rows={[{ label: '传输层', value: answer.message }]} />
        </div>
      );
  }
}
