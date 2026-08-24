# План: Сервис ротации баннеров

**Дата:** 2025-08-24
**Спека:** [02-banners-rotation.md](../../02-banners-rotation.md)
**Статус:** черновик

---

## ⚠️ Обязательные требования (гейт)

При невыполнении **любого** из требований ниже — максимальная оценка **4 балла** (незачёт):

| # | Требование | Где реализуется |
|---|------------|-----------------|
| O1 | Юнит-тесты на core-логику (бандит) | Задача 2 |
| O2 | Валидный Dockerfile и Makefile | Задача 8 |
| O3 | CI/CD-пайплайн на ветке `master` | Задача 10 |
| O3a | `golangci-lint` (последняя версия) с конфигом `.golangci.yml` | Задача 9 + 10 |
| O3b | Юнит-тесты: `go test -race -count 100` | Задача 10 |
| O3c | Сборка бинаря для Go ≥ 1.25 | Задача 8 + 10 |

---

## Архитектурные решения

| Область | Выбор | Обоснование |
|---------|-------|-------------|
| Язык | Go (stdlib в core; внешние зависимости в infra-слое) | Core-логика (`bandit`, `service`, `model`) — только stdlib; инфраструктура (`repo`, `event`, `handler`) — внешние библиотеки разрешены |
| API | REST (`net/http` + `chi` router) | Проще gRPC; спека допускает любой вариант; chi — легковесный роутер |
| База данных | SQLite (через `modernc.org/sqlite`, pure Go, без cgo) | Внешняя БД для такого масштаба не нужна |
| Очередь | RabbitMQ (`streadway/amqp`, реальная в проде, мок в тестах) | Спека упоминает очередь (kafka как пример); RabbitMQ проще: 1 контейнер, нет Zookeeper |
| Алгоритм бандита | Thompson Sampling (Beta-Bernoulli, stdlib only) | Core-логика без зависимостей; хорошо работает с разреженными данными |
| Деплой | Docker Compose (app + RabbitMQ) | Спека требует `make run` → `docker compose up`; RabbitMQ — 1 контейнер |
| CI/CD | GitHub Actions | Репозиторий на GitHub; минимальная настройка |
| Тестирование | stdlib `testing` + в-memory SQLite + мок-публикатор | Быстро, инфраструктура для unit-тестов не нужна |

**Правило:** Сторонние библиотеки **не допускаются в core-логике** (`internal/bandit`, `internal/service`, `internal/model`). Инфраструктурные пакеты (`internal/repo`, `internal/event`, `internal/handler`) могут использовать внешние зависимости.

---

## Структура проекта

```
├── .github/
│   └── workflows/
│       └── ci.yaml              # CI/CD пайплайн (lint + test + build)
├── .golangci.yml                # Конфиг golangci-lint
├── cmd/
│   └── server/
│       └── main.go              # Точка входа HTTP-сервера
├── internal/
│   ├── bandit/                  # CORE (stdlib only)
│   │   ├── thompson.go          # Thompson Sampling
│   │   └── thompson_test.go     # Unit-тесты
│   ├── repo/                    # INFRA (sqlite — external dep)
│   │   ├── repo.go              # Репозиторий БД (SQLite)
│   │   └── repo_test.go         # Тесты репозитория
│   ├── service/                 # CORE (stdlib only)
│   │   ├── service.go           # Бизнес-логика
│   │   ├── service_test.go      # Тесты сервисного слоя
│   │   └── integration_test.go  # Интеграционные тесты (e2e)
│   ├── handler/                 # INFRA (chi — external dep)
│   │   └── handler.go           # HTTP-хендлеры
│   ├── event/                   # INFRA (amqp — external dep)
│   │   ├── publisher.go         # Интерфейс `Publisher` + NOPublisher
│   │   ├── rabbitmq.go          # Реальная RabbitMQ-реализация
│   │   └── publisher_test.go    # Тесты NOPublisher
│   └── model/                   # CORE (stdlib only)
│       └── model.go             # Доменные модели (Slot, Banner, Event)
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── go.mod / go.sum
└── docs/
    └── plans/
        └── 20250824-banner-rotation.md  (этот файл)
```

