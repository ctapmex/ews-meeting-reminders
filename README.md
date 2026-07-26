# EWS Meeting Reminders

Демон для Linux: напоминания о встречах из **MS Exchange (EWS)** за фиксированное время до начала (по умолчанию **5** и **0** минут). Время задаёт сам сервис, а не значение reminder из события.

Удобно использовать вместе:
 - с Evolution - у клиента напоминания берутся из встречи и срабатывают один раз.
 - с web-версией почты - напоминания появляются не на одной из многочисленных вкладок, а на рабочем столе.

## Возможности

- Опрос календаря по протоколу/api EWS (аутентификация NTLM / Basic)
- Desktop-уведомления с кнопками «Открыть ссылку» / «Пропустить» / «Пропустить все»
- Несколько встреч → одна очередь (новые встают в хвост)
- Ссылки на подключение: из поля Location встречи или URL в теле (фильтр hostname настраивается)
- Режим `-list` — встречи на ближайшие сутки в консоль
- Статический Go-бинарник; user systemd unit

## Требования

- MS Exchange EWS (`ntlm` или `basic`)
- Графическая сессия Linux (D-Bus) для уведомлений
- `xdg-open` — для кнопки «Открыть ссылку» (или своя команда в конфиге)
- Для сборки из исходников: **Go 1.26+** либо Docker

## Установка из релиза

Архив релиза: `ews-meeting-reminders-<версия>-linux-amd64.tar.gz` (бинарник, пример конфига, unit, LICENSE, README).

```bash
VERSION=0.1.0   # подставьте версию из релиза
ARCHIVE=ews-meeting-reminders-${VERSION}-linux-amd64
# скачайте и распакуйте ARCHIVE.tar.gz

install -d ~/.local/share/ews-meeting-reminders \
           ~/.config/ews-meeting-reminders \
           ~/.config/systemd/user

install -m 0755 "${ARCHIVE}/ews-reminders" \
  ~/.local/share/ews-meeting-reminders/ews-reminders
install -m 0600 "${ARCHIVE}/config.example.yaml" \
  ~/.config/ews-meeting-reminders/config.yaml
install -m 0644 "${ARCHIVE}/systemd/ews-meeting-reminders.service" \
  ~/.config/systemd/user/ews-meeting-reminders.service

# заполните server / email / username в config.yaml
echo 'EWS_PASSWORD=секрет' > ~/.config/ews-meeting-reminders/env
chmod 600 ~/.config/ews-meeting-reminders/env

systemctl --user daemon-reload
systemctl --user enable --now ews-meeting-reminders.service
```

Проверка:

```bash
~/.local/share/ews-meeting-reminders/ews-reminders -list
journalctl --user -u ews-meeting-reminders.service -f
```

Сервис рассчитан на сессию после входа в графику.

## Установка из исходников

Нужен Go 1.26+. Скрипт соберёт бинарник, положит его в `~/.local/share/...`, при отсутствии конфига скопирует пример и включит user unit.

```bash
cd ews-meeting-reminders

# опционально — заранее; иначе создаст install.sh
mkdir -p ~/.config/ews-meeting-reminders
cp config.example.yaml ~/.config/ews-meeting-reminders/config.yaml
# заполнить server / email / username

echo 'EWS_PASSWORD=секрет' > ~/.config/ews-meeting-reminders/env
chmod 600 ~/.config/ews-meeting-reminders/env

./install.sh
```

### Сборка без локального Go (Docker)

Только сборка бинарника; запуск всё равно на хосте:

```bash
./docker-build.sh          # → ./bin/ews-reminders
./bin/ews-reminders -version
```

Дальше — как в разделе «из релиза» (бинарник + конфиг + unit) или вручную:

```bash
./bin/ews-reminders -list
./bin/ews-reminders        # демон
```

## Запуск вручную

```bash
make build

./bin/ews-reminders -version
./bin/ews-reminders -list                      # встречи на ~24 часа
./bin/ews-reminders -once                      # один цикл напоминаний
./bin/ews-reminders                            # демон (опрос каждые poll_seconds)
./bin/ews-reminders -config /path/to/config.yaml
./bin/ews-reminders -state /path/to/shown.json
```

Версия — из файла `VERSION` (через `-ldflags` в Makefile / `install.sh` / `docker-build.sh`). Без `-ldflags` бинарник покажет `dev`. Релизный тег (`vX.Y.Z` или `X.Y.Z`) должен совпадать с `VERSION`.

## Конфигурация

Файл: `~/.config/ews-meeting-reminders/config.yaml` (образец — `config.example.yaml`).

