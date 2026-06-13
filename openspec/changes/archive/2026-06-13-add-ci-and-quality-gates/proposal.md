## Why

В репозитории нет ни одного автоматического гейта качества: отсутствуют
`.github/workflows/`, `.golangci.yml`, `tool`-директивы и автообновление
зависимостей. Поэтому 5161 строка тестов, `-race`, линтер и `govulncheck`
не запускаются никем, кроме как вручную — и реальные дефекты (двойная
эскалация, нетранзакционные записи, обрыв по отмене) прошли мимо ревью именно
из-за отсутствия сетки. Вдобавок 7 из 9 поведенческих тестов помечены тегом
`integration`, хотя работают на in-memory-стабах, и потому исключены из
дефолтного `go test ./...` без причины. Это первопричина (находки T1, T2, DC2,
DC3, DC4, DC6) и фундамент, на котором держатся остальные чейнджи аудита.

## What Changes

- **CI-пайплайн (T1, DC2).** GitHub Actions с матрицей по 7 модулям `go.work`:
  `go build`, `go test -race -shuffle=on`, проверка `go mod tidy` через
  `git diff --exit-code`; отдельные джобы `lint` (golangci-lint),
  `govulncheck` и `integration` (Postgres/Redis/RabbitMQ через `services:`).
  e2e — отдельным lightweight-джобом по расписанию/диспетчеру.
- **Снятие тега `integration` со стаб-тестов (T2).** 7 файлов
  `*_integration_test.go`, которые декларируют «in-memory stubs — no external
  services required», переводятся в обычные юнит-тесты и снова охраняют
  дефолтный `go test ./...`. 2 файла с реальной инфраструктурой
  (`scheduling/store`, `scheduling/tokenindex`) тег сохраняют.
- **Конфигурация линтера (DC3).** `.golangci.yml` (v2) с набором
  errcheck/govet/staticcheck/revive/gosec/bodyclose/sqlclosecheck/nilerr/
  modernize/misspell/errname/paralleltest. В CI запускается с
  `only-new-issues: true` — гейтит новый код, не блокируя на накопленном
  долге (его закрывают профильные чейнджи при правках своих файлов).
- **Пин инструментов и автообновление (DC4).** `tool`-директивы для
  golangci-lint и govulncheck (репродуцируемые версии) + `.github/dependabot.yml`
  (gomod по всем модулям + github-actions).
- **`go.sum` для e2e (DC6).** `go mod tidy` в `tests/e2e`; если появятся
  записи — `go.sum` коммитится.
- **govulncheck сейчас информационный** (`continue-on-error: true`): он
  немедленно подсветит две достижимые уязвимости auth (DC1), но их фикс — это
  CH02, который и промоутит джоб в блокирующий. Так main остаётся зелёным, а
  PR'ы не сериализуются за CH02.

Изменений API, схем БД и контрактов событий RabbitMQ нет. **BREAKING**-изменений нет.

## Capabilities

### New Capabilities

<!-- Новых capability не вводится: чистый infra/tooling-чейндж. -->

### Modified Capabilities

<!-- Наблюдаемое поведение продуктовых capability не меняется → дельта-спеков нет
     (прецедент: harden-auth-shell — «инфраструктурный рефактор без смены поведения»). -->

## Impact

- **Затронутые сервисы (как цели CI-матрицы, код сервисов не меняется):**
  `pkg`, `services/escalation`, `services/incident`, `services/ingestion`,
  `services/notification`, `services/scheduling`, `tests/e2e`.
- **События RabbitMQ:** не затрагиваются (ни топология, ни payload, ни теги).
- **Тесты:** 7 стаб-файлов перестают требовать тег `integration` и попадают в
  дефолтный прогон; 2 реально-инфраструктурных теста (`scheduling/store`,
  `scheduling/tokenindex`) — без изменений.
- **Зависимости/инструменты:** добавляются `tool`-директивы (golangci-lint,
  govulncheck); версии самих библиотек не меняются (бамп уязвимых auth-депов —
  CH02).
- **Новые файлы:** `.github/workflows/ci.yml`, `.github/workflows/e2e.yml`,
  `.github/dependabot.yml`, `.golangci.yml`, возможно `tests/e2e/go.sum`.
- **Хэндофф для CH02:** govulncheck-джоб введён информационным; CH02 снимает
  `continue-on-error`, сделав его блокирующим гейтом после фикса DC1.
