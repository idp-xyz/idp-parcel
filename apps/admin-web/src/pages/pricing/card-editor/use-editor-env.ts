import { useEffect, useState } from 'react';
import type { ApiResult } from '../../catalogue-api';
import { listCurrencies, listUnits } from '../../reference-data/api';
import { listReferenceSeries, readPriceCardTemplateSpec, type PriceCardTemplateSpec } from '../api';

/** 编辑器在表单之外要读的东西：模板规格（必须有）、币种与单位目录、已登记的燃油费率序列（读不到各有退路）。 */
export interface EditorEnv {
  /** 币种目录；读不到是 null，页面退回手填。 */
  currencies: { code: string; label: string }[] | null;
  /** 单位目录里的中文名，按码查；读不到是空表，页面用词典里的译法。 */
  unitNames: Record<string, string>;
  /** 已登记的燃油费率序列标识；读不到是 null，页面退回手填。 */
  fuelSeries: string[] | null;
}

export interface EditorEnvState {
  spec: ApiResult<PriceCardTemplateSpec> | null;
  env: EditorEnv;
  reload: () => void;
}

const emptyEnv: EditorEnv = { currencies: null, unitNames: {}, fuelSeries: null };

export function useEditorEnv(): EditorEnvState {
  const [spec, setSpec] = useState<ApiResult<PriceCardTemplateSpec> | null>(null);
  const [env, setEnv] = useState<EditorEnv>(emptyEnv);
  const [token, setToken] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setSpec(null);
    void readPriceCardTemplateSpec().then((answer) => {
      if (!cancelled) setSpec(answer);
    });
    void Promise.all([listCurrencies(), listUnits(), listReferenceSeries()]).then(([currencies, units, series]) => {
      if (cancelled) return;
      setEnv({
        currencies:
          currencies.kind === 'outcome'
            ? currencies.body.currencies.map((currency) => ({ code: currency.code, label: `${currency.nameZh}（${currency.code}）` }))
            : null,
        unitNames: units.kind === 'outcome' ? Object.fromEntries(units.body.units.map((unit) => [unit.code, unit.nameZh])) : {},
        fuelSeries:
          series.kind === 'outcome'
            ? [...new Set(series.body.series.filter((row) => row.kind === 'FUEL_RATE').map((row) => row.seriesId))]
            : null,
      });
    });
    return () => {
      cancelled = true;
    };
  }, [token]);

  return { spec, env, reload: () => setToken((value) => value + 1) };
}
