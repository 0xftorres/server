//! Persistencia en SQLite (rusqlite enlazado contra `lib/sqlite3.dll`).
//!
//! Modelo deliberadamente simple para una API de prueba: una tabla `documents` con un JSON por
//! entidad, igual al que maneja el front. La versión definitiva debería normalizar las tablas
//! (ver INFORME-API.pdf, sección "Recomendaciones").

use std::{
    path::Path,
    sync::{Arc, Mutex, MutexGuard},
};

use rusqlite::{params, Connection, OptionalExtension};
use serde_json::Value;

use crate::error::{ApiError, ApiResult};

#[derive(Clone)]
pub struct Store {
    conn: Arc<Mutex<Connection>>,
}

const SCHEMA: &str = r#"
PRAGMA journal_mode = WAL;
CREATE TABLE IF NOT EXISTS documents (
    collection  TEXT NOT NULL,
    id          TEXT NOT NULL,
    data        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (collection, id)
);
CREATE INDEX IF NOT EXISTS idx_documents_collection ON documents (collection);
"#;

impl Store {
    pub fn open(path: &Path) -> ApiResult<Self> {
        if let Some(parent) = path.parent() {
            if !parent.as_os_str().is_empty() {
                std::fs::create_dir_all(parent)
                    .map_err(|e| ApiError::internal(format!("No se pudo crear {}: {e}", parent.display())))?;
            }
        }
        let conn = Connection::open(path)?;
        conn.execute_batch(SCHEMA)?;
        Ok(Self { conn: Arc::new(Mutex::new(conn)) })
    }

    fn lock(&self) -> MutexGuard<'_, Connection> {
        self.conn.lock().unwrap_or_else(|poisoned| poisoned.into_inner())
    }

    pub fn sqlite_version(&self) -> String {
        rusqlite::version().to_string()
    }

    pub fn is_empty(&self) -> ApiResult<bool> {
        let n: i64 = self.lock().query_row("SELECT COUNT(*) FROM documents", [], |r| r.get(0))?;
        Ok(n == 0)
    }

    pub fn count(&self, collection: &str) -> ApiResult<i64> {
        let n: i64 = self.lock().query_row(
            "SELECT COUNT(*) FROM documents WHERE collection = ?1",
            params![collection],
            |r| r.get(0),
        )?;
        Ok(n)
    }

    /// Conteo por colección (para /health y el reporte).
    pub fn counts(&self) -> ApiResult<Vec<(String, i64)>> {
        let conn = self.lock();
        let mut stmt = conn.prepare("SELECT collection, COUNT(*) FROM documents GROUP BY collection ORDER BY collection")?;
        let rows = stmt.query_map([], |r| Ok((r.get::<_, String>(0)?, r.get::<_, i64>(1)?)))?;
        Ok(rows.filter_map(Result::ok).collect())
    }

    pub fn list(&self, collection: &str) -> ApiResult<Vec<Value>> {
        let conn = self.lock();
        let mut stmt = conn.prepare("SELECT data FROM documents WHERE collection = ?1 ORDER BY rowid")?;
        let rows = stmt.query_map(params![collection], |r| r.get::<_, String>(0))?;
        let mut out = Vec::new();
        for raw in rows {
            out.push(serde_json::from_str(&raw?)?);
        }
        Ok(out)
    }

    pub fn get(&self, collection: &str, id: &str) -> ApiResult<Option<Value>> {
        let raw: Option<String> = self
            .lock()
            .query_row(
                "SELECT data FROM documents WHERE collection = ?1 AND id = ?2",
                params![collection, id],
                |r| r.get(0),
            )
            .optional()?;
        Ok(match raw {
            Some(r) => Some(serde_json::from_str(&r)?),
            None => None,
        })
    }

    /// Igual que `get` pero devuelve 404 si no existe.
    pub fn require(&self, collection: &str, id: &str) -> ApiResult<Value> {
        self.get(collection, id)?
            .ok_or_else(|| ApiError::not_found(format!("No existe {collection}/{id}.")))
    }

    pub fn exists(&self, collection: &str, id: &str) -> ApiResult<bool> {
        Ok(self.get(collection, id)?.is_some())
    }

    fn id_of(doc: &Value) -> ApiResult<String> {
        doc.get("id")
            .and_then(|v| v.as_str())
            .filter(|s| !s.is_empty())
            .map(str::to_string)
            .ok_or_else(|| ApiError::bad_request("El documento debe tener un campo `id`."))
    }

    pub fn insert(&self, collection: &str, doc: &Value) -> ApiResult<()> {
        let id = Self::id_of(doc)?;
        let data = serde_json::to_string(doc)?;
        let res = self.lock().execute(
            "INSERT INTO documents (collection, id, data) VALUES (?1, ?2, ?3)",
            params![collection, id, data],
        );
        match res {
            Ok(_) => Ok(()),
            Err(rusqlite::Error::SqliteFailure(e, _)) if e.code == rusqlite::ErrorCode::ConstraintViolation => {
                Err(ApiError::conflict(format!("Ya existe {collection}/{id}.")))
            }
            Err(e) => Err(e.into()),
        }
    }

    pub fn upsert(&self, collection: &str, doc: &Value) -> ApiResult<()> {
        let id = Self::id_of(doc)?;
        let data = serde_json::to_string(doc)?;
        self.lock().execute(
            "INSERT INTO documents (collection, id, data) VALUES (?1, ?2, ?3)
             ON CONFLICT (collection, id) DO UPDATE SET
                data = excluded.data,
                updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')",
            params![collection, id, data],
        )?;
        Ok(())
    }

    pub fn delete(&self, collection: &str, id: &str) -> ApiResult<bool> {
        let n = self.lock().execute(
            "DELETE FROM documents WHERE collection = ?1 AND id = ?2",
            params![collection, id],
        )?;
        Ok(n > 0)
    }

    pub fn clear(&self) -> ApiResult<()> {
        self.lock().execute("DELETE FROM documents", [])?;
        Ok(())
    }
}
