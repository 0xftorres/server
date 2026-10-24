//! CRUD genérico por colección (`/api/{coleccion}` y `/api/{coleccion}/{id}`) con las reglas de
//! alta/modificación/baja que tenía el mock del front (`src/services/api.js`).

use axum::{
    extract::{Path, State},
    http::StatusCode,
    Json,
};
use serde_json::{json, Value};

use crate::{
    auth::{hash_password, DEFAULT_PASSWORD},
    business,
    error::{ApiError, ApiResult},
    seed::COLLECTIONS,
    util::*,
    AppState,
};

pub fn validate_collection(col: &str) -> ApiResult<()> {
    if COLLECTIONS.contains(&col) {
        Ok(())
    } else {
        Err(ApiError::not_found(format!(
            "La colección `{col}` no existe. Disponibles: {}",
            COLLECTIONS.join(", ")
        )))
    }
}

/// Oculta campos sensibles antes de responder.
pub fn sanitize(col: &str, mut doc: Value) -> Value {
    if col == "usuarios" {
        if let Some(m) = doc.as_object_mut() {
            m.remove("passwordHash");
        }
    }
    doc
}

// ---------------------------------------------------------------------------
// Generación de identificadores (mismos prefijos que el front)
// ---------------------------------------------------------------------------
pub fn next_id(state: &AppState, col: &str, doc: &Value) -> ApiResult<String> {
    let ms = now_millis();
    let count = state.store.count(col)?;
    let base = match col {
        "usuarios" => format!("usr-{ms}"),
        "canchas" => format!("cancha-{ms}"),
        "tipos-cancha" => format!("ct-{ms}"),
        "stock" => format!("stock-{ms}"),
        "torneos" => format!("trn-{ms}"),
        "ligas" => format!("liga-{ms}"),
        "tipos-competencia" => format!("cmp-{ms}"),
        "profesores" => format!("coach-{ms}"),
        "clases" => format!("cls-{ms}"),
        "clases-particulares" => format!("prv-{ms}"),
        "descuentos" => format!("disc-{ms}"),
        "reservas" => format!("RES-{}-{:03}", year(), count + 1),
        "recibos" => format!("REC-{}-{}", year(), 1050 + count),
        "facturas" => {
            let t = { let t = s(doc, "type"); if t.is_empty() { "B".to_string() } else { t } };
            format!("FC-{t}-0001-{:08}", 412 + count + 1)
        }
        "reembolsos" => format!("RMB-{}-{:04}", year(), count + 9),
        _ => format!("{col}-{ms}"),
    };
    let mut id = base.clone();
    let mut n = 1;
    while state.store.exists(col, &id)? {
        n += 1;
        id = format!("{base}-{n}");
    }
    Ok(id)
}

