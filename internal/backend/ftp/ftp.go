package ftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	goftp "github.com/jlaffaye/ftp"
	"github.com/marmos91/horcrux/internal/backend"
)

func init() {
	backend.Register("ftp", func(opts map[string]string) (backend.Backend, error) {
		host := opts["host"]
		if host == "" {
			host = opts["bucket"]
		}
		if host == "" {
			return nil, fmt.Errorf("ftp backend requires a host (e.g. ftp://host:port/path)")
		}

		port := opts["port"]
		// Parse port from host:port authority if not explicitly set
		if port == "" {
			if h, p, ok := strings.Cut(host, ":"); ok {
				host = h
				port = p
			}
		}
		if port == "" {
			port = "21"
		}

		prefix := opts["prefix"]

		return New(host, port, prefix, opts)
	})
}

// FTP implements backend.Backend using the FTP protocol.
type FTP struct {
	host     string
	port     string
	username string
	password string
	prefix   string
	useTLS   bool
}

// New creates an FTP backend.
func New(host, port, prefix string, opts map[string]string) (*FTP, error) {
	username := opts["username"]
	if username == "" {
		username = "anonymous"
	}

	return &FTP{
		host:     host,
		port:     port,
		username: username,
		password: opts["password"],
		prefix:   prefix,
		useTLS:   opts["tls"] == "true",
	}, nil
}

func (f *FTP) connect() (*goftp.ServerConn, error) {
	addr := f.host + ":" + f.port

	opts := []goftp.DialOption{goftp.DialWithTimeout(30 * time.Second)}
	if f.useTLS {
		opts = append(opts, goftp.DialWithExplicitTLS(nil))
	}

	conn, err := goftp.Dial(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to FTP %s: %w", addr, err)
	}

	if err := conn.Login(f.username, f.password); err != nil {
		_ = conn.Quit()
		return nil, fmt.Errorf("FTP login: %w", err)
	}

	return conn, nil
}

func (f *FTP) remotePath(key string) string {
	if f.prefix != "" {
		return path.Join(f.prefix, key)
	}
	return key
}

func (f *FTP) Upload(_ context.Context, localPath string, remoteKey string) error {
	conn, err := f.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Quit() }()

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer func() { _ = src.Close() }()

	remotePath := f.remotePath(remoteKey)

	// Ensure parent directory exists
	dir := path.Dir(remotePath)
	if dir != "" && dir != "." && dir != "/" {
		_ = conn.MakeDir(dir)
	}

	if err := conn.Stor(remotePath, src); err != nil {
		return fmt.Errorf("uploading via FTP: %w", err)
	}
	return nil
}

func (f *FTP) Download(_ context.Context, remoteKey string, localPath string) error {
	conn, err := f.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Quit() }()

	resp, err := conn.Retr(f.remotePath(remoteKey))
	if err != nil {
		return fmt.Errorf("downloading via FTP: %w", err)
	}
	defer func() { _ = resp.Close() }()

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp); err != nil {
		return fmt.Errorf("writing downloaded data: %w", err)
	}
	return out.Close()
}

func (f *FTP) List(_ context.Context, prefix string) ([]backend.RemoteFile, error) {
	conn, err := f.connect()
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Quit() }()

	dir := f.prefix
	if prefix != "" {
		dir = path.Join(dir, prefix)
	}
	if dir == "" {
		dir = "."
	}

	entries, err := conn.List(dir)
	if err != nil {
		return nil, fmt.Errorf("listing FTP directory: %w", err)
	}

	var files []backend.RemoteFile
	for _, entry := range entries {
		if entry.Type != goftp.EntryTypeFile {
			continue
		}
		if !strings.HasSuffix(entry.Name, ".hrcx") {
			continue
		}
		// Include prefix in key so Download/Delete can resolve the full path
		key := entry.Name
		if prefix != "" {
			key = prefix + "/" + entry.Name
		}
		files = append(files, backend.RemoteFile{
			Key:  key,
			Size: int64(entry.Size),
		})
	}

	return files, nil
}

func (f *FTP) Delete(_ context.Context, remoteKey string) error {
	conn, err := f.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Quit() }()

	if err := conn.Delete(f.remotePath(remoteKey)); err != nil {
		return fmt.Errorf("deleting via FTP: %w", err)
	}
	return nil
}

var _ backend.Backend = (*FTP)(nil)
