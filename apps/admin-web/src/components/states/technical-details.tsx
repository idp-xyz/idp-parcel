// 「技术详情」折叠件已抬到 components/TechnicalDetails.tsx 全站共用；这里只是旧路径的转发，让既有的 import 不改也能编。
// 新代码直接引 components/TechnicalDetails。
export { TechnicalDetails, httpRows, type TechnicalRow } from '../TechnicalDetails';
