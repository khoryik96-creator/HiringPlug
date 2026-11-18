package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
)

const blobChunk = 1 << 20
const maxSourceBytes int64 = 1 << 40

type Blob struct {
	ID   string `json:"id"`
	SHA  string `json:"sha256"`
	Size int64  `json:"bytes"`
}

func (s *Store) PutBlob(ctx context.Context, r io.Reader) (Blob, error) {
	b := Blob{ID: randomID()}
	dir := filepath.Join(s.dir, "objects")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return b, err
	}
	f, err := os.OpenFile(filepath.Join(dir, b.ID+".part"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return b, err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(f.Name())
		}
	}()
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return b, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return b, err
	}
	if _, err := f.Write([]byte("HPB1")); err != nil {
		return b, err
	}
	h := sha256.New()
	buffer := make([]byte, blobChunk)
	sequence := uint64(0)
	frame := func(kind byte, plain []byte) error {
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		aad := blobAAD(b.ID, sequence, kind)
		sealed := gcm.Seal(nonce, nonce, plain, aad)
		header := make([]byte, 5)
		header[0] = kind
		binary.BigEndian.PutUint32(header[1:], uint32(len(sealed)))
		if _, err := f.Write(header); err != nil {
			return err
		}
		_, err := f.Write(sealed)
		sequence++
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return b, err
		}
		n, readErr := io.ReadFull(r, buffer)
		if n > 0 {
			b.Size += int64(n)
			if b.Size > maxSourceBytes {
				return b, errors.New("source file exceeds the one-terabyte safety bound")
			}
			_, _ = h.Write(buffer[:n])
			if err := frame(0, buffer[:n]); err != nil {
				return b, err
			}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return b, readErr
		}
	}
	final := make([]byte, 40)
	binary.BigEndian.PutUint64(final, uint64(b.Size))
	copy(final[8:], h.Sum(nil))
	if err := frame(1, final); err != nil {
		return b, err
	}
	b.SHA = hex.EncodeToString(h.Sum(nil))
	if err := f.Sync(); err != nil {
		return b, err
	}
	if err := f.Close(); err != nil {
		return b, err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, b.ID+".enc")); err != nil {
		return b, err
	}
	if _, err := s.db.Exec(`INSERT INTO blobs(id,sha,size) VALUES(?,?,?)`, b.ID, b.SHA, b.Size); err != nil {
		_ = os.Remove(filepath.Join(dir, b.ID+".enc"))
		return b, err
	}
	ok = true
	return b, nil
}

func blobAAD(id string, sequence uint64, kind byte) []byte {
	aad := append([]byte("hiringplug-extraction-v1:"+id), make([]byte, 9)...)
	binary.BigEndian.PutUint64(aad[len(aad)-9:], sequence)
	aad[len(aad)-1] = kind
	return aad
}

type blobReader struct {
	file  *os.File
	aead  cipher.AEAD
	blob  Blob
	seq   uint64
	plain *bytes.Reader
	hash  hash.Hash
	size  int64
	done  bool
}

func (s *Store) OpenBlob(id string) (io.ReadCloser, error) {
	b := Blob{ID: id}
	if err := s.db.QueryRow(`SELECT sha,size FROM blobs WHERE id=?`, id).Scan(&b.SHA, &b.Size); err != nil {
		return nil, err
	}
	if !archiveKey.MatchString(id) || len(id) != 64 {
		return nil, errors.New("invalid blob identity")
	}
	f, err := os.Open(filepath.Join(s.dir, "objects", id+".enc"))
	if err != nil {
		return nil, err
	}
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil || string(magic) != "HPB1" {
		_ = f.Close()
		return nil, errors.New("invalid encrypted object")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &blobReader{file: f, aead: gcm, blob: b, hash: sha256.New()}, nil
}
func (r *blobReader) Close() error { return r.file.Close() }
func (r *blobReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if r.plain != nil && r.plain.Len() > 0 {
			return r.plain.Read(p)
		}
		if r.done {
			return 0, io.EOF
		}
		header := make([]byte, 5)
		if _, err := io.ReadFull(r.file, header); err != nil {
			return 0, errors.New("encrypted object is incomplete")
		}
		n := int(binary.BigEndian.Uint32(header[1:]))
		if n < r.aead.NonceSize()+r.aead.Overhead() || n > blobChunk+64 {
			return 0, errors.New("invalid encrypted frame")
		}
		sealed := make([]byte, n)
		if _, err := io.ReadFull(r.file, sealed); err != nil {
			return 0, errors.New("encrypted object is incomplete")
		}
		plain, err := r.aead.Open(nil, sealed[:r.aead.NonceSize()], sealed[r.aead.NonceSize():], blobAAD(r.blob.ID, r.seq, header[0]))
		if err != nil {
			return 0, errors.New("encrypted object failed verification")
		}
		r.seq++
		if header[0] == 1 {
			var extra [1]byte
			extraN, extraErr := r.file.Read(extra[:])
			if len(plain) != 40 || int64(binary.BigEndian.Uint64(plain)) != r.size || r.size != r.blob.Size || !bytes.Equal(plain[8:], r.hash.Sum(nil)) || hex.EncodeToString(r.hash.Sum(nil)) != r.blob.SHA || extraN != 0 || extraErr != io.EOF {
				return 0, errors.New("object integrity check failed")
			}
			r.done = true
			return 0, io.EOF
		}
		if header[0] != 0 || len(plain) == 0 {
			return 0, errors.New("invalid encrypted frame kind")
		}
		r.size += int64(len(plain))
		_, _ = r.hash.Write(plain)
		r.plain = bytes.NewReader(plain)
	}
}
