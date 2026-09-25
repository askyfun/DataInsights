// 数据集字段统一数据类型的 API 层转发。
//
// 规范词表与映射实现统一收在 `lib/dataTypes.ts`（与后端
// model.StandardDataTypes / NormalizeStandardType 同口径）。此处保留
// toStandardType 兼容签名：Dataset 页用原始列类型生成默认列时调用。
import { type DataType, normalizeDataType } from '@/lib/dataTypes';

export type StandardDataType = DataType;

export function toStandardType(sourceType: string, _datasourceType: string): StandardDataType {
  return normalizeDataType(sourceType);
}