// ---------------------------------------------------------------------------
// Reglas de alta
// ---------------------------------------------------------------------------
pub fn apply_create_defaults(state: &AppState, col: &str, body: Value) -> ApiResult<Value> {
    if !body.is_object() {
        return Err(ApiError::bad_request("El cuerpo debe ser un objeto JSON."));
    }
    let mut d = body;
    if s(&d, "id").is_empty() {
        let id = next_id(state, col, &d)?;
        set(&mut d, "id", Value::String(id));
    }
    let default_str = |d: &mut Value, k: &str, v: &str| {
        if s(d, k).is_empty() {
            set(d, k, Value::String(v.to_string()));
        }
    };
    let default_val = |d: &mut Value, k: &str, v: Value| {
        if d.get(k).is_none() || d.get(k) == Some(&Value::Null) {
            set(d, k, v);
        }
    };

    match col {
        "usuarios" => {
            let email = s(&d, "email").trim().to_lowercase();
            if email.is_empty() {
                return Err(ApiError::bad_request("El email es obligatorio."));
            }
            if state.store.list(col)?.iter().any(|u| s(u, "email").to_lowercase() == email) {
                return Err(ApiError::conflict("Ya existe un usuario registrado con ese email."));
            }
            set(&mut d, "email", Value::String(email));
            default_str(&mut d, "role", "client");
            let is_admin = s(&d, "role") == "admin";
            if s(&d, "memberNumber").is_empty() {
                let admins = state.store.list(col)?.iter().filter(|u| s(u, "role") == "admin").count();
                let mn = if is_admin { format!("ADM-{:03}", admins + 1) } else { format!("TA-{}", 1000 + now_millis() % 9000) };
                set(&mut d, "memberNumber", Value::String(mn));
            }
            default_str(&mut d, "memberSince", &today());
            default_val(&mut d, "activeDiscounts", json!([]));
            default_val(&mut d, "totalBookings", json!(0));
            let plain = s(&d, "password");
            if let Some(m) = d.as_object_mut() { m.remove("password"); }
            if s(&d, "passwordHash").is_empty() {
                let pw = if plain.is_empty() { DEFAULT_PASSWORD.to_string() } else { plain };
                set(&mut d, "passwordHash", Value::String(hash_password(&pw)));
            }
        }
        "canchas" => {
            default_str(&mut d, "status", "disponible");
            default_str(&mut d, "maintenanceNotes", "");
            default_val(&mut d, "schedule", json!([]));
            default_val(&mut d, "availability", business::default_availability());
            default_str(&mut d, "photo", "https://images.unsplash.com/photo-1595435934249-5df7ed86e1c0?auto=format&fit=crop&w=600&q=80");
        }
        "tipos-cancha" => {
            let name = s(&d, "name").trim().to_string();
            if name.is_empty() {
                return Err(ApiError::bad_request("El nombre del tipo de cancha es obligatorio."));
            }
            if state.store.list(col)?.iter().any(|t| s(t, "name").to_lowercase() == name.to_lowercase()) {
                return Err(ApiError::conflict("Ya existe un tipo de cancha con ese nombre."));
            }
            set(&mut d, "name", Value::String(name.clone()));
            default_str(&mut d, "label", &name);
            default_str(&mut d, "description", "");
            default_str(&mut d, "speed", "Media");
            default_str(&mut d, "maintenanceFrequency", "");
            { let v__ = num(f(&d, "basePricePerHour")); set(&mut d, "basePricePerHour", v__); }
            let cap = f(&d, "maxStudentsPerClass");
            set(&mut d, "maxStudentsPerClass", num(if cap <= 0.0 { 30.0 } else { cap.min(30.0) }));
            default_val(&mut d, "active", json!(true));
        }
        "stock" => {
            let total = f(&d, "totalStock");
            let in_use = f(&d, "inUseStock");
            let available = (total - in_use).max(0.0);
            let min = { let m = f(&d, "minAlertThreshold"); if m <= 0.0 { 1.0 } else { m } };
            default_str(&mut d, "category", "Equipamiento");
            default_str(&mut d, "description", "");
            default_str(&mut d, "unit", "unidades");
            set(&mut d, "totalStock", num(total));
            set(&mut d, "inUseStock", num(in_use));
            set(&mut d, "availableStock", num(available));
            set(&mut d, "minAlertThreshold", num(min));
            set(&mut d, "status", Value::String(if available <= min { "alerta_baja" } else { "normal" }.into()));
        }
        "torneos" => {
            default_val(&mut d, "currentEnrolled", json!(0));
            default_str(&mut d, "status", "inscripcion_abierta");
            default_str(&mut d, "internalRules", "Reglamento Interno: Partidos al mejor de 3 sets con tie-break oficial.");
            default_val(&mut d, "participants", json!([]));
            default_val(&mut d, "bracket", json!([]));
        }
        "ligas" => {
            default_val(&mut d, "currentEnrolled", json!(0));
            default_str(&mut d, "status", "inscripcion_abierta");
            default_str(&mut d, "internalRules", "Reglamento Interno: Sistema todos contra todos por puntos.");
            default_val(&mut d, "participants", json!([]));
            default_val(&mut d, "standings", json!([]));
            default_val(&mut d, "fixtures", json!([]));
        }
        "tipos-competencia" => {
            if s(&d, "name").trim().is_empty() {
                return Err(ApiError::bad_request("El nombre del tipo de competencia es obligatorio."));
            }
            default_str(&mut d, "format", "");
            default_str(&mut d, "appliesTo", "Torneo");
            default_str(&mut d, "scoringSystem", "");
            default_str(&mut d, "description", "");
            { let v__ = num({ let v = f(&d, "minParticipants"); if v <= 0.0 { 2.0 } else { v } }); set(&mut d, "minParticipants", v__); }
            { let v__ = num({ let v = f(&d, "maxParticipants"); if v <= 0.0 { 32.0 } else { v } }); set(&mut d, "maxParticipants", v__); }
            default_val(&mut d, "active", json!(true));
        }
        "profesores" => {
            default_val(&mut d, "hasOfficialLicense", json!(true));
            let lic = d.get("licenseDetails").cloned().unwrap_or(json!({}));
            let mut lic = merge(&json!({
                "titleName": "Profesor de Tenis Certificado",
                "institution": "Asociación Argentina de Tenis (AAT)",
                "licenseNumber": format!("AAT-LIC-{}", 10000 + rand4() * 7 % 90000),
                "issueDate": today(),
                "expiryDate": "2029-12-31",
                "status": "habilitado"
            }), &lic);
            if s(&lic, "digitalDocumentUrl").is_empty() {
                let n = s(&lic, "licenseNumber");
                set(&mut lic, "digitalDocumentUrl", Value::String(format!("Certificado_{n}.pdf")));
            }
            set(&mut d, "licenseDetails", lic);
            default_str(&mut d, "type", "Profesor");
            default_str(&mut d, "photo", "https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?auto=format&fit=crop&w=400&q=80");
        }
        "clases" => {
            default_str(&mut d, "kind", "clase");
            default_str(&mut d, "type", "grupal");
            let cap = f(&d, "maxCapacity");
            if cap > 30.0 {
                return Err(ApiError::bad_request("El cupo reglamentario máximo es de hasta 30 alumnos."));
            }
            set(&mut d, "maxCapacity", num(if cap <= 0.0 { 30.0 } else { cap }));
            default_val(&mut d, "currentEnrolled", json!(0));
            default_val(&mut d, "students", json!([]));
            default_val(&mut d, "sessions", json!(["02/10", "04/10", "09/10", "11/10", "16/10", "18/10", "23/10", "25/10"]));
        }
        "clases-particulares" => {
            default_str(&mut d, "kind", "clase");
            default_str(&mut d, "status", "programada");
        }
        "recibos" => {
            default_str(&mut d, "date", &now_stamp());
            default_str(&mut d, "status", "completado");
            default_str(&mut d, "cashierName", "Administración General");
        }
        "facturas" => {
            d = business::compute_invoice(d)?;
        }
        "reembolsos" => {
            d = business::compute_refund(d)?;
        }
        "descuentos" => {
            let code = s(&d, "code").trim().to_uppercase();
            if code.is_empty() {
                return Err(ApiError::bad_request("El código del descuento es obligatorio."));
            }
            if state.store.list(col)?.iter().any(|x| s(x, "code") == code) {
                return Err(ApiError::conflict("Ya existe un descuento con ese código."));
            }
            set(&mut d, "code", Value::String(code));
            { let v__ = num(f(&d, "discountPercent").clamp(0.0, 100.0)); set(&mut d, "discountPercent", v__); }
            default_str(&mut d, "applicableTo", "Alquiler de Canchas");
            default_val(&mut d, "active", json!(true));
        }
        _ => {}
    }
    Ok(d)
}

