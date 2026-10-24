//! Endpoints con reglas de negocio (reservas, torneos, ligas, clases, caja...).
//! Son el equivalente servidor de la lógica que tenía el mock del front, así el front recibe
//! exactamente las mismas formas de respuesta.

use axum::{
    extract::{Path, State},
    http::StatusCode,
    Json,
};
use serde_json::{json, Map, Value};

use crate::{
    crud::sanitize,
    error::{ApiError, ApiResult},
    seed,
    util::*,
    AppState,
};

type J = Json<Value>;

fn body_obj(body: &Value) -> ApiResult<&Map<String, Value>> {
    body.as_object().ok_or_else(|| ApiError::bad_request("El cuerpo debe ser un objeto JSON."))
}

// ===========================================================================
// Canchas
// ===========================================================================
pub fn default_availability() -> Value {
    json!({
        "days": ["Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado", "Domingo"],
        "openTime": "08:00",
        "closeTime": "22:00",
        "slotMinutes": 60,
        "blockedRanges": [],
        "notes": ""
    })
}

/// PATCH /api/canchas/{id}/estado  { status, maintenanceNotes }
pub async fn court_status(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut court = state.store.require("canchas", &id)?;
    let status = s(&body, "status");
    if !["disponible", "ocupada", "mantenimiento"].contains(&status.as_str()) {
        return Err(ApiError::bad_request("status debe ser disponible, ocupada o mantenimiento."));
    }
    set(&mut court, "status", Value::String(status));
    set(&mut court, "maintenanceNotes", Value::String(s(&body, "maintenanceNotes")));
    state.store.upsert("canchas", &court)?;
    Ok(Json(court))
}

/// PUT /api/canchas/{id}/disponibilidad  { days, openTime, closeTime, slotMinutes, blockedRanges, notes }
pub async fn court_availability(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    body_obj(&body)?;
    let mut court = state.store.require("canchas", &id)?;
    let current = court.get("availability").cloned().unwrap_or(json!({}));
    let next = merge(&merge(&default_availability(), &current), &body);
    if arr(&next, "days").is_empty() {
        return Err(ApiError::bad_request("Debe indicar al menos un día de atención."));
    }
    if s(&next, "closeTime") <= s(&next, "openTime") {
        return Err(ApiError::bad_request("El horario de cierre debe ser posterior al de apertura."));
    }
    set(&mut court, "availability", next);
    state.store.upsert("canchas", &court)?;
    Ok(Json(court))
}

// ===========================================================================
// Stock
// ===========================================================================
/// PATCH /api/stock/{id}/ajuste  { totalStock }
pub async fn stock_adjust(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut item = state.store.require("stock", &id)?;
    let total = f(&body, "totalStock").max(0.0);
    let in_use = f(&item, "inUseStock");
    let available = (total - in_use).max(0.0);
    let min = f(&item, "minAlertThreshold");
    set(&mut item, "totalStock", num(total));
    set(&mut item, "availableStock", num(available));
    set(&mut item, "status", Value::String(if available <= min { "alerta_baja" } else { "normal" }.into()));
    state.store.upsert("stock", &item)?;
    Ok(Json(item))
}

// ===========================================================================
// Reservas
// ===========================================================================
fn lead(res: &Value) -> Value {
    arr(res, "participants").first().cloned().unwrap_or(json!({}))
}

/// POST /api/reservas — alta con recibo de seña (RD01: se confirma con el 50% abonado).
pub fn create_reservation_inner(state: &AppState, body: Value) -> ApiResult<Value> {
    body_obj(&body)?;
    let court_id = s(&body, "courtId");
    if !court_id.is_empty() {
        if let Some(court) = state.store.get("canchas", &court_id)? {
            if s(&court, "status") == "mantenimiento" {
                return Err(ApiError::conflict("La cancha está bloqueada por mantenimiento y no admite reservas (RD06)."));
            }
        }
    }
    let count = state.store.count("reservas")?;
    let mut id = format!("RES-{}-{:03}", year(), count + 1);
    let mut n = 1;
    while state.store.exists("reservas", &id)? {
        n += 1;
        id = format!("RES-{}-{:03}-{n}", year(), count + 1);
    }
    let rec_id = format!("REC-50-{}", rand4());

    let mut res = body.clone();
    set(&mut res, "id", Value::String(id.clone()));
    set(&mut res, "depositReceiptNumber", Value::String(rec_id.clone()));
    set(&mut res, "remainingPaid", json!(false));
    set(&mut res, "finalReceiptNumber", Value::Null);
    set(&mut res, "status", Value::String("confirmada".into()));
    set(&mut res, "createdAt", Value::String(now_iso()));
    set(&mut res, "cancellationDetails", Value::Null);
    if res.get("participants").is_none() {
        set(&mut res, "participants", json!([]));
    }
    state.store.insert("reservas", &res)?;

    let l = lead(&res);
    let receipt = json!({
        "id": rec_id,
        "reservationId": id,
        "clientName": if s(&l, "name").is_empty() { "Socio Tenis Ahora".to_string() } else { s(&l, "name") },
        "clientDni": if s(&l, "dni").is_empty() { "00.000.000".to_string() } else { s(&l, "dni") },
        "concept": format!("Seña 50% Obligatoria - {}", s(&res, "courtName")),
        "items": [
            { "description": format!("Alquiler {}hs {}", f(&res, "durationHours"), s(&res, "courtName")), "amount": num(f(&res, "courtCost")) },
            { "description": "Equipamiento incluido (Red, Pelotas, Raquetas)", "amount": 0 },
            { "description": "Total Turno", "amount": num(f(&res, "totalCost")) },
            { "description": "Cobro de Seña 50% para Confirmación", "amount": num(f(&res, "depositPaid")) }
        ],
        "totalPaid": num(f(&res, "depositPaid")),
        "paymentMethod": if s(&res, "depositPaymentMethod").is_empty() { "Mercado Pago (QR)".to_string() } else { s(&res, "depositPaymentMethod") },
        "status": "completado",
        "date": now_stamp(),
        "qrData": format!("https://mpago.la/pos/tenisahora/{}", rec_id.to_lowercase()),
        "cashierName": "Cobro Digital Online"
    });
    state.store.insert("recibos", &receipt)?;

    Ok(json!({ "data": res, "receipt": receipt }))
}

