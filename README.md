# Supermicro Product Key Web App (Docker)

Красивый и удобный веб-интерфейс для генерации, декодирования и управления лицензионными ключами Supermicro BMC / IPMI на базе утилиты [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key).

![Docker Ready](https://img.shields.io/badge/Docker-Ready-blue)
![Go](https://img.shields.io/badge/Go-1.21-00ADD8)

---

## ⚡ Возможности

- **Автоматическая генерация всех типов ключей в 1 клик:**
  - `SFT-OOB-LIC` — ключ управления OOB (BIOS Update, RAID, HTML5 KVM для систем X9, X10, X11).
  - `SFT-DCMS-SINGLE` — полный пакет лицензий Data Center Management Suite (ALL).
  - `SFT-SUM-LIC` — утилита Supermicro Update Manager.
  - `SFT-SPM-LIC` — Supermicro Power Manager.
  - `SFT-SCM-LIC` — Supermicro Cloud Manager.
  - `SFT-DCMS-SITE`, `SFT-DCMS-CALL-HOME`, `SFT-DCMS-SVC-KEY`, `SFT-SDDC-SINGLE`.
- **Генератор CLI команд SUM:** автоматическое формирование команд типа `./sum -i <IP> -u ADMIN -p ADMIN -c ActivateProductKey --key <KEY>`.
- **Декодер Non-JSON ключей:** просмотр свойств зашифрованного ключа (SKU, дата создания, контрольная сумма).
- **Подбор MAC-адреса (Bruteforce):** поиск исходного MAC по OOB или Non-JSON ключу.
- **Современный интерфейс:** Dark Mode, стеклянный дизайн (Glassmorphism), быстрый поиск, копирование в один клик.

---

## 🚀 Быстрый запуск в Docker

### Вариант 1: Через Docker Compose (Рекомендуется)

```bash
cd supermicro-web
docker compose up -d --build
```

Веб-интерфейс будет доступен по адресу: **`http://localhost:8080`**

### Вариант 2: Через Docker CLI

```bash
docker build -t supermicro-web .
docker run -d -p 8080:8080 --name supermicro-key-web supermicro-web
```

---

## 🛠️ Запуск без Docker (Локальный Go)

Если на машине установлен Go 1.21+:

```bash
cd supermicro-web
go run main.go
```

Приложение откроется на порту `8080`.

---

## 📁 Структура проекта

```text
supermicro-web/
├── Dockerfile            # Многоэтапная сборка (Multi-stage build)
├── docker-compose.yml    # Конфигурация для запуска одной командой
├── main.go               # REST API & Web-сервер на Go
├── go.mod                # Модуль Go с привязкой к upstream библиотеке
├── static/               # Фронтенд (HTML5 / Vanilla CSS / JS)
│   ├── index.html
│   ├── style.css
│   └── app.js
└── upstream/             # Исходный код zsrv/supermicro-product-key
```
