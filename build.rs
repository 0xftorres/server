//! Deja `sqlite3.dll` junto al ejecutable (target/<perfil>/) para que el binario la encuentre
//! en tiempo de ejecución. El enlace se resuelve con `SQLITE3_LIB_DIR` (ver .cargo/config.toml).

use std::{env, fs, path::PathBuf};

fn main() {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let lib_dir = manifest.join("lib");
    println!("cargo:rerun-if-changed=lib/sqlite3.dll");
    println!("cargo:rustc-link-search=native={}", lib_dir.display());

    let out_dir = PathBuf::from(env::var("OUT_DIR").unwrap());
    // OUT_DIR = target/<perfil>/build/<crate>-<hash>/out  ->  target/<perfil>
    if let Some(profile_dir) = out_dir.ancestors().nth(3) {
        let src = lib_dir.join("sqlite3.dll");
        let dst = profile_dir.join("sqlite3.dll");
        if src.exists() && !dst.exists() {
            let _ = fs::copy(&src, &dst);
        }
    }
}
