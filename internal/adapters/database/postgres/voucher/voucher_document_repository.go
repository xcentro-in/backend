package voucher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/core/voucher"
)

// Voucher-document lifecycle queries: creation, lookup by id / hash / business,
// status transitions, listing, and ancillary queries (template aggregation,
// business resolution by GSTIN). Kept on the same PostgresVoucherRepository
// receiver so the existing WithTx plumbing continues to apply.

// CreateVoucherDocument inserts a new document record into the voucher_documents table
func (r *PostgresVoucherRepository) CreateVoucherDocument(ctx context.Context, doc *voucher.VoucherDocument) error {
	query := `
		INSERT INTO xcentro.voucher_documents (
			id, bid, s3_path, doc_type, status, file_hash
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
	`

	_, err := r.querier.Exec(ctx, query,
		doc.ID, doc.BusinessID, doc.S3Path, doc.DocType, doc.Status, doc.FileHash,
	)
	if err != nil {
		return fmt.Errorf("failed to create voucher document: %w", err)
	}

	return nil
}

// GetVoucherDocumentBusinessID returns the owning business for a document or uuid.Nil
// if the row has no bid set. Returns an error only on unexpected DB failures or when
// the document does not exist.
func (r *PostgresVoucherRepository) GetVoucherDocumentBusinessID(ctx context.Context, docID uuid.UUID) (uuid.UUID, error) {
	query := `SELECT bid FROM xcentro.voucher_documents WHERE id = $1`
	var bid *uuid.UUID
	if err := r.querier.QueryRow(ctx, query, docID).Scan(&bid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("voucher document not found: %s", docID)
		}
		return uuid.Nil, fmt.Errorf("failed to get document business id: %w", err)
	}
	if bid == nil {
		return uuid.Nil, nil
	}
	return *bid, nil
}

// ListQueuedDocumentsOlderThan returns documents that have been sitting in QUEUED status for longer
// than the given threshold. Used by the extraction-queue reconciliation sweeper.
func (r *PostgresVoucherRepository) ListQueuedDocumentsOlderThan(ctx context.Context, staleness time.Duration, limit int) ([]*voucher.VoucherDocument, error) {
	query := `
		SELECT id, COALESCE(doc_type, ''), status, s3_path, bid
		FROM xcentro.voucher_documents
		WHERE status = 'QUEUED'
		  AND created_at < NOW() - ($1 || ' seconds')::interval
		ORDER BY created_at ASC
		LIMIT $2
	`
	rows, err := r.querier.Query(ctx, query, fmt.Sprintf("%d", int(staleness.Seconds())), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale queued documents: %w", err)
	}
	defer rows.Close()

	var docs []*voucher.VoucherDocument
	for rows.Next() {
		var doc voucher.VoucherDocument
		if err := rows.Scan(&doc.ID, &doc.DocType, &doc.Status, &doc.S3Path, &doc.BusinessID); err != nil {
			return nil, fmt.Errorf("failed to scan stale queued document: %w", err)
		}
		docs = append(docs, &doc)
	}
	return docs, nil
}

// GetDocumentByFileHash looks up a document by its SHA-256 file hash.
// Returns nil, nil when no match is found.
func (r *PostgresVoucherRepository) GetDocumentByFileHash(ctx context.Context, fileHash string) (*voucher.VoucherDocument, error) {
	query := `
		SELECT id, bid, s3_path, doc_type, status, COALESCE(file_hash, '')
		FROM xcentro.voucher_documents
		WHERE file_hash = $1
		LIMIT 1
	`

	var doc voucher.VoucherDocument
	var docType sql.NullString
	err := r.querier.QueryRow(ctx, query, fileHash).Scan(
		&doc.ID, &doc.BusinessID, &doc.S3Path, &docType, &doc.Status, &doc.FileHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lookup document by file hash: %w", err)
	}
	if docType.Valid {
		doc.DocType = docType.String
	}
	return &doc, nil
}

// GetVoucherDocumentByID retrieves a voucher document by its ID
func (r *PostgresVoucherRepository) GetVoucherDocumentByID(ctx context.Context, docID uuid.UUID) (*voucher.VoucherDocument, error) {
	query := `
		SELECT id, bid, s3_path, doc_type, status, raw_ocr, draft_data, i2d_template, processed_by, validated_data, created_at
		FROM xcentro.voucher_documents
		WHERE id = $1
	`

	var doc voucher.VoucherDocument
	var docType sql.NullString
	var draftData, validatedData []byte
	var i2dTemplate, processedBy, rawOCR sql.NullString

	err := r.querier.QueryRow(ctx, query, docID).Scan(
		&doc.ID,
		&doc.BusinessID,
		&doc.S3Path,
		&docType,
		&doc.Status,
		&rawOCR,
		&draftData,
		&i2dTemplate,
		&processedBy,
		&validatedData,
		&doc.UploadedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("voucher document not found: %s", docID)
		}
		return nil, fmt.Errorf("failed to get voucher document: %w", err)
	}

	if docType.Valid {
		doc.DocType = docType.String
	}
	if i2dTemplate.Valid {
		doc.I2DTemplate = i2dTemplate.String
	}
	if processedBy.Valid {
		doc.ProcessedBy = processedBy.String
	}
	if rawOCR.Valid {
		doc.RawOCR = rawOCR.String
	}
	if draftData != nil {
		if err := json.Unmarshal(draftData, &doc.DraftData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal draft_data: %w", err)
		}
	}
	if validatedData != nil {
		if err := json.Unmarshal(validatedData, &doc.ValidatedData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal validated_data: %w", err)
		}
	}

	return &doc, nil
}