pub async fn create_reservation(State(state): State<AppState>, Json(body): Json<Value>) -> ApiResult<(StatusCode, J)> {
    Ok((StatusCode::CREATED, Json(create_reservation_inner(&state, body)?)))
}

pub async fn list_reservations(State(state): State<AppState>) -> ApiResult<J> {
    crate::crud::list_collection(&state, "reservas")
}

/// Recalcula hora de fin, costo y saldo al modificar una reserva (PUT /api/reservas/{id}).
pub fn recalculate_reservation(state: &AppState, existing: &Value, patch: &Value) -> ApiResult<Value> {
    let mut next = merge(existing, patch);
    set(&mut next, "id", existing.get("id").cloned().unwrap_or(Value::Null));
    let dur = { let d = f(&next, "durationHours"); if d <= 0.0 { 1.0 } else { d } };

    if patch.get("startTime").is_some() || patch.get("durationHours").is_some() {
        let start = s(&next, "startTime");
        let mut parts = start.split(':');
        let h: i64 = parts.next().unwrap_or("8").trim().parse().unwrap_or(8);
        let m: i64 = parts.next().unwrap_or("0").trim().parse().unwrap_or(0);
        let total_min = h * 60 + m + (dur * 60.0).round() as i64;
        set(&mut next, "endTime", Value::String(format!("{:02}:{:02}", total_min / 60, total_min % 60)));
    }

    let court_id = s(&next, "courtId");
    if let Some(court) = state.store.get("canchas", &court_id)? {
        if patch.get("courtId").is_some() && s(&court, "status") == "mantenimiento" {
            return Err(ApiError::conflict("La cancha seleccionada está bloqueada por mantenimiento (RD06)."));
        }
        if patch.get("courtId").is_some() || patch.get("durationHours").is_some() {
            if patch.get("courtId").is_some() {
                set(&mut next, "courtName", Value::String(format!("{} ({})", s(&court, "name"), s(&court, "surfaceType"))));
                set(&mut next, "surfaceType", Value::String(s(&court, "surfaceType")));
            }
            let court_cost = f(&court, "pricePerHour") * dur;
            let total = court_cost + f(&next, "equipmentCost");
            set(&mut next, "courtCost", num(court_cost));
            set(&mut next, "totalCost", num(total));
            { let v__ = num((total - f(&next, "depositPaid")).max(0.0)); set(&mut next, "remainingBalance", v__); }
        }
    }
    set(&mut next, "updatedAt", Value::String(now_iso()));
    Ok(next)
}