---

## Разбалловка (по спеке)

| Критерий | Баллы | Задача |
|----------|-------|--------|
| Алгоритм «многорукого бандита» | 2 | 2 |
| Разделение на «слоты» и «соц.дем. группы» | 2 | 4 |
| API сервиса | 2 | 6 |
| Отправка статистики в очередь | 1 | 5 |
| Юнит-тесты | 1 | 2 |
| Интеграционные тесты | 2 | 7 |
| Тесты адекватны и покрывают функциональность | 1 | 2, 7 |
| `make build` / `make run` / `make test` | 1 | 8 |
| Понятность и чистота кода | 0–3 | 9 |
| **Итого** | **10–15** | |

**Незачёт (макс. 4 балла)** — при невыполнении хотя бы одного обязательного требования [↑ гейт](#-обязательные-требования-гейт).

---

## Задачи

### Задача 1 — Каркас проекта
**Цель:** Настроить структуру проекта, зависимости и средства сборки.

- [ ] 1.1. Убрать `main.go` наверх, создать `cmd/server/main.go` с минимальной `main()`
- [ ] 1.2. Добавить зависимости в `go.mod` (только infra-зависимости):
  - `github.com/go-chi/chi/v5` — HTTP-роутер (handler)
  - `modernc.org/sqlite` — SQLite-драйвер (repo, pure Go)
  - `github.com/streadway/amqp` — AMQP-клиент для RabbitMQ (event)
- [ ] 1.3. Создать `internal/model/model.go` с доменными типами (**stdlib only**):
  - `Slot{ID, Description}`
  - `Banner{ID, Description}`
  - `SlotBanner{SlotID, BannerID}` (связь, 1 баннер может быть в нескольких слотах)
  - `BannerStats{SlotID, BannerID, GroupID, Impressions, Clicks}` (на слот на группу)
  - `Event{Type: "click"|"impression", SlotID, BannerID, GroupID, Timestamp}`
- [ ] 1.4. Создать `Makefile` с целевыми правилами: `build`, `run`, `test`, `clean`
- [ ] 1.5. Убедиться, что `go build ./...` компилируется

**Тесты:** Проверка компиляции.
**Оценка:** 1 час

---

### Задача 2 — Алгоритм многоорукого бандита (CORE, stdlib only)
**Цель:** Реализовать и протестировать алгоритм Thompson Sampling изолированно.
**Покрытие:** ⚠️ O1, 2 балла + 1 очко (unit-тесты) + 1 очко (качество тестов)

- [ ] 2.1. Создать `internal/bandit/thompson.go`:
  - `type Bandit struct` — хранит Beta-распределения на каждую руку (alpha, beta)
  - `NewBandit() *Bandit`
  - `AddArm(id string)` — зарегистрировать новую руку (баннер)
  - `RemoveArm(id string)` — удалить руку
  - `Update(id string, clicked bool)` — обновить параметры Beta (alpha+=1 при клике, beta+=1 при отсутствии)
  - `Pick() string` — сэмплировать из Beta-распределения каждой руки, вернуть руку с наибольшим значением
  - `PickWithImpression() string` — выбрать + увеличить счётчик показов (beta+=1 для выбранной руки)
- [ ] 2.2. Создать `internal/bandit/thompson_test.go`:
  - `TestPickReturnsExistingArm` — выбор возвращает только зарегистрированные руки
  - `TestEachArmShownAtLeastOnce` — после N выборов (N >> кол-во рук), каждая рука выбрана ≥1 раза
  - `TestPopularArmGetsMoreImpressions` — если одна рука получает клики, со временем она выбирается значительно чаще
  - `TestAddRemoveArm` — руки можно динамически добавлять/удалять
  - `TestEmptyBanditReturnsEmpty` — граничный случай: нет рук → пустая строка
  - `TestBetaParametersUpdateCorrectly` — проверить математику alpha/beta после кликов/не-кликов
  - **Тесты должны проходить при `go test -race -count 100`** (дeterministic, без data races)

**Тесты:** Все подзадачи в 2.2.
**Очки:** 2 очка (бандит) + 1 очко (unit-тесты) + 1 очко (качество тестов) = **4 балла**
**Оценка:** 2 часа

---

### Задача 3 — Репозиторий базы данных (INFRA, sqlite — external dep)
**Цель:** Персистентность на SQLite для слотов, баннеров и статистики.

- [ ] 3.1. Создать `internal/repo/repo.go`:
  - Структура `Repo`, оборачивающая `*sql.DB`
  - `NewRepo(dsn string) (*Repo, error)` — открывает БД, запускает миграции
  - SQL-схема:
    ```sql
    CREATE TABLE slots (id TEXT PRIMARY KEY, description TEXT);
    CREATE TABLE banners (id TEXT PRIMARY KEY, description TEXT);
    CREATE TABLE slot_banners (slot_id TEXT, banner_id TEXT, PRIMARY KEY(slot_id, banner_id));
    CREATE TABLE banner_stats (
      slot_id TEXT, banner_id TEXT, group_id TEXT,
      impressions INTEGER DEFAULT 0, clicks INTEGER DEFAULT 0,
      PRIMARY KEY (slot_id, banner_id, group_id)
    );
    ```
  - `CreateSlot(id, desc string) error`
  - `CreateBanner(id, desc string) error`
  - `AddBannerToSlot(slotID, bannerID string) error`
  - `RemoveBannerFromSlot(slotID, bannerID string) error`
  - `GetBannersForSlot(slotID string) ([]string, error)` — возвращает ID баннеров
  - `GetBannerStats(slotID, bannerID, groupID string) (impressions, clicks int, error)`
  - `IncrementImpressions(slotID, bannerID, groupID string) error`
  - `IncrementClicks(slotID, bannerID, groupID string) error`
- [ ] 3.2. Создать `internal/repo/repo_test.go`:
  - Использовать `file::memory:?cache=shared` для встраиваемого SQLite в памяти
  - Протестировать каждый метод на корректность
  - Протестировать конкурентный доступ (несколько горутин, увеличивающих статистику)

**Тесты:** Все подзадачи в 3.2.
**Оценка:** 2 часа

---

### Задача 4 — Сервисный слой (CORE, stdlib only)
**Цель:** Бизнес-логика, оркестрация бандита + репозитория + публикатора.

- [ ] 4.1. Создать `internal/service/service.go`:
  - Структура `Service` с `Repo` (интерфейс), `Publisher` (интерфейс) и картой `*bandit.Bandit` на каждую пару (slotID, groupID)
  - `NewService(repo Repo, publisher event.Publisher) *Service`
  - `AddBanner(slotID, bannerID string) error` — добавить баннер в слот, обновить бандит
  - `RemoveBanner(slotID, bannerID string) error` — удалить из слота, обновить бандит
  - `PickBanner(slotID, groupID string) (bannerID string, err error)` —
    1. Получить баннеры для слота из репозитория
    2. Получить или создать бандит для (slotID, groupID)
    3. Синхронизировать руки бандита с текущими баннерами (добавить/удалить по необходимости)
    4. Выбрать баннер через bandit.PickWithImpression()
    5. Увеличить показы в репозитории
    6. Опубликовать событие показа
    7. Вернуть ID баннера
  - `RegisterClick(slotID, bannerID, groupID string) error` —
    1. Увеличить клики в репозитории
    2. Обновить бандит с clicked=true
    3. Опубликовать событие клика
- [ ] 4.2. Создать `internal/service/service_test.go`:
  - Использовать мок `Publisher` (считать события)
  - Интерфейсный `Repo` — мок или в-memory SQLite через repo-адаптер
  - `TestPickBannerReturnsValidBanner` — возвращённый баннер существует в слоте
  - `TestPickBannerIncrementsImpressions` — счётчик показов увеличивается
  - `TestRegisterClickIncrementsClicks` — счётчик кликов увеличивается
  - `TestEventsPublished` — события показов и кликов отправляются в публикатор

**Тесты:** Все подзадачи в 4.2.
**Очки:** 2 очка (разделение слотов и групп)
**Оценка:** 2 часа

---

### Задача 5 — Публикатор событий (INFRA, amqp — external dep)
**Цель:** Публикация событий показов/кликов в RabbitMQ.

- [ ] 5.1. Создать `internal/event/publisher.go`:
  - `type Publisher interface { Publish(ctx context.Context, event model.Event) error }`
  - `type NOPublisher struct {}` — no-op для тестов
- [ ] 5.2. Создать `internal/event/rabbitmq.go`:
  - `type RabbitMQPublisher struct { conn *amqp.Conn; ch *amqp.Channel }`
  - `NewRabbitMQPublisher(url string) (*RabbitMQPublisher, error)`
  - `Publish(ctx, event)` — сериализовать событие в JSON (`encoding/json`, stdlib), publicar в очередь
  - Декларация exchange + queue при инициализации (auto-declare)
- [ ] 5.3. Добавить URL RabbitMQ-брокера в флаги/переменные окружения сервера (`RABBITMQ_URL`)

**Тесты:** RabbitMQ-публикатор тестируется в интеграционных тестах (Задача 7). Unit-тест для NOPublisher.
**Очки:** 1 очко (статистика в очередь)
**Оценка:** 1.5 часа

---

### Задача 6 — REST API (INFRA, chi — external dep)
**Цель:** HTTP-эндпоинты для сервиса.

- [ ] 6.1. Создать `internal/handler/handler.go`:
  - `AddBanner(w http.ResponseWriter, r *http.Request)` — `POST /slots/:slotID/banners`
    - Тело: `{"banner_id": "...", "description": "..."}`
  - `RemoveBanner(w, r)` — `DELETE /slots/:slotID/banners/:bannerID`
  - `PickBanner(w, r)` — `POST /slots/:slotID/pick?groupID=...`
    - Ответ: `{"banner_id": "..."}`
  - `RegisterClick(w, r)` — `POST /slots/:slotID/banners/:bannerID/click?groupID=...`
    - Ответ: `204 No Content`
  - `CreateSlot(w, r)` — `POST /slots`
    - Тело: `{"id": "...", "description": "..."}`
- [ ] 6.2. Подключить хендлеры в `cmd/server/main.go`:
  - Инициализировать SQLite-репозиторий, RabbitMQ-публикатор, сервис
  - Настроить chi-роутер с маршрутами
  - Грациозное завершение по SIGTERM
- [ ] 6.3. Добавить валидацию запросов (возвращать 400 при отсутствии параметров)

**Тесты:** Интеграционные тесты (Задача 7).
**Очки:** 2 очка (API)
**Оценка:** 1.5 часа

---

### Задача 7 — Интеграционные тесты
**Цель:** End-to-end тесты API с реальным HTTP-сервером.

- [ ] 7.1. Создать `internal/service/integration_test.go`:
  - Запустить HTTP-сервер на случайном порту с в-memory SQLite + NOPublisher
  - `TestEndToEnd_PickAndClick`:
    1. Создать слот, добавить 3 баннера
    2. Выбрать баннер 100 раз → каждый баннер показан ≥1 раза
    3. Зарегистрировать клики только на баннер A
    4. Выбрать баннер ещё 200 раз → баннер A получает значительно больше выборов
  - `TestEndToEnd_RemoveBanner`:
    1. Добавить баннеры, выбрать, удалить один, выбрать снова → удалённый баннер никогда не возвращается
  - `TestEndToEnd_EmptySlot`:
    1. Выбрать из слота без баннеров → 404 или 400
  - `TestEndToEnd_ClickOnUnknownBanner`:
    1. Кликнуть на баннер, которого нет в слоте → 404
- [ ] 7.2. Опционально: интеграционный тест RabbitMQ с реальным брокером в Docker

**Тесты:** Все подзадачи в 7.1.
**Очки:** 2 очка (интеграционные тесты)
**Оценка:** 2 часа

---

### Задача 8 — Docker + Makefile ⚠️ O2, O3c
**Цель:** `make run` запускает сервис со всеми зависимостями; сборка для Go ≥ 1.25.

- [ ] 8.1. Создать `Dockerfile` для Go-сервиса:
  - Мультистейдж-сборка (`golang:1.25-alpine` builder → `alpine`)
  - EXPOSE порт
  - **Go ≥ 1.25** (обязательное требование)
- [ ] 8.2. Создать `docker-compose.yml`:
  - `app`: собирается из Dockerfile, монтирует конфиг, зависит от rabbitmq
  - `rabbitmq`: `rabbitmq:3-management`
- [ ] 8.3. Доработать `Makefile`:
  - `make build` → `go build -o bin/server ./cmd/server`
  - `make run` → `docker compose up --build`
  - `make test` → `go test -race -count 100 ./...` (**обязательный формат**)
  - `make clean` → `rm -rf bin/`
- [ ] 8.4. Добавить `.dockerignore`
- [ ] 8.5. Убедиться, что `go.mod` указывает `go 1.25` или выше

**Тесты:** Ручная проверка: `make build && make test` проходит успешно.
**Очки:** 1 очко (`make build/run/test`)
**Оценка:** 1.5 часа

---

### Задача 9 — golangci-lint + качество кода
**Цель:** Чистый код, lint-конфиг, обработка ошибок, документация.
**Покрытие:** ⚠️ O3a (частично)

- [ ] 9.1. Создать `.golangci.yml`:
  - Включить linters: `errcheck`, `gosimple`, `go vet`, `goimports`, `ineffassign`, `staticcheck`, `typecheck`, `unused`
  - Настроить `run.timeout = 5m`
- [ ] 9.2. Добавить обработку ошибок повсеместно (без тихих провалов)
- [ ] 9.3. Добавить timeout контекста в HTTP-хендлеры
- [ ] 9.4. Добавить README.md с:
  - Описанием проекта
  - Эндпоинтами API
  - Как запустить (`make run`)
  - Как протестировать (`make test`)
- [ ] 9.5. Запустить `golangci-lint run ./...` → 0 warnings
- [ ] 9.6. Убедиться, что все тесты проходят с `go test -race -count 100 ./...`
- [ ] 9.7. Убедиться, что `internal/bandit`, `internal/service`, `internal/model` не импортируют сторонние пакеты

**Очки:** до 3 очков (чистота кода)
**Оценка:** 1.5 часа

---

### Задача 10 — CI/CD пайплайн ⚠️ O3
**Цель:** GitHub Actions на ветке `master`: lint → test → build.

- [ ] 10.1. Создать `.github/workflows/ci.yaml`:
  - Триггер: `push` и `pull_request` на `master`
  - Job `lint`:
    - `uses: golangci/golangci-lint-action@v6`
    - Версия: latest
  - Job `test`:
    - `go test -race -count 100 ./...`
  - Job `build`:
    - `go build -o bin/server ./cmd/server`
    - Go ≥ 1.25 (через `go-version: '1.25'`)
- [ ] 10.2. Убедиться, что пайплайн проходит на ветке `master` (зелёный чек)

**Тесты:** Пайплайн проходит успешно.
**Оценка:** 1 час

---

## Сводка

| № | Задача | Баллы | Оц. | Гейт |
|---|--------|-------|-----|------|
| 1 | Каркас проекта | 0 | 1ч | |
| 2 | Алгоритм бандита + unit-тесты | 4 | 2ч | ⚠️ O1 |
| 3 | Репозиторий БД (infra) | 0 | 2ч | |
| 4 | Сервисный слой (core) | 2 | 2ч | |
| 5 | Публикатор событий (infra) | 1 | 1.5ч | |
| 6 | REST API (infra) | 2 | 1.5ч | |
| 7 | Интеграционные тесты | 2 | 2ч | |
| 8 | Docker + Makefile | 1 | 1.5ч | ⚠️ O2, O3c |
| 9 | golangci-lint + качество | 0-3 | 1.5ч | ⚠️ O3a |
| 10 | CI/CD пайплайн | 0 | 1ч | ⚠️ O3 |
| | **Итого** | **10–15** | **~16ч** | |

**Незачёт (макс. 4 балла):** любое из O1–O3 невыполнено.
**Минимум для зачёта (10 балл.):** Задачи 2, 4, 5, 6, 7, 8, 10 полностью + базовая Задача 9.
**Полный балл (15 балл.):** Все задачи, зелёный CI, `golangci-lint` без варнингов, `go test -race -count 100` проходит.