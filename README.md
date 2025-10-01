# XCentro Backend

## Quick Start

Follow these steps to get the XCentro backend up and running locally:

### 1. Start Docker Services

Navigate to the Docker deployment directory and start PostgreSQL and Redis services:

```bash
cd deployments/docker
./start.sh
```

This will start the required PostgreSQL and Redis containers.

### 2. Setup Database and Cache

Run the setup scripts to configure PostgreSQL users and Redis users:

```bash
# Setup PostgreSQL users and permissions
./setup-postgres-users.sh

# Setup Redis users and permissions
./setup-redis-users.sh
```

### 3. Environment Configuration

Copy the environment template and configure your settings:

```bash
cd ../..  # Back to project root
cp .env.example .env
```

Edit `.env` file with your database and Redis credentials if needed.

### 4. Run Database Migrations

Execute the database migrations to set up the schema:

```bash
go run ./cmd/migrate
```

This will create all necessary tables and schema in your PostgreSQL database.

### 5. Install Air (Hot Reload)

Install Air for automatic code reloading during development:

```bash
go install github.com/air-verse/air@latest
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
```

### 6. Start the Development Server

Run the application with hot reload:

```bash
air
```

The server will start on `http://0.0.0.0:8888` and automatically reload whenever you make changes to Go files.

### Development Notes

- The application uses PostgreSQL for data storage and Redis for caching
- Environment variables are loaded from `.env` file automatically
- Air watches all Go files and rebuilds/restarts the server on changes
- Check the logs for any connection issues with PostgreSQL or Redis

### Stopping Services

To stop the Docker services when done:

```bash
cd deployments/docker
docker compose down
```