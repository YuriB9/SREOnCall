## 1. (3) TTL дедупликации — выравнивание спеки

- [x] 1.1 Спека уже обновляется delta-файлом этого изменения (`specs/alert-ingestion/spec.md`); проверить, что в коде значение по умолчанию `DEDUP_TTL_SECONDS = 4h` и комментарий в `services/ingestion/internal/config/config.go` ссылается на rationale (repeat_interval Alertmanager)
- [x] 1.2 Убедиться, что `docs/env-vars.md` описывает `DEDUP_TTL_SECONDS` с дефолтом 4 часа (поправить при расхождении)

## 2. (4) Канонизация source=alertmanager

- [x] 2.1 В `pkg/domain/alert.go` заменить `SourcePrometheus AlertSource = "prometheus"` на `SourceAlertmanager AlertSource = "alertmanager"`; обновить все использования (`services/ingestion/internal/handler/alertmanager.go` и др. по `go build ./...`)
- [x] 2.2 Добавить миграцию ingestion `000002_source_alertmanager.up.sql`: `UPDATE ingestion.raw_alerts SET source='alertmanager' WHERE source='prometheus'` (+ пустой `.down.sql`)
- [x] 2.3 Добавить миграцию incident `000003_source_alertmanager.up.sql`: `UPDATE incident.incident_alerts SET source='alertmanager' WHERE source='prometheus'` (+ пустой `.down.sql`)
- [x] 2.4 В consumer incident-сервиса нормализовать source перед поиском правила группировки: `prometheus` → `alertmanager` (алиас для сообщений старого формата в очереди); ветку `"prometheus"` в `DefaultGroupingLabels` сохранить
- [x] 2.5 Обновить тесты ingestion (`alertmanager_test.go`, `webhook_integration_test.go`): нормализованный алерт имеет `source: alertmanager`
- [x] 2.6 Добавить тест incident: алерт с `source: prometheus` группируется по правилу, заданному для `alertmanager`
- [x] 2.7 Задокументировать в release notes известное ограничение: firing-алерты, принятые до деплоя, не сматчатся со своими resolved после деплоя (fingerprint включает source) — открытые инциденты закрываются вручную

## 3. (5) Словарь severity во фронтенде

- [x] 3.1 В `frontend/src/api/types.ts` заменить `IncidentSeverity` на `'critical' | 'high' | 'warning' | 'info'`
- [x] 3.2 В `frontend/src/pages/IncidentListPage.tsx` обновить `SEVERITY_LABEL`/`SEVERITY_CLASS` (подписи «Предупреждение», «Инфо»; `warning` — жёлтая схема, `info` — нейтральная) и опции селектора фильтра критичности
- [x] 3.3 Проверить остальные места использования severity (`IncidentDetailPanel.tsx` и др.) — рендер бейджа для всех четырёх значений
- [x] 3.4 Прогнать typecheck/линт фронтенда (`tsc`, `eslint`) и визуально проверить бейджи и фильтр на инцидентах `warning`/`info`

## 4. (6) Звук по умолчанию выключен

- [x] 4.1 В `frontend/src/hooks/useAudioEnabled.ts` заменить инициализацию на `localStorage.getItem('oncall.audioEnabled') === 'true'`
- [x] 4.2 Проверить сценарии: первый визит — звук выключен; явно включённый звук переживает перезагрузку

## 5. (7) Детали конфликта в теле 409

- [x] 5.1 В `services/scheduling/internal/store/store.go` расширить `CreateOverride`: при пересечении возвращать данные конфликтующего переопределения (типизированная ошибка с `StartAt`, `EndAt`, `UserID` либо отдельное значение)
- [x] 5.2 В `services/scheduling/internal/handler/handler.go` (`CreateOverride`) сериализовать 409 как `{"error": ..., "existing_start": <RFC3339>, "existing_end": <RFC3339>, "existing_user": <user_id>}`
- [x] 5.3 Дополнить интеграционный тест scheduling: тело 409 содержит все три поля конфликта в корректном формате
- [x] 5.4 Проверить модальное окно создания замены: при 409 отображается встроенная ошибка с датами конфликта (фронтенд-контракт `ConflictDetail` уже ожидает эти поля); при необходимости отобразить username вместо `user_id` через members

## 6. Верификация и закрытие

- [x] 6.1 `go build ./... && go test ./...` по всем затронутым сервисам (включая интеграционные тесты по их инструкции запуска)
- [x] 6.2 Сквозная проверка: алерт Alertmanager → инцидент с `source: alertmanager` и severity-бейджем в UI; конфликт замены → осмысленное сообщение в модальном окне
- [x] 6.3 Обновить `docs/spec-vs-code-audit.md`: пометить пункты 3–7 как исправленные этим изменением
