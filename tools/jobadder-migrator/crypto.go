package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"golang.org/x/crypto/scrypt"
)

func derive(pass string, salt []byte) ([]byte, error) {
	if len(pass) < 16 || len(pass) > 1024 {
		return nil, errors.New("use a workspace passphrase of 16–1024 characters")
	}
	return scrypt.Key([]byte(pass), salt, 32768, 8, 1, 32)
}
func seal(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, []byte("hiringplug-ja-discovery-v1")), nil
}
func unseal(key, encrypted []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(encrypted) < aead.NonceSize() {
		return nil, errors.New("invalid encrypted record")
	}
	return aead.Open(nil, encrypted[:aead.NonceSize()], encrypted[aead.NonceSize():], []byte("hiringplug-ja-discovery-v1"))
}
