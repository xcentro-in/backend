# Frontend Integration Guide: GST Invoice System

## Overview

The XCentro backend automatically generates GST-compliant invoices for all successful payments. Invoices are created when the payment callback is processed (first call after payment success) and contain convenience fee breakdown with GST details.

**Key Points:**
- ✅ Invoices are automatically created on payment success
- ✅ No frontend action required for invoice creation
- ✅ Invoices are available immediately after callback processing
- ✅ Download endpoints support both PDF and JSON formats
- ✅ JWT authentication required for all invoice endpoints

---

## Payment Flow Integration

### Current Payment Flow (No Changes Required)

```
1. User initiates payment
2. Frontend redirects to payment gateway
3. Payment succeeds at gateway
4. Gateway redirects back to: /v1/process_payment/callback
5. Callback handler processes payment and creates invoice ✅
6. Frontend receives success response
7. [NEW] Frontend can now fetch invoice
```

**Important:** Invoice creation happens automatically in step 5. No additional API calls needed during payment flow.

---

## API Endpoints

### Base URL
```
Production: https://api.xcentro.in
Development: http://localhost:8080
```

### Authentication
All invoice endpoints require JWT authentication:
```
Authorization: Bearer <jwt_token>
```

---

## 1. Download Invoice by Invoice ID

### Endpoint
```http
GET /v1/invoices/download?invoice_id={invoice_id}&format={format}
```

### Query Parameters
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `invoice_id` | integer | Yes | The unique invoice ID |
| `format` | string | No | Response format: `pdf` or `json` (default: `pdf`) |

### Example Requests

