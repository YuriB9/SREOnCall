# SREOnCall Frontend — Roadmap исправлений по итогам аудита

Дата: 2026-06-14
Назначение: план реализации находок фронтенд-аудита (`docs/audit-frontend/01..03`) через
OpenSpec-чейнджи. Параллель к бэкенд-roadmap (`docs/audit/00-roadmap.md`), отдельная нумерация
с префиксом **FE**, чтобы не пересекаться с CH-чейнджами бэкенда.

Стратегия: **энейблер → пилот → раскатка** (не «один чейндж = одна форма», не «один мега-рефактор
всех форм»). Соответствует норме проекта.

## Объём (по вердиктам владельца)

| Решение | Вердикт | Где разобрано |
|---------|---------|---------------|
| `react-hook-form` + `zod` + `@hookform/resolvers` | ✅ **делаем** (инкрементально) | [01-forms-validation.md](01-forms-validation.md) |
| `openapi-typescript` (типы из OpenAPI-спеки) | ❌ **не делаем** (нет спеки на бэке) | [02-contract-typing.md](02-contract-typing.md) |
| `@testing-library/react` (RTL) | ✅ **делаем** (энейблер + DoD форм-чейнджей) | [03-component-testing.md](03-component-testing.md) |

`zod`-валидация **ответов** api — опционально и точечно внутри форм-чейнджей (1–2 ресурса), не
отдельный чейндж. См. [02-contract-typing.md](02-contract-typing.md), Решение 2.

## Принципы нарезки и порядка

1. **Энейблер вперёд** — RTL-харнесс (FE01) даёт сетку, на которой держатся тесты форм; ставится
   до миграции, как CH01 на бэке.
2. **Пилот на простой форме, не на сложной** — `NotificationConfigPage`, а НЕ `ScheduleFormModal`.
3. **`ScheduleFormModal` (13 useState, баг F2) — последней**, когда паттерн обкатан.
4. **Каждая мигрированная форма приносит RTL-тесты на свои инварианты** (Definition of Done).
5. **Сохранить UX-качества** существующих форм (висячие ссылки, маскировка webhook, типизированные
   серверные ошибки) — критерий приёмки, не «переписать с нуля».

Легенда риска: 🟢 низкий (аддитивно/инфра) · 🟡 средний (трогает несколько форм) · 🔴 высокий
(поведенческое изменение критичной формы).

---

## Статус (дашборд)

Статусы: `☐ todo` · `🔄 in progress` · `✅ done` · `⏸ blocked`.

| FE | Чейндж | Фаза | Зависит от | Риск | Статус |
| --- | --- | --- | --- | --- | --- |
| FE01 | setup-rtl-test-harness | 0 | — | 🟢 | ☐ |
| FE02 | forms-rhf-zod-pilot | 1 | FE01 | 🟡 | ☐ |
| FE03 | forms-rhf-zod-rollout | 2 | FE02 | 🔴 | ☐ |

Прогресс: **0 / 3** done.

Решения без чейнджа: openapi-typescript — **отклонено** (зафиксировано в [02-contract-typing.md](02-contract-typing.md),
повторно не предлагать).

---

## Матрица покрытия (находка → чейндж)

| Находка | Severity | Чейндж |
|---------|----------|--------|
| R1 — нет RTL-харнесса | minor | **FE01** |
| F1 — три стиля валидации | major | **FE02** (Notification*) → **FE03** (остальные) |
| F3 — дублированный `isValidEmail` | minor | **FE02** (`lib/validators.ts`) |
| F4 — валидация мутирует state (Policy) | minor | **FE03** |
| F5 — взаимозависимые поля (Profile) | minor | **FE03** |
| F2 — `end > start` не проверяется (Schedule) | major | **FE03** (последней) |
| F6 — 13 useState (Schedule) | minor | **FE03** |
| R2 — инварианты форм без тестов | minor | **FE02 + FE03** (DoD каждого) |
| C1/C2 — контракт types.ts / нет рантайм-валидации | minor | опц. zod-`parse` в FE02/FE03; openapi — **отклонено** |

---

## Фаза 0 — Энейблер

### FE01 · `setup-rtl-test-harness` 🟢
**Корень:** нет рендер-тестового харнесса — тестам форм негде жить (R1).
**Закрывает:** R1.
**Содержимое:** dev-deps `@testing-library/react` + `@testing-library/user-event` +
`@testing-library/jest-dom` + `jsdom`; `test`-блок в `vite.config.ts`
(`environment: 'jsdom'`, `globals: true`, `setupFiles: ['./src/test/setup.ts']`); setup-файл
с импортом jest-dom-матчеров; 1 smoke-render-тест как образец. `parseGroups.test.ts` должен
продолжить проходить.
**Зависит от:** —. **Первый** — даёт сетку для FE02/FE03.
**Проверки (DoD):** `npm run test` зелёный (старый + новый тест), `npm run lint` чисто,
`tsc -b` без ошибок.

