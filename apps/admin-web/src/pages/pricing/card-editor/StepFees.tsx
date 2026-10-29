import { useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { Plus, Trash2 } from 'lucide-react';
import { field } from '../card-form/fields';
import { feeConditionText } from '../card-form/describe';
import {
  feeHasConditionBuilder,
  feeHasGroup,
  feeKindHints,
  feeKindLabels,
  feeKindOrder,
  newFee,
  newRateRow,
  suggestChargeCode,
  type FeeItemState,
  type FeeKind,
} from '../card-form/model';
import { remoteTiers } from '../card-form/workbook';
import { CellInput, FieldBox, GroupErrors, SelectControl, SelectField, TextField, hintClass, useFieldErrors, type Option } from './controls';
import { ConditionBuilder } from './ConditionBuilder';
import { useEditor } from './EditorContext';
import { RateGrid } from './StepBase';
import type { PriceCardSpecColumn } from '../api';

const newGroupValue = '__new__';

function tableColumns(kind: FeeKind): PriceCardSpecColumn[] {
  const keyTitle = kind === 'TABLE_REMOTE' ? '档位' : '分区';
  return [
    { key: 'zone', title: keyTitle, requirement: 'always', description: '' },
    { key: 'minimum', title: '重量从', requirement: 'always', description: '' },
    { key: 'maximum', title: '重量到', requirement: 'optional', description: '' },
    { key: 'amount', title: '金额', requirement: 'always', description: '' },
  ];
}

function FeeCard({ fee, index }: { fee: FeeItemState; index: number }) {
  const { state, update, env } = useEditor();
  const [creatingGroup, setCreatingGroup] = useState(false);
  const patch = (change: Partial<FeeItemState>) =>
    update((draft) => ({ ...draft, fees: draft.fees.map((item) => (item.uid === fee.uid ? { ...item, ...change } : item)) }));
  const otherCodes = state.fees.filter((item) => item.uid !== fee.uid).map((item) => item.code);
  const groups = [...new Set(state.fees.filter((item) => item.uid !== fee.uid && item.groupName.trim() !== '').map((item) => item.groupName.trim()))];
  const groupValue = creatingGroup ? newGroupValue : fee.groupName;
  const groupOptions: Option[] = [
    ...groups.map((name) => ({ value: name, label: name })),
    ...(fee.groupName !== '' && !groups.includes(fee.groupName) && !creatingGroup ? [{ value: fee.groupName, label: fee.groupName }] : []),
    { value: newGroupValue, label: '＋ 新建互斥组…' },
  ];
  const keyed = fee.kind === 'TABLE_REMOTE' || fee.kind === 'TABLE_ZONE';
  const rowIds = fee.tableRows.flatMap((row) => ['zone', 'minimum', 'maximum', 'amount'].map((column) => field.feeRow(fee.uid, row.uid, column)));
  const remoteSentence = fee.kind === 'TABLE_REMOTE' ? feeConditionText(fee) : null;
  const groupErrors = useFieldErrors(field.fee(fee.uid, 'group'));

  return (
    <section className="rounded border border-idpxyz-border p-3" aria-label={`费用项 ${index + 1}`} data-field={field.fee(fee.uid, 'card')} tabIndex={-1}>
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">费用项 {index + 1}</span>
          <span className="truncate text-[13px] font-medium text-idpxyz-textBright">{fee.name.trim() || '（还没起名）'}</span>
          <span className="text-[12px] text-idpxyz-breadcrumb">{feeKindLabels[fee.kind]}</span>
        </div>
        <Button variant="ghost" size="sm" className="text-idpxyz-breadcrumb" onClick={() => update((draft) => ({ ...draft, fees: draft.fees.filter((item) => item.uid !== fee.uid) }))}>
          <Trash2 className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
          删除这一项
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        <TextField
          id={field.fee(fee.uid, 'name')}
          label="费用名称"
          required
          value={fee.name}
          placeholder="如 超重费"
          onChange={(name) => patch({ name, code: fee.codeEdited ? fee.code : suggestChargeCode(fee.kind, name, otherCodes) })}
          hint="账单和报价上显示的名字"
        />
        <TextField
          id={field.fee(fee.uid, 'code')}
          label="费用代码"
          required
          mono
          value={fee.code}
          onChange={(code) => patch({ code: code.toUpperCase().replace(/[\s-]+/g, '_'), codeEdited: true })}
          hint={fee.codeEdited ? '大写字母、数字和下划线' : '已按名称建议一个，可以改'}
        />
      </div>

      <div className="mt-3 space-y-3">
        {fee.kind === 'FIXED' || fee.kind === 'CONDITIONAL' ? (
          <TextField
            id={field.fee(fee.uid, 'amount')}
            label={fee.kind === 'FIXED' ? '每件固定金额' : '收取金额'}
            required
            mono
            inputMode="decimal"
            className="max-w-xs"
            value={fee.amount}
            onChange={(amount) => patch({ amount })}
            hint={`金额按${state.basics.currency || '所选币种'}，只写数字`}
          />
        ) : null}

        {fee.kind === 'PERCENT' ? (
          <div className="space-y-3">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <SelectField
                id={field.fee(fee.uid, 'percentMode')}
                label="比例从哪里来"
                value={fee.percentMode}
                options={[
                  { value: 'SERIES', label: '已登记的燃油费率序列' },
                  { value: 'FIXED', label: '固定百分比' },
                ]}
                onChange={(percentMode) => patch({ percentMode: percentMode as 'SERIES' | 'FIXED' })}
              />
              <SelectField
                id={field.fee(fee.uid, 'basis')}
                label="按什么算"
                value={fee.basis}
                options={[
                  { value: 'BASE', label: '基础运费' },
                  { value: 'ALL', label: '其他已计收费用之和' },
                ]}
                onChange={(basis) => patch({ basis: basis as 'BASE' | 'ALL' })}
              />
            </div>
            {fee.percentMode === 'SERIES' ? (
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                {env.fuelSeries !== null && env.fuelSeries.length > 0 ? (
                  <SelectField
                    id={field.fee(fee.uid, 'seriesId')}
                    label="燃油费率序列"
                    required
                    value={fee.seriesId}
                    options={env.fuelSeries.map((id) => ({ value: id, label: id }))}
                    onChange={(seriesId) => patch({ seriesId })}
                    hint="当期费率随序列更新，价卡不用跟着改"
                  />
                ) : (
                  <TextField
                    id={field.fee(fee.uid, 'seriesId')}
                    label="燃油费率序列"
                    required
                    mono
                    value={fee.seriesId}
                    onChange={(seriesId) => patch({ seriesId })}
                    hint={env.fuelSeries === null ? '序列清单暂时读不到，请直接填序列标识' : '还没有登记燃油费率序列，请先去「汇率与燃油费率」登记，或直接填序列标识'}
                  />
                )}
                <TextField
                  id={field.fee(fee.uid, 'seriesFactor')}
                  label="系数"
                  required
                  mono
                  inputMode="decimal"
                  value={fee.seriesFactor}
                  onChange={(seriesFactor) => patch({ seriesFactor })}
                  hint="乘在当期费率上；1 就是原样用"
                />
              </div>
            ) : (
              <TextField
                id={field.fee(fee.uid, 'percentage')}
                label="百分比"
                required
                mono
                inputMode="decimal"
                className="max-w-xs"
                value={fee.percentage}
                onChange={(percentage) => patch({ percentage })}
                hint="填 12.5 就是 12.5%"
              />
            )}
          </div>
        ) : null}

        {keyed ? (
          <div data-field={field.fee(fee.uid, 'rows')} tabIndex={-1}>
            <span className="mb-1 block text-[12px] font-medium text-idpxyz-textBright">
              {fee.kind === 'TABLE_REMOTE' ? '偏远档位与金额' : '分区、重量段与金额'}
            </span>
            <p className={`${hintClass} mb-2`}>
              {fee.kind === 'TABLE_REMOTE'
                ? '每个档位一行，如 T1 填 12、T2 填 28；重量从 0 起、「重量到」留空就是不分重量段。收件地址落在哪一档由第六步选的偏远档位目录判定。'
                : '同一分区可以有多行，按重量段各给一个金额；重量段左闭右开，最后一段的「重量到」留空。'}
            </p>
            <RateGrid
              columns={tableColumns(fee.kind)}
              rows={fee.tableRows}
              errorIds={[field.fee(fee.uid, 'rows'), ...rowIds]}
              keyLabel={fee.kind === 'TABLE_REMOTE' ? '档位' : '分区'}
              cellId={(rowUid, column) => field.feeRow(fee.uid, rowUid, column)}
              onChange={(rowUid, column, value) =>
                patch({ tableRows: fee.tableRows.map((row) => (row.uid === rowUid ? { ...row, cells: { ...row.cells, [column]: value } } : row)) })
              }
              onAdd={() => patch({ tableRows: [...fee.tableRows, newRateRow(fee.kind === 'TABLE_REMOTE' ? { minimum: '0' } : {})] })}
              onRemove={(rowUid) => patch({ tableRows: fee.tableRows.filter((row) => row.uid !== rowUid) })}
            />
          </div>
        ) : null}

        {feeHasConditionBuilder(fee.kind) ? <ConditionBuilder fee={fee} /> : null}
        {fee.kind === 'TABLE_REMOTE' ? (
          <div>
            <span className="text-[12px] font-medium text-idpxyz-textBright">什么时候收</span>
            <p className="mt-0.5 text-[12px] text-idpxyz-text">
              {remoteSentence !== null && remoteTiers(fee).length > 0 ? `${remoteSentence}时收取（由上面价表里的档位自动得出）` : '收件地址落在价表里任一档位时收取（填了档位后这里会显示）'}
            </p>
            <GroupErrors ids={[field.fee(fee.uid, 'conditions')]} />
          </div>
        ) : null}
        {fee.kind === 'FIXED' ? <p className={hintClass}>固定金额不看条件：每件都收。</p> : null}

        {feeHasGroup(fee.kind) ? (
          <div className="grid grid-cols-1 gap-3 border-t border-idpxyz-border pt-3 md:grid-cols-2">
            <FieldBox
              id={field.fee(fee.uid, 'group')}
              label="与其他费用互斥"
              hint="同一互斥组里的费用同时命中时只收一项：先比优先级（1 最高），同级收金额较高的；不互斥就各收各的。"
            >
              <SelectControl
                id={field.fee(fee.uid, 'group')}
                label="互斥组"
                value={groupValue}
                placeholder="不互斥，各收各的"
                invalid={groupErrors.length > 0}
                options={groupOptions}
                onChange={(value) => {
                  if (value === newGroupValue) {
                    setCreatingGroup(true);
                    patch({ groupName: '' });
                  } else {
                    setCreatingGroup(false);
                    patch({ groupName: value, priority: value !== '' && fee.priority === '' ? '1' : fee.priority });
                  }
                }}
              />
              {creatingGroup ? (
                <div className="mt-2">
                  <CellInput
                    id={`${field.fee(fee.uid, 'group')}.name`}
                    label="新互斥组的名字"
                    mono={false}
                    placeholder="给这一组起个名，如 超尺寸超重"
                    value={fee.groupName}
                    onChange={(groupName) => patch({ groupName, priority: groupName.trim() !== '' && fee.priority === '' ? '1' : fee.priority })}
                  />
                </div>
              ) : null}
            </FieldBox>
            {fee.groupName.trim() !== '' ? (
              <TextField
                id={field.fee(fee.uid, 'priority')}
                label="组内优先级"
                mono
                inputMode="numeric"
                className="max-w-[10rem]"
                value={fee.priority}
                onChange={(priority) => patch({ priority })}
                hint="整数，1 最高"
              />
            ) : null}
          </div>
        ) : null}
      </div>
    </section>
  );
}

export function StepFees() {
  const { state, update } = useEditor();
  const [choosing, setChoosing] = useState(state.fees.length === 0);

  return (
    <div className="space-y-4">
      {state.fees.length === 0 ? <p className="text-[12px] text-idpxyz-text">没有其他费用就直接下一步。</p> : null}
      {state.fees.map((fee, index) => (
        <FeeCard key={fee.uid} fee={fee} index={index} />
      ))}
      {choosing ? (
        <div className="rounded border border-dashed border-idpxyz-border p-3" role="group" aria-label="选择费用项类型">
          <p className="mb-2 text-[12px] font-medium text-idpxyz-textBright">加哪一种费用？</p>
          <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
            {feeKindOrder.map((kind) => (
              <button
                key={kind}
                type="button"
                className="rounded border border-idpxyz-border px-3 py-2 text-left hover:border-idpxyz-accent hover:bg-idpxyz-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-idpxyz-accent"
                onClick={() => {
                  update((draft) => ({ ...draft, fees: [...draft.fees, newFee(kind, draft.basics.weightUnit)] }));
                  setChoosing(false);
                }}
              >
                <span className="block text-[13px] font-medium text-idpxyz-textBright">{feeKindLabels[kind]}</span>
                <span className="block text-[12px] text-idpxyz-breadcrumb">{feeKindHints[kind]}</span>
              </button>
            ))}
          </div>
          {state.fees.length > 0 ? (
            <Button variant="ghost" size="sm" className="mt-2 text-idpxyz-breadcrumb" onClick={() => setChoosing(false)}>
              先不加了
            </Button>
          ) : null}
        </div>
      ) : (
        <Button variant="outline" onClick={() => setChoosing(true)}>
          <Plus className="mr-1 h-4 w-4" aria-hidden="true" />
          添加费用项
        </Button>
      )}
    </div>
  );
}
