// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"path/filepath"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/analysis/lang/en"
	"github.com/blevesearch/bleve/index/scorch"
	"github.com/blevesearch/bleve/mapping"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
	"github.com/unidoc/unipdf/v3/common"
)

func reopenBleve(index bleve.Index) (bleve.Index, error) {
	indexPath := index.Name()
	err := index.Close()
	if err != nil {
		return nil, err
	}
	index, err = bleve.Open(indexPath)
	if err != nil {
		common.Log.Error("could not reopen Bleve index %q err=%v", indexPath, err)
	}
	return index, nil
}

// createBleveDiskIndex creates a new persistent bleve index at `indexPath`.
// If `forceCreate` is true then an existing index will be deleted.
func createBleveDiskIndex(indexPath string, forceCreate bool) (bleve.Index, error) {
	common.Log.Info("Opening existing %q.", indexPath)
	exists := utils.Exists(indexPath)
	common.Log.Info("exists=%t", exists)
	index, err := bleve.Open(indexPath)
	// if err != nil {
	// 	panic(fmt.Errorf("Could not open Bleve index %q err=%v", indexPath, err))
	// }
	if err != nil {
		mapping := buildIndexMapping()
		index, err = bleve.NewUsing(indexPath, mapping, scorch.Name, scorch.Name, nil)
		common.Log.Info("createBleveDiskIndex(%q, %t) returned name=%q err=%v",
			indexPath, forceCreate, index.Name(), err)
		if err == bleve.ErrorIndexPathExists {
			common.Log.Error("Bleve index %q exists.", indexPath)
			if forceCreate {
				common.Log.Info("Removing %q.", indexPath)
				removeBleveDiskIndex(indexPath)
				index, err = bleve.New(indexPath, mapping)
			} else {
				common.Log.Info("Opening existing Bleve index %q.", indexPath)
				index, err = bleve.Open(indexPath)
			}
		} else if err != nil {
			index, err = reopenBleve(index)
			if err != nil {
				common.Log.Error("reopenBleve failed err=%v", err)
			}
		}
	}
	common.Log.Info("createBleveDiskIndex(%q, %t) returned  err=%v",
		indexPath, forceCreate, err)
	if index != nil {
		docCount, err := index.DocCount()
		common.Log.Info("Bleve index=%q DocCount=%d err=%v", index.Name(), docCount, err)
	}
	// panic("here")
	return index, err
}

// createBleveMemIndex creates a new in-memory (unpersisted) bleve index.
func createBleveMemIndex() (bleve.Index, error) {
	mapping := buildIndexMapping()
	return bleve.NewMemOnly(mapping)
}

// createBleveMemIndex creates a new in-memory (unpersisted) bleve index.
func closeIndex(index bleve.Index) error {
	return index.Close()
}

// buildIndexMapping is from the bleve beer example code.
// It returns an IndexMapping that gives an English text Analyer of the Text field
func buildIndexMapping() mapping.IndexMapping {
	// a generic reusable mapping for english text
	englishTextFieldMapping := bleve.NewTextFieldMapping()
	englishTextFieldMapping.Analyzer = en.AnalyzerName

	// // a generic reusable mapping for keyword text
	// keywordFieldMapping := bleve.NewTextFieldMapping()
	// keywordFieldMapping.Analyzer = keyword.Name

	pdfMapping := bleve.NewDocumentMapping()

	// Text
	pdfMapping.AddFieldMappingsAt("Text", englishTextFieldMapping)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("pdf", pdfMapping)
	indexMapping.TypeField = "type"
	indexMapping.DefaultAnalyzer = "en"
	return indexMapping
}

// removeBleveDiskIndex removes the bleve index persistent data in `indexPath` from disk.
func removeBleveDiskIndex(indexPath string) {
	metaPath := filepath.Join(indexPath, "index_meta.json")
	if !utils.Exists(metaPath) {
		common.Log.Error("%q doesn't appear to a be a bleve index. %q doesn't exist.",
			indexPath, metaPath)
		return
	}
	if err := utils.RemoveDirectory(indexPath); err != nil {
		common.Log.Error("RemoveDirectory(%q) failed. err=%v", indexPath, err)
	}
}
