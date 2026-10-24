package dao

const selectionSchemaReadySQL = `SELECT to_regclass('t_share_export_candidate') IS NOT NULL
 AND to_regclass('t_share_export_candidate_item') IS NOT NULL
 AND (SELECT count(*)=2 FROM pg_attribute a JOIN pg_attrdef defaults ON defaults.adrelid=a.attrelid AND defaults.adnum=a.attnum
 WHERE a.attrelid=to_regclass('t_share_media_selection') AND NOT a.attisdropped AND a.atttypid='text'::regtype AND a.attnotnull
 AND ((a.attname='source_kind' AND pg_get_expr(defaults.adbin,defaults.adrelid)='''share''::text') OR (a.attname='external_ref' AND pg_get_expr(defaults.adbin,defaults.adrelid)='''''::text')))
 AND EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass('t_share_media_selection') AND attname='selected_candidate_id' AND NOT attisdropped AND atttypid='bigint'::regtype AND NOT attnotnull)
 AND EXISTS(SELECT 1 FROM pg_constraint constraint_row WHERE constraint_row.conrelid=to_regclass('t_share_media_selection') AND constraint_row.confrelid=to_regclass('t_share_export_candidate_item') AND constraint_row.conname='share_media_selection_member_fk' AND constraint_row.contype='f' AND constraint_row.confmatchtype='s' AND constraint_row.convalidated
 AND (SELECT array_agg(attribute.attname::text ORDER BY keys.position) FROM unnest(constraint_row.conkey) WITH ORDINALITY keys(number,position) JOIN pg_attribute attribute ON attribute.attrelid=constraint_row.conrelid AND attribute.attnum=keys.number)=ARRAY['selected_candidate_id','work_key','season_number','episode_number']
 AND (SELECT array_agg(attribute.attname::text ORDER BY keys.position) FROM unnest(constraint_row.confkey) WITH ORDINALITY keys(number,position) JOIN pg_attribute attribute ON attribute.attrelid=constraint_row.confrelid AND attribute.attnum=keys.number)=ARRAY['candidate_id','work_key','season_number','episode_number'])
 AND (SELECT count(*)=2 FROM pg_constraint WHERE conrelid=to_regclass('t_share_media_selection') AND contype='c' AND convalidated AND conname IN ('share_selection_source_kind_check','share_selection_source_ref_check'))`

const selectionMetadataUnsupportedSQL = `SELECT EXISTS(SELECT 1 FROM t_share_media_selection selection WHERE COALESCE(to_jsonb(selection)->>'source_kind','share')<>'share')`
