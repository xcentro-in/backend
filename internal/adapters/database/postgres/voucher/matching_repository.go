package voucher

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
)

// Matching repository methods: mutations the matching engine performs inside
// its single-tx write path (invoice.match_status, PO remaining_balance, line
// item qty_received) plus the audit reads (match_results + joined summary for
// the ops dashboard).

// UpdateInvoiceMatchStatus updates only the match_status column on an invoice.
func (r *PostgresVoucherRepository) UpdateInvoiceMatchStatus(ctx context.Context, invoiceID uuid.UUID, status string) error {
	query := `UPDATE xcentro.invoice SET match_status = $2 WHERE id = $1`
	result, err := r.querier.Exec(ctx, query, invoiceID, status)
	if err != nil {
		return fmt.Errorf("failed to update invoice match status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("invoice not found: %s", invoiceID)
	}
	return nil
}

// UpdateInvoicePOID sets the po_id FK on an invoice after a successful match.
func (r *PostgresVoucherRepository) UpdateInvoicePOID(ctx context.Context, invoiceID uuid.UUID, poID uuid.UUID) error {
	query := `UPDATE xcentro.invoice SET po_id = $2 WHERE id = $1`
	_, err := r.querier.Exec(ctx, query, invoiceID, poID)
	if err != nil {
		return fmt.Errorf("failed to update invoice po_id: %w", err)
	}
	return nil
}

// UpdatePurchaseOrderBalance atomically updates remaining_balance and po_status on a PO.
func (r *PostgresVoucherRepository) UpdatePurchaseOrderBalance(ctx context.Context, poID uuid.UUID, remainingBalance float64, status string) error {
	query := `UPDATE xcentro.purchase_orders SET remaining_balance = $2, po_status = $3 WHERE id = $1`
	result, err := r.querier.Exec(ctx, query, poID, remainingBalance, status)
	if err != nil {
		return fmt.Errorf("failed to update PO balance: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("purchase order not found: %s", poID)
	}
	return nil
}

// UpdateLineItemQtyReceived adds to qty_received for a line item.
func (r *PostgresVoucherRepository) UpdateLineItemQtyReceived(ctx context.Context, lineItemID uuid.UUID, qtyReceived float64) error {
	query := `UPDATE xcentro.line_items SET qty_received = qty_received + $2 WHERE id = $1`
	result, err := r.querier.Exec(ctx, query, lineItemID, qtyReceived)
	if err != nil {
		return fmt.Errorf("failed to update line item qty_received: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("line item not found: %s", lineItemID)
	}
	return nil
}

// CreateMatchResult persists a matching audit record.
func (r *PostgresVoucherRepository) CreateMatchResult(ctx context.Context, result *voucher.MatchResult) error {
	detailsJSON, err := json.Marshal(result.MatchDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal match_details: %w", err)
	}

	query := `
		INSERT INTO xcentro.match_results (invoice_id, purchase_order_id, match_status, match_details)
		VALUES ($1, $2, $3, $4)
	`
	_, err = r.querier.Exec(ctx, query, result.InvoiceID, result.PurchaseOrderID, result.MatchStatus, detailsJSON)
	if err != nil {
		return fmt.Errorf("failed to create match result: %w", err)
	}
	return nil
}

// GetMatchResultsByInvoiceID retrieves all match result records for a given invoice, most recent first.
func (r *PostgresVoucherRepository) GetMatchResultsByInvoiceID(ctx context.Context, invoiceID uuid.UUID) ([]*voucher.MatchResult, error) {
	query := `
		SELECT id, invoice_id, purchase_order_id, match_status, match_details, created_at
		FROM xcentro.match_results
		WHERE invoice_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.querier.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query match results: %w", err)
	}
	defer rows.Close()

	var results []*voucher.MatchResult
	for rows.Next() {
		var mr voucher.MatchResult
		var detailsJSON []byte
		if err := rows.Scan(
			&mr.ID, &mr.InvoiceID, &mr.PurchaseOrderID,
			&mr.MatchStatus, &detailsJSON, &mr.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan match result: %w", err)
		}
		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &mr.MatchDetails); err != nil {
				mr.MatchDetails = map[string]interface{}{"raw": string(detailsJSON)}
			}
		}
		results = append(results, &mr)
	}
	if results == nil {
		results = []*voucher.MatchResult{}
	}
	return results, rows.Err()
}

// ListMatchResults retrieves a paginated, joined view of match_results + invoice + purchase_orders.
func (r *PostgresVoucherRepository) ListMatchResults(ctx context.Context, filter voucher.MatchResultListFilter) ([]*voucher.MatchResultSummary, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.BusinessID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("inv.bid = $%d", argPos))
		args = append(args, *filter.BusinessID)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM xcentro.match_results mr
		JOIN xcentro.invoice inv ON inv.id = mr.invoice_id
		LEFT JOIN xcentro.purchase_orders po ON po.id = mr.purchase_order_id
		WHERE %s
	`, whereClause)

	var total int64
	if err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count match results: %w", err)
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
		SELECT
			mr.id,
			mr.invoice_id,
			mr.purchase_order_id,
			mr.match_status,
			mr.match_details,
			mr.created_at,
			COALESCE(inv.inv_id, ''),
			inv.inv_date,
			COALESCE(inv.net_amount, 0),
			COALESCE(inv.total_amount, 0),
			inv.document_id,
			COALESCE(po.po_number, ''),
			COALESCE(po.po_date, ''),
			COALESCE(po.vendor_name, ''),
			COALESCE(po.total_amount, 0),
			COALESCE(po.remaining_balance, 0),
			COALESCE(po.po_status, ''),
			po.document_id
		FROM xcentro.match_results mr
		JOIN xcentro.invoice inv ON inv.id = mr.invoice_id
		LEFT JOIN xcentro.purchase_orders po ON po.id = mr.purchase_order_id
		WHERE %s
		ORDER BY mr.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query match results: %w", err)
	}
	defer rows.Close()

	var results []*voucher.MatchResultSummary
	for rows.Next() {
		var s voucher.MatchResultSummary
		var detailsJSON []byte
		var invDate sql.NullTime
		var invDocID *uuid.UUID
		var poDocID *uuid.UUID

		if err := rows.Scan(
			&s.MatchResultID,
			&s.InvoiceID,
			&s.PurchaseOrderID,
			&s.MatchStatus,
			&detailsJSON,
			&s.MatchedAt,
			&s.InvoiceNumber,
			&invDate,
			&s.InvoiceNetAmount,
			&s.InvoiceTotalAmount,
			&invDocID,
			&s.PONumber,
			&s.PODate,
			&s.VendorName,
			&s.POAmount,
			&s.PORemainingBalance,
			&s.POStatus,
			&poDocID,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan match result summary: %w", err)
		}

		if invDate.Valid {
			s.InvoiceDate = &invDate.Time
		}
		s.InvoiceDocumentID = invDocID
		s.PODocumentID = poDocID
		s.InvoiceAmount = s.InvoiceNetAmount
		// Default variance uses PO total amount; overridden below if snapshot balance is available
		s.Variance = s.InvoiceNetAmount - s.POAmount

		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &s.MatchNotes); err != nil {
				s.MatchNotes = map[string]interface{}{"raw": string(detailsJSON)}
			}
			// Use the snapshot remaining balance from match time if available.
			// JSON numbers unmarshal as float64 and are stored in rupees.
			if bal, ok := s.MatchNotes["po_remaining_balance_at_match"]; ok {
				if snapshot, ok := bal.(float64); ok {
					s.PORemainingBalance = snapshot
					s.Variance = s.InvoiceNetAmount - snapshot
				}
			}
		}

		results = append(results, &s)
	}
	if results == nil {
		results = []*voucher.MatchResultSummary{}
	}
	return results, total, rows.Err()
}
