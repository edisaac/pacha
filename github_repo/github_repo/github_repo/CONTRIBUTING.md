# Guía de Contribución

¡Gracias por tu interés en contribuir a nuestro generador de horarios!

## Compilación Local

Para compilar y ejecutar el proyecto localmente, necesitas tener instalados los siguientes componentes:

1. **Go** (1.22 o superior)
2. **Java Development Kit (JDK)** (Temurin 21 o superior, necesario para compilar el módulo de Timefold Solver)
3. **Node.js** (para las dependencias del frontend si es necesario)
4. **Wails CLI** (v2)

### Pasos:

1. Clona el repositorio.
2. Asegúrate de compilar el solver primero (usando Gradle/Maven en el directorio del solver, si aplica).
3. Ejecuta Wails en modo desarrollo:
   ```bash
   wails dev
   ```

## Modelo Dual-Licensing y CLA

Este proyecto se distribuye bajo la licencia **AGPLv3** y también cuenta con un modelo de licenciamiento comercial. 

Por este motivo, **es obligatorio firmar nuestro Contributor License Agreement (CLA)** antes de que podamos aceptar tu Pull Request. Al abrir un PR, nuestro bot automatizado comentará con las instrucciones para firmarlo. Al firmar, retienes los derechos de autor de tus aportes, pero nos otorgas el permiso para sublicenciarlos bajo la AGPLv3 y nuestras licencias comerciales.

## Reporte de Bugs y Solicitud de Mejoras

Utilizamos GitHub Issues para rastrear errores y solicitudes de nuevas reglas horarias (constraints).

- **Bugs:** Utiliza la plantilla de *Reporte de Error* para fallos en la UI o en el motor de resolución.
- **Reglas Horarias:** Si necesitas una nueva restricción (hard/soft constraint), utiliza la plantilla de *Solicitud de Mejora*. Detalla claramente el caso de uso y cómo debería evaluarse en la puntuación del horario.
