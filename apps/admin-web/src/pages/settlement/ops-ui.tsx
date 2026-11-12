import type { ReactNode } from 'react';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import { formatMoney } from './money';

// 清关 · 账务 · 设置三区列表页共用的几个小件。状态胶囊、编码格与「技术详情」折叠都用 src/components 的共享件；
// 这里只放那几件之外、这三区的表格反复要用的小格：名称下挂编码的两行格、原币与结算币同格的金额格，以及把可缺席的
// 值筛成 TechnicalDetails 要的行。放在 settlement 目录是因为费用页第一个用到，customs / collection / governance 各页从这里引。

/** 可缺席的一行：缺席或空白不进折叠——缺席是「这一行上没有这件事」，不该在折叠里占一格空白。 */
export interface OptionalTechnicalRow {
  label: string;
  value: string | undefined;
}

export function technicalRows(rows: readonly OptionalTechnicalRow[]): TechnicalRow[] {
  return rows.filter((row): row is TechnicalRow => (row.value ?? '').trim() !== '');
}

/** 主字一行、次字一行的小格（如运单号下挂客户单号、名称下挂编码）。次字缺席只显主字。 */
export function TwoLine({ primary, secondary, mono }: { primary: ReactNode; secondary?: ReactNode; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <div className={`truncate text-idpxyz-textBright ${mono ? 'font-mono type-body leading-4' : ''}`.trim()}>{primary}</div>
      {secondary ? <div className="truncate font-mono type-caption text-idpxyz-textMuted">{secondary}</div> : null}
    </div>
  );
}

/** 金额与币种同格两行：上行原币，下行结算币；两者同币同额时只显一行，免得读者以为有换算。 */
export function AmountCell({
  originalAmount,
  originalCurrency,
  settlementAmount,
  settlementCurrency,
}: {
  originalAmount: string;
  originalCurrency: string;
  settlementAmount?: string;
  settlementCurrency?: string;
}) {
  const same =
    settlementAmount === undefined ||
    (settlementAmount === originalAmount && settlementCurrency === originalCurrency);
  return (
    <div className="text-right font-mono type-body leading-4 tabular-nums">
      <div className="text-idpxyz-textBright">{formatMoney(originalAmount, originalCurrency)}</div>
      {same ? null : (
        <div className="text-idpxyz-textMuted">结算 {formatMoney(settlementAmount, settlementCurrency ?? '')}</div>
      )}
    </div>
  );
}

// 写法本身在 money.ts；各页照旧从这里引。
export { formatMoney };
