# Improvement Plan 05 — HTTP Health Check Endpoint

**Priority:** Medium  
**Effort:** Low (< 1 day)  
**Impact:** Kubernetes/Docker readiness probes, operational safety  

---

## Problem

The worker has no HTTP endpoint, which means:
- Kubernetes **liveness** and **readiness** probes cannot check if the worker is healthy.
- Docker Compose cannot determine if the container is "up" beyond "process is running".
- If the worker is deadlocked (e.g., stuck on a Redis operation that silently hung) but the process is still alive, no external system will notice.
- There is no way to inspect the worker's current state (is it processing a document? how long has it been running?) without SSHing into the container.

---

## Proposed Solution

Add a lightweight HTTP health check server running on a background thread. This is a standard pattern for non-HTTP workers in containerized environments.

### Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/health` | GET | Liveness probe — returns 200 if process is alive |
| `/ready` | GET | Readiness probe — returns 200 only if Redis and DB are reachable |
| `/status` | GET | Human-readable JSON with worker state |

### `/ready` Response (healthy)
```json
{
  "status": "ok",
  "checks": {
    "redis": "ok",
    "postgres": "ok"
  },
  "queue_depths": {
    "xcentro:extraction_tasks": 3,
    "xcentro:template_tasks": 0
  }
}
```

### `/status` Response
```json
{
  "status": "running",
  "uptime_seconds": 3600,
  "docs_processed": 142,
  "last_processed_at": "2026-05-01T10:32:11Z",
  "current_doc_id": null,
  "template_count": 8
}
```

### Implementation

```python
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

class _HealthHandler(BaseHTTPRequestHandler):
    worker_state = {}  # shared mutable dict, set by main loop

    def do_GET(self):
        if self.path == "/health":
            self._respond(200, {"status": "ok"})
        elif self.path == "/ready":
            checks = self._run_checks()
            code = 200 if all(v == "ok" for v in checks["checks"].values()) else 503
            self._respond(code, checks)
        elif self.path == "/status":
            self._respond(200, self.worker_state)
        else:
            self._respond(404, {"error": "not found"})

    def _respond(self, code, body):
        payload = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, *args): pass  # suppress access logs

def start_health_server(port: int = 9100) -> None:
    server = HTTPServer(("0.0.0.0", port), _HealthHandler)
    t = threading.Thread(target=server.serve_forever, daemon=True)
    t.start()
    logger.info("Health check server listening on :%d", port)
```

### Kubernetes Configuration

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 9100
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /ready
    port: 9100
  initialDelaySeconds: 5
  periodSeconds: 10
  failureThreshold: 3
```

### Files to Modify

- `infrastructure/health_server.py` — new file with `start_health_server()` and `_HealthHandler`
- `infrastructure/config.py` — add `health_check_port: int = 9100`
- `main.py` — call `start_health_server(settings.health_check_port)` before the main loop; update `_HealthHandler.worker_state` after each document

### Acceptance Criteria

- `curl localhost:9100/health` returns `200 OK` while worker is running.
- `curl localhost:9100/ready` returns `503` within 5 seconds of Redis going down.
- Kubernetes restarts the pod automatically when `/ready` returns `503` for > 30s.
- `/status` shows the document ID currently being processed (or null when idle).
