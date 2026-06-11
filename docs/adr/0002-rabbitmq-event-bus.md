# ADR-0002: RabbitMQ как внутренняя шина событий

- Status: Accepted
- Date: 2026-06-06
- Change: sre-oncall-platform (commit c7941dd)
- Affected: все сервисы, pkg/amqp

## Context

Пять микросервисов ([ADR-0001](0001-five-go-microservices.md)) требуют асинхронного взаимодействия по цепочке alert → incident → escalation → notification. RabbitMQ уже эксплуатируется как внешний сервис в production; в локальной разработке поднимается в k3s через Helm-чарт Bitnami.

## Options considered

- **RabbitMQ (AMQP, topic exchanges)** — уже в эксплуатации, нет операционных издержек на новый брокер; fan-out через topic exchange с отдельными очередями даёт изоляцию потребителей.
- **Kafka** — даёт replay и Consumer Groups, но требует Zookeeper/KRaft, сложнее операционно, избыточна для объёма on-call алертов: replay-семантика и партиционирование не нужны. Отклонена.

## Decision

Всё взаимодействие между сервисами — через AMQP. Топология:

- exchange `alerts` (topic) → очередь `alerts.incident` → сервис `incident`
- exchange `incidents` (topic) → очередь `incidents.escalation` → сервис `escalation`
- exchange `escalations` (topic) → очередь `escalations.notification` → сервис `notification`

Все exchanges и очереди — durable. Потребители используют manual ACK.

## Consequences

- Никаких синхронных вызовов в горячем пути обработки алертов; сервисы связаны только контрактами событий (см. [ADR-0010](0010-self-sufficient-event-payloads.md)).
- Replay событий невозможен — потерянное событие не восстанавливается из брокера; durable-очереди и manual ACK обязательны для всех новых потребителей.
- В production RabbitMQ — внешний сервис (credentials через Kubernetes Secret); локально — Bitnami Helm chart в k3s.
- Новые межсервисные потоки должны добавляться как exchange/очередь в эту топологию, а не как прямые HTTP-вызовы.
