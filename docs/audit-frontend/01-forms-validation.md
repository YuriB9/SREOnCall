# Аудит SREOnCall Frontend — Область 1: Формы и валидация

Дата: 2026-06-14
Область: формы, валидация ввода, межполевые инварианты, дублирование.
Стек: React 19 + TS + RHF/zod (предлагается). shadcn даёт `form.tsx`-обёртку под RHF.

Шесть форм-файлов. Валидация **полностью ручная и несогласованная**: три разных стиля
сосуществуют, местами **в одном файле**. Один и тот же email-валидатор скопирован в двух
местах. Есть непокрытый межполевой баг (`end > start` в override). Это не «формы плохо
написаны» — каждая по отдельности аккуратна; проблема в **отсутствии единого механизма**:
каждая новая форма заново изобретает, как хранить ошибки, когда валидировать и как
блокировать submit. Решение — `react-hook-form` + `zod` + `@hookform/resolvers` (вердикт
по технологии — в [00-roadmap.md](00-roadmap.md), внедрение — FE02/FE03).

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| F1 | **major** | Три несовместимых стиля валидации, два — в одном файле `NotificationConfigPage` (одиночный `error` vs объект `errors`) | [NotificationConfigPage.tsx:39](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L39), [:151](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L151) |
| F2 | **major** | Непокрытый межполевой инвариант: override `end > start` нигде не проверяется до отправки | [ScheduleFormModal.tsx:70-71](../../frontend/src/pages/ScheduleFormModal.tsx#L70-L71) |
| F3 | **minor** | `isValidEmail` скопирован дословно в 2 файла (+ третья ad-hoc проверка url) — нет общих валидаторов | [ProfilePage.tsx:14-17](../../frontend/src/pages/ProfilePage.tsx#L14-L17), [NotificationConfigPage.tsx:22-25](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L22-L25) |
| F4 | **minor** | Валидация мутирует доменный state ради ошибок: `validate()` переписывает массив `steps`, проставляя `error` в каждый элемент | [PolicyEditorPage.tsx:205-212](../../frontend/src/pages/PolicyEditorPage.tsx#L205-L212) |
| F5 | **minor** | Взаимозависимые поля (канал↔значение) валидируются россыпью `if` внутри обработчика тоггла, два отдельных стейта предупреждений | [ProfilePage.tsx:77-107](../../frontend/src/pages/ProfilePage.tsx#L77-L107) |
| F6 | **minor** | До 13 `useState` на форму только под значения/ошибки/флаги — ручная синхронизация «грязное/валидно/можно-сабмитить» | [ScheduleFormModal.tsx:50-68](../../frontend/src/pages/ScheduleFormModal.tsx#L50-L68) |

`useState`-нагрузка по формам (значения + ошибки + UI-флаги): ScheduleFormModal — 13,
NotificationConfigPage — 10, ProfilePage — 7, WebhookTokensPage — 7, PolicyEditorPage — 5,
IncidentDetailPanel — 2.

---

## Детализация

### F1 — Три стиля валидации, два в одном файле — **major**

Один и тот же продукт, три разных способа хранить и показывать ошибки:

- **Императивный флаг + мутация state** — `PolicyEditorPage`: `let valid = true`, ручной обход
  полей, ошибки записываются обратно в `steps` ([PolicyEditorPage.tsx:197-214](../../frontend/src/pages/PolicyEditorPage.tsx#L197-L214)).
- **Одиночный `error: string`** — `MattermostSection` ([NotificationConfigPage.tsx:39](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L39)).
- **Объект `errors: { smtpFrom?, replyTo? }`** — `EmailSection` ([NotificationConfigPage.tsx:151](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L151)).

Последние два **живут в одном файле** — то есть несогласованность не межкомандная, а внутри
одной фичи. Каждая форма заново решает, когда валидировать (на submit? на change? на blur?),
как сбрасывать ошибку (вручную при каждом `onChange`: [:107](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L107),
[:210](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L210)) и как блокировать кнопку.

**Фикс.** Единый механизм через `react-hook-form` + `zodResolver`. Схема — единственный
источник правды о валидности; `<FormMessage>` из shadcn `form.tsx` рендерит ошибку поля;
сброс ошибки на изменение — встроенный. Ручные `error`/`errors`/`valid` уходят.

---

### F2 — Override `end > start` не проверяется — **major (скрытый баг)**

`canSubmitOverride` блокирует кнопку только по **наличию** значений, не по их соотношению:

```tsx
// ScheduleFormModal.tsx:70-71
const canSubmitOverride =
  Boolean(ovrUserId && ovrStart && ovrEnd) && !createOverride.isPending
```

Дальше значения уходят в `new Date(ovrStart).toISOString()` / `new Date(ovrEnd)...`
([:81-82](../../frontend/src/pages/ScheduleFormModal.tsx#L81-L82)) без сравнения. Инвертированный интервал
(`end ≤ start`) отправляется на бэкенд. Серверная проверка ловит **пересечение** с
существующими заменами ([:92-98](../../frontend/src/pages/ScheduleFormModal.tsx#L92-L98)), но не вырожденный/
отрицательный интервал — пользователь получит непредсказуемый результат вместо понятной
ошибки поля.

**Фикс.** В zod-схеме override — `.refine(d => d.end > d.start, { message: 'Конец замены
должен быть позже начала', path: ['end'] })`. Это ровно тот класс межполевых инвариантов,
который ручные `canSubmit`-булевы систематически пропускают, а `.refine`/`.superRefine`
делает декларативным. Закрывается в FE03 (миграция `ScheduleFormModal`).

---

### F3 — Дублированные валидаторы — **minor**

`isValidEmail` скопирован дословно:

```tsx
// ProfilePage.tsx:14-17  И  NotificationConfigPage.tsx:22-25 — идентичны
function isValidEmail(val: string) {
  if (!val) return true
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(val)
}
```

Плюс ad-hoc `isValidUrl` через `new URL()` в третьем месте ([NotificationConfigPage.tsx:12-20](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L12-L20)).
Разойдутся при первой же правке регэкспа в одном месте.

**Фикс.** Общие zod-примитивы в `src/lib/validators.ts` (`z.string().email()`,
`z.string().url()` с русскими `message`), переиспользуемые всеми схемами. Заодно убирает
семантику «пустое = валидно» из императивного кода в `.optional()`/`.or(z.literal(''))`.

---

### F4 — Валидация мутирует доменный state — **minor**

`validate()` в `PolicyEditorPage` не просто проверяет — он **переписывает** массив `steps`,
вкладывая `error` в каждый шаг, и вызывает `setSteps`:

```tsx
// PolicyEditorPage.tsx:205-212
const updated = steps.map((s) => {
  if (!s.notify_schedule_id) { valid = false; return { ...s, error: 'Выберите расписание' } }
  return { ...s, error: undefined }
})
setSteps(updated)
```

Ошибки валидации перемешаны с доменными данными (`DraftStep.error` соседствует с
`notify_schedule_id`/`timeout_seconds`, [:18-22](../../frontend/src/pages/PolicyEditorPage.tsx#L18-L22)). Это
лишний ре-рендер и стирание данных-под-ошибки при каждом прогоне.

**Фикс.** RHF `useFieldArray` для динамических шагов; ошибки живут в `formState.errors`,
отдельно от значений. `DraftStep.error` исчезает из доменной модели.

---

### F5 — Взаимозависимые поля россыпью `if` — **minor**

Правило «нельзя включить канал без значения» размазано по обработчику тоггла, с двумя
отдельными стейтами предупреждений и ранними `return`:

```tsx
// ProfilePage.tsx:77-91 (фрагмент)
if (enabled && ch === 'email' && !email.trim()) { setEmailWarning('Укажите email...'); return }
if (enabled && ch === 'email' && !isValidEmail(email)) { setEmailWarning('...корректный...'); return }
setEmailWarning(null)
if (enabled && ch === 'mattermost' && !mattermostUsername.trim()) { setMattermostWarning('...'); return }
```

Логика «значение ↔ канал» — это межполевой инвариант, но выражен как поток управления, а не
как правило. Добавить третий канал = ещё ветка + ещё стейт предупреждения.

**Фикс.** `superRefine` по всей форме: для каждого включённого канала проверять
непустое/валидное значение, ошибку вешать на `path: [channelValueField]`. Декларативно,
расширяемо, тестируемо.

---

### F6 — `useState`-нагрузка и ручная синхронизация флагов — **minor**

`ScheduleFormModal` держит 13 `useState` ([ScheduleFormModal.tsx:50-68](../../frontend/src/pages/ScheduleFormModal.tsx#L50-L68)):
значения формы, значения override, `error`, `ovrError`, `ovrSuccess`, плюс производный
`canSubmit` пересчитывается вручную ([:163-164](../../frontend/src/pages/ScheduleFormModal.tsx#L163-L164)). «Грязное/
тронутое/валидно/можно-сабмитить» — всё руками. Это не баг, но это поверхность, на которой
баги вроде F2 заводятся незаметно.

**Фикс.** `useForm` сводит значения+ошибки+`isDirty`/`isValid`/`isSubmitting` в один объект
`formState`. 13 `useState` → 1 `useForm` (+ возможно отдельный `useForm` для under-формы
override, т.к. это самостоятельный submit).

---

## Что сделано хорошо (для контекста)

- **UX-детали продуманы**: «висячая» ссылка на удалённое расписание показывается явной опцией,
  а не пустотой ([PolicyEditorPage.tsx:63-102](../../frontend/src/pages/PolicyEditorPage.tsx#L63-L102)); маскированный webhook
  URL никогда не пишется обратно ([NotificationConfigPage.tsx:42-45](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L42-L45)).
- **Серверные ошибки типизированы и человечны**: `OverrideConflictError`/`ScheduleValidationError`
  разбираются и переводятся в русский текст с именами и датами ([ScheduleFormModal.tsx:91-104](../../frontend/src/pages/ScheduleFormModal.tsx#L91-L104)).
- **Предупреждения, а не блокировки**, где уместно: «канал включён, но webhook не задан» не
  мешает сохранить ([NotificationConfigPage.tsx:55-57](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L55-L57)).
- **Инициализация из сервера через `useRef`-страж** от перезатирания пользовательского ввода —
  единообразна ([ProfilePage.tsx:41-59](../../frontend/src/pages/ProfilePage.tsx#L41-L59), [NotificationConfigPage.tsx:47-53](../../frontend/src/pages/settings/NotificationConfigPage.tsx#L47-L53)).

Миграция на RHF/zod **не должна** растерять эти качества — это критерий приёмки FE02/FE03.

---

## Рекомендованный порядок (детали — в roadmap)

1. **FE02 пилот — `NotificationConfigPage`** (НЕ самая сложная): `EmailSection` →
   `MattermostSection`. Вводит RHF + zod + `@hookform/resolvers` + shadcn `form.tsx` +
   `lib/validators.ts`. Закрывает F1, F3. Эталон для остальных.
2. **FE03 раскатка**: `PolicyEditorPage` (F4, `useFieldArray`) → `ProfilePage` (F5,
   `superRefine`) → `ScheduleFormModal` **последней** (F2, F6 — самая сложная, 13 useState).
3. `WebhookTokensPage` (7 useState) и `IncidentDetailPanel` (2) — оценить по месту: если полей
   1–2, RHF может быть оверкилл, оставить как есть. Не мигрировать ради единообразия.

> Бонус по контракту: zod-схемы здесь описывают **запросы**. Их можно частично переиспользовать
> для рантайм-валидации **ответов** — но это отдельное, осознанно ограниченное решение, см.
> [02-contract-typing.md](02-contract-typing.md).
</content>
</invoke>
