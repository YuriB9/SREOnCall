## MODIFIED Requirements

### Requirement: Политика эскалации по умолчанию для тенанта

Сервис escalation ДОЛЖЕН (SHALL) хранить `default_policy_id` per-tenant и автоматически назначать политику при появлении нового инцидента. Текущая политика по умолчанию ДОЛЖНА быть доступна через GET `/api/escalations/v1/{tenant}/default-policy`.

При потреблении события `incident.created` с пустым `tenant_slug` (событие от старой версии incident-сервиса) сервис ДОЛЖЕН использовать `tenant_id` события в качестве slug'а тенанта — состояние эскалации НЕ ДОЛЖНО сохраняться с пустым `tenant_slug`, поскольку slug используется для резолва дежурного в scheduling и попадает в события `escalation.triggered`.

#### Scenario: Установка политики по умолчанию

- **WHEN** администратор выполняет PUT на `/api/escalations/v1/{tenant}/default-policy` с `{ "policy_id": "<uuid>" }`
- **THEN** `default_policy_id` сохраняется; политика должна принадлежать этому тенанту, иначе HTTP 422

#### Scenario: Получение политики по умолчанию

- **WHEN** выполняется GET на `/api/escalations/v1/{tenant}/default-policy`
- **THEN** возвращается текущая конфигурация с `policy_id`; при отсутствии — HTTP 404

#### Scenario: Снятие политики по умолчанию

- **WHEN** администратор выполняет DELETE на `/api/escalations/v1/{tenant}/default-policy`
- **THEN** запись удаляется; новые инциденты создаются без автоматической эскалации

#### Scenario: Автоматическое назначение при создании инцидента

- **WHEN** сервис escalation потребляет событие `incident.created` из очереди `incidents.escalation`
- **AND** для тенанта инцидента задана политика по умолчанию
- **THEN** политика автоматически привязывается к инциденту, отслеживание стартует с 1-го уровня, и событие `escalation.triggered` для tier 1 публикуется немедленно (первичное оповещение дежурного)

#### Scenario: Fallback при пустом tenant_slug в событии

- **WHEN** сервис escalation потребляет событие `incident.created` с пустым `tenant_slug` и непустым `tenant_id`
- **THEN** состояние эскалации сохраняется с `tenant_slug`, равным `tenant_id`; резолв дежурного выполняется по этому slug'у, и `escalation.triggered` публикуется с непустым `tenant_slug`

#### Scenario: Тенант без политики по умолчанию

- **WHEN** событие `incident.created` получено, но политика по умолчанию для тенанта не задана
- **THEN** запись игнорируется; эскалация может быть назначена вручную позднее