/// POST /api/reservas/{id}/pagar-saldo  { paymentMethod }
pub async fn pay_remaining(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut res = state.store.require("reservas", &id)?;
    if b(&res, "remainingPaid", false) {
        return Err(ApiError::conflict("El saldo de esta reserva ya fue liquidado."));
    }
    if s(&res, "status") == "cancelada" {
        return Err(ApiError::conflict("La reserva está cancelada."));
    }
    let method = { let m = s(&body, "paymentMethod"); if m.is_empty() { "Efectivo".to_string() } else { m } };
    let final_id = format!("REC-FINAL-{}", rand4());
    set(&mut res, "remainingPaid", json!(true));
    set(&mut res, "status", Value::String("finalizada".into()));
    set(&mut res, "finalReceiptNumber", Value::String(final_id.clone()));
    state.store.upsert("reservas", &res)?;

    let l = lead(&res);
    let receipt = json!({
        "id": final_id,
        "reservationId": id,
        "clientName": if s(&l, "name").is_empty() { "Socio Tenis Ahora".to_string() } else { s(&l, "name") },
        "clientDni": if s(&l, "dni").is_empty() { "00.000.000".to_string() } else { s(&l, "dni") },
        "concept": format!("Liquidación Final 50% Saldo - {}", s(&res, "courtName")),
        "items": [ { "description": "Saldo restante 50% de turno finalizado", "amount": num(f(&res, "remainingBalance")) } ],
        "totalPaid": num(f(&res, "remainingBalance")),
        "paymentMethod": method,
        "status": "completado",
        "date": now_stamp(),
        "qrData": format!("https://mpago.la/pos/tenisahora/{}", final_id.to_lowercase()),
        "cashierName": "Recepción del Club"
    });
    state.store.insert("recibos", &receipt)?;
    Ok(Json(res))
}

/// POST /api/reservas/{id}/cancelar  { hoursInAdvance }
/// RD02-RD04: con 6 hs o más de antelación corresponde reembolso total; con menos, se retiene
/// el 50% de la seña como cargo y se reintegra el resto.
pub async fn cancel_reservation(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut res = state.store.require("reservas", &id)?;
    if s(&res, "status") == "cancelada" {
        return Err(ApiError::conflict("La reserva ya está cancelada."));
    }
    let hours = f(&body, "hoursInAdvance");
    let within = hours >= 6.0;
    set(&mut res, "status", Value::String("cancelada".into()));
    set(&mut res, "cancellationDetails", json!({
        "cancelledAt": now_iso(),
        "hoursInAdvance": num(hours),
        "policyApplied": if within {
            "Cancelación anticipada (>6hs): Reembolso de seña según política."
        } else {
            "Cancelación tardía (<6hs): Se aplica retención del 50% de seña por costo operativo."
        }
    }));
    state.store.upsert("reservas", &res)?;

    let l = lead(&res);
    let client_name = if s(&l, "name").is_empty() { "Socio Tenis Ahora".to_string() } else { s(&l, "name") };
    let client_dni = if s(&l, "dni").is_empty() { "00.000.000".to_string() } else { s(&l, "dni") };
    let deposit = f(&res, "depositPaid");
    let refund_count = state.store.count("reembolsos")?;
    let refund_id = format!("RMB-{}-{:04}", year(), refund_count + 9);

    let mut penalty: Value = Value::Null;
    let refund = if within {
        json!({
            "id": refund_id,
            "clientName": client_name, "clientDni": client_dni,
            "relatedReservationId": id,
            "relatedReceiptId": res.get("depositReceiptNumber").cloned().unwrap_or(Value::Null),
            "reason": format!("Cancelación de reserva {id} con {hours} hs de antelación (dentro del plazo reglamentario)."),
            "refundType": "total",
            "originalAmount": num(deposit), "refundAmount": num(deposit),
            "refundMethod": format!("Devolución a {}", { let m = s(&res, "depositPaymentMethod"); if m.is_empty() { "medio de pago original".to_string() } else { m } }),
            "status": "pendiente", "date": now_stamp(), "processedBy": "Sistema de Reservas"
        })
    } else {
        let retained = (deposit * 0.5).round();
        penalty = json!({
            "id": format!("REC-CARGO-{}", rand4()),
            "reservationId": id,
            "clientName": client_name, "clientDni": client_dni,
            "concept": format!("Cargo por cancelación fuera de plazo - {}", s(&res, "courtName")),
            "items": [
                { "description": format!("Seña abonada ({id})"), "amount": num(deposit) },
                { "description": "Retención 50% por cancelación con menos de 6 hs", "amount": num(retained) },
                { "description": "Saldo reintegrado como crédito en cuenta", "amount": num(deposit - retained) }
            ],
            "totalPaid": num(retained),
            "paymentMethod": "Retención sobre seña",
            "status": "completado", "date": now_stamp(), "cashierName": "Sistema de Reservas"
        });
        state.store.insert("recibos", &penalty)?;
        json!({
            "id": refund_id,
            "clientName": client_name, "clientDni": client_dni,
            "relatedReservationId": id,
            "relatedReceiptId": res.get("depositReceiptNumber").cloned().unwrap_or(Value::Null),
            "reason": format!("Cancelación fuera de plazo ({hours} hs). Se retiene el 50% de la seña y se reintegra el resto."),
            "refundType": "parcial",
            "originalAmount": num(deposit), "refundAmount": num(deposit - retained),
            "refundMethod": "Crédito en cuenta de socio",
            "status": "pendiente", "date": now_stamp(), "processedBy": "Sistema de Reservas"
        })
    };
    state.store.insert("reembolsos", &refund)?;

    Ok(Json(json!({ "data": res, "isWithinFreeWindow": within, "refund": refund, "penaltyReceipt": penalty })))
}

