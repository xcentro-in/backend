package payee

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
)

// PgPayeeEmployeeRepository implements payee.PayeeEmployeeRepository using PostgreSQL
type PgPayeeEmployeeRepository struct {
	querier client.Querier
}

// NewPgPayeeEmployeeRepository creates a new PostgreSQL payee employee repository
func NewPgPayeeEmployeeRepository(db client.DB) payee.PayeeEmployeeRepository {
	return &PgPayeeEmployeeRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPayeeEmployeeRepository) WithTx(tx client.Tx) payee.PayeeEmployeeRepository {
	return &PgPayeeEmployeeRepository{
		querier: tx,
	}
}

// Create creates a new employee payee record
func (r *PgPayeeEmployeeRepository) Create(ctx context.Context, employee *payee.PayeeEmployee) error {
	query := `
		INSERT INTO xcentro.payee_employee (
			payee_id, employee_code, full_name, email, phone, designation, department,
			employment_type, date_of_joining, remuneration_doc_url, remuneration_doc_uploaded_at,
			is_sensitive, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at
	`

	// Serialize JSONB fields
	var metadataJSON []byte
	var err error

	if employee.Metadata != nil {
		metadataJSON, err = json.Marshal(employee.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err = r.querier.QueryRow(ctx, query,
		employee.PayeeID,
		employee.EmployeeCode,
		employee.FullName,
		employee.Email,
		employee.Phone,
		employee.Designation,
		employee.Department,
		employee.EmploymentType,
		employee.DateOfJoining,
		employee.RemunerationDocURL,
		employee.RemunerationDocUploadedAt,
		employee.IsSensitive,
		metadataJSON,
	).Scan(&employee.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create employee payee: %w", err)
	}

	return nil
}

// FindByID retrieves an employee payee by its payee ID
func (r *PgPayeeEmployeeRepository) FindByID(ctx context.Context, payeeID string) (*payee.PayeeEmployee, error) {
	query := `
		SELECT payee_id, employee_code, full_name, email, phone, designation, department,
		       employment_type, date_of_joining, remuneration_doc_url, remuneration_doc_uploaded_at,
		       is_sensitive, metadata, created_at, updated_at
		FROM xcentro.payee_employee
		WHERE payee_id = $1
	`

	var e payee.PayeeEmployee
	var employeeCode, email, phone, designation, department, employmentType, remunerationDocURL sql.NullString
	var dateOfJoining, remunerationDocUploadedAt sql.NullTime
	var metadataJSON []byte
	var updatedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, payeeID).Scan(
		&e.PayeeID,
		&employeeCode,
		&e.FullName,
		&email,
		&phone,
		&designation,
		&department,
		&employmentType,
		&dateOfJoining,
		&remunerationDocURL,
		&remunerationDocUploadedAt,
		&e.IsSensitive,
		&metadataJSON,
		&e.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("employee payee not found")
		}
		return nil, fmt.Errorf("failed to find employee payee: %w", err)
	}

	// Handle nullable fields
	e.EmployeeCode = employeeCode.String
	e.Email = email.String
	e.Phone = phone.String
	e.Designation = designation.String
	e.Department = department.String
	e.EmploymentType = employmentType.String
	e.RemunerationDocURL = remunerationDocURL.String
	if dateOfJoining.Valid {
		e.DateOfJoining = &dateOfJoining.Time
	}
	if remunerationDocUploadedAt.Valid {
		e.RemunerationDocUploadedAt = &remunerationDocUploadedAt.Time
	}
	if updatedAt.Valid {
		e.UpdatedAt = updatedAt.Time
	}

	// Deserialize JSONB fields
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &e.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &e, nil
}

