# Ops Dashboard: Invoice Management API

## Overview

The Ops Dashboard provides invoice management endpoints for the operations team to list, filter, and export GST invoices across all businesses. These endpoints are designed for internal use and require ops dashboard authentication.

**Key Points:**
- ✅ List all invoices with pagination and filtering
- ✅ Export invoices in CSV format
- ✅ Filter by date range, business, IRN status, and filing status
- ✅ Ops JWT authentication required
- ✅ Access to all businesses (not scoped to single business)

---

## Authentication

All ops invoice endpoints require Ops Dashboard JWT authentication:
```
Authorization: Bearer <ops_jwt_token>
```

**Token Acquisition:**
1. Login via `/ops/v1/auth/login`
2. OAuth callback via `/ops/v1/auth/callback`
3. Use returned JWT token for all protected endpoints

---

## Base URL

```
Production: https://api.xcentro.in/ops/v1
Development: http://localhost:9090/ops/v1
```

---

## 1. List Invoices (Paginated)

### Endpoint
```http
GET /ops/v1/invoices
```

### Query Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `page` | integer | No | 1 | Page number (1-indexed) |
| `page_size` | integer | No | 20 | Number of invoices per page (max 100) |
| `start_date` | string | No | - | Filter by invoice date start (YYYY-MM-DD) |
| `end_date` | string | No | - | Filter by invoice date end (YYYY-MM-DD) |
| `business_id` | integer | No | - | Filter by business ID |
| `irn_status` | string | No | - | Filter by IRN status: `pending`, `irn_generated`, `failed`, `cancelled` |
| `filing_status` | string | No | - | Filter by filing status: `not_filed`, `filed`, `failed` |

### Example Requests

**List all invoices (default pagination):**
```bash
curl -X GET "https://api.xcentro.in/ops/v1/invoices" \
  -H "Authorization: Bearer ${OPS_JWT_TOKEN}"
```

**Filter by date range:**
```bash
curl -X GET "https://api.xcentro.in/ops/v1/invoices?start_date=2025-11-01&end_date=2025-11-30" \
  -H "Authorization: Bearer ${OPS_JWT_TOKEN}"
```

**Filter by business and IRN status:**
```bash
curl -X GET "https://api.xcentro.in/ops/v1/invoices?business_id=123&irn_status=pending" \
  -H "Authorization: Bearer ${OPS_JWT_TOKEN}"
```

**Pagination example:**
```bash
curl -X GET "https://api.xcentro.in/ops/v1/invoices?page=2&page_size=50" \
  -H "Authorization: Bearer ${OPS_JWT_TOKEN}"
```

### Response (200 OK)

```json
{
  "invoices": [
    {
      "invoice_id": 123,
      "payment_id": "550e8400-e29b-41d4-a716-446655440000",
      "invoice_number": "INV-123",
      "gstin_supplier": "27AATCM5865F1ZU",
      "gstin_recipient": "29ABCDE1234F1Z5",
      "invoice_date": "2025-11-19T00:00:00Z",
      "irn": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6a7b8c9d0e1f2",
      "irn_ack_no": "ACK123456789",
      "irn_ack_date": "2025-11-19T10:30:00Z",
      "irn_status": "irn_generated",
      "qr_code": "base64_encoded_qr_code_data",
      "filing_status": "filed",
      "description": "Convenience Fee",
      "sac_code": "997159",
      "taxable_value": 100.00,
      "gst_rate": 18.00,
      "gst_amount": 18.00,
      "total_amount": 118.00,
      "created_at": "2025-11-19T10:30:00Z",
      "updated_at": "2025-11-19T10:35:00Z"
    }
  ],
  "total_count": 150,
  "page": 1,
  "page_size": 20,
  "total_pages": 8
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `invoices` | array | Array of invoice objects |
| `total_count` | integer | Total number of invoices matching filters |
| `page` | integer | Current page number |
| `page_size` | integer | Number of invoices per page |
| `total_pages` | integer | Total number of pages |

---

## 2. Export Invoices (CSV)

### Endpoint
```http
GET /ops/v1/invoices/export
```

### Query Parameters

Same filtering parameters as List Invoices endpoint:
- `start_date`, `end_date`, `business_id`, `irn_status`, `filing_status`

**Note:** No pagination parameters - exports ALL matching invoices

### Example Requests

**Export all invoices:**
```bash
curl -X GET "https://api.xcentro.in/ops/v1/invoices/export" \
  -H "Authorization: Bearer ${OPS_JWT_TOKEN}" \
  -o invoices.csv
