//! Tenis Ahora — API de prueba (Rust + axum + SQLite).
//!
//! Servidor liviano para que el front (`Tenis-Ahora-Frontend`) trabaje contra datos
//! persistidos en lugar de los mocks en memoria. No está pensado para producción: ver
//! INFORME-API.pdf para el alcance y las recomendaciones para la versión definitiva.

mod auth;
mod business;
mod crud;
mod db;
mod error;
mod seed;
mod util;

use std::{net::SocketAddr, path::PathBuf, time::Instant};

use axum::{
    extract::State,
    middleware,
    response::IntoResponse,
    routing::{delete, get, patch, post, put},
    Json, Router,
};
use serde_json::{json, Value};
use tower_http::{cors::CorsLayer, services::ServeDir, trace::TraceLayer};

use crate::{db::Store, error::ApiResult};

pub const VERSION: &str = env!("CARGO_PKG_VERSION");

#[derive(Clone)]
pub struct AppState {
    pub store: Store,
    pub jwt_secret: String,
    pub seed_path: PathBuf,
    pub started_at: Instant,
}

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).ok().filter(|v| !v.trim().is_empty()).unwrap_or_else(|| default.to_string())
}

/// GET /api/health — público; el front lo usa para decidir si trabaja contra la API o con mocks.
async fn health(State(state): State<AppState>) -> ApiResult<Json<Value>> {
    let counts: serde_json::Map<String, Value> = state
        .store
        .counts()?
        .into_iter()
        .map(|(k, v)| (k, Value::from(v)))
        .collect();
    Ok(Json(json!({
        "status": "ok",
        "service": "tenis-ahora-api",
        "version": VERSION,
        "sqlite": state.store.sqlite_version(),
        "uptimeSeconds": state.started_at.elapsed().as_secs(),
        "collections": counts
    })))
}

/// Servicio que responde siempre index.html con 200 (fallback de la SPA).
fn spa_index(index: PathBuf) -> axum::routing::MethodRouter {
    get(move || {
        let index = index.clone();
        async move {
            match tokio::fs::read(&index).await {
                Ok(bytes) => ([(axum::http::header::CONTENT_TYPE, "text/html; charset=utf-8")], bytes).into_response(),
                Err(_) => (axum::http::StatusCode::NOT_FOUND, "index.html no encontrado").into_response(),
            }
        }
    })
}

