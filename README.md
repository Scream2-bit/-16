# GoShort

Веб-сервис для сокращения ссылок со статистикой переходов. Курсовая работа по МДК.01.01 «Разработка программных модулей».

Что умеет:

- сокращает ссылку до кода из 6 символов или до своего псевдонима (`/open-day`);
- ограничивает срок действия ссылки;
- считает переходы по дням и по источникам, превью-боты мессенджеров не учитываются;
- отдает статистику на отдельной странице и через API.

Данные хранятся в SQLite (один файл), сервис собирается в один бинарник.

## Запуск

Нужен Go 1.26+.

```bash
go build -o goshort ./cmd/goshort
./goshort -addr :8080 -base-url http://localhost:8080
```

Параметры:

| Флаг | Переменная окружения | По умолчанию | Описание |
|---|---|---|---|
| `-addr` | `GOSHORT_ADDR` | `:8080` | адрес HTTP-сервера |
| `-base-url` | `GOSHORT_BASE_URL` | `http://localhost:8080` | внешний адрес сервиса |
| `-db` | `GOSHORT_DB` | `goshort.db` | файл базы данных |
| `-code-len` | `GOSHORT_CODE_LEN` | `6` | длина кода (4–12) |

## API

| Метод | Описание |
|---|---|
| `POST /api/shorten` | создать короткую ссылку |
| `GET /api/links/{code}` | информация и статистика по ссылке |
| `GET /{code}` | редирект на исходный адрес |
| `GET /stats/{code}` | страница статистики |
| `GET /health` | проверка работоспособности |

Пример:

```bash
curl -X POST localhost:8080/api/shorten -d '{"url":"https://go.dev/blog","alias":"goblog","ttl_hours":24}'
```

```json
{
  "code": "goblog",
  "short_url": "http://localhost:8080/goblog",
  "original_url": "https://go.dev/blog",
  "created_at": "2026-10-20T20:02:42Z",
  "expires_at": "2026-10-21T20:02:42Z",
  "clicks": 0
}
```

Ошибки возвращаются в виде `{"error": "..."}` с кодом 400, 404, 409 или 410.

## Тесты

```bash
go test -cover ./...
```

## Структура

```
cmd/goshort          точка входа
internal/config      параметры запуска
internal/shortener   бизнес-логика: проверка URL, генерация кодов, статистика
internal/storage     работа с SQLite
internal/httpapi     HTTP-обработчики
web                  шаблоны, стили, скрипт
```
