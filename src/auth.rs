//! Autenticación: login / registro con bcrypt + JWT HS256 (firmado a mano con hmac/sha2 para no
//! depender de crates con código C). El front guarda el token y lo manda como `Bearer`.

use axum::{
    extract::{Request, State},
    http::StatusCode,
    middleware::Next,
    response::Response,
    Json,
};
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use hmac::{Hmac, Mac};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::Sha256;

use crate::{
    error::{ApiError, ApiResult},
    util::{now_millis, s, today},
    AppState,
};

type HmacSha256 = Hmac<Sha256>;

pub const DEFAULT_PASSWORD: &str = "123456";
const TOKEN_TTL_SECS: i64 = 8 * 60 * 60; // 8 horas

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct Claims {
    pub sub: String,
    pub email: String,
    pub role: String,
    pub iat: i64,
    pub exp: i64,
}

// ---------------------------------------------------------------------------
// JWT
// ---------------------------------------------------------------------------
pub fn sign_token(secret: &str, claims: &Claims) -> String {
    let header = URL_SAFE_NO_PAD.encode(br#"{"alg":"HS256","typ":"JWT"}"#);
    let payload = URL_SAFE_NO_PAD.encode(serde_json::to_vec(claims).unwrap_or_default());
    let signing_input = format!("{header}.{payload}");
    let mut mac = HmacSha256::new_from_slice(secret.as_bytes()).expect("hmac key");
    mac.update(signing_input.as_bytes());
    let sig = URL_SAFE_NO_PAD.encode(mac.finalize().into_bytes());
    format!("{signing_input}.{sig}")
}

pub fn verify_token(secret: &str, token: &str) -> Result<Claims, String> {
    let parts: Vec<&str> = token.split('.').collect();
    if parts.len() != 3 {
        return Err("Formato de token inválido".into());
    }
    let signing_input = format!("{}.{}", parts[0], parts[1]);
    let mut mac = HmacSha256::new_from_slice(secret.as_bytes()).map_err(|e| e.to_string())?;
    mac.update(signing_input.as_bytes());
    let sig = URL_SAFE_NO_PAD.decode(parts[2]).map_err(|_| "Firma ilegible")?;
    mac.verify_slice(&sig).map_err(|_| "Firma inválida (¿token de Modo Demo?)")?;
    let payload = URL_SAFE_NO_PAD.decode(parts[1]).map_err(|_| "Payload ilegible")?;
    let claims: Claims = serde_json::from_slice(&payload).map_err(|_| "Claims inválidos")?;
    if claims.exp < now_millis() / 1000 {
        return Err("Token vencido".into());
    }
    Ok(claims)
}

/// Middleware: exige `Authorization: Bearer <jwt>` válido en todas las rutas protegidas.
pub async fn require_auth(State(state): State<AppState>, mut req: Request, next: Next) -> Result<Response, ApiError> {
    let header = req
        .headers()
        .get(axum::http::header::AUTHORIZATION)
        .and_then(|h| h.to_str().ok())
        .unwrap_or("");
    let token = header.strip_prefix("Bearer ").unwrap_or("");
    if token.is_empty() {
        return Err(ApiError::unauthorized("Falta el token de autenticación (Authorization: Bearer ...)."));
    }
    let claims = verify_token(&state.jwt_secret, token).map_err(|m| ApiError::unauthorized(format!("Token rechazado: {m}")))?;
    req.extensions_mut().insert(claims);
    Ok(next.run(req).await)
}

// ---------------------------------------------------------------------------
// Usuarios <-> AuthResponseDto
// ---------------------------------------------------------------------------
pub fn hash_password(plain: &str) -> String {
    bcrypt::hash(plain, 4).unwrap_or_default()
}

fn backend_role(user: &Value) -> &'static str {
    match s(user, "role").as_str() {
        "admin" => "Empleado",
        "coach" => "Profesor",
        _ => "Socio",
    }
}

