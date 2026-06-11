## 1. Frontend: бейджи на вкладке «Алерты»

- [x] 1.1 В `frontend/src/pages/IncidentDetailPanel.tsx` (рендер списка алертов, бейдж fingerprint) выводить `fingerprint: {alert.fingerprint.slice(0, 8)}` и проставлять `title={alert.fingerprint}` на бейдже для полного значения
- [x] 1.2 Там же добавить условный бейдж `instance: {incident.labels.instance}`, отображаемый только при наличии `incident.labels?.instance`; сохранить существующий стиль (`Badge variant="secondary"`, `font-mono` для fingerprint)
- [x] 1.3 Убедиться, что длинные значения больше не распирают панель (бейджи в строке `flex flex-wrap` остаются в пределах ширины)

## 2. Верификация

- [x] 2.1 Прогнать `tsc` и `eslint` по фронтенду без ошибок
- [x] 2.2 Открыть карточку инцидента с тестовым алертом (см. README, поток alertmanager-вебхука): fingerprint показан как `fingerprint: <8 символов>`, полный хеш во всплывающем `title`, бейдж `instance` присутствует при наличии лейбла и отсутствует при его отсутствии