fn build_router(state: AppState, frontend_dist: Option<PathBuf>) -> Router {
    let public = Router::new()
        .route("/health", get(health))
        .route("/auth/login", post(auth::login))
        .route("/auth/registrar", post(auth::register));

    let protected = Router::new()
        .route("/auth/yo", get(auth::me))
        .route("/admin/reset", post(business::reset))
        // Canchas y stock
        .route("/canchas/{id}/estado", patch(business::court_status))
        .route("/canchas/{id}/disponibilidad", put(business::court_availability))
        .route("/stock/{id}/ajuste", patch(business::stock_adjust))
        // Reservas (alta propia: genera recibo de seña)
        .route("/reservas", get(business::list_reservations).post(business::create_reservation))
        .route("/reservas/{id}/pagar-saldo", post(business::pay_remaining))
        .route("/reservas/{id}/cancelar", post(business::cancel_reservation))
        // Torneos
        .route("/torneos/{id}/reglamento", put(business::tournament_rules))
        .route("/torneos/{id}/inscripciones", post(business::tournament_enroll))
        .route("/torneos/{id}/inscripciones/{pid}", delete(business::tournament_unenroll))
        .route("/torneos/{id}/enfrentamientos", post(business::tournament_add_match))
        .route("/torneos/{id}/enfrentamientos/{mid}", put(business::tournament_update_match))
        // Ligas
        .route("/ligas/{id}/reglamento", put(business::league_rules))
        .route("/ligas/{id}/inscripciones", post(business::league_enroll))
        .route("/ligas/{id}/inscripciones/{pid}", delete(business::league_unenroll))
        .route("/ligas/{id}/enfrentamientos", post(business::league_add_match))
        .route("/ligas/{id}/enfrentamientos/{mid}", put(business::league_update_match))
        .route("/ligas/{id}/enfrentamientos/{mid}/resultado", post(business::league_match_score))
        // Docentes, clases y entrenamientos
        .route("/profesores/{id}/disponibilidad", put(business::coach_availability))
        .route("/profesores/{id}/certificacion", put(business::coach_license))
        .route("/clases/{id}/alumnos", post(business::class_enroll))
        .route("/clases/{id}/alumnos/{sid}", delete(business::class_unenroll))
        .route("/clases/{id}/asistencia/alternar", post(business::class_toggle_attendance))
        .route("/clases/{id}/asistencia/sesion", post(business::class_bulk_attendance))
        .route("/clases/{id}/asistencia/registrar", post(business::class_record_attendance))
        .route("/clases-particulares/{id}/cancelar", post(business::private_cancel))
        // Caja
        .route("/facturas/{id}/anular", post(business::invoice_annul))
        .route("/reembolsos/{id}/estado", patch(business::refund_status))
        .route("/usuarios/{id}/descuentos", put(business::user_discounts))
        // CRUD genérico
        .route("/{collection}", get(crud::list).post(crud::create))
        .route("/{collection}/{id}", get(crud::get_one).put(crud::update).delete(crud::remove))
        .layer(middleware::from_fn_with_state(state.clone(), auth::require_auth));

    let mut app = Router::new().nest("/api", public.merge(protected));

    // Opcional: servir el build del front (SPA) desde el mismo puerto. Los archivos de dist/ se
    // sirven tal cual; cualquier otra ruta que no sea /api devuelve index.html con 200, así
    // funcionan las rutas de react-router al recargar o al entrar por un link directo.
    if let Some(dist) = frontend_dist {
        let index = dist.join("index.html");
        app = app
            .nest_service("/assets", ServeDir::new(dist.join("assets")))
            // `.fallback` (no `.not_found_service`, que fuerza 404) para responder index.html con 200.
            .fallback_service(ServeDir::new(&dist).fallback(spa_index(index)));
    }

    app.layer(CorsLayer::permissive())
        .layer(TraceLayer::new_for_http())
        .with_state(state)
}

#[tokio::main]
async fn main() {
    let _ = dotenvy::dotenv();
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "info,tower_http=info".into()))
        .init();

    let port: u16 = env_or("PORT", "5090").parse().unwrap_or(5090);
    let db_path = PathBuf::from(env_or("DATABASE_PATH", "data/tenis_ahora.sqlite"));
    let seed_path = PathBuf::from(env_or("SEED_PATH", "seed/seed.json"));
    let jwt_secret = env_or("JWT_SECRET", "tenis-ahora-dev-secret-cambiar-en-produccion");

    let store = match Store::open(&db_path) {
        Ok(s) => s,
        Err(e) => {
            eprintln!("No se pudo abrir la base de datos {}: {}", db_path.display(), e.message);
            std::process::exit(1);
        }
    };
    match seed::seed_if_empty(&store, &seed_path) {
        Ok(0) => tracing::info!("Base de datos existente en {} (sin recarga de seed)", db_path.display()),
        Ok(n) => tracing::info!(
            "Base vacía: se cargaron {n} documentos desde {}. Usuarios con contraseña \"{}\".",
            seed_path.display(),
            auth::DEFAULT_PASSWORD
        ),
        Err(e) => tracing::warn!("No se pudo cargar el seed: {}", e.message),
    }

    let frontend_dist = {
        let p = PathBuf::from(env_or("FRONTEND_DIST", "../Tenis-Ahora-Frontend/dist"));
        if p.join("index.html").exists() {
            tracing::info!("Sirviendo el front desde {}", p.display());
            Some(p)
        } else {
            tracing::info!("Sin build del front en {} (solo API). Generarlo con `npm run build` en el front.", p.display());
            None
        }
    };

    let state = AppState { store, jwt_secret, seed_path, started_at: Instant::now() };
    let app = build_router(state, frontend_dist);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = match tokio::net::TcpListener::bind(addr).await {
        Ok(l) => l,
        Err(e) => {
            eprintln!("No se pudo escuchar en {addr}: {e}");
            std::process::exit(1);
        }
    };
    tracing::info!("Tenis Ahora API v{VERSION} escuchando en http://localhost:{port}/api  (health: /api/health)");
    if let Err(e) = axum::serve(listener, app).await {
        eprintln!("El servidor terminó con error: {e}");
    }
}
