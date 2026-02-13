#!/bin/bash

# XCentro Backend - Test Data Seeding Script
# Loads test data from SQL dump into PostgreSQL database

set -e  # Exit on error

POSTGRES_CONTAINER="xcentro-postgres"
DB_NAME="xcentro"
DB_USER="xcentro_rw"
DB_PASSWORD="RwSecure101!"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQL_FILE="$SCRIPT_DIR/test_data_seed.sql"

echo "=========================================="
echo "XCentro - Seeding Test Data"
echo "=========================================="
echo ""

# Check if SQL dump file exists
if [ ! -f "$SQL_FILE" ]; then
    echo "❌ Error: SQL dump file not found at $SQL_FILE"
    exit 1
fi

echo "📄 SQL dump file: $SQL_FILE"
echo "🐘 Database: $DB_NAME"
echo ""

# Check if PostgreSQL container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${POSTGRES_CONTAINER}$"; then
    echo "❌ Error: PostgreSQL container '$POSTGRES_CONTAINER' is not running"
    echo "   Please start Docker services first: ./start.sh"
    exit 1
fi

echo "✅ PostgreSQL container is running"
echo ""

# Load the SQL dump
echo "📥 Loading test data..."
echo ""

# Copy SQL file into container
docker cp "$SQL_FILE" "$POSTGRES_CONTAINER":/tmp/test_data_seed.sql

# Load the data using xcentro_rw user
PGPASSWORD="$DB_PASSWORD" docker exec -i "$POSTGRES_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -f /tmp/test_data_seed.sql

if [ $? -eq 0 ]; then
    echo ""
    echo "=========================================="
    echo "✅ Test data seeded successfully!"
    echo "=========================================="
    echo ""
    echo "Test data includes:"
    echo "  • Admin user: admin@acme.com / Admin@123"
    echo "  • Business: Xcento's Team (KYC verified)"
    echo "  • Ops user: dev@xcentro.local (UUID: 00000000-0000-0000-0000-000000000001)"
    echo "  • Vendor payee: Nexa Digital Solutions"
    echo "  • Stored cards (2 cards)"
    echo "  • Payments (3 payment records with state changes)"
    echo "  • Voucher with payment"
    echo "  • Payment invoices"
    echo "  • User sessions"
    echo ""
else
    echo ""
    echo "❌ Error: Failed to seed test data"
    exit 1
fi
