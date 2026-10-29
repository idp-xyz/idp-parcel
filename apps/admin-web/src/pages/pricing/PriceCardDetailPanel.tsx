import { useMemo, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { StatusPill } from '../../components/StatusPill';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { priceCardPurposeLabels, priceDirectionLabels } from '../../vocabulary/price-card';
import { CurrencyLabel } from '../refdata-labels';
import { formatInstant } from '../moment';
import { Fact, FactStrip, InstantRange } from '../party/detail-primitives';
import { problemNote } from './presentation';
import type { PriceCardContentFoundBody, PriceCardRecord } from './api';
import {
  parsePriceCardContent,
  priceCardStatus,
  priceCardStatusLabel,
  type FeeItem,
  type PriceCardContentView,
  type ReferenceRow,
} from './price-card-content';
import { usePriceCardContent } from './use-price-card-content';

// 价格表工作台的详情栏：点开一张价卡，直接看到它有哪些费用项、每项怎么收、什么时候收，不用靠试算一个个试。
// 内容来自价卡内容读口（GET /pricing-price-cards/content）原样交出的规范化文档，翻成人话在 price-card-content.ts；
// 这里只摆。系统引用（方案标识、规范化版本、内容摘要、各引用标识、登记责任方）收进底部的「技术详情」。
//
// 读不到如实显示，不画成空：未配置、没有授予、查无、本构建读不了的格式各说各的话。读到的文档里有看不懂的结构，
// 在费用项表里标「这一项暂不支持展示」并带原文折叠，页头另提示一句——不是这张卡没有这些收费。

const sectionTitle = 'mb-2 text-[13px] font-semibold text-idpxyz-textBright';
const note = 'text-[12px] text-idpxyz-breadcrumb';

function Lines({ lines }: { lines: readonly string[] }) {
  return (
    <>
      {lines.map((line, index) => (
        <p key={`${index}-${line}`} className={index === 0 ? undefined : 'mt-0.5'}>
          {line}
        </p>
      ))}
    </>
  );
}

function FeeItemRow({ item }: { item: FeeItem }) {
  const unsupported = item.kind === 'unsupported';
  return (
    <tr className="border-b border-idpxyz-border align-top last:border-b-0" data-kind={item.kind}>
      <th scope="row" className="px-2 py-2 text-left font-medium text-idpxyz-textBright">
        {item.name}
      </th>
      <td className="px-2 py-2 text-idpxyz-text">
        <Lines lines={item.how} />
        {unsupported && item.raw !== undefined ? (
          <details className="mt-1">
            <summary className="cursor-pointer select-none text-idpxyz-breadcrumb hover:text-idpxyz-text">查看原文</summary>
            <pre className="mt-1 max-h-48 overflow-auto rounded border border-idpxyz-border bg-idpxyz-sidebar p-2 font-mono text-[11px] leading-snug">
              {JSON.stringify(item.raw, null, 2)}
            </pre>
          </details>
        ) : null}
      </td>
      <td className="px-2 py-2 text-idpxyz-text">
        {item.when.length > 0 ? <Lines lines={item.when} /> : <span className={note}>—</span>}
      </td>
      <td className="px-2 py-2 text-idpxyz-breadcrumb">
        {item.notes.length > 0 ? <Lines lines={item.notes} /> : <span>—</span>}
      </td>
    </tr>
  );
}

function FeeItemsTable({ items }: { items: readonly FeeItem[] }) {
  return (
    <div className="overflow-x-auto rounded border border-idpxyz-border">
      <table className="w-full min-w-[560px] text-[12px]">
        <thead className="bg-idpxyz-sidebar text-left text-idpxyz-breadcrumb">
          <tr className="border-b border-idpxyz-border">
            <th scope="col" className="w-[19%] px-2 py-1.5 font-medium">费用</th>
            <th scope="col" className="w-[33%] px-2 py-1.5 font-medium">怎么收</th>
            <th scope="col" className="w-[28%] px-2 py-1.5 font-medium">什么时候收</th>
            <th scope="col" className="w-[20%] px-2 py-1.5 font-medium">备注</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <FeeItemRow key={item.key} item={item} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ReferenceList({ rows }: { rows: readonly ReferenceRow[] }) {
  return (
    <ul className="space-y-2">
      {rows.map((row) => (
        <li key={`${row.kindLabel}:${row.name}`} className="rounded border border-idpxyz-border px-3 py-2 text-[12px]">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
            <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-idpxyz-breadcrumb">{row.kindLabel}</span>
            <span className="font-mono text-idpxyz-textBright">{row.name}</span>
            {/* 有对应的管理台页面才跳；目录今天没有页面，只显示。 */}
            {row.target === 'reference-series' ? (
              <a href="#/reference-series" className="ml-auto text-idpxyz-accent hover:underline focus-visible:underline">
                去「汇率与燃油费率」查看
              </a>
            ) : null}
          </div>
          {row.usedFor.map((line) => (
            <p key={line} className="mt-0.5 text-idpxyz-breadcrumb">
              {line}
            </p>
          ))}
        </li>
      ))}
    </ul>
  );
}

function Content({ body, view }: { body: PriceCardContentFoundBody; view: PriceCardContentView }) {
  const technical = [
    { label: '方案标识', value: body.planId },
    { label: '版本', value: body.planVersion },
    { label: '规范化版本', value: body.canonicalization },
    { label: '内容摘要', value: body.contentDigest },
    ...view.technicalRefs,
    ...view.references.map((reference) => ({ label: reference.kindLabel, value: reference.name })),
    { label: '登记时间', value: formatInstant(body.registeredAt) },
    { label: '登记责任方', value: body.publicationApprover },
  ];
  return (
    <div className="space-y-5">
      {view.unsupportedCount > 0 ? (
        <p role="note" className="rounded border border-idpxyz-border border-l-2 border-l-[color:var(--ds-status-warning-fg)] px-3 py-2 text-[12px] text-idpxyz-text">
          有 {view.unsupportedCount} 项这版管理台暂不支持展示，已在下表里标出并附上原文；它们是这张卡的收费规则，不是没有。
        </p>
      ) : null}

      <section aria-label="费用项">
        <h3 className={sectionTitle}>费用项</h3>
        <FeeItemsTable items={view.feeItems} />
      </section>

      {view.weightRule !== null || view.amountRounding !== null ? (
        <section aria-label="重量规则">
          <h3 className={sectionTitle}>重量规则</h3>
          {view.weightRule !== null ? <p className="text-[12px] leading-5 text-idpxyz-text">{view.weightRule}</p> : null}
          {view.amountRounding !== null ? <p className="mt-1 text-[12px] leading-5 text-idpxyz-text">{view.amountRounding}。</p> : null}
        </section>
      ) : null}

      {view.refusals.length > 0 ? (
        <section aria-label="不接收的情形">
          <h3 className={sectionTitle}>不接收的情形</h3>
          <ul className="space-y-1 text-[12px] text-idpxyz-text">
            {view.refusals.map((refusal) => (
              <li key={`${refusal.clause}:${refusal.when}`}>
                {refusal.when}时不接收<span className="text-idpxyz-breadcrumb">（条款：{refusal.clause}）</span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {view.references.length > 0 ? (
        <section aria-label="引用的参考数据">
          <h3 className={sectionTitle}>引用的参考数据</h3>
          <ReferenceList rows={view.references} />
        </section>
      ) : null}

      <TechnicalDetails rows={technical} />
    </div>
  );
}

function Body({
  answer,
  view,
  retry,
}: {
  answer: ReturnType<typeof usePriceCardContent>['answer'];
  view: PriceCardContentView | null;
  retry: () => void;
}): ReactNode {
  if (answer === null) return <p className={note}>正在读取价卡内容…</p>;
  if (answer.kind === 'unconfigured') {
    return (
      <p className={note}>
        暂时读不到价卡内容：接入渠道还没有配置。这不代表这张价卡没有收费规则；配置好之后再看这一栏。
      </p>
    );
  }
  if (isOperatorRefusal(answer)) return <OperatorRefusalNote kind={answer.kind} what="价卡内容" className={note} />;
  if (answer.kind === 'callerProblem') {
    return (
      <div className={note}>
        <p>调用方式问题：{problemNote(answer.code)}</p>
        <TechnicalDetails rows={httpRows(answer.status, answer.code)} />
      </div>
    );
  }
  if (answer.kind === 'noAnswer') {
    return (
      <SectionError
        title="服务端没有形成答案"
        description={problemNote(answer.code)}
        technical={httpRows(answer.status, answer.code)}
        onRetry={retry}
      />
    );
  }
  if (answer.kind === 'transport') {
    return <SectionError title="无法连接主数据读取服务" description={answer.message} onRetry={retry} />;
  }
  switch (answer.body.outcome) {
    case 'NOT_FOUND':
      return <p className={note}>没有查到这一版价卡登记的内容。列表里有这一行而这里查无，请刷新列表后再看。</p>;
    case 'CANONICALIZATION_UNSUPPORTED':
      return (
        <p className={note}>
          这一版价卡是按另一种内部格式保存的，当前版本的管理台读不了它的内容。这不代表它没有收费规则；升级到支持该格式的版本后再看。
        </p>
      );
    case 'PRICE_CARD_CONTENT_FOUND':
      return view === null ? null : <Content body={answer.body} view={view} />;
  }
}

/** 币种与单位格：「人民币（CNY） · 千克」。内容没读到时诚实地写原因，不留空。 */
export function PriceCardUnits({ row }: { row: PriceCardRecord }) {
  const { answer } = usePriceCardContent(row);
  const view = useParsedView(answer);
  if (answer === null) return <span className="text-idpxyz-breadcrumb">读取中…</span>;
  if (view === null) return <span className="text-idpxyz-breadcrumb">未读到</span>;
  return (
    <span className="whitespace-nowrap">
      <CurrencyLabel code={view.header.currency} /> · {view.header.weightUnit}
    </span>
  );
}

function useParsedView(answer: ReturnType<typeof usePriceCardContent>['answer']): PriceCardContentView | null {
  return useMemo(() => {
    if (answer === null || answer.kind !== 'outcome' || answer.body.outcome !== 'PRICE_CARD_CONTENT_FOUND') return null;
    return parsePriceCardContent(answer.body.document);
  }, [answer]);
}

export function PriceCardDetailPanel({ row, onClose }: { row: PriceCardRecord; onClose: () => void }) {
  const { answer, retry } = usePriceCardContent(row);
  const view = useParsedView(answer);
  const status = priceCardStatusLabel(priceCardStatus(row, new Date()));
  const direction = priceDirectionLabels[row.direction] ?? row.direction;
  const purpose = priceCardPurposeLabels[row.purpose] ?? row.purpose;

  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`价卡 ${row.planId}`}>
      <div className="space-y-2 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="break-all text-[16px] font-semibold text-idpxyz-textBright">{row.planId}</h2>
              <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">版本 {row.planVersion}</span>
              <StatusPill label={status.label} tone={status.tone} />
            </div>
            <p className="mt-0.5 text-[12px] text-idpxyz-breadcrumb">
              {direction} · {purpose} · 适用范围 {row.scope}
            </p>
          </div>
          <Button
            className="shrink-0 text-idpxyz-breadcrumb"
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="关闭详情"
            title="关闭（Esc）"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>

      <FactStrip>
        <Fact label="币种">{view ? <CurrencyLabel code={view.header.currency} /> : '—'}</Fact>
        <Fact label="重量单位">{view ? view.header.weightUnit : '—'}</Fact>
        <Fact label="计价口径">{view ? view.header.aggregation : '—'}</Fact>
        <Fact label="适用期">
          <InstantRange from={row.effectiveFrom} to={row.effectiveTo} />
        </Fact>
      </FactStrip>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <Body answer={answer} view={view} retry={retry} />
      </div>
    </div>
  );
}
