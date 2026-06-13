# Tasks — add-ci-and-quality-gates

> Каждая задача привязана к находке аудита (docs/audit/08-testing.md,
> docs/audit/11-dependencies-ci.md) и, где применимо, к file:line.
> Объём строго по «Матрице покрытия» CH01: T1, T2, DC2, DC3, DC4, DC6.

## 1. CI-пайплайн (T1, DC2)

- [x] 1.1 T1/DC2 — `.github/workflows/ci.yml`: триггеры push/PR в `main`,
      `permissions: contents: read`, матрица по 7 модулям `go.work`, `fail-fast: false`
- [x] 1.2 T1/DC2 — джоб `build-test`: `go build ./...`,
      `go test -race -shuffle=on ./...`, проверка `go mod tidy` (`git diff --exit-code`)
- [x] 1.3 DC3/T1 — джоб `lint`: golangci-lint из `./tools`; на PR
      `--new-from-merge-base` (блокирует новый код), на push в main —
      информационно (накопленный долг не блокирует)
- [x] 1.4 DC2 — джоб `govulncheck`: матрица по модулям, **`continue-on-error: true`**
      (информационный; CH02 промоутит в блокирующий после фикса DC1)
- [x] 1.5 T1 — джоб `integration`: `services:` Postgres/Redis, применение
      миграций scheduling через psql, `go test -tags integration -race` (scheduling)
- [x] 1.6 T1 — `.github/workflows/e2e.yml`: e2e `workflow_dispatch`-only
      (никогда не блокирует поток; schedule закомментирован до стабилизации стека)

## 2. Снятие тега со стаб-тестов (T2)

> docs/audit/08-testing.md §T2: 7 файлов декларируют «in-memory stubs — no
> external services required», но спрятаны за `//go:build integration`.

- [x] 2.1 T2 — снят `//go:build integration` (+ комментарии «Run with…/Uses…») с
      7 стаб-файлов: escalation/incident/notification `consumer`,
      escalation/incident/scheduling `handler`, ingestion `webhook`
- [x] 2.2 T2 — НЕ тронуты (нужна реальная инфра, тег уместен):
      `services/scheduling/internal/store/store_integration_test.go`,
      `services/scheduling/internal/tokenindex/index_integration_test.go`
- [x] 2.3 T2 — файлы переименованы `*_integration_test.go` → `*_test.go`
      (git-rename), т.к. имя без тега вводит в заблуждение
- [x] 2.4 T2 — починены два протухших стаб-теста notification (компилировались
      за тегом, разошлись с кодом): `notifier.New` получил аргумент
      `frontendBaseURL`; `TestConsumer_ExhaustedEvent_PostsToMattermost` —
      добавлен `MattermostEnabled: true` (поведение из чейнджа
      split-notification-config-mattermost-email)

## 3. Конфигурация линтера (DC3)

- [x] 3.1 DC3 — `.golangci.yml` (v2): default standard + revive, gosec, bodyclose,
      sqlclosecheck, nilerr, modernize, misspell, errname, paralleltest
- [x] 3.2 DC3 — исключения для тестов (gosec/errcheck/bodyclose) и миграций;
      `build-tags: [integration, e2e]`, чтобы линтер видел инфра-тесты

## 4. Пин инструментов и автообновление (DC4)

- [x] 4.1 DC4 — изолированный модуль `tools/` (вне `go.work`) с `tool`-директивами
      golangci-lint v2.12.2 и govulncheck v1.3.0 (единый источник версий; не
      раздувает build-модули). CI собирает бинари из него.
- [x] 4.2 DC4 — `.github/dependabot.yml`: `gomod` по всем модулям + `tools` +
      `github-actions`; группировка minor/patch

## 5. go.sum для e2e (DC6)

- [x] 5.1 DC6 — `go mod tidy` в `tests/e2e`: зависимостей нет (только stdlib),
      `go.sum` не требуется — норма (см. 11-dependencies-ci.md §DC6)

## 6. Верификация

- [x] 6.1 `go build ./...` и `go test -race -shuffle=on ./...` по всем 7 модулям — зелено
- [x] 6.2 Снятые с тега стаб-тесты проходят в дефолтном прогоне (T2 закрыт)
- [x] 6.3 `golangci-lint run` отрабатывает; baseline 188 замечаний — это долг
      других чейнджей (paralleltest→CH17, revive/modernize→CH18, errcheck→CH08),
      `only-new-issues` его не блокирует
- [x] 6.4 YAML воркфлоу/конфигов валиден; tools-бинари собираются и запускаются;
      govulncheck подтверждает DC1 (валидирует «информационный сейчас»)
- [x] 6.5 `/opsx:verify` — критичных проблем нет (no-delta tooling-чейндж → архив с `--skip-specs`)
- [x] 6.6 `docs/audit/00-roadmap.md`: CH01 → ✅ + хэндофф-блок; пометка в строке CH02
      (снять `continue-on-error` с govulncheck). `docs/spec-vs-code-audit.md` НЕ трогаем — capability не менялась.

## Backlog (вне объёма CH01 — записать для профильных чейнджей)

- [ ] CH17/T5 — `go vet` httpresponse «using resp before checking for errors» в
      `handler_test.go` (escalation/incident/scheduling): `resp, _ := http.Get/Post/Do`
      затем `defer resp.Body.Close()` — латентный nil-deref. Вскрыто снятием тега;
      не чинится здесь (тест-гигиена — объём CH17).
