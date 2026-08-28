-- Region catalog for order locations (UI countries).
-- A region is orderable only when enabled AND an online node exists for that code.

CREATE TABLE IF NOT EXISTS vps.regions (
    code        text PRIMARY KEY,
    name_en     text NOT NULL,
    name_ru     text NOT NULL,
    city_en     text NOT NULL DEFAULT '',
    city_ru     text NOT NULL DEFAULT '',
    enabled     boolean NOT NULL DEFAULT false,
    sort_order  int NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO vps.regions (code, name_en, name_ru, city_en, city_ru, enabled, sort_order) VALUES
    ('nl', 'Netherlands', 'Нидерланды', 'Amsterdam', 'Амстердам', true, 10),
    ('de', 'Germany', 'Германия', 'Frankfurt', 'Франкфурт', false, 20),
    ('fi', 'Finland', 'Финляндия', 'Helsinki', 'Хельсинки', false, 30),
    ('gb', 'United Kingdom', 'Великобритания', 'London', 'Лондон', false, 40)
ON CONFLICT (code) DO UPDATE SET
    name_en = EXCLUDED.name_en,
    name_ru = EXCLUDED.name_ru,
    city_en = EXCLUDED.city_en,
    city_ru = EXCLUDED.city_ru,
    sort_order = EXCLUDED.sort_order,
    updated_at = now();

-- Keep NL enabled; other GEO stay listed but not orderable until a node is online.
UPDATE vps.regions SET enabled = true, updated_at = now() WHERE code = 'nl';