// Update updates an employee payee record
func (r *PgPayeeEmployeeRepository) Update(ctx context.Context, employee *payee.PayeeEmployee) error {
	query := `
		UPDATE xcentro.payee_employee
		SET employee_code = $2, full_name = $3, email = $4, phone = $5, designation = $6,
		    department = $7, employment_type = $8, date_of_joining = $9,
		    remuneration_doc_url = $10, remuneration_doc_uploaded_at = $11, metadata = $12
		WHERE payee_id = $1
	`

	// Serialize JSONB fields
	var metadataJSON []byte
	var err error

	if employee.Metadata != nil {
		metadataJSON, err = json.Marshal(employee.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.querier.Exec(ctx, query,
		employee.PayeeID,
		employee.EmployeeCode,
		employee.FullName,
		employee.Email,
		employee.Phone,
		employee.Designation,
		employee.Department,
		employee.EmploymentType,
		employee.DateOfJoining,
		employee.RemunerationDocURL,
		employee.RemunerationDocUploadedAt,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update employee payee: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("employee payee not found")
	}

	return nil
}

// UpdateRemunerationDocumentKey updates the remuneration document S3 key, timestamp, and verification status
func (r *PgPayeeEmployeeRepository) UpdateRemunerationDocumentKey(ctx context.Context, payeeID, s3Key string) error {
	query := `
		UPDATE xcentro.payee_employee
		SET remuneration_doc_url = $2,
		    remuneration_doc_uploaded_at = NOW(),
		    remuneration_doc_verified = TRUE,
		    updated_at = NOW()
		WHERE payee_id = $1
	`

	result, err := r.querier.Exec(ctx, query, payeeID, s3Key)
	if err != nil {
		return fmt.Errorf("failed to update remuneration document key: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("employee payee not found")
	}

	return nil
}

// Search searches for employee payees with filters
func (r *PgPayeeEmployeeRepository) Search(ctx context.Context, filters payee.EmployeeSearchFilters) ([]*payee.PayeeEmployee, error) {
	query := `
		SELECT payee_id, employee_code, full_name, email, phone, designation, department,
		       employment_type, date_of_joining, remuneration_doc_url, remuneration_doc_uploaded_at,
		       is_sensitive, metadata, created_at, updated_at
		FROM xcentro.payee_employee
		WHERE 1=1
	`

	args := []interface{}{}
	argIdx := 1

	// Add filters
	if filters.FullName != "" {
		query += fmt.Sprintf(" AND full_name ILIKE $%d", argIdx)
		args = append(args, "%"+filters.FullName+"%")
		argIdx++
	}

	if filters.Department != "" {
		query += fmt.Sprintf(" AND department = $%d", argIdx)
		args = append(args, filters.Department)
		argIdx++
	}

	if filters.EmploymentType != "" {
		query += fmt.Sprintf(" AND employment_type = $%d", argIdx)
		args = append(args, filters.EmploymentType)
		argIdx++
	}

	// Add ordering and pagination
	query += " ORDER BY created_at DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
		argIdx++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filters.Offset)
	}

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search employee payees: %w", err)
	}
	defer rows.Close()

	var employees []*payee.PayeeEmployee

	for rows.Next() {
		var e payee.PayeeEmployee
		var employeeCode, email, phone, designation, department, employmentType, remunerationDocURL sql.NullString
		var dateOfJoining, remunerationDocUploadedAt sql.NullTime
		var metadataJSON []byte
		var updatedAt sql.NullTime

		err := rows.Scan(
			&e.PayeeID,
			&employeeCode,
			&e.FullName,
			&email,
			&phone,
			&designation,
			&department,
			&employmentType,
			&dateOfJoining,
			&remunerationDocURL,
			&remunerationDocUploadedAt,
			&e.IsSensitive,
			&metadataJSON,
			&e.CreatedAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan employee payee: %w", err)
		}

		// Handle nullable fields
		e.EmployeeCode = employeeCode.String
		e.Email = email.String
		e.Phone = phone.String
		e.Designation = designation.String
		e.Department = department.String
		e.EmploymentType = employmentType.String
		e.RemunerationDocURL = remunerationDocURL.String
		if dateOfJoining.Valid {
			e.DateOfJoining = &dateOfJoining.Time
		}
		if remunerationDocUploadedAt.Valid {
			e.RemunerationDocUploadedAt = &remunerationDocUploadedAt.Time
		}
		if updatedAt.Valid {
			e.UpdatedAt = updatedAt.Time
		}

		// Deserialize JSONB fields
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &e.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		employees = append(employees, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating employee payees: %w", err)
	}

	return employees, nil
}

// FindByBusiness retrieves all employee payees linked to a specific business
func (r *PgPayeeEmployeeRepository) FindByBusiness(ctx context.Context, businessID string, filters payee.EmployeeSearchFilters) ([]*payee.PayeeEmployee, error) {
	query := `
		SELECT e.payee_id, e.employee_code, e.full_name, e.email, e.phone,
		       e.designation, e.department, e.employment_type, e.date_of_joining,
		       e.remuneration_doc_url, e.remuneration_doc_uploaded_at, e.is_sensitive,
		       e.metadata, e.created_at, e.updated_at
		FROM xcentro.payee_employee e
		INNER JOIN xcentro.business_payee_relation r ON e.payee_id = r.payee_id
		WHERE r.business_id = $1 AND r.is_active = TRUE
	`

	args := []interface{}{businessID}
	argIdx := 2

	// Add filters
	if filters.FullName != "" {
		query += fmt.Sprintf(" AND e.full_name ILIKE $%d", argIdx)
		args = append(args, "%"+filters.FullName+"%")
		argIdx++
	}

	if filters.Department != "" {
		query += fmt.Sprintf(" AND e.department = $%d", argIdx)
		args = append(args, filters.Department)
		argIdx++
	}

	if filters.EmploymentType != "" {
		query += fmt.Sprintf(" AND e.employment_type = $%d", argIdx)
		args = append(args, filters.EmploymentType)
		argIdx++
	}

	// Add ordering and pagination
	query += " ORDER BY e.created_at DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
		argIdx++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filters.Offset)
	}

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find employees by business: %w", err)
	}
	defer rows.Close()

	var employees []*payee.PayeeEmployee

	for rows.Next() {
		var e payee.PayeeEmployee
		var employeeCode, email, phone, designation, department, employmentType, remunerationDocURL sql.NullString
		var dateOfJoining, remunerationDocUploadedAt sql.NullTime
		var metadataJSON []byte
		var updatedAt sql.NullTime

		err := rows.Scan(
			&e.PayeeID,
			&employeeCode,
			&e.FullName,
			&email,
			&phone,
			&designation,
			&department,
			&employmentType,
			&dateOfJoining,
			&remunerationDocURL,
			&remunerationDocUploadedAt,
			&e.IsSensitive,
			&metadataJSON,
			&e.CreatedAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan employee payee: %w", err)
		}

		// Handle nullable fields
		e.EmployeeCode = employeeCode.String
		e.Email = email.String
		e.Phone = phone.String
		e.Designation = designation.String
		e.Department = department.String
		e.EmploymentType = employmentType.String
		e.RemunerationDocURL = remunerationDocURL.String
		if dateOfJoining.Valid {
			e.DateOfJoining = &dateOfJoining.Time
		}
		if remunerationDocUploadedAt.Valid {
			e.RemunerationDocUploadedAt = &remunerationDocUploadedAt.Time
		}
		if updatedAt.Valid {
			e.UpdatedAt = updatedAt.Time
		}

		// Deserialize JSONB fields
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &e.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		employees = append(employees, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating employee payees: %w", err)
	}

	return employees, nil
}
