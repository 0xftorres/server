# Changelog — Tenis Ahora API de prueba

Formato: [Keep a Changelog](https://keepachangelog.com/es/1.1.0/). Versionado semántico.

## [1.0.0] — 2026-10-23

### Agregado
- Scripts `D:\fede\start-servidor.ps1` / `stop-servidor.ps1`: compilan API y front, levantan la
  API en segundo plano sirviendo el front y muestran las URLs locales y por `nip.io`.
- `tests/smoke.py`: prueba de humo de punta a punta (45 verificaciones).
- Informe final `INFORME-API.pdf` con versiones, endpoints, modelo de datos y recomendaciones.

### Cambiado
- La API responde `index.html` con 200 para las rutas de la SPA (antes 404 con el contenido).
- Build: se usa el toolchain MinGW-w64 completo de `D:\fede\tools\mingw64` (el `ld` de rustup
  no acepta los import libs del `dlltool` moderno que exigen las crates `windows-sys`).

### Verificado
- `tests/smoke.py`: 45/45 verificaciones OK, en local y a través de `http://<ip>.nip.io:5090`.
- Front servido por la API y operando contra datos persistidos desde el navegador (login real,
  altas que sobreviven a la recarga, cancelación con cargo y reembolso, Modo Demo intacto).
- Build en Windows sin compilador C para SQLite (DLL oficial 3.50.4).

## [0.4.0] — 2026-10-23

### Agregado
- La API sirve el build del front (`FRONTEND_DIST`) con fallback a `index.html` para las rutas
  de react-router.
- Front: `src/services/httpApi.js` (servicios HTTP) y `withBackend()` en `src/services/api.js`
  (HTTP si `/api/health` responde y la sesión no es Modo Demo; mocks en caso contrario).
- `AuthContext` acepta `dni` y `memberNumber` del `AuthResponseDto` y el rol `Profesor`.

## [0.3.0] — 2026-10-23

### Agregado
- Endpoints de negocio: estado y disponibilidad de canchas, ajuste de stock, alta de reserva
  con recibo de seña, pago de saldo, cancelación con reembolso o cargo (RD02–RD04, RD06),
  reglamentos, inscripciones y enfrentamientos de torneos y ligas, resultado de liga con tabla,
  disponibilidad y certificación de docentes, alumnos y asistencia de clases (RD07, RD10),
  cancelación de turnos particulares, anulación de facturas, estado de reembolsos, descuentos
  por usuario y `POST /admin/reset`.

## [0.2.0] — 2026-10-23

### Agregado
- CRUD genérico `GET/POST /api/{col}` y `GET/PUT/DELETE /api/{col}/{id}` para las 15
  colecciones, con generación de ids con los mismos prefijos que el front y las reglas de
  alta/modificación/baja del mock (unicidad, cupos, recálculos, cascadas).
- Ocultamiento de `passwordHash` en todas las respuestas de `usuarios`.

## [0.1.0] — 2026-10-23

### Agregado
- Proyecto axum + tokio; `Store` sobre SQLite (tabla `documents`, WAL) enlazando la DLL
  oficial de SQLite 3.50.4 (sin `bundled`, sin compilador C).
- Seed automático desde `seed/seed.json` cuando la base está vacía; contraseña `123456`
  para todos los usuarios del seed.
- Autenticación: `POST /auth/login`, `POST /auth/registrar`, `GET /auth/yo`; JWT HS256 de 8 h;
  middleware `Bearer` en todas las rutas protegidas.
- `GET /health` con versión, versión de SQLite y conteo de documentos.
- Errores uniformes `{ "error": "..." }`.
