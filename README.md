# EWS Meeting Reminders

Демон для Linux: напоминания о встречах из **MS Exchange (EWS)** за фиксированное время до начала (по умолчанию **5** и **0** минут). Время задаёт сам сервис, а не значение reminder из события.

Удобно использовать вместе:
 - с Evolution - у клиента напоминания берутся из встречи и срабатывают один раз.
 - с web-версией почты - напоминания появляются не на одной из многочисленных вкладок, а на рабочем столе.

## Возможности

- Опрос календаря по протоколу/api EWS (аутентификация NTLM / Basic)
- Desktop-уведомления с кнопками «Открыть ссылку» / «Отложить» / «Прекратить» / «Отложить все»
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

1. Скачайте `*.tar.gz` из [Releases](https://github.com/adobrunov/ews-meeting-reminders/releases).
2. Распакуйте архив.
3. В распакованной папке выполните:

```bash
./install
```

Скрипт сам:
- установит бинарник и user unit в нужные директории в пределах домашней папки;
- при первом запуске создаст `~/.config/ews-meeting-reminders/config.yaml` если его еще нет;
- при обновлении остановит сервис, заменит бинарник и запустит сервис снова.

После первой установки заполните параметры `server / email / username` в `~/.config/ews-meeting-reminders/config.yaml`, затем:

```bash
echo 'EWS_PASSWORD=секрет' > ~/.config/ews-meeting-reminders/env
chmod 600 ~/.config/ews-meeting-reminders/env
systemctl --user restart ews-meeting-reminders.service
```

Проверка:

```bash
~/.local/share/ews-meeting-reminders/ews-reminders -list
journalctl --user -u ews-meeting-reminders.service -f
```

Сервис рассчитан на сессию после входа в графику.

Удаление (конфиг и `env` не трогает):

```bash
./uninstall
```

## Установка из исходников

Требуется Go 1.26+. Все операции установки из исходников выполняются через `Makefile`.

```bash
cd ews-meeting-reminders

# опционально: создайте config.yaml заранее; иначе make install скопирует пример
mkdir -p ~/.config/ews-meeting-reminders
cp config.example.yaml ~/.config/ews-meeting-reminders/config.yaml
# заполните server / email / username

echo 'EWS_PASSWORD=секрет' > ~/.config/ews-meeting-reminders/env
chmod 600 ~/.config/ews-meeting-reminders/env

make install
```

### Сборка без локального Go (Docker)

Только сборка бинарника; запуск всё равно на хосте:

```bash
make docker-build          # → ./bin/ews-reminders
./bin/ews-reminders -version
```

Дальше: либо установка через релизный install-скрипт, либо ручной запуск бинарника:

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

Версия — из файла `VERSION` (через `-ldflags` в Makefile, включая `make docker-build`). Без `-ldflags` бинарник покажет `dev`. Релизный тег (`vX.Y.Z` или `X.Y.Z`) должен совпадать с `VERSION`.

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
| `reminders.snooze_minutes`         | Пауза после «Отложить» до повторного напоминания (по умолчанию `5`)                        |
| `notify`                           | Текст/кнопки уведомлений; `open_url_cmd`; `join_hosts`                                     |
| `notify.skip_action_label`         | Подпись кнопки отложить (ключ старый; по умолчанию «Отложить»)                             |
| `notify.stop_action_label`         | Подпись кнопки прекратить напоминания по встрече (по умолчанию «Прекратить»)               |

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
2. Иначе **Body** (через отдельный EWS `GetItem`, т.к. `FindItem` тело не отдаёт) — первый URL, чей hostname совпадает с `notify.join_hosts`.
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
4. Кнопки на карточке:
   - **Открыть ссылку** — открывает join URL;
   - **Отложить** (`skip_action_label`) — закрывает карточку и планирует одно повторное напоминание через `snooze_minutes` (отсчёт от момента нажатия). Пока ждём, остальные окна `offsets_minutes` этой встречи не показываются; offset’ы, которые попали бы не позже момента повтора, считаются «покрытыми» и не дублируют snooze (например, отложить на T−10 при интервале 5 мин → одно напоминание на T−5, без отдельного offset 8 и 5). Карточки **той же встречи**, уже лежащие в очереди (например offset 0, успевший попасть туда, пока баннер висел), снимаются с очереди; напоминания по **другим** встречам в очереди не трогаются;
   - **Прекратить** (`stop_action_label`) — больше никаких напоминаний по этой встрече; карточки той же встречи убираются из очереди, остальные встречи в очереди показываются как обычно;
   - **Отложить все** — откладывание для текущей карточки и всех ещё ожидающих в очереди.
5. «Отложить» на **offset 0** (или позже) тоже работает: повтор придёт через `snooze_minutes` уже после начала встречи. Можно откладывать снова — каждый раз срок сдвигается ещё на `snooze_minutes`, пока встреча ещё возвращается из EWS CalendarView (обычно пока идёт) либо пока не нажать «Прекратить».
6. Состояние: `~/.local/state/ews-meeting-reminders/shown.json` (старые ключи `id:offset` совместимы); чистка по `state_keep_hours`.

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
scripts/                 # install / uninstall / user systemd unit
Dockerfile               # сборка статического бинарника
config.example.yaml
LICENSE
```

## License

[MIT](LICENSE) © 2026 Aleksey Dobrunov
