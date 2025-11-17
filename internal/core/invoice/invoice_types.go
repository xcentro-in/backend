package invoice

// IRNStatus represents the status of Invoice Reference Number generation
type IRNStatus string

const (
	IRNStatusPending      IRNStatus = "pending"
	IRNStatusIRNGenerated IRNStatus = "irn_generated"
	IRNStatusFailed       IRNStatus = "failed"
	IRNStatusCancelled    IRNStatus = "cancelled"
)

// FilingStatus represents the invoice filing status with GST portal
type FilingStatus string

const (
	FilingStatusNotFiled FilingStatus = "not_filed"
	FilingStatusFiled    FilingStatus = "filed"
	FilingStatusFailed   FilingStatus = "failed"
)

// ExportFormat represents the export format for invoice download
type ExportFormat string

const (
	ExportFormatPDF  ExportFormat = "pdf"
	ExportFormatJSON ExportFormat = "json"
)

// IsValid checks if the IRN status is valid
func (s IRNStatus) IsValid() bool {
	switch s {
	case IRNStatusPending, IRNStatusIRNGenerated, IRNStatusFailed, IRNStatusCancelled:
		return true
	default:
		return false
	}
}

// IsValid checks if the filing status is valid
func (s FilingStatus) IsValid() bool {
	switch s {
	case FilingStatusNotFiled, FilingStatusFiled, FilingStatusFailed:
		return true
	default:
		return false
	}
}

// IsValid checks if the export format is valid
func (f ExportFormat) IsValid() bool {
	switch f {
	case ExportFormatPDF, ExportFormatJSON:
		return true
	default:
		return false
	}
}
