// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * PaperCut specific functions.
 * !@#$ Remove this in the production release.
 */
package doclib

import (
	"strings"
)

// CleanCorpus returns `corpus` with known bad files removed.
func CleanCorpus(corpus []string) []string {
	panic("CleanCorpus")
	var cleaned []string
	for _, path := range corpus {
		keep := true
		for _, bad := range badFiles {
			if strings.Contains(path, bad) {
				keep = false
			}
		}
		if keep {
			cleaned = append(cleaned, path)
		}
	}
	return cleaned
}

var badFiles = []string{
	"bookmarks_circular.pdf",            // Stack overflow in reader
	"4865ab395ed664c3ee17.pdf",          // Stack overflow in image forms
	"circularReferencesInResources.pdf", // Stack overflow in image forms
	"mrm-icdar.pdf",                     // !@#$
	"ghmt.pdf",                          // !@#$
	"SA_implementations.pdf",            // !@#$
	"naacl06-shinyama.pdf",              // !@#$
	"a_im_",                             // !@#$
	"CaiHof-CIKM2004.pdf",
	"blurhmt.pdf",
	"ESCP-R reference_151008.pdf",
	"a_imagemask.pdf",
	"sample_chapter_verilab_aop_cookbook.pdf",
	"TWISCKeyDist.pdf",
	"ergodicity/1607.04968.pdf",
	"1812.09449.pdf",                         // hangs
	"INF586.pdf",                             // hangs
	"commercial-invoice-template-230kb.pdf",  // r=invalid pad length
	"CGU_Motor_Vehicle_Insurance_Policy.pdf", // r=invalid pad length
	"Forerunner_230_OM_EN.pdf} r=invalid",    // r=invalid pad length
	"transitions_test.pdf",                   //required attribute missing (No /Type/Font )
	"page_tree_multiple_levels.pdf",          //required attribute missing
	// "book.pdf",                               //version not found

	// // !@#$
	"/Symbolics_Common_Lis",
	// "CAM_Low Back Pain",
	// "yangbio",
}
