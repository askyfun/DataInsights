-- +goose Up
-- bi_share（图表口令分享）功能正式下线。
--
-- 前端入口、后端 handler/service/model、四份端点与 /share 公开视图均已移除，
-- 删除图表/数据集/数据源的级联软删也不再触碰本表。这里**只标注、不 DROP**：
-- 历史分享行留着比删掉安全（口令虽已失效，token 归属关系仍是可审计的既成事实），
-- 且软删体系本就「绝不物理删除」。表结构保持原样，代码侧已无任何读写路径。
COMMENT ON TABLE bi_share IS
    'RETIRED（issue #112）：口令分享功能已下线，代码不再读写本表，仅保留历史数据。';

-- +goose Down
COMMENT ON TABLE bi_share IS NULL;
