import { countryLabel, currencyLabel } from './refdata-names';
import { useRefdataNames } from './use-refdata-names';

// 列表单元格里的国家 / 币种译名：「CN」→「中国（CN）」，「CNY」→「人民币（CNY）」，查不到原样显示（ADR-0335 决定七）。
// 做成组件是为了让单元格自己订阅名表——名表读回时重绘，页面不必为此多接一个钩子。

export function CountryLabel({ code }: { code: string }) {
  return <>{countryLabel(code, useRefdataNames())}</>;
}

export function CurrencyLabel({ code }: { code: string }) {
  return <>{currencyLabel(code, useRefdataNames())}</>;
}
