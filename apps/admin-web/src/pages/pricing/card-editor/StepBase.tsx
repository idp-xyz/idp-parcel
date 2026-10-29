import { Button } from '@idpxyz/ui-primitives';
import { Plus, Trash2 } from 'lucide-react';
import { rateTableFamilyLabels } from '../../../vocabulary/price-card';
import { field } from '../card-form/fields';
import { newRateRow, type RateRow } from '../card-form/model';
import { allowedValues, isTextColumn, rateColumnKind, rateColumns, sheetNames } from '../card-form/spec';
import type { PriceCardSpecColumn } from '../api';
import { CellInput, GroupErrors, SelectField, SectionTitle, hintClass } from './controls';
import { useEditor } from './EditorContext';
import { labelled, unitWord } from './options';

const familyHints: Readonly<Record<string, string>> = {
  FIRST_CONTINUE: '首重收一笔，超出的部分每满一个续重步长再加一笔，不足一步按一步算。',
  WEIGHT_ZONE: '同一分区里按重量段各给一个金额；重量段左闭右开，最后一段的上界留空表示不封顶。',
  UNIT_PRICE: '计价重量乘以每个重量单位的单价。',
};

/** 价表行的可增删小表格。列取自模板规格口对应表，分区名之外的列都按数字校验。 */
export function RateGrid({
  columns,
  rows,
  cellId,
  onChange,
  onAdd,
  onRemove,
  keyLabel = '分区',
  errorIds,
}: {
  columns: readonly PriceCardSpecColumn[];
  rows: readonly RateRow[];
  cellId: (rowUid: string, column: string) => string;
  onChange: (rowUid: string, column: string, value: string) => void;
  onAdd: () => void;
  onRemove: (rowUid: string) => void;
  keyLabel?: string;
  errorIds: readonly string[];
}) {
  const { state, env } = useEditor();
  const unitFor = (key: string) => {
    const kind = rateColumnKind(key);
    if (kind === 'weight' && state.basics.weightUnit !== '') return `（${unitWord(state.basics.weightUnit, env)}）`;
    if (kind === 'money' && state.basics.currency !== '') return `（${state.basics.currency}）`;
    return '';
  };
  return (
    <div>
      <div className="overflow-x-auto rounded border border-idpxyz-border">
        <table className="w-full min-w-[520px] text-[12px]">
          <thead className="bg-idpxyz-sidebar text-left text-idpxyz-breadcrumb">
            <tr>
              <th className="w-10 px-2 py-1.5 font-medium">#</th>
              {columns.map((column) => (
                <th key={column.key} className="px-2 py-1.5 font-medium">
                  {isTextColumn(column.key) ? keyLabel : column.title}
                  {unitFor(column.key)}
                  {column.requirement === 'optional' ? <span className="font-normal">（可空）</span> : null}
                </th>
              ))}
              <th className="w-10 px-2 py-1.5" />
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => (
              <tr key={row.uid} className="border-t border-idpxyz-border">
                <td className="px-2 py-1 text-idpxyz-breadcrumb">{index + 1}</td>
                {columns.map((column) => (
                  <td key={column.key} className="px-1 py-1">
                    <CellInput
                      id={cellId(row.uid, column.key)}
                      label={`第 ${index + 1} 行 ${isTextColumn(column.key) ? keyLabel : column.title}`}
                      mono={!isTextColumn(column.key)}
                      value={row.cells[column.key] ?? ''}
                      onChange={(value) => onChange(row.uid, column.key, value)}
                    />
                  </td>
                ))}
                <td className="px-1 py-1 text-center">
                  <Button variant="ghost" size="icon" aria-label={`删除第 ${index + 1} 行`} title="删除这一行" onClick={() => onRemove(row.uid)}>
                    <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <GroupErrors ids={errorIds} />
      <Button variant="outline" size="sm" className="mt-2" onClick={onAdd}>
        <Plus className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
        添加一行
      </Button>
    </div>
  );
}

export function StepBase() {
  const { state, update, spec, env } = useEditor();
  const { base } = state;
  const columns = rateColumns(spec, base.family);
  const ids = [field.base('rows'), ...base.rows.flatMap((row) => columns.map((column) => field.baseRow(row.uid, column.key)))];

  return (
    <div className="space-y-5">
      <SectionTitle hint="每个包裹的基础运费怎么算。其他加收的费用在下一步之后添加。">基础运费</SectionTitle>
      <SelectField
        id={field.base('family')}
        label="价表类型"
        required
        className="max-w-sm"
        value={base.family}
        options={labelled(allowedValues(spec, sheetNames.tables, 'family'), rateTableFamilyLabels)}
        onChange={(family) => update((draft) => ({ ...draft, base: { family, rows: family === draft.base.family ? draft.base.rows : [newRateRow()] } }))}
        hint={familyHints[base.family] ?? '换类型会清空下面已填的价表行'}
      />
      {base.family !== '' ? (
        <div>
          <p className={`${hintClass} mb-2`}>
            价表行：金额按{state.basics.currency || '所选币种'}，重量按{state.basics.weightUnit === '' ? '所选重量单位' : unitWord(state.basics.weightUnit, env)}填，只写数字。
          </p>
          <RateGrid
            columns={columns}
            rows={base.rows}
            errorIds={ids}
            cellId={field.baseRow}
            onChange={(rowUid, column, value) =>
              update((draft) => ({
                ...draft,
                base: { ...draft.base, rows: draft.base.rows.map((row) => (row.uid === rowUid ? { ...row, cells: { ...row.cells, [column]: value } } : row)) },
              }))
            }
            onAdd={() => update((draft) => ({ ...draft, base: { ...draft.base, rows: [...draft.base.rows, newRateRow()] } }))}
            onRemove={(rowUid) => update((draft) => ({ ...draft, base: { ...draft.base, rows: draft.base.rows.filter((row) => row.uid !== rowUid) } }))}
          />
          <div data-field={field.base('rows')} tabIndex={-1} />
        </div>
      ) : null}
    </div>
  );
}
