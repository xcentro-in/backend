package voucher

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// fakeDuplicateRepo implements just the two methods the upload service calls
// before it reaches S3 / redis (hash lookup and business-ID lookup). All other
// VoucherRepository methods are delegated to the embedded nil interface and
// will panic if called — a clear signal the test has strayed too far.
type fakeDuplicateRepo struct {
	voucher.VoucherRepository
	byHash map[string]*voucher.VoucherDocument
}

func (f *fakeDuplicateRepo) GetDocumentByFileHash(_ context.Context, hash string) (*voucher.VoucherDocument, error) {
	return f.byHash[hash], nil
}

func (f *fakeDuplicateRepo) CreateVoucherDocument(_ context.Context, _ *voucher.VoucherDocument) error {
	// Should not be called in the duplicate path — panic makes it obvious.
	panic("CreateVoucherDocument should not be called for a duplicate upload")
}

func newTestUploadService(repo voucher.VoucherRepository) *DocumentUploadService {
	return &DocumentUploadService{
		voucherRepo: repo,
		logger:      logger.NewConsole(logger.ErrorLevel, logger.TextFormat),
		// s3Client and redisClient are nil; tests must not reach those paths.
	}
}

// makeFileHeader builds a *multipart.FileHeader whose .Open() returns the
// supplied content bytes. The MIME content-type header is set from the
// provided contentType string; pass "" to omit it.
func makeFileHeader(filename, contentType string, content []byte) *multipart.FileHeader {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	fw, err := mw.CreatePart(h)
	if err != nil {
		panic("makeFileHeader: " + err.Error())
	}
	if _, err := fw.Write(content); err != nil {
		panic("makeFileHeader: " + err.Error())
	}
	mw.Close()

	mr := multipart.NewReader(&buf, mw.Boundary())
	form, err := mr.ReadForm(1 << 20)
	if err != nil {
		panic("makeFileHeader: " + err.Error())
	}
	files := form.File["file"]
	if len(files) == 0 {
		panic("makeFileHeader: no file part found")
	}
	return files[0]
}

