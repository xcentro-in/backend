# Quick Test Guide - AWS Secrets Manager Integration

## ✅ Fix: Missing Secrets Added

The error you saw (`field "APIKey" is required`) was because **FAST2SMS_API_KEY** and **TRUTHSCREEN_PASSWORD** were missing from the secrets loading logic.

**Fixed in commit `5440860`** - Now all **33 secrets** are loaded correctly.

---

## 🧪 Quick Test (5 minutes)

### Step 1: Create `.env.local` with All Secrets

```bash
cp .env.local.example .env.local
```

Then edit `.env.local` and fill in **all 33 secrets**. Here's a minimal test version:

```bash
cat > .env.local <<'EOF'
# Encryption & Authentication (5 secrets)
JWT_SECRET_KEY=test_jwt_secret_key_minimum_32_characters_long
BCRYPT_PEPPER=test_bcrypt_pepper_minimum_32_characters_long
BANK_ACCOUNT_ENCRYPTION_KEY=test_bank_encryption_key_32_bytes
OPS_TOKEN_ENCRYPTION_KEY=test_ops_token_encryption_key_32_bytes
RATE_LIMITER_HMAC_SECRET=test_rate_limiter_hmac_secret_32_bytes

# Database & Cache (3 secrets)
DB_PASSWORD=postgres
DB_MIGRATION_PASSWORD=postgres
REDIS_PASSWORD=redis123

# AWS Services (4 secrets)
S3_ACCESS_KEY=test_s3_key
S3_SECRET_KEY=test_s3_secret
SES_ACCESS_KEY=test_ses_key
SES_SECRET_KEY=test_ses_secret

# JioPay Gateway (4 secrets)
JIOPAY_API_KEY=test_jiopay_key
JIOPAY_APP_ID=test_jiopay_app_id
JIOPAY_ENCRYPTION_KEY=test_16byte_key!
JIOPAY_WEBHOOK_SECRET=test_jiopay_webhook

# Easebuzz Gateway (7 secrets)
EASEBUZZ_MERCHANT_KEY=test_easebuzz_merchant
EASEBUZZ_SALT=test_easebuzz_salt
EASEBUZZ_API_KEY=test_easebuzz_api_key
EASEBUZZ_MERCHANT_TOKEN_ID=test_token_id
EASEBUZZ_ENCRYPTION_KEY=test_easebuzz_enc_key
EASEBUZZ_ENCRYPTION_IV=test_easebuzz_iv
EASEBUZZ_WEBHOOK_SECRET=test_easebuzz_webhook

# Castler Payout (6 secrets)
CASTLER_API_KEY=test_castler_api_key
CASTLER_SECRET_KEY=test_castler_secret_key
CASTLER_WEBHOOK_SECRET=test_castler_webhook
CASTLER_ENCRYPTION_KEY=test_castler_encryption_key_32_bytes
CASTLER_ENCRYPTION_IV=TkMwVjAkMFQwTDAzMFJNRQ==
CASTLER_ESCROW_ID=test_escrow_id

# OAuth & Security (2 secrets)
OPS_OAUTH_CLIENT_SECRET=test_oauth_client_secret
TURNSTILE_SECRET_KEY=

# SMS & KYC Services (2 secrets) - NEW!
FAST2SMS_API_KEY=test_fast2sms_api_key
TRUTHSCREEN_PASSWORD=test_truthscreen_password
EOF
```

### Step 2: Create Minimal `.env` (Configuration Only)

