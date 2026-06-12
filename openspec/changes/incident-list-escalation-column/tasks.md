## 1. Backend: массовая выборка состояний (store)

- [ ] 1.1 Добавить в `services/escalation/internal/store` метод `ListEscalationStatesByIncidents(ctx, tenant, ids []string) ([]*domain.EscalationState, error)` (выборка по `incident_id = ANY($ids)` в рамках тенанта)

## 2. Backend: хендлер и маршрут

- [ ] 2.1 Реализовать `GetEscalationStates` (bulk) в `services/escalation/internal/handler/handler.go`: распарсить `incident_ids` (CSV), вернуть массив состояний; пустой/отсутствующий список → пустой массив
- [ ] 2.2 Зарегистрировать маршрут `GET /api/escalations/v1/{tenant}/incidents/state` в `cmd/server/main.go` (до/рядом с `/incidents/{incidentId}/state`, не конфликтуя по роутингу)
- [ ] 2.3 Гарантировать изоляцию тенанта (выборка только по тенанту из URL)

## 3. Backend: тесты

- [ ] 3.1 Тест: группа id, часть с состоянием, часть без → возвращаются только существующие
- [ ] 3.2 Тест: все id без состояния → пустой массив, HTTP 200
- [ ] 3.3 Тест изоляции тенанта: чужой incident_id не попадает в ответ

## 4. Frontend: API и типы

- [ ] 4.1 Добавить тип состояния эскалации (`incident_id`, `current_tier`, `status`) в `frontend/src/api/types.ts` (если отсутствует)
- [ ] 4.2 Добавить хук `useEscalationStates(tenant, incidentIds)` в `frontend/src/api/escalations.ts`: GET с `incident_ids`, ключ из отсортированных id, `enabled` при непустом списке, безопасная деградация при ошибке (пустая карта)

## 5. Frontend: колонка «Эскалация»

- [ ] 5.1 В `IncidentListPage.tsx` запросить состояния по id текущей страницы и построить `Map<incident_id, state>`
- [ ] 5.2 Добавить заголовок колонки «Эскалация» и ячейку: «—» если состояния нет; бейдж «Уровень N» + статус если есть
- [ ] 5.3 Стили бейджа по статусу (`active | acknowledged | resolved | exhausted`); согласовать нумерацию уровня с данными
- [ ] 5.4 При ошибке запроса состояний — во всех строках «—», список работает

## 6. Проверка

- [ ] 6.1 Ручная проверка: инцидент с запущенной эскалацией показывает уровень/статус; без эскалации — «—»
- [ ] 6.2 Проверка деградации: остановить escalation-сервис → список грузится, колонка «—»
- [ ] 6.3 Проверка отсутствия N+1: на странице со списком уходит один запрос состояний
