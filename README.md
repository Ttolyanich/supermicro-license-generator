# Supermicro License Generator & SUM Activator

Красивое веб-приложение в Docker для генерации, авто-активации по IP и управления лицензионными ключами Supermicro BMC / IPMI на базе [zsrv/supermicro-product-key](https://github.com/zsrv/supermicro-product-key) и встроенной утилиты **SUM 2.15**.

![Docker Ready](https://img.shields.io/badge/Docker-Ready-blue)
![Go](https://img.shields.io/badge/Go-1.21-00ADD8)
![SUM 2.15](https://img.shields.io/badge/SUM-2.15.0-green)

---

## ⚡ Два режима работы

1. **🚀 Вариант 1: Полная Авто-Активация по IP (С авто-чтением MAC через SUM & Redfish)**
   - Подключается по IP-адресу BMC (`93.185.65.54`), логину и паролю.
   - Сам считывает **BMC MAC address** с главной страницы/Redfish.
   - Сгенерирует лицензию (например `SFT-DCMS-SINGLE`).
   - Запускает утилиту SUM внутри контейнера и автоматически активирует ключ на сервере.

2. **⚡ Вариант 2: Ручной Генератор по MAC (Без авторизации)**
   - Принимает MAC в любом формате (с двоеточиями `ac:1f:6b:e4:b1:fa`, дефисами, пробелами или слитной строкой `ac1f6be4b1fa`).
   - Автоматически очищает MAC-адрес и проверяет валидность.
   - Выдает все ключи (`SFT-OOB-LIC`, `SFT-DCMS-SINGLE`, `SFT-SUM-LIC`, `SFT-SPM-LIC`, `SFT-SCM-LIC`, `SFT-SDDC-SINGLE`) с кнопками копирования в 1 клик.

---

## 🚀 Быстрый запуск в Docker

```bash
cd supermicro-license-generator
docker compose up -d --build
```

Веб-интерфейс доступен по адресу: **`http://localhost:8080`**

---

## 📤 Публикация в Git

```bash
cd supermicro-license-generator
git remote add origin https://github.com/ВАШ_ЛОГИН/supermicro-license-generator.git
git branch -M main
git push -u origin main
```