// ===========================================================================
// Torneos y ligas
// ===========================================================================
fn new_participant(body: &Value, prefix: &str) -> Value {
    let name = { let n = s(body, "name"); if n.is_empty() { s(body, "partnerName") } else { n } };
    let name = if name.is_empty() { "Participante Inscripto".to_string() } else { name };
    let dni = { let d = s(body, "dni"); if d.is_empty() { "00.000.000".to_string() } else { d } };
    json!({
        "id": format!("{prefix}-{}", now_millis()),
        "name": name,
        "dni": dni,
        "phone": s(body, "phone"),
        "partnerName": s(body, "partnerName"),
        "partnerDni": s(body, "partnerDni"),
        "enrolledAt": today()
    })
}

fn set_rules(state: &AppState, col: &str, id: &str, body: &Value) -> ApiResult<J> {
    let mut doc = state.store.require(col, id)?;
    set(&mut doc, "internalRules", Value::String(s(body, "internalRules")));
    state.store.upsert(col, &doc)?;
    Ok(Json(doc))
}

pub async fn tournament_rules(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    set_rules(&state, "torneos", &id, &body)
}
pub async fn league_rules(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    set_rules(&state, "ligas", &id, &body)
}

/// POST /api/torneos/{id}/inscripciones
pub async fn tournament_enroll(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<(StatusCode, J)> {
    let mut t = state.store.require("torneos", &id)?;
    let mut list = arr(&t, "participants");
    let max = f(&t, "maxParticipants");
    if max > 0.0 && list.len() as f64 >= max {
        return Err(ApiError::conflict("El torneo alcanzó el cupo máximo de inscriptos."));
    }
    let p = new_participant(&body, "p");
    list.push(p.clone());
    set(&mut t, "currentEnrolled", num(list.len() as f64));
    set(&mut t, "participants", Value::Array(list));
    state.store.upsert("torneos", &t)?;
    Ok((StatusCode::CREATED, Json(json!({ "success": true, "participant": p }))))
}

/// DELETE /api/torneos/{id}/inscripciones/{pid}
pub async fn tournament_unenroll(State(state): State<AppState>, Path((id, pid)): Path<(String, String)>) -> ApiResult<J> {
    let mut t = state.store.require("torneos", &id)?;
    let list: Vec<Value> = arr(&t, "participants").into_iter().filter(|p| s(p, "id") != pid).collect();
    set(&mut t, "currentEnrolled", num(list.len() as f64));
    set(&mut t, "participants", Value::Array(list));
    state.store.upsert("torneos", &t)?;
    Ok(Json(json!({ "success": true })))
}

fn new_match(body: &Value, prefix: &str) -> Value {
    let court = { let c = s(body, "court"); if c.is_empty() { "Cancha 1".to_string() } else { c } };
    let date = { let d = s(body, "date"); if d.is_empty() { today() } else { d } };
    json!({
        "id": format!("{prefix}-{}", now_millis()),
        "p1": s(body, "p1").trim(),
        "p2": s(body, "p2").trim(),
        "score": "Pendiente",
        "winner": Value::Null,
        "status": "programado",
        "court": court,
        "date": date
    })
}

fn add_match_to(doc: &mut Value, rounds_key: &str, round_name: &str, default_round: &str, m: Value) {
    let mut rounds = arr(doc, rounds_key);
    let name = if round_name.is_empty() { format!("{default_round} {}", rounds.len() + 1) } else { round_name.to_string() };
    if let Some(r) = rounds.iter_mut().find(|r| s(r, "round") == name) {
        let mut ms = arr(r, "matches");
        ms.push(m);
        set(r, "matches", Value::Array(ms));
    } else {
        rounds.push(json!({ "round": name, "matches": [m] }));
    }
    set(doc, rounds_key, Value::Array(rounds));
}

fn update_match_in(doc: &mut Value, rounds_key: &str, match_id: &str, patch: &Value) -> ApiResult<Value> {
    let mut found: Option<Value> = None;
    let rounds: Vec<Value> = arr(doc, rounds_key)
        .into_iter()
        .map(|mut r| {
            let ms: Vec<Value> = arr(&r, "matches")
                .into_iter()
                .map(|m| {
                    if s(&m, "id") == match_id {
                        let mut next = merge(&m, patch);
                        set(&mut next, "id", Value::String(match_id.to_string()));
                        found = Some(next.clone());
                        next
                    } else {
                        m
                    }
                })
                .collect();
            set(&mut r, "matches", Value::Array(ms));
            r
        })
        .collect();
    set(doc, rounds_key, Value::Array(rounds));
    found.ok_or_else(|| ApiError::not_found(format!("No existe el enfrentamiento {match_id}.")))
}

fn validate_match_body(body: &Value) -> ApiResult<()> {
    if s(body, "p1").trim().is_empty() || s(body, "p2").trim().is_empty() {
        return Err(ApiError::bad_request("p1 y p2 son obligatorios."));
    }
    if s(body, "p1").trim() == s(body, "p2").trim() {
        return Err(ApiError::bad_request("Los dos jugadores deben ser distintos."));
    }
    Ok(())
}

/// POST /api/torneos/{id}/enfrentamientos  { round, p1, p2, court, date }
pub async fn tournament_add_match(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<(StatusCode, J)> {
    validate_match_body(&body)?;
    let mut t = state.store.require("torneos", &id)?;
    let m = new_match(&body, "tm");
    add_match_to(&mut t, "bracket", &s(&body, "round"), "Ronda", m.clone());
    state.store.upsert("torneos", &t)?;
    Ok((StatusCode::CREATED, Json(json!({ "success": true, "match": m }))))
}

/// PUT /api/torneos/{id}/enfrentamientos/{mid}
pub async fn tournament_update_match(State(state): State<AppState>, Path((id, mid)): Path<(String, String)>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut t = state.store.require("torneos", &id)?;
    let m = update_match_in(&mut t, "bracket", &mid, &body)?;
    state.store.upsert("torneos", &t)?;
    Ok(Json(json!({ "success": true, "match": m })))
}

/// POST /api/ligas/{id}/inscripciones — agrega el participante y su fila en la tabla.
pub async fn league_enroll(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<(StatusCode, J)> {
    let mut l = state.store.require("ligas", &id)?;
    let mut list = arr(&l, "participants");
    let max = f(&l, "maxParticipants");
    if max > 0.0 && list.len() as f64 >= max {
        return Err(ApiError::conflict("La liga alcanzó el cupo máximo de inscriptos."));
    }
    let p = new_participant(&body, "lp");
    list.push(p.clone());
    let mut standings = arr(&l, "standings");
    if !standings.iter().any(|st| s(st, "name") == s(&p, "name")) {
        standings.push(json!({ "rank": standings.len() + 1, "name": s(&p, "name"), "pj": 0, "pg": 0, "pp": 0, "sf": 0, "sc": 0, "dif": 0, "points": 0 }));
    }
    set(&mut l, "currentEnrolled", num(list.len() as f64));
    set(&mut l, "participants", Value::Array(list));
    set(&mut l, "standings", Value::Array(standings));
    state.store.upsert("ligas", &l)?;
    Ok((StatusCode::CREATED, Json(json!({ "success": true, "participant": p }))))
}

/// DELETE /api/ligas/{id}/inscripciones/{pid}
pub async fn league_unenroll(State(state): State<AppState>, Path((id, pid)): Path<(String, String)>) -> ApiResult<J> {
    let mut l = state.store.require("ligas", &id)?;
    let removed_name = arr(&l, "participants").iter().find(|p| s(p, "id") == pid).map(|p| s(p, "name"));
    let list: Vec<Value> = arr(&l, "participants").into_iter().filter(|p| s(p, "id") != pid).collect();
    let standings: Vec<Value> = arr(&l, "standings")
        .into_iter()
        .filter(|st| Some(s(st, "name")) != removed_name)
        .enumerate()
        .map(|(i, mut st)| { set(&mut st, "rank", json!(i + 1)); st })
        .collect();
    set(&mut l, "currentEnrolled", num(list.len() as f64));
    set(&mut l, "participants", Value::Array(list));
    set(&mut l, "standings", Value::Array(standings));
    state.store.upsert("ligas", &l)?;
    Ok(Json(json!({ "success": true })))
}

/// POST /api/ligas/{id}/enfrentamientos  { round, p1, p2, court, date }
pub async fn league_add_match(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<(StatusCode, J)> {
    validate_match_body(&body)?;
    let mut l = state.store.require("ligas", &id)?;
    let m = new_match(&body, "lm");
    add_match_to(&mut l, "fixtures", &s(&body, "round"), "Jornada", m.clone());
    state.store.upsert("ligas", &l)?;
    Ok((StatusCode::CREATED, Json(json!({ "success": true, "match": m }))))
}

/// PUT /api/ligas/{id}/enfrentamientos/{mid}
pub async fn league_update_match(State(state): State<AppState>, Path((id, mid)): Path<(String, String)>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut l = state.store.require("ligas", &id)?;
    let m = update_match_in(&mut l, "fixtures", &mid, &body)?;
    state.store.upsert("ligas", &l)?;
    Ok(Json(json!({ "success": true, "match": m })))
}

/// POST /api/ligas/{id}/enfrentamientos/{mid}/resultado  { score, winner }
/// Marca el partido como finalizado y actualiza la tabla (3 puntos por victoria).
pub async fn league_match_score(State(state): State<AppState>, Path((id, mid)): Path<(String, String)>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut l = state.store.require("ligas", &id)?;
    let winner = s(&body, "winner");
    let score = s(&body, "score");
    if winner.is_empty() {
        return Err(ApiError::bad_request("Debe indicar el ganador."));
    }
    let before = arr(&l, "fixtures")
        .iter()
        .flat_map(|r| arr(r, "matches"))
        .find(|m| s(m, "id") == mid)
        .ok_or_else(|| ApiError::not_found(format!("No existe el enfrentamiento {mid}.")))?;
    if ![s(&before, "p1"), s(&before, "p2")].contains(&winner) {
        return Err(ApiError::bad_request("El ganador debe ser uno de los dos jugadores del partido."));
    }
    let already_finished = s(&before, "status") == "finalizado";
    let m = update_match_in(&mut l, "fixtures", &mid, &json!({ "score": score, "winner": winner, "status": "finalizado" }))?;

    if !already_finished {
        let loser = if winner == s(&m, "p1") { s(&m, "p2") } else { s(&m, "p1") };
        let mut standings: Vec<Value> = arr(&l, "standings")
            .into_iter()
            .map(|mut st| {
                let name = s(&st, "name");
                if name == winner {
                    { let v__ = num(f(&st, "pj") + 1.0); set(&mut st, "pj", v__); }
                    { let v__ = num(f(&st, "pg") + 1.0); set(&mut st, "pg", v__); }
                    { let v__ = num(f(&st, "points") + 3.0); set(&mut st, "points", v__); }
                    { let v__ = num(f(&st, "sf") + 2.0); set(&mut st, "sf", v__); }
                    { let v__ = num(f(&st, "dif") + 2.0); set(&mut st, "dif", v__); }
                } else if name == loser {
                    { let v__ = num(f(&st, "pj") + 1.0); set(&mut st, "pj", v__); }
                    { let v__ = num(f(&st, "pp") + 1.0); set(&mut st, "pp", v__); }
                    { let v__ = num(f(&st, "sc") + 2.0); set(&mut st, "sc", v__); }
                    { let v__ = num(f(&st, "dif") - 2.0); set(&mut st, "dif", v__); }
                }
                st
            })
            .collect();
        standings.sort_by(|a, b| {
            (f(b, "points"), f(b, "dif"))
                .partial_cmp(&(f(a, "points"), f(a, "dif")))
                .unwrap_or(std::cmp::Ordering::Equal)
        });
        let standings: Vec<Value> = standings.into_iter().enumerate().map(|(i, mut st)| { set(&mut st, "rank", json!(i + 1)); st }).collect();
        set(&mut l, "standings", Value::Array(standings));
    }
    state.store.upsert("ligas", &l)?;
    Ok(Json(json!({ "success": true, "match": m, "standings": l.get("standings").cloned().unwrap_or(json!([])) })))
}

// ===========================================================================
// Profesores / entrenadores
// ===========================================================================
/// PUT /api/profesores/{id}/disponibilidad  { availableHours }
pub async fn coach_availability(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut c = state.store.require("profesores", &id)?;
    set(&mut c, "availableHours", Value::String(s(&body, "availableHours")));
    state.store.upsert("profesores", &c)?;
    Ok(Json(c))
}

/// PUT /api/profesores/{id}/certificacion  { titleName, institution, licenseNumber, issueDate, expiryDate, status, digitalDocumentUrl }
pub async fn coach_license(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    body_obj(&body)?;
    let mut c = state.store.require("profesores", &id)?;
    let current = c.get("licenseDetails").cloned().unwrap_or(json!({}));
    set(&mut c, "licenseDetails", merge(&current, &body));
    state.store.upsert("profesores", &c)?;
    Ok(Json(c))
}

// ===========================================================================
// Clases y entrenamientos
// ===========================================================================
/// POST /api/clases/{id}/alumnos  { name, dni, phone }  — RD07/RD10: respeta el cupo de la comisión.
pub async fn class_enroll(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<(StatusCode, J)> {
    let mut cls = state.store.require("clases", &id)?;
    if s(&body, "name").trim().is_empty() {
        return Err(ApiError::bad_request("El nombre del alumno es obligatorio."));
    }
    let mut students = arr(&cls, "students");
    let cap = { let c = f(&cls, "maxCapacity"); if c <= 0.0 { 30.0 } else { c.min(30.0) } } as usize;
    if students.len() >= cap {
        return Err(ApiError::conflict(format!("No es posible agregar más alumnos: el cupo de {cap} está completo.")));
    }
    let sessions = arr(&cls, "sessions").len().max(1);
    let st = json!({
        "id": format!("std-{}", now_millis()),
        "name": s(&body, "name"),
        "dni": s(&body, "dni"),
        "phone": s(&body, "phone"),
        "enrolledAt": today(),
        "attendance": vec!["P"; sessions]
    });
    students.push(st.clone());
    set(&mut cls, "currentEnrolled", num(students.len() as f64));
    set(&mut cls, "students", Value::Array(students));
    state.store.upsert("clases", &cls)?;
    Ok((StatusCode::CREATED, Json(json!({ "success": true, "student": st }))))
}

/// DELETE /api/clases/{id}/alumnos/{sid}
pub async fn class_unenroll(State(state): State<AppState>, Path((id, sid)): Path<(String, String)>) -> ApiResult<J> {
    let mut cls = state.store.require("clases", &id)?;
    let students: Vec<Value> = arr(&cls, "students").into_iter().filter(|st| s(st, "id") != sid).collect();
    set(&mut cls, "currentEnrolled", num(students.len() as f64));
    set(&mut cls, "students", Value::Array(students));
    state.store.upsert("clases", &cls)?;
    Ok(Json(json!({ "success": true })))
}

fn attendance_response(state: &AppState) -> ApiResult<J> {
    Ok(Json(json!({ "success": true, "classes": state.store.list("clases")? })))
}

fn pad_attendance(att: &mut Vec<Value>, idx: usize) {
    while att.len() <= idx {
        att.push(Value::String("-".into()));
    }
}

/// POST /api/clases/{id}/asistencia/alternar  { studentId, sessionIndex }  — P → A → J → P
pub async fn class_toggle_attendance(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut cls = state.store.require("clases", &id)?;
    let sid = s(&body, "studentId");
    let idx = f(&body, "sessionIndex").max(0.0) as usize;
    let students: Vec<Value> = arr(&cls, "students")
        .into_iter()
        .map(|mut st| {
            if s(&st, "id") == sid {
                let mut att = arr(&st, "attendance");
                pad_attendance(&mut att, idx);
                let cur = att[idx].as_str().unwrap_or("-");
                let next = match cur { "P" => "A", "A" => "J", "J" => "P", _ => "P" };
                att[idx] = Value::String(next.into());
                set(&mut st, "attendance", Value::Array(att));
            }
            st
        })
        .collect();
    set(&mut cls, "students", Value::Array(students));
    state.store.upsert("clases", &cls)?;
    attendance_response(&state)
}

/// POST /api/clases/{id}/asistencia/sesion  { sessionIndex, status }
pub async fn class_bulk_attendance(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut cls = state.store.require("clases", &id)?;
    let idx = f(&body, "sessionIndex").max(0.0) as usize;
    let status = { let st = s(&body, "status"); if st.is_empty() { "P".to_string() } else { st } };
    let students: Vec<Value> = arr(&cls, "students")
        .into_iter()
        .map(|mut st| {
            let mut att = arr(&st, "attendance");
            pad_attendance(&mut att, idx);
            att[idx] = Value::String(status.clone());
            set(&mut st, "attendance", Value::Array(att));
            st
        })
        .collect();
    set(&mut cls, "students", Value::Array(students));
    state.store.upsert("clases", &cls)?;
    attendance_response(&state)
}

/// POST /api/clases/{id}/asistencia/registrar  { studentId, status } — agrega una sesión al alumno.
pub async fn class_record_attendance(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut cls = state.store.require("clases", &id)?;
    let sid = s(&body, "studentId");
    let status = s(&body, "status");
    let students: Vec<Value> = arr(&cls, "students")
        .into_iter()
        .map(|mut st| {
            if s(&st, "id") == sid {
                let mut att = arr(&st, "attendance");
                att.push(Value::String(status.clone()));
                set(&mut st, "attendance", Value::Array(att));
            }
            st
        })
        .collect();
    set(&mut cls, "students", Value::Array(students));
    state.store.upsert("clases", &cls)?;
    Ok(Json(json!({ "success": true })))
}

/// POST /api/clases-particulares/{id}/cancelar
pub async fn private_cancel(State(state): State<AppState>, Path(id): Path<String>) -> ApiResult<J> {
    let mut p = state.store.require("clases-particulares", &id)?;
    set(&mut p, "status", Value::String("cancelada".into()));
    state.store.upsert("clases-particulares", &p)?;
    Ok(Json(json!({ "success": true, "data": p })))
}

// ===========================================================================
// Caja: facturas, reembolsos, descuentos por usuario
// ===========================================================================
pub fn compute_invoice(mut d: Value) -> ApiResult<Value> {
    let items: Vec<Value> = arr(&d, "items")
        .into_iter()
        .map(|it| json!({
            "description": s(&it, "description"),
            "quantity": num({ let q = f(&it, "quantity"); if q <= 0.0 { 1.0 } else { q } }),
            "unitPrice": num(f(&it, "unitPrice"))
        }))
        .collect();
    if items.is_empty() || items.iter().any(|it| s(it, "description").trim().is_empty()) {
        return Err(ApiError::bad_request("La factura debe tener al menos un ítem con descripción."));
    }
    let subtotal: f64 = items.iter().map(|it| f(it, "quantity") * f(it, "unitPrice")).sum();
    if subtotal <= 0.0 {
        return Err(ApiError::bad_request("El importe de la factura debe ser mayor a cero."));
    }
    let tax_rate = if d.get("taxRate").is_some() { f(&d, "taxRate") } else { 21.0 };
    let tax = (subtotal * tax_rate / 100.0).round();
    if s(&d, "type").is_empty() { set(&mut d, "type", Value::String("B".into())); }
    if s(&d, "concept").is_empty() { set(&mut d, "concept", Value::String(s(&items[0], "description"))); }
    if s(&d, "paymentMethod").is_empty() { set(&mut d, "paymentMethod", Value::String("Efectivo".into())); }
    if s(&d, "issuedBy").is_empty() { set(&mut d, "issuedBy", Value::String("Administración General".into())); }
    if d.get("relatedReceiptId").map(|v| v.as_str().unwrap_or("").is_empty()).unwrap_or(true) {
        set(&mut d, "relatedReceiptId", Value::Null);
    }
    set(&mut d, "items", Value::Array(items));
    set(&mut d, "subtotal", num(subtotal));
    set(&mut d, "taxRate", num(tax_rate));
    set(&mut d, "taxAmount", num(tax));
    set(&mut d, "total", num(subtotal + tax));
    set(&mut d, "status", Value::String("emitida".into()));
    set(&mut d, "date", Value::String(now_stamp()));
    Ok(d)
}

pub fn compute_refund(mut d: Value) -> ApiResult<Value> {
    let original = f(&d, "originalAmount");
    let amount = f(&d, "refundAmount").min(original);
    if amount <= 0.0 {
        return Err(ApiError::bad_request("El importe a reembolsar debe ser mayor a cero."));
    }
    set(&mut d, "originalAmount", num(original));
    set(&mut d, "refundAmount", num(amount));
    set(&mut d, "refundType", Value::String(if amount >= original { "total" } else { "parcial" }.into()));
    if s(&d, "refundMethod").is_empty() { set(&mut d, "refundMethod", Value::String("Crédito en cuenta de socio".into())); }
    if s(&d, "status").is_empty() { set(&mut d, "status", Value::String("pendiente".into())); }
    if s(&d, "processedBy").is_empty() { set(&mut d, "processedBy", Value::String("Administración General".into())); }
    for k in ["relatedReservationId", "relatedReceiptId"] {
        if s(&d, k).is_empty() { set(&mut d, k, Value::Null); }
    }
    set(&mut d, "date", Value::String(now_stamp()));
    Ok(d)
}

/// POST /api/facturas/{id}/anular
pub async fn invoice_annul(State(state): State<AppState>, Path(id): Path<String>) -> ApiResult<J> {
    let mut inv = state.store.require("facturas", &id)?;
    set(&mut inv, "status", Value::String("anulada".into()));
    state.store.upsert("facturas", &inv)?;
    Ok(Json(inv))
}

/// PATCH /api/reembolsos/{id}/estado  { status }
pub async fn refund_status(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let status = s(&body, "status");
    if !["pendiente", "procesado", "rechazado"].contains(&status.as_str()) {
        return Err(ApiError::bad_request("status debe ser pendiente, procesado o rechazado."));
    }
    let mut r = state.store.require("reembolsos", &id)?;
    set(&mut r, "status", Value::String(status));
    state.store.upsert("reembolsos", &r)?;
    Ok(Json(r))
}

/// PUT /api/usuarios/{id}/descuentos  { activeDiscounts: ["PAQ10", ...] }
pub async fn user_discounts(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<Value>) -> ApiResult<J> {
    let mut u = state.store.require("usuarios", &id)?;
    let known: Vec<String> = state.store.list("descuentos")?.iter().map(|d| s(d, "code")).collect();
    let codes: Vec<Value> = arr(&body, "activeDiscounts");
    if let Some(bad) = codes.iter().find(|c| !known.contains(&c.as_str().unwrap_or("").to_string())) {
        return Err(ApiError::bad_request(format!("El descuento {bad} no existe en el catálogo.")));
    }
    set(&mut u, "activeDiscounts", Value::Array(codes));
    state.store.upsert("usuarios", &u)?;
    Ok(Json(sanitize("usuarios", u)))
}

// ===========================================================================
// Administración de la API de prueba
// ===========================================================================
/// POST /api/admin/reset — vacía la base y vuelve a cargar el seed.
pub async fn reset(State(state): State<AppState>) -> ApiResult<J> {
    let seed_data = seed::load_seed_file(&state.seed_path)?;
    state.store.clear()?;
    let n = seed::apply_seed(&state.store, &seed_data)?;
    Ok(Json(json!({ "success": true, "documents": n })))
}
