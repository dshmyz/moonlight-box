package util

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
)

type ChecksumReader struct {
	reader    io.Reader
	sha256    hash.Hash
	md5       hash.Hash
	written   int64
	completed bool
}

func NewChecksumReader(reader io.Reader) *ChecksumReader {
	return &ChecksumReader{
		reader: reader,
		sha256: sha256.New(),
		md5:    md5.New(),
	}
}

func (cr *ChecksumReader) Read(p []byte) (n int, err error) {
	n, err = cr.reader.Read(p)
	if n > 0 {
		cr.sha256.Write(p[:n])
		cr.md5.Write(p[:n])
		cr.written += int64(n)
	}
	if err == io.EOF {
		cr.completed = true
	}
	return n, err
}

func (cr *ChecksumReader) GetResult() *ChecksumResult {
	if !cr.completed {
		return nil
	}
	return &ChecksumResult{
		MD5:    hex.EncodeToString(cr.md5.Sum(nil)),
		SHA256: hex.EncodeToString(cr.sha256.Sum(nil)),
	}
}

func (cr *ChecksumReader) GetWrittenBytes() int64 {
	return cr.written
}