// DeleteVoucherDocument removes a voucher document record from the database.
func (r *PostgresVoucherRepository) DeleteVoucherDocument(ctx context.Context, docID uuid.UUID) error {
	query := `DELETE FROM xcentro.voucher_documents WHERE id = $1`
	ct, err := r.querier.Exec(ctx, query, docID)
	if err != nil {
		return fmt.Errorf("failed to delete voucher document: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("voucher document not found: %s", docID)
	}
	return nil
}

// UpdateVoucherDocumentStatus updates only the status column on a voucher document.
func (r *PostgresVoucherRepository) UpdateVoucherDocumentStatus(ctx context.Context, docID uuid.UUID, status voucher.DocumentStatus) error {
	query := `UPDATE xcentro.voucher_documents SET status = $2, updated_at = NOW() WHERE id = $1`
	ct, err := r.querier.Exec(ctx, query, docID, string(status))
	if err != nil {
		return fmt.Errorf("failed to update voucher document status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("voucher document not found: %s", docID)
	}
	return nil
}

// UpdateVoucherDocumentValidation updates the validated data and status of a voucher document
func (r *PostgresVoucherRepository) UpdateVoucherDocumentValidation(ctx context.Context, docID uuid.UUID, status voucher.DocumentStatus, validatedData map[string]interface{}) error {
	validatedJSON, err := json.Marshal(validatedData)
	if err != nil {
		return fmt.Errorf("failed to marshal validated_data: %w", err)
	}

	query := `
		UPDATE xcentro.voucher_documents
		SET status = $2, validated_data = $3
		WHERE id = $1
	`

	result, err := r.querier.Exec(ctx, query, docID, status, validatedJSON)
	if err != nil {
		return fmt.Errorf("failed to update voucher document validation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("voucher document not found: %s", docID)
	}

	return nil
}

// ListVoucherDocuments retrieves paginated list of voucher documents
func (r *PostgresVoucherRepository) ListVoucherDocuments(ctx context.Context, filter voucher.DocumentListFilter) ([]*voucher.VoucherDocument, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.BusinessID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("bid = $%d", argPos))
		args = append(args, *filter.BusinessID)
		argPos++
	}

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *filter.Status)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM xcentro.voucher_documents WHERE %s", whereClause)
	var total int64
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count voucher documents: %w", err)
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
		SELECT id, bid, s3_path, COALESCE(doc_type, ''), status, raw_ocr, draft_data, COALESCE(i2d_template, ''), COALESCE(processed_by, ''), validated_data, created_at
		FROM xcentro.voucher_documents
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query voucher documents: %w", err)
	}
	defer rows.Close()

	var docs []*voucher.VoucherDocument
	for rows.Next() {
		var doc voucher.VoucherDocument
		var rawOCR sql.NullString
		var draftData, validatedData []byte

		err := rows.Scan(
			&doc.ID, &doc.BusinessID, &doc.S3Path, &doc.DocType, &doc.Status,
			&rawOCR, &draftData, &doc.I2DTemplate, &doc.ProcessedBy, &validatedData,
			&doc.UploadedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan voucher document: %w", err)
		}

		if rawOCR.Valid {
			doc.RawOCR = rawOCR.String
		}
		if len(draftData) > 0 {
			json.Unmarshal(draftData, &doc.DraftData)
		}
		if len(validatedData) > 0 {
			json.Unmarshal(validatedData, &doc.ValidatedData)
		}

		docs = append(docs, &doc)
	}

	return docs, total, nil
}

// GetBusinessIDByGSTIN looks up a business by its GSTIN and returns the bid.
// Returns empty string if not found, error only on query failures.
func (r *PostgresVoucherRepository) GetBusinessIDByGSTIN(ctx context.Context, gstin string) (string, error) {
	query := `SELECT bid::text FROM xcentro.business WHERE gstin = $1 LIMIT 1`

	var bid string
	err := r.querier.QueryRow(ctx, query, gstin).Scan(&bid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to lookup business by GSTIN: %w", err)
	}
	return bid, nil
}

// GetValidatedDocumentsByTemplate retrieves validated_data for recent documents using a given i2d template.
func (r *PostgresVoucherRepository) GetValidatedDocumentsByTemplate(ctx context.Context, templateName string, limit int) ([]*voucher.VoucherDocument, error) {
	query := `
		SELECT id, s3_path, doc_type, status, validated_data, i2d_template, processed_by
		FROM xcentro.voucher_documents
		WHERE i2d_template = $1 AND status = 'VALIDATED' AND validated_data IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT $2
	`

	rows, err := r.querier.Query(ctx, query, templateName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query validated documents by template: %w", err)
	}
	defer rows.Close()

	var docs []*voucher.VoucherDocument
	for rows.Next() {
		var doc voucher.VoucherDocument
		var docType sql.NullString
		var validatedData []byte
		var i2dTemplate, processedBy sql.NullString
		if err := rows.Scan(
			&doc.ID, &doc.S3Path, &docType, &doc.Status,
			&validatedData, &i2dTemplate, &processedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan validated document: %w", err)
		}
		if docType.Valid {
			doc.DocType = docType.String
		}
		if i2dTemplate.Valid {
			doc.I2DTemplate = i2dTemplate.String
		}
		if processedBy.Valid {
			doc.ProcessedBy = processedBy.String
		}
		if len(validatedData) > 0 {
			if err := json.Unmarshal(validatedData, &doc.ValidatedData); err != nil {
				doc.ValidatedData = nil
			}
		}
		docs = append(docs, &doc)
	}
	if docs == nil {
		docs = []*voucher.VoucherDocument{}
	}
	return docs, rows.Err()
}
