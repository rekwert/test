\set ON_ERROR_STOP on
BEGIN;

UPDATE notification.inbox
SET title = CASE category
      WHEN 'billing' THEN 'Уведомление об оплате'
      WHEN 'support' THEN 'Новый ответ поддержки'
      WHEN 'vps' THEN 'Сервер готов к использованию'
      WHEN 'dedicated' THEN 'Выделенный сервер готов'
      WHEN 'admin' THEN 'Уведомление администратора'
      ELSE 'Системное уведомление'
    END,
    body = CASE
      WHEN body !~ '\?{3,}' THEN body
      WHEN category = 'billing'
        THEN 'Архивное уведомление об оплате или продлении сервера. Актуальная информация доступна в разделах «Серверы» и «Платежи».'
      WHEN category = 'support'
        THEN 'В вашем обращении появился новый ответ. Откройте раздел поддержки для просмотра переписки.'
      WHEN category IN ('vps', 'dedicated')
        THEN 'Сервер подготовлен. Актуальные данные подключения доступны в карточке сервера.'
      WHEN category = 'admin'
        THEN 'Архивное уведомление администратора.'
      ELSE 'Архивное системное уведомление.'
    END
WHERE title ~ '\?{3,}' OR body ~ '\?{3,}';

UPDATE notification.deliveries
SET subject = CASE template
      WHEN 'email_verify' THEN 'Код подтверждения email'
      WHEN 'password_reset' THEN 'Сброс пароля'
      WHEN 'telegram_link' THEN 'Привязка Telegram'
      ELSE 'Архивное уведомление CLOUD HUSTLE'
    END,
    body = CASE template
      WHEN 'email_verify' THEN
        'Здравствуйте!' || E'\n\n' ||
        'Код подтверждения email: ' || COALESCE(metadata->>'code', '') || E'\n\n' ||
        'Код действует 15 минут. Если вы не регистрировались на cloud-hustle.com, проигнорируйте это письмо.' ||
        E'\n\n— CLOUD HUSTLE\ncloud-hustle.com'
      WHEN 'password_reset' THEN
        'Здравствуйте!' || E'\n\n' ||
        'Код для сброса пароля: ' || COALESCE(metadata->>'code', '') || E'\n\n' ||
        'Код действует 15 минут. Если вы не запрашивали сброс, проигнорируйте письмо.' ||
        E'\n\n— CLOUD HUSTLE\ncloud-hustle.com'
      WHEN 'telegram_link' THEN
        'Здравствуйте!' || E'\n\n' ||
        'Код для привязки Telegram-бота: ' || COALESCE(metadata->>'code', '') || E'\n\n' ||
        'Код действует 15 минут. Если вы не запрашивали привязку, проигнорируйте письмо.' ||
        E'\n\n— CLOUD HUSTLE\ncloud-hustle.com'
      ELSE 'Архивное уведомление. Актуальная информация доступна в личном кабинете.'
    END,
    metadata = CASE
      WHEN COALESCE(metadata->>'html_body', '') ~ '\?{3,}' THEN metadata - 'html_body'
      ELSE metadata
    END
WHERE subject ~ '\?{3,}'
   OR body ~ '\?{3,}'
   OR metadata::text ~ '\?{3,}';

UPDATE notification.deliveries_legacy
SET subject = CASE template
      WHEN 'email_verify' THEN 'Код подтверждения email'
      WHEN 'password_reset' THEN 'Сброс пароля'
      WHEN 'telegram_link' THEN 'Привязка Telegram'
      ELSE 'Архивное уведомление CLOUD HUSTLE'
    END,
    body = CASE template
      WHEN 'email_verify' THEN
        'Здравствуйте!' || E'\n\n' ||
        'Код подтверждения email: ' || COALESCE(metadata->>'code', '') ||
        E'\n\n— CLOUD HUSTLE\ncloud-hustle.com'
      WHEN 'password_reset' THEN
        'Здравствуйте!' || E'\n\n' ||
        'Код для сброса пароля: ' || COALESCE(metadata->>'code', '') ||
        E'\n\n— CLOUD HUSTLE\ncloud-hustle.com'
      WHEN 'telegram_link' THEN
        'Здравствуйте!' || E'\n\n' ||
        'Код для привязки Telegram-бота: ' || COALESCE(metadata->>'code', '') ||
        E'\n\n— CLOUD HUSTLE\ncloud-hustle.com'
      ELSE 'Архивное уведомление. Актуальная информация доступна в личном кабинете.'
    END,
    metadata = CASE
      WHEN COALESCE(metadata->>'html_body', '') ~ '\?{3,}' THEN metadata - 'html_body'
      ELSE metadata
    END
WHERE subject ~ '\?{3,}'
   OR body ~ '\?{3,}'
   OR metadata::text ~ '\?{3,}';

UPDATE billing.invoices
SET description = btrim(
  regexp_replace(
    regexp_replace(description, '\?+10%', '10%', 'g'),
    '\?+', ' — ', 'g'
  )
)
WHERE description ~ '\?{3,}';

UPDATE billing.adjustments
SET reason = CASE kind
  WHEN 'credit' THEN 'Ручное пополнение баланса'
  WHEN 'refund' THEN 'Возврат средств администратором'
  WHEN 'debit' THEN 'Ручное списание с баланса'
  ELSE 'Корректировка баланса администратором'
END
WHERE reason ~ '\?{3,}';

UPDATE auth.audit_log
SET metadata = jsonb_set(
  metadata,
  '{reason}',
  to_jsonb(CASE action
    WHEN 'admin.credit' THEN 'Ручное пополнение баланса'
    WHEN 'admin.refund' THEN 'Возврат средств администратором'
    ELSE 'Корректировка баланса администратором'
  END),
  true
)
WHERE metadata::text ~ '\?{3,}';

UPDATE auth.audit_log_legacy
SET metadata = jsonb_set(
  metadata,
  '{reason}',
  to_jsonb(CASE action
    WHEN 'admin.credit' THEN 'Ручное пополнение баланса'
    WHEN 'admin.refund' THEN 'Возврат средств администратором'
    ELSE 'Корректировка баланса администратором'
  END),
  true
)
WHERE metadata::text ~ '\?{3,}';

UPDATE vps.regions
SET name_ru = CASE code
      WHEN 'nl' THEN 'Нидерланды'
      WHEN 'de' THEN 'Германия'
      WHEN 'fi' THEN 'Финляндия'
      WHEN 'gb' THEN 'Великобритания'
      ELSE name_ru
    END,
    city_ru = CASE code
      WHEN 'nl' THEN 'Амстердам'
      WHEN 'de' THEN 'Франкфурт'
      WHEN 'fi' THEN 'Хельсинки'
      WHEN 'gb' THEN 'Лондон'
      ELSE city_ru
    END,
    updated_at = now()
WHERE name_ru ~ '\?{3,}' OR city_ru ~ '\?{3,}';

UPDATE vps.plans
SET external_product_id = replace(external_product_id, '???', '™')
WHERE external_product_id ~ '\?{3,}';

UPDATE vps.software_profiles
SET labels = jsonb_set(
  labels,
  '{ru}',
  to_jsonb(CASE id
    WHEN 'clean' THEN 'Чистая ОС'
    WHEN 'claude-code' THEN 'Claude Code (веб-терминал)'
    ELSE COALESCE(labels->>'en', name)
  END),
  true
)
WHERE labels::text ~ '\?{3,}';

COMMIT;