// ---------------------------------------------------------------------------
// Reglas de modificación
// ---------------------------------------------------------------------------
pub fn apply_update_rules(state: &AppState, col: &str, existing: &Value, patch: &Value) -> ApiResult<Value> {
    if !patch.is_object() {
        return Err(ApiError::bad_request("El cuerpo debe ser un objeto JSON."));
    }
    let id = s(existing, "id");
    let mut d = merge(existing, patch);
    set(&mut d, "id", Value::String(id.clone())); // el id no se cambia por PUT

    match col {
        "usuarios" => {
            let plain = s(&d, "password");
            if let Some(m) = d.as_object_mut() { m.remove("password"); }
            if !plain.is_empty() {
                set(&mut d, "passwordHash", Value::String(hash_password(&plain)));
            }
            if s(&d, "passwordHash").is_empty() {
                set(&mut d, "passwordHash", existing.get("passwordHash").cloned().unwrap_or(Value::Null));
            }
            let email = s(&d, "email").trim().to_lowercase();
            if state.store.list(col)?.iter().any(|u| s(u, "id") != id && s(u, "email").to_lowercase() == email) {
                return Err(ApiError::conflict("Ya existe otro usuario con ese email."));
            }
            set(&mut d, "email", Value::String(email));
        }
        "stock" => {
            let total = f(&d, "totalStock");
            let in_use = f(&d, "inUseStock");
            let available = (total - in_use).max(0.0);
            let min = f(&d, "minAlertThreshold");
            set(&mut d, "totalStock", num(total));
            set(&mut d, "inUseStock", num(in_use));
            set(&mut d, "availableStock", num(available));
            set(&mut d, "minAlertThreshold", num(min));
            set(&mut d, "status", Value::String(if available <= min { "alerta_baja" } else { "normal" }.into()));
        }
        "tipos-cancha" => {
            let name = s(&d, "name").trim().to_string();
            if state.store.list(col)?.iter().any(|t| s(t, "id") != id && s(t, "name").to_lowercase() == name.to_lowercase()) {
                return Err(ApiError::conflict("Ya existe otro tipo de cancha con ese nombre."));
            }
            { let v__ = num(f(&d, "basePricePerHour")); set(&mut d, "basePricePerHour", v__); }
            let cap = f(&d, "maxStudentsPerClass");
            set(&mut d, "maxStudentsPerClass", num(if cap <= 0.0 { 30.0 } else { cap.min(30.0) }));
        }
        "clases" => {
            let cap = f(&d, "maxCapacity");
            if cap > 30.0 {
                return Err(ApiError::bad_request("El cupo reglamentario máximo es de hasta 30 alumnos."));
            }
            if cap <= 0.0 {
                set(&mut d, "maxCapacity", num(30.0));
            }
        }
        "descuentos" => {
            let code = s(&d, "code").trim().to_uppercase();
            if state.store.list(col)?.iter().any(|x| s(x, "id") != id && s(x, "code") == code) {
                return Err(ApiError::conflict("Ya existe otro descuento con ese código."));
            }
            set(&mut d, "code", Value::String(code));
            { let v__ = num(f(&d, "discountPercent").clamp(0.0, 100.0)); set(&mut d, "discountPercent", v__); }
        }
        "reservas" => {
            d = business::recalculate_reservation(state, existing, patch)?;
        }
        _ => {}
    }
    Ok(d)
}

