# Wago Assistant - Plataforma SaaS de Bots de WhatsApp

Wago es una plataforma corporativa para gestionar asistentes automatizados en WhatsApp impulsados por inteligencia artificial. Soporta múltiples usuarios, subscripciones (Free, Pro, Enterprise), manejo de pagos por Stripe/PayPal y un panel de administración centralizado.

## Características Principales

*   **Arquitectura Limpia:** Construido en Go con Fiber v3, siguiendo principios Domain-Driven Design (DDD).
*   **Concurrencia Segura:** Uso de Redis para el control distribuido de sesiones y límites de cuota (Rate Limiting).
*   **Persistencia Robusta:** PostgreSQL para almacenar usuarios, historiales de chat y suscripciones de manera segura.
*   **Seguridad:** Encriptación de mensajes de chat (AES-GCM), middleware CSRF en todas las peticiones POST/PUT/DELETE, y protección contra inyección SQL.
*   **Modelo SaaS:** Sistema de suscripciones integrado que bloquea bots si se excede la cuota diaria (ej. plan Free = 10 mensajes/día).
*   **Frontend Premium:** Interfaz de usuario con estética "Glassmorphism", modo oscuro nativo e interacciones suaves en JS puro.

## Requisitos Previos

*   [Docker](https://docs.docker.com/get-docker/) y [Docker Compose](https://docs.docker.com/compose/install/) instalados en tu servidor/máquina local.

## Configuración y Despliegue Rápido (Producción)

El proyecto está dockerizado para un despliegue de un solo clic. El archivo `docker-compose.yml` levantará 3 servicios: la app Go, PostgreSQL y Redis.

1.  **Clonar el repositorio:**
    ```bash
    git clone https://github.com/tu-usuario/WagoAsistant.git
    cd WagoAsistant
    ```

2.  **Configurar Variables de Entorno (Opcional pero Recomendado):**
    Puedes copiar el archivo `.env.example` a `.env` (si lo creas) o simplemente usar las variables por defecto en el `docker-compose.yml`. Las más importantes son:
    *   `ENCRYPTION_KEY`: Clave de 32 bytes para encriptar los chats (ej. `0123456789abcdef0123456789abcdef`).
    *   `ADMIN_USERNAME` / `ADMIN_PASSWORD`: Credenciales para el panel maestro.
    *   `OPENROUTER_API_KEY`: Tu clave de OpenRouter/OpenAI para la IA.

3.  **Iniciar los contenedores:**
    ```bash
    docker-compose up -d --build
    ```
    
    Este comando hará lo siguiente:
    *   Compilará la aplicación Go (Multi-stage build).
    *   Levantará PostgreSQL y Redis con volúmenes persistentes (`postgres_data`, `redis_data`).
    *   Expondrá la aplicación web en el puerto `3000`.

4.  **Acceder a la Plataforma:**
    *   Login de Usuarios: `http://localhost:3000/sing`
    *   Panel de Administrador: Ingresa con las credenciales de Admin (por defecto `admin` / `admin123`).

## Operaciones de Mantenimiento

*   **Ver los logs del sistema en tiempo real:**
    ```bash
    docker-compose logs -f app
    ```
*   **Detener la plataforma:**
    ```bash
    docker-compose down
    ```
*   **Reiniciar la plataforma (después de cambios):**
    ```bash
    docker-compose up -d --build
    ```

## Estructura del Proyecto

*   `src/domain`: Entidades core del negocio (Users, Bots, Subscriptions).
*   `src/ports`: Interfaces (Contratos) para los repositorios y servicios externos.
*   `src/adapters`: Implementaciones de bases de datos (Postgres, Redis).
*   `src/app`: Casos de uso e inyección de dependencias (BotService, ChatService).
*   `src/handlers`: Controladores HTTP (Fiber v3).
*   `src/static`: Frontend HTML, CSS y JS puro.

## Consideraciones Adicionales
- Wago utiliza `whatsmeow` para mantener sesiones activas. Si reinicias los contenedores, las sesiones no se perderán gracias a que se guardan en la base de datos PostgreSQL.
- El Webhook de pagos se encuentra en `/api/payments/webhook`, listo para integrarse con Stripe/PayPal.
