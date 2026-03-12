// Package progress provides progress reporting for split/merge operations.
package progress

import (
	"fmt"
	"io"
	"sync"

	"github.com/schollz/progressbar/v3"
)

// Reporter tracks progress across one or more file operations.
type Reporter interface {
	// StartFile begins tracking a single file operation.
	// totalBytes is the expected number of bytes to process.
	StartFile(name string, totalBytes int64) FileProgress
	// FinishFile marks a file operation as complete.
	FinishFile(name string, err error)
	// SetTotal sets the total number of files for batch mode.
	SetTotal(totalFiles int)
	// Close cleans up any resources.
	Close()
}

// FileProgress wraps writers/readers with byte-level progress tracking.
type FileProgress interface {
	WrapWriter(w io.Writer) io.Writer
	WrapReader(r io.Reader) io.Reader
	// Finish completes this file's progress bar.
	Finish()
}

// NopReporter is a no-op implementation used when progress is disabled.
type NopReporter struct{}

func (NopReporter) StartFile(string, int64) FileProgress { return nopFileProgress{} }
func (NopReporter) FinishFile(string, error)             {}
func (NopReporter) SetTotal(int)                         {}
func (NopReporter) Close()                               {}

type nopFileProgress struct{}

func (nopFileProgress) WrapWriter(w io.Writer) io.Writer { return w }
func (nopFileProgress) WrapReader(r io.Reader) io.Reader { return r }
func (nopFileProgress) Finish()                          {}

// BarReporter displays a progress bar on stderr using schollz/progressbar.
type BarReporter struct {
	output     io.Writer
	mu         sync.Mutex
	totalFiles int
	doneFiles  int
}

// NewBarReporter creates a reporter that writes progress bars to w (typically os.Stderr).
func NewBarReporter(w io.Writer) *BarReporter {
	return &BarReporter{output: w}
}

func (r *BarReporter) SetTotal(totalFiles int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalFiles = totalFiles
}

func (r *BarReporter) StartFile(name string, totalBytes int64) FileProgress {
	r.mu.Lock()
	defer r.mu.Unlock()

	desc := name
	if r.totalFiles > 0 {
		desc = fmt.Sprintf("[%d/%d] %s", r.doneFiles+1, r.totalFiles, name)
	}

	bar := progressbar.NewOptions64(
		totalBytes,
		progressbar.OptionSetWriter(r.output),
		progressbar.OptionSetDescription(desc),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(30),
		progressbar.OptionThrottle(0),
		progressbar.OptionOnCompletion(func() {
			_, _ = fmt.Fprintln(r.output)
		}),
	)

	return &barFileProgress{bar: bar}
}

func (r *BarReporter) FinishFile(_ string, _ error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.doneFiles++
}

func (r *BarReporter) Close() {}

// barFileProgress wraps a single progress bar for writer/reader tracking.
// Each file gets its own bar instance, safe for concurrent use.
type barFileProgress struct {
	bar *progressbar.ProgressBar
}

func (p *barFileProgress) WrapWriter(w io.Writer) io.Writer {
	return &progressWriter{inner: w, bar: p.bar}
}

func (p *barFileProgress) WrapReader(r io.Reader) io.Reader {
	return &progressReader{inner: r, bar: p.bar}
}

func (p *barFileProgress) Finish() {
	_ = p.bar.Finish()
}

// progressWriter counts bytes written and updates the progress bar.
type progressWriter struct {
	inner io.Writer
	bar   *progressbar.ProgressBar
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.inner.Write(p)
	if n > 0 {
		_ = w.bar.Add(n)
	}
	return n, err
}

// progressReader counts bytes read and updates the progress bar.
type progressReader struct {
	inner io.Reader
	bar   *progressbar.ProgressBar
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		_ = r.bar.Add(n)
	}
	return n, err
}

// OrNop returns r if non-nil, otherwise a NopReporter.
func OrNop(r Reporter) Reporter {
	if r != nil {
		return r
	}
	return NopReporter{}
}

// Ensure interface compliance.
var (
	_ Reporter     = NopReporter{}
	_ Reporter     = (*BarReporter)(nil)
	_ FileProgress = nopFileProgress{}
	_ FileProgress = (*barFileProgress)(nil)
)
