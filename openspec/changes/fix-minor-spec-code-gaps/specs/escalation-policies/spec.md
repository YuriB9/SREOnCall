## MODIFIED Requirements

### Requirement: Видимость состояния эскалации

Сервис escalation ДОЛЖЕН (SHALL) предоставлять GET `/api/escalations/v1/{tenant}/incidents/{incidentId}/state`, возвращающий текущее состояние эскалации инцидента: `current_tier`, `status` (`active | acknowledged | resolved | exhausted`) и `escalate_at` (время следующего автоматического перехода уровня; для неактивных состояний — время последнего перехода).

Полная история уровней НЕ входит в ответ `/state` — она предоставляется отдельным эндпоинтом `/api/escalations/v1/{tenant}/incidents/{incidentId}/history` (см. требование «История эскалации инцидента»).

#### Scenario: Запрос активного состояния эскалации

- **WHEN** выполняется GET-запрос для инцидента с активной эскалацией
- **THEN** ответ содержит `current_tier`, `status: active` и `escalate_at` — время следующего перехода уровня

#### Scenario: Состояние эскалации отсутствует

- **WHEN** выполняется GET-запрос для инцидента без назначенной эскалации
- **THEN** сервис возвращает HTTP 404
