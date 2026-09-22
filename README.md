# Гофермарт — накопительная система лояльности

HTTP-сервис накопительной системы лояльности «Гофермарт»: регистрация и аутентификация
пользователей, приём номеров заказов, расчёт начислений через внешнюю систему
и списание баллов. Полное техническое задание — в [SPECIFICATION.md](SPECIFICATION.md).

## Быстрый старт

Требования: Go 1.26+, PostgreSQL 14+ (или Docker).

```bash
# 1. PostgreSQL (пропустите, если база уже есть)
docker compose up -d

# 2. Система расчёта начислений (порт 8081)
./cmd/accrual/accrual_linux_amd64 -a localhost:8081   # Linux
#  на Windows скопируйте cmd/accrual/accrual_windows_amd64 в accrual.exe

# 3. Сервис лояльности
go run ./cmd/gophermart \
  -a localhost:8080 \
  -d "postgres://postgres:postgres@localhost:5432/praktikum?sslmode=disable" \
  -r http://localhost:8081
```

Миграции схемы применяются автоматически при старте.

## Конфигурация

Переменные окружения имеют приоритет над флагами.

| Флаг | Переменная окружения | Описание | По умолчанию |
|------|----------------------|----------|--------------|
| `-a` | `RUN_ADDRESS` | адрес и порт HTTP-сервера | `localhost:8080` |
| `-d` | `DATABASE_URI` | строка подключения к PostgreSQL | — (обязательно) |
| `-r` | `ACCRUAL_SYSTEM_ADDRESS` | адрес системы расчёта начислений | — (без него заказы остаются в `NEW`) |
| `-s` | `JWT_SECRET` | ключ подписи токенов | случайный при каждом запуске |
| `-token-ttl` | `TOKEN_TTL` | время жизни токена | `24h` |
| `-poll-interval` | `WORKER_POLL_INTERVAL` | пауза между опросами системы начислений | `1s` |
| `-batch-size` | `WORKER_BATCH_SIZE` | заказов за один опрос | `20` |
| `-concurrency` | `WORKER_CONCURRENCY` | параллельных запросов к системе начислений | `5` |
| `-shutdown-timeout` | `SHUTDOWN_TIMEOUT` | таймаут graceful shutdown | `10s` |

## HTTP API

| Метод и путь | Назначение | Коды ответа |
|--------------|------------|-------------|
| `POST /api/user/register` | регистрация `{"login","password"}` | 200, 400, 409, 500 |
| `POST /api/user/login` | аутентификация `{"login","password"}` | 200, 400, 401, 500 |
| `POST /api/user/orders` | загрузка номера заказа (`text/plain`) | 200, 202, 400, 401, 409, 422, 500 |
| `GET /api/user/orders` | список заказов, новые первыми | 200, 204, 401, 500 |
| `GET /api/user/balance` | текущий баланс `{"current","withdrawn"}` | 200, 401, 500 |
| `POST /api/user/balance/withdraw` | списание `{"order","sum"}` | 200, 400, 401, 402, 422, 500 |
| `GET /api/user/withdrawals` | список списаний, новые первыми | 200, 204, 401, 500 |

После регистрации или входа токен возвращается **и** в заголовке
`Authorization: Bearer <token>`, **и** в cookie `auth_token`; для защищённых
хендлеров подходит любой из способов. Номера заказов проверяются алгоритмом
Луна. Сжатие `gzip` поддерживается в обе стороны (`Content-Encoding` запроса,
`Accept-Encoding` ответа).

## Устройство

```
cmd/gophermart          точка входа
internal/config         флаги и переменные окружения
internal/model          доменные сущности и sentinel-ошибки
internal/luhn           алгоритм Луна
internal/auth           bcrypt-хеширование паролей, JWT-токены
internal/storage/postgres  хранилище на pgx + встроенные goose-миграции
internal/service        бизнес-логика (пользователи, заказы, баланс)
internal/handler        HTTP-хендлеры и роутер (chi)
internal/middleware     аутентификация, gzip, логирование запросов
internal/accrual        клиент системы расчёта начислений
internal/worker         фоновый опрос системы начислений
internal/app            сборка зависимостей и жизненный цикл
```

Слои общаются через небольшие интерфейсы, поэтому каждый тестируется отдельно.
Деньги во всей доменной модели — тип `model.Money`: целое число копеек
(сотых балла) поверх `int64`, в базе — `BIGINT`. Сложение, вычитание и
сравнение балансов точны, `float64` в денежных путях нет. На границе HTTP
`Money` сериализуется в обычное JSON-число (`729.98`), так что формат API из
спецификации не меняется; при разборе запроса значение округляется до копейки
(половина — от нуля). Начисление и списание выполняются в
транзакциях: `UPDATE ... WHERE balance >= sum` делает списание атомарным, а
заказ переводится в финальный статус ровно один раз, так что повторный ответ
системы начислений не зачисляет баллы дважды. При ответе `429` воркер делает
паузу на `Retry-After`.

## Тесты

```bash
go test ./...
```

Юнит-тесты не требуют внешних зависимостей (хранилище тестируется на `pgxmock`).
Покрытие:

```bash
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
```

Интеграционные тесты (реальная PostgreSQL, миграции, сквозной сценарий через
HTTP с заглушкой системы начислений) запускаются, когда задана
`TEST_DATABASE_URI`. Каждый пакет работает в собственной схеме, которая
пересоздаётся при запуске, поэтому можно указывать ту же базу, что и для
разработки:

```bash
TEST_DATABASE_URI="postgres://postgres:postgres@localhost:5432/praktikum?sslmode=disable" go test ./...
```

Статический анализ: `go vet ./...` и `staticcheck ./...`.

## Обновление шаблона

Чтобы получать обновления автотестов и других частей шаблона:

```bash
git remote add -m master template https://github.com/yandex-praktikum/go-musthave-diploma-tpl.git
git fetch template && git checkout template/master .github
```