---

## Фаза 1 — Пилот

### FE02 · `forms-rhf-zod-pilot` 🟡
**Корень:** три несогласованных стиля ручной валидации (F1), дублированные валидаторы (F3).
**Закрывает:** F1 и F3 на `NotificationConfigPage`; вводит инфраструктуру для FE03.
**Содержимое:**
- deps: `react-hook-form`, `zod`, `@hookform/resolvers`;
- `src/components/ui/form.tsx` — shadcn-обёртка под RHF (`<Form>`/`<FormField>`/`<FormMessage>`);
- `src/lib/validators.ts` — общие zod-примитивы (email/url с русскими сообщениями), убирают F3;
- миграция **`EmailSection`** (самая чистая: 4 поля, без взаимозависимостей) → затем
  **`MattermostSection`** (условная валидация url, «enabled без webhook» через `superRefine`);
- RTL-тесты на инварианты обеих секций (DoD, R2);
- **опц.**: `NotificationConfigResponseSchema` + `parse` в `queryFn` как образец zod-валидации
  ответа (C2) — если ценность подтверждается.
**Не трогать:** UX-качества (маскировка webhook [NotificationConfigPage.tsx:42-45](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L42-L45),
мягкое предупреждение missingWebhook) — должны сохраниться.
**Зависит от:** FE01.
**Проверки (DoD):** `npm run test`/`lint`/`tsc -b` чисто; ручная проверка обеих секций
(сохранение, валидация email/url, тоггл enabled).
**Это ADR-кандидат:** смена подхода к валидации форм для всего фронта — значимое арх-решение.
Зафиксировать через скилл `architecture-decision-records` (напр. `00xx-frontend-forms-rhf-zod.md`).

---

## Фаза 2 — Раскатка

### FE03 · `forms-rhf-zod-rollout` 🔴
**Корень:** оставшиеся формы на ручной валидации, межполевой баг F2.
**Закрывает:** F1 (остаток), F2, F4, F5, F6.
**Содержимое (в этом порядке):**
1. **`PolicyEditorPage`** — `useFieldArray` для динамических шагов; ошибки уходят из доменной
   `DraftStep` в `formState.errors` (F4). Сохранить логику «висячего» расписания
   ([PolicyEditorPage.tsx:63-102](../../frontend/src/pages/PolicyEditorPage.tsx#L63-L102)).
2. **`ProfilePage`** — `superRefine` на инвариант канал↔значение, вместо россыпи `if` и двух
   стейтов предупреждений (F5).
3. **`ScheduleFormModal` — последней** — 13 useState → `useForm` (F6); zod-`.refine` на
   `end > start` в override-под-форме (**закрывает баг F2**). Сохранить разбор
   `OverrideConflictError`/`ScheduleValidationError` ([ScheduleFormModal.tsx:91-104](../../frontend/src/pages/ScheduleFormModal.tsx#L91-L104)).
- RTL-тесты на инварианты каждой формы, **обязательно** регресс-тест на F2 (DoD, R2).
**Оценить по месту:** `WebhookTokensPage` (7 useState), `IncidentDetailPanel` (2) — мигрировать
только если полей достаточно, чтобы RHF окупился; иначе оставить.
**Зависит от:** FE02 (паттерн + form.tsx + validators обкатаны).
**Проверки (DoD):** `npm run test`/`lint`/`tsc -b` чисто; ручная проверка каждой формы;
**обязательно** проверить, что инвертированный интервал override теперь блокируется с ошибкой поля.

---

## Порядок и зависимости

```
FE01 (RTL-харнесс) ─► FE02 (пилот Notification*) ─► FE03 (Policy → Profile → Schedule)
```

- Строго последовательно: FE02 вводит `form.tsx`/`validators.ts`/паттерн, на которые опирается FE03.
- Не начинать FE02 без FE01 (тестам форм негде жить).
- `ScheduleFormModal` — всегда последней в FE03 (самая сложная + несёт баг F2).

---

## Backlog находок (вне текущего объёма)

- **C2 (zod-`parse` ответов)** — точечно в FE02/FE03; если не окупится — не делать.
- **openapi-typescript** — пересмотреть **только** если на бэкенде появится OpenAPI-спека по
  другим причинам (см. [02-contract-typing.md](02-contract-typing.md)). До тех пор — закрыто.
- **`WebhookTokensPage` / `IncidentDetailPanel`** — миграция опциональна, решение в FE03.
</content>
