package util

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	return nil
}

type ChecksumResult struct {
	MD5    string
	SHA256 string
}

func CalculateChecksum(reader io.Reader) (*ChecksumResult, int64, error) {
	sha256Hash := sha256.New()
	md5Hash := md5.New()

	multiWriter := io.MultiWriter(sha256Hash, md5Hash)

	n, err := io.Copy(multiWriter, reader)
	if err != nil {
		return nil, 0, err
	}

	return &ChecksumResult{
		MD5:    hex.EncodeToString(md5Hash.Sum(nil)),
		SHA256: hex.EncodeToString(sha256Hash.Sum(nil)),
	}, n, nil
}
