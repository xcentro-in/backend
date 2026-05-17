package voucher

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/voucher"
)

// Extraction repository methods: creating invoice + purchase_order records from
// validated extraction payloads, reading them back with their line items, and
// listing for the review UI. The matching engine consumes the read side of this
// file (GetInvoiceByID / GetPurchaseOrderByPONumber / line-item queries).

// CreateInvoiceFromExtraction persists validated invoice data to the existing invoice table
// and inserts line items into the line_items table within a transaction.
// Returns the generated invoice UUID.
func (r *PostgresVoucherRepository) CreateInvoiceFromExtraction(ctx context.Context, docID uuid.UUID, data map[string]interface{}) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Compute derived fields (all amounts in rupees)
	totalAmount := toFloat64(data["total_amount"])
	tds := toFloat64(data["tds_amount"])
	discount := toFloat64(data["discount"])
	netAmount := totalAmount - tds - discount
	if netAmount < 0 {
		netAmount = totalAmount // fallback if no deductions extracted
	}

	sgst := toFloat64(data["sgst"])
	cgst := toFloat64(data["cgst"])
	igst := toFloat64(data["igst"])
	subtotal := toFloat64(data["subtotal"])
	grossAmount := toFloat64(data["gross_amount"])

	// Derive gst_type from tax components
	gstType := ""
	if igst > 0 {
		gstType = "INTER_STATE"
	} else if sgst > 0 || cgst > 0 {
		gstType = "INTRA_STATE"
	}

	// 2. Insert into invoice table and return generated ID
	invoiceQuery := `
		INSERT INTO xcentro.invoice (
			document_id, inv_id, type, bid, payee_id,
			inv_date, inv_due_date, total_amount, net_amount, po_number,
			sgst, cgst, igst,
			vendor_gstin, buyer_gstin,
			match_status,
			gst_type, subtotal, gross_amount, discount,
			payment_terms, expense_category
		) VALUES (
			$1, $2, 'PURCHASE'::xcentro.invoice_type_enum,
			$3, $4,
			NULLIF($5, '')::DATE, NULLIF($6, '')::DATE, $7, $8, $9,
			$10, $11, $12,
			$13, $14,
			$15,
			NULLIF($16, ''), $17, $18, $19,
			NULLIF($20, ''), NULLIF($21, '')
		) RETURNING id
	`

	var invDate interface{} = data["invoice_date"]
	if invDateStr, ok := invDate.(string); ok && invDateStr == "" {
		invDate = nil
	}
	var invDueDate interface{} = data["invoice_due_date"]
	if invDueDateStr, ok := invDueDate.(string); ok && invDueDateStr == "" {
		invDueDate = nil
	}

	poNumber, _ := data["po_number"].(string)
	paymentTerms, _ := data["payment_terms"].(string)
	expenseCategory, _ := data["expense_category"].(string)

	var invoiceID uuid.UUID
	err = tx.QueryRow(ctx, invoiceQuery,
		docID,
		data["invoice_number"],
		parseOptionalUUID(data["business_id"]),
		parseOptionalUUID(data["payee_id"]),
		invDate,
		invDueDate,
		totalAmount,
		netAmount,
		poNumber,
		sgst,
		cgst,
		igst,
		optionalString(data["vendor_gstin"]),
		optionalString(data["buyer_gstin"]),
		voucher.InvoiceMatchPending,
		gstType,
		subtotal,
		grossAmount,
		discount,
		paymentTerms,
		expenseCategory,
	).Scan(&invoiceID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create invoice from extraction: %w", err)
	}

	// 3. Insert line items
	if err := insertLineItems(ctx, tx, nil, &invoiceID, data); err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert invoice line items: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return invoiceID, nil
}

