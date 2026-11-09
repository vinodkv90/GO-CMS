# Go CMS - Production Deployment & Operations Guide

This guide provides operational standards for deploying, scaling, monitoring, and rolling back Go CMS in high-availability enterprise environments.

---

## 1. System Architecture in Production

```
                                      [ Internet ]
                                           │
                                  [ Cloudflare / WAF ]
                                           │
                           [ Ingress / Envoy / NGINX ]
                                (TLS Termination)
                                     │     │
                    ┌────────────────┘     └────────────────┐
                    ▼                                       ▼
        [ Go CMS Pod/Instance 1 ]               [ Go CMS Pod/Instance 2 ]
        - Port: 8080                            - Port: 8080
        - Run as Non-Root (UID 10001)           - Run as Non-Root (UID 10001)
        - In-memory Token Bucket Rate Limiter   - In-memory Token Bucket Rate Limiter
        - Chi Router & Slog Observability       - Chi Router & Slog Observability
                    │                                       │
         ┌──────────┴───────────────┬───────────────────────┴──────────┐
         ▼                          ▼                                  ▼
[ PostgreSQL / MySQL Cluster ]  [ Redis Standalone / Cluster ]   [ S3 / MinIO Object Store ]
  - Primary / Replica             - Tag-based Content Caching      - Encrypted Media Assets
  - Connection Pool (Max 25)      - Session Invalidation           - HMAC Signed URLs
  - Transactional DDL
```

---

## 2. Environment Configuration Reference

All settings can be specified via YAML (`config.yaml`) or overridden via environment variables.

### Core Server & Security
| Variable | Description | Recommended Production Value |
| :--- | :--- | :--- |
| `CMS_SERVER_HOST` | Network interface to bind | `0.0.0.0` |
| `CMS_SERVER_PORT` | HTTP port | `8080` |
| `CMS_SERVER_ENV` | Runtime environment | `production` |
| `CMS_AUTH_JWT_SECRET` | HMAC-SHA256 secret key | Secure random string (>= 32 chars) |
| `CMS_AUTH_SESSION_SECRET`| Cookie session encryption secret | Secure random string (>= 32 chars) |
| `CMS_AUTH_TOKEN_EXPIRATION`| Access token lifetime | `24h` or `12h` |

### Database Connectivity & Pooling
| Variable | Description | Recommended Production Value |
| :--- | :--- | :--- |
| `CMS_DB_DRIVER` | Database engine | `postgres` (or `mysql`, `mongodb`) |
| `CMS_DB_HOST` | Database host | Primary cluster hostname |
| `CMS_DB_PORT` | Database port | `5432` |
| `CMS_DB_USER` | Database role | `cms_app` |
| `CMS_DB_PASSWORD` | Database credentials | Managed vault secret |
| `CMS_DB_NAME` | Database catalog name | `cms_production` |
| `CMS_DB_SSLMODE` | SSL certificate enforcement | `verify-full` or `require` |
| `CMS_DB_MAX_OPEN_CONNS` | Maximum open database pool connections | `25` - `50` per pod |
| `CMS_DB_MAX_IDLE_CONNS` | Maximum idle connections in pool | `10` - `20` per pod |
| `CMS_DB_CONN_MAX_LIFETIME` | Maximum connection age | `15m` |
| `CMS_DB_CONN_MAX_IDLE_TIME` | Connection idle eviction threshold | `5m` |

### Caching & Background Workers
| Variable | Description | Recommended Production Value |
| :--- | :--- | :--- |
| `CMS_CACHE_DRIVER` | Cache provider | `redis` |
| `CMS_CACHE_REDIS_URL` | Redis connection URI | `redis://:secret@redis-cluster:6379/0` |
| `CMS_JOBS_ENABLED` | Enable background worker pool | `true` |
| `CMS_JOBS_CONCURRENCY` | Worker pool concurrent goroutines | `4` - `10` per pod |

### Media & Storage
| Variable | Description | Recommended Production Value |
| :--- | :--- | :--- |
| `CMS_MEDIA_PROVIDER` | Media storage driver | `s3` (or `minio`) |
| `CMS_MEDIA_SIGNED_SECRET` | HMAC key for private media signed URLs | Secure random string (>= 32 chars) |

---

## 3. Kubernetes Deployment & Service Manifests

### `deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-cms
  labels:
    app: go-cms
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: go-cms
  template:
    metadata:
      labels:
        app: go-cms
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
        runAsGroup: 10001
        fsGroup: 10001
      containers:
        - name: cms
          image: ghcr.io/go-cms/cms:v1.0.0
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
              name: http
          resources:
            requests:
              cpu: 250m
              memory: 256Mi
            limits:
              cpu: 1000m
              memory: 1Gi
          envFrom:
            - secretRef:
                name: go-cms-secrets
            - configMapRef:
                name: go-cms-config
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 3
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 3
          lifecycle:
            preStop:
              exec:
                command: ["/bin/sleep", "5"]
```

---

## 4. Zero-Downtime Migration Procedures

Database migrations are managed with the standalone binary `cms-migrate`:

```bash
# 1. Inspect pending migrations before deployment
./bin/cms-migrate status

# 2. Apply pending migrations
./bin/cms-migrate up

# 3. Verify database health
curl -f http://localhost:8080/readyz
```

In CI/CD or Kubernetes, execute `cms-migrate up` inside an initContainer or Kubernetes Job before the deployment rollout begins.

---

## 5. Automated Backup & Disaster Recovery

### Creating a Snapshot Backup
```bash
# Linux / macOS / POSIX
./scripts/backup.sh /var/backups/cms

# Windows PowerShell
.\scripts\backup.ps1 -BackupDir C:\Backups\CMS
```
The script will dump the active database (PostgreSQL / MySQL / MongoDB), archive the media directory, compute the SHA256 integrity hash, and store a timestamped archive bundle.

### Restoring from Backup
```bash
# Verify checksum and restore
./scripts/restore.sh /var/backups/cms/cms_backup_20261109_120000.tar.gz --force
```

---

## 6. Rollback Strategy

If a newly deployed image or code version causes anomalies:

1. **Application Rollback (Immediate):**
   ```bash
   kubectl rollout undo deployment/go-cms
   ```
   Or with Docker Compose:
   ```bash
   docker compose up -d --no-deps cms-server:previous-tag
   ```

2. **Database Rollback:**
   - If transactional migrations were applied, run the corresponding down migration or restore the pre-deployment snapshot using `./scripts/restore.sh`.

3. **Cache Purge:**
   - Invalidate stale Redis keys:
     ```bash
     redis-cli -u $CMS_CACHE_REDIS_URL FLUSHDB
     ```

---

## 7. Observability & SLIs

Scrape Prometheus metrics every 15 seconds from `http://<pod-ip>:8080/metrics`:

- **Traffic Rate:** `sum(rate(cms_http_requests_total[5m]))`
- **Error Rate (5xx):** `sum(rate(cms_http_requests_total{status=~"5.."}[5m])) / sum(rate(cms_http_requests_total[5m]))`
- **P99 Latency:** `histogram_quantile(0.99, sum(rate(cms_http_request_duration_seconds_bucket[5m])) by (le))`
- **Database Pool Saturation:** `cms_db_connections_in_use / cms_db_connections_open`
