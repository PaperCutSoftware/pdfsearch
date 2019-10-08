// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"bytes"
	"path/filepath"

	"github.com/blevesearch/bleve"
	btreap "github.com/blevesearch/bleve/index/store/gtreap"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
	"github.com/unidoc/unipdf/v3/common"

	// Hack to make blevex work with our serialized in-memory bleve indexes.
	// TODO: Ask bleve to update "github.com/blevesearch/blevex with our changes
	"github.com/peterwilliams97/blevex/preload"
)

// createBleveDiskIndex creates a new persistent bleve index at `indexPath`.
// If `forceCreate` is true then an existing index will be deleted.
func createBleveDiskIndex(indexPath string, forceCreate bool) (bleve.Index, error) {
	mapping := bleve.NewIndexMapping()
	index, err := bleve.New(indexPath, mapping)
	if err == bleve.ErrorIndexPathExists {
		common.Log.Error("Bleve index %q exists.", indexPath)
		if forceCreate {
			common.Log.Info("Removing %q.", indexPath)
			removeBleveDiskIndex(indexPath)
			index, err = bleve.New(indexPath, mapping)
		} else {
			common.Log.Info("Opening existing %q.", indexPath)
			index, err = bleve.Open(indexPath)
		}
	}
	return index, err
}

// createBleveMemIndex creates a new in-memory (unpersisted) bleve index.
func createBleveMemIndex() (bleve.Index, error) {
	mapping := bleve.NewIndexMapping()
	index, err := bleve.NewMemOnly(mapping)
	return index, err
}

// removeBleveDiskIndex removes the bleve index persistent data in `indexPath` from disk.
func removeBleveDiskIndex(indexPath string) {
	metaPath := filepath.Join(indexPath, "index_meta.json")
	if !utils.Exists(metaPath) {
		common.Log.Error("%q doesn't appear to a be a Bleve index. %q doesn't exist.",
			indexPath, metaPath)
		return
	}
	if err := utils.RemoveDirectory(indexPath); err != nil {
		common.Log.Error("RemoveDirectory(%q) failed. err=%v", indexPath, err)
	}
}

// ExportBleveMem serializes bleve index `index` to a byte slice.
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

// ImportBleveMem deserializes `data` to a bleve.Index.
func ImportBleveMem(data []byte) (bleve.Index, error) {
	return bleve.NewUsing(
		"",
		bleve.NewIndexMapping(),
		bleve.Config.DefaultIndexType,
		preload.Name,
		map[string]interface{}{
			"kvStoreName_actual": btreap.Name,
			"preloadmem":         data,
		})
}