// ---------------------------------------------------------------------------
// Reglas de baja
// ---------------------------------------------------------------------------
pub fn check_delete(state: &AppState, col: &str, doc: &Value) -> ApiResult<()> {
    match col {
        "tipos-cancha" => {
            let name = s(doc, "name");
            if state.store.list("canchas")?.iter().any(|c| s(c, "surfaceType") == name) {
                return Err(ApiError::conflict(format!(
                    "No se puede eliminar: existen canchas registradas con superficie \"{name}\"."
                )));
            }
        }
        "descuentos" => {
            // Cascada: quitar el código de los usuarios que lo tenían asignado.
            let code = s(doc, "code");
            for mut u in state.store.list("usuarios")? {
                let codes = arr(&u, "activeDiscounts");
                if codes.iter().any(|c| c.as_str() == Some(code.as_str())) {
                    let next: Vec<Value> = codes.into_iter().filter(|c| c.as_str() != Some(code.as_str())).collect();
                    set(&mut u, "activeDiscounts", Value::Array(next));
                    state.store.upsert("usuarios", &u)?;
                }
            }
        }
        _ => {}
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------
pub fn list_collection(state: &AppState, col: &str) -> ApiResult<Json<Value>> {
    validate_collection(col)?;
    let items: Vec<Value> = state.store.list(col)?.into_iter().map(|d| sanitize(col, d)).collect();
    Ok(Json(Value::Array(items)))
}

pub async fn list(State(state): State<AppState>, Path(col): Path<String>) -> ApiResult<Json<Value>> {
    list_collection(&state, &col)
}

pub async fn get_one(State(state): State<AppState>, Path((col, id)): Path<(String, String)>) -> ApiResult<Json<Value>> {
    validate_collection(&col)?;
    Ok(Json(sanitize(&col, state.store.require(&col, &id)?)))
}

pub async fn create(
    State(state): State<AppState>,
    Path(col): Path<String>,
    Json(body): Json<Value>,
) -> ApiResult<(StatusCode, Json<Value>)> {
    validate_collection(&col)?;
    if col == "reservas" {
        // Las reservas tienen alta propia (genera recibo de seña): ver business::create_reservation.
        let res = business::create_reservation_inner(&state, body)?;
        return Ok((StatusCode::CREATED, Json(res)));
    }
    let doc = apply_create_defaults(&state, &col, body)?;
    state.store.insert(&col, &doc)?;
    Ok((StatusCode::CREATED, Json(sanitize(&col, doc))))
}

pub async fn update(
    State(state): State<AppState>,
    Path((col, id)): Path<(String, String)>,
    Json(body): Json<Value>,
) -> ApiResult<Json<Value>> {
    validate_collection(&col)?;
    let existing = state.store.require(&col, &id)?;
    let doc = apply_update_rules(&state, &col, &existing, &body)?;
    state.store.upsert(&col, &doc)?;
    Ok(Json(sanitize(&col, doc)))
}

pub async fn remove(State(state): State<AppState>, Path((col, id)): Path<(String, String)>) -> ApiResult<Json<Value>> {
    validate_collection(&col)?;
    let doc = state.store.require(&col, &id)?;
    check_delete(&state, &col, &doc)?;
    state.store.delete(&col, &id)?;
    Ok(Json(json!({ "success": true })))
}
