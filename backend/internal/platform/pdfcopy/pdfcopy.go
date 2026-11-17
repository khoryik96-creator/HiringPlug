// Package pdfcopy saves the PDF copy the Python worker makes of a Word file
// (a CV, a job description), the same way for every module: checked to be a
// PDF, stored under a key of its own for each job that made it, so a late or
// repeated result can be dropped (its own file deleted) without touching the
// copy another result already made live.
package pdfcopy

import (
	"encoding/base64"
	"strings"

	"github.com/oapi-codegen/nullable"
)

// Status of a copy, as the contract spells it.
const (
	Ready  = "ready"
	Failed = "failed"
)

// Putter stores a file under a key (blob.Store).
type Putter interface {
	Put(key string, data []byte) error
}

// Result is what a worker's answer comes to: the status to save, the key of
// the stored PDF (Ready only) and why it failed (Failed only).
type Result struct {
	Status string
	Key    *string
	Error  *string
}

// Save stores the worker's PDF for record id (made by job jobID): Ready with
// its key, or Failed with the reason.
func Save(files Putter, tenantID, id, jobID string, pdfBase64, workerError nullable.Nullable[string]) (Result, error) {
	out := Result{Status: Failed}
	if msg, err := workerError.Get(); err == nil && msg != "" {
		out.Error = &msg
	}
	if !pdfBase64.IsSpecified() || pdfBase64.IsNull() {
		return out, nil // no PDF: the worker's failure is recorded, not an error here
	}
	pdf, ok := decodePDF(pdfBase64.MustGet())
	if !ok {
		msg := "the PDF could not be read"
		out.Error = &msg
		return out, nil
	}
	key := tenantID + "/" + id + "_pdf_" + jobID
	if err := files.Put(key, pdf); err != nil {
		return out, err
	}
	return Result{Status: Ready, Key: &key}, nil
}

// decodePDF reads the worker's base64 PDF; false when it is not a PDF.
func decodePDF(encoded string) ([]byte, bool) {
	pdf, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !strings.HasPrefix(string(pdf), "%PDF-") {
		return nil, false
	}
	return pdf, true
}

// FileName is a download name for a file's PDF copy: "JD.docx" -> "JD.pdf".
func FileName(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	return name + ".pdf"
}