```bash
cat > .env <<'EOF'
# Basic config
ENVIRONMENT=development
LOG_LEVEL=debug
LOG_FORMAT=text

# Enable fallback mode
SECRETS_USE_FALLBACK=true
AWS_SECRET_NAME=

# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_URL=http://localhost:8080
FRONTEND_URL=http://localhost:3000

# Database config (non-sensitive)
DB_HOST=localhost
DB_PORT=5432
DB_NAME=xcentro
DB_USER=xcentro_app
DB_SCHEMA=xcentro
DB_MAX_CONNECTIONS=25
DB_MIN_CONNECTIONS=5
DB_MAX_CONN_LIFETIME=1h
DB_MAX_CONN_IDLE_TIME=30m
DB_SSL_MODE=disable
DB_MIGRATION_USER=xcentro_migration

# Redis config (non-sensitive)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_USERNAME=xcentro_redis
REDIS_DB=0
REDIS_MAX_CONNECTIONS=10

# SMS config (non-sensitive)
FAST2SMS_SENDER_ID=XCNTRO
FAST2SMS_ENDPOINT=https://www.fast2sms.com/dev/bulkV2
SMS_OTP_EXPIRY=10m

# KYC config (non-sensitive)
TRUTHSCREEN_USERNAME=test_user
AUTHBRIDGE_USE_MOCK=true

# AWS config (non-sensitive)
S3_BUCKET=test-bucket
S3_REGION=ap-south-1
SES_DEFAULT_FROM_EMAIL=test@example.com
SES_DEFAULT_FROM_NAME=Test
SES_REGION=ap-south-1

# Payment gateways config
JIOPAY_BASE_URL=https://uat.jiopay.co.in/pg/api/v2
JIOPAY_TOKENIZATION_BASE_URL=https://uat.jiopay.co.in/qatsp/api/vault
JIOPAY_MERCHANT_ID=test_merchant
JIOPAY_IS_AGGREGATOR_MODEL=false

EASEBUZZ_BASE_URL=https://pay.easebuzz.in
EASEBUZZ_IS_AGGREGATOR_MODEL=false

CASTLER_BASE_URL=https://api.castler.in
CASTLER_MOCK_ENABLED=true

MOCKPG_ENABLED=false

# Cookie & CORS
COOKIE_SECURE=false
COOKIE_SAMESITE=Lax
CORS_ALLOW_ORIGINS=http://localhost:3000
CORS_ALLOW_CREDENTIALS=true

# JWT config (non-sensitive)
BCRYPT_COST=12
JWT_ACCESS_TOKEN_EXPIRY=1h
JWT_REFRESH_TOKEN_EXPIRY=720h
JWT_MAX_WEB_SESSIONS=3
JWT_MAX_MOBILE_SESSIONS=5

# Rate limiter
RATE_LIMITER_ENABLED=false

# Ops
OPS_REJECT_ACCESS=false
OPS_OAUTH_CLIENT_ID=test_client_id
OPS_OAUTH_REDIRECT_URL=http://localhost:8080/ops/v1/auth/callback
OPS_OAUTH_TENANT_ID=organizations
OPS_OAUTH_SKIP_VALIDATION=true

# Workers
AUTO_PAYOUT_WORKER_ENABLED=false

# Validation
PAYEE_STRICT_VALIDATION_ENABLED=false

# Turnstile
TURNSTILE_ENABLED=false
EOF
```

### Step 3: Run the Server

```bash
# Start the API server
go run ./cmd/api
# or
./bin/api
```

### ✅ Expected Output

```
[2026-01-15 XX:XX:XX] WARN: AWS_SECRET_NAME not configured, using environment variables for secrets
[2026-01-15 XX:XX:XX] WARN: Loading secrets from environment variables (FALLBACK MODE)
[2026-01-15 XX:XX:XX] INFO: Loading secrets from .env.local file
[2026-01-15 XX:XX:XX] INFO: Loaded secrets from environment variables num_secrets=33 mode=fallback
[2026-01-15 XX:XX:XX] INFO: Config loaded successfully with secrets
[2026-01-15 XX:XX:XX] INFO: Logger initialized level=debug format=text environment=development
[2026-01-15 XX:XX:XX] INFO: Custom validator registered successfully
```

