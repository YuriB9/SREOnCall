## 1. Helper резолва членов тенанта

- [ ] 1.1 Добавить в `services/scheduling/internal/handler` метод `tenantMemberSet(ctx, slug) (map[string]struct{}, available bool)`, строящий множество допустимых subs из `MembersClient.GetMembers`
- [ ] 1.2 Возвращать `available=false`, если `h.members == nil` или `GetMembers` вернул ошибку (с warning в журнал)

## 2. Валидация на запись

- [ ] 2.1 В `CreateSchedule` после базовой валидации проверять все subs из `rotation` против `tenantMemberSet`; при наличии неизвестных вернуть HTTP 422 с телом `{"unknown_members": [...]}`
- [ ] 2.2 В `CreateOverride` проверять `user_id` против `tenantMemberSet`; при неизвестном — HTTP 422 с `unknown_members`
- [ ] 2.3 Если источник участников недоступен (`available=false`) — пропускать валидацию (fail-open), запись выполняется как прежде
- [ ] 2.4 Применить ту же проверку к PATCH-обновлению расписания, если/когда эндпоинт обновления реализован

## 3. Признак устаревания на чтении (oncall)

- [ ] 3.1 Добавить поле `Stale bool` (`json:"stale"`) в `domain.OncallResult`
- [ ] 3.2 В `GetOnCall` после вычисления `user_id` проверять принадлежность текущим членам через `tenantMemberSet`; если не член — `stale=true`, `username=""`, warning в журнал
- [ ] 3.3 При недоступном источнике участников не выставлять `stale` агрессивно (оставлять прежнее поведение резолва имени), залогировать предупреждение

## 4. Тесты

- [ ] 4.1 Тест CreateSchedule: ротация с неизвестным sub → 422 и корректный `unknown_members`
- [ ] 4.2 Тест CreateSchedule: валидная ротация → 201
- [ ] 4.3 Тест CreateOverride: неизвестный `user_id` → 422
- [ ] 4.4 Тест fail-open: при `members == nil` запись проходит (201) без валидации
- [ ] 4.5 Тест GetOnCall: устаревший `user_id` → ответ с `stale: true` и пустым `username`
- [ ] 4.6 Тест GetOnCall: валидный дежурный → `stale: false`, имя резолвится

## 5. Документация и проверка

- [ ] 5.1 Обновить `docs/spec-vs-code-audit.md` (сверка требования `oncall-scheduling` с реализацией)
- [ ] 5.2 Ручная проверка: создать расписание с чужим sub → 422; для устаревшего расписания `oncall` → `stale: true`; после `seed-schedules.sh` → `stale: false`