fn split_name(user: &Value) -> (String, String) {
    let nombre = s(user, "nombre");
    let apellido = s(user, "apellido");
    if !nombre.is_empty() || !apellido.is_empty() {
        return (nombre, apellido);
    }
    let full = s(user, "name");
    let mut parts = full.splitn(2, ' ');
    let first = parts.next().unwrap_or("").to_string();
    let rest = parts.next().unwrap_or("").to_string();
    (first, rest)
}

/// Misma forma que el `AuthResponseDto` del backend .NET original, más `dni` y `memberNumber`.
pub fn auth_response(state: &AppState, user: &Value) -> Value {
    let (nombre, apellido) = split_name(user);
    let now = now_millis() / 1000;
    let claims = Claims {
        sub: s(user, "id"),
        email: s(user, "email"),
        role: backend_role(user).to_string(),
        iat: now,
        exp: now + TOKEN_TTL_SECS,
    };
    json!({
        "token": sign_token(&state.jwt_secret, &claims),
        "id": s(user, "id"),
        "nombre": nombre,
        "apellido": apellido,
        "email": s(user, "email"),
        "numeroTelefono": s(user, "phone"),
        "direccion": s(user, "address"),
        "rol": backend_role(user),
        "dni": s(user, "dni"),
        "memberNumber": s(user, "memberNumber"),
        "expiraEn": claims.exp
    })
}

#[derive(Deserialize)]
pub struct LoginBody {
    pub email: String,
    pub password: String,
}

/// POST /api/auth/login
pub async fn login(State(state): State<AppState>, Json(body): Json<LoginBody>) -> ApiResult<Json<Value>> {
    let email = body.email.trim().to_lowercase();
    let user = state
        .store
        .list("usuarios")?
        .into_iter()
        .find(|u| s(u, "email").to_lowercase() == email)
        .ok_or_else(|| ApiError::unauthorized("Credenciales inválidas."))?;
    let hash = s(&user, "passwordHash");
    let ok = !hash.is_empty() && bcrypt::verify(&body.password, &hash).unwrap_or(false);
    if !ok {
        return Err(ApiError::unauthorized("Credenciales inválidas."));
    }
    Ok(Json(auth_response(&state, &user)))
}

#[derive(Deserialize)]
pub struct RegisterBody {
    pub nombre: String,
    pub apellido: String,
    pub email: String,
    pub password: String,
    #[serde(default)]
    pub direccion: String,
    #[serde(rename = "numeroTelefono", default)]
    pub numero_telefono: String,
    #[serde(default)]
    pub dni: String,
}

/// POST /api/auth/registrar — crea un usuario con rol Socio.
pub async fn register(State(state): State<AppState>, Json(body): Json<RegisterBody>) -> ApiResult<(StatusCode, Json<Value>)> {
    let email = body.email.trim().to_lowercase();
    if email.is_empty() || body.password.len() < 4 {
        return Err(ApiError::bad_request("Email y contraseña (mínimo 4 caracteres) son obligatorios."));
    }
    if state.store.list("usuarios")?.iter().any(|u| s(u, "email").to_lowercase() == email) {
        return Err(ApiError::conflict("Ya existe un usuario registrado con ese email."));
    }
    let id = format!("usr-{}", now_millis());
    let user = json!({
        "id": id,
        "name": format!("{} {}", body.nombre.trim(), body.apellido.trim()).trim(),
        "nombre": body.nombre.trim(),
        "apellido": body.apellido.trim(),
        "dni": body.dni,
        "phone": body.numero_telefono,
        "email": email,
        "address": body.direccion,
        "role": "client",
        "memberNumber": format!("TA-{}", 1000 + (now_millis() % 9000)),
        "memberSince": today(),
        "activeDiscounts": [],
        "totalBookings": 0,
        "passwordHash": hash_password(&body.password)
    });
    state.store.insert("usuarios", &user)?;
    Ok((StatusCode::CREATED, Json(auth_response(&state, &user))))
}

/// GET /api/auth/yo — devuelve los claims del token (útil para probar la autenticación).
pub async fn me(axum::Extension(claims): axum::Extension<Claims>) -> Json<Claims> {
    Json(claims)
}