Then it will try to connect to database (will fail if Postgres isn't running, which is expected).

---

## 📊 Secrets Count Breakdown

Total: **33 secrets** loaded from `.env.local`

1. Encryption & Auth: 5 (JWT, Bcrypt, Bank encryption, Ops token, Rate limiter HMAC)
2. Database & Cache: 3 (DB password, DB migration password, Redis password)
3. AWS Services: 4 (S3 access/secret, SES access/secret)
4. JioPay Gateway: 4 (API key, App ID, Encryption key, Webhook secret)
5. Easebuzz Gateway: 7 (Merchant key, Salt, API key, Token ID, Encryption key/IV, Webhook)
6. Castler Payout: 6 (API key, Secret key, Webhook, Encryption key/IV, Escrow ID)
7. OAuth & Security: 2 (OAuth client secret, Turnstile secret)
8. **SMS & KYC: 2** ✨ **NEW** (Fast2SMS API key, Truthscreen password)

---

## 🔧 Troubleshooting

### Error: "Loaded secrets from environment variables num_secrets=0"
**Fix**: The `.env.local` file is missing or not being loaded. Verify it exists:
```bash
ls -la .env.local
```

### Error: "field 'APIKey' is required"
**Fix**: Add `FAST2SMS_API_KEY=test_key` to your `.env.local`

### Error: "field 'Password' is required"
**Fix**: Add `TRUTHSCREEN_PASSWORD=test_password` to your `.env.local`

### Error: "failed to connect to database"
**Expected** - This means secrets loaded successfully! Database just isn't running.

---

## ✨ What Changed

**Commits:**
1. `454bc49` - Initial AWS Secrets Manager integration
2. `f7b8915` - Fixed compilation errors
3. `5986b1f` - Auto-load .env.local in fallback mode
4. `5440860` - **Added missing FAST2SMS_API_KEY and TRUTHSCREEN_PASSWORD** ✅

**You're now on the latest commit with all 33 secrets supported!**

---

## 📝 Production JSON Secret

When you create the secret in AWS Secrets Manager for production, use this JSON structure (with your real values):

```json
{
  "jwt_secret_key": "your_production_jwt_secret",
  "bcrypt_pepper": "your_production_bcrypt_pepper",
  "bank_account_encryption_key": "your_production_bank_encryption_key",
  "ops_token_encryption_key": "your_production_ops_token_encryption_key",
  "rate_limiter_hmac_secret": "your_production_rate_limiter_hmac_secret",

  "db_password": "your_production_db_password",
  "db_migration_password": "your_production_migration_password",
  "redis_password": "your_production_redis_password",

  "s3_access_key": "your_production_s3_access_key",
  "s3_secret_key": "your_production_s3_secret_key",
  "ses_access_key": "your_production_ses_access_key",
  "ses_secret_key": "your_production_ses_secret_key",

  "jiopay_api_key": "your_production_jiopay_api_key",
  "jiopay_app_id": "your_production_jiopay_app_id",
  "jiopay_encryption_key": "your_production_jiopay_encryption_key",
  "jiopay_webhook_secret": "your_production_jiopay_webhook_secret",

  "easebuzz_merchant_key": "your_production_easebuzz_merchant_key",
  "easebuzz_salt": "your_production_easebuzz_salt",
  "easebuzz_api_key": "your_production_easebuzz_api_key",
  "easebuzz_merchant_token_id": "your_production_easebuzz_merchant_token_id",
  "easebuzz_encryption_key": "your_production_easebuzz_encryption_key",
  "easebuzz_encryption_iv": "your_production_easebuzz_encryption_iv",
  "easebuzz_webhook_secret": "your_production_easebuzz_webhook_secret",

  "castler_api_key": "your_production_castler_api_key",
  "castler_secret_key": "your_production_castler_secret_key",
  "castler_webhook_secret": "your_production_castler_webhook_secret",
  "castler_encryption_key": "your_production_castler_encryption_key",
  "castler_encryption_iv": "your_production_castler_encryption_iv",
  "castler_escrow_id": "your_production_castler_escrow_id",

  "ops_oauth_client_secret": "your_production_oauth_client_secret",
  "turnstile_secret_key": "your_production_turnstile_secret_key",

  "fast2sms_api_key": "your_production_fast2sms_api_key",
  "truthscreen_password": "your_production_truthscreen_password"
}
```

---

**Ready to test!** Just create the two files above and run the server.
