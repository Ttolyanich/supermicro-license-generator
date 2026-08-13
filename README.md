# Supermicro License Generator & SUM Activator ⚡

Универсальный веб-генератор и автоматический активатор лицензионных ключей Supermicro BMC / IPMI в Docker и для Windows на базе [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key) и утилиты **Supermicro Update Manager (SUM 2.15.0)**.

![Docker Ready](https://img.shields.io/badge/Docker-Ready-blue)
![Go](https://img.shields.io/badge/Go-1.21-00ADD8)
![SUM 2.15](https://img.shields.io/badge/SUM-2.15.0-green)
![Windows Standalone](https://img.shields.io/badge/Windows-Standalone-0078D6)

---

## 📌 О проекте

Проект разработан на базе оригинальной утилиты [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key) и расширен полноценным графическим веб-интерфейсом, автоматическим считыванием MAC-адресов с BMC, встроенным очистителем MAC-форматов и прямым исполнителем утилиты Supermicro SUM (Linux & Windows).

- **Официальный репозиторий:** [https://github.com/Ttolyanich/supermicro-license-generator](https://github.com/Ttolyanich/supermicro-license-generator)
- **Зеркало архива SUM 2.15 (Google Drive):** [https://drive.google.com/file/d/1Vx3SUKApd5q-G7-RvHuioPPddTTBwpli/view?usp=sharing](https://drive.google.com/file/d/1Vx3SUKApd5q-G7-RvHuioPPddTTBwpli/view?usp=sharing)

---

## ⚡ 2 Режима работы

### 🚀 Вариант 1: Полная Авто-Активация по IP (Авто-чтение MAC через SUM & Redfish)
1. Введите IP-адрес BMC сервера (`93.185.65.54`), логин и пароль IPMI (default: `ADMIN`/`ADMIN`).
2. Приложение само подключается к контроллеру, считывает **BMC MAC address** с главной страницы BMC / через Redfish API.
3. Генерирует выбранную лицензию (например `SFT-DCMS-SINGLE`).
4. Автоматически запускает утилиту `SUM` в контейнере или локально и выполняет команду `ActivateProductKey`.
5. Выводит статус «УСПЕХ» и полный лог ответа SUM.

### ⚡ Вариант 2: Ручной Генератор по MAC (Без авторизации)
1. Введите MAC-адрес в **абсолютно любом формате**:
   - `ac:1f:6b:e4:b1:fa`
   - `ac-1f-6b-e4-b1-fa`
   - `ac1f6be4b1fa`
2. Бэкенд автоматически удалит двоеточия, дефисы и пробелы, проверит валидность 12 hex-символов.
3. Моментально сгенерирует все ключи (`SFT-OOB-LIC`, `SFT-DCMS-SINGLE`, `SFT-SUM-LIC`, `SFT-SPM-LIC`, `SFT-SCM-LIC`, `SFT-SDDC-SINGLE`) с кнопками копирования в один клик.

---

## 🚀 Варианты запуска

### Вариант А: Запуск в Docker (Рекомендуется для Linux/Серверов)

```bash
git clone https://github.com/Ttolyanich/supermicro-license-generator.git
cd supermicro-license-generator
docker compose up -d --build
```
Веб-интерфейс откроется по адресу: **`http://localhost:8080`**

### Вариант Б: Запуск на Windows (без Docker)

1. Убедитесь, что папка `sum_2.15.0_Win_x86_64` (содержащая `sum.exe`) находится в директории проекта.
2. Запустите скрипт `run_windows.bat` или выполните:
   ```cmd
   go run main.go
   ```
3. Откройте **`http://localhost:8080`** в вашем браузере.

---

## 📁 Структура проекта

```text
supermicro-license-generator/
├── Dockerfile                      # Многоэтапная сборка контейнера (Debian + SUM Linux)
├── docker-compose.yml              # Конфигурация для запуска одной командой
├── main.go                         # REST API сервер на Go (IP auto-MAC, Redfish, SUM launcher)
├── go.mod                          # Модуль Go
├── run_windows.bat                 # Лаунчер для Windows
├── static/                         # Веб-интерфейс (HTML5 / Vanilla CSS / JS)
│   ├── index.html
│   ├── style.css
│   └── app.js
├── sum_2.15.0_Win_x86_64/          # Компоненты SUM 2.15 для Windows (sum.exe)
└── upstream/                       # Криптографический модуль zsrv/supermicro-product-key
```

---

## 📜 Лицензия и Отказ от ответственности

Проект распространяется под лицензией **MIT License**. См. файл `LICENSE`.

*Дисклеймер: Этот инструмент предназначен для системного администрирования собственным оборудованием Supermicro. Автор не несет ответственности за любое использование данного программного обеспечения.*
