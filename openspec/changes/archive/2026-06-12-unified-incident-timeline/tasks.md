## 1. API-клиент истории эскалации

- [x] 1.1 Добавить тип `EscalationHistoryEntry` в `frontend/src/api/types.ts` (поля `id`, `incident_id`, `event_type: 'triggered' | 'tier_advanced' | 'acknowledged' | 'resolved' | 'exhausted'`, опциональные `tier`, `oncall_user_id`, `oncall_username`, `created_at`)
- [x] 1.2 Добавить хук `useEscalationHistory(tenant, incidentId)` в `frontend/src/api/escalations.ts` с запросом `GET /escalations/v1/{tenant}/incidents/{id}/history` и ключом в `escalationKeys`
- [x] 1.3 Реализовать graceful-деградацию: при ошибке запроса возвращать `[]` (по образцу `useEscalationStates`), запрос включается только при наличии `incidentId`

## 2. Объединённый таймлайн во вкладке «История»

- [x] 2.1 В `IncidentDetailPanel.tsx` вызвать `useEscalationHistory` и нормализовать обе коллекции (журнал инцидента + история эскалации) в единый тип элемента таймлайна с полем времени
- [x] 2.2 Объединить и отсортировать элементы по времени в хронологическом порядке
- [x] 2.3 Расширить карты иконок/подписей событиями эскалации (`triggered`, `tier_advanced`, `acknowledged`, `resolved`, `exhausted`); для срабатываний выводить номер уровня и `oncall_username`
- [x] 2.4 Отрендерить объединённый таймлайн в `TabsContent value="history"`, сохранив пустое состояние при отсутствии записей

## 3. Счётчик записей вкладки

- [x] 3.1 Показать счётчик длины объединённого таймлайна в `TabsTrigger value="history"` («История (N)») по аналогии с «Алерты»/«Комменты»

## 4. Проверка

- [x] 4.1 Прогнать lint/typecheck фронтенда (`npm run lint`, `tsc`) и собрать (`npm run build`)
- [x] 4.2 Проверить сценарии вкладки «История»: объединённый таймлайн, корректный счётчик, мягкая деградация при недоступном escalation-сервисе
- [x] 4.3 `openspec validate unified-incident-timeline --strict` без ошибок
