import { describe, expect, it } from 'vitest';
import {
  classifyFieldKind,
  isComplexType,
  isDateTimeType,
  isNumericType,
  normalizeDataType,
} from '../../lib/dataTypes';

describe('normalizeDataType', () => {
  it('规范词表原样通过', () => {
    expect(normalizeDataType('float')).toBe('float');
    expect(normalizeDataType('integer')).toBe('integer');
    expect(normalizeDataType('boolean')).toBe('boolean');
    expect(normalizeDataType('string')).toBe('string');
    expect(normalizeDataType('date')).toBe('date');
    expect(normalizeDataType('datetime')).toBe('datetime');
    expect(normalizeDataType('array')).toBe('array');
    expect(normalizeDataType('map')).toBe('map');
  });

  it('历史词归一：number→float、json→map、unknown→string', () => {
    expect(normalizeDataType('number')).toBe('float');
    expect(normalizeDataType('json')).toBe('map');
    expect(normalizeDataType('unknown')).toBe('string');
    expect(normalizeDataType('')).toBe('string');
    expect(normalizeDataType(null)).toBe('string');
    expect(normalizeDataType(undefined)).toBe('string');
  });

  it('原始列类型兜底归一', () => {
    expect(normalizeDataType('timestamp')).toBe('datetime');
    expect(normalizeDataType('timestamp without time zone')).toBe('datetime');
    expect(normalizeDataType('bigint')).toBe('integer');
    expect(normalizeDataType('int(11)')).toBe('integer');
    expect(normalizeDataType('varchar(255)')).toBe('string');
    expect(normalizeDataType('Numeric')).toBe('float');
    expect(normalizeDataType('Nullable(String)')).toBe('string');
  });
});

describe('类型助手', () => {
  it('isNumericType 只认 float/integer', () => {
    expect(isNumericType('float')).toBe(true);
    expect(isNumericType('integer')).toBe(true);
    expect(isNumericType('boolean')).toBe(false);
    expect(isNumericType('string')).toBe(false);
  });

  it('isDateTimeType 认 date/datetime（原始类型词兜底）', () => {
    expect(isDateTimeType('date')).toBe(true);
    expect(isDateTimeType('datetime')).toBe(true);
    expect(isDateTimeType('timestamp')).toBe(true);
    expect(isDateTimeType('string')).toBe(false);
  });

  it('isComplexType 认 array/map', () => {
    expect(isComplexType('array')).toBe(true);
    expect(isComplexType('map')).toBe(true);
    expect(isComplexType('string')).toBe(false);
  });

  it('classifyFieldKind 三分类', () => {
    expect(classifyFieldKind('datetime')).toBe('date');
    expect(classifyFieldKind('integer')).toBe('number');
    expect(classifyFieldKind('float')).toBe('number');
    expect(classifyFieldKind('map')).toBe('string');
  });
});
