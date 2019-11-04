// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/papercutsoftware/pdfsearch/internal/utils"
)

// loadFileDescList deserializes a file descriptor list `fdList` from json file `jsonPath` if
// `jsonPath` exists, or creates an empty list if it doesn't.
func loadFileDescList(jsonPath string) ([]fileDesc, error) {
	b, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		if !utils.Exists(jsonPath) {
			return nil, nil
		}
		return nil, err
	}
	var fdList []fileDesc
	err = json.Unmarshal(b, &fdList)
	return fdList, err
}

// saveFileDescList serializes file descriptor list `fdList` to json file `jsonPath`.
func saveFileDescList(jsonPath string, fdList []fileDesc) error {
	b, err := json.MarshalIndent(fdList, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(jsonPath, b, 0666)
}

// fileDesc describes a PDF file.
// The fields are capitalized so that this json.Unmarshal and json.MarshalIndent will work directly
// on this struct. These fields are not meant to be referenced outside this library.
type fileDesc struct {
	InPath string  // Full path to PDF file.
	Hash   string  // SHA-256 hash of file contents.
	SizeMB float64 // Size of PDF file on disk in megabytes.
}

// String returns a human readable description of `fd`.
func (fd fileDesc) String() string {
	return fmt.Sprintf("{fileDesc: %#q %.2f MB %q}", fd.Hash, fd.SizeMB, fd.InPath)
}

// createFileDesc returns the fileDesc for PDF `inPath`.
func createFileDesc(inPath string) (fileDesc, error) {
	hash, err := utils.FileHash(inPath)
	if err != nil {
		return fileDesc{}, err
	}
	size, err := utils.FileSize(inPath)
	if err != nil {
		return fileDesc{}, err
	}
	return fileDesc{
		InPath: inPath,
		Hash:   hash,
		SizeMB: float64(size) / 1024.0 / 1024.0,
	}, nil
}