**Download PDF:**
```javascript
const downloadInvoicePDF = async (invoiceId) => {
  const response = await fetch(
    `${API_BASE_URL}/v1/invoices/download?invoice_id=${invoiceId}&format=pdf`,
    {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${jwtToken}`
      }
    }
  );

  if (!response.ok) {
    throw new Error('Failed to download invoice');
  }

  // Convert response to blob for PDF download
  const blob = await response.blob();
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `invoice_${invoiceId}.pdf`;
  document.body.appendChild(a);
  a.click();
  window.URL.revokeObjectURL(url);
  document.body.removeChild(a);
};
```

**Fetch JSON:**
```javascript
const getInvoiceDetails = async (invoiceId) => {
  const response = await fetch(
    `${API_BASE_URL}/v1/invoices/download?invoice_id=${invoiceId}&format=json`,
    {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${jwtToken}`,
        'Content-Type': 'application/json'
      }
    }
  );

  if (!response.ok) {
    throw new Error('Failed to fetch invoice');
  }

  return await response.json();
};
```

### Response (format=json)
```json
{
  "invoice_id": 123,
  "payment_id": "550e8400-e29b-41d4-a716-446655440000",
  "invoice_number": "INV-123",
  "gstin_supplier": "27AATCM5865F1ZU",
  "gstin_recipient": "29ABCDE1234F1Z5",
  "invoice_date": "2025-11-19",
  "irn": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6a7b8c9d0e1f2",
  "irn_status": "pending",
  "filing_status": "not_filed",
  "description": "Payment Processing Fee",
  "sac_code": "998599",
  "taxable_value": 100.00,
  "gst_rate": 18.00,
  "gst_amount": 18.00,
  "total_amount": 118.00,
  "created_at": "2025-11-19T10:30:00Z",
  "updated_at": "2025-11-19T10:30:00Z"
}
```

### Response (format=pdf)
- **Content-Type:** `application/pdf`
- **Content-Disposition:** `attachment; filename="invoice_{invoice_id}.pdf"`
- Binary PDF data

---

## 2. Download Invoice by Payment ID

### Endpoint
```http
GET /v1/invoices/download?payment_id={payment_id}&format={format}
```

### Query Parameters
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `payment_id` | UUID | Yes | The payment UUID |
| `format` | string | No | Response format: `pdf` or `json` (default: `pdf`) |

### Example Requests

**Download PDF by Payment ID:**
```javascript
const downloadInvoiceByPaymentId = async (paymentId) => {
  const response = await fetch(
    `${API_BASE_URL}/v1/invoices/download?payment_id=${paymentId}&format=pdf`,
    {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${jwtToken}`
      }
    }
  );

  if (!response.ok) {
    throw new Error('Failed to download invoice');
  }

  const blob = await response.blob();
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `invoice_payment_${paymentId}.pdf`;
  document.body.appendChild(a);
  a.click();
  window.URL.revokeObjectURL(url);
  document.body.removeChild(a);
};
```

**Fetch JSON by Payment ID:**
```javascript
const getInvoiceByPaymentId = async (paymentId) => {
  const response = await fetch(
    `${API_BASE_URL}/v1/invoices/download?payment_id=${paymentId}&format=json`,
    {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${jwtToken}`,
        'Content-Type': 'application/json'
      }
    }
  );

  if (!response.ok) {
    throw new Error('Failed to fetch invoice');
  }

  return await response.json();
};
```

### Response
Same as "Download Invoice by Invoice ID" endpoint.

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
| `400` | Bad Request | Missing or invalid parameters (invoice_id or payment_id required) |
| `401` | Unauthorized | Missing or invalid JWT token |
| `404` | Not Found | Invoice not found for the given ID or payment ID |
| `500` | Internal Server Error | Server error during invoice generation |

### Example Error Responses

**Missing Parameters (400):**
```json
{
  "error": "Either invoice_id or payment_id is required"
}
```

**Invoice Not Found (404):**
```json
{
  "error": "Invoice not found"
}
```

**Unauthorized (401):**
```json
{
  "error": "missing or malformed jwt"
}
```

---

## Integration Examples

### React Component Example

```jsx
import React, { useState } from 'react';
import axios from 'axios';

const InvoiceDownload = ({ paymentId }) => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const downloadInvoice = async () => {
    setLoading(true);
    setError(null);

    try {
      const response = await axios.get(
        `/v1/invoices/download?payment_id=${paymentId}&format=pdf`,
        {
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
          },
          responseType: 'blob' // Important for PDF download
        }
      );

      // Create blob link to download
      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `invoice_${paymentId}.pdf`);
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);
    } catch (err) {
      setError(err.response?.data?.error || 'Failed to download invoice');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <button onClick={downloadInvoice} disabled={loading}>
        {loading ? 'Downloading...' : 'Download Invoice'}
      </button>
      {error && <p style={{ color: 'red' }}>{error}</p>}
    </div>
  );
};

export default InvoiceDownload;
```

### Vue.js Example

```vue
<template>
  <div>
    <button @click="downloadInvoice" :disabled="loading">
      {{ loading ? 'Downloading...' : 'Download Invoice' }}
    </button>
    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>

<script>
import axios from 'axios';

export default {
  name: 'InvoiceDownload',
  props: {
    paymentId: {
      type: String,
      required: true
    }
  },
  data() {
    return {
      loading: false,
      error: null
    };
  },
  methods: {
    async downloadInvoice() {
      this.loading = true;
      this.error = null;

      try {
        const response = await axios.get(
          `/v1/invoices/download?payment_id=${this.paymentId}&format=pdf`,
          {
            headers: {
              'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
            },
            responseType: 'blob'
          }
        );

        const url = window.URL.createObjectURL(new Blob([response.data]));
        const link = document.createElement('a');
        link.href = url;
        link.setAttribute('download', `invoice_${this.paymentId}.pdf`);
        document.body.appendChild(link);
        link.click();
        link.remove();
        window.URL.revokeObjectURL(url);
      } catch (err) {
        this.error = err.response?.data?.error || 'Failed to download invoice';
      } finally {
        this.loading = false;
      }
    }
  }
};
</script>

<style scoped>
.error {
  color: red;
  margin-top: 10px;
}
</style>
```

### Angular Example

```typescript
import { Component, Input } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';

@Component({
  selector: 'app-invoice-download',
  template: `
    <button (click)="downloadInvoice()" [disabled]="loading">
      {{ loading ? 'Downloading...' : 'Download Invoice' }}
    </button>
    <p *ngIf="error" style="color: red">{{ error }}</p>
  `
})
export class InvoiceDownloadComponent {
  @Input() paymentId!: string;
  loading = false;
  error: string | null = null;

  constructor(private http: HttpClient) {}

  downloadInvoice(): void {
    this.loading = true;
    this.error = null;

    const token = localStorage.getItem('jwt_token');
    const headers = new HttpHeaders({
      'Authorization': `Bearer ${token}`
    });

    this.http.get(
      `/v1/invoices/download?payment_id=${this.paymentId}&format=pdf`,
      { headers, responseType: 'blob' }
    ).subscribe({
      next: (blob) => {
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `invoice_${this.paymentId}.pdf`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        window.URL.revokeObjectURL(url);
        this.loading = false;
      },
      error: (err) => {
        this.error = err.error?.error || 'Failed to download invoice';
        this.loading = false;
      }
    });
  }
}
```

---

## UI/UX Recommendations

### 1. Payment Success Page
After successful payment, display an option to download the invoice:

```
✅ Payment Successful!

Transaction ID: TXN123456789
Amount Paid: ₹118.00

[Download Invoice PDF] [View Invoice Details]
```

### 2. Transaction History
In the transaction list, add a download icon/button for each completed payment:

```
Payment Date | Amount | Status | Actions
2025-11-19   | ₹118   | Success | [📄 Download Invoice]
```

### 3. Invoice Availability
- Invoices are available **immediately** after payment callback completes
- No need to poll or wait
- If download fails with 404, the payment might still be processing

### 4. Loading States
```javascript
// Show loading state while downloading
<button disabled={loading}>
  {loading ? 'Generating Invoice...' : 'Download Invoice'}
</button>
```

### 5. Error Handling
```javascript
// Graceful error handling
if (error.status === 404) {
  showMessage('Invoice not found. Please contact support.');
} else if (error.status === 401) {
  showMessage('Please login again to download invoice.');
} else {
  showMessage('Failed to download invoice. Please try again.');
}
```

---

## Testing Guide

### Test Scenarios

1. **Successful Download (PDF)**
   - Navigate to a successful payment
   - Click "Download Invoice"
   - Verify PDF downloads with correct filename
   - Open PDF and verify content

2. **Successful Download (JSON)**
   - Call endpoint with `format=json`
   - Verify JSON structure matches response format
   - Verify all amounts are in rupees (not paise)

3. **Invoice Not Found**
   - Try to download invoice for non-existent payment_id
   - Verify 404 error response
   - Display user-friendly error message

4. **Unauthorized Access**
   - Try to download without JWT token
   - Verify 401 error response
   - Redirect to login page

5. **Invalid Parameters**
   - Call endpoint without invoice_id or payment_id
   - Verify 400 error response
   - Display validation error message

### Test Data

Use these test payment IDs in development environment:
```
Valid Payment ID: 550e8400-e29b-41d4-a716-446655440000
Invalid Payment ID: 00000000-0000-0000-0000-000000000000
```

---

## Invoice PDF Format

The generated PDF includes:

### Header Section
- **Company Name:** XCentro Technologies Pvt. Ltd.
- **GSTIN:** 27AATCM5865F1ZU
- **Invoice Number:** INV-{id}
- **Invoice Date:** YYYY-MM-DD

### Customer Section
- **Customer GSTIN:** {business_gstin}
- **Payment ID:** {payment_id}

### Line Items
| Description | SAC Code | Taxable Value | GST Rate | GST Amount | Total |
|-------------|----------|---------------|----------|------------|-------|
| Payment Processing Fee | 998599 | ₹100.00 | 18% | ₹18.00 | ₹118.00 |

### Footer Section
- **IRN:** {64-character hash} (if generated)
- **QR Code:** (if IRN available)

---

## Important Notes

### Amount Conversion
- All amounts in API responses are in **rupees** (e.g., 100.00 = ₹100)
- Backend stores convenience fees in paise, converts to rupees for invoices
- No conversion needed on frontend

### Invoice Number Format
- Format: `INV-{invoice_id}`
- Example: `INV-123`, `INV-456`
- Sequential numbering based on database ID

### IRN (Invoice Reference Number)
- 64-character SHA256 hash
- Format: SHA256(GSTIN|INV|INV-{id}|FY-YYYY-YY)
- Initially status is `pending`
- Used for GST e-invoicing compliance

### GSTIN Validation
- Supplier GSTIN: `27AATCM5865F1ZU` (fixed)
- Recipient GSTIN: From business profile
- 15-character alphanumeric format

---

## Troubleshooting

### Issue: Invoice download returns 404
**Possible Causes:**
1. Payment hasn't completed callback processing yet
2. Payment failed (no invoice created for failed payments)
3. Invoice was deleted due to verification mismatch

**Solution:**
- Wait a few seconds and retry
- Check payment status first
- Contact support if persists

### Issue: PDF downloads but is corrupted
**Possible Causes:**
1. Response not handled as blob
2. Incorrect Content-Type header

**Solution:**
```javascript
// Ensure responseType is set to 'blob'
axios.get(url, { responseType: 'blob' })
```

### Issue: JWT token expired
**Possible Causes:**
1. Token expired (24-hour validity)
2. Token not refreshed

**Solution:**
- Refresh JWT token before download
- Handle 401 errors and redirect to login

---

## Support

For integration issues or questions:
- **Technical Documentation:** `/docs/payment_module.md`
- **API Reference:** `/docs/API_REFERENCE.md`
- **Backend Team:** backend@xcentro.in

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2025-11-19 | Initial release with automatic invoice generation |

---

## Appendix: Complete TypeScript Interface

```typescript
interface Invoice {
  invoice_id: number;
  payment_id: string; // UUID
  invoice_number: string; // Format: INV-{id}
  gstin_supplier: string; // Default: "27AATCM5865F1ZU"
  gstin_recipient: string; // Business GSTIN (15 chars)
  invoice_date: string; // ISO date format: YYYY-MM-DD
  irn?: string; // 64-character hash (optional)
  irn_ack_no?: string; // IRN acknowledgment number (optional)
  irn_ack_date?: string; // ISO timestamp (optional)
  irn_status: 'pending' | 'irn_generated' | 'failed' | 'cancelled';
  qr_code?: string; // Base64 encoded QR code (optional)
  filing_status: 'not_filed' | 'filed' | 'failed';
  description: string; // Default: "Payment Processing Fee"
  sac_code: string; // Default: "998599"
  taxable_value: number; // In rupees (e.g., 100.00)
  gst_rate: number; // Percentage (e.g., 18.00)
  gst_amount: number; // In rupees (e.g., 18.00)
  total_amount: number; // In rupees (e.g., 118.00)
  created_at: string; // ISO timestamp
  updated_at: string; // ISO timestamp
}

interface InvoiceDownloadParams {
  invoice_id?: number;
  payment_id?: string;
  format?: 'pdf' | 'json';
}

interface InvoiceErrorResponse {
  error: string;
}
```

---

**Document Version:** 1.0.0
**Last Updated:** 2025-11-19
**Maintained By:** XCentro Backend Team
