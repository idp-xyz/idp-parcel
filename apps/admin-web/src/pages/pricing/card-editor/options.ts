import type { PriceCardTemplateSpec } from '../api';
import { allowedValues, sheetNames } from '../card-form/spec';
import { lengthUnitLabels, weightUnitLabels } from '../../../vocabulary/price-card';
import type { Option } from './controls';
import type { EditorEnv } from './use-editor-env';

/** 规格口给的可取值 → 下拉选项；中文译法取自词典，词典没有的取值以码原样显示。 */
export function labelled(codes: readonly string[], labels: Readonly<Record<string, string>>): Option[] {
  return codes.map((code) => ({ value: code, label: labels[code] ?? code }));
}

/** 重量单位：可取值来自规格口，中文名优先取单位目录，目录读不到就用词典。 */
export function weightUnitOptions(spec: PriceCardTemplateSpec, env: EditorEnv): Option[] {
  return allowedValues(spec, sheetNames.tables, 'weightUnit').map((code) => ({
    value: code,
    label: `${env.unitNames[code] ?? weightUnitLabels[code] ?? code}（${code}）`,
  }));
}

export function lengthUnitOptions(spec: PriceCardTemplateSpec, env: EditorEnv): Option[] {
  return allowedValues(spec, sheetNames.card, 'volumetricLengthUnit').map((code) => ({
    value: code,
    label: `${env.unitNames[code] ?? lengthUnitLabels[code] ?? code}（${code}）`,
  }));
}

/** 单位的短写法，如「千克」。 */
export function unitWord(code: string, env: EditorEnv): string {
  return env.unitNames[code] ?? weightUnitLabels[code] ?? lengthUnitLabels[code] ?? code;
}