```

**Export invoices for specific date range:**
```bash
curl -X GET "https://api.xcentro.in/ops/v1/invoices/export?start_date=2025-11-01&end_date=2025-11-30" \
  -H "Authorization: Bearer ${OPS_JWT_TOKEN}" \
  -o invoices_november_2025.csv
```

**Export invoices for specific business:**
```bash
curl -X GET "https://api.xcentro.in/ops/v1/invoices/export?business_id=123" \
  -H "Authorization: Bearer ${OPS_JWT_TOKEN}" \
  -o business_123_invoices.csv
```

### Response (200 OK)

**Content-Type:** `text/csv`
**Content-Disposition:** `attachment; filename="invoices_export.csv"`

**CSV Format:**
```csv
Invoice ID,Payment ID,Invoice Number,GSTIN Supplier,GSTIN Recipient,Invoice Date,IRN,IRN Ack No,IRN Ack Date,IRN Status,Filing Status,Description,SAC Code,Taxable Value,GST Rate,GST Amount,Total Amount,Created At,Updated At
123,550e8400-e29b-41d4-a716-446655440000,INV-123,27AATCM5865F1ZU,29ABCDE1234F1Z5,2025-11-19,a1b2c3d4...,ACK123456789,2025-11-19T10:30:00Z,irn_generated,filed,Convenience Fee,997159,100.00,18.00,18.00,118.00,2025-11-19T10:30:00Z,2025-11-19T10:35:00Z
```

---

## Error Responses

### Error Format
```json
{
  "error": "Error message description"
}
```

### Common Error Codes

| Status Code | Error | Description |
|-------------|-------|-------------|
| `400` | Bad Request | Invalid filter parameters or values |
| `401` | Unauthorized | Missing or invalid Ops JWT token |
| `403` | Forbidden | Insufficient permissions (not an ops user) |
| `500` | Internal Server Error | Server error during query execution |

### Example Error Responses

**Invalid Date Format (400):**
```json
{
  "error": "Invalid date format. Use YYYY-MM-DD"
}
```

**Unauthorized (401):**
```json
{
  "error": "missing or malformed jwt"
}
```

**Forbidden (403):**
```json
{
  "error": "Access denied - ops dashboard access required"
}
```

---

## Integration Examples

### JavaScript/TypeScript Example

```typescript
interface OpsInvoiceListParams {
  page?: number;
  page_size?: number;
  start_date?: string; // YYYY-MM-DD
  end_date?: string; // YYYY-MM-DD
  business_id?: number;
  irn_status?: 'pending' | 'irn_generated' | 'failed' | 'cancelled';
  filing_status?: 'not_filed' | 'filed' | 'failed';
}

interface OpsInvoiceListResponse {
  invoices: Invoice[];
  total_count: number;
  page: number;
  page_size: number;
  total_pages: number;
}

interface Invoice {
  invoice_id: number;
  payment_id: string;
  invoice_number: string;
  gstin_supplier: string;
  gstin_recipient: string;
  invoice_date: string;
  irn?: string;
  irn_ack_no?: string;
  irn_ack_date?: string;
  irn_status: 'pending' | 'irn_generated' | 'failed' | 'cancelled';
  qr_code?: string;
  filing_status: 'not_filed' | 'filed' | 'failed';
  description: string;
  sac_code: string;
  taxable_value: number;
  gst_rate: number;
  gst_amount: number;
  total_amount: number;
  created_at: string;
  updated_at: string;
}

