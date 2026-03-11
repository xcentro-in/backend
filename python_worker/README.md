# XCentro Python Extraction Worker

This is the background worker responsible for processing documents (Invoices, Purchase Orders) uploaded via the XCentro Go backend. It handles downloading files, extracting structured data using template matching (OCR) with an LLM fallback, validating the extracted data, and updating the database with the results.

## Architecture

The worker is built using **Clean Architecture** principles to separate concerns, making it highly testable and independent of external frameworks.

```text
python_worker/
├── domain/                  # Enterprise business rules
│   ├── entities.py          # Core data structures (Document, ExtractionResult, LineItem)
│   ├── enums.py             # DocumentType, DocumentStatus, ProcessedBy
│   ├── exceptions.py        # Custom domain exceptions
│   └── ports.py             # Abstract interfaces (Repositories, Extractors, Queue)
├── usecases/                # Application business rules
│   ├── extraction_service.py # Core orchestration flow logic
│   └── validation.py        # Pure functions for GST, metadata, and math validation
├── infrastructure/          # Frameworks and drivers (Implementations of ports)
│   ├── config.py            # Environment variable configuration via Pydantic
│   ├── database.py          # PostgreSQL repository implementation (psycopg2)
│   ├── redis_client.py      # Redis task queue implementation
│   ├── invoice2data_wrapper.py # Template-based OCR extraction (invoice2data)
│   ├── llm_client.py        # Google Gemini LLM fallback extraction
│   └── file_downloader.py   # HTTP file downloader for S3
├── templates/               # YML templates used by invoice2data
├── main.py                  # Application entry point / Composition Root
└── requirements.txt         # Python dependencies
```

### The Processing Flow
1. **Queueing (Go Backend):** The main Go API handles the document upload, saves the file to S3, inserts a `QUEUED` row into the `xcentro.voucher_documents` PostgreSQL table, and pushes a task payload to the `xcentro:extraction_tasks` Redis queue.
2. **Polling (Python Worker):** `main.py` infinitely polls the Redis queue (blocking pop).
3. **Orchestration (`ExtractionService`):**
   - Retrieves the document from PostgreSQL via ID and updates status to `OCR_PROCESSING`.
   - Downloads the document from the provided S3 pre-signed URL to a secure temporary directory `/tmp/extraction`.
   - Attempts extraction using `Invoice2DataExtractor` (Template matching).
   - If a template match is found, it runs validation (`validation.py`). If validation passes, the results are saved and status becomes `VALIDATED`.
   - **Fallback Flow:** If no template matches or template validation fails, the status is updated to `LLM_PROCESSING`, and the document is sent to `GeminiLLMExtractor`.
   - LLM extraction results are validated. If they pass, status is `VALIDATED`. If they fail validation or data is missing, status becomes `REVIEW_REQUIRED`.
   - Always updates `xcentro.voucher_documents` at every state transition.

## Starting Point

The entry point for the worker is `main.py`.

It acts as the **Composition Root**:
1. Loads environment settings via `infrastructure.config.Settings`.
2. Instantiates concrete infrastructure adapters (Database, Redis, Extractors). Note: Google Gemini initialization is deferred lazily so the worker boots even without an API key if LLM fallback isn't used immediately.
3. Injects the infrastructure adapters into the `ExtractionService` (Dependency Injection).
4. Listens for OS signals (SIGINT/SIGTERM) for graceful shutdown.
5. Starts the infinite Redis polling loop.

## Development Setup

### System Dependencies
The `invoice2data` OCR wrapper requires system-level tools for PDF parsing and OCR:
```bash
# macOS (using Homebrew)
brew install poppler tesseract

# Debian/Ubuntu (used in Dockerfile)
apt-get install poppler-utils tesseract-ocr
```

### Python Environment
Requires Python 3.12.
```bash
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt
```
*(Note: `requirements.txt` specifically pins `setuptools>=75.0.0,<82` because version 82+ removed the built-in `pkg_resources` library which `invoice2data` depends on).*

### Environment Variables
Copy the `.env.example` to `.env` and fill in your credentials. The worker requires the same `POSTGRES_` and `REDIS_` credentials defined in the main backend environment.
```dotenv
DATABASE_URL=postgresql://xcentro_rw:XcentroDB789!@localhost:5432/xcentro
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_USER=xcentro_redis
REDIS_PASSWORD=XcentroRedis456!
REDIS_QUEUE=xcentro:extraction_tasks
GEMINI_API_KEY=your_google_ai_studio_api_key  # Optional unless LLM fallback is needed
```

### Running Locally
```bash
python main.py
```

## Docker Execution
This worker runs as an independent container service defined in the `docker-compose.yml`.

```bash
cd deployments/docker
docker compose build python_worker
docker compose up -d python_worker
```
To view logs: `docker logs -f xcentro-python-worker`
