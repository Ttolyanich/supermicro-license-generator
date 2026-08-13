# Supermicro License Generator & SUM Activator ⚡

Универсальный веб-генератор и автоматический активатор лицензионных ключей Supermicro BMC / IPMI в Docker и в виде **автономного `.exe` приложения для Windows** на базе [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key) и утилиты **Supermicro Update Manager (SUM 2.15.0)**.

![Docker Ready](https://img.shields.io/badge/Docker-Ready-blue)
![Go](https://img.shields.io/badge/Go-1.21-00ADD8)
![SUM 2.15](https://img.shields.io/badge/SUM-2.15.0-green)
![Windows Standalone](https://img.shields.io/badge/Windows-Standalone--EXE-0078D6)

---

## 📌 О проекте

Проект разработан на базе утилиты [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key) и полностью расширен графическим веб-интерфейсом, автоматическим считыванием MAC-адресов с BMC, очистителем MAC-форматов и встроенным исполнителем утилиты Supermicro SUM (Linux & Windows).

- **Официальный репозиторий:** [https://github.com/Ttolyanich/supermicro-license-generator](https://github.com/Ttolyanich/supermicro-license-generator)
- **Зеркало архива SUM 2.15 (Google Drive):** [https://drive.google.com/file/d/1Vx3SUKApd5q-G7-RvHuioPPddTTBwpli/view?usp=sharing](https://drive.google.com/file/d/1Vx3SUKApd5q-G7-RvHuioPPddTTBwpli/view?usp=sharing)

---

## ⚡ 2 Режима работы

### 🚀 Вариант 1: Полная Авто-Активация по IP (Авто-чтение MAC через SUM & Redfish)
1. Введите IP-адрес BMC сервера (`93.185.65.54`), логин и пароль IPMI (default: `ADMIN`/`ADMIN`).
2. Приложение само подключается к контроллеру, считывает **BMC MAC address** с главной страницы BMC / через Redfish API.
3. Генерирует выбранную лицензию (например `SFT-DCMS-SINGLE`).
4. Автоматически запускает утилиту `SUM` и выполняет команду `ActivateProductKey`.
5. Выводит статус «УСПЕХ» и полный лог ответа SUM в реальном времени.

### ⚡ Вариант 2: Ручной Генератор по MAC (Без авторизации)
1. Введите MAC-адрес в **абсолютно любом формате** (`ac:1f:6b:e4:b1:fa`, `ac-1f-6b-e4-b1-fa`, `ac1f6be4b1fa`).
2. Бэкенд автоматически удалит двоеточия, дефисы и пробелы, проверит валидность 12 hex-символов.
3. Моментально сгенерирует все ключи (`SFT-OOB-LIC`, `SFT-DCMS-SINGLE`, `SFT-SUM-LIC`, `SFT-SPM-LIC`, `SFT-SCM-LIC`, `SFT-SDDC-SINGLE`) с кнопками копирования в один клик.

---

## 🖥️ Автономное `.exe` Приложение для Windows (Без Docker)

Весь веб-интерфейс (HTML/CSS/JS) зашит **прямо внутрь одного `.exe` файла**. Ему не требуются внешние папки или установка стороннего ПО.

### Как запустить на Windows:

1. Зайдите в раздел **[GitHub Actions Artifacts](https://github.com/Ttolyanich/supermicro-license-generator/actions)** и скачайте готовую сборку `supermicro-license-generator-windows` (`supermicro-license-generator.exe`).
2. Убедитесь, что папка `sum_2.15.0_Win_x86_64` (содержит `sum.exe`) находится в одной директории с `.exe` файлом.
3. Запустите **`supermicro-license-generator.exe`** двойным кликом.
4. Приложение автоматически поднимет веб-сервер и **само откроет ваш браузер** по адресу `http://localhost:8080`!

---

## 🐳 Запуск в Docker (Linux / Сервер)

```bash
git clone https://github.com/Ttolyanich/supermicro-license-generator.git
cd supermicro-license-generator
docker compose up -d --build
```
Веб-интерфейс откроется по адресу: **`http://localhost:8080`**

---

## 📁 Структура проекта

```text
supermicro-license-generator/
├── .github/workflows/docker-build.yml # Автоматическая сборка Docker образа & Windows .exe
├── Dockerfile                          # Многоэтапная сборка контейнера (Debian + SUM Linux)
├── docker-compose.yml                  # Конфигурация запуска контейнера
├── main.go                             # REST API на Go с вшитым фронтендом (//go:embed)
├── run_windows.bat                     # Лаунчер для Windows
├── static/                             # Веб-интерфейс (HTML5 / Vanilla CSS / JS)
│   ├── index.html
│   ├── style.css
│   └── app.js
├── sum_2.15.0_Win_x86_64/              # Компоненты SUM 2.15 для Windows (sum.exe)
└── upstream/                           # Криптографический модуль zsrv/supermicro-product-key
```

---

## 📜 Лицензия и Отказ от ответственности

Проект распространяется под лицензией **MIT License**. См. файл `LICENSE`.

*Дисклеймер: Этот инструмент предназначен для системного администрирования собственным оборудованием Supermicro. Автор не несет ответственности за любое использование данного программного обеспечения.*
