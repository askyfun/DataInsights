-- +goose Up
-- 统一数据集字段数据类型词表：规范集合为
-- float/integer/boolean/string/date/datetime/array/map。
-- 历史值归一：number→float、json→map、unknown/空串→string（与应用层
-- model.NormalizeStandardType 同一口径；应用层读路径还会兜底归一，
-- 本迁移只是把存量一次性刷掉，避免新旧词混存）。
UPDATE bi_dataset
SET columns = (
    SELECT jsonb_agg(
        CASE
            WHEN col->>'type' = 'number' THEN jsonb_set(col, '{type}', '"float"')
            WHEN col->>'type' = 'json'   THEN jsonb_set(col, '{type}', '"map"')
            WHEN col->>'type' IN ('unknown', '') OR col->>'type' IS NULL
                                         THEN jsonb_set(col, '{type}', '"string"')
            ELSE col
        END
        ORDER BY ord
    )
    FROM jsonb_array_elements(columns) WITH ORDINALITY AS t(col, ord)
)
WHERE jsonb_typeof(columns) = 'array'
  AND columns <> '[]'::jsonb
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(columns) AS c(col)
      WHERE c.col->>'type' IN ('number', 'json', 'unknown', '')
         OR c.col->>'type' IS NULL
  );

-- +goose Down
-- 无法可靠还原历史词（float 原本可能是 number 也可能是 float），不做反向重写。
SELECT 1;
