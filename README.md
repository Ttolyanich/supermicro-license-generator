# Supermicro License Generator & SUM Activator ⚡

Веб-генератор и автоматический активатор лицензионных ключей Supermicro BMC / IPMI. Работает в Docker и как **автономное `.exe` приложение для Windows**. Криптографическое ядро — [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key); активация выполняется утилитой **Supermicro Update Manager (SUM 2.15.0)**.

![Docker Ready](https://img.shields.io/badge/Docker-Ready-blue)
![Go](https://img.shields.io/badge/Go-1.21-00ADD8)
![SUM 2.15](https://img.shields.io/badge/SUM-2.15.0-green)
[![Download EXE](https://img.shields.io/badge/Download-Windows--EXE-0078D6?logo=windows&logoColor=white)](https://github.com/Ttolyanich/supermicro-license-generator/releases/latest)

---

## 💾 Скачивание

- 📥 **[supermicro-license-generator.exe (последний релиз)](https://github.com/Ttolyanich/supermicro-license-generator/releases/latest)** — автономное Windows-приложение.
- 📦 **[Страница релизов (GitHub Releases)](https://github.com/Ttolyanich/supermicro-license-generator/releases)**
- 🐳 **Docker-образ:** `ghcr.io/ttolyanich/supermicro-license-generator:latest`

> **Утилита SUM в проект НЕ входит** — это проприетарное ПО Supermicro, не подлежащее распространению. Скачивайте её **только с официального Download Center Supermicro**:
> - 📥 **[supermicro.com/…/downloadcenter/smsdownload](https://www.supermicro.com/en/support/resources/downloadcenter/smsdownload)** → выберите продукт **SUM** и версию под свою платформу:
>   - **Docker (Linux-сервер)** → **Linux** версия SUM (`sum_..._Linux_x86_64`);
>   - **Автономный `.exe` (Windows)** → **Windows** версия SUM (`sum_..._Win_x86_64`).
> - Инструкция по активации: [store.supermicro.com — Software License Key Activation](https://store.supermicro.com/software/software-license-key-activation-usage)
>
> Скачанный архив SUM можно загрузить прямо в веб-интерфейсе (раздел «Справка SKU & SUM» → «Управление утилитой SUM») — он распакуется, проверится и сохранится вне контейнера.

---

## 📌 О проекте

Обёртка над [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key) с графическим веб-интерфейсом: автоматическое считывание MAC-адреса с BMC (через SUM, IPMI Web CGI и Redfish), нормализация формата MAC, генерация ключей и запуск активации через SUM.

- **Репозиторий:** [https://github.com/Ttolyanich/supermicro-license-generator](https://github.com/Ttolyanich/supermicro-license-generator)

---

## ⚡ Режимы работы

### 🚀 Вариант 1: Авто-активация по IP
1. Введите IP-адрес BMC (например `192.0.2.10`), логин и пароль IPMI (по умолчанию `ADMIN`/`ADMIN`). MAC можно указать вручную — тогда обращение к BMC за MAC пропускается.
2. Приложение считывает **BMC MAC** через SUM `GetBmcInfo`, страницу IPMI Web CGI или Redfish API.
3. Генерирует выбранную лицензию (например `SFT-DCMS-SINGLE`).
4. Запускает `SUM` с командой `ActivateProductKey` (нужен установленный SUM — см. ниже).
5. Показывает статус и полный лог ответа SUM.

### ⚡ Вариант 2: Ручной генератор по MAC (без авторизации)
1. Введите MAC-адрес в **любом формате** (`ac:1f:6b:e4:b1:fa`, `ac-1f-6b-e4-b1-fa`, `ac1f6be4b1fa`).
2. Бэкенд удалит разделители и проверит валидность (12 hex-символов).
3. Сгенерирует все ключи (`SFT-OOB-LIC`, `SFT-DCMS-SINGLE`, `SFT-SUM-LIC`, `SFT-SPM-LIC`, `SFT-SCM-LIC`, `SFT-SDDC-SINGLE` и др.) с копированием в один клик.

### 🔍 Вариант 3: Декодер и Bruteforce
Декодирование атрибутов зашифрованного ключа и подбор MAC по имеющемуся ключу в пределах OUI-блоков Supermicro.

---

## 🖥️ Автономное `.exe` для Windows (без Docker)

Весь веб-интерфейс (HTML/CSS/JS) вшит **внутрь `.exe`** через `//go:embed` — внешние папки не нужны.

1. Скачайте **[supermicro-license-generator.exe](https://github.com/Ttolyanich/supermicro-license-generator/releases/latest)**.
2. Для генерации/декодирования ключей SUM не требуется. Для **активации** скачайте **Windows-версию SUM** (`sum_..._Win_x86_64` с `sum.exe`) с [официального Download Center Supermicro](https://www.supermicro.com/en/support/resources/downloadcenter/smsdownload) и либо положите эту папку рядом с `.exe`, либо загрузите архив SUM прямо в веб-интерфейсе, либо укажите путь через переменную `SUM_PATH`.
3. Запустите `.exe` двойным кликом.
4. Приложение поднимет локальный веб-сервер на `http://localhost:8080` и откроет браузер.

По умолчанию сервер слушает только `127.0.0.1` (localhost) — из сети он недоступен. В консоли при старте выводится версия (`...Web App v1.1.0`).

### ⚙️ Переменные окружения для `.exe` (Windows)

`.exe` учитывает те же переменные, что и Docker (`HOST`, `PORT`, `SUM_PATH`, `SUM_DATA_DIR`, `BASIC_AUTH_USER`/`BASIC_AUTH_PASS`, `NO_BROWSER`) — см. таблицу в разделе [Безопасность](#-безопасность).

⚠️ **Двойной клик не задаёт переменные окружения.** Их нужно выставить в той же сессии консоли перед запуском либо через `.bat`-файл.

**PowerShell** (сменить порт и открыть доступ из сети):
```powershell
$env:PORT="9000"; $env:HOST="0.0.0.0"; .\supermicro-license-generator.exe
```

**cmd.exe:**
```cmd
set PORT=9000 && set HOST=0.0.0.0 && supermicro-license-generator.exe
```

**`.bat`-файл рядом с `.exe`** (тогда можно запускать двойным кликом):
```bat
@echo off
rem Доступ из локальной сети на порту 9000 с паролем
set HOST=0.0.0.0
set PORT=9000
set BASIC_AUTH_USER=admin
set BASIC_AUTH_PASS=change-me
supermicro-license-generator.exe
```

> 🔒 `HOST=0.0.0.0` делает `.exe` доступным из локальной сети. Так как endpoint активации ходит по любому IP с любыми учётными данными (SSRF), при доступе из сети **обязательно** задайте `BASIC_AUTH_USER`/`BASIC_AUTH_PASS` и/или ограничьте фаерволом. Для обычного локального использования оставляйте дефолтный `127.0.0.1`.

---

## 🐳 Запуск в Docker (Linux / сервер)

`docker-compose.yml` по умолчанию тянет **готовый образ из GHCR** (собирается в CI), а не собирает локально:

```bash
git clone https://github.com/Ttolyanich/supermicro-license-generator.git
cd supermicro-license-generator
docker compose up -d
```

Обновление до свежего образа:
```bash
docker compose pull && docker compose up -d
```

Интерфейс: **`http://localhost:8080`** (в `docker-compose.yml` порт по умолчанию привязан к `127.0.0.1`).

Без клонирования репозитория, одной командой:
```bash
docker run -d -p 127.0.0.1:8080:8080 --name supermicro-license-generator \
  -v sum_data:/app/data \
  ghcr.io/ttolyanich/supermicro-license-generator:latest
```

> Хотите собирать из исходников — в `docker-compose.yml` закомментируйте `image:` и раскомментируйте блок `build:`, затем `docker compose up -d --build`.

**Активация через SUM в Docker.** Бинарник SUM в образ не встроен. Скачайте **Linux-версию SUM** (`sum_..._Linux_x86_64`) с [официального Download Center Supermicro](https://www.supermicro.com/en/support/resources/downloadcenter/smsdownload). Проще всего — загрузить этот архив прямо в веб-интерфейсе (сохранится в томе `sum_data`). Либо смонтировать распакованный SUM и указать `SUM_PATH`:
```bash
docker run -d -p 127.0.0.1:8080:8080 \
  -e SUM_PATH=/app/sum_tool/sum \
  -v /opt/sum_2.15.0_Linux_x86_64:/app/sum_tool:ro \
  ghcr.io/ttolyanich/supermicro-license-generator:latest
```
Без SUM работают генерация ключей, декодирование и чтение MAC через Redfish/IPMI Web.

---

## 🔒 Безопасность

Эндпоинты активации подключаются к любому указанному IP с переданными учётными данными и возвращают фрагменты ответа — это по сути инструмент обращения к BMC, поэтому:

- Сервер по умолчанию слушает **только localhost** (кроме Docker, где `HOST=0.0.0.0`, а наружу порт привязан к `127.0.0.1`).
- Для доступа из сети включите HTTP Basic Auth: задайте `BASIC_AUTH_USER` и `BASIC_AUTH_PASS`.
- Запросы к API с других сайтов блокируются (проверка `Sec-Fetch-Site`/`Origin` и `Content-Type: application/json`).

| Переменная | По умолчанию | Назначение |
|------------|--------------|------------|
| `HOST` | `127.0.0.1` | Интерфейс прослушивания (`0.0.0.0` — все) |
| `PORT` | `8080` | Порт |
| `SUM_PATH` | — | Явный путь к бинарнику SUM (высший приоритет) |
| `SUM_DATA_DIR` | `sum_data` | Каталог для SUM, загруженного через веб (переживает обновления) |
| `BASIC_AUTH_USER` / `BASIC_AUTH_PASS` | — | Включают Basic Auth, если заданы |
| `NO_BROWSER` | — | Любое значение отключает авто-открытие браузера |

---

## 📁 Структура проекта

```text
supermicro-license-generator/
├── .github/workflows/docker-build.yml  # Сборка Docker-образа и Windows .exe
├── Dockerfile                          # Многоэтапная сборка (Debian runtime)
├── docker-compose.yml                  # Конфигурация запуска контейнера
├── main.go                             # REST API на Go с вшитым фронтендом (//go:embed)
├── run_windows.bat                     # Лаунчер для Windows
├── static/                             # Веб-интерфейс (HTML / CSS / JS)
│   ├── index.html
│   ├── style.css
│   └── app.js
└── upstream/                           # Криптоядро zsrv/supermicro-product-key (vendored)
```

> Папки `sum_2.15.0_*` намеренно исключены из репозитория (`.gitignore`) — это проприетарные бинарники SUM, которые нужно скачать отдельно.

---

## 📜 Лицензия, атрибуция и отказ от ответственности

Собственный код этого проекта (`main.go`, `static/`, Docker/CI-конфигурация) распространяется под **MIT License** (см. `LICENSE`).

Каталог `upstream/` — копия проекта [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key). На момент включения оригинальный репозиторий не содержал файла лицензии; авторские права принадлежат его автору, и MIT данного проекта на код в `upstream/` не распространяется. **Supermicro Update Manager (SUM)** — проприетарное ПО Supermicro, в репозиторий не входит и распространяется по собственной лицензии Supermicro.

*Дисклеймер: инструмент предназначен для системного администрирования собственного оборудования Supermicro. Автор не несёт ответственности за использование данного ПО.*
