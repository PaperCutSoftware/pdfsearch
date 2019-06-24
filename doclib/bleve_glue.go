// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"bytes"
	"path/filepath"

	"github.com/blevesearch/bleve"
	btreap "github.com/blevesearch/bleve/index/store/gtreap"
	"github.com/blevesearch/blevex/preload"
	"github.com/unidoc/unipdf/v3/common"
)

// CreateBleveIndex creates a new persistent Bleve index at `indexPath`.
// If `forceCreate` is true then an existing index will be deleted.
// If `allowAppend` is true then an existing index will be appended to.
// TODO: Remove `allowAppend` argument. Instead always append to an existing index if
//      `forceCreate` is false.
func CreateBleveIndex(indexPath string, forceCreate, allowAppend bool) (bleve.Index, error) {
	// Create a new index.
	mapping := bleve.NewIndexMapping()
	index, err := bleve.New(indexPath, mapping)
	if err == bleve.ErrorIndexPathExists {
		common.Log.Error("Bleve index %q exists.", indexPath)
		if forceCreate {
			common.Log.Info("Removing %q.", indexPath)
			removeIndex(indexPath)
			index, err = bleve.New(indexPath, mapping)
		} else if allowAppend {
			common.Log.Info("Opening existing %q.", indexPath)
			index, err = bleve.Open(indexPath)
		}
	}
	return index, err
}

// CreateBleveMemIndex creates a new in-memory (unpersisted) Bleve index.
func CreateBleveMemIndex() (bleve.Index, error) {
	// Create a new index.
	mapping := bleve.NewIndexMapping()
	index, err := bleve.NewMemOnly(mapping)
	return index, err
}

// removeIndex removes the Bleve index persistent data in `indexPath` from disk.
func removeIndex(indexPath string) {
	metaPath := filepath.Join(indexPath, "index_meta.json")
	if !Exists(metaPath) {
		common.Log.Error("%q doesn't appear to a be a Bleve index. %q doesn't exist.",
			indexPath, metaPath)
		return
	}
	if err := RemoveDirectory(indexPath); err != nil {
		common.Log.Error("RemoveDirectory(%q) failed. err=%v", indexPath, err)
	}
}

// !@#$ Move to a test module.
func TestRoundtripMem(index bleve.Index) bleve.Index {
	data, err := ExportBleveMem(index)
	if err != nil {
		panic(err)
	}
	index2, err := ImportBleveMem(data)
	if err != nil {
		panic(err)
	}
	common.Log.Info("!!!! data=%d", len(data))
	return index2
}

func ExportBleveMem(index bleve.Index) ([]byte, error) {
	var b bytes.Buffer
	w := &b
	i, _, err := index.Advanced()
	if err != nil {
		return nil, err
	}
	if err = preload.ExportBleve(i, w); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func ImportBleveMem(data []byte) (bleve.Index, error) {

	index, err := bleve.NewUsing(
		"",
		bleve.NewIndexMapping(),
		bleve.Config.DefaultIndexType,
		preload.Name,
		map[string]interface{}{
			"kvStoreName_actual": btreap.Name,
			"preloadmem":         data,
		})

	return index, err
}
