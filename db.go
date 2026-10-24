package main

// Persistencia en SQLite (modernc.org/sqlite: Go puro, sin cgo ni compilador C).
//
// Modelo deliberadamente simple para una API de prueba: una tabla `documents` con un JSON por
// entidad, igual al que maneja el front. La versión definitiva debería normalizar las tablas
// (ver INFORME-API.pdf, sección "Recomendaciones").

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	db            *sql.DB
	sqliteVersion string
}

const schema = `
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
`

func openStore(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, internalError(fmt.Sprintf("No se pudo crear %s: %v", dir, err))
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, dbError(err)
	}
	// Una sola conexión: serializa los accesos y evita SQLITE_BUSY entre conexiones del pool.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, dbError(err)
	}
	st := &Store{db: db}
	if err := db.QueryRow("SELECT sqlite_version()").Scan(&st.sqliteVersion); err != nil {
		return nil, dbError(err)
	}
	return st, nil
}

func (st *Store) isEmpty() (bool, error) {
	var n int64
	if err := st.db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&n); err != nil {
		return false, dbError(err)
	}
	return n == 0, nil
}

func (st *Store) count(collection string) (int64, error) {
	var n int64
	err := st.db.QueryRow("SELECT COUNT(*) FROM documents WHERE collection = ?", collection).Scan(&n)
	if err != nil {
		return 0, dbError(err)
	}
	return n, nil
}

// counts devuelve el conteo por colección (para /health).
func (st *Store) counts() (map[string]int64, error) {
	rows, err := st.db.Query("SELECT collection, COUNT(*) FROM documents GROUP BY collection ORDER BY collection")
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var col string
		var n int64
		if err := rows.Scan(&col, &n); err != nil {
			return nil, dbError(err)
		}
		out[col] = n
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(err)
	}
	return out, nil
}

func decodeDoc(raw string) (M, error) {
	var doc M
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, serializationError(err)
	}
	return doc, nil
}

func (st *Store) list(collection string) ([]M, error) {
	rows, err := st.db.Query("SELECT data FROM documents WHERE collection = ? ORDER BY rowid", collection)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	out := []M{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, dbError(err)
		}
		doc, err := decodeDoc(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(err)
	}
	return out, nil
}

func (st *Store) get(collection, id string) (M, bool, error) {
	var raw string
	err := st.db.QueryRow("SELECT data FROM documents WHERE collection = ? AND id = ?", collection, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, dbError(err)
	}
	doc, err := decodeDoc(raw)
	if err != nil {
		return nil, false, err
	}
	return doc, true, nil
}

// require es igual que get pero devuelve 404 si no existe.
func (st *Store) require(collection, id string) (M, error) {
	doc, ok, err := st.get(collection, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notFound(fmt.Sprintf("No existe %s/%s.", collection, id))
	}
	return doc, nil
}

func (st *Store) exists(collection, id string) (bool, error) {
	_, ok, err := st.get(collection, id)
	return ok, err
}

func idOf(doc M) (string, error) {
	id := s(doc, "id")
	if id == "" {
		return "", badRequest("El documento debe tener un campo `id`.")
	}
	return id, nil
}

func (st *Store) insert(collection string, doc M) error {
	id, err := idOf(doc)
	if err != nil {
		return err
	}
	data, err := marshalJSON(doc)
	if err != nil {
		return serializationError(err)
	}
	// DO NOTHING + RowsAffected detecta el duplicado sin depender de los códigos de error del driver.
	res, err := st.db.Exec(
		"INSERT INTO documents (collection, id, data) VALUES (?, ?, ?) ON CONFLICT (collection, id) DO NOTHING",
		collection, id, string(data),
	)
	if err != nil {
		return dbError(err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return dbError(err)
	} else if n == 0 {
		return conflict(fmt.Sprintf("Ya existe %s/%s.", collection, id))
	}
	return nil
}

func (st *Store) upsert(collection string, doc M) error {
	id, err := idOf(doc)
	if err != nil {
		return err
	}
	data, err := marshalJSON(doc)
	if err != nil {
		return serializationError(err)
	}
	_, err = st.db.Exec(
		`INSERT INTO documents (collection, id, data) VALUES (?, ?, ?)
		 ON CONFLICT (collection, id) DO UPDATE SET
		    data = excluded.data,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		collection, id, string(data),
	)
	if err != nil {
		return dbError(err)
	}
	return nil
}

func (st *Store) delete(collection, id string) (bool, error) {
	res, err := st.db.Exec("DELETE FROM documents WHERE collection = ? AND id = ?", collection, id)
	if err != nil {
		return false, dbError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, dbError(err)
	}
	return n > 0, nil
}

func (st *Store) clear() error {
	if _, err := st.db.Exec("DELETE FROM documents"); err != nil {
		return dbError(err)
	}
	return nil
}
