## 1. Миграция БД

- [ ] 1.1 Создать `services/incident/migrations/000004_incident_alert_labels.up.sql`: `ALTER TABLE incident.incident_alerts ADD COLUMN labels jsonb NOT NULL DEFAULT '{}';`
- [ ] 1.2 Создать `000004_incident_alert_labels.down.sql`: `ALTER TABLE incident.incident_alerts DROP COLUMN labels;`

## 2. Доменная модель

- [ ] 2.1 Добавить поле `Labels map[string]string \`json:"labels"\`` в `IncidentAlert` (`services/incident/internal/domain/incident.go`)

## 3. Сторадж

- [ ] 3.1 В `AttachAlert` сохранять `labels` (INSERT колонки `labels`, значение из `ia.Labels`; nil → `{}`)
- [ ] 3.2 В `ListIncidentAlerts` добавить `labels` в SELECT и Scan в `ia.Labels`
- [ ] 3.3 Убедиться в корректной (де)сериализации `map[string]string` ↔ `jsonb` (pgx)

## 4. Консьюмер

- [ ] 4.1 В `handleFiring` (`services/incident/internal/consumer/consumer.go`) проставлять `ia.Labels = alert.Labels` перед `AttachAlert`

## 5. Хендлер / API

- [ ] 5.1 Убедиться, что `GET .../incidents/{id}/alerts` отдаёт `labels` (следует автоматически из доменной модели; проверить сериализацию пустого набора как `{}`)
- [ ] 5.2 Ручная привязка `POST .../alerts` без лейблов сохраняет `{}` (не nil)

## 6. Фронтенд

- [ ] 6.1 Добавить `labels?: Record<string, string>` в тип `Alert` (`frontend/src/api/types.ts`)
- [ ] 6.2 В `IncidentDetailPanel.tsx` на карточке алерта показывать различающие лейблы алерта (в первую очередь `instance`) из `alert.labels`, а не из `incident.labels`
- [ ] 6.3 Если у алерта нет различающих лейблов — бейдж не показывать (без шума)

## 7. Тесты

- [ ] 7.1 Интеграционный тест стораджа: `AttachAlert` сохраняет лейблы, `ListIncidentAlerts` их возвращает
- [ ] 7.2 Тест консьюмера: при автоматической привязке лейблы алерта сохраняются в `incident_alerts`
- [ ] 7.3 Тест хендлера `/alerts`: ответ содержит `labels` по каждому алерту; пустой набор сериализуется как `{}`
- [ ] 7.4 Тест: два алерта одного инцидента с разными `instance` возвращают разные `labels`

## 8. Проверка

- [ ] 8.1 Ручная проверка: отправить 4 алерта (одинаковые `alertname`/`job`, разные `instance`), открыть инцидент — на вкладке «Алерты» у каждого свой `instance`
- [ ] 8.2 Обновить `docs/spec-vs-code-audit.md` (устранение расхождения по лейблам алерта)
