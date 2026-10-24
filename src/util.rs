//! Helpers para trabajar con `serde_json::Value` (los documentos se guardan como JSON libre,
//! con la misma forma que usa el front en `src/utils/mockData.js`).

use chrono::{Local, Utc};
use serde_json::{Map, Value};

/// Lee una clave como String (vacío si no existe o no es string).
pub fn s(v: &Value, key: &str) -> String {
    v.get(key).and_then(|x| x.as_str()).unwrap_or("").to_string()
}

/// Lee una clave numérica como f64 (0 si no existe). Acepta números y strings numéricos.
pub fn f(v: &Value, key: &str) -> f64 {
    match v.get(key) {
        Some(Value::Number(n)) => n.as_f64().unwrap_or(0.0),
        Some(Value::String(t)) => t.trim().parse::<f64>().unwrap_or(0.0),
        _ => 0.0,
    }
}

pub fn b(v: &Value, key: &str, default: bool) -> bool {
    v.get(key).and_then(|x| x.as_bool()).unwrap_or(default)
}

pub fn obj(v: &Value) -> Map<String, Value> {
    v.as_object().cloned().unwrap_or_default()
}

pub fn arr(v: &Value, key: &str) -> Vec<Value> {
    v.get(key).and_then(|x| x.as_array()).cloned().unwrap_or_default()
}

pub fn set(v: &mut Value, key: &str, value: Value) {
    if let Some(m) = v.as_object_mut() {
        m.insert(key.to_string(), value);
    }
}

/// Número con parte entera si es entero (para que `9600.0` se serialice como `9600`).
pub fn num(x: f64) -> Value {
    if x.fract() == 0.0 && x.abs() < 9.0e15 {
        Value::from(x as i64)
    } else {
        Value::from(x)
    }
}

/// Mezcla superficial: las claves de `patch` pisan las de `base` (igual que `{ ...base, ...patch }`).
pub fn merge(base: &Value, patch: &Value) -> Value {
    let mut out = obj(base);
    for (k, v) in obj(patch) {
        out.insert(k, v);
    }
    Value::Object(out)
}

pub fn now_millis() -> i64 {
    Utc::now().timestamp_millis()
}

/// `2026-10-23 16:46` (misma forma que el mock: ISO sin segundos, separado por espacio).
pub fn now_stamp() -> String {
    Local::now().format("%Y-%m-%d %H:%M").to_string()
}

/// `2026-10-23`
pub fn today() -> String {
    Local::now().format("%Y-%m-%d").to_string()
}

pub fn now_iso() -> String {
    Utc::now().to_rfc3339_opts(chrono::SecondsFormat::Millis, true)
}

pub fn year() -> i32 {
    Local::now().format("%Y").to_string().parse().unwrap_or(2026)
}

/// Pseudo-aleatorio simple (sin dependencia extra) para números de recibo tipo `REC-50-8321`.
pub fn rand4() -> i64 {
    let t = now_millis() as u64;
    let mut x = t ^ 0x9E37_79B9_7F4A_7C15;
    x ^= x >> 33;
    x = x.wrapping_mul(0xff51_afd7_ed55_8ccd);
    x ^= x >> 33;
    1000 + (x % 9000) as i64
}
