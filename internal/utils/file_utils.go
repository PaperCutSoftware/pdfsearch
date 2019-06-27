// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
)

// RegularFile returns true if file `filename` is a regular file.
func RegularFile(filename string) (bool, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return false, err
	}
	return fi.Mode().IsRegular(), nil
}

// FileSize returns the size of file `filename` in bytes.
func FileSize(filename string) (int64, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// FileHashSize is the maximum number of hexidecimal digits returned for file hashes.
var FileHashSize = 10

// FileHash returns a hex encoded string of the SHA-256 digest of the contents of file `filename`.
func FileHash(filename string) (string, error) {
	panic("FileHash")
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return makeHash(b), nil
}

// ReaderSizeHash return the size in bytes and the hash of the file referenced by `rs`.
// The hash is hex encoded string of the SHA-256 digest of the contents of `rs`.
func ReaderSizeHash(rs io.ReadSeeker) (int64, string, error) {
	numBytes, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, "", err
	}
	b := make([]byte, numBytes)
	_, err = rs.Seek(0, io.SeekStart)
	if err != nil {
		return 0, "", err
	}
	_, err = rs.Read(b)
	if err != nil {
		return 0, "", err
	}
	return numBytes, makeHash(b), nil
}

// makeHash returns a hex encoded string of the SHA-256 digest of `b.
func makeHash(b []byte) string {
	hasher := sha256.New()
	hasher.Write(b)
	digest := hex.EncodeToString(hasher.Sum(nil))
	if FileHashSize > 0 && FileHashSize < len(digest) {
		digest = digest[:FileHashSize]
	}
	return digest
}
