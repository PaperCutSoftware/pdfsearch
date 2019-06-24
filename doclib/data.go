// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.
package doclib

import (
	"bufio"
	"bytes"
	"compress/flate"
	"fmt"
	"io"

	"github.com/unidoc/unipdf/v3/common"
)

// CompressInPlace replaces `data` with its flate compressed image on success.
// On failure `data` is not changed.
func CompressInPlace(data *[]byte) error {
	tmp, err := Compress(*data)
	if err != nil {
		return err
	}
	*data = tmp
	return nil
}

// CompressInPlace replaces the flate compressed `data` with its raw data on success.
// On failure `data` is not changed.
func DecompressInPlace(data *[]byte) error {
	tmp, err := Decompress(*data)
	if err != nil {
		return err
	}
	*data = tmp
	return nil
}

// Compress returns flate compressed `data`.
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		common.Log.Error("Compress: empty data")
		return data, nil
	}
	r := bytes.NewReader(data)
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	zw, err := flate.NewWriter(w, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(zw, r)
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}

	common.Log.Info("Compress: raw=%d compressed=%d copied=%d (%s)",
		len(data), b.Len(), n, ratio(len(data), b.Len()))
	return b.Bytes(), err
}

// Decompress returns the raw data of flate compressed `data`.
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		common.Log.Error("Decompress: empty data")
		return data, nil
	}
	r := bytes.NewReader(data)
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	zr := flate.NewReader(r)
	n, err := io.Copy(w, zr)
	if err != nil {
		return nil, err
	}
	zr.Close()
	if err := w.Flush(); err != nil {
		return nil, err
	}

	common.Log.Info("Decompress: raw=%d compressed=%d copied=%d (%s)",
		b.Len(), len(data), n, ratio(b.Len(), len(data)))
	return b.Bytes(), err
}

func ratio(raw, compressed int) string {
	if raw == 0 || compressed == 0 {
		return "<undefined>"
	}
	r := float64(compressed) / float64(raw)
	return fmt.Sprintf("ratio=%.3g=%.2gx compression", r, 1.0/r)
}
