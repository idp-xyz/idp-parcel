import type { ReactNode } from 'react';
import { Input } from '@idpxyz/ui-primitives';
import { OperatorRefusalNote } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { deliveryScopeReadProblemNote, type DeliveryScopeListFilter } from './delivery-pre-advice';

/**
 * 送站预报区与送站交接单区共用的筛选条：区切换的 chip、单据号、节点与日子。日子用浏览器的日期控件，换成哪两个瞬间在 delivery-pre-advice
 * 的 localDayBoundsOf——一日按看的人所在的时区起算。单据号交服务端逐字找，不是在这一页里搜。
 */
export function DeliveryScopeFilters({
  chips,
  filter,
  dayLabel,
  numberLabel,
  onChange,
}: {
  chips?: ReactNode;
  filter: DeliveryScopeListFilter;
  dayLabel: string;
  numberLabel: string;
  onChange: (filter: DeliveryScopeListFilter) => void;
}) {
  return (
    <>
      {chips}
      <Input
        value={filter.documentNumber ?? ''}
        className="h-8 w-[200px] font-mono text-[12px]"
        placeholder={`${numberLabel}（逐字，空即不限）`}
        aria-label={numberLabel}
        onChange={(event) => onChange({ ...filter, documentNumber: event.target.value })}
      />
      <Input
        value={filter.node}
        className="h-8 w-[200px] font-mono text-[12px]"
        placeholder="网点编码（空即全部网点）"
        aria-label="网点"
        onChange={(event) => onChange({ ...filter, node: event.target.value })}
      />
      <label className="inline-flex items-center gap-1 text-xs text-idpxyz-textMuted">
        {dayLabel}
        <input
          type="date"
          className="h-8 rounded border border-idpxyz-border bg-transparent px-1 font-mono text-[12px]"
          value={filter.day}
          onChange={(event) => onChange({ ...filter, day: event.target.value })}
        />
      </label>
    </>
  );
}

/** 详情读（按标识读一份）没拿到业务答案时的那一句：未配置、未授予、看不见、没形成答案、没到达各写各的。 */
export function DeliveryScopeReadNote({ answer, what }: { answer: Exclude<ApiResult<unknown>, { kind: 'outcome' }>; what: string }) {
  const frame = 'text-xs';
  switch (answer.kind) {
    case 'unconfigured':
      return <p className={`${frame} text-idpxyz-textMuted`}>查看入口还没有配置好，重试也不会改变结果；请联系系统管理员完成配置。</p>;
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what={what} className={`${frame} text-idpxyz-textMuted`} />;
    case 'callerProblem':
      return (
        <p className={`${frame} text-idpxyz-danger`}>
          没有读出来：{deliveryScopeReadProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className={`${frame} text-idpxyz-danger`}>
          暂时没有读出来：{deliveryScopeReadProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className={`${frame} text-idpxyz-danger`}>连不上服务，请检查网络后重试（{answer.message}）</p>;
  }
}
