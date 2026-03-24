mod pb {
    include!(concat!(env!("OUT_DIR"), "/engine.rs"));
}
mod antipatterns;
mod grep;
mod import_graph;
mod metrics;
mod typescript_imports;

use pb::{
    EngineRequest, EngineResponse, PingResult,
    engine_request::Command,
    engine_response::Payload,
};
use prost::Message;
use std::io::{self, Read, Write};

const VERSION: &str = env!("CARGO_PKG_VERSION");

fn main() {
    let request = match read_message() {
        Ok(r) => r,
        Err(e) => {
            let resp = error_response(format!("failed to read request: {e}"));
            write_message(&resp).expect("failed to write error response");
            std::process::exit(1);
        }
    };

    let responses = handle_request(request);
    for resp in &responses {
        write_message(resp).expect("failed to write response");
    }
}

fn handle_request(request: EngineRequest) -> Vec<EngineResponse> {
    match request.command {
        Some(Command::Ping(_)) => vec![EngineResponse {
            payload: Some(Payload::PingResult(PingResult {
                version: VERSION.to_string(),
                capabilities: vec!["ping".to_string(), "grep".to_string(), "import_graph".to_string(), "anti_pattern".to_string(), "metric".to_string()],
            })),
        }],
        Some(Command::Check(req)) => handle_check(req),
        None => vec![error_response("empty request: no command specified".to_string())],
    }
}

/// Route a CheckRequest to the appropriate engine handlers and merge responses.
/// Grep rules and import_graph rules run independently; each returns its own
/// CheckComplete. The Go client merges them.
fn handle_check(req: pb::CheckRequest) -> Vec<EngineResponse> {
    let mut responses = Vec::new();
    responses.extend(grep::handle_check(req.clone()));
    responses.extend(import_graph::handle_import_graph_check(&req));
    responses.extend(antipatterns::handle_anti_pattern_check(&req));
    responses.extend(metrics::handle_metric_check(&req));
    responses
}

fn error_response(message: String) -> EngineResponse {
    EngineResponse {
        payload: Some(Payload::Error(pb::EngineError {
            message,
            code: "INVALID_REQUEST".to_string(),
        })),
    }
}

/// Maximum message size: 64 MiB. Rejects oversized payloads before allocating.
const MAX_MSG_SIZE: usize = 64 * 1024 * 1024;

/// Read a length-prefixed protobuf message from stdin.
/// Wire format: 4-byte little-endian u32 length, then N bytes of protobuf.
fn read_message() -> io::Result<EngineRequest> {
    let mut stdin = io::stdin().lock();

    let mut len_buf = [0u8; 4];
    stdin.read_exact(&mut len_buf)?;
    let len = u32::from_le_bytes(len_buf) as usize;

    if len > MAX_MSG_SIZE {
        return Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!("message too large: {len} bytes (max {MAX_MSG_SIZE})"),
        ));
    }

    let mut msg_buf = vec![0u8; len];
    stdin.read_exact(&mut msg_buf)?;

    EngineRequest::decode(&msg_buf[..])
        .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))
}

/// Write a length-prefixed protobuf message to stdout.
fn write_message(response: &EngineResponse) -> io::Result<()> {
    let mut stdout = io::stdout().lock();

    let encoded = response.encode_to_vec();
    let len = (encoded.len() as u32).to_le_bytes();

    stdout.write_all(&len)?;
    stdout.write_all(&encoded)?;
    stdout.flush()?;

    Ok(())
}