func TestUploadDocument_RejectsOversizedFile(t *testing.T) {
	svc := newTestUploadService(&fakeDuplicateRepo{byHash: map[string]*voucher.VoucherDocument{}})
	req := &UploadDocumentRequest{
		BusinessID: uuid.New(),
		File: &multipart.FileHeader{
			Filename: "invoice.pdf",
			Size:     maxUploadSizeBytes + 1,
			Header:   textproto.MIMEHeader{"Content-Type": []string{"application/pdf"}},
		},
	}
	_, err := svc.UploadDocument(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
}

func TestUploadDocument_RejectsEmptyFile(t *testing.T) {
	svc := newTestUploadService(&fakeDuplicateRepo{byHash: map[string]*voucher.VoucherDocument{}})
	req := &UploadDocumentRequest{
		BusinessID: uuid.New(),
		File: &multipart.FileHeader{
			Filename: "invoice.pdf",
			Size:     0,
			Header:   textproto.MIMEHeader{"Content-Type": []string{"application/pdf"}},
		},
	}
	_, err := svc.UploadDocument(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

func TestUploadDocument_RejectsDisallowedExtension(t *testing.T) {
	svc := newTestUploadService(&fakeDuplicateRepo{byHash: map[string]*voucher.VoucherDocument{}})
	req := &UploadDocumentRequest{
		BusinessID: uuid.New(),
		File: &multipart.FileHeader{
			Filename: "malware.exe",
			Size:     1024,
			Header:   textproto.MIMEHeader{"Content-Type": []string{"application/pdf"}},
		},
	}
	_, err := svc.UploadDocument(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for disallowed extension, got nil")
	}
}

func TestUploadDocument_RejectsDisallowedMIMEType(t *testing.T) {
	svc := newTestUploadService(&fakeDuplicateRepo{byHash: map[string]*voucher.VoucherDocument{}})
	req := &UploadDocumentRequest{
		BusinessID: uuid.New(),
		File: &multipart.FileHeader{
			Filename: "invoice.pdf",
			Size:     1024,
			Header:   textproto.MIMEHeader{"Content-Type": []string{"text/html"}},
		},
	}
	_, err := svc.UploadDocument(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for disallowed MIME type, got nil")
	}
}

func TestUploadDocument_AllowsAbsentMIMEHeader(t *testing.T) {
	// A MIME type header that is absent (not empty) must not block the upload;
	// the extension check is the authoritative gate in that case.
	// The call will eventually fail when it tries to open the zero-byte
	// FileHeader — that's fine, we just want to confirm it passes the MIME check.
	svc := newTestUploadService(&fakeDuplicateRepo{byHash: map[string]*voucher.VoucherDocument{}})
	req := &UploadDocumentRequest{
		BusinessID: uuid.New(),
		File: &multipart.FileHeader{
			Filename: "invoice.pdf",
			Size:     1, // non-zero so the empty-file check passes
			Header:   textproto.MIMEHeader{}, // no Content-Type key at all
		},
	}
	_, err := svc.UploadDocument(context.Background(), req)
	// Error expected (Open will fail on zero-content header), but NOT a MIME
	// rejection error.
	if err != nil {
		msg := err.Error()
		if msg == "content type '' is not allowed; accepted: application/pdf, image/jpeg, image/png, image/webp" {
			t.Errorf("absent MIME header should not be rejected; got: %v", err)
		}
	}
}

func TestUploadDocument_DuplicateReturnsSameBusinessDoc(t *testing.T) {
	content := []byte("%PDF-1.4 minimal test content")
	fh := makeFileHeader("invoice.pdf", "application/pdf", content)

	// Compute the SHA-256 hash that the service will compute from the file bytes.
	// We don't import crypto/sha256 here — we let the service compute it and
	// confirm that it returns the existing doc rather than hitting CreateVoucherDocument.
	bid := uuid.New()
	existingDocID := uuid.New()
	existing := &voucher.VoucherDocument{
		ID:         existingDocID,
		S3Path:     "voucher_documents/existing.pdf",
		Status:     voucher.DocStatusQueued,
		BusinessID: &bid,
	}

	// We don't know the hash yet — seed the repo with a wildcard matcher by
	// overriding GetDocumentByFileHash to return the doc for any hash.
	repo := &fakeAnyHashRepo{doc: existing, bid: bid}
	svc := newTestUploadService(repo)

	req := &UploadDocumentRequest{
		BusinessID: bid,
		File:       fh,
		DocType:    "INVOICE",
	}

	resp, err := svc.UploadDocument(context.Background(), req)
	if err != nil {
		t.Fatalf("duplicate upload should not error; got %v", err)
	}
	if resp.DocumentID != existingDocID {
		t.Errorf("expected existing doc ID %s; got %s", existingDocID, resp.DocumentID)
	}
}

func TestUploadDocument_DuplicateDifferentBusinessGetsNewRecord(t *testing.T) {
	// A file whose hash matches an existing doc from another business must NOT
	// be short-circuited — the uploading tenant must get their own record.
	// The service will proceed past the hash check and eventually panic on the
	// nil S3 client; we capture that panic as proof the early-return did NOT
	// fire. A premature return of the other tenant's doc would set resp.DocumentID
	// to existingDocID and would skip the S3 call, so no panic would occur.
	content := []byte("%PDF-1.4 minimal test content")
	fh := makeFileHeader("invoice.pdf", "application/pdf", content)

	ownerBID := uuid.New()
	callerBID := uuid.New() // different business
	existingDocID := uuid.New()
	existing := &voucher.VoucherDocument{
		ID:         existingDocID,
		S3Path:     "voucher_documents/existing.pdf",
		Status:     voucher.DocStatusQueued,
		BusinessID: &ownerBID, // owned by a different business
	}

	repo := &fakeAnyHashRepo{doc: existing, bid: ownerBID}
	svc := newTestUploadService(repo)

	req := &UploadDocumentRequest{
		BusinessID: callerBID,
		File:       fh,
		DocType:    "INVOICE",
	}

	var resp *UploadDocumentResponse
	var proceededPastHashCheck bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Service tried to call the nil S3 client — it did NOT short-circuit.
				proceededPastHashCheck = true
			}
		}()
		resp, _ = svc.UploadDocument(context.Background(), req)
	}()

	if !proceededPastHashCheck {
		if resp != nil && resp.DocumentID == existingDocID {
			t.Errorf("cross-business duplicate must not return the other tenant's doc ID")
		}
	}
	// If proceededPastHashCheck is true, the cross-business isolation is correct.
}

// fakeAnyHashRepo returns the seeded document for any hash lookup.
type fakeAnyHashRepo struct {
	voucher.VoucherRepository
	doc *voucher.VoucherDocument
	bid uuid.UUID
}

func (f *fakeAnyHashRepo) GetDocumentByFileHash(_ context.Context, _ string) (*voucher.VoucherDocument, error) {
	return f.doc, nil
}

func (f *fakeAnyHashRepo) GetVoucherDocumentBusinessID(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return f.bid, nil
}

func (f *fakeAnyHashRepo) CreateVoucherDocument(_ context.Context, _ *voucher.VoucherDocument) error {
	// For the cross-business test path this will be called — allow it silently.
	return nil
}
