// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.
package doclib

import (
	"bytes"
	"testing"

	"github.com/unidoc/unipdf/v3/common"
)

func TestCompress(t *testing.T) {
	for i, s := range testData {
		common.Log.Info("TestCompress: %d %d", i, len(s))
		compressRoundTrip(t, s)
		compressInplaceRoundTrip(t, s)
	}
}

func compressRoundTrip(t *testing.T, s string) {
	data := []byte(s)
	compressed, err := Compress(data)
	if err != nil {
		t.Fatalf("Compress failed. err=%v", err)
	}
	uncompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed. err=%v", err)
	}
	if !bytes.Equal(data, uncompressed) {
		t.Fatalf("data != uncompressed")
	}
}

func compressInplaceRoundTrip(t *testing.T, s string) {
	data := []byte(s)
	if err := CompressInPlace(&data); err != nil {
		t.Fatalf("CompressInPlace failed. err=%v", err)
	}
	if err := DecompressInPlace(&data); err != nil {
		t.Fatalf("DecompressInPlace failed. err=%v", err)
	}
	if !bytes.Equal([]byte(s), data) {
		t.Fatal("data != s")
	}
}

var testData = []string{
	`Investigate parallelization of text extraction and bleve indexing. Both this should be completely
parallelizable within themselves and between them. The page extraction->bleve parallelization would
be through (either a page or document) queue.`,
}
