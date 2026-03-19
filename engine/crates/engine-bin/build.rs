use std::io::Result;

fn main() -> Result<()> {
    let protoc = protoc_bin_vendored::protoc_bin_path().expect("vendored protoc not found");
    // SAFETY: build scripts are single-threaded
    unsafe { std::env::set_var("PROTOC", protoc) };
    prost_build::compile_protos(&["../../proto/engine.proto"], &["../../proto/"])?;
    Ok(())
}
