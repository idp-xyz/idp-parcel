import { Button } from '@idpxyz/ui-primitives';
import { Plus, Trash2 } from 'lucide-react';
import { comparisonLabels, featureLabels, lengthUnitLabels, volumeUnitLabels } from '../../../vocabulary/price-card';
import { conditionRowText, feeConditionText } from '../card-form/describe';
import { conditionCategoryChoices } from '../card-form/options';
import { field } from '../card-form/fields';
import { newCondition, type ConditionRow, type FeeItemState } from '../card-form/model';
import { allowedValues, sheetNames } from '../card-form/spec';
import { featureMeasure, type Measure } from '../price-card-content';
import { CellInput, GroupErrors, SelectControl, hintClass, useFieldErrors, type Option } from './controls';
import { useEditor } from './EditorContext';
import { labelled, lengthUnitOptions, weightUnitOptions } from './options';

const weightOnly = new Set(['VOLUMETRIC_WEIGHT', 'CHARGEABLE_WEIGHT']);

function ControlWithErrors({ id, children }: { id: string; children: (invalid: boolean) => React.ReactNode }) {
  return <>{children(useFieldErrors(id).length > 0)}</>;
}

/** 触发条件构造器：每行「特征 · 比较 · 阈值 · 单位」，行之间选全部成立或任一成立，行末实时念成人话。 */
export function ConditionBuilder({ fee }: { fee: FeeItemState }) {
  const { state, update, spec, env } = useEditor();
  const sources = allowedValues(spec, sheetNames.conditions, 'source');
  const operators = allowedValues(spec, sheetNames.conditions, 'operator');
  const patchFee = (change: Partial<FeeItemState>) =>
    update((draft) => ({ ...draft, fees: draft.fees.map((item) => (item.uid === fee.uid ? { ...item, ...change } : item)) }));
  const patchRow = (rowUid: string, change: Partial<ConditionRow>) =>
    patchFee({ conditions: fee.conditions.map((row) => (row.uid === rowUid ? { ...row, ...change } : row)) });

  const numericOperators: Option[] = operators.filter((code) => code !== 'EQ').map((code) => ({ value: code, label: comparisonLabels[code] ?? code }));
  const weightUnits = weightUnitOptions(spec, env);
  const lengthUnits = lengthUnitOptions(spec, env);
  const unitOptionsFor = (source: string): Option[] => {
    const measure: Measure | undefined = featureMeasure(source);
    if (measure === 'weight') {
      return weightOnly.has(source) ? weightUnits.filter((unit) => unit.value === state.basics.weightUnit) : weightUnits;
    }
    if (measure === 'volume') return lengthUnits.map((unit) => ({ value: unit.value, label: volumeUnitLabels[unit.value] ?? unit.label }));
    if (measure === 'length') return lengthUnits.map((unit) => ({ value: unit.value, label: lengthUnitLabels[unit.value] ?? unit.label }));
    return [];
  };

  const onSource = (row: ConditionRow, source: string) => {
    const measure = featureMeasure(source);
    const wasCategory = featureMeasure(row.source) === 'category';
    const units = unitOptionsFor(source);
    const defaultUnit =
      measure === 'weight'
        ? state.basics.weightUnit
        : measure === 'length' || measure === 'volume'
          ? state.weight.lengthUnit
          : '';
    patchRow(row.uid, {
      source,
      operator: measure === 'category' ? 'EQ' : row.operator === 'EQ' || row.operator === '' ? 'GT' : row.operator,
      value: measure === 'category' !== wasCategory || row.source === '' ? '' : row.value,
      unit: units.some((unit) => unit.value === defaultUnit) ? defaultUnit : units.length === 1 ? units[0].value : '',
    });
  };

  const sentence = feeConditionText(fee);
  const ids = fee.conditions.flatMap((row) => [
    field.condition(fee.uid, row.uid, 'source'),
    field.condition(fee.uid, row.uid, 'operator'),
    field.condition(fee.uid, row.uid, 'value'),
    field.condition(fee.uid, row.uid, 'unit'),
  ]);

  return (
    <div data-field={field.fee(fee.uid, 'conditions')} tabIndex={-1}>
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <span className="text-[12px] font-medium text-idpxyz-textBright">什么时候收（触发条件）</span>
        {fee.conditions.length > 1 ? (
          <label className="flex items-center gap-1.5 text-[12px] text-idpxyz-breadcrumb">
            下面几条
            <SelectControl
              id={field.fee(fee.uid, 'combine')}
              label="条件之间的关系"
              value={fee.combine}
              placeholder="请选择"
              options={[
                { value: 'ALL_OF', label: '全部成立才收' },
                { value: 'ANY_OF', label: '任一成立就收' },
              ]}
              onChange={(combine) => patchFee({ combine: combine as 'ALL_OF' | 'ANY_OF' })}
            />
          </label>
        ) : null}
      </div>
      <div className="space-y-2">
        {fee.conditions.map((row, index) => {
          const measure = featureMeasure(row.source);
          const category = measure === 'category';
          const categoryChoices = category ? conditionCategoryChoices(spec, row.source) : null;
          const text = conditionRowText(row);
          const units = unitOptionsFor(row.source);
          return (
            <div key={row.uid} className="rounded border border-idpxyz-border p-2">
              <div className="flex flex-wrap items-start gap-2">
                <span className="w-12 pt-1.5 text-[12px] text-idpxyz-breadcrumb">条件 {index + 1}</span>
                <div className="w-44">
                  <ControlWithErrors id={field.condition(fee.uid, row.uid, 'source')}>
                    {(invalid) => (
                      <SelectControl
                        id={field.condition(fee.uid, row.uid, 'source')}
                        label={`条件 ${index + 1} 的特征`}
                        value={row.source}
                        invalid={invalid}
                        placeholder="选特征"
                        options={labelled(sources, featureLabels)}
                        onChange={(source) => onSource(row, source)}
                      />
                    )}
                  </ControlWithErrors>
                </div>
                {row.source === '' ? null : category ? (
                  <>
                    <span className="pt-1.5 text-[12px] text-idpxyz-text">{comparisonLabels.EQ}</span>
                    <div className="w-36">
                      {categoryChoices !== null ? (
                        <ControlWithErrors id={field.condition(fee.uid, row.uid, 'value')}>
                          {(invalid) => (
                            <SelectControl
                              id={field.condition(fee.uid, row.uid, 'value')}
                              label="类别"
                              value={row.value}
                              invalid={invalid}
                              options={categoryChoices}
                              onChange={(value) => patchRow(row.uid, { value })}
                            />
                          )}
                        </ControlWithErrors>
                      ) : (
                        <CellInput
                          id={field.condition(fee.uid, row.uid, 'value')}
                          label="取值"
                          mono={false}
                          placeholder={row.source === 'ZONE' ? '分区，如 Z1' : row.source === 'ADDRESS_TYPE' ? '档位，如 T1' : '服务选项'}
                          value={row.value}
                          onChange={(value) => patchRow(row.uid, { value })}
                        />
                      )}
                    </div>
                  </>
                ) : (
                  <>
                    <div className="w-28">
                      <ControlWithErrors id={field.condition(fee.uid, row.uid, 'operator')}>
                        {(invalid) => (
                          <SelectControl
                            id={field.condition(fee.uid, row.uid, 'operator')}
                            label="比较"
                            value={row.operator}
                            invalid={invalid}
                            options={numericOperators}
                            onChange={(operator) => patchRow(row.uid, { operator })}
                          />
                        )}
                      </ControlWithErrors>
                    </div>
                    <div className="w-28">
                      <CellInput id={field.condition(fee.uid, row.uid, 'value')} label="比较值" placeholder="数值" value={row.value} onChange={(value) => patchRow(row.uid, { value })} />
                    </div>
                    <div className="w-36">
                      <ControlWithErrors id={field.condition(fee.uid, row.uid, 'unit')}>
                        {(invalid) => (
                          <SelectControl
                            id={field.condition(fee.uid, row.uid, 'unit')}
                            label="单位"
                            value={row.unit}
                            invalid={invalid}
                            placeholder="选单位"
                            options={units}
                            onChange={(unit) => patchRow(row.uid, { unit })}
                          />
                        )}
                      </ControlWithErrors>
                    </div>
                  </>
                )}
                <span className="min-w-0 flex-1 basis-40 pt-1.5 text-[12px] text-idpxyz-breadcrumb">{text === null ? '' : `→ ${text}`}</span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`删除条件 ${index + 1}`}
                  title="删除这条条件"
                  disabled={fee.conditions.length <= 1}
                  onClick={() => patchFee({ conditions: fee.conditions.filter((item) => item.uid !== row.uid) })}
                >
                  <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                </Button>
              </div>
            </div>
          );
        })}
      </div>
      <GroupErrors ids={[field.fee(fee.uid, 'conditions'), field.fee(fee.uid, 'combine'), ...ids]} />
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <Button variant="outline" size="sm" onClick={() => patchFee({ conditions: [...fee.conditions, newCondition()] })}>
          <Plus className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
          添加条件
        </Button>
        {sentence === null ? null : <span className="text-[12px] text-idpxyz-text">合起来：{sentence}时收取</span>}
      </div>
      <p className={`${hintClass} mt-1`}>想表示「每件都收」，保留「实重大于 0」这一条即可。</p>
    </div>
  );
}