| Секция / ключ                      | Назначение                                                                                 |
|------------------------------------|--------------------------------------------------------------------------------------------|
| `ews`                              | URL EWS, учётка, `auth: ntlm` / `basic`, `verify_ssl`                                      |
| `reminders.offsets_minutes`        | Окна напоминаний, минуты до старта (по умолчанию `[5, 0]`)                                 |
| `reminders.poll_seconds`           | Интервал опроса (по умолчанию `30`)                                                        |
| `reminders.lookahead_hours`        | Горизонт CalendarView вперёд (по умолчанию `12`)                                           |
| `reminders.grace_after_seconds`    | Допуск после порога offset, чтобы не пропустить из‑за интервала опроса (по умолчанию `90`) |
| `reminders.include_response_types` | Фильтр по `MyResponseType` (по умолчанию `[Accept, Organizer]`)                            |
| `reminders.state_keep_hours`       | Сколько хранить записи в `shown.json` (по умолчанию `24`)                                  |
| `notify`                           | Текст/кнопки уведомлений; `open_url_cmd`; `join_hosts`                                     |

### Пароль: `EWS_PASSWORD`

| Переменная     | Обязательная | Описание                                       |
|----------------|--------------|------------------------------------------------|
| `EWS_PASSWORD` | да*          | Пароль EWS; приоритетнее `ews.password` в YAML |

\* Если пароль не задан в `ews.password`. Лучше не писать его в YAML: файл `~/.config/ews-meeting-reminders/env` (`chmod 600`) — systemd подхватывает через `EnvironmentFile`.

### `reminders.include_response_types`

В список попадают только встречи с указанным `MyResponseType`.

| Значение             | Смысл               |
|----------------------|---------------------|
| `Accept`             | приглашение принято |
| `Organizer`          | вы организатор      |
| `Tentative`          | «под вопросом»      |
| `Decline`            | отклонено           |
| `NoResponseReceived` | без ответа          |
| `Unknown`            | статус неизвестен   |

### Ссылка на подключение

1. **Location** — любой `http://` / `https://` URL → ссылка на подключение (домен не фильтруется).
2. Иначе **Body** — первый URL, чей hostname совпадает с `notify.join_hosts`.
3. Если ссылки нет — кнопки «Открыть ссылку» нет.

### `notify.join_hosts`

Маски **hostname** (не regex). Используются **только при поиске в теле**. Без учёта регистра; порт и `user@` отбрасываются.

Если ключ пустой или отсутствует — дефолты: `*.ktalk.ru`, `zoom.us`, `*.zoom.us`.  
В `config.example.yaml` дополнительно указан `trueconf.x.com` — добавьте свои хосты туда же.

| Маска           | Что совпадает               | Пример                                   |
|-----------------|-----------------------------|------------------------------------------|
| `host.example`  | сам хост или любой поддомен | `zoom.us` → `zoom.us`, `us05web.zoom.us` |
| `*.example.com` | `example.com` или поддомены | `*.ktalk.ru` → `ktalk.ru`, `x.ktalk.ru`  |
| `.example.com`  | то же по суффиксу           | `.zoom.us` → `zoom.us`, `a.zoom.us`      |

`*` допустим только как префикс `*.` у маски хоста. Путь, схема и query на матч не влияют.

### `notify.open_url_cmd`

Команда открытия ссылки; URL — последний аргумент. По умолчанию `xdg-open`. Пример: `xterm -e xdg-open`.

## Как это работает

1. Процесс раз в `poll_seconds` читает календарь через EWS на горизонт `lookahead_hours`.
2. Для встреч с подходящим `include_response_types` проверяет окна `offsets_minutes` (с допуском `grace_after_seconds`).
3. Если окно наступило и напоминание ещё не показывали — D-Bus notification (очередь, если баннер уже открыт).
4. Показанные ключи: `~/.local/state/ews-meeting-reminders/shown.json`; старые записи чистятся по `state_keep_hours`.

## Отладка уведомлений

Отдельное приложение без EWS (сборка: `make build`):

```bash
./bin/ews-test-notify                          # одно тестовое уведомление
./bin/ews-test-notify -count 3                 # очередь
./bin/ews-test-notify -url 'https://example.com/c/1'
./bin/ews-test-notify -wait 10m
```

## Ограничения

- Только on-prem EWS (`ntlm` / `basic`); OAuth / Microsoft 365 не реализован.
- Уведомления требуют активной графической сессии пользователя.
- Сервис должен работать в хост-сессии (D-Bus пользователя).

## Структура репозитория

```
VERSION                  # semver релиза
cmd/ews-reminders/       # демон / -list / -once
cmd/ews-test-notify/     # smoke-test уведомлений
internal/                # app, config, ews, joinurl, notify, state, version
systemd/                 # user unit
Dockerfile               # сборка статического бинарника
docker-build.sh
install.sh               # сборка + установка user service
config.example.yaml
LICENSE
```

## License

[MIT](LICENSE) © 2026 Aleksey Dobrunov