// List invoices
async function listInvoices(
  token: string,
  params: OpsInvoiceListParams = {}
): Promise<OpsInvoiceListResponse> {
  const queryString = new URLSearchParams(
    Object.entries(params).reduce((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = String(value);
      }
      return acc;
    }, {} as Record<string, string>)
  ).toString();

  const response = await fetch(
    `https://api.xcentro.in/ops/v1/invoices?${queryString}`,
    {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      }
    }
  );

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to fetch invoices');
  }

  return await response.json();
}

// Export invoices
async function exportInvoices(
  token: string,
  params: Omit<OpsInvoiceListParams, 'page' | 'page_size'> = {}
): Promise<Blob> {
  const queryString = new URLSearchParams(
    Object.entries(params).reduce((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = String(value);
      }
      return acc;
    }, {} as Record<string, string>)
  ).toString();

  const response = await fetch(
    `https://api.xcentro.in/ops/v1/invoices/export?${queryString}`,
    {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${token}`
      }
    }
  );

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to export invoices');
  }

  return await response.blob();
}

// Usage example
const token = 'your_ops_jwt_token';

// List invoices with filters
const invoices = await listInvoices(token, {
  page: 1,
  page_size: 50,
  start_date: '2025-11-01',
  end_date: '2025-11-30',
  irn_status: 'pending'
});

console.log(`Found ${invoices.total_count} invoices`);

// Export invoices
const csvBlob = await exportInvoices(token, {
  start_date: '2025-11-01',
  end_date: '2025-11-30'
});

// Download CSV file
const url = window.URL.createObjectURL(csvBlob);
const a = document.createElement('a');
a.href = url;
a.download = 'invoices_export.csv';
document.body.appendChild(a);
a.click();
window.URL.revokeObjectURL(url);
document.body.removeChild(a);
```

### React Component Example

```tsx
import React, { useState, useEffect } from 'react';
import axios from 'axios';

interface InvoiceFilters {
  startDate?: string;
  endDate?: string;
  businessId?: number;
  irnStatus?: string;
  filingStatus?: string;
}

const OpsInvoiceList: React.FC = () => {
  const [invoices, setInvoices] = useState([]);
  const [totalCount, setTotalCount] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [filters, setFilters] = useState<InvoiceFilters>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchInvoices = async () => {
    setLoading(true);
    setError(null);

    try {
      const token = localStorage.getItem('ops_jwt_token');
      const params = {
        page,
        page_size: pageSize,
        start_date: filters.startDate,
        end_date: filters.endDate,
        business_id: filters.businessId,
        irn_status: filters.irnStatus,
        filing_status: filters.filingStatus
      };

      const response = await axios.get('/ops/v1/invoices', {
        params,
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });

      setInvoices(response.data.invoices);
      setTotalCount(response.data.total_count);
    } catch (err: any) {
      setError(err.response?.data?.error || 'Failed to fetch invoices');
    } finally {
      setLoading(false);
    }
  };

  const exportInvoices = async () => {
    try {
      const token = localStorage.getItem('ops_jwt_token');
      const params = {
        start_date: filters.startDate,
        end_date: filters.endDate,
        business_id: filters.businessId,
        irn_status: filters.irnStatus,
        filing_status: filters.filingStatus
      };

      const response = await axios.get('/ops/v1/invoices/export', {
        params,
        headers: {
          'Authorization': `Bearer ${token}`
        },
        responseType: 'blob'
      });

      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', 'invoices_export.csv');
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);
    } catch (err: any) {
      setError(err.response?.data?.error || 'Failed to export invoices');
    }
  };

  useEffect(() => {
    fetchInvoices();
  }, [page, filters]);

  return (
    <div>
      <h1>Invoice Management</h1>

      {/* Filters */}
      <div className="filters">
        <input
          type="date"
          placeholder="Start Date"
          onChange={(e) => setFilters({ ...filters, startDate: e.target.value })}
        />
        <input
          type="date"
          placeholder="End Date"
          onChange={(e) => setFilters({ ...filters, endDate: e.target.value })}
        />
        <select onChange={(e) => setFilters({ ...filters, irnStatus: e.target.value })}>
          <option value="">All IRN Status</option>
          <option value="pending">Pending</option>
          <option value="irn_generated">Generated</option>
          <option value="failed">Failed</option>
          <option value="cancelled">Cancelled</option>
        </select>
        <button onClick={exportInvoices}>Export CSV</button>
      </div>

      {error && <div className="error">{error}</div>}
      {loading && <div>Loading...</div>}

      {/* Invoice Table */}
      <table>
        <thead>
          <tr>
            <th>Invoice ID</th>
            <th>Invoice Number</th>
            <th>Payment ID</th>
            <th>Date</th>
            <th>GSTIN</th>
            <th>Amount</th>
            <th>IRN Status</th>
            <th>Filing Status</th>
          </tr>
        </thead>
        <tbody>
          {invoices.map((inv: any) => (
            <tr key={inv.invoice_id}>
              <td>{inv.invoice_id}</td>
              <td>{inv.invoice_number}</td>
              <td>{inv.payment_id}</td>
              <td>{new Date(inv.invoice_date).toLocaleDateString()}</td>
              <td>{inv.gstin_recipient}</td>
              <td>₹{inv.total_amount.toFixed(2)}</td>
              <td>{inv.irn_status}</td>
              <td>{inv.filing_status}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Pagination */}
      <div className="pagination">
        <button disabled={page === 1} onClick={() => setPage(page - 1)}>
          Previous
        </button>
        <span>Page {page} of {Math.ceil(totalCount / pageSize)}</span>
        <button
          disabled={page * pageSize >= totalCount}
          onClick={() => setPage(page + 1)}
        >
          Next
        </button>
      </div>
    </div>
  );
};

export default OpsInvoiceList;
```

---

## Testing Guide

### Test Scenarios

1. **List All Invoices**
   - Call `/ops/v1/invoices` without filters
   - Verify pagination works correctly
   - Verify response includes all required fields

2. **Filter by Date Range**
   - Set `start_date` and `end_date`
   - Verify only invoices within range are returned
   - Test edge cases (same date, invalid dates)

3. **Filter by Business**
   - Set `business_id` parameter
   - Verify only invoices for that business are returned

4. **Filter by Status**
   - Test `irn_status` filter (pending, irn_generated, etc.)
   - Test `filing_status` filter (not_filed, filed, etc.)

5. **Export CSV**
   - Call `/ops/v1/invoices/export`
   - Verify CSV file downloads correctly
   - Verify CSV contains all columns
   - Test with various filters

6. **Unauthorized Access**
   - Try without JWT token
   - Try with expired token
   - Verify 401 errors

7. **Pagination Edge Cases**
   - Test page 1, last page, beyond last page
   - Test various page sizes (1, 20, 100)

---

## Important Notes

### Date Filtering
- All dates are in UTC
- Date format: `YYYY-MM-DD`
- `start_date` is inclusive
- `end_date` is inclusive
- Both dates are optional

### Pagination
- Page numbers are 1-indexed (first page is 1, not 0)
- Default page size: 20
- Maximum page size: 100
- Empty pages return empty array, not 404

### CSV Export
- No pagination applied - exports ALL matching invoices
- Large exports may take time
- Consider adding date range filters for better performance
- CSV uses UTF-8 encoding

### Permissions
- Only ops dashboard users can access these endpoints
- Regular business users cannot access ops endpoints
- JWT token must be from ops authentication flow

---

## Support

For ops dashboard issues:
- **Ops Dashboard Docs:** `/docs/ops_dashboard.md`
- **Backend Team:** backend@xcentro.in
- **Slack Channel:** #ops-dashboard-support

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2025-12-07 | Initial ops invoice endpoints documentation |

---

**Document Version:** 1.0.0
**Last Updated:** 2025-12-07
**Maintained By:** XCentro Backend Team
