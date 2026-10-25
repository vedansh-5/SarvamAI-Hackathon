//! Client for the VANS runtime, which runs as a child process.
//!
//! The runtime is spawned with pipes and is not reachable any other way. That
//! is deliberate: `commit` performs irreversible side effects, and a localhost
//! port would let any process on the machine trigger one. See D-036.

use std::collections::HashMap;
use std::io::{BufRead, BufReader, Write};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::{Arc, Mutex};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tauri::{AppHandle, Emitter};
use tokio::sync::oneshot;

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct RpcError {
    pub code: String,
    pub message: String,
    #[serde(default)]
    pub data: Option<Value>,
}

impl std::fmt::Display for RpcError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}: {}", self.code, self.message)
    }
}

#[derive(Debug, Serialize)]
struct Request {
    id: i64,
    method: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    params: Option<Value>,
}

#[derive(Debug, Deserialize)]
struct Response {
    #[serde(default)]
    id: i64,
    #[serde(default)]
    result: Option<Value>,
    #[serde(default)]
    error: Option<RpcError>,
}

/// A progress line, forwarded to the window so a ten-second cold run does not
/// look like a hang.
#[derive(Debug, Clone, Serialize)]
pub struct Progress {
    pub for_request: i64,
    pub message: String,
}

type Pending = Arc<Mutex<HashMap<i64, oneshot::Sender<Result<Value, RpcError>>>>>;

pub struct Engine {
    stdin: Mutex<ChildStdin>,
    pending: Pending,
    next_id: AtomicI64,
    _child: Mutex<Child>,
}

impl Engine {
    /// Spawns the runtime and starts reading its output.
    pub fn spawn(app: AppHandle, exe: &std::path::Path) -> std::io::Result<Self> {
        let mut child = Command::new(exe)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            // stderr is inherited on purpose: the runtime keeps its
            // diagnostics off stdout so the protocol stays parseable, and
            // letting them reach the terminal is how you debug a dev run.
            .stderr(Stdio::inherit())
            .spawn()?;

        let stdin = child.stdin.take().expect("stdin was piped");
        let stdout = child.stdout.take().expect("stdout was piped");

        let pending: Pending = Arc::new(Mutex::new(HashMap::new()));
        let reader_pending = Arc::clone(&pending);

        std::thread::spawn(move || {
            let reader = BufReader::new(stdout);
            for line in reader.lines() {
                let Ok(line) = line else { break };
                if line.trim().is_empty() {
                    continue;
                }
                let Ok(value) = serde_json::from_str::<Value>(&line) else {
                    continue;
                };

                // Progress arrives unsolicited and carries the request it
                // belongs to, so a window running several calls can route it.
                if let Some(method) = value.get("method").and_then(Value::as_str) {
                    if method == "log" {
                        let message = value
                            .get("params")
                            .and_then(|p| p.get("message"))
                            .and_then(Value::as_str)
                            .unwrap_or_default()
                            .to_string();
                        let for_request = value.get("for").and_then(Value::as_i64).unwrap_or(0);
                        let _ = app.emit("engine://progress", Progress { for_request, message });
                    } else {
                        // Anything else the runtime pushes reaches the window
                        // under its own name, so a new one needs no change here.
                        let _ = app.emit(&format!("engine://{method}"), value.clone());
                    }
                    continue;
                }

                let Ok(resp) = serde_json::from_value::<Response>(value) else {
                    continue;
                };
                let waiter = reader_pending.lock().unwrap().remove(&resp.id);
                if let Some(tx) = waiter {
                    let outcome = match resp.error {
                        Some(e) => Err(e),
                        None => Ok(resp.result.unwrap_or(Value::Null)),
                    };
                    let _ = tx.send(outcome);
                }
            }

            // The runtime died. Anything still waiting would otherwise hang
            // forever, so fail them all with something the window can show.
            let mut waiting = reader_pending.lock().unwrap();
            for (_, tx) in waiting.drain() {
                let _ = tx.send(Err(RpcError {
                    code: "engine_stopped".into(),
                    message: "The VANS runtime stopped unexpectedly. Restart the app.".into(),
                    data: None,
                }));
            }
        });

        Ok(Self {
            stdin: Mutex::new(stdin),
            pending,
            next_id: AtomicI64::new(1),
            _child: Mutex::new(child),
        })
    }

    /// Sends a request and waits for its response.
    pub async fn call(&self, method: &str, params: Option<Value>) -> Result<Value, RpcError> {
        let id = self.next_id.fetch_add(1, Ordering::SeqCst);
        let (tx, rx) = oneshot::channel();
        self.pending.lock().unwrap().insert(id, tx);

        let req = Request {
            id,
            method: method.to_string(),
            params,
        };
        let mut line = serde_json::to_string(&req).map_err(|e| RpcError {
            code: "internal".into(),
            message: e.to_string(),
            data: None,
        })?;
        line.push('\n');

        {
            let mut stdin = self.stdin.lock().unwrap();
            if let Err(e) = stdin.write_all(line.as_bytes()).and_then(|_| stdin.flush()) {
                self.pending.lock().unwrap().remove(&id);
                return Err(RpcError {
                    code: "engine_stopped".into(),
                    message: format!("Could not reach the VANS runtime: {e}"),
                    data: None,
                });
            }
        }

        rx.await.unwrap_or_else(|_| {
            Err(RpcError {
                code: "engine_stopped".into(),
                message: "The VANS runtime stopped before answering.".into(),
                data: None,
            })
        })
    }
}
