//! Carga inicial: si la base está vacía, inserta los datos de `seed/seed.json`
//! (exportado desde `src/utils/mockData.js` del front) y les asigna la contraseña por defecto
//! a todos los usuarios.

use std::path::Path;

use serde_json::{Map, Value};

use crate::{
    auth::{hash_password, DEFAULT_PASSWORD},
    db::Store,
    error::{ApiError, ApiResult},
    util::{s, set},
};

/// Colecciones conocidas (también se usan para validar las rutas genéricas).
pub const COLLECTIONS: &[&str] = &[
    "usuarios",
    "canchas",
    "tipos-cancha",
    "stock",
    "reservas",
    "torneos",
    "ligas",
    "tipos-competencia",
    "profesores",
    "clases",
    "clases-particulares",
    "recibos",
    "facturas",
    "reembolsos",
    "descuentos",
];

pub fn load_seed_file(path: &Path) -> ApiResult<Map<String, Value>> {
    let raw = std::fs::read_to_string(path)
        .map_err(|e| ApiError::internal(format!("No se pudo leer el seed {}: {e}", path.display())))?;
    let value: Value = serde_json::from_str(&raw)?;
    value
        .as_object()
        .cloned()
        .ok_or_else(|| ApiError::internal("El seed debe ser un objeto { coleccion: [...] }"))
}

/// Inserta el seed completo. Devuelve la cantidad de documentos cargados.
pub fn apply_seed(store: &Store, seed: &Map<String, Value>) -> ApiResult<usize> {
    let password_hash = hash_password(DEFAULT_PASSWORD);
    let mut total = 0;
    for col in COLLECTIONS {
        let Some(items) = seed.get(*col).and_then(|v| v.as_array()) else { continue };
        for item in items {
            let mut doc = item.clone();
            if *col == "usuarios" && s(&doc, "passwordHash").is_empty() {
                set(&mut doc, "passwordHash", Value::String(password_hash.clone()));
            }
            if s(&doc, "id").is_empty() {
                continue;
            }
            store.upsert(col, &doc)?;
            total += 1;
        }
    }
    Ok(total)
}

pub fn seed_if_empty(store: &Store, path: &Path) -> ApiResult<usize> {
    if !store.is_empty()? {
        return Ok(0);
    }
    let seed = load_seed_file(path)?;
    apply_seed(store, &seed)
}
