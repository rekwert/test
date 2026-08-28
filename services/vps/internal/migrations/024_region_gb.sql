-- Replace Poland (coming soon) with United Kingdom in the region catalog.

INSERT INTO vps.regions (code, name_en, name_ru, city_en, city_ru, enabled, sort_order) VALUES
    ('gb', 'United Kingdom', 'Великобритания', 'London', 'Лондон', false, 40)
ON CONFLICT (code) DO UPDATE SET
    name_en = EXCLUDED.name_en,
    name_ru = EXCLUDED.name_ru,
    city_en = EXCLUDED.city_en,
    city_ru = EXCLUDED.city_ru,
    sort_order = EXCLUDED.sort_order,
    updated_at = now();

DELETE FROM vps.regions WHERE code = 'pl';
