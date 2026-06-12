## 1. Слой API (frontend/src/api/schedules.ts)

- [x] 1.1 Добавить input-тип `ScheduleInput` (`name`, `timezone`, `rotation: string[]`, `shift_duration`, `start_date`) — в `schedules.ts` или `types.ts`
- [x] 1.2 `useCreateSchedule(tenant)` — `POST /schedules/v1/{tenant}/schedules`, при успехе `invalidateQueries(scheduleKeys(tenant).list())`
- [x] 1.3 `useUpdateSchedule(tenant)` — `PATCH /schedules/v1/{tenant}/schedules/{id}`, инвалидция `list()`
- [x] 1.4 `useDeleteSchedule(tenant)` — `DELETE /schedules/v1/{tenant}/schedules/{id}`, инвалидция `list()`
- [x] 1.5 Прокинуть ошибку `422` (тело `{ missing_fields }` / `{ error }`) до вызывающего кода для inline-отображения

## 2. Модальная форма (frontend/src/pages/ScheduleFormModal.tsx — новый файл)

- [x] 2.1 Создать компонент `ScheduleFormModal` в стиле `CreateOverrideModal` (overlay, `<form>`, `Button`, кнопка закрытия)
- [x] 2.2 Props: `tenant`, `members`, `schedule?: Schedule` (наличие → режим редактирования), `onClose`
- [x] 2.3 Поля: название (text), часовой пояс (select/Intl, дефолт `UTC`), `shift_duration` (select пресетов P1D/P7D/P14D/PT8H/PT12H), дата старта (`type="date"`)
- [x] 2.4 Редактор ротации: добавление инженера из `members`, удаление, перемещение вверх/вниз; отправка `rotation` как массив `user_id`; подписи по `preferred_username`
- [x] 2.5 В режиме редактирования предзаполнить поля из `schedule`
- [x] 2.6 Submit: создание → `useCreateSchedule`, редактирование → `useUpdateSchedule`; конвертировать значения в формат API (`start_date` = `YYYY-MM-DD`)
- [x] 2.7 `canSubmit` блокирует отправку при пустых обязательных полях и пустой ротации; показать состояние «Создание…/Сохранение…»
- [x] 2.8 Inline-отображение ошибок `422` (`missing_fields` / `error`), модалка остаётся открытой
- [x] 2.9 При успехе закрыть модалку (`onClose`)

## 3. Точки входа на странице (frontend/src/pages/SchedulesPage.tsx)

- [x] 3.1 Кнопка «Создать расписание» (например, рядом с «Создать замену»), открывающая `ScheduleFormModal` без `schedule`
- [x] 3.2 Обновить пустое состояние Gantt (`GanttGrid`): вместо «Создайте первое через API» — текст + кнопка «Создать расписание»
- [x] 3.3 Обновить пустое состояние виджета «Кто дежурит сейчас» (опционально дать ту же кнопку)
- [x] 3.4 В строке расписания (`GanttGrid` row header) добавить действия «Редактировать» (открывает модалку с `schedule`) и «Удалить»
- [x] 3.5 Удаление — подтверждение перед `useDeleteSchedule`
- [x] 3.6 Состояние модалки расписания (`scheduleModal: { schedule?: Schedule } | null`) в `SchedulesPage`

## 4. Проверка

- [x] 4.1 Создать расписание из UI на пустом тенанте → появляется строка в Gantt и карточка в виджете
- [x] 4.2 Отредактировать (название/ротация/длительность) → изменения отражаются после сохранения
- [x] 4.3 Удалить → расписание исчезает из Gantt и виджета
- [x] 4.4 Отправка с незаполненными полями / пустой ротацией → inline-ошибка `422`, модалка открыта
- [x] 4.5 `npm run build` / линт фронтенда проходят