// CreatePurchaseOrder persists validated PO data to the purchase_orders table
// and inserts line items into the line_items table within a transaction.
// Returns the generated PO UUID.
func (r *PostgresVoucherRepository) CreatePurchaseOrder(ctx context.Context, docID uuid.UUID, data map[string]interface{}) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Insert into purchase_orders table and return generated ID
	poQuery := `
		INSERT INTO xcentro.purchase_orders (
			document_id, bid, payee_id,
			po_number, po_date,
			vendor_name, vendor_gstin, buyer_gstin,
			total_amount, discount, discount_type,
			remaining_balance, po_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`

	totalAmount := toFloat64(data["total_amount"])
	discount := toFloat64(data["discount"])

	var poID uuid.UUID
	err = tx.QueryRow(ctx, poQuery,
		docID,
		parseOptionalUUID(data["business_id"]),
		parseOptionalUUID(data["payee_id"]),
		data["po_number"],
		data["invoice_date"], // po_date maps from invoice_date in draft_data
		data["vendor_name"],
		data["vendor_gstin"],
		data["buyer_gstin"],
		totalAmount,
		discount,
		data["discount_type"],
		totalAmount, // remaining_balance starts equal to total_amount
		voucher.POStatusOpen,
	).Scan(&poID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create purchase order: %w", err)
	}

	// 2. Insert line items
	if err := insertLineItems(ctx, tx, &poID, nil, data); err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert purchase order line items: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return poID, nil
}

