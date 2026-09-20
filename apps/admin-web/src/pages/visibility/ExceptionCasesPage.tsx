import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import {
  StatusBadgeFor,
  domainStatusTones,
  type DomainStatus,
} from '../../domain/status';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listExceptionCaseRecords,
  type ApiResult,
  type ExceptionCaseListResponseBody,
} from './case-api';
import { caseLabelOf, casePhaseLabels } from './case-presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-cases'];

/**
 * 异常案件册的列表行（GET /exception-case-records，票 admin-skeleton-closure-batch/06）。
 *
 * 骨架期的「当前工作条件」「严重度」「处置优先级」「响应周期」四列不在本表：案件行
 * 在存储上刻意只落精简主生命周期（0008），那四键结构上不存在——列出来只能代填，
 * 代填会把工作条件升格成第二套主状态（票 06 Comments 记明）。那几维登记落地后随
 * 查询契约扩列。
 */
interface CaseRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

// 主状态词（待响应/处理中/已关闭）与共享词表同词，按词表着色；词表没收录的词
// 原样示文，不猜色调。
function toneWordOrText(value: string) {
  return value in domainStatusTones ? (
    <StatusBadgeFor status={value as DomainStatus} />
  ) : (
    value
  );
}

function col(
  id: string,
  header: string,
  options?: {
    mono?: boolean;
    align?: 'left' | 'center' | 'right';
    className?: string;
    toneWord?: boolean;
  },
): ListColumn<CaseRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) => {
      const value = row.values[id] ?? '—';
      if (options?.toneWord) return toneWordOrText(value);
      if (options?.mono) return <span className="font-mono text-[12px]">{value}</span>;
      return value;
    },
  };
}

// 关闭结论只随已关闭在场（0008 成对约束）；归并指向在场即受控归并——被并入的
// 案件不从册面消失，原编号照列（归并是登记事实不是删除）。
const columns: ListColumn<CaseRow>[] = [
  col('caseId', '案件标识', { mono: true }),
  col('rootParcel', '根对象', { mono: true }),
  col('impactScope', '影响范围', { mono: true }),
  col('phase', '主状态', { toneWord: true, className: 'w-[96px]' }),
  col('responsibleTeam', '责任团队', { mono: true }),
  col('establishedAt', '建立时间', { mono: true }),
  col('firstResponse', '首次响应', { mono: true }),
  col('closedAt', '关闭时间', { mono: true }),
  col('conclusion', '关闭结论', { mono: true }),
  col('mergedInto', '归并指向', { mono: true }),
];

// 「导出所选（CSV）」按列取字：行上 values 已是按列 id 备好的字面量（主状态那格是词表词，与徽章同词），直接取；
// 没登记的格导出为空，不代填「—」——那个破折号是屏幕上的留白记号，不是数据。
const csvCellText = (row: CaseRow, column: ListColumn<CaseRow>) => row.values[column.id] ?? '';

/**
 * 异常案件（visibility-exception）。行对象是围绕同一因果链和处置范围建立的
 * 业务案件。案件关闭不修改源事实、不解除来源限制；查阅不推进阶段、不合并、
 * 不关闭——那些是案件命令面的判断，本页只消费存储读面。
 */
export function ExceptionCasesPage() {
  const [search, setSearch] = useState('');
  // 多选集（票 admin-web-workspace-form/04）：按行键记，改检索词不清。批量动作只有导出所选——案件的归并 / 关闭是命令面的判断，
  // 本仓今天没有对一批案件的命令端点，不传 extra。
  const [checked, setChecked] = useState<ReadonlySet<string>>(() => new Set());
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ExceptionCaseListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listExceptionCaseRecords().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows: CaseRow[] = body
    ? body.cases.map((record) => ({
        key: `case:${record.caseId}`,
        values: {
          caseId: record.caseId,
          rootParcel: record.rootParcel,
          impactScope: record.impactScope,
          phase: caseLabelOf(casePhaseLabels, record.phase),
          responsibleTeam: record.responsibleTeam,
          establishedAt: formatInstant(record.establishedAt),
          firstResponse: record.firstResponse ? formatInstant(record.firstResponse) : '',
          closedAt: record.closedAt ? formatInstant(record.closedAt) : '',
          conclusion: record.conclusion ?? '',
          mergedInto: record.mergedInto ?? '',
        },
      }))
    : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CaseRow>
      title={info.title}
      // 页头携带归并与重开的共同底线：两者都以追加表达，不删除历史。
      description={`${info.owner}——受控归并以「已归并」关闭并关联主案件（原编号照列），均不删除历史；严重度、优先级与工作条件在案件行上无登记格，本表不代填`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索案件标识 / 根对象',
      }}
      filterSummary={body ? `异常案件 ${visibleRows.length} 行` : undefined}
      columns={columns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      selection={{ selected: checked, onChange: setChecked }}
      bulkActions={{ csv: { fileName: 'exception-cases.csv', cellText: csvCellText } }}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /exception-case-records',
        emptyTitle: '当前租户尚无异常案件',
        emptyDescription:
          '读取入口已配置，登记册为空——案件由信号、分诊、建案的编排形成，事实在接入渠道墙后面，空册是预期不是缺陷。',
      })}
    />
  );
}