// insertLineItems inserts line items from the data map into the line_items table.
// Exactly one of poID or invoiceID must be non-nil.
func insertLineItems(ctx context.Context, tx client.Tx, poID *uuid.UUID, invoiceID *uuid.UUID, data map[string]interface{}) error {
	rawItems, ok := data["line_items"]
	if !ok || rawItems == nil {
		return nil // no line items to insert
	}

	items, ok := rawItems.([]interface{})
	if !ok {
		return nil // unexpected shape, skip gracefully
	}

	lineItemQuery := `
		INSERT INTO xcentro.line_items (
			purchase_order_id, invoice_id,
			sku, description, quantity, unit_price,
			tax_rate, tax_amount, total_amount
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		sku, _ := item["sku"].(string)
		description, _ := item["description"].(string)
		quantity := toFloat64(item["quantity"])
		// Frontend sends "rate" (unit price), "gst_rate" (tax %), "gst_amount" (tax in rupees), "amount" (pre-tax subtotal)
		unitPrice := toFloat64(item["rate"])
		taxRate := toFloat64(item["gst_rate"])
		taxAmount := toFloat64(item["gst_amount"])
		// total_amount = pre-tax amount + GST amount
		totalAmount := toFloat64(item["amount"]) + toFloat64(item["gst_amount"])

		_, err := tx.Exec(ctx, lineItemQuery,
			poID,
			invoiceID,
			sku,
			description,
			quantity,
			unitPrice,
			taxRate,
			taxAmount,
			totalAmount,
		)
		if err != nil {
			return fmt.Errorf("failed to insert line item: %w", err)
		}
	}

	return nil
}

// toFloat64 safely converts an interface{} to float64.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// parseOptionalUUID safely extracts a UUID from an interface{} value.
// Returns nil if the value is nil, empty, or not a valid UUID string.
func parseOptionalUUID(v interface{}) *uuid.UUID {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &parsed
}

// optionalString safely extracts a string from an interface{} value.
// Returns nil if the value is nil or not a string.
func optionalString(v interface{}) *string {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}

// ListExtractedInvoices retrieves paginated list of extracted invoices
func (r *PostgresVoucherRepository) ListExtractedInvoices(ctx context.Context, filter voucher.ExtractedInvoiceListFilter) ([]*voucher.ExtractedInvoice, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.BusinessID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("bid = $%d", argPos))
		args = append(args, *filter.BusinessID)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM xcentro.invoice WHERE %s", whereClause)
	var total int64
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count extracted invoices: %w", err)
	}

	limit := filter.PageSize
	if limit == 0 {
		limit = 20
	}
	offset := (filter.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(inv_id, ''), COALESCE(type::text, ''), bid, payment_id, payee_id, inv_date, inv_due_date, COALESCE(cgst, 0), COALESCE(igst, 0), COALESCE(sgst, 0), COALESCE(total_amount, 0), COALESCE(net_amount, 0), COALESCE(po_number, ''), COALESCE(match_status, 'PENDING_MATCH'), COALESCE(currency, ''), COALESCE(invoice_url, ''), COALESCE(comments, ''), COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''), document_id, created_at, updated_at
		FROM xcentro.invoice
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query extracted invoices: %w", err)
	}
	defer rows.Close()

	var invoices []*voucher.ExtractedInvoice
	for rows.Next() {
		var inv voucher.ExtractedInvoice
		err := rows.Scan(
			&inv.ID, &inv.InvID, &inv.Type, &inv.BusinessID, &inv.PaymentID, &inv.PayeeID,
			&inv.InvDate, &inv.InvDueDate, &inv.CGST, &inv.IGST, &inv.SGST, &inv.TotalAmount,
			&inv.NetAmount, &inv.PONumber, &inv.MatchStatus,
			&inv.Currency, &inv.InvoiceURL, &inv.Comments, &inv.VendorGSTIN, &inv.BuyerGSTIN, &inv.DocumentID, &inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan extracted invoice: %w", err)
		}
		invoices = append(invoices, &inv)
	}

	return invoices, total, nil
}

// ListPurchaseOrders retrieves paginated list of purchase orders
func (r *PostgresVoucherRepository) ListPurchaseOrders(ctx context.Context, filter voucher.PurchaseOrderListFilter) ([]*voucher.PurchaseOrder, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.BusinessID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("bid = $%d", argPos))
		args = append(args, *filter.BusinessID)
		argPos++
	}

	if filter.DocumentID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("document_id = $%d", argPos))
		args = append(args, *filter.DocumentID)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM xcentro.purchase_orders WHERE %s", whereClause)
	var total int64
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count purchase orders: %w", err)
	}

	limit := filter.PageSize
	if limit == 0 {
		limit = 20
	}
	offset := (filter.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, document_id, bid, payee_id, COALESCE(po_number, ''), COALESCE(po_date, ''), COALESCE(vendor_name, ''), COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''), COALESCE(total_amount, 0), COALESCE(remaining_balance, 0), COALESCE(po_status, 'OPEN'), COALESCE(discount, 0), COALESCE(discount_type, ''), created_at, updated_at
		FROM xcentro.purchase_orders
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query purchase orders: %w", err)
	}
	defer rows.Close()

	var pos []*voucher.PurchaseOrder
	for rows.Next() {
		var po voucher.PurchaseOrder
		err := rows.Scan(
			&po.ID, &po.DocumentID, &po.BusinessID, &po.PayeeID, &po.PONumber, &po.PODate, &po.VendorName, &po.VendorGSTIN,
			&po.BuyerGSTIN, &po.TotalAmount, &po.RemainingBalance, &po.POStatus, &po.Discount, &po.DiscountType, &po.CreatedAt, &po.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan purchase order: %w", err)
		}
		pos = append(pos, &po)
	}

	return pos, total, nil
}

// GetInvoiceByID retrieves a fully-typed invoice record by its primary key.
func (r *PostgresVoucherRepository) GetInvoiceByID(ctx context.Context, invoiceID uuid.UUID) (*voucher.ExtractedInvoice, error) {
	query := `
		SELECT id, document_id,
		       COALESCE(inv_id, ''), COALESCE(type::text, ''),
		       bid, payee_id, inv_date,
		       COALESCE(total_amount, 0), COALESCE(net_amount, 0),
		       COALESCE(po_number, ''), COALESCE(match_status, 'PENDING_MATCH'),
		       COALESCE(sgst, 0), COALESCE(cgst, 0), COALESCE(igst, 0),
		       COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''),
		       created_at, updated_at
		FROM xcentro.invoice
		WHERE id = $1
	`

	var inv voucher.ExtractedInvoice
	var invDate sql.NullTime
	var bidPtr *uuid.UUID
	err := r.querier.QueryRow(ctx, query, invoiceID).Scan(
		&inv.ID, &inv.DocumentID,
		&inv.InvID, &inv.Type,
		&bidPtr, &inv.PayeeID, &invDate,
		&inv.TotalAmount, &inv.NetAmount,
		&inv.PONumber, &inv.MatchStatus,
		&inv.SGST, &inv.CGST, &inv.IGST,
		&inv.VendorGSTIN, &inv.BuyerGSTIN,
		&inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("invoice not found: %s", invoiceID)
		}
		return nil, fmt.Errorf("failed to get invoice by ID: %w", err)
	}
	if bidPtr != nil {
		inv.BusinessID = *bidPtr
	}
	if invDate.Valid {
		inv.InvDate = &invDate.Time
	}
	return &inv, nil
}

// GetPurchaseOrderByPONumber retrieves a PO by its po_number, scoped to a business.
func (r *PostgresVoucherRepository) GetPurchaseOrderByPONumber(ctx context.Context, poNumber string, businessID uuid.UUID) (*voucher.PurchaseOrder, error) {
	query := `
		SELECT id, document_id, bid, payee_id,
		       COALESCE(po_number, ''), COALESCE(po_date, ''),
		       COALESCE(vendor_name, ''), COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''),
		       COALESCE(total_amount, 0), COALESCE(remaining_balance, 0), COALESCE(po_status, 'OPEN'),
		       COALESCE(discount, 0), COALESCE(discount_type, ''),
		       created_at, updated_at
		FROM xcentro.purchase_orders
		WHERE po_number = $1 AND bid = $2
		LIMIT 1
	`

	var po voucher.PurchaseOrder
	err := r.querier.QueryRow(ctx, query, poNumber, businessID).Scan(
		&po.ID, &po.DocumentID, &po.BusinessID, &po.PayeeID,
		&po.PONumber, &po.PODate,
		&po.VendorName, &po.VendorGSTIN, &po.BuyerGSTIN,
		&po.TotalAmount, &po.RemainingBalance, &po.POStatus,
		&po.Discount, &po.DiscountType,
		&po.CreatedAt, &po.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // nil, nil = not found
		}
		return nil, fmt.Errorf("failed to get PO by po_number: %w", err)
	}
	return &po, nil
}

// getLineItemsByFilter is the shared implementation for fetching typed line items.
// For legacy rows where unit_price/tax_rate/tax_amount were stored as 0 (due to a now-fixed
// field-name mismatch), it falls back to the JSONB draft/validated data on the source document.
func (r *PostgresVoucherRepository) getLineItemsByFilter(ctx context.Context, column string, id uuid.UUID) ([]voucher.LineItemDetail, error) {
	query := fmt.Sprintf(`
		SELECT
		    li.id, li.purchase_order_id, li.invoice_id,
		    COALESCE(li.sku, ''), li.description,
		    li.quantity, COALESCE(li.qty_received, 0),
		    -- unit_price: prefer stored value; fall back to JSONB "rate" field
		    CASE WHEN li.unit_price != 0 THEN li.unit_price
		         ELSE COALESCE((jdata.jitem->>'rate')::numeric, 0)
		    END AS unit_price,
		    -- tax_rate: prefer stored value; fall back to JSONB "gst_rate"
		    CASE WHEN COALESCE(li.tax_rate, 0) != 0 THEN COALESCE(li.tax_rate, 0)
		         ELSE COALESCE((jdata.jitem->>'gst_rate')::numeric, 0)
		    END AS tax_rate,
		    -- tax_amount: prefer stored value; fall back to JSONB "gst_amount"
		    CASE WHEN COALESCE(li.tax_amount, 0) != 0 THEN COALESCE(li.tax_amount, 0)
		         ELSE COALESCE((jdata.jitem->>'gst_amount')::numeric, 0)
		    END AS tax_amount,
		    COALESCE(li.total_amount, 0)
		FROM xcentro.line_items li
		LEFT JOIN xcentro.invoice inv
		    ON inv.id = li.invoice_id
		LEFT JOIN xcentro.purchase_orders po
		    ON po.id = li.purchase_order_id
		LEFT JOIN xcentro.voucher_documents vd
		    ON vd.id = COALESCE(inv.document_id, po.document_id)
		LEFT JOIN LATERAL (
		    SELECT elem AS jitem
		    FROM jsonb_array_elements(
		        COALESCE(vd.validated_data->'line_items', vd.draft_data->'line_items')
		    ) elem
		    WHERE COALESCE(elem->>'description', '') = li.description
		    LIMIT 1
		) jdata ON true
		WHERE li.%s = $1
	`, column)

	rows, err := r.querier.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query line items (%s=%s): %w", column, id, err)
	}
	defer rows.Close()

	var items []voucher.LineItemDetail
	for rows.Next() {
		var item voucher.LineItemDetail
		err := rows.Scan(
			&item.ID, &item.PurchaseOrderID, &item.InvoiceID,
			&item.SKU, &item.Description,
			&item.Quantity, &item.QtyReceived, &item.UnitPrice,
			&item.TaxRate, &item.TaxAmount, &item.TotalAmount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan line item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetLineItemsByPurchaseOrderID retrieves all typed line items for a PO.
func (r *PostgresVoucherRepository) GetLineItemsByPurchaseOrderID(ctx context.Context, poID uuid.UUID) ([]voucher.LineItemDetail, error) {
	return r.getLineItemsByFilter(ctx, "purchase_order_id", poID)
}

// GetLineItemsByInvoiceID retrieves all typed line items for an invoice.
func (r *PostgresVoucherRepository) GetLineItemsByInvoiceID(ctx context.Context, invoiceID uuid.UUID) ([]voucher.LineItemDetail, error) {
	return r.getLineItemsByFilter(ctx, "invoice_id", invoiceID)
}

// GetInvoiceBusinessID returns the owning bid of an invoice for ownership checks.
// Returns uuid.Nil (and no error) when the invoice exists but has no bid set.
func (r *PostgresVoucherRepository) GetInvoiceBusinessID(ctx context.Context, invoiceID uuid.UUID) (uuid.UUID, error) {
	var bid *uuid.UUID
	err := r.querier.QueryRow(ctx, `SELECT bid FROM xcentro.invoice WHERE id = $1`, invoiceID).Scan(&bid)
	if err != nil {
		return uuid.Nil, fmt.Errorf("GetInvoiceBusinessID: %w", err)
	}
	if bid == nil {
		return uuid.Nil, nil
	}
	return *bid, nil
}

// GetPurchaseOrderBusinessID returns the owning bid of a PO for ownership checks.
// Returns uuid.Nil (and no error) when the PO exists but has no bid set.
func (r *PostgresVoucherRepository) GetPurchaseOrderBusinessID(ctx context.Context, poID uuid.UUID) (uuid.UUID, error) {
	var bid *uuid.UUID
	err := r.querier.QueryRow(ctx, `SELECT bid FROM xcentro.purchase_orders WHERE id = $1`, poID).Scan(&bid)
	if err != nil {
		return uuid.Nil, fmt.Errorf("GetPurchaseOrderBusinessID: %w", err)
	}
	if bid == nil {
		return uuid.Nil, nil
	}
	return *bid, nil
}

// GetOrphanInvoicesByPONumber finds invoices in ORPHAN_INVOICE status that reference
// the given po_number, scoped to a single business so the query never crosses tenant
// boundaries. po_number can hold a comma-separated list of PO references, so we split
// on commas and trim each token before comparing — a plain equality check would miss
// rows whose po_number is e.g. "PO-A, PO-B" when searching for "PO-A".
func (r *PostgresVoucherRepository) GetOrphanInvoicesByPONumber(ctx context.Context, poNumber string, businessID uuid.UUID) ([]*voucher.ExtractedInvoice, error) {
	query := `
		SELECT id, document_id,
		       COALESCE(inv_id, ''), COALESCE(type::text, ''),
		       bid, payee_id, inv_date,
		       COALESCE(total_amount, 0), COALESCE(net_amount, 0),
		       COALESCE(po_number, ''), COALESCE(match_status, 'PENDING_MATCH'),
		       COALESCE(sgst, 0), COALESCE(cgst, 0), COALESCE(igst, 0),
		       COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''),
		       created_at, updated_at
		FROM xcentro.invoice
		WHERE match_status = $2 AND bid = $3 AND EXISTS (
		    SELECT 1 FROM unnest(string_to_array(po_number, ',')) AS t(s)
		    WHERE btrim(s) = $1
		)
	`

	rows, err := r.querier.Query(ctx, query, poNumber, voucher.InvoiceMatchOrphan, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to query orphan invoices: %w", err)
	}
	defer rows.Close()

	var invoices []*voucher.ExtractedInvoice
	for rows.Next() {
		var inv voucher.ExtractedInvoice
		var invDate sql.NullTime
		var bidPtr *uuid.UUID
		err := rows.Scan(
			&inv.ID, &inv.DocumentID,
			&inv.InvID, &inv.Type,
			&bidPtr, &inv.PayeeID, &invDate,
			&inv.TotalAmount, &inv.NetAmount,
			&inv.PONumber, &inv.MatchStatus,
			&inv.SGST, &inv.CGST, &inv.IGST,
			&inv.VendorGSTIN, &inv.BuyerGSTIN,
			&inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan orphan invoice: %w", err)
		}
		if bidPtr != nil {
			inv.BusinessID = *bidPtr
		}
		if invDate.Valid {
			inv.InvDate = &invDate.Time
		}
		invoices = append(invoices, &inv)
	}
	return invoices, rows.Err()
}
